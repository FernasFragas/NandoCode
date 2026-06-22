package permissions

import (
	"testing"
)

func TestParsePattern(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		wantTool     string
		wantGlob     string
		wantOk       bool
	}{
		{"SimpleBash", "Bash(ls*)", "Bash", "ls*", true},
		{"FileRead", "FileRead(docs/**)", "FileRead", "docs/**", true},
		{"FileWrite", "FileWrite(/tmp/*)", "FileWrite", "/tmp/*", true},
		{"NoParens", "Bash ls", "", "", false},
		{"EmptyTool", "(ls)", "", "", false},
		{"EmptyGlob", "Bash()", "", "", false},
		{"NoCloseParen", "Bash(ls", "", "", false},
		{"TrailingSpace", "  Bash(ls)  ", "Bash", "ls", true},
		{"Complex", "CustomTool(path/*/file-*.txt)", "CustomTool", "path/*/file-*.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, glob, ok := ParsePattern(tt.pattern)
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
			}
			if ok {
				if tool != tt.wantTool {
					t.Errorf("tool = %q, want %q", tool, tt.wantTool)
				}
				if glob != tt.wantGlob {
					t.Errorf("glob = %q, want %q", glob, tt.wantGlob)
				}
			}
		})
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		toolName string
		target   string
		expected bool
	}{
		// Tool name mismatch
		{"ToolMismatch", "Bash(ls)", "FileRead", "ls", false},

		// Basic glob matching
		{"ExactMatch", "Bash(ls)", "Bash", "ls", true},
		{"GlobStar", "Bash(ls*)", "Bash", "ls -la", true},
		{"GlobQuestion", "Bash(??)", "Bash", "cd", true},

		// No match
		{"NoMatch", "Bash(ls)", "Bash", "pwd", false},
		{"PartialNoMatch", "Bash(ls*)", "Bash", "pwd", false},

		// Path-like patterns
		{"UnixPath", "FileRead(/home/user/*)", "FileRead", "/home/user/file.txt", true},

		// Command-like patterns
		{"CommandGlob", "Bash(npm test*)", "Bash", "npm test", true},
		{"CommandNoMatch", "Bash(npm test*)", "Bash", "npm build", false},

		// Invalid patterns
		{"InvalidPattern", "Bash(", "Bash", "ls", false},
		{"EmptyTool", "(ls)", "Bash", "ls", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matches(tt.pattern, tt.toolName, tt.target)
			if result != tt.expected {
				t.Errorf("matches(%q, %q, %q) = %v, want %v", tt.pattern, tt.toolName, tt.target, result, tt.expected)
			}
		})
	}
}

func TestExtractTarget(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			"BashInput",
			struct {
				Command string
				Other   string
			}{Command: "ls -la", Other: "ignored"},
			"ls -la",
		},
		{
			"FileReadInput",
			struct {
				Path string
			}{Path: "/tmp/file.txt"},
			"/tmp/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTarget(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractTarget() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		target   string
		expected bool
	}{
		// Basic patterns
		{"Literal", "file.txt", "file.txt", true},
		{"LiteralNoMatch", "file.txt", "other.txt", false},

		// Star patterns
		{"StarEnd", "*.txt", "file.txt", true},
		{"StarMiddle", "f*.txt", "file.txt", true},
		{"StarStart", "f*", "file", true},

		// Question mark
		{"Question", "f?le", "file", true},
		{"QuestionNoMatch", "f?le", "fl", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := globMatch(tt.pattern, tt.target)
			if result != tt.expected {
				t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.target, result, tt.expected)
			}
		})
	}
}

func TestNormalizePathSeparators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"UnixPath", "/home/user/file.txt", "/home/user/file.txt"},
		{"NoSeparators", "filename.txt", "filename.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePathSeparators(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePathSeparators(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Test that PermissionTargeter interface is recognized
type mockTargeter struct {
	target string
}

func (m mockTargeter) PermissionTarget() string {
	return m.target
}

func TestExtractTargetFromTargeter(t *testing.T) {
	mt := mockTargeter{target: "custom-target"}
	result := ExtractTarget(mt)
	if result != "custom-target" {
		t.Errorf("ExtractTarget from PermissionTargeter = %q, want %q", result, "custom-target")
	}
}
