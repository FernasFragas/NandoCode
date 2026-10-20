package server

import (
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/tasks"
)

type SessionState string

const (
	SessionStateReady   SessionState = "ready"
	SessionStateRunning SessionState = "running"
	SessionStateClosing SessionState = "closing"
)

type SessionEvent struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Time      time.Time      `json:"time"`
	SessionID string         `json:"session_id"`
	Data      map[string]any `json:"data,omitempty"`
}

type SessionView struct {
	SessionID       string                 `json:"session_id"`
	CreatedAt       time.Time              `json:"created_at"`
	LastActive      time.Time              `json:"last_active"`
	State           SessionState           `json:"state"`
	Running         bool                   `json:"running"`
	CoordinatorMode bool                   `json:"coordinator_mode,omitempty"`
	WorkerCount     int                    `json:"worker_count,omitempty"`
	Workers         []tasks.WorkerSnapshot `json:"workers"`
	// WorkspaceRoot is the session working directory; the browser uses it to
	// match absolute tool paths against workspace-relative open tabs.
	WorkspaceRoot string `json:"workspace_root,omitempty"`
}

type MessageRequest struct {
	Prompt    string `json:"prompt"`
	MessageID string `json:"message_id,omitempty"`
}

type PermissionResolveRequest struct {
	Decision string `json:"decision"`
}

type TerminalSnapshot struct {
	Reason agent.TerminalReason `json:"reason"`
	Detail string               `json:"detail,omitempty"`
	Usage  agent.Usage          `json:"usage"`
}

type modelsResponse struct {
	Models []llm.ModelInfo `json:"models"`
}

type ModelUpdateRequest struct {
	Model string `json:"model"`
}
