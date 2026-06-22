package tasks

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestOutputWriterAndTailLines(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "session", "tasks", "task-1.jsonl")
	w, err := NewOutputWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := w.WriteText("stdout", "line"); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.WriteExit(0); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	lines, err := TailLines(path, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	last := lines[len(lines)-1]
	if last.Kind != "exit" || last.Code != 0 {
		t.Fatalf("expected exit sentinel as last line, got %#v", last)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected 0600 file mode, got %o", got)
	}
	dirSt, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirSt.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected 0700 dir mode, got %o", got)
	}
}

func TestTailLinesEmptyAndMissing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := TailLines(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected empty tail, got %d lines", len(lines))
	}
	if _, err := TailLines(filepath.Join(t.TempDir(), "missing.jsonl"), 5); err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestOutputWriterConcurrentWritesProduceValidJSONL(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "task.jsonl")
	w, err := NewOutputWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				stream := "stdout"
				if idx%2 == 0 {
					stream = "stderr"
				}
				if err := w.WriteText(stream, "chunk"); err != nil {
					t.Errorf("write error: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	if err := w.WriteExit(7); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	count := 0
	for sc.Scan() {
		var line OutputLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			t.Fatalf("invalid json line: %v", err)
		}
		count++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("expected output lines")
	}
}
