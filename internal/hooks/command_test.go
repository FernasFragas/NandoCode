package hooks

import (
	"context"
	"runtime"
	"testing"
)

func TestRunCommandHookExitTwoBlocks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX shell syntax")
	}
	h := Hook{
		Kind:    KindCommand,
		Event:   EventPreToolUse,
		Command: "echo denied by policy >&2; exit 2",
	}
	res := runCommandHook(context.Background(), h, Envelope{Event: EventPreToolUse}, DefaultConfig())
	if res.Decision != DecisionDeny {
		t.Fatalf("expected deny, got %q warning=%q", res.Decision, res.Warning)
	}
	if res.Reason != "denied by policy" {
		t.Fatalf("expected stderr reason, got %q", res.Reason)
	}
}

func TestRunCommandHookExitOneWarnsOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX shell syntax")
	}
	h := Hook{
		Kind:    KindCommand,
		Event:   EventPreToolUse,
		Command: "echo script failed >&2; exit 1",
	}
	res := runCommandHook(context.Background(), h, Envelope{Event: EventPreToolUse}, DefaultConfig())
	if res.Decision != DecisionNone {
		t.Fatalf("expected no blocking decision, got %q", res.Decision)
	}
	if res.Warning != "script failed" {
		t.Fatalf("expected warning, got %q", res.Warning)
	}
}

func TestRunCommandHookParsesStructuredStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX shell syntax")
	}
	h := Hook{
		Kind:    KindCommand,
		Event:   EventPreToolUse,
		Command: `printf '{"decision":"allow","reason":"checked"}'`,
	}
	res := runCommandHook(context.Background(), h, Envelope{Event: EventPreToolUse}, DefaultConfig())
	if res.Decision != DecisionAllow {
		t.Fatalf("expected allow, got %q", res.Decision)
	}
	if res.Reason != "checked" {
		t.Fatalf("expected reason checked, got %q", res.Reason)
	}
}
