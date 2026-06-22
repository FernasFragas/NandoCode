package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/paths"
)

// ScopeRoot resolves the memory project scope root.
func ScopeRoot(workingDir, gitRoot string) (string, error) {
	if strings.TrimSpace(gitRoot) != "" {
		return filepath.Clean(gitRoot), nil
	}
	if strings.TrimSpace(workingDir) == "" {
		return "", errors.New("working directory is required")
	}

	start := filepath.Clean(workingDir)
	cur := start
	for {
		gitPath := filepath.Join(cur, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return cur, nil
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	return start, nil
}

// DirForScope returns the memory directory path for a scope root.
func DirForScope(scopeRoot string) string {
	return paths.MemoryDir(scopeRoot)
}
