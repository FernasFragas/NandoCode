package tui

import (
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/state"
)

type RunPhase string

const (
	RunPhaseIdle               RunPhase = "idle"
	RunPhaseQueued             RunPhase = "queued"
	RunPhaseWaitingModel       RunPhase = "waiting_for_model"
	RunPhaseStreaming          RunPhase = "streaming"
	RunPhaseThinking           RunPhase = "thinking"
	RunPhaseRunningTool        RunPhase = "running_tool"
	RunPhasePermissionRequired RunPhase = "permission_required"
	RunPhaseRetrying           RunPhase = "retrying"
	RunPhaseCompacting         RunPhase = "compacting"
)

type RunUIState struct {
	Phase       RunPhase
	Label       string
	QueuedCount int
}

func (m *Model) snapshotRunUIState(app state.App, now time.Time) RunUIState {
	s := RunUIState{
		Phase:       RunPhaseIdle,
		Label:       "",
		QueuedCount: len(app.QueuedPrompts),
	}
	switch {
	case app.PermissionPrompt != nil:
		s.Phase = RunPhasePermissionRequired
		s.Label = "Permission required"
	case hasRunningTool(app.ActiveTools):
		s.Phase = RunPhaseRunningTool
		s.Label = "Running tool"
	case m.compactingActive:
		s.Phase = RunPhaseCompacting
		s.Label = "Compacting"
	case !m.retryActiveUntil.IsZero() && now.Before(m.retryActiveUntil):
		s.Phase = RunPhaseRetrying
		if notice := strings.TrimSpace(app.LastRetryNotice); notice != "" {
			s.Label = notice
		} else {
			s.Label = "Retrying"
		}
	case app.ActiveRun && m.thinkingActive:
		s.Phase = RunPhaseThinking
		s.Label = "Thinking"
	case app.ActiveRun && !m.firstStreamAt.IsZero():
		s.Phase = RunPhaseStreaming
		s.Label = "Streaming"
	case app.ActiveRun:
		s.Phase = RunPhaseWaitingModel
		s.Label = "Waiting for model"
	case len(app.QueuedPrompts) > 0:
		s.Phase = RunPhaseQueued
		s.Label = "Queued"
	default:
		s.Phase = RunPhaseIdle
		s.Label = "Idle"
	}
	return s
}
