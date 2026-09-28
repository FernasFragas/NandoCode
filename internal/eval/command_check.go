package eval

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/logging"
	"github.com/FernasFragas/Nandocode/internal/tools/filewrite"
)

func RunTestCommand(ctx context.Context, workspaceDir, artifactDir string, env map[string]string, cfg TestConfig) (TestResult, error) {
	res := TestResult{
		Name:     cfg.Name,
		Command:  append([]string(nil), cfg.Command...),
		Required: cfg.Required,
	}
	if len(cfg.Command) == 0 {
		res.Status = "error"
		res.Failure = "empty command"
		return res, errors.New("empty command")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Running the fixture's configured test command is the purpose of this
	// function. It runs without a shell (argv only), in the workspace, with a
	// minimal environment and a timeout; fixtures are trusted inputs.
	cmd := exec.CommandContext(runCtx, cfg.Command[0], cfg.Command[1:]...) // #nosec G204 -- intended: executes the fixture's test command (see above)
	cmd.Dir = workspaceDir
	cmd.Env = buildCommandEnv(env)
	// Kill the whole process tree on timeout (e.g. the test binary spawned by
	// `go test`), and stop waiting on inherited pipes shortly after.
	configureProcessTree(cmd)
	cmd.WaitDelay = commandWaitDelay
	stdoutBuf := &cappedBuffer{limit: maxCommandOutputBytes}
	stderrBuf := &cappedBuffer{limit: maxCommandOutputBytes}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	start := time.Now()
	err := cmd.Run()
	res.DurationMS = time.Since(start).Milliseconds()
	res.StdoutPath = filepath.Join(artifactDir, cfg.Name+".stdout.txt")
	res.StderrPath = filepath.Join(artifactDir, cfg.Name+".stderr.txt")
	if writeErr := os.MkdirAll(artifactDir, 0o750); writeErr != nil {
		return res, writeErr
	}
	if writeErr := filewrite.AtomicWrite(res.StdoutPath, []byte(logging.Redact(stdoutBuf.String())), 0o644); writeErr != nil {
		return res, writeErr
	}
	if writeErr := filewrite.AtomicWrite(res.StderrPath, []byte(logging.Redact(stderrBuf.String())), 0o644); writeErr != nil {
		return res, writeErr
	}

	if err == nil {
		res.Status = "passed"
		return res, nil
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.Status = "timeout"
		res.ExitCode = -1
		res.Failure = "command timed out"
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		res.Status = "failed"
		res.Failure = fmt.Sprintf("exit code %d", res.ExitCode)
		return res, nil
	}
	res.Status = "error"
	res.Failure = logging.Redact(err.Error())
	return res, err
}

// buildCommandEnv starts from a minimal allowlist plus fixture overrides.
// GOCACHE/GOMODCACHE are inherited so scored commands reuse the caller's warm
// build cache (content-addressed and safe to share) instead of recompiling the
// standard library for every fixture.
func buildCommandEnv(overrides map[string]string) []string {
	allow := map[string]string{}
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "USER", "SHELL", "LANG", "LC_ALL", "TERM", "GOCACHE", "GOMODCACHE", "GOPATH", "LOCALAPPDATA", "APPDATA", "SYSTEMROOT", "USERPROFILE"} {
		if value := os.Getenv(key); value != "" {
			allow[key] = value
		}
	}
	allow["CGO_ENABLED"] = "0"
	for key, value := range overrides {
		allow[key] = value
	}
	out := make([]string, 0, len(allow))
	for key, value := range allow {
		out = append(out, key+"="+strings.TrimSpace(value))
	}
	return out
}

const (
	maxCommandOutputBytes = 1 << 20
	commandWaitDelay      = 5 * time.Second
)

// cappedBuffer keeps at most limit bytes and drops the rest, so a noisy or
// runaway command cannot exhaust memory.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room < len(p) {
		b.truncated = true
		if room > 0 {
			b.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) String() string {
	if b.truncated {
		return b.buf.String() + fmt.Sprintf("\n[output truncated at %d bytes]\n", b.limit)
	}
	return b.buf.String()
}
