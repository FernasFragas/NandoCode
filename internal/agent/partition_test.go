package agent

import (
	"encoding/json"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

// mockTool implements tools.Tool for testing.
type mockTool struct {
	name        string
	safe        bool
	destructive bool
}

func (m *mockTool) Name() string                                    { return m.name }
func (m *mockTool) Description() string                             { return "" }
func (m *mockTool) Aliases() []string                               { return nil }
func (m *mockTool) JSONSchema() map[string]any                      { return map[string]any{} }
func (m *mockTool) UnmarshalInput(raw json.RawMessage) (any, error) { return nil, nil }
func (m *mockTool) IsEnabled(ctx tools.Context) bool                { return true }
func (m *mockTool) IsReadOnly(input any) bool                       { return !m.destructive }
func (m *mockTool) IsConcurrencySafe(input any) bool                { return m.safe }
func (m *mockTool) IsDestructive(input any) bool                    { return m.destructive }
func (m *mockTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}
func (m *mockTool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	return tools.Result{}, nil
}
func (m *mockTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: m.name}
}

func makeSafeTool(name string) *mockTool {
	return &mockTool{name: name, safe: true, destructive: false}
}

func makeUnsafeTool(name string) *mockTool {
	return &mockTool{name: name, safe: false, destructive: true}
}

func TestPartitionEmpty(t *testing.T) {
	batches := Partition(nil)
	if len(batches) != 0 {
		t.Errorf("expected 0 batches for empty input, got %d", len(batches))
	}
}

func TestPartitionAllSafe(t *testing.T) {
	calls := []indexedCall{
		{Index: 0, Tool: makeSafeTool("FileRead-1")},
		{Index: 1, Tool: makeSafeTool("FileRead-2")},
		{Index: 2, Tool: makeSafeTool("FileRead-3")},
	}
	batches := Partition(calls)
	if len(batches) != 1 {
		t.Errorf("expected 1 batch for all-safe input, got %d", len(batches))
	}
	if !batches[0].Safe {
		t.Error("expected batch to be marked as safe")
	}
	if len(batches[0].Calls) != 3 {
		t.Errorf("expected batch with 3 calls, got %d", len(batches[0].Calls))
	}
}

func TestPartitionAllUnsafe(t *testing.T) {
	calls := []indexedCall{
		{Index: 0, Tool: makeUnsafeTool("Bash-1")},
		{Index: 1, Tool: makeUnsafeTool("Bash-2")},
		{Index: 2, Tool: makeUnsafeTool("Bash-3")},
	}
	batches := Partition(calls)
	if len(batches) != 3 {
		t.Errorf("expected 3 batches for all-unsafe input, got %d", len(batches))
	}
	for i, batch := range batches {
		if batch.Safe {
			t.Errorf("batch %d should not be marked as safe", i)
		}
		if len(batch.Calls) != 1 {
			t.Errorf("batch %d should have 1 call, got %d", i, len(batch.Calls))
		}
	}
}

func TestPartitionMixed(t *testing.T) {
	// [safe, safe, unsafe, safe]
	calls := []indexedCall{
		{Index: 0, Tool: makeSafeTool("FileRead-1")},
		{Index: 1, Tool: makeSafeTool("FileRead-2")},
		{Index: 2, Tool: makeUnsafeTool("Bash")},
		{Index: 3, Tool: makeSafeTool("FileRead-3")},
	}
	batches := Partition(calls)
	if len(batches) != 3 {
		t.Errorf("expected 3 batches for [safe, safe, unsafe, safe], got %d", len(batches))
	}

	// First batch: [safe, safe]
	if !batches[0].Safe || len(batches[0].Calls) != 2 {
		t.Errorf("batch 0: expected safe batch with 2 calls, got safe=%v len=%d", batches[0].Safe, len(batches[0].Calls))
	}

	// Second batch: [unsafe]
	if batches[1].Safe || len(batches[1].Calls) != 1 {
		t.Errorf("batch 1: expected unsafe batch with 1 call, got safe=%v len=%d", batches[1].Safe, len(batches[1].Calls))
	}

	// Third batch: [safe]
	if !batches[2].Safe || len(batches[2].Calls) != 1 {
		t.Errorf("batch 2: expected safe batch with 1 call, got safe=%v len=%d", batches[2].Safe, len(batches[2].Calls))
	}
}

func TestPartitionOrderPreserved(t *testing.T) {
	calls := []indexedCall{
		{Index: 0, Tool: makeSafeTool("FileRead-1")},
		{Index: 1, Tool: makeUnsafeTool("Bash")},
		{Index: 2, Tool: makeSafeTool("FileRead-2")},
		{Index: 3, Tool: makeSafeTool("FileRead-3")},
		{Index: 4, Tool: makeUnsafeTool("Bash2")},
	}
	batches := Partition(calls)

	// Collect all indices from batches
	var allIndices []int
	for _, batch := range batches {
		for _, ic := range batch.Calls {
			allIndices = append(allIndices, ic.Index)
		}
	}

	expected := []int{0, 1, 2, 3, 4}
	if len(allIndices) != len(expected) {
		t.Errorf("expected %v, got %v", expected, allIndices)
	}
	for i := range expected {
		if allIndices[i] != expected[i] {
			t.Errorf("index mismatch at position %d: expected %d, got %d", i, expected[i], allIndices[i])
		}
	}
}

func TestPartitionSingleSafe(t *testing.T) {
	calls := []indexedCall{
		{Index: 0, Tool: makeSafeTool("FileRead")},
	}
	batches := Partition(calls)
	if len(batches) != 1 {
		t.Errorf("expected 1 batch, got %d", len(batches))
	}
	if !batches[0].Safe {
		t.Error("expected batch to be safe")
	}
}

func TestPartitionSingleUnsafe(t *testing.T) {
	calls := []indexedCall{
		{Index: 0, Tool: makeUnsafeTool("Bash")},
	}
	batches := Partition(calls)
	if len(batches) != 1 {
		t.Errorf("expected 1 batch, got %d", len(batches))
	}
	if batches[0].Safe {
		t.Error("expected batch to be unsafe")
	}
}

// TestPartitionAlternating tests the worst-case scenario of alternating safe/unsafe.
func TestPartitionAlternating(t *testing.T) {
	calls := []indexedCall{
		{Index: 0, Tool: makeSafeTool("FileRead-1")},
		{Index: 1, Tool: makeUnsafeTool("Bash-1")},
		{Index: 2, Tool: makeSafeTool("FileRead-2")},
		{Index: 3, Tool: makeUnsafeTool("Bash-2")},
		{Index: 4, Tool: makeSafeTool("FileRead-3")},
	}
	batches := Partition(calls)
	if len(batches) != 5 {
		t.Errorf("expected 5 batches for alternating pattern, got %d", len(batches))
	}
}

// BenchmarkPartition benchmarks the partition algorithm with realistic input sizes.
func BenchmarkPartition(b *testing.B) {
	// Create a realistic mix: mostly safe tool calls with occasional unsafe ones
	calls := make([]indexedCall, 100)
	for i := 0; i < 100; i++ {
		if i%10 == 0 {
			calls[i].Tool = makeUnsafeTool("Bash")
		} else {
			calls[i].Tool = makeSafeTool("FileRead")
		}
		calls[i].Index = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Partition(calls)
	}
}

func TestPartitionPropertiesRandom(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	for n := 1; n <= 8; n++ {
		for iter := 0; iter < 500; iter++ {
			in := make([]indexedCall, n)
			for i := 0; i < n; i++ {
				if rng.Intn(2) == 0 {
					in[i] = indexedCall{Index: i, Tool: makeSafeTool("s")}
				} else {
					in[i] = indexedCall{Index: i, Tool: makeUnsafeTool("u")}
				}
			}
			batches := Partition(in)
			assertPartitionInvariants(t, in, batches)
		}
	}
}

func FuzzPartition(f *testing.F) {
	seeds := [][]byte{
		{0},
		{1},
		{1, 0, 1, 0},
		{1, 1, 1, 1, 1},
		{0, 0, 0, 0, 0},
		{1, 0, 0, 1, 0, 1},
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, mask []byte) {
		if len(mask) == 0 {
			return
		}
		if len(mask) > 64 {
			mask = mask[:64]
		}
		in := make([]indexedCall, len(mask))
		for i, b := range mask {
			if b%2 == 0 {
				in[i] = indexedCall{Index: i, Tool: makeSafeTool("s")}
			} else {
				in[i] = indexedCall{Index: i, Tool: makeUnsafeTool("u")}
			}
		}
		batches := Partition(in)
		assertPartitionInvariants(t, in, batches)
	})
}

func assertPartitionInvariants(t *testing.T, input []indexedCall, batches []Batch) {
	t.Helper()
	flat := make([]indexedCall, 0, len(input))
	for _, b := range batches {
		if len(b.Calls) == 0 {
			t.Fatal("empty batch")
		}
		seenSafe := 0
		seenUnsafe := 0
		for _, c := range b.Calls {
			if isSafe(c.Tool, c.Input) {
				seenSafe++
			} else {
				seenUnsafe++
			}
			flat = append(flat, c)
		}
		if seenSafe > 0 && seenUnsafe > 0 {
			t.Fatal("batch contains mixed safe/unsafe calls")
		}
		if b.Safe && seenUnsafe > 0 {
			t.Fatal("safe batch contains unsafe call")
		}
		if !b.Safe && len(b.Calls) != 1 {
			t.Fatal("unsafe batch must be singleton")
		}
	}
	if len(flat) != len(input) {
		t.Fatalf("call count mismatch: input=%d flat=%d", len(input), len(flat))
	}
	flatIdx := make([]int, 0, len(flat))
	for _, c := range flat {
		flatIdx = append(flatIdx, c.Index)
	}
	sorted := append([]int(nil), flatIdx...)
	sort.Ints(sorted)
	for i := range sorted {
		if sorted[i] != i {
			t.Fatalf("missing/duplicate index set: got=%v", sorted)
		}
	}
	// Order must be submission order.
	for i := 1; i < len(flatIdx); i++ {
		if flatIdx[i-1] > flatIdx[i] {
			t.Fatalf("order violated: %v", flatIdx)
		}
	}
}

func BenchmarkPartition1000Calls(b *testing.B) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	calls := make([]indexedCall, 1000)
	for i := 0; i < len(calls); i++ {
		if rng.Intn(10) == 0 {
			calls[i] = indexedCall{Index: i, Tool: makeUnsafeTool("u")}
		} else {
			calls[i] = indexedCall{Index: i, Tool: makeSafeTool("s")}
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Partition(calls)
	}
}
