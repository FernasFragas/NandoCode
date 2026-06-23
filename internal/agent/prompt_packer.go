package agent

import (
	"strings"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

type packResult struct {
	Messages []llm.Message
	Report   PromptPackReport
}

func packPromptHistory(history []llm.Message, inputBudgetTokens int) packResult {
	if len(history) == 0 {
		return packResult{
			Messages: nil,
			Report: PromptPackReport{
				InputBudgetTokens: inputBudgetTokens,
				DroppedRoles:      map[string]int{},
			},
		}
	}

	if inputBudgetTokens <= 0 {
		inputBudgetTokens = 1
	}

	type estMsg struct {
		index  int
		tokens int
	}
	estimated := make([]estMsg, 0, len(history))
	total := 0
	for i := range history {
		toks := estimatePromptTokens([]llm.Message{history[i]})
		estimated = append(estimated, estMsg{index: i, tokens: toks})
		total += toks
	}

	if total <= inputBudgetTokens {
		includedMentionBlocks := 0
		for i := range history {
			includedMentionBlocks += mentionBlockCount(history[i].Content)
		}
		return packResult{
			Messages: append([]llm.Message(nil), history...),
			Report: PromptPackReport{
				InputBudgetTokens:       inputBudgetTokens,
				EstimatedIncluded:       total,
				IncludedMessages:        len(history),
				DroppedRoles:            map[string]int{},
				IncludedMentionBlocks:   includedMentionBlocks,
				LastUserMessageIncluded: true,
				SystemMessageIncluded:   history[0].Role == llm.RoleSystem,
			},
		}
	}

	included := map[int]struct{}{}
	used := 0
	forced := false

	// Always try to keep first system message as anchor.
	if history[0].Role == llm.RoleSystem {
		included[0] = struct{}{}
		used += estimated[0].tokens
	}

	// Fill from newest to oldest to keep recency.
	for i := len(history) - 1; i >= 0; i-- {
		if _, ok := included[i]; ok {
			continue
		}
		toks := estimated[i].tokens
		if used+toks > inputBudgetTokens {
			continue
		}
		included[i] = struct{}{}
		used += toks
	}

	// Force include most recent message so the current user turn is preserved.
	lastIdx := len(history) - 1
	if _, ok := included[lastIdx]; !ok {
		included[lastIdx] = struct{}{}
		used += estimated[lastIdx].tokens
		forced = true
	}

	packed := make([]llm.Message, 0, len(included))
	includedTokens := 0
	skippedTokens := 0
	droppedBytes := 0
	droppedRoles := map[string]int{}
	droppedMentionBlocks := 0
	includedMentionBlocks := 0
	lastUserIncluded := false
	systemIncluded := false
	lastUserIdx := -1
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == llm.RoleUser {
			lastUserIdx = i
			break
		}
	}
	for i := 0; i < len(history); i++ {
		if _, ok := included[i]; ok {
			packed = append(packed, history[i])
			includedTokens += estimated[i].tokens
			includedMentionBlocks += mentionBlockCount(history[i].Content)
			if i == lastUserIdx {
				lastUserIncluded = true
			}
			if i == 0 && history[i].Role == llm.RoleSystem {
				systemIncluded = true
			}
		} else {
			skippedTokens += estimated[i].tokens
			droppedBytes += len(history[i].Content)
			droppedRoles[string(history[i].Role)]++
			droppedMentionBlocks += mentionBlockCount(history[i].Content)
		}
	}

	return packResult{
		Messages: packed,
		Report: PromptPackReport{
			InputBudgetTokens:       inputBudgetTokens,
			EstimatedIncluded:       includedTokens,
			EstimatedSkipped:        skippedTokens,
			IncludedMessages:        len(packed),
			SkippedMessages:         len(history) - len(packed),
			ForcedIncludeLast:       forced,
			DroppedRoles:            droppedRoles,
			DroppedBytes:            droppedBytes,
			DroppedMentionBlocks:    droppedMentionBlocks,
			IncludedMentionBlocks:   includedMentionBlocks,
			LastUserMessageIncluded: lastUserIncluded,
			SystemMessageIncluded:   systemIncluded,
		},
	}
}

func mentionBlockCount(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "<file ") + strings.Count(content, "<directory ")
}
