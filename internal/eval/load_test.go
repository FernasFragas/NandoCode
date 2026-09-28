package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadAndValidateOK(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, fixtureSpec{ID: "alpha-task"})
	writeFixture(t, root, fixtureSpec{ID: "beta-task"})

	fixtures, err := LoadAndValidate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 2 {
		t.Fatalf("len(fixtures)=%d want 2", len(fixtures))
	}
	if fixtures[0].ID != "alpha-task" || fixtures[1].ID != "beta-task" {
		t.Fatalf("unexpected fixture order: %+v", []string{fixtures[0].ID, fixtures[1].ID})
	}
}

func TestLoadAndValidateRejectsDuplicateID(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, fixtureSpec{ID: "same-id", Dir: "first"})
	writeFixture(t, root, fixtureSpec{ID: "same-id", Dir: "second"})

	_, err := LoadAndValidate(root)
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
	if !strings.Contains(err.Error(), "duplicate fixture id") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadAndValidateRejectsPathEscapePattern(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, fixtureSpec{
		ID:         "escape-path",
		ExtraYAML:  "  must_exist:\n    - ../secret.txt\n",
		OverrideWS: true,
	})

	_, err := LoadAndValidate(root)
	if err == nil {
		t.Fatal("expected invalid path error")
	}
	if !strings.Contains(err.Error(), "must not contain \"..\" path segments") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadFixturesRejectsUnknownField(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, fixtureSpec{
		ID:        "unknown-field",
		ExtraRoot: "unknown: true\n",
	})

	_, err := LoadFixtures(root)
	if err == nil {
		t.Fatal("expected yaml unknown field error")
	}
	if !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadAndValidateRejectsEscapingRecordingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "default.json")
	if err := os.WriteFile(outside, []byte(validRecordingJSON("recorded/symlink")), 0o600); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, fixtureSpec{ID: "symlink-recording"})
	target := filepath.Join(root, "symlink-recording", "recordings", "default.json")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}

	_, err := LoadAndValidate(root)
	if err == nil {
		t.Fatal("expected symlink rejection")
	}
	if !strings.Contains(err.Error(), "resolves outside fixture root") {
		t.Fatalf("error = %v", err)
	}
}

type fixtureSpec struct {
	ID         string
	Dir        string
	ExtraRoot  string
	ExtraYAML  string
	OverrideWS bool
}

func writeFixture(t *testing.T, root string, spec fixtureSpec) {
	t.Helper()
	dir := spec.Dir
	if dir == "" {
		dir = spec.ID
	}
	fixtureRoot := filepath.Join(root, dir)
	mustMkdirAll(t, filepath.Join(fixtureRoot, "repo"))
	mustMkdirAll(t, filepath.Join(fixtureRoot, "expected"))
	mustMkdirAll(t, filepath.Join(fixtureRoot, "recordings"))

	mustWriteFile(t, filepath.Join(fixtureRoot, "task.md"), "# Task\n\nUpdate the parser and run tests.\n")
	mustWriteFile(t, filepath.Join(fixtureRoot, "repo", "go.mod"), "module fixture\n\ngo 1.26.2\n")
	mustWriteFile(t, filepath.Join(fixtureRoot, "repo", "parser.go"), "package fixture\n\nfunc Parse() int { return 1 }\n")
	mustWriteFile(t, filepath.Join(fixtureRoot, "expected", "parser.go"), "package fixture\n\nfunc Parse() int { return 1 }\n")
	mustWriteFile(t, filepath.Join(fixtureRoot, "recordings", "default.json"), validRecordingJSON("recorded/"+spec.ID))

	workspaceBlock := `workspace:
  allowed_changes:
    - "parser.go"
  required_changes:
    - "parser.go"
  forbidden_changes:
    - "go.mod"
  must_exist:
    - "parser.go"
  must_not_exist: []
  max_changed_files: 1
  max_diff_lines: 40
`
	if spec.OverrideWS {
		workspaceBlock = "workspace:\n" + spec.ExtraYAML
		spec.ExtraYAML = ""
	}

	scoring := fmt.Sprintf(`version: 1
id: %s
description: Update parser behavior safely.
tags:
  - deterministic
  - go
execution:
  timeout: 30s
  max_turns: 6
  max_tool_calls: 12
  permission_mode: default
  approval_strategy: allow
model:
  live:
    temperature: 0
  recorded:
    recording: default
tests:
  - name: go-test
    command: ["go", "test", "./..."]
    timeout: 10s
    required: true
%sgolden:
  compare: exact
  files:
    - "parser.go"
checks:
  final_answer_contains:
    - "tests"
  required_tools:
    - FileRead
  forbidden_tools: []
weights:
  task_completion: 0.20
  tests: 0.30
  expected_files: 0.25
  change_scope: 0.20
  final_answer: 0.05
pass_threshold: 0.90
%s`, spec.ID, workspaceBlock, spec.ExtraRoot)
	mustWriteFile(t, filepath.Join(fixtureRoot, "scoring.yaml"), scoring)
}

func validRecordingJSON(model string) string {
	return fmt.Sprintf(`{
  "version": 1,
  "model": %q,
  "strict": true,
  "turns": [
    {
      "match": {
        "latest_user_contains": "run tests",
        "required_tools": ["FileRead"]
      },
      "events": [
        {
          "message": {
            "role": "assistant",
            "content": "Recorded fixture placeholder."
          }
        },
        {
          "done": true,
          "done_reason": "stop"
        }
      ]
    }
  ]
}
`, model)
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
