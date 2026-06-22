package sendmessage

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/paths"
	"github.com/FernasFragas/nandocodego/internal/tasks"
	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/types"
)

var taskIDPattern = regexp.MustCompile(`^a-[0-9a-f]{12}$`)

var ErrNotSupported = errors.New("feature not supported in this phase")

type Input struct {
	To           string `json:"to"`
	Message      string `json:"message"`
	Summary      string `json:"summary,omitempty"`
	MessageID    string `json:"message_id,omitempty"`
	WaitForReply bool   `json:"wait_for_reply,omitempty"`
}

type Tool struct {
	supervisor *tasks.Supervisor
	logger     *slog.Logger
	resumeFn   ResumeFunc
}

type ResumeFunc func(ctx tools.Context, taskID string, msg tasks.PendingMessage) (newTaskID string, err error)

type Option func(*Tool)

func WithResumeFunc(fn ResumeFunc) Option {
	return func(t *Tool) {
		t.resumeFn = fn
	}
}

func New(supervisor *tasks.Supervisor, logger *slog.Logger, opts ...Option) *Tool {
	tool := &Tool{supervisor: supervisor, logger: logger}
	for _, opt := range opts {
		if opt != nil {
			opt(tool)
		}
	}
	return tool
}

func (t *Tool) Name() string { return "SendMessage" }

func (t *Tool) Description() string {
	return "Send a message to an agent by task ID, registered name, or uds: socket address."
}

func (t *Tool) Aliases() []string { return nil }

func (t *Tool) JSONSchema() map[string]any {
	return tools.ObjectSchema(map[string]any{
		"to":             tools.StringProperty("Agent task ID, agent name, uds:<path>, or bridge:<session-id>."),
		"message":        tools.StringProperty("Message body."),
		"summary":        tools.StringProperty("Short routing summary."),
		"message_id":     tools.StringProperty("Idempotency key for duplicate suppression."),
		"wait_for_reply": map[string]any{"type": "boolean", "description": "Reserved; unsupported in this phase."},
	}, []string{"to", "message"})
}

func (t *Tool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var in Input
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	in.To = strings.TrimSpace(in.To)
	in.Message = strings.TrimSpace(in.Message)
	if in.To == "" || in.Message == "" {
		return nil, fmt.Errorf("to and message are required")
	}
	return in, nil
}

func (t *Tool) IsEnabled(ctx tools.Context) bool { return true }
func (t *Tool) IsReadOnly(input any) bool        { return false }
func (t *Tool) IsConcurrencySafe(input any) bool { return false }
func (t *Tool) IsDestructive(input any) bool     { return false }

func (t *Tool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}

func (t *Tool) Call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in := input.(Input)
	if in.WaitForReply {
		return tools.Result{}, ErrNotSupported
	}
	if t.supervisor == nil {
		return tools.Result{}, fmt.Errorf("task supervisor unavailable")
	}
	switch {
	case strings.HasPrefix(in.To, "bridge:"):
		return tools.Result{}, ErrNotSupported
	case strings.HasPrefix(in.To, "uds:"):
		if err := t.sendUDS(strings.TrimPrefix(in.To, "uds:"), in); err != nil {
			return tools.Result{}, err
		}
		return tools.Result{Display: "message delivered via uds"}, nil
	default:
		taskID, err := t.resolveTaskID(in.To)
		if err != nil {
			return tools.Result{}, err
		}
		st, ok := t.supervisor.Get(taskID)
		if !ok {
			return tools.Result{}, fmt.Errorf("agent task not found: %s", taskID)
		}
		sum := st.ToSummary()
		if sum.Kind != types.KindAgent {
			return tools.Result{}, fmt.Errorf("task %s is not an agent task", taskID)
		}
		msg := tasks.PendingMessage{
			ID:        strings.TrimSpace(in.MessageID),
			FromAgent: "coordinator",
			ToAgent:   taskID,
			Summary:   strings.TrimSpace(in.Summary),
			Content:   in.Message,
			Timestamp: time.Now().UTC(),
		}
		if sum.Status != types.StatusRunning && sum.Status != types.StatusPending {
			if t.resumeFn == nil {
				return tools.Result{}, fmt.Errorf("agent task is terminal; auto-resume unavailable in this session")
			}
			newTaskID, err := t.resumeFn(ctx, taskID, msg)
			if err != nil {
				return tools.Result{}, err
			}
			if strings.TrimSpace(newTaskID) == "" {
				return tools.Result{Display: fmt.Sprintf("agent %s resumed", taskID)}, nil
			}
			return tools.Result{Display: fmt.Sprintf("agent %s resumed as %s", taskID, newTaskID)}, nil
		}
		if err := t.supervisor.QueueMessage(taskID, msg); err != nil {
			return tools.Result{}, err
		}
		return tools.Result{Display: fmt.Sprintf("message queued for %s", taskID)}, nil
	}
}

func (t *Tool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "SendMessage", Summary: "agent message routed"}
}

func (t *Tool) resolveTaskID(target string) (string, error) {
	if taskIDPattern.MatchString(target) {
		return target, nil
	}
	if id, ok := t.supervisor.LookupByName(target); ok {
		return id, nil
	}
	return "", fmt.Errorf("unknown agent target: %s", target)
}

func (t *Tool) sendUDS(path string, in Input) error {
	socketPath, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return err
	}
	socketRoot := filepath.Join(paths.StateDir(), "sockets")
	rootAbs, err := filepath.Abs(socketRoot)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(socketPath, rootAbs+string(filepath.Separator)) && socketPath != rootAbs {
		return fmt.Errorf("uds path must be under %s", rootAbs)
	}
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	payload := tasks.PendingMessage{
		ID:        strings.TrimSpace(in.MessageID),
		FromAgent: "coordinator",
		ToAgent:   in.To,
		Summary:   strings.TrimSpace(in.Summary),
		Content:   in.Message,
		Timestamp: time.Now().UTC(),
	}
	return json.NewEncoder(conn).Encode(payload)
}
