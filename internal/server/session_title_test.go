package server

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
)

type titleClient struct {
	fakeClient
	calls atomic.Int32
	err   error
}

func (c *titleClient) Chat(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.calls.Add(1)
	if c.err != nil {
		return nil, c.err
	}
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Message: llm.Message{Content: "Review project files"}, Done: true}
	close(ch)
	return ch, nil
}

func runTitleCase(t *testing.T, cloud bool, client *titleClient) *Session {
	t.Helper()
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	t.Setenv("OLLAMA_API_KEY", "")
	srv := testServer(t)
	s := newSession(context.Background(), "title-test", srv.baseApp, tinyRunner{})
	s.llmClient = client
	if cloud {
		s.appState.LLMProvider = string(llm.ProviderOllamaCloudAPI)
		s.models = &modelruntime.Service{Creds: &credentials.Resolver{Store: &testCredStore{}}}
	}
	if err := s.StartRun(MessageRequest{Prompt: "Please inspect the complicated project files and report all issues", MessageID: "m1"}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	return s
}

func waitTitle(t *testing.T, s *Session) []SessionEvent {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		items := s.Replay("")
		for _, e := range items {
			if e.Type == "session_titled" {
				return items
			}
		}
		select {
		case <-deadline.C:
			t.Fatalf("no title event: %+v", items)
		case <-tick.C:
		}
	}
}

func TestSessionTitleAfterFirstTerminal(t *testing.T) {
	c := &titleClient{}
	s := runTitleCase(t, false, c)
	events := waitTitle(t, s)
	terminal, titled := -1, -1
	for i, e := range events {
		if e.Type == "terminal" {
			terminal = i
		}
		if e.Type == "session_titled" {
			titled = i
			if e.Data["title"] != "Review project files" || e.Data["source"] != "model" {
				t.Fatalf("event=%+v", e)
			}
		}
	}
	if terminal < 0 || titled <= terminal || c.calls.Load() != 1 {
		t.Fatalf("order/calls wrong: terminal=%d titled=%d calls=%d", terminal, titled, c.calls.Load())
	}
	if got := s.listItem().Title; got != "Review project files" {
		t.Fatalf("list title=%q", got)
	}
	s.handleAgentEvent(agent.Terminal{Reason: agent.TerminalCompleted})
	if c.calls.Load() != 1 {
		t.Fatal("second terminal generated another title")
	}
}

func TestSessionTitleFallbackOnError(t *testing.T) {
	c := &titleClient{err: errors.New("offline")}
	s := runTitleCase(t, false, c)
	events := waitTitle(t, s)
	for _, e := range events {
		if e.Type == "session_titled" {
			if e.Data["source"] != "fallback" || !strings.HasPrefix(e.Data["title"].(string), "Please inspect") {
				t.Fatalf("event=%+v", e)
			}
			return
		}
	}
}

func TestSessionTitleNeedsCloudCredential(t *testing.T) {
	c := &titleClient{}
	s := runTitleCase(t, true, c)
	events := waitTitle(t, s)
	for _, e := range events {
		if e.Type == "session_titled" && e.Data["source"] != "fallback" {
			t.Fatalf("event=%+v", e)
		}
	}
	if c.calls.Load() != 0 {
		t.Fatal("cloud title call without credential")
	}
}

func TestShortTitleStripsControlAndFormatChars(t *testing.T) {
	got := shortTitle("a\x1b[31mb\x00c\u202edef\u200bg\thi")
	if got != "a[31mbcdefg hi" {
		t.Fatalf("got %q", got)
	}
}

type blockClient struct {
	fakeClient
	entered      chan struct{}
	release      chan struct{}
	canceled     chan struct{}
	enteredOnce  sync.Once
	canceledOnce sync.Once
}

func (c *blockClient) Chat(ctx context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.enteredOnce.Do(func() { close(c.entered) })
	select {
	case <-c.release:
	case <-ctx.Done():
		if c.canceled != nil {
			c.canceledOnce.Do(func() { close(c.canceled) })
		}
	}
	return nil, errors.New("released")
}

type observedRunner struct {
	entered    chan struct{}
	canceled   <-chan struct{}
	overlapped chan struct{}
}

func (r observedRunner) Run(_ context.Context, _ agent.Input) <-chan agent.Event {
	select {
	case <-r.canceled:
	default:
		close(r.overlapped)
	}
	close(r.entered)
	return tinyRunner{}.Run(context.Background(), agent.Input{})
}

func TestStartRunCancelsTitleBeforeNextContextSend(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	srv := testServer(t)
	c := &blockClient{entered: make(chan struct{}), release: make(chan struct{}), canceled: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(c.release) }) }
	defer release()
	r := observedRunner{entered: make(chan struct{}), canceled: c.canceled, overlapped: make(chan struct{})}
	s := newSession(context.Background(), "title-cancel", srv.baseApp, tinyRunner{})
	s.llmClient = c
	if err := s.StartRun(MessageRequest{Prompt: "first", MessageID: "m1"}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("title call did not start")
	}
	s.mu.Lock()
	s.runner = r
	s.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- s.StartRun(MessageRequest{Prompt: "second", MessageID: "m2"}, agent.DefaultConfig()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("StartRun waited for title call")
	}
	select {
	case <-c.canceled:
	case <-time.After(time.Second):
		t.Fatal("title context was not canceled")
	}
	select {
	case <-r.entered:
	case <-time.After(time.Second):
		t.Fatal("next run did not start after title cancellation")
	}
	select {
	case <-r.overlapped:
		t.Fatal("next run sent context while title call was active")
	default:
	}
	release()
	for _, event := range waitTitle(t, s) {
		if event.Type == "session_titled" {
			if event.Data["title"] != "second" || event.Data["source"] != "fallback" {
				t.Fatalf("title after cancellation = %+v", event.Data)
			}
			return
		}
	}
}

func TestStartRunNotBlockedByTitleCall(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	srv := testServer(t)
	c := &blockClient{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(c.release)
	s := newSession(context.Background(), "title-block", srv.baseApp, tinyRunner{})
	s.llmClient = c
	if err := s.StartRun(MessageRequest{Prompt: "first", MessageID: "m1"}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	<-c.entered
	done := make(chan error, 1)
	go func() { done <- s.StartRun(MessageRequest{Prompt: "second", MessageID: "m2"}, agent.DefaultConfig()) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartRun blocked behind title generation")
	}
}
