package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSupportsRepeatAndJobs(t *testing.T) {
	root := t.TempDir()
	writeRunnableFixture(t, root, "repeatable-fixture")

	fixtures, err := LoadAndValidate(root)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), fixtures, RunOptions{
		Provider:  ProviderRecorded,
		Recording: "default",
		OutputDir: filepath.Join(root, "artifacts"),
		Jobs:      2,
		Repeat:    2,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(report.Results); got != 2 {
		t.Fatalf("len(results)=%d want 2", got)
	}
	for i, result := range report.Results {
		if result.Status != StatusPassed {
			t.Fatalf("result[%d].status=%s want passed", i, result.Status)
		}
		if result.Repetition != i+1 {
			t.Fatalf("result[%d].repetition=%d want %d", i, result.Repetition, i+1)
		}
	}
	if report.Counts.Passed != 2 || report.Counts.Fixtures != 2 {
		t.Fatalf("counts=%+v", report.Counts)
	}
}

func TestRunWritesStableEmptyArrays(t *testing.T) {
	root := t.TempDir()
	writeRunnableFixture(t, root, "stable-json")

	fixtures, err := LoadAndValidate(root)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), fixtures, RunOptions{
		Provider:  ProviderRecorded,
		Recording: "default",
		OutputDir: filepath.Join(root, "artifacts"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(report.Artifacts.ResultJSON)
	if err != nil {
		t.Fatal(err)
	}
	if contains := string(data); contains == "" {
		t.Fatal("expected non-empty json report")
	}
	for _, needle := range []string{`"unexpected_files_changed": []`, `"checks": [`, `"results": [`} {
		if !strings.Contains(string(data), needle) {
			t.Fatalf("json report missing %q", needle)
		}
	}
}

func writeRunnableFixture(t *testing.T, root, id string) {
	t.Helper()
	fixtureRoot := filepath.Join(root, id)
	mustMkdirAll(t, filepath.Join(fixtureRoot, "repo"))
	mustMkdirAll(t, filepath.Join(fixtureRoot, "expected"))
	mustMkdirAll(t, filepath.Join(fixtureRoot, "recordings"))
	mustWriteFile(t, filepath.Join(fixtureRoot, "task.md"), "# Task\n\nUpdate file and run tests.\n")
	mustWriteFile(t, filepath.Join(fixtureRoot, "repo", "go.mod"), "module fixture\n\ngo 1.26.2\n")
	mustWriteFile(t, filepath.Join(fixtureRoot, "repo", "main.go"), "package fixture\n\nfunc Value() string { return \"before\" }\n")
	mustWriteFile(t, filepath.Join(fixtureRoot, "repo", "main_test.go"), "package fixture\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) {\n\tif Value() == \"\" {\n\t\tt.Fatal(\"expected value\")\n\t}\n}\n")
	mustWriteFile(t, filepath.Join(fixtureRoot, "expected", "main.go"), "package fixture\n\nfunc Value() string { return \"after\" }\n")
	mustWriteFile(t, filepath.Join(fixtureRoot, "recordings", "default.json"), `{
  "version": 1,
  "model": "recorded/fixture",
  "strict": true,
  "turns": [
    {
      "match": {"latest_user_contains":"Update file and run tests.","required_tools":["FileRead","FileWrite","Bash"]},
      "events": [{"message":{"role":"assistant","tool_calls":[{"function":{"name":"FileRead","arguments":{"path":"main.go"}}}]}},{"done":true,"done_reason":"stop"}]
    },
    {
      "match": {"latest_user_contains":"Update file and run tests.","required_tools":["FileRead","FileWrite","Bash"]},
      "events": [{"message":{"role":"assistant","tool_calls":[{"function":{"name":"FileWrite","arguments":{"path":"main.go","content":"package fixture\n\nfunc Value() string { return \"after\" }\n"}}}]}},{"done":true,"done_reason":"stop"}]
    },
    {
      "match": {"latest_user_contains":"Update file and run tests.","required_tools":["FileRead","FileWrite","Bash"]},
      "events": [{"message":{"role":"assistant","tool_calls":[{"function":{"name":"Bash","arguments":{"command":"go test ./...","description":"Run tests","timeout_ms":30000}}}]}},{"done":true,"done_reason":"stop"}]
    },
    {
      "match": {"latest_user_contains":"Update file and run tests.","required_tools":["FileRead","FileWrite","Bash"]},
      "events": [{"message":{"role":"assistant","content":"Updated the file and ran tests."}},{"done":true,"done_reason":"stop"}]
    }
  ]
}`)
	mustWriteFile(t, filepath.Join(fixtureRoot, "scoring.yaml"), `version: 1
id: `+id+`
description: Runnable fixture.
tags:
  - deterministic
execution:
  timeout: 30s
  max_turns: 6
  max_tool_calls: 10
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
  max_diff_lines: 20
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
}
