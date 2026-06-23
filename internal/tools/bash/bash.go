// Package bash implements the Bash tool.
package bash

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

// Input is the Bash tool input.
type Input struct {
	Command     string            `json:"command"`
	Description string            `json:"description,omitempty"`
	TimeoutMS   int               `json:"timeout_ms,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

// PermissionTarget returns the command text for permission target matching.
func (i Input) PermissionTarget() string {
	return i.Command
}

// Output is the Bash tool output.
type Output struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
}

// NewBashTool creates a Bash tool.
func NewBashTool() tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:              "Bash",
		Description:       "Run a shell command in the working directory.",
		Schema:            schema(),
		Unmarshal:         unmarshalInput,
		IsReadOnlyFunc:    isReadOnly,
		IsConcurrentFunc:  isReadOnly,
		IsDestructiveFunc: isDestructive,
		CheckPermFunc:     checkPermissions,
		CallFunc:          call,
		RenderFunc: func(input any, result tools.Result) tools.RenderHints {
			in, _ := input.(Input)
			return tools.RenderHints{Title: "Bash", Summary: in.Command}
		},
	})
}

func schema() map[string]any {
	properties := map[string]any{
		"command":     tools.StringProperty("Shell command to run."),
		"description": tools.StringProperty("Short human description of why the command is needed."),
		"timeout_ms":  tools.IntegerProperty("Optional timeout in milliseconds.", 1),
		"env": map[string]any{
			"type":                 "object",
			"additionalProperties": map[string]any{"type": "string"},
			"description":          "Optional environment variable overrides.",
		},
	}
	return tools.ObjectSchema(properties, []string{"command"})
}

func unmarshalInput(raw json.RawMessage) (any, error) {
	var input Input
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Command) == "" {
		return nil, errors.New("command is required")
	}
	if input.TimeoutMS < 0 {
		return nil, errors.New("timeout_ms must be non-negative")
	}
	for key := range input.Env {
		if !validEnvName(key) {
			return nil, fmt.Errorf("invalid environment variable name: %s", key)
		}
	}
	return input, nil
}

func isReadOnly(input any) bool {
	in, ok := input.(Input)
	return ok && classify(in.Command).readOnly
}

func isDestructive(input any) bool {
	in, ok := input.(Input)
	return !ok || classify(in.Command).destructive
}

func checkPermissions(ctx tools.Context, input any) tools.PermissionResult {
	in, ok := input.(Input)
	if !ok {
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "invalid Bash input"}
	}
	classification := classify(in.Command)
	readOnly := classification.readOnly
	destructive := classification.destructive

	// In neutral/default mode, intrinsically destructive commands return PermDeny.
	// This ensures that "rm -rf /" is denied even in ModeBypass.
	if destructive && ctx.PermissionMode == tools.PermissionDefault {
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "command is classified as destructive"}
	}

	switch ctx.PermissionMode {
	case tools.PermissionBypassPermissions:
		// Bypass allows everything except intrinsic denial (destructive commands).
		if destructive {
			return tools.PermissionResult{Decision: tools.PermDeny, Reason: "destructive commands cannot be bypassed"}
		}
		return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
	case tools.PermissionPlan:
		if readOnly {
			return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
		}
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "plan mode allows only read-only commands"}
	case tools.PermissionDontAsk:
		if readOnly {
			return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
		}
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "command requires permission prompt"}
	default:
		if readOnly {
			return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
		}
		return tools.PermissionResult{Decision: tools.PermAsk, Reason: "command is not classified as read-only", UpdatedInput: input}
	}
}

func call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	in, ok := input.(Input)
	if !ok {
		return tools.Result{}, errors.New("invalid Bash input")
	}
	if strings.TrimSpace(ctx.WorkingDir) == "" {
		return tools.Result{}, errors.New("working directory is required")
	}

	timeout := ctx.EffectiveBashTimeout()
	if in.TimeoutMS > 0 {
		requested := time.Duration(in.TimeoutMS) * time.Millisecond
		if requested < timeout {
			timeout = requested
		}
	}
	cmdCtx, cancel := context.WithTimeout(ctx.EffectiveContext(), timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", in.Command)
	cmd.Dir = ctx.WorkingDir
	cmd.Env = append([]string(nil), ctx.Env...)
	for key, value := range in.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return tools.Result{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return tools.Result{}, err
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return tools.Result{}, err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go copyStream(&wg, stdout, &stdoutBuf, progress, "stdout")
	go copyStream(&wg, stderr, &stderrBuf, progress, "stderr")
	waitErr := cmd.Wait()
	wg.Wait()
	duration := time.Since(start)

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) || errors.Is(cmdCtx.Err(), context.Canceled) {
			exitCode = -1
			stderrBuf.WriteString(cmdCtx.Err().Error())
		} else {
			return tools.Result{}, waitErr
		}
	}

	out := Output{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		Duration: duration.String(),
	}
	display, _ := tools.TruncateDisplay(formatOutput(out), ctx.EffectiveMaxResultChars())
	return tools.Result{Data: out, Display: display}, nil
}

func copyStream(wg *sync.WaitGroup, src io.Reader, dst *bytes.Buffer, progress chan<- tools.ProgressEvent, stream string) {
	defer wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			dst.WriteString(chunk)
			if progress != nil {
				select {
				case progress <- tools.ProgressEvent{Tool: "Bash", Stream: stream, Message: chunk}:
				default:
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !(r == '_' || unicode.IsLetter(r)) {
			return false
		}
		if !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func formatOutput(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Exit code: %d\n", out.ExitCode)
	fmt.Fprintf(&b, "Duration: %s\n", out.Duration)
	if out.Stdout != "" {
		b.WriteString("\nstdout:\n")
		b.WriteString(out.Stdout)
	}
	if out.Stderr != "" {
		b.WriteString("\nstderr:\n")
		b.WriteString(out.Stderr)
	}
	return b.String()
}
