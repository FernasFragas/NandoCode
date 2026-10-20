package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/runctl"
)

// abortAwareRunner announces itself, then blocks until the run is cancelled
// (context or ParentAbort) and ends with an aborted Terminal, like the real loop.
type abortAwareRunner struct{ started chan struct{} }

func (r abortAwareRunner) Run(ctx context.Context, in agent.Input) <-chan agent.Event {
	ch := make(chan agent.Event, 4)
	go func() {
		defer close(ch)
		close(r.started)
		select {
		case <-ctx.Done():
		case <-in.ParentAbort:
		}
		ch <- agent.Terminal{Reason: agent.TerminalAborted, Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "partial"}}}
	}()
	return ch
}

func cancelSession(t *testing.T, s *Server) *Session {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.handleCreateSession(w, r)
	var view SessionView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	sess, ok := s.registry.get(view.SessionID)
	if !ok {
		t.Fatal("session missing")
	}
	return sess
}

func countTerminals(sess *Session) int {
	n := 0
	for _, e := range sess.Replay("") {
		if e.Type == "terminal" {
			n++
		}
	}
	return n
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		runtime.Gosched()
	}
}

func TestCancelDuringRunEmitsExactlyOneTerminal(t *testing.T) {
	s := testServer(t)
	runner := abortAwareRunner{started: make(chan struct{})}
	s.runner = runner
	sess := cancelSession(t, s)
	sess.runner = runner

	if err := sess.StartRun(MessageRequest{Prompt: "go"}, s.agentCfg); err != nil {
		t.Fatal(err)
	}
	<-runner.started

	w := routeDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/cancel")
	if w.Code != http.StatusAccepted {
		t.Fatalf("cancel status=%d body=%s", w.Code, w.Body.String())
	}
	waitUntil(t, "run to finish", func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return !sess.running
	})
	if got := countTerminals(sess); got != 1 {
		t.Fatalf("terminal events = %d, want exactly 1", got)
	}
	if sess.lastTerminal.Reason != agent.TerminalAborted {
		t.Fatalf("terminal reason = %q, want aborted", sess.lastTerminal.Reason)
	}
	// A second cancel after the run ended is harmless and adds no terminal.
	w = routeDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/cancel")
	if w.Code != http.StatusAccepted {
		t.Fatalf("repeat cancel status=%d", w.Code)
	}
	if got := countTerminals(sess); got != 1 {
		t.Fatalf("terminal events after repeat cancel = %d, want 1", got)
	}
	// The session is reusable after a cancel.
	sess.runner = tinyRunner{}
	if err := sess.StartRun(MessageRequest{Prompt: "again"}, s.agentCfg); err != nil {
		t.Fatalf("StartRun after cancel: %v", err)
	}
}

func TestCancelUnknownSessionIs404(t *testing.T) {
	s := testServer(t)
	if w := routeDo(t, s, http.MethodPost, "/v1/sessions/nope/cancel"); w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

func TestCancelWithoutActiveRunIs202AndEmitsNothing(t *testing.T) {
	s := testServer(t)
	sess := cancelSession(t, s)
	w := routeDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/cancel")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d, want 202", w.Code)
	}
	if got := countTerminals(sess); got != 0 {
		t.Fatalf("terminal events = %d, want 0", got)
	}
}

func TestCancelRouteIsPostOnly(t *testing.T) {
	s := testServer(t)
	w := routeDo(t, s, http.MethodPut, "/v1/sessions/abc/cancel")
	if w.Code != http.StatusMethodNotAllowed || !strings.Contains(w.Header().Get("Allow"), "POST") {
		t.Fatalf("status=%d allow=%q", w.Code, w.Header().Get("Allow"))
	}
}

func TestSecondRunWhileActiveIsRejected(t *testing.T) {
	s := testServer(t)
	runner := abortAwareRunner{started: make(chan struct{})}
	sess := cancelSession(t, s)
	sess.runner = runner
	if err := sess.StartRun(MessageRequest{Prompt: "one"}, s.agentCfg); err != nil {
		t.Fatal(err)
	}
	<-runner.started
	if err := sess.StartRun(MessageRequest{Prompt: "two"}, s.agentCfg); err == nil {
		t.Fatal("second StartRun should fail while a run is active")
	}
	sess.cancelRun()
	waitUntil(t, "run to finish", func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return !sess.running
	})
}

func runFinished(sess *Session) bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return !sess.running
}

// toolCallRunner is mid tool call when cancelled: it reports the failed tool
// result and then the aborted terminal, as the real loop does.
type toolCallRunner struct{ started chan struct{} }

func (r toolCallRunner) Run(ctx context.Context, _ agent.Input) <-chan agent.Event {
	ch := make(chan agent.Event, 4)
	go func() {
		defer close(ch)
		ch <- agent.ToolUseStart{ID: "t1", Name: "bash"}
		close(r.started)
		<-ctx.Done()
		ch <- agent.ToolUseResult{ID: "t1", Err: ctx.Err()}
		ch <- agent.Terminal{Reason: agent.TerminalAborted}
	}()
	return ch
}

func TestCancelDuringToolCallEmitsExactlyOneTerminal(t *testing.T) {
	s := testServer(t)
	runner := toolCallRunner{started: make(chan struct{})}
	sess := cancelSession(t, s)
	sess.runner = runner
	if err := sess.StartRun(MessageRequest{Prompt: "go"}, s.agentCfg); err != nil {
		t.Fatal(err)
	}
	<-runner.started
	if w := routeDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/cancel"); w.Code != http.StatusAccepted {
		t.Fatalf("status=%d", w.Code)
	}
	waitUntil(t, "run to finish", func() bool { return runFinished(sess) })
	if got := countTerminals(sess); got != 1 {
		t.Fatalf("terminals=%d, want 1", got)
	}
}

// permissionRunner blocks in the permission prompt, like the agent loop does
// when a tool needs approval, then ends with one terminal.
type permissionRunner struct{ got chan permissions.Decision }

func (r permissionRunner) Run(ctx context.Context, in agent.Input) <-chan agent.Event {
	ch := make(chan agent.Event, 2)
	go func() {
		defer close(ch)
		d, _, _ := in.PermissionPrompt(ctx, permissions.Prompt{ToolName: "bash", Target: "ls"})
		r.got <- d
		ch <- agent.Terminal{Reason: agent.TerminalAborted}
	}()
	return ch
}

func TestCancelWithPendingPermissionReleasesBrokerAndEmitsOneTerminal(t *testing.T) {
	s := testServer(t)
	runner := permissionRunner{got: make(chan permissions.Decision, 1)}
	sess := cancelSession(t, s)
	sess.runner = runner
	if err := sess.StartRun(MessageRequest{Prompt: "go"}, s.agentCfg); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "permission prompt", func() bool {
		sess.broker.mu.Lock()
		defer sess.broker.mu.Unlock()
		return len(sess.broker.pending) == 1
	})
	if w := routeDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/cancel"); w.Code != http.StatusAccepted {
		t.Fatalf("status=%d", w.Code)
	}
	waitUntil(t, "run to finish", func() bool { return runFinished(sess) })
	if d := <-runner.got; d != permissions.DecisionDeny {
		t.Fatalf("prompt decision=%v, want deny", d)
	}
	sess.broker.mu.Lock()
	n := len(sess.broker.pending)
	sess.broker.mu.Unlock()
	if n != 0 {
		t.Fatalf("pending permissions=%d, want 0", n)
	}
	if got := countTerminals(sess); got != 1 {
		t.Fatalf("terminals=%d, want 1", got)
	}
}

func TestConcurrentDoubleCancelEmitsOneTerminal(t *testing.T) {
	s := testServer(t)
	runner := abortAwareRunner{started: make(chan struct{})}
	sess := cancelSession(t, s)
	sess.runner = runner
	if err := sess.StartRun(MessageRequest{Prompt: "go"}, s.agentCfg); err != nil {
		t.Fatal(err)
	}
	<-runner.started
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := routeDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/cancel"); w.Code != http.StatusAccepted {
				t.Errorf("status=%d", w.Code)
			}
		}()
	}
	wg.Wait()
	waitUntil(t, "run to finish", func() bool { return runFinished(sess) })
	if got := countTerminals(sess); got != 1 {
		t.Fatalf("terminals=%d, want 1", got)
	}
}

func beginRun(t *testing.T, sess *Session) runctl.Run {
	t.Helper()
	run, err := sess.ctl.Begin(sess.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestPrepFailedAfterCancelIsAbortedAndLeavesHistoryAlone(t *testing.T) {
	s := testServer(t)
	sess := cancelSession(t, s)
	sess.conversation = []llm.Message{{Role: llm.RoleUser, Content: "q0"}, {Role: llm.RoleAssistant, Content: "a0"}}
	sess.runPrompt = "never started"
	run := beginRun(t, sess)
	sess.ctl.Cancel()
	sess.prepFailed(run, errors.New("pack failed"))
	sess.ctl.Finish(run)

	if got := countTerminals(sess); got != 1 {
		t.Fatalf("terminals=%d, want 1", got)
	}
	for _, e := range sess.Replay("") {
		if e.Type == "error" {
			t.Fatalf("unexpected error event: %+v", e)
		}
	}
	if sess.lastTerminal.Reason != agent.TerminalAborted {
		t.Fatalf("reason=%q", sess.lastTerminal.Reason)
	}
	if len(sess.conversation) != 2 || sess.conversation[1].Role != llm.RoleAssistant {
		t.Fatalf("history changed: %+v", sess.conversation)
	}
}

func TestPrepFailedWithoutCancelStaysAnError(t *testing.T) {
	s := testServer(t)
	sess := cancelSession(t, s)
	run := beginRun(t, sess)
	sess.prepFailed(run, errors.New("pack failed"))
	sess.ctl.Finish(run)
	if got := countTerminals(sess); got != 0 {
		t.Fatalf("terminals=%d, want 0", got)
	}
	var sawErr bool
	for _, e := range sess.Replay("") {
		if e.Type == "error" {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("expected an error event")
	}
}
