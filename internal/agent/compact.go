package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

// CompactionConfig controls when and how compaction fires.
type CompactionConfig struct {
	// Threshold is the fraction of MaxContextTokens that triggers proactive compaction.
	// Default 0.8. Set to 0 to disable proactive compaction.
	Threshold float64

	// MinTurns is the minimum number of complete turns required before compaction.
	// Default 4.
	MinTurns int

	// SummaryModel overrides the model for the summary side-query.
	// Defaults to the session model.
	SummaryModel string

	// SummaryPrompt overrides the system prompt for the summary side-query.
	SummaryPrompt string

	// MaxSummaryLen is the maximum tokens in the summary response. Default 2000.
	MaxSummaryLen int

	// MaxContextTokens is the model's context window size.
	// 0 disables proactive compaction.
	MaxContextTokens int64

	// Disabled disables all automatic compaction.
	Disabled bool
}

// DefaultCompactionConfig returns the production default configuration.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		Threshold:     0.8,
		MinTurns:      4,
		MaxSummaryLen: 2000,
	}
}

// CompactionResult contains before/after state after a compaction attempt.
type CompactionResult struct {
	Before       int
	After        int
	TokensBefore int64
	TokensAfter  int64
	Summary      string
	Layer        int
	Skipped      bool
	Error        string
	Messages     []llm.Message // compacted message slice
}

var thinkingRe = regexp.MustCompile(`(?s)<thinking>.*?</thinking>`)

// CountTurnsExported is the exported version of countTurns for use by the TUI.
func CountTurnsExported(messages []llm.Message) int {
	return countTurns(messages)
}

// EmergencyTruncateExported is the exported version of emergencyTruncate for use by the TUI.
func EmergencyTruncateExported(messages []llm.Message, minTurns int) []llm.Message {
	return emergencyTruncate(messages, minTurns)
}

// countTurns returns the number of complete user+assistant turn pairs.
func countTurns(messages []llm.Message) int {
	turns := 0
	i := 0
	for i < len(messages) {
		if messages[i].Role == llm.RoleSystem {
			i++
			continue
		}
		if messages[i].Role == llm.RoleUser {
			// Look for a following assistant message
			j := i + 1
			for j < len(messages) && messages[j].Role == llm.RoleTool {
				j++
			}
			if j < len(messages) && messages[j].Role == llm.RoleAssistant {
				turns++
				i = j + 1
				continue
			}
		}
		i++
	}
	return turns
}

const toolResultCollapseThreshold = 500

// collapseToolResults replaces long tool result messages with one-line summaries.
func collapseToolResults(messages []llm.Message) []llm.Message {
	out := make([]llm.Message, len(messages))
	copy(out, messages)
	for i, msg := range out {
		if msg.Role != llm.RoleTool {
			continue
		}
		if len(msg.Content) > toolResultCollapseThreshold {
			name := msg.ToolName
			if name == "" {
				name = "unknown"
			}
			out[i].Content = fmt.Sprintf("[Tool result: %s, %d chars, truncated for compaction]", name, len(msg.Content))
		}
	}
	return out
}

// stripThinkingBlocks removes <thinking>...</thinking> from older turns (all but last 2).
func stripThinkingBlocks(messages []llm.Message) []llm.Message {
	// Find the indices of the last 2 assistant messages to preserve
	assistantIndices := []int{}
	for i, m := range messages {
		if m.Role == llm.RoleAssistant {
			assistantIndices = append(assistantIndices, i)
		}
	}

	preserveFrom := 0
	if len(assistantIndices) > 2 {
		preserveFrom = assistantIndices[len(assistantIndices)-2]
	}

	out := make([]llm.Message, len(messages))
	copy(out, messages)
	for i := range out {
		if i >= preserveFrom {
			continue
		}
		if out[i].Thinking != "" {
			out[i].Thinking = ""
		}
		if strings.Contains(out[i].Content, "<thinking>") {
			out[i].Content = thinkingRe.ReplaceAllString(out[i].Content, "")
		}
	}
	return out
}

// emergencyTruncate preserves initial system messages and the last minTurns turn pairs.
func emergencyTruncate(messages []llm.Message, minTurns int) []llm.Message {
	// Collect leading system messages
	var sysMsgs []llm.Message
	start := 0
	for start < len(messages) && messages[start].Role == llm.RoleSystem {
		sysMsgs = append(sysMsgs, messages[start])
		start++
	}

	rest := messages[start:]

	// Find turn boundaries from the end: collect last minTurns user+assistant pairs
	type pair struct{ from, to int }
	var pairs []pair
	i := len(rest) - 1
	for i >= 0 && len(pairs) < minTurns {
		if rest[i].Role == llm.RoleAssistant {
			// Walk back to find the matching user message
			j := i - 1
			for j >= 0 && rest[j].Role == llm.RoleTool {
				j--
			}
			if j >= 0 && rest[j].Role == llm.RoleUser {
				pairs = append([]pair{{j, i}}, pairs...)
				i = j - 1
				continue
			}
		}
		i--
	}

	if len(pairs) == 0 {
		return append(sysMsgs, rest...)
	}

	keepFrom := pairs[0].from
	kept := rest[keepFrom:]
	return append(sysMsgs, kept...)
}

const defaultSummaryPromptPrefix = "Summarize the following conversation concisely in 3-5 sentences, preserving key decisions, file paths, and current task status:\n\n"

// buildSummaryPrompt builds the prompt text for the summary side-query.
func buildSummaryPrompt(cfg CompactionConfig, messages []llm.Message) string {
	prefix := cfg.SummaryPrompt
	if prefix == "" {
		prefix = defaultSummaryPromptPrefix
	}

	var b strings.Builder
	b.WriteString(prefix)
	for _, m := range messages {
		b.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}
	return b.String()
}

// summarizeMessages calls the LLM non-streaming to summarize messages.
func summarizeMessages(ctx context.Context, client llm.Client, cfg CompactionConfig, model string, messages []llm.Message) (string, error) {
	if client == nil {
		return "", fmt.Errorf("no LLM client available")
	}

	summaryModel := cfg.SummaryModel
	if summaryModel == "" {
		summaryModel = model
	}

	maxTokens := cfg.MaxSummaryLen
	if maxTokens <= 0 {
		maxTokens = 2000
	}

	prompt := buildSummaryPrompt(cfg, messages)

	req := &llm.ChatRequest{
		Model: summaryModel,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Stream: false,
		Options: map[string]any{
			"num_predict": maxTokens,
		},
	}

	stream, err := client.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM summary request failed: %w", err)
	}

	var buf strings.Builder
	for evt := range stream {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if evt.Message.Content != "" {
			buf.WriteString(evt.Message.Content)
		}
	}

	return buf.String(), nil
}

// Compact reduces the message history using the 4-layer strategy.
// It is non-destructive: the caller's slice is not modified.
func Compact(ctx context.Context, client llm.Client, cfg CompactionConfig, model string, messages []llm.Message) CompactionResult {
	before := len(messages)

	if countTurns(messages) < cfg.MinTurns {
		return CompactionResult{
			Before:  before,
			After:   before,
			Skipped: true,
		}
	}

	// Layer 3: strip thinking from all but last 2 turns
	processed := stripThinkingBlocks(messages)
	// Layer 2: collapse tool results
	processed = collapseToolResults(processed)

	// Determine split: summarize all but last MinTurns turns
	// Count from the end to find the start of the last MinTurns turn pairs
	splitIdx := findSplitIndex(processed, cfg.MinTurns)

	toSummarize := processed[:splitIdx]
	toKeep := processed[splitIdx:]

	// Collect leading system messages to preserve always
	var sysMsgs []llm.Message
	nonSysStart := 0
	for nonSysStart < len(toSummarize) && toSummarize[nonSysStart].Role == llm.RoleSystem {
		sysMsgs = append(sysMsgs, toSummarize[nonSysStart])
		nonSysStart++
	}
	conversationToSummarize := toSummarize[nonSysStart:]

	if len(conversationToSummarize) == 0 {
		// Nothing to summarize
		return CompactionResult{
			Before:   before,
			After:    before,
			Skipped:  true,
			Messages: messages,
		}
	}

	// Layer 1: LLM summarization
	summary, err := summarizeMessages(ctx, client, cfg, model, conversationToSummarize)
	if err == nil && summary != "" {
		summaryMsg := llm.Message{
			Role:    llm.RoleSystem,
			Content: fmt.Sprintf("[Summary of earlier conversation]\n\n%s", summary),
		}
		compacted := make([]llm.Message, 0, len(sysMsgs)+1+len(toKeep))
		compacted = append(compacted, sysMsgs...)
		compacted = append(compacted, summaryMsg)
		compacted = append(compacted, toKeep...)

		return CompactionResult{
			Before:   before,
			After:    len(compacted),
			Summary:  summary,
			Layer:    1,
			Messages: compacted,
		}
	}

	// Layer 4: emergency truncate
	truncated := emergencyTruncate(messages, cfg.MinTurns)
	errMsg := "LLM summarization failed; truncated oldest turns"
	if err != nil {
		errMsg = fmt.Sprintf("LLM summarization failed (%s); truncated oldest turns", err.Error())
	}

	return CompactionResult{
		Before:   before,
		After:    len(truncated),
		Layer:    4,
		Error:    errMsg,
		Messages: truncated,
	}
}

// findSplitIndex returns the index at which the last minTurns turn pairs begin.
func findSplitIndex(messages []llm.Message, minTurns int) int {
	// Count non-system messages from the end; find the start of last minTurns turns
	type pair struct{ userIdx int }
	var pairs []pair

	i := len(messages) - 1
	for i >= 0 && len(pairs) < minTurns {
		if messages[i].Role == llm.RoleAssistant {
			j := i - 1
			for j >= 0 && messages[j].Role == llm.RoleTool {
				j--
			}
			if j >= 0 && messages[j].Role == llm.RoleUser {
				pairs = append([]pair{{j}}, pairs...)
				i = j - 1
				continue
			}
		}
		i--
	}

	if len(pairs) == 0 {
		return 0
	}
	return pairs[0].userIdx
}
