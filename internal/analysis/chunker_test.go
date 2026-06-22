package analysis

import (
	"strings"
	"testing"
)

func TestChunkText_BoundedAndOrdered(t *testing.T) {
	content := strings.Repeat("line content for chunking\n", 400)
	chunks := ChunkText("internal/tui/app.go", content, 120)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	lastEnd := 0
	for i, c := range chunks {
		if c.ChunkIndex != i {
			t.Fatalf("chunk index mismatch: %d vs %d", c.ChunkIndex, i)
		}
		if c.StartByte != lastEnd {
			t.Fatalf("start byte mismatch: got %d want %d", c.StartByte, lastEnd)
		}
		if c.EndByte <= c.StartByte {
			t.Fatalf("invalid chunk bytes: %d..%d", c.StartByte, c.EndByte)
		}
		if c.EstTokens <= 0 {
			t.Fatalf("expected token estimate > 0, got %d", c.EstTokens)
		}
		lastEnd = c.EndByte
	}
}
