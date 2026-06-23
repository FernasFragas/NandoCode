package permissions

import (
	"testing"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

func TestModeNormalize(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		expected Mode
	}{
		{"EmptyMode", "", ModeDefault},
		{"ModeBypass", ModeBypass, ModeBypass},
		{"ModeDontAsk", ModeDontAsk, ModeDontAsk},
		{"ModeAuto", ModeAuto, ModeAuto},
		{"ModeAcceptEdits", ModeAcceptEdits, ModeAcceptEdits},
		{"ModeDefault", ModeDefault, ModeDefault},
		{"ModePlan", ModePlan, ModePlan},
		{"ModeBubble", ModeBubble, ModeBubble},
		{"UnknownMode", Mode("unknown"), ModeDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mode.Normalize()
			if result != tt.expected {
				t.Errorf("Normalize(%q) = %q, want %q", tt.mode, result, tt.expected)
			}
		})
	}
}

func TestFromToolsMode(t *testing.T) {
	tests := []struct {
		name     string
		toolMode tools.PermissionMode
		expected Mode
	}{
		{"ToolsBypass", tools.PermissionBypassPermissions, ModeBypass},
		{"ToolsDontAsk", tools.PermissionDontAsk, ModeDontAsk},
		{"ToolsPlan", tools.PermissionPlan, ModePlan},
		{"ToolsDefault", tools.PermissionDefault, ModeDefault},
		{"ToolsEmpty", tools.PermissionMode(""), ModeDefault},
		{"ToolsUnknown", tools.PermissionMode("unknown"), ModeDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromToolsMode(tt.toolMode)
			if result != tt.expected {
				t.Errorf("FromToolsMode(%q) = %q, want %q", tt.toolMode, result, tt.expected)
			}
		})
	}
}

func TestModeString(t *testing.T) {
	tests := []struct {
		mode     Mode
		expected string
	}{
		{ModeBypass, "bypass"},
		{ModeDontAsk, "dontAsk"},
		{ModeDefault, "default"},
		{Mode("custom"), "custom"},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			result := tt.mode.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}
