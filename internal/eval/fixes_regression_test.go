package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
)

func TestEvaluateChecksDeletedGoldenFileIsTaskFailure(t *testing.T) {
	expectedDir, workspaceDir := t.TempDir(), t.TempDir()
	mustWriteFile(t, filepath.Join(expectedDir, "a.go"), "package a\n")

	_, checks, hard, _, _, err := EvaluateChecks(scoringContext{
		Fixture: Fixture{
			ExpectedDir: expectedDir,
			Config:      ScoringConfig{Golden: GoldenConfig{Files: []string{"a.go"}}},
		},
		Workspace: Workspace{Path: workspaceDir},
		Changes:   []FileChange{{Path: "a.go", Change: ChangeDeleted}},
		Terminal:  "completed",
	})
	if err != nil {
		t.Fatalf("missing golden file in workspace must not be a harness error: %v", err)
	}
	if !containsString(hard, "golden file mismatch") {
		t.Fatalf("hard failures = %v, want golden file mismatch", hard)
	}
	if c := findCheck(checks, "expected_files"); c.Status != "failed" {
		t.Fatalf("expected_files check = %+v, want failed", c)
	}
}

func TestEvaluateChecksReportsToolUsageCheck(t *testing.T) {
	cfg := ScoringConfig{
		Execution: ExecutionConfig{MaxToolCalls: 2},
		Checks:    ChecksConfig{RequiredTools: []string{"Bash"}, ForbiddenTools: []string{"WebFetch"}},
	}
	run := func(tools map[string]int, limit bool) (CheckResult, []string) {
		_, checks, hard, _, _, err := EvaluateChecks(scoringContext{
			Fixture:   Fixture{Config: cfg},
			Workspace: Workspace{Path: t.TempDir()},
			ToolNames: tools,
			ToolLimit: limit,
			Terminal:  "completed",
		})
		if err != nil {
			t.Fatal(err)
		}
		return findCheck(checks, "tool_usage"), hard
	}

	if c, hard := run(map[string]int{"Bash": 1}, false); c.Status != "passed" || len(hard) != 0 {
		t.Fatalf("clean run: check=%+v hard=%v", c, hard)
	}
	c, hard := run(map[string]int{"WebFetch": 1}, false)
	if c.Status != "failed" || !strings.Contains(c.Details, "required tools not called: Bash") || !strings.Contains(c.Details, "forbidden tools called: WebFetch") {
		t.Fatalf("tool misuse: check=%+v", c)
	}
	if !containsString(hard, "tool usage check failed") {
		t.Fatalf("tool misuse: hard=%v", hard)
	}
	c, hard = run(map[string]int{"Bash": 3}, true)
	if c.Status != "failed" || !strings.Contains(c.Details, "tool call limit 2 exceeded") || !containsString(hard, "tool call limit exceeded") {
		t.Fatalf("limit: check=%+v hard=%v", c, hard)
	}
}

func TestPermissionObserverCountsDeniedCallsAndEnforcesLimit(t *testing.T) {
	limitCalls := 0
	m := newRunMetrics(2, func() { limitCalls++ })
	observe := m.permissionObserver()
	ctx := context.Background()
	observe(ctx, permissions.Request{ToolName: "Bash"}, permissions.Result{Decision: permissions.DecisionDeny})
	observe(ctx, permissions.Request{ToolName: "FileRead"}, permissions.Result{Decision: permissions.DecisionAllow})
	if limitCalls != 0 {
		t.Fatal("limit fired before being exceeded")
	}
	observe(ctx, permissions.Request{ToolName: "Bash"}, permissions.Result{Decision: permissions.DecisionAllow})
	observe(ctx, permissions.Request{ToolName: "Bash"}, permissions.Result{Decision: permissions.DecisionAllow})

	snap := m.snapshot()
	if snap.ToolCallsByName["Bash"] != 3 || snap.ToolCallsByName["FileRead"] != 1 {
		t.Fatalf("tool counts = %v, want denied calls included", snap.ToolCallsByName)
	}
	if !snap.ToolLimitExceeded || limitCalls != 1 {
		t.Fatalf("limit exceeded=%v fired=%d, want true/1", snap.ToolLimitExceeded, limitCalls)
	}
}

func TestRecordedClientVerify(t *testing.T) {
	recording := Recording{
		Version: 1,
		Model:   "recorded/x",
		Strict:  true,
		Turns: []RecordedTurn{
			{Events: []llm.StreamEvent{{Done: true}}},
			{Events: []llm.StreamEvent{{Done: true}}},
		},
	}

	c := NewRecordedClient(recording)
	if _, err := c.Chat(context.Background(), &llm.ChatRequest{Model: "other"}); err == nil {
		t.Fatal("expected model mismatch")
	}
	if err := c.Verify(false); err == nil || !strings.Contains(err.Error(), "model mismatch") {
		t.Fatalf("Verify() = %v, want retained model mismatch", err)
	}

	c = NewRecordedClient(recording)
	if _, err := c.Chat(context.Background(), &llm.ChatRequest{Model: "recorded/x"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(true); err == nil || !strings.Contains(err.Error(), "1 unused turn") {
		t.Fatalf("Verify(completed) = %v, want unused turn error", err)
	}
	if err := c.Verify(false); err != nil {
		t.Fatalf("Verify(interrupted) = %v, unused turns expected after an interrupted run", err)
	}
}

func TestRunReportsUnusedRecordedTurnAsError(t *testing.T) {
	root := t.TempDir()
	writeRunnableFixture(t, root, "unused-turn")
	recPath := filepath.Join(root, "unused-turn", "recordings", "default.json")
	extraTurn := `,
    {
      "match": {"latest_user_contains":"Update file and run tests."},
      "events": [{"message":{"role":"assistant","content":"never requested"}},{"done":true,"done_reason":"stop"}]
    }
  ]
}`
	rewriteFile(t, recPath, func(s string) string {
		i := strings.LastIndex(s, "\n  ]\n}")
		return s[:i] + extraTurn
	})

	result := runSingleFixture(t, root)
	if result.Status != StatusError || !strings.Contains(result.FailureReason, "unused turn") {
		t.Fatalf("status=%s reason=%q, want error about unused turn", result.Status, result.FailureReason)
	}
}

func TestRunEnforcesMaxToolCalls(t *testing.T) {
	root := t.TempDir()
	writeRunnableFixture(t, root, "tool-limit")
	rewriteFile(t, filepath.Join(root, "tool-limit", "scoring.yaml"), func(s string) string {
		return strings.Replace(s, "max_tool_calls: 10", "max_tool_calls: 1", 1)
	})

	result := runSingleFixture(t, root)
	if result.Status != StatusFailed {
		t.Fatalf("status=%s reason=%q, want failed", result.Status, result.FailureReason)
	}
	if c := findCheck(result.Checks, "tool_usage"); c.Status != "failed" || !strings.Contains(c.Details, "tool call limit 1 exceeded") {
		t.Fatalf("tool_usage check = %+v", c)
	}
}

func TestRunOptionsValidate(t *testing.T) {
	cases := []struct {
		opts    RunOptions
		wantErr string
	}{
		{RunOptions{Provider: ProviderRecorded}, ""},
		{RunOptions{Provider: ProviderLive, Model: "m", OllamaURL: "http://localhost:11434"}, ""},
		{RunOptions{Provider: ProviderRecorded, Model: "m"}, "--model"},
		{RunOptions{Provider: ProviderRecorded, OllamaURL: "http://localhost:11434"}, "--ollama-url"},
		{RunOptions{Provider: "bogus"}, "unsupported provider"},
	}
	for _, tc := range cases {
		err := tc.opts.Validate()
		if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("Validate(%+v) = %v, want error containing %q", tc.opts, err, tc.wantErr)
		}
	}
}

func TestChatOptionsClientAppliesTemperatureWithoutMutatingRequest(t *testing.T) {
	inner := &captureChatClient{}
	client := withChatOptions(inner, map[string]any{"temperature": 0.0})
	req := &llm.ChatRequest{Model: "m", Options: map[string]any{"num_ctx": 4096}}
	if _, err := client.Chat(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got := inner.last.Options; got["temperature"] != 0.0 || got["num_ctx"] != 4096 {
		t.Fatalf("sent options = %v", got)
	}
	if _, ok := req.Options["temperature"]; ok {
		t.Fatal("caller's request options were mutated")
	}
}

func TestCappedBufferTruncates(t *testing.T) {
	b := &cappedBuffer{limit: 4}
	if n, err := b.Write([]byte("abcdef")); n != 6 || err != nil {
		t.Fatalf("Write = %d, %v; must report full length so the child is not blocked", n, err)
	}
	if got := b.String(); !strings.HasPrefix(got, "abcd\n[output truncated at 4 bytes]") {
		t.Fatalf("String() = %q", got)
	}
}

type captureChatClient struct {
	llm.Client
	last *llm.ChatRequest
}

func (c *captureChatClient) Chat(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.last = req
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}

func runSingleFixture(t *testing.T, root string) FixtureResult {
	t.Helper()
	fixtures, err := LoadAndValidate(root)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), fixtures, RunOptions{
		Provider:  ProviderRecorded,
		OutputDir: filepath.Join(root, "artifacts"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return report.Results[0]
}

func rewriteFile(t *testing.T, path string, edit func(string) string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, path, edit(string(data)))
}

func findCheck(checks []CheckResult, name string) CheckResult {
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	return CheckResult{}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
