package observability

import (
	"context"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/permissions"
)

func TestPermissionObserverRecords(t *testing.T) {
	m := NewMeter()
	obs := PermissionObserver(m, nil)
	obs(context.Background(), permissions.Request{
		Mode:     permissions.ModeDefault,
		ToolName: "Bash",
	}, permissions.Result{
		Decision: permissions.DecisionAllow,
		Stage:    permissions.StageMode,
	})
	if len(m.Snapshot().PermissionDecisions) != 1 {
		t.Fatalf("permission decisions not recorded")
	}
}
