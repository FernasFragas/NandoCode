package tui

import "testing"

func TestDefaultStylesSemanticRolesRenderNonEmpty(t *testing.T) {
	s := DefaultStyles()
	sample := "sample"
	tests := []struct {
		name string
		got  string
	}{
		{name: "SemMuted", got: s.SemMuted.Render(sample)},
		{name: "SemAccent", got: s.SemAccent.Render(sample)},
		{name: "SemWarning", got: s.SemWarning.Render(sample)},
		{name: "SemInfo", got: s.SemInfo.Render(sample)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got == "" {
				t.Fatalf("%s rendered empty output", tc.name)
			}
		})
	}
}

