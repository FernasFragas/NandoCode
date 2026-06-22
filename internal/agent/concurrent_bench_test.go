package agent

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

// BenchmarkConcurrentFileRead models 5 independent read-like calls where concurrent
// execution should approach one-call latency instead of additive latency.
func BenchmarkConcurrentFileRead(b *testing.B) {
	tool := &slowTool{name: "FileRead", duration: 10 * time.Millisecond}
	batch := Batch{Safe: true, Calls: make([]indexedCall, 0, 5)}
	for i := 0; i < 5; i++ {
		call := llm.ToolCall{}
		call.Function.Name = "FileRead"
		batch.Calls = append(batch.Calls, indexedCall{
			Index:  i,
			ToolID: "tool-" + strconv.Itoa(i),
			Tool:   tool,
			Call:   call,
		})
	}
	executor := NewSpeculativeExecutor(nil, newMockToolContext(), 10, nil)
	events := make(chan Event, 100000)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range events {
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.ExecuteBatches(context.Background(), []Batch{batch}, events)
	}
	b.StopTimer()
	close(events)
	<-done
}

// BenchmarkSerialFileRead is the serial baseline for the same 5 read-like calls.
func BenchmarkSerialFileRead(b *testing.B) {
	tool := &slowTool{name: "FileRead", duration: 10 * time.Millisecond}
	batches := make([]Batch, 0, 5)
	for i := 0; i < 5; i++ {
		call := llm.ToolCall{}
		call.Function.Name = "FileRead"
		batches = append(batches, Batch{
			Safe: false,
			Calls: []indexedCall{
				{Index: i, ToolID: "tool-" + strconv.Itoa(i), Tool: tool, Call: call},
			},
		})
	}
	executor := NewSpeculativeExecutor(nil, newMockToolContext(), 10, nil)
	events := make(chan Event, 100000)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range events {
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.ExecuteBatches(context.Background(), batches, events)
	}
	b.StopTimer()
	close(events)
	<-done
}
