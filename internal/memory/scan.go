package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Scan loads active top-level memory entries from a directory.
func Scan(ctx context.Context, dir string) (ScanResult, error) {
	start := time.Now()
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ScanResult{Entries: []Entry{}, Duration: time.Since(start)}, nil
		}
		return ScanResult{}, err
	}

	res := ScanResult{
		Entries:  make([]Entry, 0, len(ents)),
		Warnings: []string{},
	}
	for _, de := range ents {
		select {
		case <-ctx.Done():
			return ScanResult{}, ctx.Err()
		default:
		}
		name := de.Name()
		if de.IsDir() {
			continue
		}
		if name == "MEMORY.md" || !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		st, err := os.Stat(path)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: stat failed: %v", name, err))
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: open failed: %v", name, err))
			continue
		}
		entry, parseErr := ParseFrontmatter(name, f, st.ModTime(), st.Size())
		f.Close()
		if parseErr != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", name, parseErr))
			continue
		}
		entry.Path = path
		res.Entries = append(res.Entries, entry)
		res.FileCount++
	}
	sort.Slice(res.Entries, func(i, j int) bool {
		return res.Entries[i].Filename < res.Entries[j].Filename
	})
	res.Duration = time.Since(start)
	return res, nil
}
