// Package permissions implements the permission resolution system.
package permissions

import (
	"github.com/FernasFragas/nandocodego/internal/tools"
)

// Mode represents a permission mode that determines how the resolver handles
// tool classifications.
type Mode string

const (
	// ModeBypass allows all tool calls except those intrinsically denied
	// (destructive Bash, tool-classifier deny).
	ModeBypass Mode = "bypass"
	// ModeDontAsk allows read-only calls and denies anything that would
	// require a prompt or explicit permission.
	ModeDontAsk Mode = "dontAsk"
	// ModeAuto allows read-only calls, denies destructive calls, and asks
	// for classification ambiguity (mutating calls with no classifier yet).
	// Phase 5 has no LLM classifier, so this behaves like ModeDefault.
	ModeAuto Mode = "auto"
	// ModeAcceptEdits allows read-only calls, allows contained file edits,
	// and asks for other mutating calls.
	ModeAcceptEdits Mode = "acceptEdits"
	// ModeDefault allows read-only calls and asks for anything else.
	ModeDefault Mode = "default"
	// ModePlan allows only read-only calls and denies anything that would
	// modify state.
	ModePlan Mode = "plan"
	// ModeBubble asks for all decisions except denials from rules/hooks/tool.
	// Used for forwarding to parent agents or TUI layers.
	ModeBubble Mode = "bubble"
)

// Normalize returns the mode, or ModeDefault if empty or unknown.
func (m Mode) Normalize() Mode {
	if m == "" {
		return ModeDefault
	}
	switch m {
	case ModeBypass, ModeDontAsk, ModeAuto, ModeAcceptEdits, ModeDefault, ModePlan, ModeBubble:
		return m
	default:
		return ModeDefault
	}
}

// String returns the string representation of the mode.
func (m Mode) String() string {
	return string(m)
}

// FromToolsMode converts a tools.PermissionMode to a Phase 5 Mode.
// This provides backward compatibility with the Phase 3/4 permission modes.
func FromToolsMode(mode tools.PermissionMode) Mode {
	switch mode {
	case tools.PermissionBypassPermissions:
		return ModeBypass
	case tools.PermissionDontAsk:
		return ModeDontAsk
	case tools.PermissionPlan:
		return ModePlan
	case tools.PermissionDefault, "":
		return ModeDefault
	default:
		return ModeDefault
	}
}

// ToToolsMode converts a Phase 5 mode to the legacy tools permission mode.
func ToToolsMode(mode Mode) tools.PermissionMode {
	switch mode.Normalize() {
	case ModeBypass:
		return tools.PermissionBypassPermissions
	case ModeDontAsk:
		return tools.PermissionDontAsk
	case ModePlan:
		return tools.PermissionPlan
	default:
		return tools.PermissionDefault
	}
}
