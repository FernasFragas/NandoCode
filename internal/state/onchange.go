package state

import (
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/permissions"
)

// OnChange is the production callback that mirrors selected app state fields into bootstrap.
// It is called exactly once per Store Set operation, after the lock is released.
// OnChange must be side-effect-light and must not perform I/O, call tools, or interact with models.
//
// Mirrored fields (infrastructure):
//   - ActiveModel
//   - LLMProvider
//   - LLMBaseURL
//   - WorkingDir
//   - Tool budget settings (MaxResultChars, MaxReadChars, dir mention caps, BashTimeout)
//   - PermissionMode
//   - PermissionRules
//
// Non-mirrored fields (UI/session):
//   - Messages, QueuedPrompts, InputBuffer
//   - Active tool calls
//   - Permission prompt modal state
//   - Retry notices
//   - Terminal reason/detail
//   - Usage statistics
//   - Tasks
func OnChange(prev, next App) {
	bs := bootstrap.Global()

	bs.Update(func(snap *bootstrap.Snapshot) {
		// Only update if fields actually changed to avoid unnecessary churn
		if prev.ActiveModel != next.ActiveModel {
			snap.DefaultModel = next.ActiveModel
		}
		if prev.LLMProvider != next.LLMProvider {
			snap.LLMProvider = next.LLMProvider
		}
		if prev.LLMBaseURL != next.LLMBaseURL {
			snap.LLMBaseURL = next.LLMBaseURL
		}

		if prev.ToolSettings.WorkingDir != next.ToolSettings.WorkingDir {
			snap.WorkingDir = next.ToolSettings.WorkingDir
		}

		if prev.ToolSettings.MaxResultChars != next.ToolSettings.MaxResultChars {
			snap.MaxResultChars = next.ToolSettings.MaxResultChars
		}

		if prev.ToolSettings.MaxReadChars != next.ToolSettings.MaxReadChars {
			snap.MaxReadChars = next.ToolSettings.MaxReadChars
		}

		if prev.ToolSettings.MaxDirFiles != next.ToolSettings.MaxDirFiles {
			snap.MaxDirFiles = next.ToolSettings.MaxDirFiles
		}

		if prev.ToolSettings.MaxPromptFiles != next.ToolSettings.MaxPromptFiles {
			snap.MaxPromptFiles = next.ToolSettings.MaxPromptFiles
		}

		if prev.ToolSettings.MaxDirBytes != next.ToolSettings.MaxDirBytes {
			snap.MaxDirBytes = next.ToolSettings.MaxDirBytes
		}

		if prev.ToolSettings.MaxPromptBytes != next.ToolSettings.MaxPromptBytes {
			snap.MaxPromptBytes = next.ToolSettings.MaxPromptBytes
		}

		if prev.ToolSettings.MaxDirDepth != next.ToolSettings.MaxDirDepth {
			snap.MaxDirDepth = next.ToolSettings.MaxDirDepth
		}
		if prev.ToolSettings.MentionDirectorySource != next.ToolSettings.MentionDirectorySource {
			snap.MentionDirectorySource = next.ToolSettings.MentionDirectorySource
		}
		if prev.ToolSettings.MentionIncludeGitignoredOnExplicit != next.ToolSettings.MentionIncludeGitignoredOnExplicit {
			snap.MentionIncludeGitignoredOnExplicit = next.ToolSettings.MentionIncludeGitignoredOnExplicit
		}
		if prev.ToolSettings.PromptDumpMode != next.ToolSettings.PromptDumpMode {
			snap.PromptDumpMode = next.ToolSettings.PromptDumpMode
		}
		if prev.ToolSettings.PromptDumpKeep != next.ToolSettings.PromptDumpKeep {
			snap.PromptDumpKeep = next.ToolSettings.PromptDumpKeep
		}
		if prev.ToolSettings.PromptPreviewChars != next.ToolSettings.PromptPreviewChars {
			snap.PromptPreviewChars = next.ToolSettings.PromptPreviewChars
		}

		if prev.ToolSettings.BashTimeout != next.ToolSettings.BashTimeout {
			snap.BashTimeout = next.ToolSettings.BashTimeout
		}

		if prev.PermissionMode != next.PermissionMode {
			snap.PermissionMode = next.PermissionMode
		}

		// For permission rules, always update defensively to ensure consistency
		// (Bootstrap's Update will re-copy the rules anyway)
		if !rulesEqual(prev.PermissionRules, next.PermissionRules) {
			snap.PermissionRules = next.PermissionRules
		}
	})
}

// rulesEqual reports whether two permission rule sets are equal.
func rulesEqual(a, b permissions.Rules) bool {
	if len(a.AlwaysAllow) != len(b.AlwaysAllow) {
		return false
	}
	if len(a.AlwaysDeny) != len(b.AlwaysDeny) {
		return false
	}
	if len(a.AlwaysAsk) != len(b.AlwaysAsk) {
		return false
	}

	for i, rule := range a.AlwaysAllow {
		if rule.Pattern != b.AlwaysAllow[i].Pattern || rule.Source != b.AlwaysAllow[i].Source {
			return false
		}
	}
	for i, rule := range a.AlwaysDeny {
		if rule.Pattern != b.AlwaysDeny[i].Pattern || rule.Source != b.AlwaysDeny[i].Source {
			return false
		}
	}
	for i, rule := range a.AlwaysAsk {
		if rule.Pattern != b.AlwaysAsk[i].Pattern || rule.Source != b.AlwaysAsk[i].Source {
			return false
		}
	}

	return true
}
