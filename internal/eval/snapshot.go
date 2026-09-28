package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"
)

type Manifest struct {
	Root  string
	Files map[string]FileInfo
}

type FileInfo struct {
	Path       string
	Size       int64
	Digest     string
	Executable bool
	Binary     bool
	Mode       os.FileMode
	Content    []byte
}

// Snapshot records every file under root. It reads through an os.Root so a
// symlink created by the agent cannot make the snapshot read outside the
// workspace.
func Snapshot(root string) (Manifest, error) {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return Manifest{}, err
	}
	defer rootFS.Close()
	fsys := rootFS.FS()

	files := make(map[string]FileInfo)
	err = fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		content, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		files[rel] = FileInfo{
			Path:       rel,
			Size:       info.Size(),
			Digest:     hex.EncodeToString(sum[:]),
			Executable: info.Mode().Perm()&0o111 != 0,
			Binary:     isBinary(content),
			Mode:       info.Mode(),
			Content:    content,
		}
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{Root: root, Files: files}, nil
}

func isBinary(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	if !utf8.Valid(content) {
		return true
	}
	for _, b := range content {
		if b == 0 {
			return true
		}
	}
	return false
}

func normalizeText(in []byte) string {
	return strings.ReplaceAll(string(in), "\r\n", "\n")
}
