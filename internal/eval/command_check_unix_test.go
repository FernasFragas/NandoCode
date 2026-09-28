//go:build unix

package eval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunTestCommandTimeoutKillsProcessTree(t *testing.T) {
	workspace, artifacts := t.TempDir(), t.TempDir()
	// The shell backgrounds a grandchild that holds stdout open, like the test
	// binary spawned by `go test`, and records its pid.
	res, err := RunTestCommand(context.Background(), workspace, artifacts, nil, TestConfig{
		Name:    "hang",
		Command: []string{"sh", "-c", "sleep 60 & echo $! > child.pid; wait"},
		Timeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "timeout" {
		t.Fatalf("status = %q, want timeout", res.Status)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("grandchild %d still running after command timeout", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
