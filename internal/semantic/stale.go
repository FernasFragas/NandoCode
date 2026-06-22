package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func HashText(s string) string {
	return HashBytes([]byte(s))
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func CanonicalRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}

func WorkspaceID(root, model string, dimensions, schemaVersion int) (string, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(canonical)
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(model))
	b.WriteByte('\n')
	b.WriteString(strconvInt(dimensions))
	b.WriteByte('\n')
	b.WriteString(strconvInt(schemaVersion))
	return HashText(b.String())[:24], nil
}

func IsRecordStale(record Record, fullPath string) (bool, error) {
	if strings.TrimSpace(record.ContentHash) == "" {
		return true, nil
	}
	current, err := HashFile(fullPath)
	if err != nil {
		return false, err
	}
	return current != record.ContentHash, nil
}

func strconvInt(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var a [20]byte
	i := len(a)
	for v > 0 {
		i--
		a[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		a[i] = '-'
	}
	return string(a[i:])
}
