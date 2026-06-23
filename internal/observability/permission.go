package observability

import (
	"context"

	"github.com/FernasFragas/Nandocode/internal/permissions"
)

// PermissionObserver returns a resolver observer that records permission outcomes.
func PermissionObserver(meter *Meter, bridge Bridge) permissions.ObserverFunc {
	if bridge == nil {
		bridge = noopBridge{}
	}
	return func(_ context.Context, req permissions.Request, result permissions.Result) {
		if meter != nil {
			meter.RecordPermissionDecision(req.Mode, result.Stage, req.ToolName, result.Decision)
		}
		bridge.RecordPermissionDecision(req.Mode, result.Stage, req.ToolName, result.Decision)
	}
}
