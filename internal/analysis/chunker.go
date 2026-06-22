package analysis

import (
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// FileChunk represents a token-bounded slice of a file for map/reduce analysis.
type FileChunk struct {
	Path       string `json:"path"`
	ChunkIndex int    `json:"chunk_index"`
	StartByte  int    `json:"start_byte"`
	EndByte    int    `json:"end_byte"`
	Text       string `json:"text"`
	EstTokens  int    `json:"est_tokens"`
}

// ChunkText splits text into chunks targeting maxTokens estimated tokens per chunk.
func ChunkText(path, content string, maxTokens int) []FileChunk {
	if maxTokens <= 0 {
		maxTokens = 1200
	}
	const minChunkBytes = 512
	maxBytes := maxTokens * 4
	if maxBytes < minChunkBytes {
		maxBytes = minChunkBytes
	}
	path = filepath.ToSlash(strings.TrimSpace(path))
	if content == "" {
		return nil
	}
	raw := []byte(content)
	out := make([]FileChunk, 0, len(raw)/maxBytes+1)
	start := 0
	chunkIndex := 0
	for start < len(raw) {
		end := start + maxBytes
		if end > len(raw) {
			end = len(raw)
		} else {
			end = snapToBoundary(raw, start, end)
		}
		segment := string(raw[start:end])
		out = append(out, FileChunk{
			Path:       path,
			ChunkIndex: chunkIndex,
			StartByte:  start,
			EndByte:    end,
			Text:       segment,
			EstTokens:  estimateTokens(segment),
		})
		start = end
		chunkIndex++
	}
	return out
}

func snapToBoundary(raw []byte, start, end int) int {
	if end >= len(raw) {
		return len(raw)
	}
	// Prefer the last newline in the current window.
	for i := end; i > start+64; i-- {
		if raw[i-1] == '\n' {
			return i
		}
	}
	// Keep UTF-8 rune integrity when no newline boundary exists.
	for end > start && !utf8.Valid(raw[start:end]) {
		end--
	}
	if end <= start {
		return start + 1
	}
	return end
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	// Simple conservative estimate for budgeting.
	return (len(s) + 3) / 4
}
