package eval

import (
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
)

const SchemaVersion = 1

type Provider string

const (
	ProviderLive     Provider = "live"
	ProviderRecorded Provider = "recorded"
)

type Status string

const (
	StatusPassed   Status = "passed"
	StatusFailed   Status = "failed"
	StatusError    Status = "error"
	StatusTimeout  Status = "timeout"
	StatusCanceled Status = "canceled"
	StatusSkipped  Status = "skipped"
)

type ApprovalStrategy string

const (
	ApprovalStrategyAllow       ApprovalStrategy = "allow"
	ApprovalStrategyDeny        ApprovalStrategy = "deny"
	ApprovalStrategyScripted    ApprovalStrategy = "scripted"
	ApprovalStrategyUnavailable ApprovalStrategy = "unavailable"
)

type CompareMode string

const (
	CompareExact CompareMode = "exact"
)

type Fixture struct {
	ID            string        `json:"id"`
	Root          string        `json:"root"`
	Task          string        `json:"task"`
	TaskPath      string        `json:"task_path"`
	RepoDir       string        `json:"repo_dir"`
	ExpectedDir   string        `json:"expected_dir"`
	RecordingsDir string        `json:"recordings_dir,omitempty"`
	Config        ScoringConfig `json:"config"`
}

type RunOptions struct {
	Provider       Provider      `json:"provider"`
	Model          string        `json:"model,omitempty"`
	OllamaURL      string        `json:"ollama_url,omitempty"`
	Recording      string        `json:"recording,omitempty"`
	OutputDir      string        `json:"output_dir,omitempty"`
	KeepWorkspaces bool          `json:"keep_workspaces,omitempty"`
	Jobs           int           `json:"jobs,omitempty"`
	Repeat         int           `json:"repeat,omitempty"`
	Timeout        time.Duration `json:"timeout,omitempty"`
	FailUnder      float64       `json:"fail_under,omitempty"`
}

type ScoringConfig struct {
	Version       int             `json:"version" yaml:"version"`
	ID            string          `json:"id" yaml:"id"`
	Description   string          `json:"description" yaml:"description"`
	Tags          []string        `json:"tags,omitempty" yaml:"tags"`
	Execution     ExecutionConfig `json:"execution" yaml:"execution"`
	Model         ModelConfig     `json:"model" yaml:"model"`
	Tests         []TestConfig    `json:"tests,omitempty" yaml:"tests"`
	Workspace     WorkspaceConfig `json:"workspace" yaml:"workspace"`
	Golden        GoldenConfig    `json:"golden" yaml:"golden"`
	Checks        ChecksConfig    `json:"checks" yaml:"checks"`
	Weights       WeightsConfig   `json:"weights" yaml:"weights"`
	PassThreshold float64         `json:"pass_threshold" yaml:"pass_threshold"`
}

type ExecutionConfig struct {
	Timeout          time.Duration     `json:"timeout,omitempty" yaml:"timeout"`
	MaxTurns         int               `json:"max_turns,omitempty" yaml:"max_turns"`
	MaxToolCalls     int               `json:"max_tool_calls,omitempty" yaml:"max_tool_calls"`
	PermissionMode   string            `json:"permission_mode,omitempty" yaml:"permission_mode"`
	ApprovalStrategy ApprovalStrategy  `json:"approval_strategy,omitempty" yaml:"approval_strategy"`
	Environment      map[string]string `json:"environment,omitempty" yaml:"environment"`
}

type ModelConfig struct {
	Live     LiveModelConfig     `json:"live" yaml:"live"`
	Recorded RecordedModelConfig `json:"recorded" yaml:"recorded"`
}

type LiveModelConfig struct {
	Temperature float64 `json:"temperature,omitempty" yaml:"temperature"`
}

type RecordedModelConfig struct {
	Recording string `json:"recording,omitempty" yaml:"recording"`
}

type TestConfig struct {
	Name     string        `json:"name" yaml:"name"`
	Command  []string      `json:"command" yaml:"command"`
	Timeout  time.Duration `json:"timeout,omitempty" yaml:"timeout"`
	Required bool          `json:"required,omitempty" yaml:"required"`
}

type WorkspaceConfig struct {
	AllowedChanges         []string `json:"allowed_changes,omitempty" yaml:"allowed_changes"`
	RequiredChanges        []string `json:"required_changes,omitempty" yaml:"required_changes"`
	ForbiddenChanges       []string `json:"forbidden_changes,omitempty" yaml:"forbidden_changes"`
	MustExist              []string `json:"must_exist,omitempty" yaml:"must_exist"`
	MustNotExist           []string `json:"must_not_exist,omitempty" yaml:"must_not_exist"`
	MaxChangedFiles        int      `json:"max_changed_files,omitempty" yaml:"max_changed_files"`
	MaxDiffLines           int      `json:"max_diff_lines,omitempty" yaml:"max_diff_lines"`
	AllowUnexpectedChanges bool     `json:"allow_unexpected_changes,omitempty" yaml:"allow_unexpected_changes"`
}

type GoldenConfig struct {
	Compare CompareMode `json:"compare,omitempty" yaml:"compare"`
	Files   []string    `json:"files,omitempty" yaml:"files"`
}

type ChecksConfig struct {
	FinalAnswerContains []string `json:"final_answer_contains,omitempty" yaml:"final_answer_contains"`
	RequiredTools       []string `json:"required_tools,omitempty" yaml:"required_tools"`
	ForbiddenTools      []string `json:"forbidden_tools,omitempty" yaml:"forbidden_tools"`
}

type WeightsConfig struct {
	TaskCompletion float64 `json:"task_completion" yaml:"task_completion"`
	Tests          float64 `json:"tests" yaml:"tests"`
	ExpectedFiles  float64 `json:"expected_files" yaml:"expected_files"`
	ChangeScope    float64 `json:"change_scope" yaml:"change_scope"`
	FinalAnswer    float64 `json:"final_answer" yaml:"final_answer"`
}

type Recording struct {
	Version int            `json:"version"`
	Model   string         `json:"model"`
	Strict  bool           `json:"strict"`
	Turns   []RecordedTurn `json:"turns"`
}

type RecordedTurn struct {
	Match  RecordedMatch     `json:"match"`
	Events []llm.StreamEvent `json:"events"`
}

type RecordedMatch struct {
	LatestUserContains string   `json:"latest_user_contains"`
	RequiredTools      []string `json:"required_tools"`
}

type Diagnostic struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ChangeType string

const (
	ChangeAdded      ChangeType = "added"
	ChangeModified   ChangeType = "modified"
	ChangeDeleted    ChangeType = "deleted"
	ChangeModeChange ChangeType = "mode_changed"
)

type FileChange struct {
	Path   string     `json:"path"`
	Change ChangeType `json:"change"`
}

type DiffSize struct {
	Files        int `json:"files"`
	LinesAdded   int `json:"lines_added"`
	LinesDeleted int `json:"lines_deleted"`
	BytesAdded   int `json:"bytes_added"`
	BytesDeleted int `json:"bytes_deleted"`
	BinaryFiles  int `json:"binary_files"`
}

type CheckResult struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Score   float64 `json:"score,omitempty"`
	Details string  `json:"details,omitempty"`
}

type ArtifactPaths struct {
	ResultJSON      string            `json:"result_json,omitempty"`
	FinalAnswer     string            `json:"final_answer,omitempty"`
	DiffPatch       string            `json:"diff_patch,omitempty"`
	MarkdownReport  string            `json:"markdown_report,omitempty"`
	Workspace       string            `json:"workspace,omitempty"`
	TestStdoutPaths map[string]string `json:"test_stdout_paths,omitempty"`
	TestStderrPaths map[string]string `json:"test_stderr_paths,omitempty"`
}

type TestResult struct {
	Name       string   `json:"name"`
	Command    []string `json:"command"`
	Status     string   `json:"status"`
	ExitCode   int      `json:"exit_code"`
	DurationMS int64    `json:"duration_ms"`
	StdoutPath string   `json:"stdout_path,omitempty"`
	StderrPath string   `json:"stderr_path,omitempty"`
	Required   bool     `json:"required"`
	Failure    string   `json:"failure,omitempty"`
}

type ToolCallSummary struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type PermissionCounts struct {
	Allowed int `json:"allowed"`
	Denied  int `json:"denied"`
	Asked   int `json:"asked"`
}

type FixtureResult struct {
	SchemaVersion          int               `json:"schema_version"`
	RunID                  string            `json:"run_id"`
	FixtureID              string            `json:"fixture_id"`
	Repetition             int               `json:"repetition,omitempty"`
	Description            string            `json:"description,omitempty"`
	Status                 Status            `json:"status"`
	Score                  float64           `json:"score"`
	TaskCompletionStatus   string            `json:"task_completion_status"`
	TestsPassed            int               `json:"tests_passed"`
	TestsFailed            int               `json:"tests_failed"`
	FilesChanged           []FileChange      `json:"files_changed"`
	UnexpectedFilesChanged []string          `json:"unexpected_files_changed"`
	ToolCalls              int               `json:"tool_calls"`
	ApprovalsRequired      int               `json:"approvals_required"`
	RuntimeMS              int64             `json:"runtime_ms"`
	ModelUsed              string            `json:"model_used"`
	Provider               Provider          `json:"provider"`
	Retries                int               `json:"retries"`
	FinalDiffSize          DiffSize          `json:"final_diff_size"`
	FailureReason          string            `json:"failure_reason"`
	Checks                 []CheckResult     `json:"checks,omitempty"`
	TestResults            []TestResult      `json:"test_results,omitempty"`
	Artifacts              ArtifactPaths     `json:"artifacts,omitempty"`
	ToolCallsByName        map[string]int    `json:"tool_calls_by_name,omitempty"`
	PermissionCounts       PermissionCounts  `json:"permission_counts,omitempty"`
	ToolSummaries          []ToolCallSummary `json:"tool_summaries,omitempty"`
	TerminalReason         string            `json:"terminal_reason,omitempty"`
	Usage                  agent.Usage       `json:"usage,omitempty"`
	StartedAt              string            `json:"started_at,omitempty"`
	FinishedAt             string            `json:"finished_at,omitempty"`
}

type AggregateCounts struct {
	Fixtures int `json:"fixtures"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Errors   int `json:"errors"`
	Skipped  int `json:"skipped"`
}

type RunReport struct {
	SchemaVersion  int             `json:"schema_version"`
	RunID          string          `json:"run_id"`
	Provider       Provider        `json:"provider"`
	Model          string          `json:"model,omitempty"`
	Recording      string          `json:"recording,omitempty"`
	StartedAt      string          `json:"started_at"`
	FinishedAt     string          `json:"finished_at"`
	AggregateScore float64         `json:"aggregate_score"`
	Counts         AggregateCounts `json:"counts"`
	Results        []FixtureResult `json:"results"`
	Artifacts      ArtifactPaths   `json:"artifacts,omitempty"`
}
