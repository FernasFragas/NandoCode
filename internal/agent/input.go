package agent

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// Input defines the parameters for a single agent run.
type Input struct {
	Model            string
	LLMProvider      string
	SystemPrompt     string
	Messages         []llm.Message
	ToolContext      tools.Context
	ContextMode      string
	PromptIntent     string
	AttachmentPolicy string
	OriginalUserText string
	HistoryPolicy    string
	ToolsetName      string
	ToolMode         string
	RouteAction      string
	RouteReason      string
	RouteProfile     string
	EvidencePack     *EvidencePackReport

	PermissionMode   permissions.Mode
	PermissionRules  permissions.Rules
	PermissionPrompt permissions.PromptFunc
	HookDecision     permissions.HookDecisionFunc
	PostToolUse      ToolHookFunc
	PermissionDenied ToolHookFunc
	StopHook         StopHookFunc

	// Phase 11: sub-agent context.
	IsSubagent  bool
	ParentAbort <-chan struct{}
	OutputSink  io.Writer

	// CompactRequest signals a manual compaction request from the TUI.
	// Use a buffered channel of size 1 so the sender never blocks.
	CompactRequest <-chan struct{}

	// MaxOutputTokens overrides Config.MaxOutputTokens for this run when > 0.
	// Set this from the live model limits so each run uses the model's actual capability.
	MaxOutputTokens int

	// PendingMessagesProvider returns user-role messages to inject between completed
	// turns/tool rounds (for mailbox/task notifications). It is never consulted mid-stream.
	PendingMessagesProvider func(context.Context) []llm.Message
}

type toolModeContextKey struct{}

const (
	HistoryPolicyDefault    = "default"
	HistoryPolicyLatestOnly = "latest_only"

	ToolsetDefault  = "default"
	ToolsetReadOnly = "read_only"

	ToolModeDefault = "default"
	ToolModeNone    = "none"
)

type ToolHookFunc func(context.Context, ToolHookEvent)

type StopHookFunc func(context.Context, []llm.Message) (string, bool)
type ToolBatchObserverFunc func(batchSize int, safe bool, duration time.Duration)

type EvidencePackReport struct {
	OriginalRequestBytes   int
	BudgetTokens           int
	EstimatedTokens        int
	FilesReferenced        int
	FilesRaw               int
	FilesExcerpted         int
	FilesSummarized        int
	FilesOmitted           int
	DirectoriesReferenced  int
	DirectoryTreesIncluded int
	RawBytesIncluded       int
	RawBytesOmitted        int
	AnchorAdded            bool
	Packed                 bool
	Omitted                []OmittedEvidence
	LargestOmitted         []OmittedEvidence
	IncludedRanges         []EvidenceRangeReport
}

type EvidenceRangeReport struct {
	Path      string
	Kind      string
	StartLine int
	EndLine   int
	Bytes     int
	Reason    string
}

type OmittedEvidence struct {
	Path         string
	Kind         string
	Reason       string
	BytesOmitted int
}

type ToolHookEvent struct {
	ToolName       string
	Input          any
	Target         string
	Result         tools.Result
	Err            error
	ToolContext    tools.Context
	PermissionMode permissions.Mode
	Model          string
}

// Config defines behavior and budget parameters for the agent.
type Config struct {
	MaxTurns           int
	MaxOutputTokens    int
	LengthRetryTokens  int
	ChatKeepAlive      string
	NumCtx             int
	MaxConcurrentTools int
	PermissionObserver permissions.ObserverFunc
	ToolBatchObserver  ToolBatchObserverFunc
	Watchdog           llm.WatchdogConfig
	CloudWatchdog      llm.WatchdogConfig
	Compaction         CompactionConfig
	ContextMode        string
	ContextMinNumCtx   int
	ContextMaxNumCtx   int
	ContextReserve     int
}

// DefaultConfig returns the standard Phase 4 agent configuration.
func DefaultConfig() Config {
	return Config{
		MaxTurns:           200,
		MaxOutputTokens:    8192,
		LengthRetryTokens:  65536,
		NumCtx:             32768,
		MaxConcurrentTools: 10,
		Watchdog:           llm.DefaultWatchdogConfig(),
		CloudWatchdog:      llm.DefaultCloudWatchdogConfig(),
		Compaction:         DefaultCompactionConfig(),
		ContextMode:        "auto",
		ContextMinNumCtx:   8192,
		ContextMaxNumCtx:   0,
		ContextReserve:     4096,
	}
}

func (c Config) watchdogForProvider(provider string) llm.WatchdogConfig {
	if provider == string(llm.ProviderOllamaCloudAPI) && c.CloudWatchdog.IdleTimeout > 0 {
		return c.CloudWatchdog
	}
	if c.Watchdog.IdleTimeout > 0 {
		return c.Watchdog
	}
	return llm.DefaultWatchdogConfig()
}

// validateInput checks required fields and applies defaults to ToolContext and permissions.
func validateInput(ctx context.Context, in *Input) error {
	if in.Model == "" {
		return errors.New("input.Model is required")
	}
	if in.ToolContext.Context == nil {
		in.ToolContext.Context = ctx
	}
	if in.ToolContext.WorkingDir == "" {
		if wd, err := os.Getwd(); err == nil {
			in.ToolContext.WorkingDir = wd
		}
	}
	if in.ToolContext.Logger == nil {
		in.ToolContext.Logger = defaultLogger()
	}
	if in.ToolsetName == "" {
		in.ToolsetName = ToolsetDefault
	}
	switch strings.ToLower(strings.TrimSpace(in.ToolMode)) {
	case "", ToolModeDefault:
		in.ToolMode = ToolModeDefault
	case ToolModeNone:
		in.ToolMode = ToolModeNone
	default:
		in.ToolMode = ToolModeDefault
	}
	if in.ToolContext.Context != nil {
		in.ToolContext.Context = context.WithValue(in.ToolContext.Context, toolModeContextKey{}, in.ToolMode)
	}

	// Apply permission defaults.
	if in.PermissionMode == "" {
		// If no explicit mode, derive from tools.Context (Phase 4 compatibility).
		in.PermissionMode = permissions.FromToolsMode(in.ToolContext.PermissionMode)
	}
	if in.PermissionMode == "" {
		// Still empty after conversion, default to ModeDefault.
		in.PermissionMode = permissions.ModeDefault
	} else {
		// Normalize the mode.
		in.PermissionMode = in.PermissionMode.Normalize()
	}

	return nil
}

func toolModeFromContext(ctx context.Context) string {
	if ctx == nil {
		return ToolModeDefault
	}
	raw := ctx.Value(toolModeContextKey{})
	mode, _ := raw.(string)
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ToolModeNone:
		return ToolModeNone
	default:
		return ToolModeDefault
	}
}
