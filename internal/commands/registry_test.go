package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/analysis"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/observability"
	"github.com/FernasFragas/Nandocode/internal/skills"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/types"
)

type fakeLLM struct {
	models []llm.ModelInfo
}

func (f *fakeLLM) Chat(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}
func (f *fakeLLM) Embed(context.Context, string, []string) ([][]float32, error) { return nil, nil }
func (f *fakeLLM) ListModels(context.Context) ([]llm.ModelInfo, error)          { return f.models, nil }
func (f *fakeLLM) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (f *fakeLLM) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

type fakeCredStore struct {
	v string
}

func (f *fakeCredStore) Get(_, _ string) (string, error) { return f.v, nil }
func (f *fakeCredStore) Set(_, _ string, s string) error { f.v = s; return nil }
func (f *fakeCredStore) Delete(_, _ string) error        { f.v = ""; return nil }

type promptDumpLLM struct{}

func (p *promptDumpLLM) Chat(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Message: llm.Message{Content: "docs/\n- a.md"}}
	ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	close(ch)
	return ch, nil
}
func (p *promptDumpLLM) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}
func (p *promptDumpLLM) ListModels(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (p *promptDumpLLM) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (p *promptDumpLLM) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

func TestModelSwitchAndShow(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	ctx := HandlerContext{
		Store:     st,
		LLMClient: &fakeLLM{models: []llm.ModelInfo{{Name: "qwen3"}}},
	}
	out := r.Dispatch(context.Background(), "model", []string{"qwen3"}, ctx)
	if out.Kind != OutputSystem || out.Content == "" {
		t.Fatalf("unexpected output: %#v", out)
	}
	if got := st.Get().ActiveModel; got != "qwen3" {
		t.Fatalf("active model = %q, want qwen3", got)
	}
}

func TestModelCloudSwitchRequiresCredentialNonInteractive(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	local := &fakeLLM{}
	cloud := &fakeLLM{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	runtime := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434")
	rt := &modelruntime.Service{
		LocalClient:  local,
		LocalBaseURL: "http://localhost:11434",
		Runtime:      runtime,
		Resolver:     &modelresolver.Resolver{LocalClient: local, CloudClient: cloud, CloudEnabled: true},
		Creds:        &credentials.Resolver{Store: &fakeCredStore{}},
	}
	out := r.Dispatch(context.Background(), "model", []string{"gpt-oss:120b"}, HandlerContext{
		Store:        st,
		LLMClient:    runtime,
		ModelRuntime: rt,
	})
	if !strings.Contains(out.Content, "requires OLLAMA_API_KEY") {
		t.Fatalf("unexpected output: %s", out.Content)
	}
	if got := st.Get().ActiveModel; got == "gpt-oss:120b" {
		t.Fatalf("model should not have switched on missing credential")
	}
}

func TestModelsAllListsLocalAndCloud(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	local := &fakeLLM{models: []llm.ModelInfo{{Name: "qwen3.6:35b", Size: 10}}}
	cloud := &fakeLLM{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	rt := &modelruntime.Service{
		LocalClient: local,
		Resolver:    &modelresolver.Resolver{LocalClient: local, CloudClient: cloud, CloudEnabled: true},
	}
	out := r.Dispatch(context.Background(), "models", []string{"--all"}, HandlerContext{ModelRuntime: rt})
	if !strings.Contains(out.Content, "Local") || !strings.Contains(out.Content, "Ollama Cloud") {
		t.Fatalf("unexpected output: %s", out.Content)
	}
}

func TestPermissionsAllowAddsSessionRule(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	ctx := HandlerContext{Store: st}
	_ = r.Dispatch(context.Background(), "permissions", []string{"allow", "Bash(ls *)"}, ctx)
	if len(st.Get().PermissionRules.AlwaysAllow) != 1 {
		t.Fatalf("expected one allow rule")
	}
}

func TestUnknownCommandShowsAvailableList(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	out := r.Dispatch(context.Background(), "doesnotexist", nil, HandlerContext{})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected output kind: %v", out.Kind)
	}
	if out.Content == "" || out.Content[0] != '[' {
		t.Fatalf("unexpected output content: %q", out.Content)
	}
}

func TestHooksReloadRequiresConfirmation(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	out := r.Dispatch(context.Background(), "hooks", []string{"reload"}, HandlerContext{})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected output kind: %v", out.Kind)
	}
	if out.Content == "" {
		t.Fatal("expected non-empty message")
	}
}

func TestHooksListGroupedByEvent(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	snap := hooks.Snapshot{
		Hooks: []hooks.Hook{
			{Kind: hooks.KindCommand, Event: hooks.EventPreToolUse, Matcher: "Bash(*)", Source: hooks.SourceUser, Enabled: true},
			{Kind: hooks.KindPrompt, Event: hooks.EventStop, Matcher: "Bash(rm*)", Source: hooks.SourceUser, Enabled: true},
		},
	}
	out := r.Dispatch(context.Background(), "hooks", []string{"list"}, HandlerContext{HookSnapshot: &snap})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected output kind: %v", out.Kind)
	}
	if out.Content == "" {
		t.Fatal("expected hooks output")
	}
}

func TestSkillsListAndShow(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	user := t.TempDir()
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(user, "s.md"), []byte("---\nname: s1\ndescription: desc\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader, err := skills.NewLoader(user, proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	ctx := HandlerContext{SkillLoader: loader}

	listOut := r.Dispatch(context.Background(), "skills", []string{"list"}, ctx)
	if listOut.Kind != OutputSystem || !strings.Contains(listOut.Content, "s1") {
		t.Fatalf("unexpected list output: %#v", listOut)
	}
	showOut := r.Dispatch(context.Background(), "skills", []string{"show", "s1"}, ctx)
	if showOut.Kind != OutputAssistant || !strings.Contains(showOut.Content, "body") {
		t.Fatalf("unexpected show output: %#v", showOut)
	}
}

func TestSkillsShowUnknown(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	loader, err := skills.NewLoader(t.TempDir(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	out := r.Dispatch(context.Background(), "skills", []string{"show", "missing"}, HandlerContext{SkillLoader: loader})
	if out.Kind != OutputSystem || !strings.Contains(out.Content, "Unknown skill") {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestAgentsListNoTasks(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	out := r.Dispatch(context.Background(), "agents", []string{"list"}, HandlerContext{Store: st})
	if out.Kind != OutputSystem || !strings.Contains(out.Content, "No agent tasks.") {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestAgentsListFiltersOnlyAgentTasks(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	now := time.Now().UTC()
	st.Set(func(app state.App) state.App {
		app.Tasks["a-1"] = types.TaskSummary{ID: "a-1", Kind: types.KindAgent, Status: types.StatusRunning, Description: "agent", CreatedAt: now}
		app.Tasks["b-1"] = types.TaskSummary{ID: "b-1", Kind: types.KindBash, Status: types.StatusRunning, Description: "bash", CreatedAt: now.Add(time.Second)}
		app.Tasks["a-2"] = types.TaskSummary{ID: "a-2", Kind: types.KindAgent, Status: types.StatusCompleted, Description: "agent2", CreatedAt: now.Add(2 * time.Second)}
		return app
	})
	out := r.Dispatch(context.Background(), "agents", []string{"list"}, HandlerContext{Store: st})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected output kind: %v", out.Kind)
	}
	if strings.Contains(out.Content, "b-1") {
		t.Fatalf("bash task should not appear in agents list: %s", out.Content)
	}
	if !strings.Contains(out.Content, "a-1") || !strings.Contains(out.Content, "a-2") {
		t.Fatalf("expected both agent task rows: %s", out.Content)
	}
}

func TestCostUsesMeterWhenAvailable(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	meter := observability.NewMeter()
	meter.RecordAgentRun(agentUsage(2, 10, 5), time.Second, "")
	meter.RecordAgentRetry(agent.RetryNotice{Kind: "incomplete_assistant_response", Cause: "incomplete", DoneReason: "stop"})
	out := r.Dispatch(context.Background(), "cost", nil, HandlerContext{
		Meter: meter,
	})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected kind: %v", out.Kind)
	}
	if !strings.Contains(out.Content, "Session usage:") {
		t.Fatalf("unexpected cost output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Total tokens:") {
		t.Fatalf("unexpected cost output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Retries:") || !strings.Contains(out.Content, "Last retry kind:") {
		t.Fatalf("unexpected cost output: %s", out.Content)
	}
}

func TestPrepareMemoryEditFileSeedsTemplate(t *testing.T) {
	dir := t.TempDir()
	path, err := PrepareMemoryEditFile(dir, "notes")
	if err != nil {
		t.Fatalf("PrepareMemoryEditFile() error = %v", err)
	}
	if filepath.Base(path) != "notes" {
		t.Fatalf("expected notes path, got %q", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prepared file: %v", err)
	}
	content := string(body)
	if !strings.Contains(content, "name: notes") {
		t.Fatalf("expected seeded frontmatter with slug, got %q", content)
	}
}

func TestQueueListClearDrop(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	st.Set(func(app state.App) state.App {
		app.QueuedPrompts = []string{"first prompt", "second prompt"}
		return app
	})
	ctx := HandlerContext{Store: st}

	out := r.Dispatch(context.Background(), "queue", []string{"list"}, ctx)
	if out.Kind != OutputSystem || !strings.Contains(out.Content, "1. first prompt") {
		t.Fatalf("unexpected /queue list output: %#v", out)
	}

	out = r.Dispatch(context.Background(), "queue", []string{"drop", "1"}, ctx)
	if out.Kind != OutputSystem || !strings.Contains(out.Content, "[Dropped queue item]") {
		t.Fatalf("unexpected /queue drop output: %#v", out)
	}
	q := st.Get().QueuedPrompts
	if len(q) != 1 || q[0] != "second prompt" {
		t.Fatalf("unexpected queue after drop: %#v", q)
	}

	out = r.Dispatch(context.Background(), "queue", []string{"clear"}, ctx)
	if out.Kind != OutputSystem || out.Content != "[Queue cleared]" {
		t.Fatalf("unexpected /queue clear output: %#v", out)
	}
	if got := len(st.Get().QueuedPrompts); got != 0 {
		t.Fatalf("expected empty queue, got %d", got)
	}
}

func TestQueueDropIndexValidation(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	st.Set(func(app state.App) state.App {
		app.QueuedPrompts = []string{"first prompt"}
		return app
	})
	ctx := HandlerContext{Store: st}

	out := r.Dispatch(context.Background(), "queue", []string{"drop", "x"}, ctx)
	if out.Kind != OutputSystem || !strings.Contains(out.Content, "positive integer") {
		t.Fatalf("unexpected /queue drop non-integer output: %#v", out)
	}
	out = r.Dispatch(context.Background(), "queue", []string{"drop", "2"}, ctx)
	if out.Kind != OutputSystem || !strings.Contains(out.Content, "out of range") {
		t.Fatalf("unexpected /queue drop out-of-range output: %#v", out)
	}
}

func TestTraceLastUsesMeterWhenAvailable(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	st.Set(func(app state.App) state.App {
		app.ContextMode = "large"
		app.RuntimeNumCtx = 65536
		return app
	})
	meter := observability.NewMeter()
	meter.RecordRunTrace(observability.RunTrace{
		RunStartedAt:           time.Now().UTC(),
		FirstEventLatency:      10 * time.Millisecond,
		FirstAssistantLatency:  20 * time.Millisecond,
		FirstThinkingLatency:   30 * time.Millisecond,
		FirstToolStartLatency:  40 * time.Millisecond,
		FirstRetryLatency:      50 * time.Millisecond,
		CompactionStartLatency: 60 * time.Millisecond,
		CompactionEndLatency:   70 * time.Millisecond,
		TerminalLatency:        1500 * time.Millisecond,
		TerminalReason:         "completed",
		DoneReason:             "stop",
		RetryCount:             1,
		RetryKinds: map[string]int64{
			"incomplete_assistant_response": 1,
		},
		MentionMode:             "tree",
		MentionDirs:             2,
		MentionFilesDiscovered:  18,
		MentionFileBodies:       3,
		ContextMode:             "large",
		ToolMode:                "none",
		RouteAction:             "skip_all_retrieval",
		RouteReason:             "skip_general_prompt",
		RouteProfile:            "general_prompt",
		PromptPackInputBudget:   8192,
		PromptPackIncluded:      3,
		PromptPackSkipped:       2,
		PromptPackDroppedBlocks: 1,
		EvidencePacked:          true,
		EvidenceBudget:          4096,
		EvidenceFiles:           4,
		EvidenceRawBytes:        2048,
		EvidenceRawBytesOmitted: 1024,
		EvidenceExcerpted:       1,
		EvidenceOmitted:         1,
		StageLatencies: map[string]time.Duration{
			"semantic_retrieve": 1200 * time.Millisecond,
			"mention_expand":    800 * time.Millisecond,
			"memory_recall":     200 * time.Millisecond,
		},
	})
	out := r.Dispatch(context.Background(), "trace", []string{"last"}, HandlerContext{
		Store: st,
		Meter: meter,
	})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected kind: %v", out.Kind)
	}
	if !strings.Contains(out.Content, "Last run trace:") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "First tool start:") || !strings.Contains(out.Content, "Compaction completed:") || !strings.Contains(out.Content, "Terminal reason:") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Context mode:         large") || !strings.Contains(out.Content, "Effective context:    65536") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Tool mode:            none") ||
		!strings.Contains(out.Content, "Route profile:        general_prompt") ||
		!strings.Contains(out.Content, "Route action:         skip_all_retrieval") ||
		!strings.Contains(out.Content, "Route reason:         skip_general_prompt") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Diagnosis:            semantic retrieval was the main recorded cost (1.2s, 80% of terminal); retries=1") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Prompt pack budget:   8192") ||
		!strings.Contains(out.Content, "Prompt pack skipped:  2") ||
		!strings.Contains(out.Content, "Mention blocks drop:  1") {
		t.Fatalf("unexpected trace prompt-pack output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Evidence packed:      true") ||
		!strings.Contains(out.Content, "Evidence omitted raw: 1024") ||
		!strings.Contains(out.Content, "Evidence omitted:     1") {
		t.Fatalf("unexpected trace evidence output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Slowest stages:") || !strings.Contains(out.Content, "semantic_retrieve: 1.2s") || !strings.Contains(out.Content, "mention_expand: 800ms") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Stage latencies:") || !strings.Contains(out.Content, "memory_recall:") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Slow-stage threshold:") || !strings.Contains(out.Content, "source:") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
}

func TestTraceLastShowsActiveRunWhenNoTerminalTraceYet(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	meter := observability.NewMeter()
	meter.RecordCurrentRunTrace(observability.RunTrace{
		RunStartedAt:          time.Now().UTC(),
		FirstEventLatency:     11 * time.Millisecond,
		FirstAssistantLatency: 22 * time.Millisecond,
		RetryKinds:            map[string]int64{},
		StageLatencies:        map[string]time.Duration{},
	})
	out := r.Dispatch(context.Background(), "trace", []string{"last"}, HandlerContext{
		Store: state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil),
		Meter: meter,
	})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected kind: %v", out.Kind)
	}
	if !strings.Contains(out.Content, "Current run trace (active):") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "terminal trace not recorded yet") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
}

func TestPromptLastWithoutDump(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	out := r.Dispatch(context.Background(), "prompt", []string{"last"}, HandlerContext{})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected kind: %v", out.Kind)
	}
	if !strings.Contains(out.Content, "No prompt dump recorded yet") {
		t.Fatalf("unexpected output: %s", out.Content)
	}
}

func TestPromptCommandUsage(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	out := r.Dispatch(context.Background(), "prompt", nil, HandlerContext{})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected kind: %v", out.Kind)
	}
	if !strings.Contains(out.Content, "Usage: /prompt") {
		t.Fatalf("unexpected output: %s", out.Content)
	}
}

func TestPromptLastShowsListingPolicies(t *testing.T) {
	r := New()
	RegisterDefaults(r)

	a, err := agent.New(&promptDumpLLM{}, nil)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	events := a.Run(context.Background(), agent.Input{
		Model:            "qwen3",
		Messages:         []llm.Message{{Role: llm.RoleUser, Content: "User request:\nlist all files in @docs/\n\nDirectory tree data:\ndocs/\ndocs/a.md"}},
		PromptIntent:     "directory_listing",
		AttachmentPolicy: "listing_tree_only",
		OriginalUserText: "list all files in @docs/",
		HistoryPolicy:    agent.HistoryPolicyLatestOnly,
		MaxOutputTokens:  256,
	})
	for range events {
	}

	out := r.Dispatch(context.Background(), "prompt", []string{"last"}, HandlerContext{})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected kind: %v", out.Kind)
	}
	want := []string{
		"Intent:               directory_listing",
		"Attachment policy:    listing_tree_only",
		"History policy:       latest_only",
		"Memory policy:        skipped_listing_intent",
		"Retry policy:         anchored_original_request",
		"File bodies:          0",
		"Directory tree:       true",
		"Options:",
		"num_ctx:",
		"num_predict:",
	}
	for _, s := range want {
		if !strings.Contains(out.Content, s) {
			t.Fatalf("expected %q in output:\n%s", s, out.Content)
		}
	}
}

func TestPromptLastShowsEvidencePackOmissionDetails(t *testing.T) {
	r := New()
	RegisterDefaults(r)

	a, err := agent.New(&promptDumpLLM{}, nil)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	events := a.Run(context.Background(), agent.Input{
		Model: "qwen3",
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: "Original user request:\nreport @a.txt\n\nReferenced content:\nReferenced path manifest:\n- a.txt (file)",
		}},
		PromptIntent:     "file_status",
		AttachmentPolicy: "content",
		HistoryPolicy:    agent.HistoryPolicyLatestOnly,
		EvidencePack: &agent.EvidencePackReport{
			Packed:           true,
			BudgetTokens:     1200,
			FilesReferenced:  1,
			FilesExcerpted:   1,
			FilesOmitted:     1,
			RawBytesIncluded: 512,
			RawBytesOmitted:  4096,
			AnchorAdded:      true,
			LargestOmitted: []agent.OmittedEvidence{{
				Path:         "a.txt",
				Reason:       "excerpted",
				BytesOmitted: 4096,
			}},
		},
		MaxOutputTokens: 256,
	})
	for range events {
	}

	out := r.Dispatch(context.Background(), "prompt", []string{"last"}, HandlerContext{})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected kind: %v", out.Kind)
	}
	want := []string{
		"Evidence packed:      true",
		"Evidence omitted raw: 4096",
		"Omitted[1]:          a.txt (excerpted, 4096 bytes)",
	}
	for _, s := range want {
		if !strings.Contains(out.Content, s) {
			t.Fatalf("expected %q in output:\n%s", s, out.Content)
		}
	}
}

func TestTraceThresholdGetAndSet(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)

	getOut := r.Dispatch(context.Background(), "trace", []string{"threshold"}, HandlerContext{Store: st})
	if getOut.Kind != OutputSystem || !strings.Contains(getOut.Content, "Slow-stage notice threshold") || !strings.Contains(getOut.Content, "source:") {
		t.Fatalf("unexpected threshold get output: %#v", getOut)
	}

	setOut := r.Dispatch(context.Background(), "trace", []string{"threshold", "2s"}, HandlerContext{Store: st})
	if setOut.Kind != OutputSystem || !strings.Contains(setOut.Content, "Set slow-stage notice threshold") {
		t.Fatalf("unexpected threshold set output: %#v", setOut)
	}
	if st.Get().SlowStageNoticeThreshold != 2*time.Second {
		t.Fatalf("slow stage threshold=%s", st.Get().SlowStageNoticeThreshold)
	}
	if st.Get().SlowStageNoticeThresholdSource != "session" {
		t.Fatalf("slow stage threshold source=%q", st.Get().SlowStageNoticeThresholdSource)
	}
}

func TestContextStatusAndSet(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)

	status := r.Dispatch(context.Background(), "context", []string{"status"}, HandlerContext{Store: st})
	if status.Kind != OutputSystem || !strings.Contains(status.Content, "Context mode:") {
		t.Fatalf("unexpected context status output: %#v", status)
	}
	set := r.Dispatch(context.Background(), "context", []string{"large"}, HandlerContext{Store: st})
	if set.Kind != OutputSystem || !strings.Contains(set.Content, "Set context mode: large") {
		t.Fatalf("unexpected context set output: %#v", set)
	}
	if st.Get().ContextMode != "large" {
		t.Fatalf("context mode=%q", st.Get().ContextMode)
	}
}

func TestMemoryRecallModeSet(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	out := r.Dispatch(context.Background(), "memory", []string{"recall", "off"}, HandlerContext{Store: st, MemoryDir: t.TempDir()})
	if out.Kind != OutputSystem || !strings.Contains(out.Content, "Set memory recall mode: off") {
		t.Fatalf("unexpected memory recall output: %#v", out)
	}
	if st.Get().MemoryRecallMode != "off" {
		t.Fatalf("memory recall mode=%q", st.Get().MemoryRecallMode)
	}
}

func TestTraceLastShowsConfiguredThresholdSource(t *testing.T) {
	r := New()
	RegisterDefaults(r)
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	st.Set(func(app state.App) state.App {
		app.SlowStageNoticeThreshold = 1400 * time.Millisecond
		app.SlowStageNoticeThresholdSource = "project"
		return app
	})
	meter := observability.NewMeter()
	meter.RecordRunTrace(observability.RunTrace{
		RunStartedAt:    time.Now().UTC(),
		TerminalLatency: 200 * time.Millisecond,
		TerminalReason:  "completed",
	})
	out := r.Dispatch(context.Background(), "trace", []string{"last"}, HandlerContext{
		Store: st,
		Meter: meter,
	})
	if out.Kind != OutputSystem {
		t.Fatalf("unexpected kind: %v", out.Kind)
	}
	if !strings.Contains(out.Content, "Slow-stage threshold:   1.4s (source: project)") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Context mode:         auto") {
		t.Fatalf("unexpected trace output: %s", out.Content)
	}
}

func TestCheckpointStatusAndClear(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	if err := analysis.SaveCheckpoint(analysis.Checkpoint{
		Model:              "test-model",
		PendingFinalAnswer: true,
		SynthesisStage:     "map",
		InspectedFiles:     []string{"internal/tui/app.go"},
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	r := New()
	RegisterDefaults(r)
	status := r.Dispatch(context.Background(), "checkpoint", []string{"status"}, HandlerContext{})
	if status.Kind != OutputSystem || !strings.Contains(status.Content, "pending_final_answer") {
		t.Fatalf("unexpected status output: %#v", status)
	}
	clear := r.Dispatch(context.Background(), "checkpoint", []string{"clear"}, HandlerContext{})
	if clear.Kind != OutputSystem || !strings.Contains(clear.Content, "Checkpoint cleared") {
		t.Fatalf("unexpected clear output: %#v", clear)
	}
	statusAfter := r.Dispatch(context.Background(), "checkpoint", []string{"status"}, HandlerContext{})
	if statusAfter.Kind != OutputSystem || !strings.Contains(statusAfter.Content, "none") {
		t.Fatalf("unexpected status after clear: %#v", statusAfter)
	}
}

func agentUsage(turns int, prompt, completion int64) agent.Usage {
	return agent.Usage{
		Turns:           turns,
		PromptEvalCount: prompt,
		EvalCount:       completion,
		DoneReason:      "stop",
	}
}
