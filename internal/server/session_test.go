package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/mentions"
	"github.com/FernasFragas/Nandocode/internal/semantic"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tasks"
	"github.com/FernasFragas/Nandocode/internal/types"
)

type fakeRunner struct{}

func (fakeRunner) Run(_ context.Context, _ agent.Input) <-chan agent.Event {
	ch := make(chan agent.Event, 3)
	ch <- agent.AssistantTextDelta{Content: "ok"}
	ch <- agent.Terminal{Reason: agent.TerminalCompleted, Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "ok"}}}
	close(ch)
	return ch
}

type blockingRunner struct {
	done chan struct{}
}

type capturingRunner struct {
	inputs chan agent.Input
}

type semanticStub struct {
	mu      sync.Mutex
	reqs    []semantic.RetrieveRequest
	res     semantic.RetrieveResult
	retErr  error
	status  semantic.Status
	statErr error
}

func (s *semanticStub) Status(context.Context, string) (semantic.Status, error) {
	return s.status, s.statErr
}

func (*semanticStub) Build(context.Context, semantic.BuildRequest) (semantic.BuildReport, error) {
	return semantic.BuildReport{}, nil
}

func (*semanticStub) Refresh(context.Context, semantic.RefreshRequest) (semantic.BuildReport, error) {
	return semantic.BuildReport{}, nil
}

func (*semanticStub) Clear(context.Context, string) error { return nil }

func (s *semanticStub) Retrieve(_ context.Context, req semantic.RetrieveRequest) (semantic.RetrieveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	return s.res, s.retErr
}

func (s *semanticStub) reqCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reqs)
}

func (s *semanticStub) lastReq() (semantic.RetrieveRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reqs) == 0 {
		return semantic.RetrieveRequest{}, false
	}
	return s.reqs[len(s.reqs)-1], true
}

func (r capturingRunner) Run(_ context.Context, in agent.Input) <-chan agent.Event {
	select {
	case r.inputs <- in:
	default:
	}
	ch := make(chan agent.Event, 1)
	ch <- agent.Terminal{Reason: agent.TerminalCompleted, Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "ok"}}}
	close(ch)
	return ch
}

func (r blockingRunner) Run(ctx context.Context, _ agent.Input) <-chan agent.Event {
	ch := make(chan agent.Event)
	go func() {
		defer close(ch)
		<-ctx.Done()
		close(r.done)
	}()
	return ch
}

func TestSessionStartRunAndReplay(t *testing.T) {
	init := bootstrap.DefaultInitial(".")
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	s := newSession(context.Background(), "s1", app, fakeRunner{})
	if err := s.StartRun(MessageRequest{Prompt: "hi"}, agent.DefaultConfig()); err != nil {
		t.Fatalf("StartRun err=%v", err)
	}
	time.Sleep(40 * time.Millisecond)
	events := s.Replay("")
	if len(events) == 0 {
		t.Fatal("expected replay events")
	}
	foundTerminal := false
	for _, e := range events {
		if e.Type == "terminal" {
			foundTerminal = true
		}
	}
	if !foundTerminal {
		t.Fatal("expected terminal event")
	}
}

func TestDeleteSessionCancelsRunningAgent(t *testing.T) {
	init := bootstrap.DefaultInitial(".")
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	done := make(chan struct{})
	s := newSession(context.Background(), "s2", app, blockingRunner{done: done})
	if err := s.StartRun(MessageRequest{Prompt: "hold"}, agent.DefaultConfig()); err != nil {
		t.Fatalf("StartRun err=%v", err)
	}
	s.stop()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runner did not observe cancellation")
	}
}

func TestSessionEmitsTaskLifecycleEvents(t *testing.T) {
	init := bootstrap.DefaultInitial(".")
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	store := state.NewStore(app, nil)
	sup := tasks.NewSupervisor(t.TempDir(), store)
	s := newSession(context.Background(), "s3", app, fakeRunner{})
	s.setCoordinatorRuntime(sup, store, nil, false)
	_, err := sup.Start(context.Background(), types.KindAgent, "worker", func(ctx context.Context, out *tasks.OutputWriter) (int, error) {
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	found := false
	for _, evt := range s.Replay("") {
		if evt.Type == "task_lifecycle" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected task_lifecycle event")
	}
	s.stop()
}

func TestSessionRunUsesPackedPromptWithTailRangeEvidence(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 4200; i++ {
		line := "noise"
		if i == 4199 {
			line = "LATEST_STATUS_MARKER implemented context packing"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(root, "phase-log.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	init := bootstrap.DefaultInitial(root)
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	inputs := make(chan agent.Input, 1)
	s := newSession(context.Background(), "s4", app, capturingRunner{inputs: inputs})
	if err := s.StartRun(MessageRequest{Prompt: "review @phase-log.md"}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	select {
	case in := <-inputs:
		if len(in.Messages) == 0 {
			t.Fatal("expected at least one message")
		}
		prompt := in.Messages[len(in.Messages)-1].Content
		if !strings.Contains(prompt, "<file_range path=\"phase-log.md\"") {
			t.Fatalf("expected file_range evidence block:\n%s", prompt)
		}
		if !strings.Contains(prompt, "LATEST_STATUS_MARKER implemented context packing") {
			t.Fatalf("expected tail marker in packed prompt:\n%s", prompt)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for captured input")
	}
}

func TestSessionRunInjectsSemanticContextAndEmitsEvent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	init := bootstrap.DefaultInitial(root)
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	inputs := make(chan agent.Input, 1)
	svc := &semanticStub{
		status: semantic.Status{Exists: true, Compatible: true},
		res: semantic.RetrieveResult{
			Used:            true,
			RenderedContext: "<semantic_context model=\"qwen3-embedding:8b\"></semantic_context>",
			Records:         []semantic.SearchHit{{}},
			Files:           []semantic.RetrievedFile{{Path: "main.go"}},
			ContextBytes:    64,
		},
	}
	s := newSession(context.Background(), "s5", app, capturingRunner{inputs: inputs})
	s.semanticCfg = semantic.DefaultConfig()
	s.semanticCfg.Enabled = true
	s.semanticSvc = svc
	if err := s.StartRun(MessageRequest{Prompt: "fix authentication flow"}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	select {
	case in := <-inputs:
		if len(in.Messages) == 0 {
			t.Fatal("expected at least one message")
		}
		prompt := in.Messages[len(in.Messages)-1].Content
		if !strings.Contains(prompt, "<semantic_context model=\"qwen3-embedding:8b\">") {
			t.Fatalf("expected semantic context in prompt:\n%s", prompt)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for captured input")
	}
	time.Sleep(30 * time.Millisecond)
	found := false
	for _, evt := range s.Replay("") {
		if evt.Type != "semantic_retrieval" {
			continue
		}
		found = true
		if got, _ := evt.Data["records"].(int); got != 1 {
			t.Fatalf("semantic_retrieval records=%v", evt.Data["records"])
		}
		break
	}
	if !found {
		t.Fatal("expected semantic_retrieval event")
	}
}

func TestSessionRunEmitsSemanticFallbackEvent(t *testing.T) {
	init := bootstrap.DefaultInitial(t.TempDir())
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	inputs := make(chan agent.Input, 1)
	svc := &semanticStub{
		status: semantic.Status{Exists: true, Compatible: true},
		retErr: semantic.ErrIndexMissing,
	}
	s := newSession(context.Background(), "s6", app, capturingRunner{inputs: inputs})
	s.semanticCfg = semantic.DefaultConfig()
	s.semanticCfg.Enabled = true
	s.semanticSvc = svc
	if err := s.StartRun(MessageRequest{Prompt: "find auth issue"}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	select {
	case in := <-inputs:
		if len(in.Messages) == 0 {
			t.Fatal("expected at least one message")
		}
		prompt := in.Messages[len(in.Messages)-1].Content
		if strings.Contains(prompt, "<semantic_context") {
			t.Fatalf("did not expect semantic context on fallback:\n%s", prompt)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for captured input")
	}
	time.Sleep(30 * time.Millisecond)
	found := false
	for _, evt := range s.Replay("") {
		if evt.Type != "semantic_retrieval" {
			continue
		}
		found = true
		fallback, _ := evt.Data["fallback"].(bool)
		if !fallback {
			t.Fatalf("expected fallback=true, got: %#v", evt.Data)
		}
	}
	if !found {
		t.Fatal("expected semantic_retrieval fallback event")
	}
}

func TestSessionRunBypassesSemanticRetrievalForListingIntent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	init := bootstrap.DefaultInitial(root)
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	inputs := make(chan agent.Input, 1)
	svc := &semanticStub{}
	s := newSession(context.Background(), "s7", app, capturingRunner{inputs: inputs})
	s.semanticCfg = semantic.DefaultConfig()
	s.semanticCfg.Enabled = true
	s.semanticSvc = svc
	if err := s.StartRun(MessageRequest{Prompt: "list all files in @docs/"}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	select {
	case in := <-inputs:
		if in.AttachmentPolicy != string(mentions.AttachListingTreeOnly) {
			t.Fatalf("attachment policy=%q want %q", in.AttachmentPolicy, mentions.AttachListingTreeOnly)
		}
		if got := svc.reqCount(); got != 0 {
			t.Fatalf("semantic retrieve calls=%d want 0", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for captured input")
	}
}

func TestSessionRunBypassesSemanticRetrievalForExplicitFileStatusPrompt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("status\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	init := bootstrap.DefaultInitial(root)
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	inputs := make(chan agent.Input, 1)
	svc := &semanticStub{}
	s := newSession(context.Background(), "s8", app, capturingRunner{inputs: inputs})
	s.semanticCfg = semantic.DefaultConfig()
	s.semanticCfg.Enabled = true
	s.semanticSvc = svc
	prompt := "what is the current status of @plan.md"
	if err := s.StartRun(MessageRequest{Prompt: prompt}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	select {
	case in := <-inputs:
		if in.PromptIntent != string(mentions.IntentFileStatus) {
			t.Fatalf("prompt intent=%q want %q", in.PromptIntent, mentions.IntentFileStatus)
		}
		if got := svc.reqCount(); got != 0 {
			t.Fatalf("semantic retrieve calls=%d want 0", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for captured input")
	}
}

func TestSessionRunBypassesSemanticRetrievalForSimpleExplicitFilePrompt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("status\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	init := bootstrap.DefaultInitial(root)
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	inputs := make(chan agent.Input, 1)
	svc := &semanticStub{}
	s := newSession(context.Background(), "s9", app, capturingRunner{inputs: inputs})
	s.semanticCfg = semantic.DefaultConfig()
	s.semanticCfg.Enabled = true
	s.semanticSvc = svc
	prompt := "@plan.md can you access this document"
	if err := s.StartRun(MessageRequest{Prompt: prompt}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	select {
	case in := <-inputs:
		if in.AttachmentPolicy == string(mentions.AttachListingTreeOnly) {
			t.Fatalf("unexpected listing-only attachment policy for simple file prompt")
		}
		if got := svc.reqCount(); got != 0 {
			t.Fatalf("semantic retrieve calls=%d want 0", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for captured input")
	}
}
