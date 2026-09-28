package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommandVersion(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "nandocodego") {
		t.Fatalf("version output = %q", out.String())
	}
}

func TestRootCommandHasDoctor(t *testing.T) {
	cmd := NewRootCmd()
	if _, _, err := cmd.Find([]string{"doctor"}); err != nil {
		t.Fatal(err)
	}
}

func TestRootCommandHasEval(t *testing.T) {
	cmd := NewRootCmd()
	if _, _, err := cmd.Find([]string{"eval"}); err != nil {
		t.Fatal(err)
	}
}

func TestRootCommandNoArgsShowsHelp(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// With no args, it tries to launch the REPL, which fails without a TTY.
	// Pass --help to test the help behavior instead.
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("help output = %q", out.String())
	}
}

func TestRunNoArgs(t *testing.T) {
	// No args launches the interactive REPL. The launcher is stubbed: starting
	// the real TUI depends on the environment (it fails without /dev/tty on
	// Unix but runs indefinitely against the Windows console).
	orig := runREPLFn
	t.Cleanup(func() { runREPLFn = orig })
	var got *replOptions
	runREPLFn = func(_ context.Context, _ *cobra.Command, opts replOptions) error {
		got = &opts
		return nil
	}

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--no-alt-screen"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected no-args invocation to launch the REPL")
	}
	if !got.noAltScreen {
		t.Fatalf("REPL options not forwarded: %+v", *got)
	}
}

func TestRootCommandUnknownCommand(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"missing-command"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unknown command error")
	}
}

func TestVersionSubcommand(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "nandocodego") {
		t.Fatalf("version output = %q", out.String())
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Fatalf("ExitCode(nil) = %d", got)
	}
	if got := ExitCode(&cliExitError{code: 2, err: assertionError("boom")}); got != 2 {
		t.Fatalf("ExitCode(cliExitError) = %d", got)
	}
	if got := ExitCode(assertionError("boom")); got != 1 {
		t.Fatalf("ExitCode(error) = %d", got)
	}
}

func TestRootPrintPassesNumCtxOption(t *testing.T) {
	orig := runPrintFn
	defer func() { runPrintFn = orig }()
	captured := printOptions{}
	runPrintFn = func(_ context.Context, _ *cobra.Command, opts printOptions) error {
		captured = opts
		return nil
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--print", "hello", "--num-ctx", "131072"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if captured.numCtx != 131072 {
		t.Fatalf("num_ctx=%d want 131072", captured.numCtx)
	}
}

func TestRootPrintPassesWatchdogTimeoutOptions(t *testing.T) {
	orig := runPrintFn
	defer func() { runPrintFn = orig }()
	captured := printOptions{}
	runPrintFn = func(_ context.Context, _ *cobra.Command, opts printOptions) error {
		captured = opts
		return nil
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{
		"--print", "hello",
		"--llm-stream-idle-timeout", "95s",
		"--cloud-llm-stream-idle-timeout", "8m",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if captured.llmStreamIdleTimeout != "95s" {
		t.Fatalf("llmStreamIdleTimeout=%q", captured.llmStreamIdleTimeout)
	}
	if captured.cloudLLMStreamIdleTimeout != "8m" {
		t.Fatalf("cloudLLMStreamIdleTimeout=%q", captured.cloudLLMStreamIdleTimeout)
	}
}

type assertionError string

func (e assertionError) Error() string {
	return string(e)
}
