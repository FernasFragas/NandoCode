package server

import (
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/llm"
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
	SessionID       string       `json:"session_id"`
	CreatedAt       time.Time    `json:"created_at"`
	LastActive      time.Time    `json:"last_active"`
	State           SessionState `json:"state"`
	Running         bool         `json:"running"`
	CoordinatorMode bool         `json:"coordinator_mode,omitempty"`
	WorkerCount     int          `json:"worker_count,omitempty"`
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

type TreeEntry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
}

type TreeResponse struct {
	Root    string      `json:"root"`
	Entries []TreeEntry `json:"entries"`
	Stats   TreeStats   `json:"stats"`
}

type TreeStats struct {
	Truncated bool   `json:"truncated"`
	Reason    string `json:"reason,omitempty"`
	Source    string `json:"source,omitempty"`
}
