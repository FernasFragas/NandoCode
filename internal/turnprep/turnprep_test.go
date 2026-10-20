package turnprep

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/retrievalroute"
	"github.com/FernasFragas/Nandocode/internal/semantic"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

const hallucinationPrompt = "Find the server path that updates session models, verify whether cloud-listed models can actually be selected and summarize the behavioral currently"

type stubSemantic struct {
	mu     sync.Mutex
	reqs   []semantic.RetrieveRequest
	res    semantic.RetrieveResult
	err    error
	status semantic.Status
}

func (s *stubSemantic) Status(context.Context, string) (semantic.Status, error) { return s.status, nil }
func (*stubSemantic) Build(context.Context, semantic.BuildRequest) (semantic.BuildReport, error) {
	return semantic.BuildReport{}, nil
}

func (*stubSemantic) Refresh(context.Context, semantic.RefreshRequest) (semantic.BuildReport, error) {
	return semantic.BuildReport{}, nil
}
func (*stubSemantic) Clear(context.Context, string) error { return nil }
func (s *stubSemantic) Retrieve(_ context.Context, req semantic.RetrieveRequest) (semantic.RetrieveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	return s.res, s.err
}

func workspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("status\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func baseRequest(root, prompt string) Request {
	return Request{
		Prompt:          prompt,
		ToolContext:     tools.DefaultContext(context.Background(), root),
		AgentConfig:     agent.Config{ContextMode: "auto", MaxOutputTokens: 4096, NumCtx: 32768, ContextReserve: 4096},
		Model:           "test-model",
		ContextMode:     "auto",
		MaxOutputTokens: 4096,
	}
}

func fields(in agent.Input) string {
	return fmt.Sprintf("intent=%s attach=%s hist=%s tool=%s action=%s reason=%s profile=%s orig=%q msgs=%d pack=%t",
		in.PromptIntent, in.AttachmentPolicy, in.HistoryPolicy, in.ToolMode, in.RouteAction, in.RouteReason,
		in.RouteProfile, in.OriginalUserText, len(in.Messages), in.EvidencePack != nil)
}

// The table below is the agent.Input each surface produced before the
// extraction (see the *_turnprep_char_test.go files in cli, server and tui).
func TestPrepareReproducesSurfaceInputs(t *testing.T) {
	root := workspace(t)
	prompts := []string{"Respond with exactly: ok", "what is the current status of @plan.md", "list the files in @pkg/", "fix authentication flow", hallucinationPrompt}
	line := func(intent, attach, hist, tool, action, reason, profile, orig string) string {
		return fmt.Sprintf("intent=%s attach=%s hist=%s tool=%s action=%s reason=%s profile=%s orig=%q msgs=1 pack=true",
			intent, attach, hist, tool, action, reason, profile, orig)
	}
	status := func(p string) string {
		return line("file_status", "content", "latest_only", "default", "use_explicit_context_only", "skip_explicit_context", "file_status", p)
	}
	listing := func(p string) string {
		return line("directory_listing", "listing_tree_only", "latest_only", "default", "use_local_search_only", "skip_listing_intent", "listing_intent", p)
	}
	off := func(p string) string {
		return line("", "", "default", "default", "use_explicit_context_only", "skip_explicit_context", "semantic_off", p)
	}
	disc := func(p string) string {
		return line("", "", "default", "default", "use_semantic_full", "run_workspace_discovery", "workspace_discovery", p)
	}
	surfaces := map[string]struct {
		routing Routing
		want    []string
	}{
		"server_and_tui": {RoutingAuto, []string{off(prompts[0]), status(prompts[1]), listing(prompts[2]), off(prompts[3]), off(prompts[4])}},
		"print": {RoutingStaticAuto, []string{
			line("", "", "default", "none", "skip_all_retrieval", "skip_general_prompt", "general_prompt", prompts[0]),
			status(prompts[1]), listing(prompts[2]), disc(prompts[3]), disc(prompts[4]),
		}},
	}
	for name, s := range surfaces {
		for i, p := range prompts {
			t.Run(name+"/"+p, func(t *testing.T) {
				req := baseRequest(root, p)
				req.Routing = s.routing
				got, err := Prepare(req)
				if err != nil {
					t.Fatal(err)
				}
				if f := fields(got.Input); f != s.want[i] {
					t.Fatalf("got:\n%s\nwant:\n%s", f, s.want[i])
				}
			})
		}
	}
}

func TestPrepareRoutingNoneLeavesRouteFieldsEmpty(t *testing.T) {
	req := baseRequest(workspace(t), "Respond with exactly: ok")
	req.Routing = RoutingNone
	req.PreExpanded = true
	got, err := Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	in := got.Input
	if in.ToolMode != "" || in.RouteAction != "" || in.RouteReason != "" || in.RouteProfile != "" {
		t.Fatalf("route fields set: %s", fields(in))
	}
	if in.EvidencePack != nil {
		t.Fatal("pre-expanded input must not carry an evidence pack")
	}
	if len(in.Messages) != 1 || in.Messages[0].Content != "Respond with exactly: ok" {
		t.Fatalf("messages=%#v", in.Messages)
	}
}

func TestPreparePreExpandedSkipsPacking(t *testing.T) {
	req := baseRequest(workspace(t), "review @plan.md")
	req.PreExpanded = true
	req.Input = "already expanded body"
	got, err := Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Expanded != "already expanded body" || got.Input.EvidencePack != nil {
		t.Fatalf("expanded=%q pack=%v", got.Expanded, got.Input.EvidencePack)
	}
	if got.Input.OriginalUserText != "review @plan.md" {
		t.Fatalf("original=%q", got.Input.OriginalUserText)
	}
}

func TestPrepareInjectsSemanticContextAndReportsOutcome(t *testing.T) {
	root := workspace(t)
	svc := &stubSemantic{
		status: semantic.Status{Exists: true, Compatible: true},
		res: semantic.RetrieveResult{
			Used:            true,
			RenderedContext: "<semantic_context></semantic_context>",
			Records:         []semantic.SearchHit{{}},
			Files:           []semantic.RetrievedFile{{Path: "main.go"}},
		},
	}
	cfg := semantic.DefaultConfig()
	cfg.Enabled = true
	req := baseRequest(root, "fix authentication flow")
	req.Semantic = Semantic{Service: svc, Config: cfg}
	var routed []retrievalroute.Decision
	var started int
	req.Hooks = Hooks{
		OnRoute:         func(d retrievalroute.Decision) { routed = append(routed, d) },
		OnSemanticStart: func() { started++ },
	}
	got, err := Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(routed) != 1 || started != 1 {
		t.Fatalf("routed=%d started=%d", len(routed), started)
	}
	if got.Semantic.Kind != SemanticUsed || !got.Semantic.Attempted {
		t.Fatalf("semantic=%+v", got.Semantic)
	}
	last := got.Input.Messages[len(got.Input.Messages)-1].Content
	if !strings.HasSuffix(last, "<semantic_context></semantic_context>") {
		t.Fatalf("semantic context not appended:\n%s", last)
	}
	if len(svc.reqs) != 1 || svc.reqs[0].Query != "fix authentication flow" {
		t.Fatalf("retrieve reqs=%+v", svc.reqs)
	}
}

func TestPrepareClassifiesSemanticFallbackAndError(t *testing.T) {
	cfg := semantic.DefaultConfig()
	cfg.Enabled = true
	status := semantic.Status{Exists: true, Compatible: true}
	for name, tc := range map[string]struct {
		res  semantic.RetrieveResult
		err  error
		want SemanticKind
	}{
		"fallback_reason": {res: semantic.RetrieveResult{FallbackReason: "no index"}, want: SemanticFallback},
		"hard_error":      {err: errors.New("boom"), want: SemanticError},
	} {
		t.Run(name, func(t *testing.T) {
			req := baseRequest(workspace(t), "fix authentication flow")
			req.Semantic = Semantic{Service: &stubSemantic{status: status, res: tc.res, err: tc.err}, Config: cfg}
			got, err := Prepare(req)
			if err != nil {
				t.Fatal(err)
			}
			if got.Semantic.Kind != tc.want {
				t.Fatalf("kind=%v want %v", got.Semantic.Kind, tc.want)
			}
			last := got.Input.Messages[len(got.Input.Messages)-1].Content
			if strings.Contains(last, "semantic_context") {
				t.Fatal("failed retrieval must not alter the prompt")
			}
		})
	}
}

func TestPrepareForceDeepIsHonored(t *testing.T) {
	cfg := semantic.DefaultConfig()
	cfg.Enabled = true
	svc := &stubSemantic{status: semantic.Status{Exists: true, Compatible: true}, res: semantic.RetrieveResult{FallbackReason: "x"}}
	req := baseRequest(workspace(t), "fix authentication flow")
	req.Semantic = Semantic{Service: svc, Config: cfg}
	req.ForceDeep = true
	got, err := Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Route.Profile != "deep" || got.Input.RouteProfile != "workspace_deep" {
		t.Fatalf("profile=%q request profile=%q", got.Route.Profile, got.Input.RouteProfile)
	}
}

func TestPrepareKeepsHistoryAndTrimsOnlyWhenAsked(t *testing.T) {
	history := []llm.Message{{Role: llm.RoleUser, Content: "old"}, {Role: llm.RoleAssistant, Content: "older"}}
	req := baseRequest(workspace(t), "what is the current status of @plan.md")
	req.History = history
	got, err := Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Input.Messages) != 3 || len(got.Messages) != 3 {
		t.Fatalf("input=%d messages=%d", len(got.Input.Messages), len(got.Messages))
	}
	req.TrimToLatest = true
	got, err = Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Input.Messages) != 1 || len(got.Messages) != 3 {
		t.Fatalf("trimmed input=%d messages=%d", len(got.Input.Messages), len(got.Messages))
	}
	if &history[0] == &got.Messages[0] {
		t.Fatal("history must be copied, not aliased")
	}
}

func TestPrepareDoesNotMutateCallerHistory(t *testing.T) {
	history := make([]llm.Message, 1, 8)
	history[0] = llm.Message{Role: llm.RoleUser, Content: "old"}
	req := baseRequest(workspace(t), "fix authentication flow")
	req.History = history
	if _, err := Prepare(req); err != nil {
		t.Fatal(err)
	}
	if extra := history[:2][1]; extra.Content != "" {
		t.Fatalf("caller backing array was written: %#v", extra)
	}
}

func TestPrepareSurfacesEvidenceTooLarge(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("very-large-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := baseRequest(root, "report status for @note.txt")
	req.ToolContext.MaxPromptBytes = 1
	_, err := Prepare(req)
	if err == nil {
		t.Fatal("expected too-large error")
	}
	if !IsEvidenceTooLarge(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestSemanticRouteConfigMapsLimitsAndGatesOnServiceAndEnabled(t *testing.T) {
	cfg := semantic.DefaultConfig()
	cfg.Enabled = true
	cfg.Mode = " Explicit "
	cfg.LightDeadlineMS = 120
	got := SemanticRouteConfig(cfg, true)
	if got.Mode != "explicit" || got.Light.Deadline != 120*time.Millisecond {
		t.Fatalf("cfg=%+v", got)
	}
	if SemanticRouteConfig(cfg, false).Mode != "off" {
		t.Fatal("no service must force mode off")
	}
	cfg.Enabled = false
	if SemanticRouteConfig(cfg, true).Mode != "off" {
		t.Fatal("disabled config must force mode off")
	}
	cfg.Enabled = true
	cfg.Mode = ""
	if SemanticRouteConfig(cfg, true).Mode != "auto" {
		t.Fatal("empty mode defaults to auto")
	}
}

func TestGroundingPromptAppliedOnlyWhenToolsAreAvailable(t *testing.T) {
	root := workspace(t)
	req := baseRequest(root, hallucinationPrompt)
	req.Routing = RoutingStaticAuto
	req.Grounding = true
	got, err := Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Input.SystemPrompt, GroundingMarker) || !got.Evidence.Grounded {
		t.Fatalf("grounding missing: %q", got.Input.SystemPrompt)
	}

	req = baseRequest(root, "Respond with exactly: ok")
	req.Routing = RoutingStaticAuto
	req.Grounding = true
	got, err = Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Input.ToolMode != agent.ToolModeNone || got.Input.SystemPrompt != "" || got.Evidence.Grounded {
		t.Fatalf("tool-less run must stay ungrounded: tool=%s sys=%q", got.Input.ToolMode, got.Input.SystemPrompt)
	}
}

func TestGroundingComposesWithCallerSystemPrompt(t *testing.T) {
	req := baseRequest(workspace(t), "fix authentication flow")
	req.Routing = RoutingNone
	req.PreExpanded = true
	req.SystemPrompt = "EVALUATOR RULES"
	req.Grounding = true
	got, err := Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	sp := got.Input.SystemPrompt
	if !strings.HasPrefix(sp, "EVALUATOR RULES") || !strings.Contains(sp, GroundingMarker) {
		t.Fatalf("system prompt=%q", sp)
	}
}

func TestGroundingPromptIsGroundedInRealTools(t *testing.T) {
	// The prompt may only name tools that exist, so a rename cannot leave it
	// instructing the model to call a tool it does not have.
	for _, name := range []string{"Grep", "Glob", "FileRead"} {
		if !strings.Contains(GroundingPrompt, name) {
			t.Fatalf("grounding prompt must mention %s", name)
		}
	}
}

func TestEvidenceStateSummarizesAttachments(t *testing.T) {
	req := baseRequest(workspace(t), "what is the current status of @plan.md")
	got, err := Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Evidence.AttachedFiles != 1 || got.Evidence.AttachedDirs != 0 {
		t.Fatalf("evidence=%+v", got.Evidence)
	}
}
