package permissions

import "testing"

func TestClampMode(t *testing.T) {
	tests := []struct {
		name      string
		parent    Mode
		requested Mode
		want      Mode
	}{
		{"PlanParentBypassRequest", ModePlan, ModeBypass, ModePlan},
		{"PlanParentDefaultRequest", ModePlan, ModeDefault, ModePlan},
		{"PlanParentEmptyRequest", ModePlan, "", ModePlan},
		{"PlanParentUnknownRequest", ModePlan, Mode("whatever"), ModePlan},
		{"DefaultParentBypassRequest", ModeDefault, ModeBypass, ModeDefault},
		{"DefaultParentAcceptEditsRequest", ModeDefault, ModeAcceptEdits, ModeDefault},
		{"DefaultParentPlanRequest", ModeDefault, ModePlan, ModePlan},
		{"DefaultParentDontAskRequest", ModeDefault, ModeDontAsk, ModeDontAsk},
		{"DefaultParentEmptyRequest", ModeDefault, "", ModeDefault},
		{"BypassParentDefaultRequest", ModeBypass, ModeDefault, ModeDefault},
		{"BypassParentBypassRequest", ModeBypass, ModeBypass, ModeBypass},
		{"BypassParentEmptyRequest", ModeBypass, "", ModeBypass},
		{"DontAskParentBypassRequest", ModeDontAsk, ModeBypass, ModeDontAsk},
		{"DontAskParentDefaultRequest", ModeDontAsk, ModeDefault, ModeDontAsk},
		{"DontAskParentPlanRequest", ModeDontAsk, ModePlan, ModePlan},
		{"AcceptEditsParentAutoRequest", ModeAcceptEdits, ModeAuto, ModeAuto},
		{"AutoParentAcceptEditsRequest", ModeAuto, ModeAcceptEdits, ModeAuto},
		{"BubbleParentBypassRequest", ModeBubble, ModeBypass, ModeBubble},
		{"EmptyParentBypassRequest", "", ModeBypass, ModeDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClampMode(tt.parent, tt.requested); got != tt.want {
				t.Fatalf("ClampMode(%q, %q) = %q, want %q", tt.parent, tt.requested, got, tt.want)
			}
		})
	}
}
