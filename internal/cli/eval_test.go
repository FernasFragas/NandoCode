package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalValidateCommand(t *testing.T) {
	root := writeCLIFixtureSet(t)

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"eval", "validate", root})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Validated 1 fixture") {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(out.String(), "cli-fixture") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestEvalValidateInvalidFixtureReturnsExitCode2(t *testing.T) {
	root := t.TempDir()
	mustMkdirCLI(t, filepath.Join(root, "broken", "repo"))
	mustMkdirCLI(t, filepath.Join(root, "broken", "expected"))
	mustWriteCLI(t, filepath.Join(root, "broken", "task.md"), "# Broken\n")
	mustWriteCLI(t, filepath.Join(root, "broken", "scoring.yaml"), "version: 1\nid: broken\ndescription: bad\n")

	err := Run(t.Context(), []string{"eval", "validate", root})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d want 2", ExitCode(err))
	}
}

func TestEvalRunRejectsInvalidOptionsWithExitCode2(t *testing.T) {
	root := writeCLIFixtureSet(t)
	for _, args := range [][]string{
		{"--model", "llama3"},
		{"--provider", "bogus"},
	} {
		err := Run(t.Context(), append([]string{"eval", "run", root, "--output-dir", t.TempDir()}, args...))
		if ExitCode(err) != 2 {
			t.Fatalf("args %v: exit code = %d (err=%v), want 2", args, ExitCode(err), err)
		}
	}
}

func TestEvalRunPassesForDeterministicFixture(t *testing.T) {
	root := writeCLIFixtureSet(t)

	outDir := t.TempDir()
	err := Run(t.Context(), []string{"eval", "run", root, "--output-dir", outDir})
	if err != nil {
		t.Fatalf("run error = %v\n%s", err, evalResultsForDebug(t, outDir))
	}
}

// evalResultsForDebug returns the results.json of the run under outDir so a
// failing assertion shows each fixture's failure_reason.
func evalResultsForDebug(t *testing.T, outDir string) string {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(outDir, "*", "results.json"))
	if len(matches) == 0 {
		return "(no results.json written)"
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func writeCLIFixtureSet(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	fixtureRoot := filepath.Join(root, "cli-fixture")
	mustMkdirCLI(t, filepath.Join(fixtureRoot, "repo"))
	mustMkdirCLI(t, filepath.Join(fixtureRoot, "expected"))
	mustMkdirCLI(t, filepath.Join(fixtureRoot, "recordings"))
	mustWriteCLI(t, filepath.Join(fixtureRoot, "task.md"), "# Task\n\nRun tests.\n")
	mustWriteCLI(t, filepath.Join(fixtureRoot, "repo", "go.mod"), "module cli\n\ngo 1.26.2\n")
	mustWriteCLI(t, filepath.Join(fixtureRoot, "repo", "main.go"), "package cli\n\nfunc Message() string { return \"before\" }\n")
	mustWriteCLI(t, filepath.Join(fixtureRoot, "repo", "main_test.go"), "package cli\n\nimport \"testing\"\n\nfunc TestMessage(t *testing.T) {\n\tif Message() == \"\" {\n\t\tt.Fatal(\"expected message\")\n\t}\n}\n")
	mustWriteCLI(t, filepath.Join(fixtureRoot, "expected", "main.go"), "package cli\n\nfunc Message() string { return \"after\" }\n")
	mustWriteCLI(t, filepath.Join(fixtureRoot, "recordings", "default.json"), `{
  "version": 1,
  "model": "recorded/cli-fixture",
  "strict": true,
  "turns": [
    {
      "match": {"latest_user_contains":"Run tests.","required_tools":["FileRead","FileWrite","Bash"]},
      "events": [{"message":{"role":"assistant","tool_calls":[{"function":{"name":"FileRead","arguments":{"path":"main.go"}}}]}},{"done":true,"done_reason":"stop"}]
    },
    {
      "match": {"latest_user_contains":"Run tests.","required_tools":["FileRead","FileWrite","Bash"]},
      "events": [{"message":{"role":"assistant","tool_calls":[{"function":{"name":"FileWrite","arguments":{"path":"main.go","content":"package cli\n\nfunc Message() string { return \"after\" }\n"}}}]}},{"done":true,"done_reason":"stop"}]
    },
    {
      "match": {"latest_user_contains":"Run tests.","required_tools":["FileRead","FileWrite","Bash"]},
      "events": [{"message":{"role":"assistant","tool_calls":[{"function":{"name":"Bash","arguments":{"command":"go test ./...","description":"Run tests","timeout_ms":30000}}}]}},{"done":true,"done_reason":"stop"}]
    },
    {
      "match": {"latest_user_contains":"Run tests.","required_tools":["FileRead","FileWrite","Bash"]},
      "events": [{"message":{"role":"assistant","content":"Updated the file and ran tests."}},{"done":true,"done_reason":"stop"}]
    }
  ]
}`)
	mustWriteCLI(t, filepath.Join(fixtureRoot, "scoring.yaml"), `version: 1
id: cli-fixture
description: CLI validation fixture.
tags:
  - deterministic
execution:
  timeout: 120s
  max_turns: 4
  max_tool_calls: 8
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
    timeout: 60s
    required: true
workspace:
  allowed_changes:
    - "main.go"
  required_changes:
    - "main.go"
  forbidden_changes:
    - "go.mod"
  must_exist:
    - "main.go"
  must_not_exist: []
  max_changed_files: 1
  max_diff_lines: 10
golden:
  compare: exact
  files:
    - "main.go"
checks:
  final_answer_contains:
    - "tests"
  required_tools:
    - FileRead
    - FileWrite
    - Bash
  forbidden_tools: []
weights:
  task_completion: 0.20
  tests: 0.30
  expected_files: 0.25
  change_scope: 0.20
  final_answer: 0.05
pass_threshold: 0.90
`)
	return root
}

func mustMkdirCLI(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteCLI(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
