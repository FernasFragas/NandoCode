package contextpack

import (
	"context"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

func TestClampToolContextToBudgetShrinksCaps(t *testing.T) {
	ctx := tools.DefaultContext(context.Background(), t.TempDir())
	ctx.MaxPromptBytes = 2 * 1024 * 1024
	ctx.MaxDirBytes = 512 * 1024
	ctx.MaxReadChars = 100_000

	out := ClampToolContextToBudget(ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 10_000})
	if out.MaxPromptBytes != 40_000 {
		t.Fatalf("max prompt bytes=%d want %d", out.MaxPromptBytes, 40_000)
	}
	if out.MaxDirBytes != 40_000 {
		t.Fatalf("max dir bytes=%d want %d", out.MaxDirBytes, 40_000)
	}
	if out.MaxReadChars != 20_000 {
		t.Fatalf("max read chars=%d want %d", out.MaxReadChars, 20_000)
	}
}

func TestClampToolContextToBudgetNoopWhenNoEvidenceBudget(t *testing.T) {
	ctx := tools.DefaultContext(context.Background(), t.TempDir())
	original := ctx
	out := ClampToolContextToBudget(ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 0})
	if out.MaxPromptBytes != original.MaxPromptBytes || out.MaxReadChars != original.MaxReadChars || out.MaxDirBytes != original.MaxDirBytes {
		t.Fatalf("unexpected change with zero budget: before=%+v after=%+v", original, out)
	}
}
