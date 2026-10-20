package permissions

import "testing"

func TestSessionAllowRuleMatchesOnlyTheApprovedCall(t *testing.T) {
	t.Parallel()
	rule, ok := SessionAllowRule("Bash", "go test ./...")
	if !ok {
		t.Fatal("expected a rule for a non-empty target")
	}
	if rule.Source != SourceSession {
		t.Fatalf("source=%v want session", rule.Source)
	}
	rules := Rules{AlwaysAllow: []Rule{rule}}
	if _, d, ok := rules.FirstMatchingRule("Bash", "go test ./..."); !ok || d != DecisionAllow {
		t.Fatalf("same tool and target must match, ok=%v d=%v", ok, d)
	}
	for _, c := range []struct{ tool, target string }{
		{"Bash", "rm -rf /"},
		{"Bash", "go test ./... && rm -rf /"},
		{"Write", "go test ./..."},
	} {
		if _, _, ok := rules.FirstMatchingRule(c.tool, c.target); ok {
			t.Fatalf("rule must not match %s(%s)", c.tool, c.target)
		}
	}
}

func TestSessionAllowRuleEscapesGlobMetacharacters(t *testing.T) {
	t.Parallel()
	rule, ok := SessionAllowRule("Bash", "rm -rf *")
	if !ok {
		t.Fatal("expected a rule")
	}
	rules := Rules{AlwaysAllow: []Rule{rule}}
	if _, _, ok := rules.FirstMatchingRule("Bash", "rm -rf *"); !ok {
		t.Fatal("literal target must match itself")
	}
	if _, _, ok := rules.FirstMatchingRule("Bash", "rm -rf /home"); ok {
		t.Fatal("a * in the approved target must not act as a wildcard")
	}
	rule, ok = SessionAllowRule("Read", "notes[1].txt")
	if !ok {
		t.Fatal("expected a rule")
	}
	rules = Rules{AlwaysAllow: []Rule{rule}}
	if _, _, ok := rules.FirstMatchingRule("Read", "notes[1].txt"); !ok {
		t.Fatal("literal [ ] target must match itself")
	}
	if _, _, ok := rules.FirstMatchingRule("Read", "notes1.txt"); ok {
		t.Fatal("[1] must not act as a character class")
	}
}

func TestSessionAllowRuleRejectsEmptyInput(t *testing.T) {
	t.Parallel()
	if _, ok := SessionAllowRule("", "x"); ok {
		t.Fatal("empty tool name must not produce a rule")
	}
	if _, ok := SessionAllowRule("Bash", "  "); ok {
		t.Fatal("empty target must not produce a rule")
	}
}
