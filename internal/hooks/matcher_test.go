package hooks

import "testing"

func TestMatchesHookToolPattern(t *testing.T) {
	h := Hook{Event: EventPreToolUse, Matcher: "Bash(rm -rf*)"}
	env := Envelope{
		Event: EventPreToolUse,
		Tool:  &ToolInput{Name: "Bash", Target: "rm -rf /tmp/demo"},
	}
	if !matchesHook(h, env) {
		t.Fatalf("expected hook to match")
	}
}

func TestMatchesHookRejectsWrongTool(t *testing.T) {
	h := Hook{Event: EventPreToolUse, Matcher: "Bash(rm -rf*)"}
	env := Envelope{
		Event: EventPreToolUse,
		Tool:  &ToolInput{Name: "FileRead", Target: "rm -rf /tmp/demo"},
	}
	if matchesHook(h, env) {
		t.Fatalf("expected hook not to match")
	}
}
