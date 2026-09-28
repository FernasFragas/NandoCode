package eval

import (
	"context"
	"sync"

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

	// maxToolCalls caps attempted tool calls (0 = unlimited); onLimit is
	// invoked once when the cap is exceeded, to stop the run.
	maxToolCalls      int
	onLimit           func()
	attemptedCalls    int
	toolLimitExceeded bool
}

type metricsSnapshot struct {
	ApprovalsRequired int
	PermissionCounts  PermissionCounts
	ToolCallsByName   map[string]int
	ToolSummaries     []ToolCallSummary
	Retries           int
	ToolLimitExceeded bool
}

func newRunMetrics(maxToolCalls int, onLimit func()) *runMetrics {
	return &runMetrics{
		toolCallsByName: make(map[string]int),
		maxToolCalls:    maxToolCalls,
		onLimit:         onLimit,
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

// permissionObserver sees every tool call the model attempts, including calls
// the permission layer denies before any ToolUseStart event, so it is the
// source of truth for per-tool usage counts and the tool-call limit.
func (m *runMetrics) permissionObserver() permissions.ObserverFunc {
	return func(_ context.Context, req permissions.Request, result permissions.Result) {
		m.mu.Lock()
		m.attemptedCalls++
		if req.ToolName != "" {
			m.toolCallsByName[req.ToolName]++
		}
		limitHit := m.maxToolCalls > 0 && m.attemptedCalls > m.maxToolCalls && !m.toolLimitExceeded
		if limitHit {
			m.toolLimitExceeded = true
		}
		switch result.Decision {
		case permissions.DecisionAllow:
			m.permissionCounts.Allowed++
		case permissions.DecisionDeny:
			m.permissionCounts.Denied++
		case permissions.DecisionAsk:
			m.permissionCounts.Asked++
		}
		m.mu.Unlock()
		if limitHit && m.onLimit != nil {
			m.onLimit()
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

func (m *runMetrics) snapshot() metricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	byName := make(map[string]int, len(m.toolCallsByName))
	for k, v := range m.toolCallsByName {
		byName[k] = v
	}
	return metricsSnapshot{
		ApprovalsRequired: m.approvalsRequired,
		PermissionCounts:  m.permissionCounts,
		ToolCallsByName:   byName,
		ToolSummaries:     append([]ToolCallSummary(nil), m.toolSummaries...),
		Retries:           m.retries,
		ToolLimitExceeded: m.toolLimitExceeded,
	}
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
