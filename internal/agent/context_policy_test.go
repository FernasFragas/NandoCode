package agent

import (
	"testing"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

func TestEffectiveNumCtxModes(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ContextMinNumCtx = 8192
	cfg.ContextMaxNumCtx = 65536
	cfg.ContextReserve = 4096
	history := []llm.Message{{Role: llm.RoleUser, Content: "hello"}}

	if got := effectiveNumCtx(cfg, Input{ContextMode: "small"}, "qwen3", history, 1024); got != 8192 {
		t.Fatalf("small mode num_ctx=%d", got)
	}
	if got := effectiveNumCtx(cfg, Input{ContextMode: "large"}, "qwen3", history, 1024); got != 32768 {
		t.Fatalf("large mode num_ctx=%d", got)
	}
	if got := effectiveNumCtx(cfg, Input{ContextMode: "max"}, "qwen3", history, 1024); got != 65536 {
		t.Fatalf("max mode num_ctx=%d", got)
	}
}

func TestEffectiveNumCtxAutoScales(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ContextMode = "auto"
	cfg.ContextMinNumCtx = 8192
	cfg.ContextMaxNumCtx = 32768
	cfg.ContextReserve = 4096

	small := []llm.Message{{Role: llm.RoleUser, Content: "small"}}
	large := []llm.Message{{Role: llm.RoleUser, Content: string(make([]byte, 120000))}}

	smallCtx := effectiveNumCtx(cfg, Input{}, "qwen3", small, 1024)
	largeCtx := effectiveNumCtx(cfg, Input{}, "qwen3", large, 1024)
	if smallCtx >= largeCtx {
		t.Fatalf("expected auto mode to scale up: small=%d large=%d", smallCtx, largeCtx)
	}
}

func TestEffectiveNumCtxUsesLiveLimitAboveStaticRecommendation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ContextMinNumCtx = 8192
	cfg.ContextMaxNumCtx = 0
	cfg.NumCtx = 131072
	cfg.ContextReserve = 4096
	history := []llm.Message{{Role: llm.RoleUser, Content: string(make([]byte, 300000))}}

	got := effectiveNumCtx(cfg, Input{ContextMode: "max"}, llm.DefaultModel, history, 1024)
	if got != 131072 {
		t.Fatalf("max mode num_ctx=%d, want live/config limit 131072", got)
	}
}

func TestBuildAssemblyBudgetUsesEffectiveNumCtxAndReserves(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NumCtx = 131072
	cfg.ContextMode = "max"
	cfg.ContextReserve = 4096
	cfg.MaxOutputTokens = 2048
	in := Input{
		Model:       llm.DefaultModel,
		ContextMode: "max",
	}
	budget := BuildAssemblyBudget(cfg, in, nil, AssemblyEstimate{})
	if budget.EffectiveNumCtx != 131072 {
		t.Fatalf("effective num_ctx=%d want 131072", budget.EffectiveNumCtx)
	}
	if budget.OutputReserveTokens != 2048 {
		t.Fatalf("output reserve=%d want 2048", budget.OutputReserveTokens)
	}
	if budget.ContextReserveTokens != 4096 {
		t.Fatalf("context reserve=%d want 4096", budget.ContextReserveTokens)
	}
	if budget.EstimatedSystemTokens <= 0 || budget.EstimatedToolSchemaTokens <= 0 {
		t.Fatalf("expected non-zero estimates: %+v", budget)
	}
	if budget.AvailableEvidenceTokens <= 0 {
		t.Fatalf("expected positive evidence budget: %+v", budget)
	}
}

func TestBuildAssemblyBudgetHonorsInputMaxOutputOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ContextMode = "max"
	cfg.NumCtx = 65536
	cfg.MaxOutputTokens = 2048
	in := Input{
		Model:           llm.DefaultModel,
		ContextMode:     "max",
		MaxOutputTokens: 1024,
	}
	budget := BuildAssemblyBudget(cfg, in, nil, AssemblyEstimate{SystemTokens: 100, ToolSchemaTokens: 200})
	if budget.OutputReserveTokens != 1024 {
		t.Fatalf("output reserve=%d want 1024", budget.OutputReserveTokens)
	}
}

func TestBuildAssemblyBudgetZeroToolSchemaForToolModeNone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ContextMode = "max"
	cfg.NumCtx = 65536
	cfg.MaxOutputTokens = 2048
	in := Input{
		Model:       llm.DefaultModel,
		ContextMode: "max",
		ToolMode:    ToolModeNone,
	}
	budget := BuildAssemblyBudget(cfg, in, nil, AssemblyEstimate{})
	if budget.EstimatedToolSchemaTokens != 0 {
		t.Fatalf("tool schema tokens=%d want 0", budget.EstimatedToolSchemaTokens)
	}
}
