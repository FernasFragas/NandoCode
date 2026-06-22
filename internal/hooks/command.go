package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func runCommandHook(ctx context.Context, h Hook, env Envelope, cfg Config) Result {
	timeout := h.Timeout(cfg.DefaultTimeout)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := json.Marshal(env)
	if err != nil {
		return Result{Warning: "failed to encode hook input: " + err.Error()}
	}

	shell, arg := shellCommand()
	cmd := exec.CommandContext(runCtx, shell, arg, h.Command)
	cmd.Dir = cfg.WorkingDir
	cmd.Env = hookEnv(h, cfg)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if runCtx.Err() != nil {
		return Result{Warning: "hook timed out"}
	}

	if err == nil {
		return parseHookJSON(stdout.String())
	}

	exitCode := exitCode(err)
	reason := sanitize(stderr.String())
	if reason == "" {
		reason = "hook command failed"
	}
	if exitCode == 2 {
		return Result{Decision: DecisionDeny, Reason: reason}
	}
	return Result{Warning: reason}
}

func shellCommand() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "/bin/sh", "-c"
}

func hookEnv(h Hook, cfg Config) []string {
	env := baseEnv()
	env = append(env, "NANDOCODEGO_PROJECT_DIR="+cfg.WorkingDir)
	env = append(env, "NANDOCODEGO_SESSION_ID="+cfg.SessionID)
	for k, v := range h.Env {
		if strings.TrimSpace(k) != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func baseEnv() []string {
	names := []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "TEMP", "TMP", "SystemRoot", "ComSpec", "USERPROFILE"}
	env := make([]string, 0, len(names))
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
