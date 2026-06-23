package state

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/types"
)

// fileSnapshotStore is a session-scoped, mutex-protected map of file content snapshots.
type fileSnapshotStore struct {
	mu         sync.RWMutex
	snaps      map[string][]byte
	rangeSnaps map[string][]byte
}

func (s *fileSnapshotStore) record(path string, content []byte) {
	s.mu.Lock()
	s.snaps[path] = append([]byte(nil), content...)
	s.mu.Unlock()
}

func (s *fileSnapshotStore) read(path string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.snaps[path]
	return b, ok
}

func rangeSnapshotKey(path string, startLine, lineCount int, mtimeUnixNano int64) string {
	return path + "|" + strconv.Itoa(startLine) + "|" + strconv.Itoa(lineCount) + "|" + strconv.FormatInt(mtimeUnixNano, 10)
}

func (s *fileSnapshotStore) recordRange(path string, startLine, lineCount int, mtimeUnixNano int64, content []byte) {
	s.mu.Lock()
	if s.rangeSnaps == nil {
		s.rangeSnaps = make(map[string][]byte)
	}
	s.rangeSnaps[rangeSnapshotKey(path, startLine, lineCount, mtimeUnixNano)] = append([]byte(nil), content...)
	s.mu.Unlock()
}

func (s *fileSnapshotStore) readRange(path string, startLine, lineCount int, mtimeUnixNano int64) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.rangeSnaps[rangeSnapshotKey(path, startLine, lineCount, mtimeUnixNano)]
	return b, ok
}

// VimMode represents the current Vim editing mode.
type VimMode string

const (
	VimInsert VimMode = "insert"
	VimNormal VimMode = "normal"
	VimVisual VimMode = "visual"
)

// ToolSettings contains tool execution parameters.
type ToolSettings struct {
	WorkingDir                         string
	AdditionalWorkingDirs              []string
	Env                                []string
	BashTimeout                        time.Duration
	MaxResultChars                     int
	MaxReadChars                       int
	MaxDirFiles                        int
	MaxPromptFiles                     int
	MaxDirBytes                        int64
	MaxPromptBytes                     int64
	MaxDirDepth                        int
	MentionDirectorySource             string
	MentionIncludeGitignoredOnExplicit bool
	PromptDumpMode                     string
	PromptDumpKeep                     int
	PromptPreviewChars                 int
}

// ToolUse represents an active or completed tool invocation.
type ToolUse struct {
	ID        string
	Name      string
	Summary   string
	StartedAt time.Time
	Done      bool
	Error     string
}

// PermissionPrompt represents an active permission modal or prompt.
type PermissionPrompt struct {
	ID       string
	ToolName string
	Target   string
	Reason   string
}

// App represents the complete UI/session state for the REPL, TUI, or other front-end.
type App struct {
	Messages                       []llm.Message
	QueuedPrompts                  []string
	InputBuffer                    string
	VimMode                        VimMode
	ActiveModel                    string
	LLMProvider                    string
	LLMBaseURL                     string
	ContextMode                    string
	MemoryRecallMode               string
	MaxOutputTokens                int // dynamic; 0 means use agent.Config default
	RuntimeNumCtx                  int // effective num_ctx for this session's model/runtime policy
	ToolSettings                   ToolSettings
	SlowStageNoticeThreshold       time.Duration
	SlowStageNoticeThresholdSource string
	PermissionMode                 permissions.Mode
	PermissionRules                permissions.Rules
	PermissionPrompt               *PermissionPrompt
	CoordinatorMode                bool
	WorkerCount                    int
	ActiveRun                      bool
	ActiveTools                    map[string]ToolUse
	Tasks                          map[string]types.TaskSummary
	LastRetryNotice                string
	TerminalReason                 agent.TerminalReason
	TerminalDetail                 string
	Usage                          agent.Usage
	fileSnapshots                  *fileSnapshotStore // session-scoped; pointer shared across clones
	TodoList                       any                // *todo.TodoList; any to avoid import cycle
}

// DefaultApp creates an App initialized from a bootstrap snapshot.
func DefaultApp(snap bootstrap.Snapshot) App {
	snaps := &fileSnapshotStore{snaps: make(map[string][]byte), rangeSnaps: make(map[string][]byte)}
	provider := snap.LLMProvider
	if provider == "" {
		provider = string(llm.ProviderOllamaLocal)
	}
	baseURL := snap.LLMBaseURL
	if baseURL == "" {
		baseURL = snap.OllamaBaseURL
	}
	return App{
		fileSnapshots:    snaps,
		Messages:         []llm.Message{},
		QueuedPrompts:    []string{},
		InputBuffer:      "",
		VimMode:          VimInsert,
		ActiveModel:      snap.DefaultModel,
		LLMProvider:      provider,
		LLMBaseURL:       baseURL,
		ContextMode:      snap.ContextMode,
		MemoryRecallMode: snap.MemoryRecallMode,
		ToolSettings: ToolSettings{
			WorkingDir:                         snap.WorkingDir,
			AdditionalWorkingDirs:              []string{},
			Env:                                []string{},
			BashTimeout:                        snap.BashTimeout,
			MaxResultChars:                     snap.MaxResultChars,
			MaxReadChars:                       snap.MaxReadChars,
			MaxDirFiles:                        snap.MaxDirFiles,
			MaxPromptFiles:                     snap.MaxPromptFiles,
			MaxDirBytes:                        snap.MaxDirBytes,
			MaxPromptBytes:                     snap.MaxPromptBytes,
			MaxDirDepth:                        snap.MaxDirDepth,
			MentionDirectorySource:             snap.MentionDirectorySource,
			MentionIncludeGitignoredOnExplicit: snap.MentionIncludeGitignoredOnExplicit,
			PromptDumpMode:                     snap.PromptDumpMode,
			PromptDumpKeep:                     snap.PromptDumpKeep,
			PromptPreviewChars:                 snap.PromptPreviewChars,
		},
		SlowStageNoticeThreshold:       snap.SlowStageNoticeThreshold,
		SlowStageNoticeThresholdSource: snap.SlowStageNoticeThresholdSource,
		PermissionMode:                 snap.PermissionMode,
		PermissionRules:                copyRules(snap.PermissionRules),
		PermissionPrompt:               nil,
		CoordinatorMode:                false,
		WorkerCount:                    0,
		ActiveRun:                      false,
		ActiveTools:                    make(map[string]ToolUse),
		Tasks:                          make(map[string]types.TaskSummary),
		LastRetryNotice:                "",
		TerminalReason:                 "",
		TerminalDetail:                 "",
		Usage: agent.Usage{
			PromptEvalCount: 0,
			EvalCount:       0,
			TotalDuration:   0,
			Turns:           0,
			ToolCalls:       0,
		},
	}
}

// Clone creates a deep copy of the App state, including all slices, maps, and pointer fields.
func (a App) Clone() App {
	cloned := App{
		Messages:         append([]llm.Message{}, a.Messages...),
		QueuedPrompts:    append([]string{}, a.QueuedPrompts...),
		InputBuffer:      a.InputBuffer,
		VimMode:          a.VimMode,
		ActiveModel:      a.ActiveModel,
		LLMProvider:      a.LLMProvider,
		LLMBaseURL:       a.LLMBaseURL,
		ContextMode:      a.ContextMode,
		MemoryRecallMode: a.MemoryRecallMode,
		MaxOutputTokens:  a.MaxOutputTokens,
		RuntimeNumCtx:    a.RuntimeNumCtx,
		ToolSettings: ToolSettings{
			WorkingDir:                         a.ToolSettings.WorkingDir,
			AdditionalWorkingDirs:              append([]string{}, a.ToolSettings.AdditionalWorkingDirs...),
			Env:                                append([]string{}, a.ToolSettings.Env...),
			BashTimeout:                        a.ToolSettings.BashTimeout,
			MaxResultChars:                     a.ToolSettings.MaxResultChars,
			MaxReadChars:                       a.ToolSettings.MaxReadChars,
			MaxDirFiles:                        a.ToolSettings.MaxDirFiles,
			MaxPromptFiles:                     a.ToolSettings.MaxPromptFiles,
			MaxDirBytes:                        a.ToolSettings.MaxDirBytes,
			MaxPromptBytes:                     a.ToolSettings.MaxPromptBytes,
			MaxDirDepth:                        a.ToolSettings.MaxDirDepth,
			MentionDirectorySource:             a.ToolSettings.MentionDirectorySource,
			MentionIncludeGitignoredOnExplicit: a.ToolSettings.MentionIncludeGitignoredOnExplicit,
			PromptDumpMode:                     a.ToolSettings.PromptDumpMode,
			PromptDumpKeep:                     a.ToolSettings.PromptDumpKeep,
			PromptPreviewChars:                 a.ToolSettings.PromptPreviewChars,
		},
		SlowStageNoticeThreshold:       a.SlowStageNoticeThreshold,
		SlowStageNoticeThresholdSource: a.SlowStageNoticeThresholdSource,
		PermissionMode:                 a.PermissionMode,
		PermissionRules:                copyRules(a.PermissionRules),
		PermissionPrompt:               nil,
		CoordinatorMode:                a.CoordinatorMode,
		WorkerCount:                    a.WorkerCount,
		ActiveRun:                      a.ActiveRun,
		ActiveTools:                    make(map[string]ToolUse),
		Tasks:                          make(map[string]types.TaskSummary, len(a.Tasks)),
		LastRetryNotice:                a.LastRetryNotice,
		TerminalReason:                 a.TerminalReason,
		TerminalDetail:                 a.TerminalDetail,
		Usage:                          a.Usage,
		fileSnapshots:                  a.fileSnapshots, // shared pointer — intentional
		TodoList:                       a.TodoList,
	}

	// Deep copy permission prompt if present
	if a.PermissionPrompt != nil {
		perm := *a.PermissionPrompt
		cloned.PermissionPrompt = &perm
	}

	// Deep copy active tools map
	for k, v := range a.ActiveTools {
		cloned.ActiveTools[k] = v
	}
	for k, v := range a.Tasks {
		cloned.Tasks[k] = v
	}

	return cloned
}

// ToolContext builds a fresh tools.Context for agent execution using app settings.
// This method does not store the context or logger inside app state, only uses them.
func (a App) ToolContext(ctx context.Context) tools.Context {
	tc := tools.Context{
		Context:                            ctx,
		Logger:                             nil, // Will be set by the caller
		WorkingDir:                         a.ToolSettings.WorkingDir,
		AdditionalWorkingDirs:              a.ToolSettings.AdditionalWorkingDirs,
		Env:                                a.ToolSettings.Env,
		BashTimeout:                        a.ToolSettings.BashTimeout,
		MaxResultChars:                     a.ToolSettings.MaxResultChars,
		MaxReadChars:                       a.ToolSettings.MaxReadChars,
		MaxDirFiles:                        a.ToolSettings.MaxDirFiles,
		MaxPromptFiles:                     a.ToolSettings.MaxPromptFiles,
		MaxDirBytes:                        a.ToolSettings.MaxDirBytes,
		MaxPromptBytes:                     a.ToolSettings.MaxPromptBytes,
		MaxDirDepth:                        a.ToolSettings.MaxDirDepth,
		MentionDirectorySource:             a.ToolSettings.MentionDirectorySource,
		MentionIncludeGitignoredOnExplicit: a.ToolSettings.MentionIncludeGitignoredOnExplicit,
		PromptDumpMode:                     a.ToolSettings.PromptDumpMode,
		PromptDumpKeep:                     a.ToolSettings.PromptDumpKeep,
		PromptPreviewChars:                 a.ToolSettings.PromptPreviewChars,
		PermissionMode:                     permissions.ToToolsMode(a.PermissionMode),
		TodoList:                           a.TodoList,
	}
	if a.fileSnapshots != nil {
		snaps := a.fileSnapshots
		tc.RecordFileSnapshot = func(path string, content []byte) {
			snaps.record(path, content)
		}
		tc.ReadFileSnapshot = func(path string) ([]byte, bool) {
			return snaps.read(path)
		}
		tc.RecordFileRangeSnapshot = func(path string, startLine, lineCount int, mtimeUnixNano int64, content []byte) {
			snaps.recordRange(path, startLine, lineCount, mtimeUnixNano, content)
		}
		tc.ReadFileRangeSnapshot = func(path string, startLine, lineCount int, mtimeUnixNano int64) ([]byte, bool) {
			return snaps.readRange(path, startLine, lineCount, mtimeUnixNano)
		}
	}
	return tc
}

// copyRules creates a deep copy of permission rules to prevent aliasing.
func copyRules(rules permissions.Rules) permissions.Rules {
	if rules.Empty() {
		return permissions.Rules{}
	}

	return permissions.Rules{
		AlwaysAllow: append([]permissions.Rule{}, rules.AlwaysAllow...),
		AlwaysDeny:  append([]permissions.Rule{}, rules.AlwaysDeny...),
		AlwaysAsk:   append([]permissions.Rule{}, rules.AlwaysAsk...),
	}
}
