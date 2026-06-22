package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type store struct {
	dir string
}

func newStore(dir string) *store {
	return &store{dir: dir}
}

func (s *store) ensure(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	pending := filepath.Join(s.dir, "pending")
	return os.MkdirAll(pending, 0o700)
}

func (s *store) indexPath() string {
	return filepath.Join(s.dir, "MEMORY.md")
}

func (s *store) loadIndex(cfg Config) (Index, error) {
	return LoadIndex(s.indexPath(), cfg.IndexMaxLines, cfg.IndexMaxBytes)
}

func (s *store) readActive(filename string) (string, error) {
	path, err := s.activePath(filename)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *store) readSelected(entries []Entry, now time.Time) ([]LoadedEntry, error) {
	out := make([]LoadedEntry, 0, len(entries))
	for _, e := range entries {
		body, err := os.ReadFile(e.Path)
		if err != nil {
			return nil, err
		}
		out = append(out, LoadedEntry{
			Entry:            e,
			Content:          string(body),
			StalenessWarning: StalenessWarning(now, e.UpdatedAt),
		})
	}
	return out, nil
}

func (s *store) writePending(ctx context.Context, draft Draft) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	name := strings.TrimSpace(draft.Filename)
	if name == "" {
		name = time.Now().UTC().Format("20060102T150405Z") + "-memory.md"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		name += ".md"
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid pending draft filename: %q", name)
	}
	pendingDir := filepath.Join(s.dir, "pending")
	target := filepath.Join(pendingDir, name)
	tmp := target + ".tmp"
	if err := os.MkdirAll(pendingDir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(tmp, []byte(draft.Content), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", err
	}
	return target, nil
}

func (s *store) activePath(filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" {
		return "", fmt.Errorf("empty filename")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid filename: %q", name)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		return "", fmt.Errorf("memory filename must end with .md: %q", name)
	}
	return filepath.Join(s.dir, name), nil
}
