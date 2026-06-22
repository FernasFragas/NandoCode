package tools

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const defaultBashTimeout = 30 * time.Second

// PermissionMode is the caller's current permission policy.
type PermissionMode string

const (
	// PermissionDefault allows read-only calls and asks for writes.
	PermissionDefault PermissionMode = "default"
	// PermissionBypassPermissions allows all tool calls.
	PermissionBypassPermissions PermissionMode = "bypassPermissions"
	// PermissionPlan denies write operations.
	PermissionPlan PermissionMode = "plan"
	// PermissionDontAsk denies anything that would require a prompt.
	PermissionDontAsk PermissionMode = "dontAsk"
)

// Context carries execution state shared across tool calls.
type Context struct {
	Context               context.Context
	Logger                *slog.Logger
	WorkingDir            string
	AdditionalWorkingDirs []string
	Env                   []string
	BashTimeout           time.Duration
	MaxResultChars        int
	MaxReadChars          int
	MaxDirFiles           int
	MaxPromptFiles        int
	MaxDirBytes           int64
	MaxPromptBytes        int64
	MaxDirDepth           int
	MentionDirectorySource string
	MentionIncludeGitignoredOnExplicit bool
	PromptDumpMode         string
	PromptDumpKeep         int
	PromptPreviewChars     int
	PermissionMode        PermissionMode
	IsSubagent            bool
	RecordFileSnapshot    func(path string, content []byte)
	ReadFileSnapshot      func(path string) ([]byte, bool)
	RecordFileRangeSnapshot func(path string, startLine, lineCount int, mtimeUnixNano int64, content []byte)
	ReadFileRangeSnapshot   func(path string, startLine, lineCount int, mtimeUnixNano int64) ([]byte, bool)
	AllowLocalFetch       bool
	TodoList              any // *todo.TodoList; any to avoid import cycle
}

// DefaultContext builds a conservative tool context for a working directory.
func DefaultContext(ctx context.Context, workingDir string) Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if workingDir == "" {
		if wd, err := os.Getwd(); err == nil {
			workingDir = wd
		}
	}
	if abs, err := filepath.Abs(workingDir); err == nil {
		workingDir = abs
	}
	return Context{
		Context:        ctx,
		Logger:         slog.Default(),
		WorkingDir:     workingDir,
		Env:            os.Environ(),
		BashTimeout:    defaultBashTimeout,
		MaxResultChars: defaultMaxResultChars,
		MaxReadChars:   defaultMaxReadChars,
		MaxDirFiles:    defaultMaxDirFiles,
		MaxPromptFiles: defaultMaxPromptFiles,
		MaxDirBytes:    defaultMaxDirBytes,
		MaxPromptBytes: defaultMaxPromptBytes,
		MaxDirDepth:    defaultMaxDirDepth,
		MentionDirectorySource: "auto",
		MentionIncludeGitignoredOnExplicit: true,
		PromptDumpMode: "off",
		PromptDumpKeep: 10,
		PromptPreviewChars: 600,
		PermissionMode: PermissionDefault,
	}
}

// EffectiveContext returns a non-nil context.
func (c Context) EffectiveContext() context.Context {
	if c.Context == nil {
		return context.Background()
	}
	return c.Context
}

// EffectiveBashTimeout returns a bounded default timeout.
func (c Context) EffectiveBashTimeout() time.Duration {
	if c.BashTimeout <= 0 {
		return defaultBashTimeout
	}
	return c.BashTimeout
}

// EffectiveMaxResultChars returns the model-facing result limit.
func (c Context) EffectiveMaxResultChars() int {
	if c.MaxResultChars <= 0 {
		return defaultMaxResultChars
	}
	return c.MaxResultChars
}

// EffectiveMaxReadChars returns the file-read limit.
func (c Context) EffectiveMaxReadChars() int {
	if c.MaxReadChars <= 0 {
		return defaultMaxReadChars
	}
	return c.MaxReadChars
}

// EffectiveMaxDirFiles returns the per-directory file expansion cap.
func (c Context) EffectiveMaxDirFiles() int {
	if c.MaxDirFiles <= 0 {
		return defaultMaxDirFiles
	}
	return c.MaxDirFiles
}

// EffectiveMaxPromptFiles returns the per-prompt file expansion cap.
func (c Context) EffectiveMaxPromptFiles() int {
	if c.MaxPromptFiles <= 0 {
		return defaultMaxPromptFiles
	}
	return c.MaxPromptFiles
}

// EffectiveMaxDirBytes returns the per-directory byte expansion cap.
func (c Context) EffectiveMaxDirBytes() int64 {
	if c.MaxDirBytes <= 0 {
		return defaultMaxDirBytes
	}
	return c.MaxDirBytes
}

// EffectiveMaxPromptBytes returns the per-prompt byte expansion cap.
func (c Context) EffectiveMaxPromptBytes() int64 {
	if c.MaxPromptBytes <= 0 {
		return defaultMaxPromptBytes
	}
	return c.MaxPromptBytes
}

// EffectiveMaxDirDepth returns the maximum directory traversal depth.
func (c Context) EffectiveMaxDirDepth() int {
	if c.MaxDirDepth <= 0 {
		return defaultMaxDirDepth
	}
	return c.MaxDirDepth
}
