package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
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

func Snapshot(root string) (Manifest, error) {
	files := make(map[string]FileInfo)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		files[rel] = FileInfo{
			Path:       rel,
			Size:       info.Size(),
			Digest:     hex.EncodeToString(sum[:]),
			Executable: info.Mode().Perm()&0o111 != 0,
			Binary:     isBinary(content),
			Mode:       info.Mode(),
			Content:    append([]byte(nil), content...),
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
