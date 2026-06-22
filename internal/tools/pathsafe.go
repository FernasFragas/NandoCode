package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PathMode controls how missing paths are resolved.
type PathMode int

const (
	// PathRead requires the target to exist.
	PathRead PathMode = iota
	// PathWrite permits a missing final path under an existing parent.
	PathWrite
)

var errPathOutsideRoots = errors.New("path is outside allowed working directories")

// ResolvePath returns a clean absolute path contained in an allowed root.
func ResolvePath(ctx Context, requested string, mode PathMode) (string, error) {
	if strings.TrimSpace(requested) == "" {
		return "", errors.New("path is empty")
	}
	if isSpecialPath(requested) {
		return "", fmt.Errorf("%w: %s", errPathOutsideRoots, requested)
	}

	candidate := requested
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(ctx.WorkingDir, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if isSpecialPath(abs) {
		return "", fmt.Errorf("%w: %s", errPathOutsideRoots, requested)
	}

	resolved, err := resolveExistingOrParent(abs, mode)
	if err != nil {
		return "", err
	}

	roots, err := allowedRoots(ctx)
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		if isWithin(root, resolved) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("%w: %s", errPathOutsideRoots, requested)
}

func allowedRoots(ctx Context) ([]string, error) {
	raw := append([]string{ctx.WorkingDir}, ctx.AdditionalWorkingDirs...)
	roots := make([]string, 0, len(raw))
	for _, root := range raw {
		if strings.TrimSpace(root) == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		if evaluated, err := filepath.EvalSymlinks(abs); err == nil {
			abs = evaluated
		}
		roots = append(roots, filepath.Clean(abs))
	}
	if len(roots) == 0 {
		return nil, errors.New("no allowed working directory configured")
	}
	return roots, nil
}

func resolveExistingOrParent(path string, mode PathMode) (string, error) {
	if evaluated, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(evaluated), nil
	} else if mode == PathRead {
		return "", err
	}

	parent := filepath.Dir(path)
	if evaluated, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(evaluated, filepath.Base(path)), nil
	}
	return "", os.ErrNotExist
}

func isWithin(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if samePath(root, path) {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func isSpecialPath(path string) bool {
	clean := filepath.Clean(path)
	return clean == "/dev/null" ||
		clean == "/dev/random" ||
		clean == "/dev/urandom" ||
		clean == "/dev/zero" ||
		clean == "/dev/stdin" ||
		strings.HasPrefix(clean, "/dev/fd/")
}
