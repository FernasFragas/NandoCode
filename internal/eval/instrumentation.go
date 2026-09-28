package eval

import (
	"context"
	"sync"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/logging"
	"github.com/FernasFragas/Nandocode/internal/permissions"
)

type runMetrics struct {
	mu                sync.Mutex
	approvalsRequired int
	permissionCounts  PermissionCounts
	toolCallsByName   map[string]int
	toolSummaries     []ToolCallSummary
	retries           int
	terminal          *agent.Terminal
	terminalReason    string
}

func newRunMetrics() *runMetrics {
	return &runMetrics{
		toolCallsByName: make(map[string]int),
	}
}

func (m *runMetrics) wrapPrompt(next permissions.PromptFunc) permissions.PromptFunc {
	if next == nil {
		return nil
	}
	return func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
		m.mu.Lock()
		m.approvalsRequired++
		m.mu.Unlock()
		return next(ctx, prompt)
	}
}

func (m *runMetrics) permissionObserver() permissions.ObserverFunc {
	return func(_ context.Context, _ permissions.Request, result permissions.Result) {
		m.mu.Lock()
		defer m.mu.Unlock()
		switch result.Decision {
		case permissions.DecisionAllow:
			m.permissionCounts.Allowed++
		case permissions.DecisionDeny:
			m.permissionCounts.Denied++
		case permissions.DecisionAsk:
			m.permissionCounts.Asked++
		}
	}
}

func (m *runMetrics) collect(events <-chan agent.Event) (string, agent.Terminal) {
	finalAnswer := ""
	var terminal agent.Terminal
	for evt := range events {
		switch e := evt.(type) {
		case agent.AssistantTextDelta:
			finalAnswer += e.Content
		case agent.ToolUseStart:
			m.mu.Lock()
			m.toolCallsByName[e.Name]++
			m.toolSummaries = append(m.toolSummaries, ToolCallSummary{ID: e.ID, Name: e.Name})
			m.mu.Unlock()
		case agent.ToolUseResult:
			m.mu.Lock()
			for i := len(m.toolSummaries) - 1; i >= 0; i-- {
				if m.toolSummaries[i].ID == e.ID {
					m.toolSummaries[i].OK = e.Err == nil
					if e.Err != nil {
						m.toolSummaries[i].Error = logging.Redact(e.Err.Error())
					}
					break
				}
			}
			m.mu.Unlock()
		case agent.RetryNotice:
			m.mu.Lock()
			m.retries++
			m.mu.Unlock()
		case agent.Terminal:
			copy := e
			terminal = copy
			m.mu.Lock()
			m.terminal = &copy
			m.terminalReason = string(copy.Reason)
			m.mu.Unlock()
		}
	}
	if finalAnswer == "" && len(terminal.Conversation) > 0 {
		for i := len(terminal.Conversation) - 1; i >= 0; i-- {
			if terminal.Conversation[i].Role == "assistant" && terminal.Conversation[i].Content != "" {
				finalAnswer = terminal.Conversation[i].Content
				break
			}
		}
	}
	return finalAnswer, terminal
}

func (m *runMetrics) snapshot() (int, PermissionCounts, map[string]int, []ToolCallSummary, int, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	counts := m.permissionCounts
	byName := make(map[string]int, len(m.toolCallsByName))
	for k, v := range m.toolCallsByName {
		byName[k] = v
	}
	summaries := append([]ToolCallSummary(nil), m.toolSummaries...)
	return m.approvalsRequired, counts, byName, summaries, m.retries, m.terminalReason
}

func terminalTaskCompletion(term agent.TerminalReason) string {
	switch term {
	case agent.TerminalCompleted:
		return "completed"
	case agent.TerminalAborted:
		return "aborted"
	case agent.TerminalMaxTurns:
		return "max_turns"
	case agent.TerminalContextOverflow:
		return "context_overflow"
	case agent.TerminalStopHook:
		return "stop_hook"
	case agent.TerminalUnrecoverable:
		return "unrecoverable"
	default:
		return "not_started"
	}
}

func runtimeMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}
