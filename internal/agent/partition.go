package agent

import (
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

// Batch is a group of tool calls that can execute concurrently.
// A batch with one item is a singleton (serial execution).
type Batch struct {
	Calls []indexedCall
	Safe  bool
}

type indexedCall struct {
	Index  int
	ToolID string
	Call   llm.ToolCall
	Tool   tools.Tool
	Input  any
}

// isSafe returns true if a tool call is safe for concurrent execution.
// A tool must be both concurrency-safe and non-destructive to run in a batch.
func isSafe(tool tools.Tool, input any) bool {
	return tool.IsConcurrencySafe(input) && !tool.IsDestructive(input)
}

// Partition applies the greedy left-to-right algorithm to partition tool calls
// into batches of concurrent-safe calls and singleton batches of unsafe calls.
//
// Algorithm:
//  1. Walk calls left-to-right.
//  2. If current call is safe and accumulator is empty or all-safe: add to accumulator.
//  3. If current call is unsafe: emit accumulator as a batch (if non-empty);
//     emit current call as a singleton batch; reset accumulator.
//  4. After all calls: emit remaining accumulator as final batch.
//
// Property guarantees:
//   - All input calls appear exactly once in the output batches.
//   - Input call order is preserved across batches and within batches.
//   - No batch contains a mix of safe and unsafe calls.
//   - A safe batch always contains only safe calls.
//   - A singleton batch contains exactly one call (safe or unsafe).
//   - Safe calls adjacent to each other are grouped in the same batch.
//   - A single unsafe call between two safe groups splits them into three batches.
func Partition(indexed []indexedCall) []Batch {
	if len(indexed) == 0 {
		return nil
	}

	var batches []Batch
	var accumulator []indexedCall

	for _, ic := range indexed {
		if isSafe(ic.Tool, ic.Input) {
			accumulator = append(accumulator, ic)
		} else {
			if len(accumulator) > 0 {
				batches = append(batches, Batch{
					Calls: accumulator,
					Safe:  true,
				})
				accumulator = nil
			}
			batches = append(batches, Batch{
				Calls: []indexedCall{ic},
				Safe:  false,
			})
		}
	}

	if len(accumulator) > 0 {
		batches = append(batches, Batch{
			Calls: accumulator,
			Safe:  true,
		})
	}

	return batches
}
