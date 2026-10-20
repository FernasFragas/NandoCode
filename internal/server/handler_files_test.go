package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func filesFixture(t *testing.T) (*Server, string, string) {
	t.Helper()
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	root := t.TempDir()
	s := testServer(t)
	id := createTestSession(t, s)
	sess, _ := s.registry.get(id)
	sess.mu.Lock()
	sess.appState.ToolSettings.WorkingDir = root
	sess.mu.Unlock()
	return s, id, root
}

func filesGet(t *testing.T, s *Server, id, path string) (int, map[string]any) {
	t.Helper()
	w := routeDo(t, s, http.MethodGet, "/v1/sessions/"+id+"/files?path="+path)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

func TestFilesRejectsTraversalAndSymlinkEscape(t *testing.T) {
	s, id, root := filesFixture(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if code, _ := filesGet(t, s, id, ".."); code != http.StatusForbidden {
		t.Errorf("traversal: %d, want 403", code)
	}
	for _, path := range []string{"escape", "escape/secret.txt"} {
		if code, _ := filesGet(t, s, id, path); code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", path, code)
		}
	}
	if code, body := filesGet(t, s, id, "."); code != http.StatusOK || body["kind"] != "dir" {
		t.Errorf("root: %d %v", code, body)
	}
}

func TestFilesTooLargeAndBinary(t *testing.T) {
	s, id, root := filesFixture(t)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(strings.Repeat("x", 2*1024*1024+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary.dat"), []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, body := filesGet(t, s, id, "big.txt"); code != http.StatusOK || body["kind"] != "file" || body["too_large"] != true || body["size"] != float64(2*1024*1024+1) {
		t.Errorf("large: %d %v", code, body)
	}
	if code, _ := filesGet(t, s, id, "binary.dat"); code != http.StatusUnsupportedMediaType {
		t.Errorf("binary: %d, want 415", code)
	}
}

func TestFilesReturnsHash(t *testing.T) {
	s, id, root := filesFixture(t)
	content := []byte("hello\n")
	if err := os.WriteFile(filepath.Join(root, "hello.go"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	code, body := filesGet(t, s, id, "hello.go")
	hash := sha256.Sum256(content)
	if code != http.StatusOK || body["kind"] != "file" || body["content"] != string(content) || body["sha256"] != hex.EncodeToString(hash[:]) || body["size"] != float64(len(content)) || body["lang"] != "go" {
		t.Errorf("file: %d %v", code, body)
	}
}

func TestTreeRouteRemoved(t *testing.T) {
	s, id, _ := filesFixture(t)
	if w := routeDo(t, s, http.MethodGet, "/v1/sessions/"+id+"/tree"); w.Code != http.StatusNotFound {
		t.Fatalf("/tree status=%d, want 404", w.Code)
	}
}

// Migrated B1-4 tree traversal checks: root listing, excludes, nested path,
// traversal, and symlinks keep their original security expectations.
func TestFilesDirectoryListing(t *testing.T) {
	s, id, root := filesFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "nested.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, body := filesGet(t, s, id, ".")
	if code != http.StatusOK || body["kind"] != "dir" {
		t.Fatalf("root: %d %v", code, body)
	}
	entries := body["entries"].([]any)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.(map[string]any)["name"].(string)] = true
	}
	if !names["sub"] || !names["file.txt"] || names[".git"] || names["nested.txt"] {
		t.Fatalf("root entries: %v", names)
	}
	code, body = filesGet(t, s, id, "sub")
	if code != http.StatusOK || len(body["entries"].([]any)) != 1 || body["entries"].([]any)[0].(map[string]any)["name"] != "nested.txt" {
		t.Fatalf("sub: %d %v", code, body)
	}
}

func TestFilesRejectsSymlinkEscapingWorkspace(t *testing.T) {
	s, id, root := filesFixture(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"escape", "escape/secret.txt"} {
		if code, _ := filesGet(t, s, id, path); code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", path, code)
		}
	}
	if code, body := filesGet(t, s, id, "inside"); code != http.StatusOK || body["kind"] != "dir" {
		t.Errorf("inside: %d %v", code, body)
	}
	if code, body := filesGet(t, s, id, "."); code != http.StatusOK {
		t.Fatalf("root: %d %v", code, body)
	} else {
		for _, entry := range body["entries"].([]any) {
			if entry.(map[string]any)["name"] == "escape" {
				t.Fatal("root listing exposed escaping link")
			}
		}
	}
}

func TestFilesRejectsEveryTraversalVariant(t *testing.T) {
	s, id, root := filesFixture(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"..",
		"../",
		"../secret.txt",
		"sub/../..",
		"sub/../../x",
		"%2e%2e",
		"%2e%2e%2fsecret.txt",
		"..%2fsecret.txt",
		"%2e%2e/%2e%2e/etc/passwd",
		"..%5csecret.txt",
		"/etc/passwd",
		url.QueryEscape(filepath.Join(outside, "secret.txt")),
		url.QueryEscape(outside),
		"sub%2f..%2f..%2f..%2fetc",
		"%00",
		"sub%00/../..",
	}
	if runtime.GOOS != "windows" {
		// On Windows "/" has no drive letter, so it is not absolute and
		// resolves to the workspace root, which is allowed.
		cases = append(cases, "/")
	}
	for _, p := range cases {
		code, body := filesGet(t, s, id, p)
		if code != http.StatusForbidden && code != http.StatusNotFound && code != http.StatusBadRequest {
			t.Errorf("path %q: status %d body %v, want 403/404/400", p, code, body)
		}
		if _, leaked := body["content"]; leaked {
			t.Errorf("path %q leaked content", p)
		}
	}
	// A path that stays inside the workspace still works.
	if code, _ := filesGet(t, s, id, "sub/../sub"); code != http.StatusOK {
		t.Errorf("sub/../sub: %d, want 200", code)
	}
}

func TestFilesRejectsInvalidUTF8(t *testing.T) {
	s, id, root := filesFixture(t)
	if err := os.WriteFile(filepath.Join(root, "latin1.txt"), []byte("caf\xe9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("café\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := filesGet(t, s, id, "latin1.txt"); code != http.StatusUnsupportedMediaType {
		t.Errorf("invalid utf-8: %d, want 415", code)
	}
	if code, body := filesGet(t, s, id, "ok.txt"); code != http.StatusOK || body["content"] != "café\n" {
		t.Errorf("valid utf-8: %d %v", code, body)
	}
}

func TestFilesReadsResolvedPath(t *testing.T) {
	s, id, root := filesFixture(t)
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if code, body := filesGet(t, s, id, "link.txt"); code != http.StatusOK || body["content"] != "real" {
		t.Errorf("in-workspace file symlink: %d %v", code, body)
	}
	if !withinAnyRoot(mustEval(t, root), []string{mustEval(t, root)}) {
		t.Error("withinAnyRoot must accept the root itself")
	}
	if withinAnyRoot(mustEval(t, t.TempDir()), []string{mustEval(t, root)}) {
		t.Error("withinAnyRoot must reject an outside path")
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOpenInsideRootsConfinesReads(t *testing.T) {
	ws, extra, outside := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(extra, "a.txt"), []byte("extra"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, rel, ok := openInsideRoots(ws, []string{extra}, filepath.Join(extra, "a.txt"))
	if !ok {
		t.Fatal("file in an additional root was rejected")
	}
	defer root.Close()
	if got, err := root.ReadFile(rel); err != nil || string(got) != "extra" {
		t.Fatalf("ReadFile(%q)=%q,%v", rel, got, err)
	}
	if _, err := root.ReadFile(filepath.Join("..", filepath.Base(outside), "secret")); err == nil {
		t.Fatal("root allowed a read outside itself")
	}
	if _, _, ok := openInsideRoots(ws, []string{extra}, filepath.Join(outside, "secret")); ok {
		t.Fatal("file outside every root was accepted")
	}
}
