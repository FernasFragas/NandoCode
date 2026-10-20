// Package applyconfig holds the single config-to-bootstrap mapping shared by
// every surface. It lives apart from package bootstrap because internal/config
// imports internal/semantic, whose test build reaches bootstrap through
// agent; bootstrap importing config would close an import cycle.
package applyconfig

import (
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/config"
)

// ApplyConfig copies every config.Config field that belongs in bootstrap
// state into i. slowStageSource is config.ConfigSources.SlowStageNoticeThreshold.
// MemoryEnabled, SkillsDir, ProjectSkillsDir and SemanticIndex are read from
// config directly and are not part of Initial. Call bootstrap.ApplyOverrides
// afterwards so command-line flags win.
func ApplyConfig(i *bootstrap.Initial, c config.Config, slowStageSource string) {
	i.DefaultModel = c.DefaultModel
	i.OllamaBaseURL = c.OllamaBaseURL
	i.LLMBaseURL = c.OllamaBaseURL
	i.OllamaCloudEnabled = c.OllamaCloudEnabled
	i.KeepAlive = c.ChatKeepAlive
	i.LLMStreamIdleTimeout = c.LLMStreamIdleTimeout
	i.CloudLLMStreamIdleTimeout = c.CloudLLMStreamIdleTimeout
	i.PermissionMode = c.PermissionMode
	i.MaxTurns = c.MaxTurns
	i.MaxConcurrentTools = c.MaxConcurrentTools
	i.BashTimeout = c.BashTimeout
	i.MaxReadChars = c.MaxReadChars
	i.MaxResultChars = c.MaxResultChars
	i.MaxDirFiles = c.MaxDirFiles
	i.MaxPromptFiles = c.MaxPromptFiles
	i.MaxDirBytes = c.MaxDirBytes
	i.MaxPromptBytes = c.MaxPromptBytes
	i.MaxDirDepth = c.MaxDirDepth
	i.MentionDirectorySource = c.MentionDirectorySource
	i.MentionIncludeGitignoredOnExplicit = c.MentionIncludeGitignoredOnExplicit
	i.PromptDumpMode = c.PromptDumpMode
	i.PromptDumpKeep = c.PromptDumpKeep
	i.PromptPreviewChars = c.PromptPreviewChars
	i.LogLevel = c.LogLevel
	i.LogFormat = c.LogFormat
	i.SlowStageNoticeThreshold = c.SlowStageNoticeThreshold
	i.SlowStageNoticeThresholdSource = slowStageSource
	i.ContextMode = c.ContextMode
	i.MemoryRecallMode = c.MemoryRecallMode
}
