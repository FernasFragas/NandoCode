package hooks

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

func TestAggregatePrecedence(t *testing.T) {
	res := aggregate([]Result{
		{Decision: DecisionAllow, Reason: "allowed"},
		{Decision: DecisionAsk, Reason: "confirm"},
		{Decision: DecisionDeny, Reason: "blocked"},
	})
	if res.Decision != DecisionDeny {
		t.Fatalf("expected deny to win, got %q", res.Decision)
	}
	if res.Reason != "blocked" {
		t.Fatalf("expected deny reason, got %q", res.Reason)
	}
}

func TestLoadSnapshotEnablesHTTPAndAgentHooks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		hook    string
		enabled bool
	}{
		{
			name:    "http",
			hook:    `{"kind":"http","event":"PreToolUse","url":"http://localhost:11434/hook"}`,
			enabled: true,
		},
		{
			name:    "agent",
			hook:    `{"kind":"agent","event":"PreToolUse","prompt":"review the event"}`,
			enabled: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeHookConfig(t, `{"hooks":[`+tc.hook+`]}`)
			snap := LoadSnapshot(LoadOptions{UserPath: path})
			if tc.enabled {
				if len(snap.Hooks) != 1 {
					t.Fatalf("expected executable hook, got %d", len(snap.Hooks))
				}
				return
			}
			if len(snap.Hooks) != 0 {
				t.Fatalf("expected no executable hooks, got %d", len(snap.Hooks))
			}
		})
	}
}

func TestLoadSnapshotRejectsHooksMissingRequiredFields(t *testing.T) {
	t.Parallel()
	path := writeHookConfig(t, `{"hooks":[
		{"kind":"command","event":"PreToolUse"},
		{"kind":"prompt","event":"UserPromptSubmit"},
		{"kind":"http","event":"PreToolUse"},
		{"kind":"agent","event":"PreToolUse"}
	]}`)
	snap := LoadSnapshot(LoadOptions{UserPath: path})
	if len(snap.Hooks) != 0 {
		t.Fatalf("expected no executable hooks, got %d", len(snap.Hooks))
	}
	if len(snap.Disabled) != 4 {
		t.Fatalf("expected four disabled hooks, got %d", len(snap.Disabled))
	}
	reasons := []string{
		snap.Disabled[0].Reason,
		snap.Disabled[1].Reason,
		snap.Disabled[2].Reason,
		snap.Disabled[3].Reason,
	}
	want := []string{
		"command hook requires command",
		"prompt hook requires prompt",
		"http hook requires url",
		"agent hook requires prompt",
	}
	for i, reason := range reasons {
		if reason != want[i] {
			t.Fatalf("disabled[%d] reason=%q want %q", i, reason, want[i])
		}
	}
}

func TestDispatchDefaultHooksRemainSerial(t *testing.T) {
	t.Parallel()
	client := newBlockingHookClient()
	dispatcher := NewDispatcher(Snapshot{Hooks: []Hook{
		{Kind: KindPrompt, Event: EventUserPromptSubmit, Prompt: "first", Enabled: true},
		{Kind: KindPrompt, Event: EventUserPromptSubmit, Prompt: "second", Enabled: true},
	}}, client, Config{Model: "test-model", DefaultTimeout: time.Second})
	done := make(chan Result, 1)
	go func() {
		done <- dispatcher.Dispatch(context.Background(), Envelope{Event: EventUserPromptSubmit})
	}()
	waitUntil(t, time.Second, func() bool { return client.started() == 1 })
	client.releaseAll()
	<-done
	if got := client.maxActive(); got != 1 {
		t.Fatalf("expected serial dispatch max in-flight=1, got %d", got)
	}
}

func TestDispatchParallelSafeHooksRunConcurrently(t *testing.T) {
	t.Parallel()
	client := newBlockingHookClient()
	dispatcher := NewDispatcher(Snapshot{Hooks: []Hook{
		{Kind: KindPrompt, Event: EventUserPromptSubmit, Prompt: "first", ParallelSafe: true, Enabled: true},
		{Kind: KindPrompt, Event: EventUserPromptSubmit, Prompt: "second", ParallelSafe: true, Enabled: true},
	}}, client, Config{Model: "test-model", DefaultTimeout: time.Second})
	done := make(chan Result, 1)
	go func() {
		done <- dispatcher.Dispatch(context.Background(), Envelope{Event: EventUserPromptSubmit})
	}()
	waitUntil(t, time.Second, func() bool { return client.started() == 2 })
	client.releaseAll()
	<-done
	if got := client.maxActive(); got < 2 {
		t.Fatalf("expected concurrent dispatch max in-flight>=2, got %d", got)
	}
}

func TestDispatchParallelSafeHooksRespectConcurrencyLimit(t *testing.T) {
	t.Parallel()
	client := newBlockingHookClient()
	hooks := make([]Hook, 0, 6)
	for _, prompt := range []string{"one", "two", "three", "four", "five", "six"} {
		hooks = append(hooks, Hook{
			Kind:         KindPrompt,
			Event:        EventUserPromptSubmit,
			Prompt:       prompt,
			ParallelSafe: true,
			Enabled:      true,
		})
	}
	dispatcher := NewDispatcher(Snapshot{Hooks: hooks}, client, Config{Model: "test-model", DefaultTimeout: time.Second})
	done := make(chan Result, 1)
	go func() {
		done <- dispatcher.Dispatch(context.Background(), Envelope{Event: EventUserPromptSubmit})
	}()
	waitUntil(t, time.Second, func() bool { return client.started() == parallelHookLimit })
	if got := client.maxActive(); got != parallelHookLimit {
		t.Fatalf("expected max in-flight to reach %d before release, got %d", parallelHookLimit, got)
	}
	client.releaseAll()
	<-done
	if got := client.maxActive(); got != parallelHookLimit {
		t.Fatalf("expected max in-flight=%d, got %d", parallelHookLimit, got)
	}
	if got := client.started(); got != len(hooks) {
		t.Fatalf("expected all hooks to run, got %d", got)
	}
}

func TestDispatchParallelSafeAggregateOrderIsDeterministic(t *testing.T) {
	t.Parallel()
	client := &timedResponseHookClient{
		responses: map[string]timedResponse{
			"first":  {delay: 90 * time.Millisecond, body: `{"decision":"ask","reason":"ask first"}`},
			"second": {delay: 10 * time.Millisecond, body: `{"decision":"deny","reason":"deny second"}`},
			"third":  {delay: 20 * time.Millisecond, body: `{"decision":"deny","reason":"deny third"}`},
		},
	}
	dispatcher := NewDispatcher(Snapshot{Hooks: []Hook{
		{Kind: KindPrompt, Event: EventUserPromptSubmit, Prompt: "first", ParallelSafe: true, Enabled: true},
		{Kind: KindPrompt, Event: EventUserPromptSubmit, Prompt: "second", ParallelSafe: true, Enabled: true},
		{Kind: KindPrompt, Event: EventUserPromptSubmit, Prompt: "third", ParallelSafe: true, Enabled: true},
	}}, client, Config{Model: "test-model", DefaultTimeout: time.Second})

	res := dispatcher.Dispatch(context.Background(), Envelope{Event: EventUserPromptSubmit})
	if res.Decision != DecisionDeny {
		t.Fatalf("expected deny decision, got %q", res.Decision)
	}
	if res.Reason != "deny second" {
		t.Fatalf("expected deterministic deny reason from second hook, got %q", res.Reason)
	}
}

func TestDispatchParallelSafeRespectsCanceledContext(t *testing.T) {
	t.Parallel()
	client := &timedResponseHookClient{
		responses: map[string]timedResponse{
			"first": {delay: time.Second, body: `{"decision":"allow"}`},
		},
	}
	dispatcher := NewDispatcher(Snapshot{Hooks: []Hook{
		{Kind: KindPrompt, Event: EventUserPromptSubmit, Prompt: "first", ParallelSafe: true, Enabled: true},
	}}, client, Config{Model: "test-model", DefaultTimeout: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	res := dispatcher.Dispatch(ctx, Envelope{Event: EventUserPromptSubmit})
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("expected quick return on canceled context")
	}
	if res.Decision != DecisionNone {
		t.Fatalf("expected no decision on canceled context, got %q", res.Decision)
	}
}

type blockingHookClient struct {
	mu          sync.Mutex
	startedN    int
	inFlight    int
	maxInFlight int
	releaseCh   chan struct{}
}

func newBlockingHookClient() *blockingHookClient {
	return &blockingHookClient{releaseCh: make(chan struct{})}
}

func (f *blockingHookClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.mu.Lock()
	f.startedN++
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	f.mu.Unlock()
	select {
	case <-ctx.Done():
	case <-f.releaseCh:
	}
	f.mu.Lock()
	f.inFlight--
	f.mu.Unlock()
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: `{"decision":"allow","reason":"ok"}`}, Done: true}
	close(ch)
	return ch, nil
}

func (f *blockingHookClient) releaseAll() {
	close(f.releaseCh)
}

func (f *blockingHookClient) started() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startedN
}

func (f *blockingHookClient) maxActive() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

func (f *blockingHookClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}

func (f *blockingHookClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}

func (f *blockingHookClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}

func (f *blockingHookClient) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

type timedResponse struct {
	delay time.Duration
	body  string
}

type timedResponseHookClient struct {
	responses map[string]timedResponse
}

func (f *timedResponseHookClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	prompt := ""
	if n := len(req.Messages); n > 1 {
		prompt = req.Messages[1].Content
	}
	prompt, _, _ = strings.Cut(prompt, "\n")
	resp, ok := f.responses[prompt]
	if !ok {
		resp = timedResponse{body: `{"decision":"allow","reason":"default"}`}
	}
	timer := time.NewTimer(resp.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: resp.body}, Done: true}
	close(ch)
	return ch, nil
}

func (f *timedResponseHookClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}

func (f *timedResponseHookClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}

func (f *timedResponseHookClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}

func (f *timedResponseHookClient) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

func waitUntil(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
