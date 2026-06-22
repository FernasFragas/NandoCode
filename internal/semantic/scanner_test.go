package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestScanWorkspaceExtractsRecordsAndLineRanges(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)

	result, err := ScanWorkspace(context.Background(), ScanOptions{
		Root:          root,
		MaxFileBytes:  256,
		MaxChunkBytes: 64,
		ChunkOverlap:  1,
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if result.FilesIndexed == 0 {
		t.Fatal("expected indexed files")
	}

	var foundLogin bool
	var foundHelper bool
	for _, r := range result.Records {
		if r.Path != "main.go" || r.Kind != RecordKindSymbol {
			continue
		}
		if r.Name == "(*Service).Login" {
			foundLogin = true
			if r.StartLine != 5 || r.EndLine != 7 {
				t.Fatalf("unexpected Login lines: %d-%d", r.StartLine, r.EndLine)
			}
		}
		if r.Name == "helper" {
			foundHelper = true
			if r.StartLine != 9 || r.EndLine != 11 {
				t.Fatalf("unexpected helper lines: %d-%d", r.StartLine, r.EndLine)
			}
		}
	}
	if !foundLogin {
		t.Fatal("expected Login symbol record")
	}
	if !foundHelper {
		t.Fatal("expected helper symbol record")
	}

	var foundAuthSection bool
	var foundTokenSection bool
	var foundChunk bool
	for _, r := range result.Records {
		if r.Path == "README.md" && r.Kind == RecordKindDocSection && r.Name == "Authentication" {
			foundAuthSection = true
			if r.StartLine != 5 || r.EndLine != 8 {
				t.Fatalf("unexpected Authentication range: %d-%d", r.StartLine, r.EndLine)
			}
		}
		if r.Path == "README.md" && r.Kind == RecordKindDocSection && r.Name == "Token Flow" {
			foundTokenSection = true
			if r.StartLine != 9 || r.EndLine != 12 {
				t.Fatalf("unexpected Token Flow range: %d-%d", r.StartLine, r.EndLine)
			}
		}
		if r.Path == "notes.txt" && r.Kind == RecordKindChunk {
			foundChunk = true
		}
	}
	if !foundAuthSection {
		t.Fatal("expected Authentication doc section")
	}
	if !foundTokenSection {
		t.Fatal("expected Token Flow doc section")
	}
	if !foundChunk {
		t.Fatal("expected generic chunk record")
	}
}

func TestScanWorkspaceSkipReasonsAndDeterminism(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	mustWriteFile(t, filepath.Join(root, "vendor", "dep.go"), []byte("package vendor\n"))
	mustWriteFile(t, filepath.Join(root, "generated", "file.gen.go"), []byte("package generated\n"))
	mustWriteFile(t, filepath.Join(root, ".env"), []byte("PASSWORD=abc1234567890token\n"))
	mustWriteFile(t, filepath.Join(root, "binary.bin"), []byte{0, 1, 2, 3, 4})
	mustWriteFile(t, filepath.Join(root, "big.txt"), []byte("0123456789abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz"))

	opts := ScanOptions{
		Root:          root,
		MaxFileBytes:  80,
		MaxChunkBytes: 64,
		ChunkOverlap:  1,
	}
	first, err := ScanWorkspace(context.Background(), opts)
	if err != nil {
		t.Fatalf("first scan failed: %v", err)
	}
	second, err := ScanWorkspace(context.Background(), opts)
	if err != nil {
		t.Fatalf("second scan failed: %v", err)
	}

	if !reflect.DeepEqual(first.Records, second.Records) {
		t.Fatal("expected deterministic record ordering and ids between scans")
	}

	reasons := map[string]string{}
	for _, s := range first.Skipped {
		reasons[s.Path] = s.Reason
	}
	if reasons["vendor/dep.go"] != string(SkipReasonVendor) {
		t.Fatalf("vendor skip mismatch: %q", reasons["vendor/dep.go"])
	}
	if reasons["generated/file.gen.go"] != string(SkipReasonGenerated) {
		t.Fatalf("generated skip mismatch: %q", reasons["generated/file.gen.go"])
	}
	if reasons[".env"] != string(SkipReasonSecret) {
		t.Fatalf("secret skip mismatch: %q", reasons[".env"])
	}
	if reasons["binary.bin"] != string(SkipReasonBinary) {
		t.Fatalf("binary skip mismatch: %q", reasons["binary.bin"])
	}
	if reasons["big.txt"] != string(SkipReasonLarge) {
		t.Fatalf("large skip mismatch: %q", reasons["big.txt"])
	}
}

func TestScanWorkspaceDeterministicWithParallelWorkers(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	for i := 0; i < 120; i++ {
		name := fmt.Sprintf("pkg/file_%03d.go", i)
		body := []byte(fmt.Sprintf("package pkg\n\nfunc F%03d() string {\n\treturn \"ok\"\n}\n", i))
		mustWriteFile(t, filepath.Join(root, name), body)
	}
	for i := 0; i < 120; i++ {
		name := fmt.Sprintf("docs/doc_%03d.md", i)
		body := []byte(fmt.Sprintf("# Title %03d\n\n## Section\n\nSome content for %03d.\n", i, i))
		mustWriteFile(t, filepath.Join(root, name), body)
	}

	opts := ScanOptions{
		Root:             root,
		MaxFileBytes:     1 << 20,
		MaxChunkBytes:    256,
		ChunkOverlap:     16,
		ProgressEvery:    1,
		ProgressInterval: time.Nanosecond,
	}
	first, err := ScanWorkspace(context.Background(), opts)
	if err != nil {
		t.Fatalf("first scan failed: %v", err)
	}
	second, err := ScanWorkspace(context.Background(), opts)
	if err != nil {
		t.Fatalf("second scan failed: %v", err)
	}

	if !reflect.DeepEqual(first.Records, second.Records) {
		t.Fatal("expected deterministic record ordering under parallel scan")
	}
	if !reflect.DeepEqual(first.Skipped, second.Skipped) {
		t.Fatal("expected deterministic skipped ordering under parallel scan")
	}
}

func TestScanWorkspaceCancelDuringParallelProcessing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	blob := make([]byte, 64*1024)
	for i := 0; i < 200; i++ {
		name := fmt.Sprintf("bulk/file_%03d.txt", i)
		mustWriteFile(t, filepath.Join(root, name), blob)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var progressCalls atomic.Int32
	_, err := ScanWorkspace(ctx, ScanOptions{
		Root:             root,
		MaxFileBytes:     1 << 20,
		MaxChunkBytes:    512,
		ChunkOverlap:     32,
		ProgressEvery:    1,
		ProgressInterval: time.Nanosecond,
		OnProgress: func(p ScanProgress) {
			progressCalls.Add(1)
			if p.FilesSeen > 0 {
				cancel()
			}
		},
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if progressCalls.Load() == 0 {
		t.Fatal("expected progress callback to be invoked before cancellation")
	}
}

func buildFixtureWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("testdata", "workspace")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(src, e.Name()))
		if readErr != nil {
			t.Fatalf("read fixture file %s: %v", e.Name(), readErr)
		}
		mustWriteFile(t, filepath.Join(root, e.Name()), body)
	}
	return root
}

func mustWriteFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
