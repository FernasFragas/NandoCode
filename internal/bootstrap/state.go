// Package bootstrap provides the infrastructure singleton for session-level configuration and runtime facts.
package bootstrap

import (
	"os"
	"sync"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/paths"
	"github.com/FernasFragas/nandocodego/internal/permissions"
)

// Initial contains initialization parameters for bootstrap state.
type Initial struct {
	WorkingDir                         string
	GitRoot                            string
	ConfigDir                          string
	DataDir                            string
	CacheDir                           string
	StateDir                           string
	SessionID                          string
	SessionDir                         string
	DefaultModel                       string
	OllamaBaseURL                      string
	LLMProvider                        string
	LLMBaseURL                         string
	OllamaCloudEnabled                 bool
	LLMStreamIdleTimeout               time.Duration
	CloudLLMStreamIdleTimeout          time.Duration
	KeepAlive                          string
	LogLevel                           string
	LogFormat                          string
	ContextMode                        string
	MemoryRecallMode                   string
	MaxTurns                           int
	MaxConcurrentTools                 int
	MaxOutputTokens                    int
	LengthRetryTokens                  int
	NumCtx                             int
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
	BashTimeout                        time.Duration
	SlowStageNoticeThreshold           time.Duration
	SlowStageNoticeThresholdSource     string
	PermissionMode                     permissions.Mode
	PermissionRules                    permissions.Rules
	TelemetryEnabled                   bool
	TelemetryEndpoint                  string
}

// Snapshot is a point-in-time copy of bootstrap state.
type Snapshot struct {
	WorkingDir                         string
	GitRoot                            string
	ConfigDir                          string
	DataDir                            string
	CacheDir                           string
	StateDir                           string
	SessionID                          string
	SessionDir                         string
	DefaultModel                       string
	OllamaBaseURL                      string
	LLMProvider                        string
	LLMBaseURL                         string
	OllamaCloudEnabled                 bool
	LLMStreamIdleTimeout               time.Duration
	CloudLLMStreamIdleTimeout          time.Duration
	KeepAlive                          string
	LogLevel                           string
	LogFormat                          string
	ContextMode                        string
	MemoryRecallMode                   string
	MaxTurns                           int
	MaxConcurrentTools                 int
	MaxOutputTokens                    int
	LengthRetryTokens                  int
	NumCtx                             int
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
	BashTimeout                        time.Duration
	SlowStageNoticeThreshold           time.Duration
	SlowStageNoticeThresholdSource     string
	PermissionMode                     permissions.Mode
	PermissionRules                    permissions.Rules
	TelemetryEnabled                   bool
	TelemetryEndpoint                  string
	CreatedAt                          time.Time
	UpdatedAt                          time.Time
}

// State is the thread-safe bootstrap singleton.
type State struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

var (
	globalState *State
	globalMu    sync.Mutex
)

// DefaultInitial creates an Initial struct with sensible defaults.
// If workingDir is empty, it falls back to os.Getwd().
// SessionID is generated as session-<unix-nano> if empty.
func DefaultInitial(workingDir string) Initial {
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			workingDir = "."
		}
	}

	sessionID := "session-" + time.Now().Format("20060102150405-000000000")

	return Initial{
		WorkingDir:                         workingDir,
		GitRoot:                            "",
		ConfigDir:                          paths.ConfigDir(),
		DataDir:                            paths.DataDir(),
		CacheDir:                           paths.CacheDir(),
		StateDir:                           paths.StateDir(),
		SessionID:                          sessionID,
		SessionDir:                         paths.SessionDir(sessionID),
		DefaultModel:                       llm.DefaultModel,
		OllamaBaseURL:                      "http://localhost:11434",
		LLMProvider:                        string(llm.ProviderOllamaLocal),
		LLMBaseURL:                         "http://localhost:11434",
		OllamaCloudEnabled:                 true,
		LLMStreamIdleTimeout:               90 * time.Second,
		CloudLLMStreamIdleTimeout:          5 * time.Minute,
		KeepAlive:                          "5m",
		LogLevel:                           "info",
		LogFormat:                          "text",
		ContextMode:                        "auto",
		MemoryRecallMode:                   "fast",
		MaxTurns:                           200,
		MaxConcurrentTools:                 10,
		MaxOutputTokens:                    8192,
		LengthRetryTokens:                  65536,
		NumCtx:                             0,
		MaxResultChars:                     8192,
		MaxReadChars:                       65536,
		MaxDirFiles:                        200,
		MaxPromptFiles:                     400,
		MaxDirBytes:                        512 * 1024,
		MaxPromptBytes:                     2 * 1024 * 1024,
		MaxDirDepth:                        8,
		MentionDirectorySource:             "auto",
		MentionIncludeGitignoredOnExplicit: true,
		PromptDumpMode:                     "off",
		PromptDumpKeep:                     10,
		PromptPreviewChars:                 600,
		BashTimeout:                        5 * time.Minute,
		SlowStageNoticeThreshold:           750 * time.Millisecond,
		SlowStageNoticeThresholdSource:     "default",
		PermissionMode:                     permissions.ModeDefault,
		PermissionRules:                    permissions.Rules{},
		TelemetryEnabled:                   false,
		TelemetryEndpoint:                  "",
	}
}

// New creates a new bootstrap State from an Initial struct.
// It normalizes the permission mode and defensively copies permission rules.
func New(initial Initial) *State {
	if initial.LLMProvider == "" {
		initial.LLMProvider = string(llm.ProviderOllamaLocal)
	}
	if initial.LLMBaseURL == "" {
		initial.LLMBaseURL = initial.OllamaBaseURL
	}

	// Normalize permission mode
	mode := initial.PermissionMode.Normalize()

	// Copy permission rules to avoid aliasing
	rulesCopy := copyRules(initial.PermissionRules)

	now := time.Now()
	snapshot := Snapshot{
		WorkingDir:                         initial.WorkingDir,
		GitRoot:                            initial.GitRoot,
		ConfigDir:                          initial.ConfigDir,
		DataDir:                            initial.DataDir,
		CacheDir:                           initial.CacheDir,
		StateDir:                           initial.StateDir,
		SessionID:                          initial.SessionID,
		SessionDir:                         initial.SessionDir,
		DefaultModel:                       initial.DefaultModel,
		OllamaBaseURL:                      initial.OllamaBaseURL,
		LLMProvider:                        initial.LLMProvider,
		LLMBaseURL:                         initial.LLMBaseURL,
		OllamaCloudEnabled:                 initial.OllamaCloudEnabled,
		LLMStreamIdleTimeout:               initial.LLMStreamIdleTimeout,
		CloudLLMStreamIdleTimeout:          initial.CloudLLMStreamIdleTimeout,
		KeepAlive:                          initial.KeepAlive,
		LogLevel:                           initial.LogLevel,
		LogFormat:                          initial.LogFormat,
		ContextMode:                        initial.ContextMode,
		MemoryRecallMode:                   initial.MemoryRecallMode,
		MaxTurns:                           initial.MaxTurns,
		MaxConcurrentTools:                 initial.MaxConcurrentTools,
		MaxOutputTokens:                    initial.MaxOutputTokens,
		LengthRetryTokens:                  initial.LengthRetryTokens,
		NumCtx:                             initial.NumCtx,
		MaxResultChars:                     initial.MaxResultChars,
		MaxReadChars:                       initial.MaxReadChars,
		MaxDirFiles:                        initial.MaxDirFiles,
		MaxPromptFiles:                     initial.MaxPromptFiles,
		MaxDirBytes:                        initial.MaxDirBytes,
		MaxPromptBytes:                     initial.MaxPromptBytes,
		MaxDirDepth:                        initial.MaxDirDepth,
		MentionDirectorySource:             initial.MentionDirectorySource,
		MentionIncludeGitignoredOnExplicit: initial.MentionIncludeGitignoredOnExplicit,
		PromptDumpMode:                     initial.PromptDumpMode,
		PromptDumpKeep:                     initial.PromptDumpKeep,
		PromptPreviewChars:                 initial.PromptPreviewChars,
		BashTimeout:                        initial.BashTimeout,
		SlowStageNoticeThreshold:           initial.SlowStageNoticeThreshold,
		SlowStageNoticeThresholdSource:     initial.SlowStageNoticeThresholdSource,
		PermissionMode:                     mode,
		PermissionRules:                    rulesCopy,
		TelemetryEnabled:                   initial.TelemetryEnabled,
		TelemetryEndpoint:                  initial.TelemetryEndpoint,
		CreatedAt:                          now,
		UpdatedAt:                          now,
	}

	return &State{snapshot: snapshot}
}

// Global returns the global bootstrap state singleton, initializing it with defaults if needed.
func Global() *State {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalState == nil {
		globalState = New(DefaultInitial(""))
	}
	return globalState
}

// InitGlobal initializes the global bootstrap state with the provided Initial struct.
// This should only be called once during application startup, before concurrent access.
func InitGlobal(initial Initial) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalState == nil {
		globalState = New(initial)
	}
}

// ResetGlobalForTest resets the global singleton for testing.
// This is only for test use and must be clearly marked as such.
// DO NOT USE IN PRODUCTION CODE.
func ResetGlobalForTest(initial Initial) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalState = New(initial)
}

// Snapshot returns a point-in-time copy of the bootstrap state.
// Slices and maps inside permission rules are deep-copied to prevent aliasing.
func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := s.snapshot
	snap.PermissionRules = copyRules(snap.PermissionRules)
	return snap
}

// Update calls the provided function with a mutable reference to the current snapshot,
// then normalizes permission mode and defensively copies rules before updating.
// The write lock is held during the update, and UpdatedAt is refreshed.
func (s *State) Update(f func(*Snapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f(&s.snapshot)

	// Normalize permission mode
	s.snapshot.PermissionMode = s.snapshot.PermissionMode.Normalize()

	// Defensively copy permission rules
	s.snapshot.PermissionRules = copyRules(s.snapshot.PermissionRules)

	// Update timestamp
	s.snapshot.UpdatedAt = time.Now()
}

// copyRules creates a deep copy of permission rules to avoid aliasing.
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
