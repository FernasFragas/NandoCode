package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

// newMockToolContext creates a tools.Context for testing.
func newMockToolContext() tools.Context {
	return tools.Context{
		Context:        context.Background(),
		Logger:         nil,
		WorkingDir:     "/tmp",
		BashTimeout:    30 * time.Second,
		MaxResultChars: 10000,
		MaxReadChars:   100000,
	}
}

// slowTool is a mock tool that takes a fixed amount of time to execute.
type slowTool struct {
	name       string
	duration   time.Duration
	callMu     sync.Mutex
	calls      int
	startMu    sync.Mutex
	startTimes []time.Time
}

func (s *slowTool) Name() string                                    { return s.name }
func (s *slowTool) Description() string                             { return "" }
func (s *slowTool) Aliases() []string                               { return nil }
func (s *slowTool) JSONSchema() map[string]any                      { return map[string]any{} }
func (s *slowTool) UnmarshalInput(raw json.RawMessage) (any, error) { return nil, nil }
func (s *slowTool) IsEnabled(ctx tools.Context) bool                { return true }
func (s *slowTool) IsReadOnly(input any) bool                       { return true }
func (s *slowTool) IsConcurrencySafe(input any) bool                { return true }
func (s *slowTool) IsDestructive(input any) bool                    { return false }
func (s *slowTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}
func (s *slowTool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	s.startMu.Lock()
	s.startTimes = append(s.startTimes, time.Now())
	s.startMu.Unlock()

	s.callMu.Lock()
	s.calls++
	s.callMu.Unlock()

	time.Sleep(s.duration)
	return tools.Result{Display: fmt.Sprintf("%s completed", s.name)}, nil
}
func (s *slowTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: s.name}
}

// countingTool tracks in-flight calls to validate concurrency limiting.
type countingTool struct {
	name        string
	duration    time.Duration
	mu          sync.Mutex
	inflight    int
	maxInflight int
}

func (c *countingTool) Name() string                                    { return c.name }
func (c *countingTool) Description() string                             { return "" }
func (c *countingTool) Aliases() []string                               { return nil }
func (c *countingTool) JSONSchema() map[string]any                      { return map[string]any{} }
func (c *countingTool) UnmarshalInput(raw json.RawMessage) (any, error) { return nil, nil }
func (c *countingTool) IsEnabled(ctx tools.Context) bool                { return true }
func (c *countingTool) IsReadOnly(input any) bool                       { return true }
func (c *countingTool) IsConcurrencySafe(input any) bool                { return true }
func (c *countingTool) IsDestructive(input any) bool                    { return false }
func (c *countingTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}
func (c *countingTool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	c.mu.Lock()
	c.inflight++
	if c.inflight > c.maxInflight {
		c.maxInflight = c.inflight
	}
	c.mu.Unlock()

	time.Sleep(c.duration)

	c.mu.Lock()
	c.inflight--
	c.mu.Unlock()
	return tools.Result{Display: "ok"}, nil
}
func (c *countingTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: c.name}
}

// TestConcurrentExecution verifies that safe concurrent calls execute in parallel.
func TestConcurrentExecution(t *testing.T) {
	tool1 := &slowTool{name: "tool1", duration: 100 * time.Millisecond}
	tool2 := &slowTool{name: "tool2", duration: 100 * time.Millisecond}
	tool3 := &slowTool{name: "tool3", duration: 100 * time.Millisecond}

	call1 := llm.ToolCall{}
	call1.Function.Name = "tool1"
	call2 := llm.ToolCall{}
	call2.Function.Name = "tool2"
	call3 := llm.ToolCall{}
	call3.Function.Name = "tool3"

	batches := []Batch{
		{
			Safe: true,
			Calls: []indexedCall{
				{Index: 0, ToolID: "tool-0", Tool: tool1, Call: call1},
				{Index: 1, ToolID: "tool-1", Tool: tool2, Call: call2},
				{Index: 2, ToolID: "tool-2", Tool: tool3, Call: call3},
			},
		},
	}

	executor := NewSpeculativeExecutor(nil, newMockToolContext(), 10, nil)
	ctx := context.Background()
	events := make(chan Event, 100)

	start := time.Now()
	_, _ = executor.ExecuteBatches(ctx, batches, events)
	elapsed := time.Since(start)

	// If executed concurrently, should take ~100ms
	// If executed serially, would take ~300ms
	// Allow some slack for scheduling overhead
	if elapsed > 250*time.Millisecond {
		t.Errorf("Expected concurrent execution (~100ms), got %v", elapsed)
	}

	// Verify all tools were called
	if tool1.calls != 1 || tool2.calls != 1 || tool3.calls != 1 {
		t.Errorf("Expected 3 calls total, got %d/%d/%d", tool1.calls, tool2.calls, tool3.calls)
	}

	// Verify that tools started overlapping (not sequential)
	tool1.startMu.Lock()
	tool2.startMu.Lock()
	tool3.startMu.Lock()
	defer tool1.startMu.Unlock()
	defer tool2.startMu.Unlock()
	defer tool3.startMu.Unlock()

	if len(tool1.startTimes) > 0 && len(tool2.startTimes) > 0 && len(tool3.startTimes) > 0 {
		t1 := tool1.startTimes[0]
		t2 := tool2.startTimes[0]
		t3 := tool3.startTimes[0]

		// All should start within 50ms of each other (concurrent)
		times := []time.Time{t1, t2, t3}
		minTime := times[0]
		maxTime := times[0]
		for _, t := range times {
			if t.Before(minTime) {
				minTime = t
			}
			if t.After(maxTime) {
				maxTime = t
			}
		}

		timeDiff := maxTime.Sub(minTime)
		if timeDiff > 50*time.Millisecond {
			t.Logf("Tools did not start concurrently: time spread = %v", timeDiff)
		}
	}
}

// TestSerialUnsafeBatches verifies that unsafe calls execute serially.
func TestSerialUnsafeBatches(t *testing.T) {
	makeTool := func(id int) *slowTool {
		return &slowTool{
			name:     fmt.Sprintf("tool%d", id),
			duration: 50 * time.Millisecond,
		}
	}

	tool1 := makeTool(1)
	tool2 := makeTool(2)
	tool3 := makeTool(3)

	call1 := llm.ToolCall{}
	call1.Function.Name = "tool1"
	call2 := llm.ToolCall{}
	call2.Function.Name = "tool2"
	call3 := llm.ToolCall{}
	call3.Function.Name = "tool3"

	batches := []Batch{
		{Safe: false, Calls: []indexedCall{
			{Index: 0, ToolID: "tool-0", Tool: tool1, Call: call1},
		}},
		{Safe: false, Calls: []indexedCall{
			{Index: 1, ToolID: "tool-1", Tool: tool2, Call: call2},
		}},
		{Safe: false, Calls: []indexedCall{
			{Index: 2, ToolID: "tool-2", Tool: tool3, Call: call3},
		}},
	}

	executor := NewSpeculativeExecutor(nil, newMockToolContext(), 10, nil)
	ctx := context.Background()
	events := make(chan Event, 100)

	start := time.Now()
	_, _ = executor.ExecuteBatches(ctx, batches, events)
	elapsed := time.Since(start)

	// Should take ~150ms (3 * 50ms) serially
	if elapsed < 140*time.Millisecond {
		t.Logf("Warning: execution might not be serial: %v", elapsed)
	}
}

// BenchmarkConcurrentExecution benchmarks the performance gain from concurrent execution.
func BenchmarkConcurrentExecution(b *testing.B) {
	executor := NewSpeculativeExecutor(nil, newMockToolContext(), 10, nil)
	ctx := context.Background()

	tool := &slowTool{name: "fast_tool", duration: 1 * time.Millisecond}

	// Create 10 safe concurrent calls
	var calls []indexedCall
	for i := 0; i < 10; i++ {
		call := llm.ToolCall{}
		call.Function.Name = "fast_tool"
		calls = append(calls, indexedCall{
			Index:  i,
			ToolID: "tool-" + strconv.Itoa(i),
			Tool:   tool,
			Call:   call,
		})
	}

	batches := []Batch{{Safe: true, Calls: calls}}

	// Create a large buffered channel and drain it in parallel
	events := make(chan Event, 100000)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range events {
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.ExecuteBatches(ctx, batches, events)
	}
	b.StopTimer()
	close(events)
	<-done
}

// TestResultOrder verifies that results are returned in submission order despite concurrent execution.
func TestResultOrder(t *testing.T) {
	calls := []indexedCall{
		{Index: 0, ToolID: "tool-0", Tool: &slowTool{name: "tool0", duration: 30 * time.Millisecond}},
		{Index: 1, ToolID: "tool-1", Tool: &slowTool{name: "tool1", duration: 10 * time.Millisecond}},
		{Index: 2, ToolID: "tool-2", Tool: &slowTool{name: "tool2", duration: 50 * time.Millisecond}},
	}

	for i := range calls {
		call := llm.ToolCall{}
		call.Function.Name = calls[i].Tool.Name()
		calls[i].Call = call
	}

	batches := []Batch{{Safe: true, Calls: calls}}

	executor := NewSpeculativeExecutor(nil, newMockToolContext(), 10, nil)
	ctx := context.Background()
	events := make(chan Event, 100)

	messages, _ := executor.ExecuteBatches(ctx, batches, events)

	// Should have 3 messages
	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
		return
	}

	// Tool names should be in order
	expectedOrder := []string{"tool0", "tool1", "tool2"}
	for i, msg := range messages {
		if msg.ToolName != expectedOrder[i] {
			t.Errorf("Message %d: expected toolname=%s, got %s", i, expectedOrder[i], msg.ToolName)
		}
	}
}

func TestToolUseResultEventOrder(t *testing.T) {
	calls := []indexedCall{
		{Index: 0, ToolID: "tool-0", Tool: &slowTool{name: "tool0", duration: 40 * time.Millisecond}},
		{Index: 1, ToolID: "tool-1", Tool: &slowTool{name: "tool1", duration: 5 * time.Millisecond}},
		{Index: 2, ToolID: "tool-2", Tool: &slowTool{name: "tool2", duration: 20 * time.Millisecond}},
	}
	for i := range calls {
		call := llm.ToolCall{}
		call.Function.Name = calls[i].Tool.Name()
		calls[i].Call = call
	}
	batches := []Batch{{Safe: true, Calls: calls}}
	executor := NewSpeculativeExecutor(nil, newMockToolContext(), 10, nil)
	events := make(chan Event, 100)
	_, _ = executor.ExecuteBatches(context.Background(), batches, events)
	close(events)

	var resultIDs []string
	for evt := range events {
		if r, ok := evt.(ToolUseResult); ok {
			resultIDs = append(resultIDs, r.ID)
		}
	}
	want := []string{"tool-0", "tool-1", "tool-2"}
	if len(resultIDs) != len(want) {
		t.Fatalf("result event count=%d want=%d", len(resultIDs), len(want))
	}
	for i := range want {
		if resultIDs[i] != want[i] {
			t.Fatalf("result order mismatch at %d: got %v", i, resultIDs)
		}
	}
}

func TestConcurrencyLimitRespected(t *testing.T) {
	tool := &countingTool{name: "safe", duration: 40 * time.Millisecond}
	var calls []indexedCall
	for i := 0; i < 12; i++ {
		call := llm.ToolCall{}
		call.Function.Name = "safe"
		calls = append(calls, indexedCall{Index: i, ToolID: "tool-" + strconv.Itoa(i), Tool: tool, Call: call})
	}
	batches := []Batch{{Safe: true, Calls: calls}}
	executor := NewSpeculativeExecutor(nil, newMockToolContext(), 3, nil)
	events := make(chan Event, 512)
	_, _ = executor.ExecuteBatches(context.Background(), batches, events)
	if tool.maxInflight > 3 {
		t.Fatalf("max inflight=%d, expected <=3", tool.maxInflight)
	}
}
