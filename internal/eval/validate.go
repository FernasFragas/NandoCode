package eval

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type ValidationError struct {
	Diagnostics []Diagnostic
}

func (e *ValidationError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "invalid evaluation fixtures"
	}
	if len(e.Diagnostics) == 1 {
		d := e.Diagnostics[0]
		return fmt.Sprintf("%s: %s", d.Path, d.Message)
	}
	var b strings.Builder
	b.WriteString("invalid evaluation fixtures:")
	for _, d := range e.Diagnostics {
		b.WriteString("\n- ")
		b.WriteString(d.Path)
		b.WriteString(": ")
		b.WriteString(d.Message)
	}
	return b.String()
}

func ValidateFixtures(fixtures []Fixture) error {
	var diags []Diagnostic
	if len(fixtures) == 0 {
		return &ValidationError{Diagnostics: []Diagnostic{{
			Path:    ".",
			Message: "no fixtures discovered",
		}}}
	}

	seenIDs := make(map[string]string, len(fixtures))
	for _, fixture := range fixtures {
		fixtureDiags := validateFixture(fixture)
		diags = append(diags, fixtureDiags...)
		if fixture.ID != "" {
			if prev, ok := seenIDs[fixture.ID]; ok {
				diags = append(diags, Diagnostic{
					Path:    fixture.Root,
					Message: fmt.Sprintf("duplicate fixture id %q already used by %s", fixture.ID, prev),
				})
			} else {
				seenIDs[fixture.ID] = fixture.Root
			}
		}
	}

	if len(diags) == 0 {
		return nil
	}
	sortDiagnostics(diags)
	return &ValidationError{Diagnostics: diags}
}

func validateFixture(f Fixture) []Diagnostic {
	var diags []Diagnostic
	cfg := f.Config

	add := func(rel, msg string) {
		base := f.Root
		if strings.TrimSpace(rel) != "" {
			base = filepath.Join(f.Root, rel)
		}
		diags = append(diags, Diagnostic{Path: base, Message: msg})
	}

	if cfg.Version != SchemaVersion {
		add("scoring.yaml", fmt.Sprintf("unsupported version %d; want %d", cfg.Version, SchemaVersion))
	}
	if !slugPattern.MatchString(cfg.ID) {
		add("scoring.yaml", "id must match ^[a-z0-9][a-z0-9-]*$")
	}
	if want := filepath.Base(f.Root); cfg.ID != want {
		add("scoring.yaml", fmt.Sprintf("id %q must match fixture directory %q", cfg.ID, want))
	}
	if strings.TrimSpace(f.Task) == "" {
		add("task.md", "task.md must not be empty")
	}

	tagSeen := map[string]struct{}{}
	for i, tag := range cfg.Tags {
		if !slugPattern.MatchString(tag) {
			add("scoring.yaml", fmt.Sprintf("tags[%d] must match ^[a-z0-9][a-z0-9-]*$", i))
			continue
		}
		if _, ok := tagSeen[tag]; ok {
			add("scoring.yaml", fmt.Sprintf("duplicate tag %q", tag))
			continue
		}
		tagSeen[tag] = struct{}{}
	}

	if cfg.Execution.Timeout <= 0 {
		add("scoring.yaml", "execution.timeout must be > 0")
	}
	if cfg.Execution.MaxTurns <= 0 {
		add("scoring.yaml", "execution.max_turns must be > 0")
	}
	if cfg.Execution.MaxToolCalls <= 0 {
		add("scoring.yaml", "execution.max_tool_calls must be > 0")
	}
	if !slices.Contains([]string{"bypass", "dontAsk", "auto", "acceptEdits", "default", "plan", "bubble"}, cfg.Execution.PermissionMode) {
		add("scoring.yaml", fmt.Sprintf("unsupported execution.permission_mode %q", cfg.Execution.PermissionMode))
	}
	if !slices.Contains([]ApprovalStrategy{
		ApprovalStrategyAllow,
		ApprovalStrategyDeny,
		ApprovalStrategyScripted,
		ApprovalStrategyUnavailable,
	}, cfg.Execution.ApprovalStrategy) {
		add("scoring.yaml", fmt.Sprintf("unsupported execution.approval_strategy %q", cfg.Execution.ApprovalStrategy))
	}

	testNames := map[string]struct{}{}
	for i, test := range cfg.Tests {
		if strings.TrimSpace(test.Name) == "" {
			add("scoring.yaml", fmt.Sprintf("tests[%d].name must not be empty", i))
		} else if _, ok := testNames[test.Name]; ok {
			add("scoring.yaml", fmt.Sprintf("duplicate tests[%d].name %q", i, test.Name))
		} else {
			testNames[test.Name] = struct{}{}
		}
		if len(test.Command) == 0 {
			add("scoring.yaml", fmt.Sprintf("tests[%d].command must not be empty", i))
		}
		for j, arg := range test.Command {
			if strings.TrimSpace(arg) == "" {
				add("scoring.yaml", fmt.Sprintf("tests[%d].command[%d] must not be empty", i, j))
			}
		}
		if test.Timeout <= 0 {
			add("scoring.yaml", fmt.Sprintf("tests[%d].timeout must be > 0", i))
		}
	}

	validatePatterns := func(field string, values []string) {
		for i, value := range values {
			if err := validatePortablePathPattern(value); err != nil {
				add("scoring.yaml", fmt.Sprintf("%s[%d]: %v", field, i, err))
			}
		}
	}
	validatePatterns("workspace.allowed_changes", cfg.Workspace.AllowedChanges)
	validatePatterns("workspace.required_changes", cfg.Workspace.RequiredChanges)
	validatePatterns("workspace.forbidden_changes", cfg.Workspace.ForbiddenChanges)
	validatePatterns("workspace.must_exist", cfg.Workspace.MustExist)
	validatePatterns("workspace.must_not_exist", cfg.Workspace.MustNotExist)
	validatePatterns("golden.files", cfg.Golden.Files)

	if cfg.Workspace.MaxChangedFiles < 0 {
		add("scoring.yaml", "workspace.max_changed_files must be >= 0")
	}
	if cfg.Workspace.MaxDiffLines < 0 {
		add("scoring.yaml", "workspace.max_diff_lines must be >= 0")
	}
	if cfg.Golden.Compare != CompareExact {
		add("scoring.yaml", fmt.Sprintf("unsupported golden.compare %q", cfg.Golden.Compare))
	}
	for _, rel := range cfg.Golden.Files {
		full := filepath.Join(f.ExpectedDir, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil {
			add("expected", fmt.Sprintf("golden file %q is missing from expected/: %v", rel, err))
			continue
		}
		if info.IsDir() {
			add("expected", fmt.Sprintf("golden file %q must be a file", rel))
		}
	}

	for i, text := range cfg.Checks.FinalAnswerContains {
		if strings.TrimSpace(text) == "" {
			add("scoring.yaml", fmt.Sprintf("checks.final_answer_contains[%d] must not be empty", i))
		}
	}
	validateToolNames := func(field string, tools []string) {
		seen := map[string]struct{}{}
		for i, tool := range tools {
			if strings.TrimSpace(tool) == "" {
				add("scoring.yaml", fmt.Sprintf("%s[%d] must not be empty", field, i))
				continue
			}
			if _, ok := seen[tool]; ok {
				add("scoring.yaml", fmt.Sprintf("duplicate %s entry %q", field, tool))
				continue
			}
			seen[tool] = struct{}{}
		}
	}
	validateToolNames("checks.required_tools", cfg.Checks.RequiredTools)
	validateToolNames("checks.forbidden_tools", cfg.Checks.ForbiddenTools)

	weights := []struct {
		name  string
		value float64
	}{
		{"weights.task_completion", cfg.Weights.TaskCompletion},
		{"weights.tests", cfg.Weights.Tests},
		{"weights.expected_files", cfg.Weights.ExpectedFiles},
		{"weights.change_scope", cfg.Weights.ChangeScope},
		{"weights.final_answer", cfg.Weights.FinalAnswer},
	}
	total := 0.0
	for _, weight := range weights {
		if weight.value < 0 || weight.value > 1 {
			add("scoring.yaml", fmt.Sprintf("%s must be between 0 and 1", weight.name))
		}
		total += weight.value
	}
	if math.Abs(total-1.0) > 1e-6 {
		add("scoring.yaml", fmt.Sprintf("weights must sum to 1.0, got %.6f", total))
	}
	if cfg.PassThreshold < 0 || cfg.PassThreshold > 1 {
		add("scoring.yaml", "pass_threshold must be between 0 and 1")
	}

	if HasTag(cfg.Tags, "deterministic") || strings.TrimSpace(cfg.Model.Recorded.Recording) != "" {
		recordingName := cfg.Model.Recorded.Recording
		recordingRel := filepath.Join("recordings", recordingName+".json")
		recordingPath, err := resolveFixturePath(f.Root, recordingRel, false)
		if !slugPattern.MatchString(recordingName) {
			add("scoring.yaml", "model.recorded.recording must match ^[a-z0-9][a-z0-9-]*$")
		} else if err != nil {
			add(recordingRel, fmt.Sprintf("recording is required: %v", err))
		} else if _, err := os.Stat(recordingPath); err != nil {
			add(filepath.Join("recordings", recordingName+".json"), fmt.Sprintf("recording is required: %v", err))
		} else {
			recording, err := LoadRecording(f.Root, recordingName)
			if err != nil {
				add(recordingRel, fmt.Sprintf("invalid recording: %v", err))
			} else {
				validateRecording(recordingPath, recording, &diags)
			}
		}
	}

	return diags
}

func validateRecording(path string, recording Recording, diags *[]Diagnostic) {
	add := func(msg string) {
		*diags = append(*diags, Diagnostic{Path: path, Message: msg})
	}
	if recording.Version != SchemaVersion {
		add(fmt.Sprintf("unsupported recording version %d; want %d", recording.Version, SchemaVersion))
	}
	if strings.TrimSpace(recording.Model) == "" {
		add("recording.model must not be empty")
	}
	if len(recording.Turns) == 0 {
		add("recording.turns must not be empty")
	}
	for i, turn := range recording.Turns {
		if len(turn.Events) == 0 {
			add(fmt.Sprintf("turns[%d].events must not be empty", i))
		}
		seenTools := map[string]struct{}{}
		for j, tool := range turn.Match.RequiredTools {
			if strings.TrimSpace(tool) == "" {
				add(fmt.Sprintf("turns[%d].match.required_tools[%d] must not be empty", i, j))
				continue
			}
			if _, ok := seenTools[tool]; ok {
				add(fmt.Sprintf("turns[%d].match.required_tools has duplicate %q", i, tool))
				continue
			}
			seenTools[tool] = struct{}{}
		}
	}
}

func HasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func validatePortablePathPattern(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("must not be empty")
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("must use forward slashes")
	}
	if strings.HasPrefix(value, "/") {
		return fmt.Errorf("must be relative")
	}
	segments := strings.Split(value, "/")
	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf("must not contain empty path segments")
		}
		if segment == "." || segment == ".." {
			return fmt.Errorf("must not contain %q path segments", segment)
		}
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("must stay within the fixture workspace")
	}
	return nil
}

func sortDiagnostics(diags []Diagnostic) {
	slices.SortFunc(diags, func(a, b Diagnostic) int {
		if cmp := strings.Compare(a.Path, b.Path); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Message, b.Message)
	})
}
