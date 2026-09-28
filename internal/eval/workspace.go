package eval

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type Workspace struct {
	Root     string
	Path     string
	RepoPath string
}

func PrepareWorkspace(root string, fixture Fixture) (Workspace, error) {
	runRoot, err := os.MkdirTemp(root, "nandocodego-eval-")
	if err != nil {
		return Workspace{}, err
	}
	workspaceDir := filepath.Join(runRoot, "workspace")
	if err := copyTree(fixture.RepoDir, workspaceDir); err != nil {
		_ = os.RemoveAll(runRoot)
		return Workspace{}, err
	}
	return Workspace{Root: runRoot, Path: workspaceDir, RepoPath: fixture.RepoDir}, nil
}

func CleanupWorkspace(ws Workspace, keep bool) {
	if keep || ws.Root == "" {
		return
	}
	_ = os.RemoveAll(ws.Root)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := dst
		if rel != "." {
			target = filepath.Join(dst, rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks inside repo are not supported: %s", path)
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
