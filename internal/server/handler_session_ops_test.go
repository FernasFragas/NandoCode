package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/semantic"
	"github.com/FernasFragas/Nandocode/internal/tools/todo"
)

func opsSession(t *testing.T) (*Server, *Session) {
	t.Helper()
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	s := testServer(t)
	s.client = &testLLMClient{} // closed Chat stream: summaries fail, compaction falls back
	id := createTestSession(t, s)
	sess, ok := s.registry.get(id)
	if !ok {
		t.Fatal("session not registered")
	}
	return s, sess
}

func opsTurns(n int) []llm.Message {
	var out []llm.Message
	for i := 0; i < n; i++ {
		out = append(out,
			llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("q%d", i)},
			llm.Message{Role: llm.RoleAssistant, Content: fmt.Sprintf("a%d", i)})
	}
	return out
}

func opsDo(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	return w
}

func opsEvents(sess *Session, ch chan SessionEvent, want string) (SessionEvent, bool) {
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e := <-ch:
			if e.Type == want {
				return e, true
			}
		case <-deadline:
			return SessionEvent{}, false
		}
	}
}

func TestClearEmptiesHistory(t *testing.T) {
	s, sess := opsSession(t)
	sess.mu.Lock()
	sess.conversation = opsTurns(3)
	sess.mu.Unlock()

	w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/clear", "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"ok":true}` {
		t.Fatalf("clear status=%d body=%s", w.Code, w.Body.String())
	}
	sess.mu.Lock()
	n := len(sess.conversation)
	sess.mu.Unlock()
	if n != 0 {
		t.Fatalf("history has %d messages after clear, want 0", n)
	}
}

func TestClearRemovesTerminalResidue(t *testing.T) {
	s, sess := opsSession(t)
	sess.mu.Lock()
	sess.conversation = opsTurns(1)
	sess.lastTerminal = TerminalSnapshot{Reason: agent.TerminalCompleted, Usage: agent.Usage{Turns: 1}}
	sess.appState.TerminalReason = agent.TerminalCompleted
	sess.appState.TerminalDetail = "old run"
	sess.appState.LastRetryNotice = "old retry"
	sess.mu.Unlock()
	if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/clear", ""); w.Code != http.StatusOK {
		t.Fatalf("clear status=%d", w.Code)
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.lastTerminal.Reason != "" || sess.appState.TerminalReason != "" || sess.appState.TerminalDetail != "" || sess.appState.LastRetryNotice != "" {
		t.Fatalf("terminal residue remains: snapshot=%+v app=%+v", sess.lastTerminal, sess.appState)
	}
}

type opsRecordingClient struct {
	testLLMClient
	chatCount int
	response  string
}

func (c *opsRecordingClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.chatCount++
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: c.response}, Done: true}
	close(ch)
	return ch, nil
}

func TestBrowserCompactUsesHooksAndWrappedClient(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		denied         bool
	}{
		{"deny", `{"decision":"deny","reason":"policy"}`, true},
		{"allow", `{"decision":"allow"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, sess := opsSession(t)
			client := &opsRecordingClient{response: tc.response}
			sess.mu.Lock()
			sess.conversation = opsTurns(10)
			sess.llmClient = client
			sess.mu.Unlock()
			s.hookSnapshot = hooks.Snapshot{Hooks: []hooks.Hook{
				{Kind: hooks.KindPrompt, Event: hooks.EventPreCompact, Prompt: "check", Enabled: true},
				{Kind: hooks.KindPrompt, Event: hooks.EventPostCompact, Prompt: "record", Enabled: true},
			}}
			w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/compact", "")
			if w.Code != http.StatusOK {
				t.Fatalf("compact status=%d body=%s", w.Code, w.Body.String())
			}
			var got struct {
				Denied bool `json:"denied"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Denied != tc.denied {
				t.Fatalf("denied=%v want %v", got.Denied, tc.denied)
			}
			wantCalls := 3 // pre hook, summary, post hook
			if tc.denied {
				wantCalls = 1 // denied before the summary and post hook
			}
			if client.chatCount != wantCalls {
				t.Fatalf("wrapped client chats=%d, want %d", client.chatCount, wantCalls)
			}
			sess.mu.Lock()
			n := len(sess.conversation)
			sess.mu.Unlock()
			if tc.denied && n != 20 {
				t.Fatalf("denied compact changed history to %d messages", n)
			}
		})
	}
}

func TestBrowserCompactSummaryUsesWrappedClient(t *testing.T) {
	s, sess := opsSession(t)
	client := &opsRecordingClient{response: "summary"}
	sess.mu.Lock()
	sess.conversation = opsTurns(10)
	sess.llmClient = client
	sess.mu.Unlock()
	w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/compact", "")
	if w.Code != http.StatusOK {
		t.Fatalf("compact status=%d", w.Code)
	}
	if client.chatCount != 1 {
		t.Fatalf("wrapped client chats=%d, want one summary call", client.chatCount)
	}
}

func TestClearAndCompactRefusedWhileRunActive(t *testing.T) {
	s, sess := opsSession(t)
	sess.mu.Lock()
	sess.conversation = opsTurns(6)
	sess.running = true
	sess.mu.Unlock()
	for _, op := range []string{"clear", "compact"} {
		w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/"+op, "")
		if w.Code != http.StatusConflict {
			t.Fatalf("%s during a run status=%d, want 409", op, w.Code)
		}
	}
	sess.mu.Lock()
	n := len(sess.conversation)
	sess.mu.Unlock()
	if n != 12 {
		t.Fatalf("history changed during a refused op: %d messages, want 12", n)
	}
}

func TestClearAndCompactUnknownSession(t *testing.T) {
	s, _ := opsSession(t)
	for _, op := range []string{"clear", "compact"} {
		if w := opsDo(t, s, http.MethodPost, "/v1/sessions/nope/"+op, ""); w.Code != http.StatusNotFound {
			t.Fatalf("%s unknown session status=%d, want 404", op, w.Code)
		}
	}
	if w := opsDo(t, s, http.MethodGet, "/v1/sessions/"+"x"+"/clear", ""); w.Code == http.StatusOK {
		t.Fatalf("GET clear must not succeed")
	}
}

func TestCompactRunsSharedOperationAndEmitsEvents(t *testing.T) {
	s, sess := opsSession(t)
	sess.mu.Lock()
	sess.conversation = opsTurns(10)
	sess.mu.Unlock()
	ch := sess.Subscribe()
	defer sess.Unsubscribe(ch)

	w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/compact", "")
	if w.Code != http.StatusOK {
		t.Fatalf("compact status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp["ok"] != true {
		t.Fatalf("compact body=%s err=%v", w.Body.String(), err)
	}
	sess.mu.Lock()
	n := len(sess.conversation)
	running := sess.running
	sess.mu.Unlock()
	if n >= 20 {
		t.Fatalf("history has %d messages after compact, want fewer than 20", n)
	}
	if running {
		t.Fatal("session still marked running after compact")
	}
	if _, ok := opsEvents(sess, ch, "compaction_started"); !ok {
		t.Fatal("no compaction_started event")
	}
	done, ok := opsEvents(sess, ch, "compaction_completed")
	if !ok {
		t.Fatal("no compaction_completed event")
	}
	if done.Data["before"] != 20 {
		t.Fatalf("compaction_completed before=%v want 20", done.Data["before"])
	}
}

func TestCompactSkipsShortHistory(t *testing.T) {
	s, sess := opsSession(t)
	sess.mu.Lock()
	sess.conversation = opsTurns(1)
	sess.mu.Unlock()
	w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/compact", "")
	if w.Code != http.StatusOK {
		t.Fatalf("compact status=%d", w.Code)
	}
	sess.mu.Lock()
	n := len(sess.conversation)
	sess.mu.Unlock()
	if n != 2 {
		t.Fatalf("short history changed: %d messages, want 2", n)
	}
}

func TestCostReportsLastRunAndHistory(t *testing.T) {
	s, sess := opsSession(t)
	sess.mu.Lock()
	sess.conversation = opsTurns(2)
	sess.lastTerminal = TerminalSnapshot{Reason: agent.TerminalCompleted, Usage: agent.Usage{PromptEvalCount: 11, EvalCount: 7, Turns: 2}}
	sess.mu.Unlock()
	w := opsDo(t, s, http.MethodGet, "/v1/sessions/"+sess.id+"/cost", "")
	if w.Code != http.StatusOK {
		t.Fatalf("cost status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Messages int `json:"messages"`
		LastRun  struct {
			Reason string
			Usage  agent.Usage
		} `json:"last_run"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Messages != 4 || resp.LastRun.Usage.PromptEvalCount != 11 || resp.LastRun.Usage.EvalCount != 7 {
		t.Fatalf("cost=%s", w.Body.String())
	}
	if w := opsDo(t, s, http.MethodGet, "/v1/sessions/nope/cost", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown session cost status=%d", w.Code)
	}
}

// fakeIndexService is a semantic.Service whose Build/Refresh can be held open.
type fakeIndexService struct {
	mu      sync.Mutex
	status  semantic.Status
	builds  int
	refresh int
	gate    chan struct{}
	started chan struct{}
	err     error
}

func (f *fakeIndexService) Status(context.Context, string) (semantic.Status, error) {
	return f.status, nil
}

func (f *fakeIndexService) run(req semantic.EventSink, counter *int) (semantic.BuildReport, error) {
	f.mu.Lock()
	*counter++
	f.mu.Unlock()
	if req != nil {
		req.Publish(semantic.Event{Stage: semantic.StageScanStart, Message: "scan start", Path: "secret/path.go"})
	}
	if f.started != nil {
		close(f.started)
	}
	if f.gate != nil {
		<-f.gate
	}
	return semantic.BuildReport{FilesIndexed: 3, RecordsIndexed: 9, Duration: time.Millisecond}, f.err
}

func (f *fakeIndexService) Build(_ context.Context, r semantic.BuildRequest) (semantic.BuildReport, error) {
	return f.run(r.EventSink, &f.builds)
}

func (f *fakeIndexService) Refresh(_ context.Context, r semantic.RefreshRequest) (semantic.BuildReport, error) {
	return f.run(r.EventSink, &f.refresh)
}
func (f *fakeIndexService) Clear(context.Context, string) error { return nil }
func (f *fakeIndexService) Retrieve(context.Context, semantic.RetrieveRequest) (semantic.RetrieveResult, error) {
	return semantic.RetrieveResult{}, nil
}

func TestIndexStatusEndpoint(t *testing.T) {
	s, sess := opsSession(t)
	s.semanticSvc = &fakeIndexService{status: semantic.Status{Exists: true, Compatible: true, Model: "embed-x", Dimensions: 8, RecordCount: 5, VectorCount: 5, FileCount: 2}}
	sess.semanticSvc = s.semanticSvc
	w := opsDo(t, s, http.MethodGet, "/v1/sessions/"+sess.id+"/index/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["exists"] != true || got["model"] != "embed-x" || got["records"] != float64(5) || got["files"] != float64(2) {
		t.Fatalf("status body=%s", w.Body.String())
	}
	sess.semanticSvc = nil
	if w := opsDo(t, s, http.MethodGet, "/v1/sessions/"+sess.id+"/index/status", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no service status=%d, want 503", w.Code)
	}
}

func TestIndexBuildRunsAsyncAndEmitsProgressAndDone(t *testing.T) {
	s, sess := opsSession(t)
	svc := &fakeIndexService{}
	sess.semanticSvc = svc
	ch := sess.Subscribe()
	defer sess.Unsubscribe(ch)

	w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/index/build", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("build status=%d body=%s", w.Code, w.Body.String())
	}
	prog, ok := opsEvents(sess, ch, "index_progress")
	if !ok {
		t.Fatal("no index_progress event")
	}
	if strings.Contains(fmt.Sprint(prog.Data), "secret/path.go") {
		t.Fatalf("progress leaks file paths: %v", prog.Data)
	}
	done, ok := opsEvents(sess, ch, "index_done")
	if !ok {
		t.Fatal("no index_done event")
	}
	if done.Data["mode"] != "build" || done.Data["records"] != 9 {
		t.Fatalf("index_done=%v", done.Data)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.builds != 1 || svc.refresh != 0 {
		t.Fatalf("builds=%d refresh=%d", svc.builds, svc.refresh)
	}
}

func TestIndexRefreshModeAndBadMode(t *testing.T) {
	s, sess := opsSession(t)
	svc := &fakeIndexService{}
	sess.semanticSvc = svc
	ch := sess.Subscribe()
	defer sess.Unsubscribe(ch)
	if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/index/build", `{"mode":"bogus"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad mode status=%d", w.Code)
	}
	if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/index/build", `{"mode":"refresh"}`); w.Code != http.StatusAccepted {
		t.Fatalf("refresh status=%d", w.Code)
	}
	if _, ok := opsEvents(sess, ch, "index_done"); !ok {
		t.Fatal("no index_done")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.refresh != 1 || svc.builds != 0 {
		t.Fatalf("builds=%d refresh=%d", svc.builds, svc.refresh)
	}
}

func TestIndexBuildRefusesSecondConcurrentBuild(t *testing.T) {
	s, sess := opsSession(t)
	svc := &fakeIndexService{gate: make(chan struct{}), started: make(chan struct{})}
	sess.semanticSvc = svc
	ch := sess.Subscribe()
	defer sess.Unsubscribe(ch)
	if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/index/build", ""); w.Code != http.StatusAccepted {
		t.Fatalf("first build status=%d", w.Code)
	}
	<-svc.started
	if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/index/build", ""); w.Code != http.StatusConflict {
		t.Fatalf("second build status=%d, want 409", w.Code)
	}
	close(svc.gate)
	if _, ok := opsEvents(sess, ch, "index_done"); !ok {
		t.Fatal("no index_done")
	}
	// The slot is free again once the build finished.
	svc2 := &fakeIndexService{}
	sess.semanticSvc = svc2
	if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/index/build", ""); w.Code != http.StatusAccepted {
		t.Fatalf("build after finish status=%d", w.Code)
	}
	if _, ok := opsEvents(sess, ch, "index_done"); !ok {
		t.Fatal("no second index_done")
	}
}

func TestIndexRunsBelongToTheirServer(t *testing.T) {
	s1, sess1 := opsSession(t)
	s2, sess2 := opsSession(t)
	root := t.TempDir()
	sess1.appState.ToolSettings.WorkingDir = root
	sess2.appState.ToolSettings.WorkingDir = root
	first := &fakeIndexService{gate: make(chan struct{}), started: make(chan struct{})}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(first.gate) }) })
	sess1.semanticSvc = first
	sess2.semanticSvc = &fakeIndexService{}
	ch1 := sess1.Subscribe()
	defer sess1.Unsubscribe(ch1)
	ch2 := sess2.Subscribe()
	defer sess2.Unsubscribe(ch2)
	if w := opsDo(t, s1, http.MethodPost, "/v1/sessions/"+sess1.id+"/index/build", ""); w.Code != http.StatusAccepted {
		t.Fatalf("first server build status=%d", w.Code)
	}
	<-first.started
	if w := opsDo(t, s2, http.MethodPost, "/v1/sessions/"+sess2.id+"/index/build", ""); w.Code != http.StatusAccepted {
		t.Fatalf("second server build status=%d, want 202", w.Code)
	}
	if _, ok := opsEvents(sess2, ch2, "index_done"); !ok {
		t.Fatal("second server build did not finish")
	}
	release.Do(func() { close(first.gate) })
	if _, ok := opsEvents(sess1, ch1, "index_done"); !ok {
		t.Fatal("first server build did not finish")
	}
}

func TestIndexSlotReleasedBeforeTerminalEvent(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{"done", nil}, {"error", fmt.Errorf("failed")}} {
		t.Run(tc.name, func(t *testing.T) {
			s, sess := opsSession(t)
			sess.appState.ToolSettings.WorkingDir = t.TempDir()
			sess.semanticSvc = &fakeIndexService{err: tc.err}
			ch := sess.Subscribe()
			defer sess.Unsubscribe(ch)
			if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/index/build", ""); w.Code != http.StatusAccepted {
				t.Fatalf("first build status=%d", w.Code)
			}
			want := "index_done"
			if tc.err != nil {
				want = "index_error"
			}
			if _, ok := opsEvents(sess, ch, want); !ok {
				t.Fatalf("missing %s", want)
			}
			if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/index/build", ""); w.Code != http.StatusAccepted {
				t.Fatalf("rebuild after %s status=%d, want 202", want, w.Code)
			}
			if _, ok := opsEvents(sess, ch, want); !ok {
				t.Fatalf("missing second %s", want)
			}
		})
	}
}

func TestIndexBuildFailureEmitsIndexError(t *testing.T) {
	s, sess := opsSession(t)
	sess.semanticSvc = &fakeIndexService{err: fmt.Errorf("embedder down")}
	ch := sess.Subscribe()
	defer sess.Unsubscribe(ch)
	if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/index/build", ""); w.Code != http.StatusAccepted {
		t.Fatalf("status=%d", w.Code)
	}
	e, ok := opsEvents(sess, ch, "index_error")
	if !ok || !strings.Contains(fmt.Sprint(e.Data["error"]), "embedder down") {
		t.Fatalf("index_error=%v ok=%v", e.Data, ok)
	}
}

func TestClearResetsTodoListAndEmitsEmptyTodoUpdated(t *testing.T) {
	s, sess := opsSession(t)
	list := todo.NewTodoList()
	if err := list.Replace([]todo.TodoItem{{ID: "1", Content: "keep going", Status: "pending", Priority: "high"}}); err != nil {
		t.Fatal(err)
	}
	sess.mu.Lock()
	sess.appState.TodoList = list
	sess.mu.Unlock()
	if w := opsDo(t, s, http.MethodPost, "/v1/sessions/"+sess.id+"/clear", ""); w.Code != http.StatusOK {
		t.Fatalf("clear status=%d", w.Code)
	}
	sess.mu.Lock()
	got, ok := sess.appState.TodoList.(*todo.TodoList)
	sess.mu.Unlock()
	if !ok || len(got.All()) != 0 {
		t.Fatalf("todo list not reset after clear: ok=%v items=%v", ok, got)
	}
	var found bool
	for _, e := range sess.Replay("") {
		if e.Type != "todo_updated" {
			continue
		}
		items, isList := e.Data["todos"].([]todo.TodoItem)
		found = isList && len(items) == 0
	}
	if !found {
		t.Fatal("no todo_updated event with empty todos after clear")
	}
}
