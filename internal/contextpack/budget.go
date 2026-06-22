package contextpack

import (
	"strings"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

const conservativeCharsPerToken = 4
const renderedEvidenceOverheadTokenAllowance = 512

func estimateRenderedEvidenceTokens(renderedPrompt string) int {
	return estimateTokensFromChars(len(renderedPrompt))
}

// ClampToolContextToBudget narrows mention-expansion byte caps to fit the
// available current-turn evidence budget.
func ClampToolContextToBudget(ctx tools.Context, budget agent.AssemblyBudget) tools.Context {
	if budget.AvailableEvidenceTokens <= 0 {
		return ctx
	}
	maxBytes := int64(budget.AvailableEvidenceTokens * conservativeCharsPerToken)
	if maxBytes <= 0 {
		return ctx
	}
	if ctx.MaxPromptBytes <= 0 || maxBytes < ctx.MaxPromptBytes {
		ctx.MaxPromptBytes = maxBytes
	}
	if ctx.MaxDirBytes <= 0 || maxBytes < ctx.MaxDirBytes {
		ctx.MaxDirBytes = maxBytes
	}
	maxReadChars := int(maxBytes / 2)
	if maxReadChars < 1 {
		maxReadChars = 1
	}
	if ctx.MaxReadChars <= 0 || maxReadChars < ctx.MaxReadChars {
		ctx.MaxReadChars = maxReadChars
	}
	return ctx
}

func rebalanceBudgetForExplicitMentions(input string, budget agent.AssemblyBudget) agent.AssemblyBudget {
	if budget.EffectiveNumCtx <= 0 {
		return budget
	}
	if len(parseMentionRefs(input)) == 0 {
		return budget
	}
	lower := strings.ToLower(input)
	intentLike := strings.Contains(lower, "review") ||
		strings.Contains(lower, "status") ||
		strings.Contains(lower, "implemented") ||
		strings.Contains(lower, "analy") ||
		strings.Contains(lower, "what is")
	if !intentLike && budget.AvailableEvidenceTokens > 0 {
		return budget
	}
	baselineWithoutOutput := budget.EffectiveNumCtx -
		budget.ContextReserveTokens -
		budget.EstimatedSystemTokens -
		budget.EstimatedToolSchemaTokens -
		budget.EstimatedHistoryTokens
	if baselineWithoutOutput <= 0 {
		return budget
	}
	cappedOutput := budget.OutputReserveTokens
	maxCap := budget.EffectiveNumCtx / 2
	if maxCap > 16000 {
		maxCap = 16000
	}
	if maxCap < 1024 {
		maxCap = 1024
	}
	if cappedOutput > maxCap {
		cappedOutput = maxCap
	}
	available := baselineWithoutOutput - cappedOutput
	if available < 0 {
		available = 0
	}
	if available == 0 && baselineWithoutOutput > 256 {
		available = min(1024, baselineWithoutOutput/8)
		cappedOutput = baselineWithoutOutput - available
		if cappedOutput < 0 {
			cappedOutput = 0
		}
	}
	budget.OutputReserveTokens = cappedOutput
	budget.AvailableEvidenceTokens = available
	return budget
}
