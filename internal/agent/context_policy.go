package agent

import (
	"strings"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

const (
	defaultEstimatedSystemTokens     = 256
	defaultEstimatedToolSchemaTokens = 1024
)

type AssemblyBudget struct {
	EffectiveNumCtx           int
	OutputReserveTokens       int
	ContextReserveTokens      int
	EstimatedSystemTokens     int
	EstimatedToolSchemaTokens int
	EstimatedHistoryTokens    int
	AvailableEvidenceTokens   int
}

type AssemblyEstimate struct {
	SystemTokens     int
	ToolSchemaTokens int
}

func effectiveNumCtx(cfg Config, in Input, model string, history []llm.Message, outputTokenBudget int) int {
	mode := strings.ToLower(strings.TrimSpace(in.ContextMode))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(cfg.ContextMode))
	}
	if mode == "" {
		mode = "auto"
	}
	modelLimit := modelContextLimit(cfg, model)
	switch mode {
	case "small":
		return clampNumCtx(8192, cfg, modelLimit)
	case "large":
		return clampNumCtx(32768, cfg, modelLimit)
	case "max":
		return clampNumCtx(modelLimit, cfg, modelLimit)
	default:
		estimated := estimatePromptTokens(history)
		needed := estimated + outputTokenBudget + cfg.ContextReserve
		target := nextContextTier(needed)
		return clampNumCtx(target, cfg, modelLimit)
	}
}

func modelContextLimit(cfg Config, model string) int {
	if cfg.ContextMaxNumCtx > 0 {
		return cfg.ContextMaxNumCtx
	}
	cap := llm.ModelCapabilities(model)
	if cfg.NumCtx > 0 && cap.RecommendedNumCtx > 0 {
		if cfg.NumCtx > cap.RecommendedNumCtx {
			return cfg.NumCtx
		}
		return cap.RecommendedNumCtx
	}
	if cfg.NumCtx > 0 {
		return cfg.NumCtx
	}
	if cap.RecommendedNumCtx > 0 {
		return cap.RecommendedNumCtx
	}
	return 32768
}

func clampNumCtx(v int, cfg Config, modelLimit int) int {
	if v <= 0 {
		v = modelLimit
	}
	minCtx := cfg.ContextMinNumCtx
	if minCtx <= 0 {
		minCtx = 8192
	}
	if v < minCtx {
		v = minCtx
	}
	if modelLimit > 0 && v > modelLimit {
		v = modelLimit
	}
	if v <= 0 {
		return minCtx
	}
	return v
}

func nextContextTier(needed int) int {
	tiers := []int{8192, 16384, 32768, 65536, 131072}
	for _, t := range tiers {
		if needed <= t {
			return t
		}
	}
	return needed
}

func estimatePromptTokens(history []llm.Message) int {
	totalChars := 0
	for _, m := range history {
		totalChars += len(m.Content)
		totalChars += len(m.Thinking)
		totalChars += 32 // role + structure overhead
		totalChars += len(m.ToolCalls) * 120
	}
	if totalChars <= 0 {
		return 0
	}
	// Rough heuristic: ~4 chars/token for mixed english+code.
	tokens := totalChars / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func BuildAssemblyBudget(cfg Config, in Input, history []llm.Message, est AssemblyEstimate) AssemblyBudget {
	outputTokenBudget := cfg.MaxOutputTokens
	if in.MaxOutputTokens > 0 {
		outputTokenBudget = in.MaxOutputTokens
	}
	if outputTokenBudget <= 0 {
		outputTokenBudget = DefaultConfig().MaxOutputTokens
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = llm.DefaultModel
	}
	effective := effectiveNumCtx(cfg, in, model, history, outputTokenBudget)
	systemTokens := est.SystemTokens
	if systemTokens <= 0 {
		systemTokens = defaultEstimatedSystemTokens
	}
	if strings.TrimSpace(in.SystemPrompt) != "" {
		systemTokens = estimatePromptTokens([]llm.Message{{Role: llm.RoleSystem, Content: in.SystemPrompt}})
		if systemTokens < defaultEstimatedSystemTokens {
			systemTokens = defaultEstimatedSystemTokens
		}
	}
	toolSchemaTokens := est.ToolSchemaTokens
	if toolSchemaTokens <= 0 {
		if strings.EqualFold(strings.TrimSpace(in.ToolMode), ToolModeNone) {
			toolSchemaTokens = 0
		} else {
			toolSchemaTokens = defaultEstimatedToolSchemaTokens
		}
	}
	historyTokens := estimatePromptTokens(history)
	contextReserve := cfg.ContextReserve
	if contextReserve <= 0 {
		contextReserve = DefaultConfig().ContextReserve
	}
	available := effective - outputTokenBudget - contextReserve - systemTokens - toolSchemaTokens - historyTokens
	if available < 0 {
		available = 0
	}
	return AssemblyBudget{
		EffectiveNumCtx:           effective,
		OutputReserveTokens:       outputTokenBudget,
		ContextReserveTokens:      contextReserve,
		EstimatedSystemTokens:     systemTokens,
		EstimatedToolSchemaTokens: toolSchemaTokens,
		EstimatedHistoryTokens:    historyTokens,
		AvailableEvidenceTokens:   available,
	}
}
