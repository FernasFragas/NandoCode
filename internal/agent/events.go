// Package agent implements the Phase 4 model-driven agent loop.
package agent

import (
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// Event is a sealed interface for agent events.
type Event interface{ isEvent() }

// AssistantTurnStarted marks the beginning of a model turn.
type AssistantTurnStarted struct {
	Turn int
}

func (AssistantTurnStarted) isEvent() {}

// AssistantTextDelta contains streaming assistant content.
type AssistantTextDelta struct {
	Content string
}

func (AssistantTextDelta) isEvent() {}

// AssistantThinkingDelta contains streaming assistant thinking.
type AssistantThinkingDelta struct {
	Thinking string
}

func (AssistantThinkingDelta) isEvent() {}

// ToolUseStart signals the start of a tool execution.
type ToolUseStart struct {
	ID    string
	Name  string
	Input any
}

func (ToolUseStart) isEvent() {}

// ToolUseProgress forwards progress from a running tool.
type ToolUseProgress struct {
	ID   string
	Data any
}

func (ToolUseProgress) isEvent() {}

// ToolUseResult contains the result or error from a tool execution.
type ToolUseResult struct {
	ID     string
	Result tools.Result
	Err    error
}

func (ToolUseResult) isEvent() {}

// RetryNotice indicates the agent is retrying a failed operation.
type RetryNotice struct {
	Attempt        int
	Cause          string
	Kind           string
	DoneReason     string
	AssistantChars int
	ThinkingChars  int
}

func (RetryNotice) isEvent() {}

// HookNotice contains a non-blocking hook warning or status message.
type HookNotice struct {
	Message string
}

func (HookNotice) isEvent() {}

// LLMIdleWarning reports that the active model stream has been idle past the warning threshold.
type LLMIdleWarning struct {
	Provider string
	Timeout  time.Duration
}

func (LLMIdleWarning) isEvent() {}

// LLMRequestStarted signals that the model request is about to be sent.
type LLMRequestStarted struct{}

func (LLMRequestStarted) isEvent() {}

// LLMStreamOpened signals that the streaming connection was opened.
type LLMStreamOpened struct {
	Latency time.Duration
}

func (LLMStreamOpened) isEvent() {}

// FirstTokenReceived signals the first visible token/thinking/tool signal from the model.
type FirstTokenReceived struct {
	Latency time.Duration
}

func (FirstTokenReceived) isEvent() {}

// Terminal signals the end of the agent run.
type Terminal struct {
	Reason       TerminalReason
	Detail       string
	Usage        Usage
	Conversation []llm.Message
}

func (Terminal) isEvent() {}

// TerminalReason is a string-typed enum for terminal states.
type TerminalReason string

const (
	// TerminalCompleted means the agent finished normally.
	TerminalCompleted TerminalReason = "completed"
	// TerminalAborted means the context was canceled.
	TerminalAborted TerminalReason = "aborted"
	// TerminalMaxTurns means the turn budget was exhausted.
	TerminalMaxTurns TerminalReason = "max_turns"
	// TerminalContextOverflow means the context length was exceeded repeatedly.
	TerminalContextOverflow TerminalReason = "context_overflow"
	// TerminalStopHook means a hook requested termination (Phase 9+).
	TerminalStopHook TerminalReason = "stop_hook"
	// TerminalUnrecoverable means an unrecoverable error occurred.
	TerminalUnrecoverable TerminalReason = "unrecoverable"
)

// CompactionStarted signals that context compaction is beginning.
type CompactionStarted struct {
	TurnCount     int
	ContextTokens int64
}

func (CompactionStarted) isEvent() {}

// CompactionCompleted signals that compaction has finished (or was skipped/failed).
type CompactionCompleted struct {
	Result CompactionResult
}

func (CompactionCompleted) isEvent() {}

// StageTiming reports elapsed time for a named pre/post stage in the run pipeline.
type StageTiming struct {
	Stage    string
	Duration time.Duration
}

func (StageTiming) isEvent() {}

// PromptPackReport summarizes how prompt packing trimmed history to fit budget.
type PromptPackReport struct {
	InputBudgetTokens       int
	EstimatedIncluded       int
	EstimatedSkipped        int
	IncludedMessages        int
	SkippedMessages         int
	ForcedIncludeLast       bool
	DroppedRoles            map[string]int
	DroppedBytes            int
	DroppedMentionBlocks    int
	IncludedMentionBlocks   int
	LastUserMessageIncluded bool
	SystemMessageIncluded   bool
	HistoryPolicy           string
	Intent                  string
	AttachmentPolicy        string
	MemoryPolicy            string
	RetryPolicy             string
	IncludedFileBodies      int
	DirectoryTreeAttached   bool
}

func (PromptPackReport) isEvent() {}
