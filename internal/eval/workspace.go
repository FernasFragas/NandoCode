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

// copyTree copies the fixture repo into dst. Both sides are accessed through
// os.Root and symlinks are rejected, so the copy can neither read nor write
// outside its directories.
func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return err
	}
	srcRoot, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer srcRoot.Close()
	dstRoot, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer dstRoot.Close()

	return fs.WalkDir(srcRoot.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks inside repo are not supported: %s", filepath.Join(src, filepath.FromSlash(rel)))
		}
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		name := filepath.FromSlash(rel)
		if d.IsDir() {
			return dstRoot.Mkdir(name, info.Mode().Perm())
		}
		return copyFile(srcRoot, dstRoot, name, info.Mode().Perm())
	})
}

func copyFile(srcRoot, dstRoot *os.Root, name string, perm os.FileMode) error {
	in, err := srcRoot.Open(name)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := dstRoot.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
