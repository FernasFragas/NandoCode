package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/permissions"
)

const (
	envTelemetry    = "NANDOCODEGO_TELEMETRY"
	envOTELEndpoint = "NANDOCODEGO_OTEL_ENDPOINT"
)

// TelemetrySettings derives runtime telemetry configuration from environment.
type TelemetrySettings struct {
	Enabled  bool
	Endpoint string
	Warning  string
}

func TelemetryFromEnv() TelemetrySettings {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(envTelemetry)))
	enabled := raw == "1" || raw == "true" || raw == "yes" || raw == "on"
	endpoint := strings.TrimSpace(os.Getenv(envOTELEndpoint))
	out := TelemetrySettings{
		Enabled:  enabled,
		Endpoint: endpoint,
	}
	if enabled && endpoint == "" {
		out.Warning = "telemetry enabled but NANDOCODEGO_OTEL_ENDPOINT not set; OTEL export disabled"
	}
	return out
}

func (t TelemetrySettings) DoctorStatus() string {
	if !t.Enabled {
		return "disabled"
	}
	if t.Endpoint == "" {
		return "warning (endpoint not configured)"
	}
	return "otel -> " + t.Endpoint
}

// Bridge forwards observability data to an external telemetry backend.
// Phase 16 keeps the default bridge as no-op unless explicitly configured.
type Bridge interface {
	RecordLLMChat(firstTokenLatency, duration time.Duration, promptTokens, completionTokens int64, doneReason string, err error)
	RecordToolCall(toolName string, duration time.Duration, err error)
	RecordAgentRun(usage agent.Usage, duration time.Duration, reason agent.TerminalReason)
	RecordPermissionDecision(mode permissions.Mode, stage permissions.Stage, toolName string, decision permissions.Decision)
	RecordToolBatch(batchSize int, safe bool, duration time.Duration)
	Shutdown(ctx context.Context) error
}

type noopBridge struct{}

func (noopBridge) RecordLLMChat(time.Duration, time.Duration, int64, int64, string, error) {}
func (noopBridge) RecordToolCall(string, time.Duration, error)                             {}
func (noopBridge) RecordAgentRun(agent.Usage, time.Duration, agent.TerminalReason)         {}
func (noopBridge) RecordPermissionDecision(permissions.Mode, permissions.Stage, string, permissions.Decision) {
}
func (noopBridge) RecordToolBatch(int, bool, time.Duration) {}
func (noopBridge) Shutdown(context.Context) error           { return nil }

// NewBridgeFromEnv returns a bridge that is no-op by default.
// When telemetry env is enabled and endpoint is configured, it currently logs the activation
// and keeps no-op forwarding as a safe default integration point.
func NewBridgeFromEnv(logger *slog.Logger) (Bridge, TelemetrySettings) {
	settings := TelemetryFromEnv()
	if logger == nil {
		logger = slog.Default()
	}
	if settings.Warning != "" {
		logger.Warn(settings.Warning)
	}
	if settings.Enabled && settings.Endpoint != "" {
		logger.Info("telemetry export enabled", "endpoint", settings.Endpoint)
	}
	return noopBridge{}, settings
}
