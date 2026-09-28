package eval

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/logging"
	"github.com/FernasFragas/Nandocode/internal/tools/filewrite"
)

func Run(ctx context.Context, fixtures []Fixture, opts RunOptions, progress func(string)) (RunReport, error) {
	if len(fixtures) == 0 {
		return RunReport{}, errors.New("no fixtures selected")
	}
	jobs := opts.Jobs
	if jobs <= 0 {
		jobs = 1
	}
	repeat := opts.Repeat
	if repeat <= 0 {
		repeat = 1
	}
	started := time.Now().UTC()
	runID := fmt.Sprintf("%s-%08x", started.Format("20060102T150405Z"), rand.Uint32())
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = ".tmp-evals"
	}
	runDir := filepath.Join(outputDir, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "fixtures"), 0o755); err != nil {
		return RunReport{}, err
	}

	report := RunReport{
		SchemaVersion: SchemaVersion,
		RunID:         runID,
		Provider:      opts.Provider,
		Model:         opts.Model,
		Recording:     opts.Recording,
		StartedAt:     started.Format(time.RFC3339),
	}

	targets := expandTargets(fixtures, repeat)
	results := runTargets(ctx, runID, runDir, targets, opts, jobs, progress)
	totalScore := 0.0
	for _, result := range results {
		report.Results = append(report.Results, result)
		totalScore += result.Score
		report.Counts.Fixtures++
		switch result.Status {
		case StatusPassed:
			report.Counts.Passed++
		case StatusFailed, StatusTimeout, StatusCanceled:
			report.Counts.Failed++
		case StatusError:
			report.Counts.Errors++
		case StatusSkipped:
			report.Counts.Skipped++
		}
	}
	report.AggregateScore = totalScore / float64(len(report.Results))
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	report.Artifacts = ArtifactPaths{
		ResultJSON:     filepath.Join(runDir, "results.json"),
		MarkdownReport: filepath.Join(runDir, "report.md"),
	}
	if err := WriteJSONReport(report.Artifacts.ResultJSON, report); err != nil {
		return RunReport{}, err
	}
	if err := WriteMarkdownReport(report.Artifacts.MarkdownReport, report); err != nil {
		return RunReport{}, err
	}
	return report, nil
}

type runTarget struct {
	fixture     Fixture
	repetition  int
	repeatTotal int
	index       int
}

func expandTargets(fixtures []Fixture, repeat int) []runTarget {
	targets := make([]runTarget, 0, len(fixtures)*repeat)
	idx := 0
	for _, fixture := range fixtures {
		for repetition := 1; repetition <= repeat; repetition++ {
			targets = append(targets, runTarget{
				fixture:     fixture,
				repetition:  repetition,
				repeatTotal: repeat,
				index:       idx,
			})
			idx++
		}
	}
	return targets
}

func runTargets(ctx context.Context, runID, runDir string, targets []runTarget, opts RunOptions, jobs int, progress func(string)) []FixtureResult {
	results := make([]FixtureResult, len(targets))
	work := make(chan runTarget)
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range work {
				if progress != nil {
					name := target.fixture.ID
					if target.repeatTotal > 1 {
						name = fmt.Sprintf("%s#%d", target.fixture.ID, target.repetition)
					}
					progress(fmt.Sprintf("Running %s", name))
				}
				results[target.index] = runFixture(ctx, runID, runDir, target, opts)
			}
		}()
	}
	for _, target := range targets {
		work <- target
	}
	close(work)
	wg.Wait()
	return results
}

func runFixture(parent context.Context, runID, runDir string, target runTarget, opts RunOptions) FixtureResult {
	fixture := target.fixture
	started := time.Now().UTC()
	result := FixtureResult{
		SchemaVersion: SchemaVersion,
		RunID:         runID,
		FixtureID:     fixture.ID,
		Repetition:    target.repetition,
		Description:   fixture.Config.Description,
		Provider:      opts.Provider,
		StartedAt:     started.Format(time.RFC3339),
		Status:        StatusError,
	}
	fixtureDir := filepath.Join(runDir, "fixtures", fixture.ID)
	if target.repeatTotal > 1 {
		fixtureDir = filepath.Join(fixtureDir, fmt.Sprintf("run-%02d", target.repetition))
	}
	testsDir := filepath.Join(fixtureDir, "tests")
	result.Artifacts = ArtifactPaths{
		ResultJSON:      filepath.Join(fixtureDir, "result.json"),
		FinalAnswer:     filepath.Join(fixtureDir, "final-answer.md"),
		DiffPatch:       filepath.Join(fixtureDir, "diff.patch"),
		TestStdoutPaths: map[string]string{},
		TestStderrPaths: map[string]string{},
	}
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		result.FailureReason = logging.Redact(err.Error())
		return finalizeFixtureResult(result)
	}

	timeout := fixture.Config.Execution.Timeout
	if opts.Timeout > 0 && (timeout == 0 || opts.Timeout < timeout) {
		timeout = opts.Timeout
	}
	ctx := parent
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, timeout)
		defer cancel()
	}

	workspace, err := PrepareWorkspace("", fixture)
	if err != nil {
		result.FailureReason = logging.Redact(err.Error())
		return finalizeFixtureResult(result)
	}
	if opts.KeepWorkspaces {
		result.Artifacts.Workspace = workspace.Path
	}
	defer CleanupWorkspace(workspace, opts.KeepWorkspaces)

	before, err := Snapshot(workspace.Path)
	if err != nil {
		result.FailureReason = logging.Redact(err.Error())
		return finalizeFixtureResult(result)
	}

	bundle, err := buildRuntime(ctx, fixture, workspace.Path, opts)
	if err != nil {
		result.FailureReason = logging.Redact(err.Error())
		return finalizeFixtureResult(result)
	}
	result.ModelUsed = bundle.model
	input, prompt := buildAgentInput(ctx, fixture, workspace.Path, fixture.Config, bundle.model)
	metrics := newRunMetrics()
	input.PermissionPrompt = metrics.wrapPrompt(prompt)
	bundle.agentCfg.PermissionObserver = metrics.permissionObserver()
	runner, err := agent.New(bundle.client, bundle.registry, agent.WithConfig(bundle.agentCfg))
	if err != nil {
		result.FailureReason = logging.Redact(err.Error())
		return finalizeFixtureResult(result)
	}
	events := runner.Run(ctx, input)
	finalAnswer, terminal := metrics.collect(events)

	result.TaskCompletionStatus = terminalTaskCompletion(terminal.Reason)
	result.TerminalReason = string(terminal.Reason)
	result.Usage = terminal.Usage
	result.ToolCalls = terminal.Usage.ToolCalls
	result.RuntimeMS = time.Since(started).Milliseconds()

	after, err := Snapshot(workspace.Path)
	if err != nil {
		result.FailureReason = logging.Redact(err.Error())
		return finalizeFixtureResult(result)
	}
	changes, diffSize, patch, err := DiffManifests(before, after)
	if err != nil {
		result.FailureReason = logging.Redact(err.Error())
		return finalizeFixtureResult(result)
	}
	result.FilesChanged = changes
	result.FinalDiffSize = diffSize
	if patch != "" {
		if err := filewrite.AtomicWrite(result.Artifacts.DiffPatch, []byte(patch), 0o644); err != nil {
			result.FailureReason = logging.Redact(err.Error())
			return finalizeFixtureResult(result)
		}
	}
	if err := filewrite.AtomicWrite(result.Artifacts.FinalAnswer, []byte(finalAnswer), 0o644); err != nil {
		result.FailureReason = logging.Redact(err.Error())
		return finalizeFixtureResult(result)
	}

	for _, testCfg := range fixture.Config.Tests {
		testRes, testErr := RunTestCommand(ctx, workspace.Path, testsDir, fixture.Config.Execution.Environment, testCfg)
		result.TestResults = append(result.TestResults, testRes)
		result.Artifacts.TestStdoutPaths[testRes.Name] = testRes.StdoutPath
		result.Artifacts.TestStderrPaths[testRes.Name] = testRes.StderrPath
		if testRes.Status == "passed" {
			result.TestsPassed++
		} else {
			result.TestsFailed++
		}
		if testErr != nil {
			result.FailureReason = logging.Redact(testErr.Error())
			return finalizeFixtureResult(result)
		}
	}

	approvals, permissionCounts, byName, summaries, retries, _ := metrics.snapshot()
	result.ApprovalsRequired = approvals
	result.PermissionCounts = permissionCounts
	result.ToolCallsByName = byName
	result.ToolSummaries = summaries
	result.Retries = retries

	score, checks, hardFailures, failureReason, unexpected, err := EvaluateChecks(scoringContext{
		Fixture:     fixture,
		Workspace:   workspace,
		Changes:     changes,
		Diff:        diffSize,
		FinalAnswer: finalAnswer,
		ToolNames:   byName,
		Terminal:    string(terminal.Reason),
		TestResults: result.TestResults,
	})
	if err != nil {
		result.FailureReason = logging.Redact(err.Error())
		return finalizeFixtureResult(result)
	}
	result.Score = score
	result.Checks = checks
	result.UnexpectedFilesChanged = unexpected

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.Status = StatusTimeout
		result.TaskCompletionStatus = "timeout"
		if result.FailureReason == "" {
			result.FailureReason = "fixture timeout"
		}
	case errors.Is(ctx.Err(), context.Canceled):
		result.Status = StatusCanceled
		result.FailureReason = "fixture canceled"
	case len(hardFailures) > 0:
		result.Status = StatusFailed
		result.FailureReason = failureReason
	case score < fixture.Config.PassThreshold:
		result.Status = StatusFailed
		result.FailureReason = failureReason
	default:
		result.Status = StatusPassed
	}

	return finalizeFixtureResult(result)
}

func finalizeFixtureResult(result FixtureResult) FixtureResult {
	if result.FilesChanged == nil {
		result.FilesChanged = []FileChange{}
	}
	if result.UnexpectedFilesChanged == nil {
		result.UnexpectedFilesChanged = []string{}
	}
	if result.Checks == nil {
		result.Checks = []CheckResult{}
	}
	if result.TestResults == nil {
		result.TestResults = []TestResult{}
	}
	if result.ToolSummaries == nil {
		result.ToolSummaries = []ToolCallSummary{}
	}
	if result.ToolCallsByName == nil {
		result.ToolCallsByName = map[string]int{}
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	_ = WriteJSONReport(result.Artifacts.ResultJSON, result)
	return result
}
