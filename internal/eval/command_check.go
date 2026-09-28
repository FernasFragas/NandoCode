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

	cmd := exec.CommandContext(runCtx, cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir = workspaceDir
	cmd.Env = buildCommandEnv(env, workspaceDir)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	err := cmd.Run()
	res.DurationMS = time.Since(start).Milliseconds()
	res.StdoutPath = filepath.Join(artifactDir, cfg.Name+".stdout.txt")
	res.StderrPath = filepath.Join(artifactDir, cfg.Name+".stderr.txt")
	if writeErr := os.MkdirAll(artifactDir, 0o755); writeErr != nil {
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

func buildCommandEnv(overrides map[string]string, workspaceDir string) []string {
	allow := map[string]string{}
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "USER", "SHELL", "LANG", "LC_ALL", "TERM", "GOCACHE", "GOMODCACHE", "GOPATH"} {
		if value := os.Getenv(key); value != "" {
			allow[key] = value
		}
	}
	cacheDir := filepath.Join(workspaceDir, ".tmp-go-cache")
	modCache := filepath.Join(workspaceDir, ".tmp-go-mod-cache")
	allow["GOCACHE"] = cacheDir
	allow["GOMODCACHE"] = modCache
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
