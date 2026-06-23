package fileindex

import (
	"context"
	"strings"
	"sync"

	"github.com/FernasFragas/Nandocode/internal/tools/dirwalk"
)

const (
	defaultMaxEntries = 50000
)

// Entry represents one indexed path.
type Entry struct {
	Rel       string
	Base      string
	RelLower  string
	BaseLower string
	CharMask  [4]uint64 // ASCII presence bitmap for quick query prefiltering.
	IsDir     bool
}

// Index stores an immutable snapshot of workspace entries.
type Index struct {
	root       string
	entries    []Entry
	byPath     map[string]int
	truncated  bool
	maxEntries int
	mu         sync.RWMutex
}

// New creates an empty index for root.
func New(root string) *Index {
	return &Index{
		root:       root,
		entries:    nil,
		byPath:     map[string]int{},
		truncated:  false,
		maxEntries: defaultMaxEntries,
	}
}

// Refresh rebuilds the index snapshot.
func (i *Index) Refresh(ctx context.Context) error {
	walked, stats, err := dirwalk.Walk(ctx, i.root, dirwalk.Options{
		MaxFiles: i.maxEntries,
	})
	if err != nil {
		return err
	}

	entries := make([]Entry, 0, len(walked))
	byPath := make(map[string]int, len(walked))
	for _, e := range walked {
		if e.Skipped || e.RelPath == "" {
			continue
		}
		idx := len(entries)
		entries = append(entries, Entry{
			Rel:       e.RelPath,
			Base:      baseName(e.RelPath),
			RelLower:  strings.ToLower(e.RelPath),
			BaseLower: strings.ToLower(baseName(e.RelPath)),
			CharMask:  asciiMask(strings.ToLower(e.RelPath)),
			IsDir:     e.IsDir,
		})
		byPath[e.RelPath] = idx
	}

	i.mu.Lock()
	i.entries = entries
	i.byPath = byPath
	i.truncated = stats.Truncated
	i.mu.Unlock()
	return nil
}

// Snapshot returns a copy of the current entries.
func (i *Index) Snapshot() []Entry {
	i.mu.RLock()
	defer i.mu.RUnlock()
	cp := make([]Entry, len(i.entries))
	copy(cp, i.entries)
	return cp
}

// Truncated reports whether refresh hit the entry cap.
func (i *Index) Truncated() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.truncated
}

func baseName(rel string) string {
	for idx := len(rel) - 1; idx >= 0; idx-- {
		if rel[idx] == '/' {
			return rel[idx+1:]
		}
	}
	return rel
}

func asciiMask(s string) [4]uint64 {
	var out [4]uint64
	for i := 0; i < len(s); i++ {
		b := s[i]
		out[b/64] |= 1 << (b % 64)
	}
	return out
}
