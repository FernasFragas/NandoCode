package permissions

import "testing"

func TestStarMatchesSlash(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		toolName string
		target   string
		want     bool
	}{
		{"DenyCatSlash", "Bash(cat *)", "Bash", "cat a/b", true},
		{"DenyCatDeepSlash", "Bash(cat *)", "Bash", "cat /etc/passwd", true},
		{"StarAcrossDirs", "FileWrite(src/*)", "FileWrite", "src/a/b.go", true},
		{"StarSuffixAcrossDirs", "FileRead(*.go)", "FileRead", "internal/x/y.go", true},
		{"StarStillAnchored", "FileRead(src/*)", "FileRead", "other/src/a.go", false},
		{"QuestionOneChar", "Bash(l?)", "Bash", "ls", true},
		{"QuestionNotZero", "Bash(l?)", "Bash", "l", false},
		{"CharClass", "Bash(l[sx])", "Bash", "ls", true},
		{"CharClassMiss", "Bash(l[sx])", "Bash", "lz", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matches(tt.pattern, tt.toolName, tt.target); got != tt.want {
				t.Fatalf("matches(%q, %q, %q) = %v, want %v", tt.pattern, tt.toolName, tt.target, got, tt.want)
			}
		})
	}
}

func TestDoubleStarGlob(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    bool
	}{
		{"TrailingDeep", "docs/**", "docs/a/b/c.md", true},
		{"TrailingOne", "docs/**", "docs/a.md", true},
		{"TrailingDirItself", "docs/**", "docs", true},
		{"TrailingWrongRoot", "docs/**", "other/a.md", false},
		{"MiddleZeroDirs", "src/**/x.go", "src/x.go", true},
		{"MiddleManyDirs", "src/**/x.go", "src/a/b/x.go", true},
		{"MiddleWrongName", "src/**/x.go", "src/a/y.go", false},
		{"LeadingZeroDirs", "**/x.go", "x.go", true},
		{"LeadingManyDirs", "**/x.go", "a/b/x.go", true},
		{"LeadingWrongName", "**/x.go", "a/b/y.go", false},
		{"TwoDoubleStars", "a/**/b/**/c", "a/b/c", true},
		{"TwoDoubleStarsDeep", "a/**/b/**/c", "a/x/b/y/z/c", true},
		{"EmbeddedIsPlainStar", "a**b", "a/x/b", true},
		{"EmbeddedNoMatch", "a**b", "a/x/c", false},
		{"GeneratedAnywhere", "**/generated/**", "pkg/generated/x.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := globMatch(tt.pattern, tt.target); got != tt.want {
				t.Fatalf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.target, got, tt.want)
			}
		})
	}
}

func TestGlobMatchManyStarsTerminates(t *testing.T) {
	pattern := "*a*a*a*a*a*a*a*a*a*a*a*a*b"
	target := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if globMatch(pattern, target) {
		t.Fatal("expected no match")
	}
}

func TestDenyRuleCatBlocksSlashPath(t *testing.T) {
	rules := Rules{AlwaysDeny: []Rule{{Pattern: "Bash(cat *)", Source: SourceUser}}}
	_, decision, ok := rules.FirstMatchingRule("Bash", "cat a/b")
	if !ok || decision != DecisionDeny {
		t.Fatalf("expected deny rule to match cat a/b, got ok=%v decision=%v", ok, decision)
	}
}

func TestManyDoubleStarSegmentsStillMatch(t *testing.T) {
	if !globMatch("**/**/**/**/**/**/**/x", "x") {
		t.Fatal("adjacent ** segments must collapse")
	}
	if !globMatch("a/**/b/**/c/**/d/**/e/**/f/**/g/**/h", "a/b/c/d/e/f/g/h") {
		t.Fatal("seven separate ** segments must all match zero directories")
	}
	if globMatch("**/**/**/**/**/**/**/x", "y") {
		t.Fatal("must not match a different name")
	}
}

func TestBackslashEscapeSurvivesRuleNormalization(t *testing.T) {
	// The rule glob is not slash-normalized, so a backslash escape keeps
	// meaning "literal next character" on every platform.
	if !matches(`Bash(rm \*)`, "Bash", "rm *") {
		t.Fatal("escaped * should match a literal star")
	}
	if matches(`Bash(rm \*)`, "Bash", "rm file") {
		t.Fatal("escaped * must not act as a wildcard")
	}
}
