package eval

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

type scoringContext struct {
	Fixture     Fixture
	Workspace   Workspace
	Changes     []FileChange
	Diff        DiffSize
	FinalAnswer string
	ToolNames   map[string]int
	ToolLimit   bool // execution.max_tool_calls was exceeded
	Terminal    string
	TestResults []TestResult
}

func EvaluateChecks(ctx scoringContext) (float64, []CheckResult, []string, string, []string, error) {
	var checks []CheckResult
	var hardFailures []string

	testsPassed := 0
	testsTotal := len(ctx.TestResults)
	requiredTestFailed := false
	for _, test := range ctx.TestResults {
		if test.Status == "passed" {
			testsPassed++
		}
		if test.Required && test.Status != "passed" {
			requiredTestFailed = true
		}
	}
	testsScore := 1.0
	if testsTotal > 0 {
		testsScore = float64(testsPassed) / float64(testsTotal)
	}
	if requiredTestFailed {
		hardFailures = append(hardFailures, "required test failed")
	}
	checks = append(checks, CheckResult{Name: "tests", Status: passFail(testsScore == 1.0), Score: testsScore})

	expectedMatches, expectedTotal, err := compareExpectedFiles(ctx.Fixture.ExpectedDir, ctx.Workspace.Path, ctx.Fixture.Config.Golden.Files)
	if err != nil {
		return 0, nil, nil, "", nil, err
	}
	expectedScore := 1.0
	if expectedTotal > 0 {
		expectedScore = float64(expectedMatches) / float64(expectedTotal)
	}
	if expectedMatches != expectedTotal {
		hardFailures = append(hardFailures, "golden file mismatch")
	}
	checks = append(checks, CheckResult{Name: "expected_files", Status: passFail(expectedScore == 1.0), Score: expectedScore})

	unexpected := unexpectedChanges(ctx.Fixture.Config.Workspace.AllowedChanges, ctx.Changes)
	if len(unexpected) > 0 && !ctx.Fixture.Config.Workspace.AllowUnexpectedChanges {
		hardFailures = append(hardFailures, "unexpected files changed")
	}
	missingRequiredChanges := missingRequiredChangePatterns(ctx.Fixture.Config.Workspace.RequiredChanges, ctx.Changes)
	if len(missingRequiredChanges) > 0 {
		hardFailures = append(hardFailures, "required changes missing")
	}
	forbidden := forbiddenChanges(ctx.Fixture.Config.Workspace.ForbiddenChanges, ctx.Changes)
	if len(forbidden) > 0 {
		hardFailures = append(hardFailures, "forbidden files changed")
	}
	mustExistMissing, err := missingRequiredPaths(ctx.Workspace.Path, ctx.Fixture.Config.Workspace.MustExist)
	if err != nil {
		return 0, nil, nil, "", nil, err
	}
	if len(mustExistMissing) > 0 {
		hardFailures = append(hardFailures, "required path missing")
	}
	mustNotExistPresent, err := presentForbiddenPaths(ctx.Workspace.Path, ctx.Fixture.Config.Workspace.MustNotExist)
	if err != nil {
		return 0, nil, nil, "", nil, err
	}
	if len(mustNotExistPresent) > 0 {
		hardFailures = append(hardFailures, "forbidden path present")
	}
	if max := ctx.Fixture.Config.Workspace.MaxChangedFiles; max > 0 && len(ctx.Changes) > max {
		hardFailures = append(hardFailures, "changed file limit exceeded")
	}
	if max := ctx.Fixture.Config.Workspace.MaxDiffLines; max > 0 && (ctx.Diff.LinesAdded+ctx.Diff.LinesDeleted) > max {
		hardFailures = append(hardFailures, "diff line limit exceeded")
	}
	changeScopeOK := len(unexpected) == 0 && len(missingRequiredChanges) == 0 && len(forbidden) == 0 && len(mustExistMissing) == 0 && len(mustNotExistPresent) == 0
	changeScopeScore := 0.0
	if changeScopeOK {
		changeScopeScore = 1.0
	}
	checks = append(checks, CheckResult{Name: "change_scope", Status: passFail(changeScopeOK), Score: changeScopeScore, Details: strings.Join(unexpected, ", ")})

	requiredToolsMissing := missingTools(ctx.Fixture.Config.Checks.RequiredTools, ctx.ToolNames)
	forbiddenToolsUsed := presentTools(ctx.Fixture.Config.Checks.ForbiddenTools, ctx.ToolNames)
	var toolProblems []string
	if len(requiredToolsMissing) > 0 {
		toolProblems = append(toolProblems, "required tools not called: "+strings.Join(requiredToolsMissing, ", "))
	}
	if len(forbiddenToolsUsed) > 0 {
		toolProblems = append(toolProblems, "forbidden tools called: "+strings.Join(forbiddenToolsUsed, ", "))
	}
	if ctx.ToolLimit {
		toolProblems = append(toolProblems, fmt.Sprintf("tool call limit %d exceeded", ctx.Fixture.Config.Execution.MaxToolCalls))
		hardFailures = append(hardFailures, "tool call limit exceeded")
	}
	if len(requiredToolsMissing) > 0 || len(forbiddenToolsUsed) > 0 {
		hardFailures = append(hardFailures, "tool usage check failed")
	}
	toolUsageScore := 1.0
	if len(toolProblems) > 0 {
		toolUsageScore = 0
	}
	// tool_usage is a hard gate with no weight; it is reported so a failed
	// status is never shown next to an all-passed check list.
	checks = append(checks, CheckResult{Name: "tool_usage", Status: passFail(len(toolProblems) == 0), Score: toolUsageScore, Details: strings.Join(toolProblems, "; ")})

	finalAnswerScore := containsAllScore(ctx.FinalAnswer, ctx.Fixture.Config.Checks.FinalAnswerContains)
	checks = append(checks, CheckResult{Name: "final_answer", Status: passFail(finalAnswerScore == 1.0), Score: finalAnswerScore})

	taskCompletionScore := 0.0
	switch ctx.Terminal {
	case "completed":
		taskCompletionScore = 1.0
	case "unrecoverable", "context_overflow", "max_turns":
		hardFailures = append(hardFailures, "terminal reason indicates task failure")
	}
	checks = append(checks, CheckResult{Name: "task_completion", Status: passFail(taskCompletionScore == 1.0), Score: taskCompletionScore})

	score := (taskCompletionScore * ctx.Fixture.Config.Weights.TaskCompletion) +
		(testsScore * ctx.Fixture.Config.Weights.Tests) +
		(expectedScore * ctx.Fixture.Config.Weights.ExpectedFiles) +
		(changeScopeScore * ctx.Fixture.Config.Weights.ChangeScope) +
		(finalAnswerScore * ctx.Fixture.Config.Weights.FinalAnswer)
	failureReason := ""
	if len(hardFailures) > 0 {
		failureReason = hardFailures[0]
	} else if score < ctx.Fixture.Config.PassThreshold {
		failureReason = fmt.Sprintf("score %.2f below threshold %.2f", score, ctx.Fixture.Config.PassThreshold)
	}

	return score, checks, hardFailures, failureReason, unexpected, nil
}

func compareExpectedFiles(expectedDir, workspaceDir string, files []string) (int, int, error) {
	if len(files) == 0 {
		return 0, 0, nil
	}
	// Read through os.Root so neither a golden path nor a symlink the agent
	// planted in the workspace can resolve to a file outside its directory.
	expectedRoot, err := os.OpenRoot(expectedDir)
	if err != nil {
		return 0, 0, err
	}
	defer expectedRoot.Close()
	workspaceRoot, err := os.OpenRoot(workspaceDir)
	if err != nil {
		return 0, 0, err
	}
	defer workspaceRoot.Close()

	matches := 0
	for _, rel := range files {
		name := filepath.FromSlash(rel)
		expectedBytes, err := expectedRoot.ReadFile(name)
		if err != nil {
			return 0, 0, err
		}
		actualBytes, err := workspaceRoot.ReadFile(name)
		if err != nil {
			if agentReplacedGoldenPath(workspaceRoot, name, err) {
				continue // a task failure (mismatch), not a harness error
			}
			return 0, 0, err
		}
		if normalizeText(expectedBytes) == normalizeText(actualBytes) {
			matches++
		}
	}
	return matches, len(files), nil
}

// agentReplacedGoldenPath reports whether a golden file is unreadable because
// the agent deleted it or replaced it with a symlink (os.Root refuses links
// that escape the workspace).
func agentReplacedGoldenPath(root *os.Root, name string, readErr error) bool {
	if os.IsNotExist(readErr) {
		return true
	}
	info, err := root.Lstat(name)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func unexpectedChanges(allowed []string, changes []FileChange) []string {
	var out []string
	for _, change := range changes {
		if !matchesAnyPattern(allowed, change.Path) {
			out = append(out, change.Path)
		}
	}
	return out
}

func missingRequiredChangePatterns(required []string, changes []FileChange) []string {
	var changedPaths []string
	for _, change := range changes {
		changedPaths = append(changedPaths, change.Path)
	}
	var missing []string
	for _, pattern := range required {
		if !matchesAnyPath(pattern, changedPaths) {
			missing = append(missing, pattern)
		}
	}
	return missing
}

func forbiddenChanges(forbidden []string, changes []FileChange) []string {
	var out []string
	for _, change := range changes {
		if matchesAnyPattern(forbidden, change.Path) {
			out = append(out, change.Path)
		}
	}
	return out
}

func missingRequiredPaths(root string, required []string) ([]string, error) {
	var missing []string
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, rel)
				continue
			}
			return nil, err
		}
	}
	return missing, nil
}

func presentForbiddenPaths(root string, forbidden []string) ([]string, error) {
	var present []string
	for _, rel := range forbidden {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			present = append(present, rel)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return present, nil
}

func containsAllScore(text string, required []string) float64 {
	if len(required) == 0 {
		return 1
	}
	text = strings.ToLower(text)
	matched := 0
	for _, needle := range required {
		if strings.Contains(text, strings.ToLower(needle)) {
			matched++
		}
	}
	return float64(matched) / float64(len(required))
}

func missingTools(required []string, used map[string]int) []string {
	var missing []string
	for _, tool := range required {
		if used[tool] == 0 {
			missing = append(missing, tool)
		}
	}
	return missing
}

func presentTools(forbidden []string, used map[string]int) []string {
	var out []string
	for _, tool := range forbidden {
		if used[tool] > 0 {
			out = append(out, tool)
		}
	}
	return out
}

func matchesAnyPath(pattern string, paths []string) bool {
	for _, candidate := range paths {
		if matchesPattern(pattern, candidate) {
			return true
		}
	}
	return false
}

func matchesAnyPattern(patterns []string, value string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if matchesPattern(pattern, value) {
			return true
		}
	}
	return false
}

func matchesPattern(patternValue, candidate string) bool {
	patternValue = path.Clean(patternValue)
	candidate = path.Clean(candidate)
	match, err := path.Match(patternValue, candidate)
	if err != nil {
		return false
	}
	return match || patternValue == candidate || slices.Contains([]string{patternValue, candidate}, ".")
}

func passFail(ok bool) string {
	if ok {
		return "passed"
	}
	return "failed"
}
