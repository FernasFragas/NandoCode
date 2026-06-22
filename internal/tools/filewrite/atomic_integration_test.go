//go:build integration

package filewrite

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAtomicWriteSurvivesKillMidWrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	oldContent := []byte("old-content")
	newContent := bytes.Repeat([]byte("new-content\n"), 8*1024*1024)

	if err := os.WriteFile(target, oldContent, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestAtomicWriteKillHelperProcess", "--", target)
	cmd.Env = append(os.Environ(), "NANDOCODEGO_ATOMIC_KILL_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)
	if cmd.ProcessState == nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, oldContent) || bytes.Equal(got, newContent) {
		return
	}
	t.Fatalf("target contained partial data: got %d bytes", len(got))
}

func TestAtomicWriteKillHelperProcess(t *testing.T) {
	if os.Getenv("NANDOCODEGO_ATOMIC_KILL_HELPER") != "1" {
		t.Skip("helper process only")
	}
	if len(os.Args) == 0 {
		os.Exit(2)
	}
	target := os.Args[len(os.Args)-1]
	content := bytes.Repeat([]byte("new-content\n"), 8*1024*1024)
	if err := AtomicWrite(target, content, 0o600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
