package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/analysis"
	"github.com/FernasFragas/nandocodego/internal/bootstrap"
	"github.com/FernasFragas/nandocodego/internal/contextpack"
	"github.com/FernasFragas/nandocodego/internal/credentials"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/llm/modelresolver"
	"github.com/FernasFragas/nandocodego/internal/llm/modelruntime"
	"github.com/FernasFragas/nandocodego/internal/mentions"
	"github.com/FernasFragas/nandocodego/internal/observability"
	"github.com/FernasFragas/nandocodego/internal/permissions"
	"github.com/FernasFragas/nandocodego/internal/state"
	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/tui/picker"
	tea "github.com/charmbracelet/bubbletea"
)

type recordingRunner struct {
	calls int
	input agent.Input
}

type noopProgramSender struct{}

func (noopProgramSender) Send(msg tea.Msg) {}

type testLLMClient struct {
	models []llm.ModelInfo
}

func (t *testLLMClient) Chat(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}
func (t *testLLMClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}
func (t *testLLMClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	out := make([]llm.ModelInfo, len(t.models))
	copy(out, t.models)
	return out, nil
}
func (t *testLLMClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (t *testLLMClient) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

type testCredStore struct {
	v string
}

func (t *testCredStore) Get(_, _ string) (string, error) { return t.v, nil }
func (t *testCredStore) Set(_, _ string, s string) error { t.v = s; return nil }
func (t *testCredStore) Delete(_, _ string) error        { t.v = ""; return nil }

func installTestModelRuntime(model *Model, localModels, cloudModels []string) {
	local := &testLLMClient{models: make([]llm.ModelInfo, 0, len(localModels))}
	for _, name := range localModels {
		local.models = append(local.models, llm.ModelInfo{Name: name})
	}
	cloud := &testLLMClient{models: make([]llm.ModelInfo, 0, len(cloudModels))}
	for _, name := range cloudModels {
		cloud.models = append(cloud.models, llm.ModelInfo{Name: name})
	}
	runtime := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434")
	model.cmdCtx.LLMClient = runtime
	model.SetModelRuntime(&modelruntime.Service{
		LocalClient:  local,
		LocalBaseURL: "http://localhost:11434",
		Runtime:      runtime,
		Resolver: &modelresolver.Resolver{
			LocalClient:  local,
			CloudClient:  cloud,
			CloudEnabled: true,
		},
		Creds: &credentials.Resolver{Store: &testCredStore{}},
	})
}

func newTestModel(t *testing.T) *Model {
	t.Helper()

	initial := bootstrap.DefaultInitial("")
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)

	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return model
}

func appendMarkdownHistory(model *Model, turns int) {
	for i := 0; i < turns; i++ {
		model.transcript = append(model.transcript, CreateUserItem(fmt.Sprintf("prompt %d", i)))
		model.transcript = append(model.transcript, TranscriptItem{
			Kind: TranscriptAssistant,
			Content: fmt.Sprintf(
				"## Reply %d\n\n%s\n\n- item %d\n- item %d",
				i,
				strings.Repeat("Historical markdown block with emphasis **bold** and wrapped text. ", 4),
				i,
				i+1,
			),
		})
	}
}

func snapshotAssistantRendered(model *Model) map[int]string {
	cached := make(map[int]string)
	for i, item := range model.transcript {
		if item.Kind == TranscriptAssistant && item.Rendered != "" {
			cached[i] = item.Rendered
		}
	}
	return cached
}

func appendAssistantTail(model *Model, content string) {
	model.transcript = AppendAssistantDelta(model.transcript, content)
	if model.heightCache != nil && len(model.transcript) > 0 {
		last := len(model.transcript) - 1
		model.heightCache[last] = estimateTranscriptItemLines(model.transcript[last])
	}
}

func TestAssistantTextDeltaAppearsInView(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)

	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model.handleAgentEvent(agent.AssistantTextDelta{Content: "How can I help you today?"})

	view := model.View()
	if !strings.Contains(view, "How can I help you today?") {
		t.Fatalf("view does not contain assistant response:\n%s", view)
	}
}

func TestTerminalNonCompletedAppearsInView(t *testing.T) {
	model := newTestModel(t)

	model.handleAgentEvent(agent.Terminal{
		Reason: agent.TerminalMaxTurns,
		Detail: "exceeded maximum turn count",
	})

	view := model.View()
	if !strings.Contains(view, "Run ended: max_turns: exceeded maximum turn count") {
		t.Fatalf("view does not contain terminal detail:\n%s", view)
	}
}

func TestStatusBarLabelsCumulativeSessionTokens(t *testing.T) {
	model := newTestModel(t)
	meter := observability.NewMeter()
	meter.RecordLLMChat(0, 0, 10, 5, "stop", nil)
	model.SetMeter(meter)

	view := model.View()
	if !strings.Contains(view, "session tokens: 15") {
		t.Fatalf("view does not label session tokens:\n%s", view)
	}
	if strings.Contains(view, "| tokens: 15") {
		t.Fatalf("view should not label cumulative meter tokens as current tokens:\n%s", view)
	}
}

func TestStatusBarShowsCoordinatorBadge(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.CoordinatorMode = true
		app.WorkerCount = 2
		return app
	})
	out := model.renderStatusBar(model.store.Get())
	if !strings.Contains(out, "[COORDINATOR]") {
		t.Fatalf("expected coordinator badge, got %q", out)
	}
	if !strings.Contains(out, "[workers: 2]") {
		t.Fatalf("expected worker count badge, got %q", out)
	}
}

func TestStatusBarShowsCloudProvider(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveModel = "gpt-oss:120b"
		app.LLMProvider = string(llm.ProviderOllamaCloudAPI)
		return app
	})
	out := model.renderStatusBar(model.store.Get())
	if !strings.Contains(out, "Provider: Ollama Cloud") {
		t.Fatalf("expected cloud provider badge, got %q", out)
	}
}

func TestModelCommandRejectedDuringActiveRun(t *testing.T) {
	model := newTestModel(t)
	installTestModelRuntime(model, []string{"qwen3.6:35b"}, []string{"gpt-oss:120b"})
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	model.input.SetValue("/model qwen3.6:35b")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		_ = cmd()
	}
	if len(model.transcript) == 0 || !strings.Contains(model.transcript[len(model.transcript)-1].Content, "cannot switch models while a run is active") {
		t.Fatalf("expected active run rejection, got %+v", model.transcript)
	}
}

func TestModelCloudCommandPromptsAndCancelLeavesState(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	model := newTestModel(t)
	installTestModelRuntime(model, []string{"qwen3.6:35b"}, []string{"gpt-oss:120b"})
	before := model.store.Get().ActiveModel

	model.input.SetValue("/model gpt-oss:120b")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected async model switch cmd")
	}
	msg := cmd()
	model.Update(msg)
	if model.cloudCredentialPrompt == nil {
		t.Fatal("expected cloud credential modal")
	}
	_, escCmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if escCmd != nil {
		model.Update(escCmd())
	}
	if model.cloudCredentialPrompt != nil {
		t.Fatal("expected credential modal to close")
	}
	after := model.store.Get().ActiveModel
	if after != before {
		t.Fatalf("model changed on cancel: before=%q after=%q", before, after)
	}
}

func TestCloudCredentialModalPasteUpdatesAPIKeyField(t *testing.T) {
	model := newTestModel(t)
	prompt := newCloudCredentialPrompt(modelSwitchNeedsCredentialMsg{
		Requested: "kimi-k2.6:cloud",
		Resolved:  llm.ResolvedModel{Model: "kimi-k2.6"},
	})
	prompt.FocusIndex = 2
	prompt.KeyInput.Blur()
	model.cloudCredentialPrompt = &prompt
	model.input.SetValue("main prompt")

	model.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("ollama-secret"),
		Paste: true,
	})

	if model.cloudCredentialPrompt == nil {
		t.Fatal("expected credential modal to remain open")
	}
	if got := model.cloudCredentialPrompt.KeyInput.Value(); got != "ollama-secret" {
		t.Fatalf("expected pasted key in modal input, got %q", got)
	}
	if got := model.cloudCredentialPrompt.FocusIndex; got != 0 {
		t.Fatalf("expected paste to focus API key field, got focus index %d", got)
	}
	if got := model.input.Value(); got != "main prompt" {
		t.Fatalf("expected main prompt input unchanged, got %q", got)
	}
}

func TestFirstPromptCloudDefaultPromptsBeforePackingAndCancelRestoresInput(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	model := newTestModel(t)
	installTestModelRuntime(model, []string{"qwen3.6:35b"}, []string{"gpt-oss:120b"})
	model.store.Set(func(app state.App) state.App {
		app.ActiveModel = "gpt-oss:120b"
		return app
	})
	model.input.SetValue("hello cloud")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected preflight cmd")
	}
	model.Update(cmd())
	if model.cloudCredentialPrompt == nil {
		t.Fatal("expected cloud credential modal before context packing")
	}
	_, escCmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if escCmd != nil {
		model.Update(escCmd())
	}
	if model.input.Value() != "hello cloud" {
		t.Fatalf("expected input restored after cancel, got %q", model.input.Value())
	}
	if len(model.store.Get().Messages) != 0 {
		t.Fatalf("expected no run messages after cancel, got %d", len(model.store.Get().Messages))
	}
	for _, item := range model.transcript {
		if item.Kind == TranscriptUser && strings.Contains(item.Content, "hello cloud") {
			t.Fatalf("did not expect user prompt transcript entry on cancel, got %+v", item)
		}
	}
}

func TestRunStatePriorityPermissionOverTool(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		app.ActiveTools["t1"] = state.ToolUse{ID: "t1", Name: "Bash", Done: false}
		app.PermissionPrompt = &state.PermissionPrompt{ID: "p1", ToolName: "Bash", Target: "ls"}
		return app
	})
	view := model.View()
	if !strings.Contains(view, "[Permission required]") {
		t.Fatalf("expected permission-required status, got:\n%s", view)
	}
}

func TestRunStateWaitingThenStreamingPhase(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	if out := model.renderStatusBar(model.store.Get()); !strings.Contains(out, "[Waiting for model...]") {
		t.Fatalf("expected waiting state, got %q", out)
	}
	model.handleAgentEvent(agent.AssistantTextDelta{Content: "hi"})
	if out := model.renderStatusBar(model.store.Get()); !strings.Contains(out, "[Streaming...]") {
		t.Fatalf("expected streaming state, got %q", out)
	}
}

func TestRunStateRetryAndCompactingPriority(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	model.handleAgentEvent(agent.RetryNotice{Attempt: 1, Cause: "retry"})
	if out := model.renderStatusBar(model.store.Get()); !strings.Contains(out, "[Retrying...]") {
		t.Fatalf("expected retry state, got %q", out)
	}
	model.handleAgentEvent(agent.CompactionStarted{})
	if out := model.renderStatusBar(model.store.Get()); !strings.Contains(out, "[Compacting...]") {
		t.Fatalf("expected compacting state to override retry, got %q", out)
	}
	model.handleAgentEvent(agent.CompactionCompleted{})
	if out := model.renderStatusBar(model.store.Get()); !strings.Contains(out, "[Retrying...]") {
		t.Fatalf("expected retry state after compaction, got %q", out)
	}
}

func TestStatusBarShowsRunningToolElapsed(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		app.ActiveTools["t1"] = state.ToolUse{
			ID:        "t1",
			Name:      "Bash",
			Done:      false,
			StartedAt: time.Now().Add(-3 * time.Second),
		}
		return app
	})
	out := model.renderStatusBar(model.store.Get())
	if !strings.Contains(out, "[Running Bash") {
		t.Fatalf("expected running tool status, got %q", out)
	}
	if !strings.Contains(out, "3s]") && !strings.Contains(out, "4s]") {
		t.Fatalf("expected elapsed seconds in running tool status, got %q", out)
	}
}

func TestFormatElapsedCompact(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{in: 200 * time.Millisecond, want: "<1s"},
		{in: 3 * time.Second, want: "3s"},
		{in: 65 * time.Second, want: "1m05s"},
	}
	for _, tc := range tests {
		if got := formatElapsedCompact(tc.in); got != tc.want {
			t.Fatalf("formatElapsedCompact(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRenderPermissionModalShowsModeAndEscHint(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.PermissionMode = "plan"
		return app
	})
	out := model.renderPermissionModal(&state.PermissionPrompt{
		ID:       "p1",
		ToolName: "Bash",
		Target:   "ls",
		Reason:   "test",
	})
	if !strings.Contains(out, "Mode: plan") {
		t.Fatalf("expected mode in permission modal, got:\n%s", out)
	}
	if !strings.Contains(out, "[Esc] Deny") {
		t.Fatalf("expected Esc hint in permission modal, got:\n%s", out)
	}
}

func TestTranscriptWindowVirtualizesWhenAtBottom(t *testing.T) {
	model := newTestModel(t)
	for i := 0; i < 500; i++ {
		model.transcript = append(model.transcript, CreateSystemItem("item"))
	}
	model.viewport.SetContent("a\nb\nc\n")
	model.viewport.Height = 5
	model.viewport.GotoBottom()

	got := model.transcriptWindow()
	if len(got) >= len(model.transcript) {
		t.Fatalf("expected virtualized subset, got %d of %d", len(got), len(model.transcript))
	}
	lines := 0
	for _, item := range got {
		lines += estimateTranscriptItemLines(item)
	}
	if lines > model.transcriptVirtualLineBudget() {
		t.Fatalf("expected window lines <= budget (%d), got %d", model.transcriptVirtualLineBudget(), lines)
	}
}

func TestTranscriptWindowKeepsFullHistoryWhenScrolledUp(t *testing.T) {
	model := newTestModel(t)
	for i := 0; i < 500; i++ {
		model.transcript = append(model.transcript, CreateSystemItem("item"))
	}
	model.viewport.SetContent(strings.Repeat("row\n", 300))
	model.viewport.Height = 10
	model.viewport.GotoBottom()
	model.viewport.LineUp(20)
	if model.viewport.AtBottom() {
		t.Fatal("test setup failed: viewport should be scrolled up")
	}
	got := model.transcriptWindow()
	if len(got) != len(model.transcript) {
		t.Fatalf("expected full transcript while scrolled up, got %d want %d", len(got), len(model.transcript))
	}
}

func TestRefreshViewportContentStickyScrollGuard(t *testing.T) {
	model := newTestModel(t)
	for i := 0; i < 400; i++ {
		model.transcript = append(model.transcript, CreateSystemItem("row"))
	}
	model.viewport.SetContent(model.renderTranscript())
	model.viewport.Height = 10
	model.viewport.GotoBottom()
	model.viewport.LineUp(15)
	before := model.viewport.YOffset
	if model.viewport.AtBottom() {
		t.Fatal("test setup failed: viewport should be scrolled up")
	}

	model.transcript = append(model.transcript, CreateSystemItem("new"))
	model.refreshViewportContent(true)
	after := model.viewport.YOffset
	if after <= 0 {
		t.Fatalf("expected sticky guard to keep viewport scrolled up, before=%d after=%d", before, after)
	}
}

func TestRenderTranscriptCachesAssistantMarkdown(t *testing.T) {
	model := newTestModel(t)
	model.transcript = append(model.transcript, TranscriptItem{
		Kind:    TranscriptAssistant,
		Content: "hello **world**",
	})
	_ = model.renderTranscript()
	if model.transcript[0].Rendered == "" {
		t.Fatal("expected assistant rendered markdown cache to be populated")
	}
	cached := model.transcript[0].Rendered
	_ = model.renderTranscript()
	if model.transcript[0].Rendered != cached {
		t.Fatal("expected assistant rendered markdown cache to be reused")
	}
}

func TestRenderTranscriptStreamingTailPreservesHistoricalMarkdownCache(t *testing.T) {
	model := newTestModel(t)
	appendMarkdownHistory(model, 24)
	_ = model.renderTranscript()
	history := snapshotAssistantRendered(model)
	if len(history) == 0 {
		t.Fatal("expected historical assistant markdown cache to be populated")
	}

	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	model.transcript = append(model.transcript, CreateUserItem("continue"))
	appendAssistantTail(model, "tail **chunk**")
	_ = model.renderTranscript()

	last := len(model.transcript) - 1
	if got := model.transcript[last].Rendered; got != "" {
		t.Fatalf("expected streaming tail to stay uncached, got %q", got)
	}
	for idx, cached := range history {
		if got := model.transcript[idx].Rendered; got != cached {
			t.Fatalf("historical markdown cache changed at %d", idx)
		}
	}

	appendAssistantTail(model, " more tail")
	_ = model.renderTranscript()
	if got := model.transcript[last].Rendered; got != "" {
		t.Fatalf("expected streaming tail to remain uncached after further deltas, got %q", got)
	}
	for idx, cached := range history {
		if got := model.transcript[idx].Rendered; got != cached {
			t.Fatalf("historical markdown cache changed after tail delta at %d", idx)
		}
	}
}

func TestRenderTranscriptCachesCompletedTailWithoutClearingHistory(t *testing.T) {
	model := newTestModel(t)
	appendMarkdownHistory(model, 24)
	_ = model.renderTranscript()
	history := snapshotAssistantRendered(model)
	if len(history) == 0 {
		t.Fatal("expected historical assistant markdown cache to be populated")
	}

	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	model.transcript = append(model.transcript, CreateUserItem("continue"))
	appendAssistantTail(model, "tail **chunk**")
	_ = model.renderTranscript()

	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = false
		return app
	})
	_ = model.renderTranscript()

	last := len(model.transcript) - 1
	if got := model.transcript[last].Rendered; got == "" {
		t.Fatal("expected completed tail markdown to be cached")
	}
	for idx, cached := range history {
		if got := model.transcript[idx].Rendered; got != cached {
			t.Fatalf("historical markdown cache changed after completion at %d", idx)
		}
	}
}

func TestEnsureViewportContentSkipsInputOnlyUpdates(t *testing.T) {
	model := newTestModel(t)
	model.transcript = append(model.transcript, CreateSystemItem("hello"))
	if updated := model.ensureViewportContent(); !updated {
		t.Fatal("expected initial viewport content update")
	}
	keyBefore := model.viewportRenderKey

	model.input.SetValue("typing without submit")
	if updated := model.ensureViewportContent(); updated {
		t.Fatal("expected no viewport content update for input-only change")
	}
	if model.viewportRenderKey != keyBefore {
		t.Fatal("expected viewport content key to remain unchanged")
	}
}

func TestEnsureViewportContentSkipsSteadyStateActiveTail(t *testing.T) {
	model := newTestModel(t)
	appendMarkdownHistory(model, 24)
	_ = model.renderTranscript()
	model.viewport.Height = 10
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	model.transcript = append(model.transcript, CreateUserItem("continue"))
	appendAssistantTail(model, "tail **chunk**")

	if updated := model.ensureViewportContent(); !updated {
		t.Fatal("expected initial viewport content update for active tail")
	}
	model.viewport.GotoBottom()
	_ = model.ensureViewportContent()
	keyBefore := model.viewportRenderKey
	if updated := model.ensureViewportContent(); updated {
		t.Fatal("expected steady-state active tail to reuse viewport content")
	}
	if model.viewportRenderKey != keyBefore {
		t.Fatal("expected viewport render key to remain stable for steady-state active tail")
	}
}

func TestEnsureViewportContentInvalidatesOnTranscriptMutation(t *testing.T) {
	model := newTestModel(t)
	model.transcript = append(model.transcript, CreateSystemItem("hello"))
	if updated := model.ensureViewportContent(); !updated {
		t.Fatal("expected initial viewport content update")
	}

	model.transcript = append(model.transcript, CreateSystemItem("new line"))
	if updated := model.ensureViewportContent(); !updated {
		t.Fatal("expected viewport content update after transcript mutation")
	}
}

func TestHandleWindowSizeInvalidatesMarkdownAndTranscriptCache(t *testing.T) {
	model := newTestModel(t)
	model.transcript = append(model.transcript, TranscriptItem{
		Kind:    TranscriptAssistant,
		Content: "hello **world**",
	})
	_ = model.renderTranscript()
	if model.transcript[0].Rendered == "" {
		t.Fatal("expected assistant markdown cache to be populated")
	}
	if updated := model.ensureViewportContent(); !updated {
		t.Fatal("expected initial viewport content update")
	}

	_ = model.handleWindowSize(tea.WindowSizeMsg{Width: 120, Height: 40})
	if model.transcript[0].Rendered != "" {
		t.Fatal("expected assistant markdown cache cleared on width change")
	}
	if updated := model.ensureViewportContent(); !updated {
		t.Fatal("expected viewport content update after width change")
	}
}

func TestShouldRefreshStreamingEventThrottlesInInteractiveMode(t *testing.T) {
	model := newTestModel(t)
	model.program = noopProgramSender{}
	model.lastStreamRenderAt = time.Now()
	if model.shouldRefreshStreamingEvent() {
		t.Fatal("expected throttled streaming refresh immediately after last render")
	}
	model.lastStreamRenderAt = time.Now().Add(-60 * time.Millisecond)
	if !model.shouldRefreshStreamingEvent() {
		t.Fatal("expected refresh allowed after throttle interval")
	}
}

func TestUpdateToolItemRefreshesHeightCache(t *testing.T) {
	model := newTestModel(t)
	model.transcript = append(model.transcript, CreateToolItem("t1", "Bash"))
	model.toolIndex["t1"] = 0
	model.heightCache[0] = 1
	ok := model.updateToolItem("t1", func(item *TranscriptItem) {
		item.Content = strings.Repeat("line\n", 20)
	})
	if !ok {
		t.Fatal("expected tool item update to succeed")
	}
	if got := model.heightCache[0]; got <= 1 {
		t.Fatalf("expected refreshed height cache, got %d", got)
	}
}

func TestRenderTranscriptUsesPlainTailAssistantDuringActiveRun(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	model.transcript = append(model.transcript, TranscriptItem{
		Kind:    TranscriptAssistant,
		Content: "tail **markdown**",
	})
	_ = model.renderTranscript()
	if got := model.transcript[0].Rendered; got != "" {
		t.Fatalf("expected no markdown cache during active-run tail render, got %q", got)
	}
}

func TestRenderTranscriptCachesAssistantAfterRunCompletes(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = false
		return app
	})
	model.transcript = append(model.transcript, TranscriptItem{
		Kind:    TranscriptAssistant,
		Content: "final **markdown**",
	})
	_ = model.renderTranscript()
	if got := model.transcript[0].Rendered; got == "" {
		t.Fatal("expected markdown cache once run is not active")
	}
}

func TestTickLifecycleStartsAndStopsWithRunState(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	if !model.shouldRunTick(time.Now()) {
		t.Fatal("expected ticks active while run is active")
	}
	model.handleAgentEvent(agent.Terminal{Reason: agent.TerminalCompleted})
	if model.shouldRunTick(time.Now()) {
		t.Fatal("expected ticks stopped after terminal")
	}
}

func TestRunUIStatePhasePriorityTable(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		prepare func(*Model)
		want    RunPhase
	}{
		{
			name: "idle",
			prepare: func(m *Model) {
				m.store.Set(func(app state.App) state.App { return app })
			},
			want: RunPhaseIdle,
		},
		{
			name: "queued",
			prepare: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.QueuedPrompts = []string{"next"}
					return app
				})
			},
			want: RunPhaseQueued,
		},
		{
			name: "waiting",
			prepare: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveRun = true
					return app
				})
			},
			want: RunPhaseWaitingModel,
		},
		{
			name: "streaming",
			prepare: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveRun = true
					return app
				})
				m.firstStreamAt = now
			},
			want: RunPhaseStreaming,
		},
		{
			name: "thinking",
			prepare: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveRun = true
					return app
				})
				m.thinkingActive = true
			},
			want: RunPhaseThinking,
		},
		{
			name: "retry overrides thinking",
			prepare: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveRun = true
					app.LastRetryNotice = "Retry 1: test"
					return app
				})
				m.thinkingActive = true
				m.retryActiveUntil = now.Add(2 * time.Second)
			},
			want: RunPhaseRetrying,
		},
		{
			name: "compacting overrides retry",
			prepare: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveRun = true
					app.LastRetryNotice = "Retry 1: test"
					return app
				})
				m.retryActiveUntil = now.Add(2 * time.Second)
				m.compactingActive = true
			},
			want: RunPhaseCompacting,
		},
		{
			name: "tool overrides compacting",
			prepare: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveRun = true
					app.ActiveTools["t1"] = state.ToolUse{ID: "t1", Name: "Bash", Done: false}
					return app
				})
				m.compactingActive = true
			},
			want: RunPhaseRunningTool,
		},
		{
			name: "permission overrides tool",
			prepare: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveRun = true
					app.ActiveTools["t1"] = state.ToolUse{ID: "t1", Name: "Bash", Done: false}
					app.PermissionPrompt = &state.PermissionPrompt{ID: "p1", ToolName: "Bash", Target: "ls"}
					return app
				})
			},
			want: RunPhasePermissionRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := newTestModel(t)
			tc.prepare(model)
			got := model.snapshotRunUIState(model.store.Get(), now).Phase
			if got != tc.want {
				t.Fatalf("phase = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTickLifecycleEdgesAbortQueueRetryExpiry(t *testing.T) {
	model := newTestModel(t)

	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	if !model.shouldRunTick(time.Now()) {
		t.Fatal("expected ticks while active run is true")
	}

	model.handleAgentEvent(agent.Terminal{Reason: agent.TerminalAborted})
	if model.shouldRunTick(time.Now()) {
		t.Fatal("expected ticks to stop after aborted terminal")
	}

	model.store.Set(func(app state.App) state.App {
		app.QueuedPrompts = []string{"queued-only"}
		return app
	})
	if model.shouldRunTick(time.Now()) {
		t.Fatal("queued-only state should not schedule run ticks")
	}

	model.retryActiveUntil = time.Now().Add(500 * time.Millisecond)
	if !model.shouldRunTick(time.Now()) {
		t.Fatal("expected ticks while retry notice is active")
	}
	model.retryActiveUntil = time.Now().Add(-500 * time.Millisecond)
	if model.shouldRunTick(time.Now()) {
		t.Fatal("expected ticks to stop after retry window expiry")
	}
}

func TestIncompleteResponseRetryTranscriptIncludesFinalReport(t *testing.T) {
	model := newTestModel(t)

	model.handleAgentEvent(agent.AssistantTextDelta{Content: "Now I have the full picture. Let me write the missing-implementation summary:"})
	model.handleAgentEvent(agent.AssistantThinkingDelta{Thinking: strings.Repeat("evidence collected\n", 20)})
	model.handleAgentEvent(agent.RetryNotice{
		Attempt:        1,
		Cause:          "assistant response looked incomplete, requesting an anchored continuation",
		Kind:           "incomplete_assistant_response",
		DoneReason:     "stop",
		AssistantChars: 76,
		ThinkingChars:  360,
	})
	model.handleAgentEvent(agent.AssistantTextDelta{Content: "Missing implementation summary:\n- Add evidence ledger.\n- Add checkpoint resume."})
	model.handleAgentEvent(agent.Terminal{Reason: agent.TerminalCompleted})

	view := model.View()
	if !strings.Contains(view, "assistant response looked incomplete") {
		t.Fatalf("view does not contain retry notice:\n%s", view)
	}
	if !strings.Contains(view, "Missing implementation summary") {
		t.Fatalf("view does not contain final report:\n%s", view)
	}
	if !strings.Contains(view, "Thinking (") {
		t.Fatalf("view does not contain finalized thinking block:\n%s", view)
	}
}

func TestToggleThinkingEmptyTranscriptNoop(t *testing.T) {
	model := newTestModel(t)
	model.toggleLastThinkingItem()
	if len(model.transcript) != 0 {
		t.Fatalf("expected empty transcript after toggle, got %#v", model.transcript)
	}
}

func TestShortID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		n    int
		want string
	}{
		{name: "short", id: "tool-0", n: 8, want: "tool-0"},
		{name: "exact", id: "tool-123", n: 8, want: "tool-123"},
		{name: "long", id: "tool-123456", n: 8, want: "tool-123"},
		{name: "empty", id: "", n: 8, want: ""},
		{name: "zero-width", id: "tool-0", n: 0, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortID(tc.id, tc.n); got != tc.want {
				t.Fatalf("shortID(%q, %d) = %q, want %q", tc.id, tc.n, got, tc.want)
			}
		})
	}
}

func TestRenderToolPanel_SafeToolIDTruncation(t *testing.T) {
	model := newTestModel(t)

	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "short", id: "tool-0", want: "tool-0"},
		{name: "exact", id: "tool-123", want: "tool-123"},
		{name: "long", id: "tool-123456", want: "tool-123"},
		{name: "empty", id: "", want: "()"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("renderToolPanel panicked with id %q: %v", tc.id, r)
				}
			}()

			out := model.renderToolPanel(TranscriptItem{
				Kind:     TranscriptTool,
				ToolID:   tc.id,
				ToolName: "read_file",
				Content:  "ok",
			})

			if !strings.Contains(out, tc.want) {
				t.Fatalf("render output %q does not contain %q", out, tc.want)
			}
		})
	}
}

func TestHandleAgentEvent_ToolUseProgressWithShortID(t *testing.T) {
	model := newTestModel(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleAgentEvent panicked: %v", r)
		}
	}()

	model.handleAgentEvent(agent.ToolUseStart{
		ID:   "tool-1",
		Name: "read_file",
	})
	model.handleAgentEvent(agent.ToolUseProgress{
		ID:   "tool-1",
		Data: "streaming chunk",
	})

	if len(model.transcript) == 0 {
		t.Fatal("expected transcript entries")
	}

	last := model.transcript[len(model.transcript)-1]
	if last.Kind != TranscriptTool {
		t.Fatalf("expected last transcript item to be tool, got %q", last.Kind)
	}
	if !strings.Contains(last.Content, "[tool-1] streaming chunk") {
		t.Fatalf("unexpected tool progress content: %q", last.Content)
	}
}

func TestHandleAgentEvent_ToolUseInterleaving(t *testing.T) {
	model := newTestModel(t)

	model.handleAgentEvent(agent.ToolUseStart{ID: "tool-a", Name: "read_file"})
	model.handleAgentEvent(agent.ToolUseStart{ID: "tool-b", Name: "list_dir"})
	model.handleAgentEvent(agent.ToolUseProgress{ID: "tool-a", Data: "chunk-a"})
	model.handleAgentEvent(agent.ToolUseProgress{ID: "tool-b", Data: "chunk-b"})
	model.handleAgentEvent(agent.ToolUseResult{ID: "tool-a", Result: tools.Result{Display: "done-a"}})
	model.handleAgentEvent(agent.ToolUseResult{ID: "tool-b", Result: tools.Result{Display: "done-b"}})

	if len(model.transcript) < 2 {
		t.Fatalf("expected at least 2 transcript items, got %d", len(model.transcript))
	}

	var itemA, itemB *TranscriptItem
	for i := range model.transcript {
		if model.transcript[i].Kind != TranscriptTool {
			continue
		}
		switch model.transcript[i].ToolID {
		case "tool-a":
			itemA = &model.transcript[i]
		case "tool-b":
			itemB = &model.transcript[i]
		}
	}

	if itemA == nil || itemB == nil {
		t.Fatalf("missing tool transcript items: a=%v b=%v", itemA != nil, itemB != nil)
	}
	if !strings.Contains(itemA.Content, "[OK] done-a") {
		t.Fatalf("unexpected tool-a content: %q", itemA.Content)
	}
	if !strings.Contains(itemB.Content, "[OK] done-b") {
		t.Fatalf("unexpected tool-b content: %q", itemB.Content)
	}
}

func TestPermissionModalEscDeniesPrompt(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.PermissionPrompt = &state.PermissionPrompt{
			ID:       "perm-1",
			ToolName: "Bash",
			Target:   "ls",
			Reason:   "test",
		}
		return app
	})
	cmd := model.handlePermissionKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command for esc on permission modal")
	}
	msg := cmd()
	if _, ok := msg.(permissionResolvedMsg); !ok {
		t.Fatalf("expected permissionResolvedMsg, got %T", msg)
	}
	model.Update(msg)
	if model.store.Get().PermissionPrompt != nil {
		t.Fatal("expected permission prompt to be cleared after esc deny")
	}
}

func TestPermissionAlwaysAllowUsesLiteralTargetPattern(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.PermissionPrompt = &state.PermissionPrompt{
			ID:       "perm-2",
			ToolName: "Bash",
			Target:   "ls -la docs",
			Reason:   "test",
		}
		return app
	})
	cmd := model.handlePermissionKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if cmd == nil {
		t.Fatal("expected command for always-allow")
	}
	msg := cmd()
	model.Update(msg)

	rules := model.store.Get().PermissionRules
	if len(rules.AlwaysAllow) != 1 {
		t.Fatalf("expected one allow rule, got %d", len(rules.AlwaysAllow))
	}
	if got, want := rules.AlwaysAllow[0].Pattern, "Bash(ls -la docs)"; got != want {
		t.Fatalf("allow pattern = %q, want %q", got, want)
	}
	if rules.AlwaysAllow[0].Source != permissions.SourceSession {
		t.Fatalf("allow rule source = %q, want %q", rules.AlwaysAllow[0].Source, permissions.SourceSession)
	}
}

func TestClearTranscriptResetsTransientState(t *testing.T) {
	model := newTestModel(t)
	model.transcript = []TranscriptItem{{Kind: TranscriptAssistant, Content: "hello"}}
	model.thinkingActive = true
	model.toolIndex["x"] = 1
	model.err = context.DeadlineExceeded
	model.firstStreamAt = time.Now()
	model.firstRenderSet = true
	model.slowStageNotified = map[string]bool{"m": true}
	model.picker = picker.State{Visible: true}

	model.clearTranscript()

	if len(model.transcript) != 0 || model.thinkingActive {
		t.Fatal("clearTranscript did not clear transcript/thinking state")
	}
	if len(model.toolIndex) != 0 {
		t.Fatal("clearTranscript did not clear tool index")
	}
	if model.err != nil {
		t.Fatal("clearTranscript did not clear model error")
	}
	if !model.firstStreamAt.IsZero() || model.firstRenderSet {
		t.Fatal("clearTranscript did not clear streaming/render transient state")
	}
	if len(model.slowStageNotified) != 0 {
		t.Fatal("clearTranscript did not clear slow stage notices")
	}
	if model.picker.Visible {
		t.Fatal("clearTranscript did not close picker")
	}
}

func TestHandleAgentEvent_SlowStageTimingNotice(t *testing.T) {
	model := newTestModel(t)
	model.handleAgentEvent(agent.StageTiming{
		Stage:    "memory_recall",
		Duration: 1200 * time.Millisecond,
	})
	view := model.View()
	if !strings.Contains(view, "[slow stage] memory_recall took 1.2s") {
		t.Fatalf("expected slow stage notice, got:\n%s", view)
	}
}

func TestRecordFirstVisibleRenderAddsSlowStageNotice(t *testing.T) {
	model := newTestModel(t)
	meter := observability.NewMeter()
	model.SetMeter(meter)
	model.runStartedAt = time.Now().Add(-2 * time.Second)
	model.firstStreamAt = time.Now().Add(-1300 * time.Millisecond)

	model.recordFirstVisibleRenderIfNeeded()

	view := model.View()
	if !strings.Contains(view, "[slow stage] first_stream_to_visible_render took") {
		t.Fatalf("expected first stream slow notice, got:\n%s", view)
	}
	stages := meter.ConsumePendingRunStages()
	if _, ok := stages["first_stream_to_visible_render"]; !ok {
		t.Fatalf("expected first_stream_to_visible_render pending stage, got: %v", stages)
	}
	if _, ok := stages["first_visible_render"]; !ok {
		t.Fatalf("expected first_visible_render pending stage, got: %v", stages)
	}
}

func TestSlowStageNoticeUsesConfiguredThreshold(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.SlowStageNoticeThreshold = 2 * time.Second
		return app
	})
	model.handleAgentEvent(agent.StageTiming{
		Stage:    "memory_recall",
		Duration: 1200 * time.Millisecond,
	})
	view := model.View()
	if strings.Contains(view, "[slow stage] memory_recall took") {
		t.Fatalf("slow-stage notice should not appear when below configured threshold:\n%s", view)
	}
}

func TestSlowStageNoticeDedupesSameStagePerRun(t *testing.T) {
	model := newTestModel(t)
	model.handleAgentEvent(agent.StageTiming{
		Stage:    "memory_recall",
		Duration: 1200 * time.Millisecond,
	})
	model.handleAgentEvent(agent.StageTiming{
		Stage:    "memory_recall",
		Duration: 1800 * time.Millisecond,
	})
	count := 0
	for _, item := range model.transcript {
		if item.Kind == TranscriptSystem && strings.Contains(item.Content, "[slow stage] memory_recall took") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one slow-stage notice for repeated stage, got %d", count)
	}
}

func TestTerminalAppendsStageSummaryFromTrace(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.SlowStageNoticeThreshold = 800 * time.Millisecond
		return app
	})
	meter := observability.NewMeter()
	meter.RecordRunTrace(observability.RunTrace{
		RunStartedAt: time.Now().UTC(),
		StageLatencies: map[string]time.Duration{
			"memory_recall":                  1200 * time.Millisecond,
			"hook_user_prompt_submit":        700 * time.Millisecond,
			"first_stream_to_visible_render": 900 * time.Millisecond,
			"mention_expand":                 300 * time.Millisecond,
		},
	})
	model.SetMeter(meter)
	model.handleAgentEvent(agent.Terminal{Reason: agent.TerminalCompleted})
	view := model.View()
	if !strings.Contains(view, "[stage summary] slowest:") {
		t.Fatalf("expected stage summary in view, got:\n%s", view)
	}
	if !strings.Contains(view, "memory_recall 1.2s") {
		t.Fatalf("expected top stage in summary, got:\n%s", view)
	}
	if strings.Contains(view, "hook_user_prompt_submit 700ms") || strings.Contains(view, "mention_expand 300ms") {
		t.Fatalf("expected summary to include only stages above threshold, got:\n%s", view)
	}
}

func TestTerminalSkipsStageSummaryWhenNoStageCrossesThreshold(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.SlowStageNoticeThreshold = 2 * time.Second
		return app
	})
	meter := observability.NewMeter()
	meter.RecordRunTrace(observability.RunTrace{
		RunStartedAt: time.Now().UTC(),
		StageLatencies: map[string]time.Duration{
			"memory_recall":  900 * time.Millisecond,
			"mention_expand": 300 * time.Millisecond,
		},
	})
	model.SetMeter(meter)
	model.handleAgentEvent(agent.Terminal{Reason: agent.TerminalCompleted})
	view := model.View()
	if strings.Contains(view, "[stage summary] slowest:") {
		t.Fatalf("expected no stage summary when no stage crosses threshold, got:\n%s", view)
	}
}

func TestTruncateForDisplay(t *testing.T) {
	short := "hello"
	if got := truncateForDisplay(short, 10); got != short {
		t.Fatalf("short truncation changed value: got %q", got)
	}

	long := strings.Repeat("a", 20)
	got := truncateForDisplay(long, 8)
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
	if len(got) <= 8 {
		t.Fatalf("expected suffix after truncation, got %q", got)
	}
}

func (r *recordingRunner) Run(ctx context.Context, input agent.Input) <-chan agent.Event {
	r.calls++
	r.input = input

	events := make(chan agent.Event, 1)
	events <- agent.Terminal{Reason: agent.TerminalCompleted}
	close(events)
	return events
}

func TestPromptSubmissionPassesCurrentUserMessageToAgent(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}

	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	model.input.SetValue("hello ollama")

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected prompt submission to start an agent command")
	}
	cmd()

	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	if runner.input.Model != "test-model" {
		t.Fatalf("agent model = %q, want %q", runner.input.Model, "test-model")
	}
	if len(runner.input.Messages) != 1 {
		t.Fatalf("agent message count = %d, want 1", len(runner.input.Messages))
	}
	msg := runner.input.Messages[0]
	if msg.Role != llm.RoleUser || msg.Content != "hello ollama" {
		t.Fatalf("agent first message = %#v, want user message with submitted content", msg)
	}

	stored := store.Get()
	if len(stored.Messages) != 1 {
		t.Fatalf("stored message count = %d, want 1", len(stored.Messages))
	}
	if stored.Messages[0].Content != "hello ollama" {
		t.Fatalf("stored first message content = %q, want %q", stored.Messages[0].Content, "hello ollama")
	}
}

func TestRenderTranscriptEmptyStateVariants(t *testing.T) {
	model := newTestModel(t)
	initial := model.renderTranscript()
	if !strings.Contains(initial, "Type a prompt or /help to begin") {
		t.Fatalf("expected initial empty-state hint, got %q", initial)
	}
	model.everHadInput = true
	afterClear := model.renderTranscript()
	if !strings.Contains(afterClear, "transcript cleared - type a new prompt") {
		t.Fatalf("expected post-clear hint, got %q", afterClear)
	}
}

func TestClearCommandResetsTransientAppState(t *testing.T) {
	model := newTestModel(t)
	model.everHadInput = true
	model.transcript = append(model.transcript, CreateSystemItem("hello"))
	model.store.Set(func(app state.App) state.App {
		app.Messages = []llm.Message{{Role: llm.RoleUser, Content: "x"}}
		app.LastRetryNotice = "retry"
		app.TerminalReason = agent.TerminalMaxTurns
		app.TerminalDetail = "detail"
		app.ActiveTools["t1"] = state.ToolUse{ID: "t1", Name: "Bash", Done: false}
		return app
	})
	model.input.SetValue("/clear")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		cmd()
	}
	app := model.store.Get()
	if len(app.Messages) != 0 {
		t.Fatalf("expected messages cleared, got %d", len(app.Messages))
	}
	if app.LastRetryNotice != "" || app.TerminalReason != "" || app.TerminalDetail != "" {
		t.Fatalf("expected terminal/retry state reset, got retry=%q reason=%q detail=%q", app.LastRetryNotice, app.TerminalReason, app.TerminalDetail)
	}
	if len(app.ActiveTools) != 0 {
		t.Fatalf("expected active tools cleared, got %#v", app.ActiveTools)
	}
	if len(model.transcript) != 0 {
		t.Fatalf("expected transcript cleared, got %d items", len(model.transcript))
	}
}

func TestBracketedPasteForcesInsertAndUpdatesInput(t *testing.T) {
	model := newTestModel(t)
	model.vim.EnterNormal()
	model.input.Blur()
	model.input.Reset()

	model.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("hello"),
		Paste: true,
	})

	if !model.vim.IsInsert() {
		t.Fatal("expected paste to force insert mode")
	}
	if got := model.input.Value(); got != "hello" {
		t.Fatalf("expected pasted text in input, got %q", got)
	}
}

func TestNormalModeGGAndGViewportNavigation(t *testing.T) {
	model := newTestModel(t)
	model.vim.EnterNormal()
	for i := 0; i < 300; i++ {
		model.transcript = append(model.transcript, CreateSystemItem("row"))
	}
	model.viewport.SetContent(model.renderTranscript())
	model.viewport.Height = 10
	model.viewport.GotoBottom()
	if model.viewport.YOffset == 0 {
		t.Fatal("expected non-zero offset at bottom with long content")
	}

	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if model.viewport.YOffset != 0 {
		t.Fatalf("expected gg to go to top, got offset %d", model.viewport.YOffset)
	}

	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if model.viewport.YOffset == 0 {
		t.Fatal("expected G to go to bottom with non-zero offset")
	}
}

func TestNormalModeGChordTimeout(t *testing.T) {
	model := newTestModel(t)
	model.vim.EnterNormal()
	model.lastGChordAt = time.Now().Add(-2 * time.Second)
	model.viewport.SetContent(strings.Repeat("row\n", 300))
	model.viewport.Height = 10
	model.viewport.GotoBottom()
	before := model.viewport.YOffset
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if model.viewport.YOffset != before {
		t.Fatalf("expected single stale g not to move viewport, before=%d after=%d", before, model.viewport.YOffset)
	}
}

func TestTranscriptSearchCommandAndNavigation(t *testing.T) {
	model := newTestModel(t)
	model.transcript = append(model.transcript,
		CreateSystemItem("alpha"),
		CreateSystemItem("beta needle one"),
		CreateSystemItem("gamma"),
		CreateSystemItem("delta needle two"),
	)
	model.input.SetValue("/search needle")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.searchQuery != "needle" {
		t.Fatalf("expected search query set, got %q", model.searchQuery)
	}
	if len(model.searchMatches) != 2 {
		t.Fatalf("expected two matches, got %d", len(model.searchMatches))
	}
	if model.searchPos != 0 {
		t.Fatalf("expected initial search pos 0, got %d", model.searchPos)
	}

	model.vim.EnterNormal()
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if model.searchPos != 1 {
		t.Fatalf("expected n to move to next match, got %d", model.searchPos)
	}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	if model.searchPos != 0 {
		t.Fatalf("expected N to move to prev match, got %d", model.searchPos)
	}
}

func TestTranscriptSearchClear(t *testing.T) {
	model := newTestModel(t)
	model.searchQuery = "x"
	model.searchMatches = []int{1, 2}
	model.searchPos = 1
	model.input.SetValue("/search clear")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.searchQuery != "" || len(model.searchMatches) != 0 || model.searchPos != 0 {
		t.Fatalf("expected cleared search state, got query=%q matches=%v pos=%d", model.searchQuery, model.searchMatches, model.searchPos)
	}
}

func TestBTWCommandValidationAndQueue(t *testing.T) {
	model := newTestModel(t)
	model.handleBTWCommand(nil)
	if len(model.transcript) == 0 || !strings.Contains(model.transcript[len(model.transcript)-1].Content, "Usage: /btw") {
		t.Fatalf("expected usage error in transcript, got %#v", model.transcript)
	}

	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	model.handleBTWCommand([]string{"quick", "question"})
	if got := strings.TrimSpace(model.pendingBTW); got != "quick question" {
		t.Fatalf("expected pending btw question, got %q", got)
	}
}

func TestBTWCommandUsesReadOnlyLatestOnlyPlanInput(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	model.program = noopProgramSender{}
	model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	cmd := model.handleBTWCommand([]string{"side", "question"})
	if cmd == nil {
		t.Fatal("expected btw command to start run")
	}
	cmd()
	if runner.calls != 1 {
		t.Fatalf("runner calls=%d want 1", runner.calls)
	}
	if runner.input.HistoryPolicy != agent.HistoryPolicyLatestOnly {
		t.Fatalf("history policy=%q want %q", runner.input.HistoryPolicy, agent.HistoryPolicyLatestOnly)
	}
	if runner.input.PermissionMode != permissions.ModePlan {
		t.Fatalf("permission mode=%q want %q", runner.input.PermissionMode, permissions.ModePlan)
	}
	if runner.input.ToolsetName != agent.ToolsetReadOnly {
		t.Fatalf("toolset=%q want %q", runner.input.ToolsetName, agent.ToolsetReadOnly)
	}
}

func TestTerminalDoesNotPersistConversationForBTW(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.Messages = []llm.Message{{Role: llm.RoleUser, Content: "main"}}
		return app
	})
	before := len(model.store.Get().Messages)
	model.activeRunKind = "btw"
	model.handleAgentEvent(agent.Terminal{
		Reason:       agent.TerminalCompleted,
		Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "side"}},
	})
	after := len(model.store.Get().Messages)
	if before != after {
		t.Fatalf("expected main messages unchanged for btw terminal, before=%d after=%d", before, after)
	}
}

func TestBGCommandMarksBackgroundAndReports(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	model.handleBackgroundCommand(nil)
	if !model.backgroundedRun {
		t.Fatal("expected backgrounded run flag true")
	}
	model.handleBackgroundCommand(nil)
	if len(model.transcript) == 0 || !strings.Contains(model.transcript[len(model.transcript)-1].Content, "[BG] phase=") {
		t.Fatalf("expected bg status line, got %#v", model.transcript)
	}
}

func TestStatusBarShowsBackgroundAndBTWQueued(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		app.ActiveModel = "test-model"
		return app
	})
	model.backgroundedRun = true
	model.pendingBTW = "question"
	out := model.renderStatusBar(model.store.Get())
	if !strings.Contains(out, "[Background]") {
		t.Fatalf("expected background marker, got %q", out)
	}
	if !strings.Contains(out, "[BTW queued]") {
		t.Fatalf("expected btw queued marker, got %q", out)
	}
}

func TestRenderActiveTaskAndTipLines(t *testing.T) {
	model := newTestModel(t)
	idleActivity := model.renderActiveTaskLine(model.store.Get())
	if !strings.Contains(idleActivity, "Activity: idle") {
		t.Fatalf("expected idle activity line, got %q", idleActivity)
	}
	idleTip := model.renderTipLine(model.store.Get())
	if !strings.Contains(idleTip, "Tip: use /help") {
		t.Fatalf("expected default tip line, got %q", idleTip)
	}

	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	activeTip := model.renderTipLine(model.store.Get())
	if !strings.Contains(activeTip, "/bg") || !strings.Contains(activeTip, "/btw") {
		t.Fatalf("expected run tip line, got %q", activeTip)
	}
}

func TestPromptSubmissionExpandsFileMentions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("from file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}

	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	model.input.SetValue("summarize @note.txt")

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected prompt submission to start an agent command")
	}
	cmd()

	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	msg := runner.input.Messages[0]
	if !strings.Contains(msg.Content, "summarize @note.txt") {
		t.Fatalf("expanded message missing original prompt:\n%s", msg.Content)
	}
	if !strings.Contains(msg.Content, "<file path=\"note.txt\">") || !strings.Contains(msg.Content, "from file\n") {
		t.Fatalf("expanded message missing file content:\n%s", msg.Content)
	}

	if len(model.transcript) == 0 || model.transcript[0].Content != "summarize @note.txt" {
		t.Fatalf("transcript should keep original user text, got %#v", model.transcript)
	}
}

func TestPromptSubmissionDirectoryMentionAddsExpansionSummary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("from file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}

	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	model.input.SetValue("summarize @.")

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected prompt submission to start an agent command")
	}
	cmd()

	if len(model.transcript) < 2 {
		t.Fatalf("expected user line plus summary, got %#v", model.transcript)
	}
	found := false
	for _, item := range model.transcript {
		if item.Kind == TranscriptSystem && strings.Contains(item.Content, "expanded 1 directories") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing directory expansion summary in transcript: %#v", model.transcript)
	}
}

func TestPromptSubmissionListingContentModeShowsWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("from file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}

	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	model.input.SetValue("list all the files in @.?content")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected prompt submission to start an agent command")
	}
	cmd()

	found := false
	for _, item := range model.transcript {
		if item.Kind == TranscriptSystem && strings.Contains(item.Content, "listing prompt expanded file bodies") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing listing/content warning in transcript: %#v", model.transcript)
	}
}

func TestPromptSubmissionListingTreeFinalPromptShape(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "a.txt"), []byte("from file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}

	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	model.input.SetValue("list all the files in @docs/")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected prompt submission to start an agent command")
	}
	cmd()

	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	msg := runner.input.Messages[0]
	if !strings.Contains(msg.Content, "User request:") || !strings.Contains(msg.Content, "Directory tree data:") {
		t.Fatalf("expected listing envelope in expanded prompt:\n%s", msg.Content)
	}
	if strings.Contains(msg.Content, "<file path=\"docs/a.txt\">") {
		t.Fatalf("did not expect file body blocks in listing/tree prompt:\n%s", msg.Content)
	}
	if strings.Contains(msg.Content, "<directory ") {
		t.Fatalf("did not expect xml directory block in listing envelope:\n%s", msg.Content)
	}
	if strings.Contains(msg.Content, "Listing response constraint:") {
		t.Fatalf("did not expect listing answer constraint in expanded prompt:\n%s", msg.Content)
	}
}

func TestContinueUsesCheckpointResumePrompt(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	if err := analysis.SaveCheckpoint(analysis.Checkpoint{
		Model:               "test-model",
		WorkingDir:          ".",
		LastUserPrompt:      "analyze project status",
		LastAssistantOutput: "Now I have the full picture. Let me write the summary:",
		PendingFinalAnswer:  true,
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	model.input.SetValue("continue")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected run command")
	}
	cmd()
	if runner.calls != 1 || len(runner.input.Messages) == 0 {
		t.Fatalf("runner not called as expected: calls=%d", runner.calls)
	}
	got := runner.input.Messages[0].Content
	if !strings.Contains(got, "Continue the previous task from checkpoint") {
		t.Fatalf("expected resume prompt wrapper, got:\n%s", got)
	}
}

func TestAnalyzeProjectAddsRetrievedMentions(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	t.Setenv("NANDOCODEGO_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "tui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "tui", "app.go"), []byte("package tui\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := model.fileIndex.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	model.input.SetValue("/analyze-project internal review tui rendering")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected run command")
	}
	cmd()
	if runner.calls != 1 || len(runner.input.Messages) == 0 {
		t.Fatalf("runner not called as expected: calls=%d", runner.calls)
	}
	got := runner.input.Messages[0].Content
	if !strings.Contains(got, "Evidence summaries") {
		t.Fatalf("expected workflow evidence prompt, got:\n%s", got)
	}
	if !strings.Contains(got, "internal/tui/app.go =>") {
		t.Fatalf("expected summary entry for retrieved file, got:\n%s", got)
	}
	if strings.Contains(got, "@internal/tui/app.go") {
		t.Fatalf("analyze-project prompt should use summaries, not raw mention expansion: %s", got)
	}
}

func TestAnalyzeProjectKeepsExplicitMentionPriority(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	t.Setenv("NANDOCODEGO_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "tui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "tui", "app.go"), []byte("package tui\nfunc Render(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "agent.go"), []byte("package internal\nfunc Run(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := model.fileIndex.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	model.input.SetValue("/analyze-project internal review @internal/agent.go and tui rendering")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected run command")
	}
	cmd()
	got := runner.input.Messages[0].Content
	idxExplicit := strings.Index(got, "internal/agent.go =>")
	idxRetrieved := strings.Index(got, "internal/tui/app.go =>")
	if idxExplicit < 0 || idxRetrieved < 0 {
		t.Fatalf("expected both summary lines, got:\n%s", got)
	}
	if idxExplicit > idxRetrieved {
		t.Fatalf("expected explicit mention summary first, got:\n%s", got)
	}
}

func TestLargeStatusPromptDoesNotUseWorkflowFallback(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	t.Setenv("NANDOCODEGO_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	body := strings.Repeat("phase-22-content\n", 5000)
	if err := os.WriteFile(filepath.Join(dir, "plan.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	prompt := "what is the current status of @plan.md in the codebase, don't implement anything just report"
	model.input.SetValue(prompt)
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected run command")
	}
	cmd()
	got := runner.input.Messages[0].Content
	if strings.Contains(got, "Evidence summaries") {
		t.Fatalf("did not expect project-analysis prompt rewrite, got:\n%s", got)
	}
	if runner.input.PromptIntent != string(mentions.IntentFileStatus) {
		t.Fatalf("prompt intent=%q want %q", runner.input.PromptIntent, mentions.IntentFileStatus)
	}
	if runner.input.HistoryPolicy != agent.HistoryPolicyLatestOnly {
		t.Fatalf("history policy=%q want %q", runner.input.HistoryPolicy, agent.HistoryPolicyLatestOnly)
	}
	for _, item := range model.transcript {
		if item.Kind == TranscriptSystem && strings.Contains(item.Content, "[Analysis fallback:") {
			t.Fatalf("unexpected fallback status in transcript: %#v", model.transcript)
		}
	}
}

func TestPromptSubmissionFileStatusMentionUsesLatestOnlyHistory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("review what was implemented in @note.txt and report status")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected run command")
	}
	cmd()
	if runner.input.HistoryPolicy != agent.HistoryPolicyLatestOnly {
		t.Fatalf("history policy=%q want %q", runner.input.HistoryPolicy, agent.HistoryPolicyLatestOnly)
	}
	if runner.input.PromptIntent != string(mentions.IntentFileStatus) {
		t.Fatalf("prompt intent=%q want %q", runner.input.PromptIntent, mentions.IntentFileStatus)
	}
	foundStatusNotice := false
	for _, item := range model.transcript {
		if item.Kind == TranscriptSystem && strings.Contains(item.Content, "[Status prompt: explicit references detected, history=latest_only]") {
			foundStatusNotice = true
			break
		}
	}
	if !foundStatusNotice {
		t.Fatalf("expected status/latest-only notice, got %#v", model.transcript)
	}
}

func TestPromptSubmissionGenericStatusWithoutMentionKeepsDefaultHistory(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("what is the current status?")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected run command")
	}
	cmd()
	if runner.input.HistoryPolicy != agent.HistoryPolicyDefault {
		t.Fatalf("history policy=%q want %q", runner.input.HistoryPolicy, agent.HistoryPolicyDefault)
	}
}

func TestPromptSubmissionTooLargeEvidenceStopsBeforeRunner(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("very large content"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	appState.ToolSettings.MaxPromptBytes = 1
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("report status for @note.txt")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		cmd()
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls=%d want 0", runner.calls)
	}
	found := false
	for _, item := range model.transcript {
		if item.Kind == TranscriptSystem && strings.Contains(item.Content, "[Context too large:") {
			found = true
		}
		if item.Kind == TranscriptSystem && strings.Contains(item.Content, "[Analysis fallback:") {
			t.Fatalf("unexpected analysis fallback in transcript: %#v", model.transcript)
		}
	}
	if !found {
		t.Fatalf("expected context-too-large notice, got %#v", model.transcript)
	}
}

func TestPromptSubmissionEvidenceBudgetUsesRuntimeNumCtx(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte(strings.Repeat("x", 20000)), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	appState.RuntimeNumCtx = 131072
	appState.MaxOutputTokens = 2048
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("report status for @note.txt")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected run command")
	}
	cmd()
	if runner.input.EvidencePack == nil {
		t.Fatal("expected evidence pack metadata")
	}
	cfg := agent.DefaultConfig()
	cfg.ContextMode = appState.ContextMode
	cfg.NumCtx = appState.RuntimeNumCtx
	cfg.MaxOutputTokens = appState.MaxOutputTokens
	want := agent.BuildAssemblyBudget(cfg, agent.Input{
		Model:           appState.ActiveModel,
		ContextMode:     appState.ContextMode,
		MaxOutputTokens: appState.MaxOutputTokens,
	}, appState.Messages, agent.AssemblyEstimate{})
	if runner.input.EvidencePack.BudgetTokens != want.AvailableEvidenceTokens {
		t.Fatalf("budget tokens=%d want %d", runner.input.EvidencePack.BudgetTokens, want.AvailableEvidenceTokens)
	}
}

func TestPromptAssemblyParityWithSharedContextPacker(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("a.txt", "short\n")
	mustWrite("b.txt", strings.Repeat("B", 20000))
	mustWrite("docs/guide.md", "guide body\n")
	mustWrite("docs/config.toml", "key = \"value\"\n")

	tests := []string{
		"summarize @a.txt",
		"report status for @b.txt",
		"compare @a.txt and @b.txt",
		"review @docs",
	}
	for _, prompt := range tests {
		t.Run(prompt, func(t *testing.T) {
			initial := bootstrap.DefaultInitial(dir)
			initial.DefaultModel = "test-model"
			appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
			appState.RuntimeNumCtx = 65536
			store := state.NewStore(appState, nil)
			runner := &recordingRunner{}
			model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			model.input.SetValue(prompt)
			_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("expected run command")
			}
			cmd()
			if runner.calls != 1 {
				t.Fatalf("runner calls=%d want 1", runner.calls)
			}
			tuiPrompt := runner.input.Messages[0].Content

			cfg := agent.DefaultConfig()
			cfg.ContextMode = appState.ContextMode
			cfg.NumCtx = appState.RuntimeNumCtx
			if appState.MaxOutputTokens > 0 {
				cfg.MaxOutputTokens = appState.MaxOutputTokens
			}
			packed, _, err := contextpack.BuildCurrentTurnPrompt(prompt, appState.ToolContext(context.Background()), cfg, agent.Input{
				Model:           appState.ActiveModel,
				ContextMode:     appState.ContextMode,
				MaxOutputTokens: appState.MaxOutputTokens,
			}, appState.Messages)
			if err != nil {
				t.Fatal(err)
			}
			if tuiPrompt != packed.Prompt {
				t.Fatalf("tui and shared packer prompts diverged\n---tui---\n%s\n---shared---\n%s", tuiPrompt, packed.Prompt)
			}
		})
	}
}

func TestPromptAssemblyTooLargeParity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("x", 80000)), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	appState.ToolSettings.MaxPromptBytes = 1
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	prompt := "report status for @big.txt"
	model.input.SetValue(prompt)
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		cmd()
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls=%d want 1", runner.calls)
	}
	if !strings.Contains(runner.input.Messages[0].Content, "<partial_file_notice path=\"big.txt\">") {
		t.Fatalf("expected partial file notice in packed prompt:\n%s", runner.input.Messages[0].Content)
	}

	cfg := agent.DefaultConfig()
	cfg.ContextMode = appState.ContextMode
	_, _, err = contextpack.BuildCurrentTurnPrompt(prompt, appState.ToolContext(context.Background()), cfg, agent.Input{
		Model:       appState.ActiveModel,
		ContextMode: appState.ContextMode,
	}, appState.Messages)
	if err != nil {
		t.Fatalf("expected tiny-partial packet instead of error, got %v", err)
	}
}

func TestPickerTabInsertsSelectedFileAndCloses(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.fileIndex.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("read @not")
	model.input.SetCursor(len("read @not"))
	model.refreshPicker()
	if !model.picker.Visible || len(model.picker.Items) == 0 {
		t.Fatalf("picker not visible: %#v", model.picker)
	}
	model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := model.input.Value(); !strings.Contains(got, "@note.txt ") {
		t.Fatalf("value=%q", got)
	}
	if model.picker.Visible {
		t.Fatal("picker should close after file insert")
	}
}

func TestPickerDirectorySelectionKeepsPickerOpen(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	model.input.SetValue("@in")
	model.input.SetCursor(len("@in"))
	model.picker = picker.State{
		Visible: true,
		Trigger: picker.TriggerFile,
		Token: picker.Context{
			Kind:   picker.TriggerFile,
			Start:  0,
			End:    3,
			Query:  "in",
			Active: true,
		},
		Items: []picker.Suggestion{
			{Display: "internal/", Insert: "internal", IsDir: true},
		},
		Index: 0,
	}

	model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := model.input.Value(); got != "@internal/" {
		t.Fatalf("value=%q", got)
	}
	if !model.picker.Visible {
		t.Fatal("picker should stay visible for directory drill-down")
	}
}

func TestPickerShiftTabAcceptsDirectoryMentionAndCloses(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	model.input.SetValue("@in")
	model.input.SetCursor(len("@in"))
	model.picker = picker.State{
		Visible: true,
		Trigger: picker.TriggerFile,
		Token: picker.Context{
			Kind:   picker.TriggerFile,
			Start:  0,
			End:    3,
			Query:  "in",
			Active: true,
		},
		Items: []picker.Suggestion{
			{Display: "internal/", Insert: "internal", IsDir: true},
		},
		Index: 0,
	}

	model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := model.input.Value(); got != "@internal/ " {
		t.Fatalf("value=%q", got)
	}
	if model.picker.Visible {
		t.Fatal("picker should close after shift-tab directory accept")
	}
}

func TestPickerEscClosesWithoutLeavingInsertMode(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.picker = picker.State{Visible: true}
	model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.vim.IsNormal() {
		t.Fatal("expected to remain in insert mode when closing picker")
	}
	if model.picker.Visible {
		t.Fatal("picker should be closed")
	}
}

func TestPickerEnterAcceptsInsteadOfSubmitting(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("@he")
	model.input.SetCursor(len("@he"))
	model.picker = picker.State{
		Visible: true,
		Trigger: picker.TriggerFile,
		Token: picker.Context{
			Kind:   picker.TriggerFile,
			Start:  0,
			End:    3,
			Query:  "he",
			Active: true,
		},
		Items: []picker.Suggestion{
			{Display: "hello.txt", Insert: "hello.txt"},
		},
		Index: 0,
	}
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if runner.calls != 0 {
		t.Fatalf("expected no submission, calls=%d", runner.calls)
	}
	if got := model.input.Value(); got != "@hello.txt " {
		t.Fatalf("value=%q", got)
	}
}

func TestPickerReplacesOnlyActiveMention(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("compare @first.txt and @se")
	model.input.SetCursor(len("compare @first.txt and @se"))
	model.picker = picker.State{
		Visible: true,
		Trigger: picker.TriggerFile,
		Token: picker.Context{
			Kind:   picker.TriggerFile,
			Start:  len("compare @first.txt and "),
			End:    len("compare @first.txt and @se"),
			Query:  "se",
			Active: true,
		},
		Items: []picker.Suggestion{
			{Display: "second.txt", Insert: "second.txt"},
		},
	}
	model.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := model.input.Value()
	if got != "compare @first.txt and @second.txt " {
		t.Fatalf("value=%q", got)
	}
}

func TestPickerAcceptTouchesFrecency(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("@no")
	model.input.SetCursor(len("@no"))
	model.picker = picker.State{
		Visible: true,
		Trigger: picker.TriggerFile,
		Token: picker.Context{
			Kind:   picker.TriggerFile,
			Start:  0,
			End:    3,
			Query:  "no",
			Active: true,
		},
		Items: []picker.Suggestion{
			{Display: "note.txt", Insert: "note.txt"},
		},
	}
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if score := model.frecency.Score("note.txt"); score <= 0 {
		t.Fatalf("expected frecency score >0, got %v", score)
	}
}

func TestPickerInsertionPreservesOtherLines(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("line1\ncheck @no\nline3")
	model.input.CursorUp()
	model.input.SetCursor(len("check @no"))
	model.picker = picker.State{
		Visible: true,
		Trigger: picker.TriggerFile,
		Token: picker.Context{
			Kind:   picker.TriggerFile,
			Start:  len("check "),
			End:    len("check @no"),
			Query:  "no",
			Active: true,
		},
		Items: []picker.Suggestion{
			{Display: "note.txt", Insert: "note.txt"},
		},
	}
	model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := model.input.Value(); got != "line1\ncheck @note.txt \nline3" {
		t.Fatalf("value=%q", got)
	}
}

func TestPickerAcceptedMentionResolvesOnSubmit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("summarize @no")
	model.input.SetCursor(len("summarize @no"))
	model.picker = picker.State{
		Visible: true,
		Trigger: picker.TriggerFile,
		Token: picker.Context{
			Kind:   picker.TriggerFile,
			Start:  len("summarize "),
			End:    len("summarize @no"),
			Query:  "no",
			Active: true,
		},
		Items: []picker.Suggestion{
			{Display: "note.txt", Insert: "note.txt"},
		},
	}
	model.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit cmd")
	}
	cmd()
	if runner.calls != 1 {
		t.Fatalf("runner calls=%d", runner.calls)
	}
	if !strings.Contains(runner.input.Messages[0].Content, "<file path=\"note.txt\">") {
		t.Fatalf("expanded prompt missing resolved mention:\n%s", runner.input.Messages[0].Content)
	}
}

func TestAnalyzeProjectCommandBuildsPromptAndRunsAgent(t *testing.T) {
	dir := t.TempDir()
	initial := bootstrap.DefaultInitial(dir)
	initial.DefaultModel = "test-model"
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.input.SetValue("/analyze-project . find architectural bottlenecks")
	model.input.SetCursor(len(model.input.Value()))
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit cmd")
	}
	cmd()
	if runner.calls != 1 {
		t.Fatalf("runner calls=%d", runner.calls)
	}
	if !strings.Contains(runner.input.Messages[0].Content, "find architectural bottlenecks") {
		t.Fatalf("unexpected prompt content: %q", runner.input.Messages[0].Content)
	}
}
