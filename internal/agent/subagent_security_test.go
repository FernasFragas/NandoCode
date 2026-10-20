package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/bash"
)

func TestSubagentCannotEscalatePlanToBypass(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	client := newFakeClient([]fakeTurn{
		toolCallTurn("Bash", map[string]any{"command": "sh -c 'echo changed > marker'"}),
		textTurn("done"),
	})
	reg := tools.NewRegistry()
	reg.Register(bash.NewBashTool())

	_, _ = RunSubagent(context.Background(), Input{
		Model:          "test",
		ToolContext:    tools.DefaultContext(context.Background(), dir),
		PermissionMode: permissions.ModePlan,
	}, SubagentParams{Task: "write", PermissionMode: permissions.ModeBypass}, client, reg, DefaultConfig())

	if _, err := os.Stat(filepath.Join(dir, "marker")); err == nil {
		t.Fatal("plan-mode parent's sub-agent escalated to bypass and wrote marker")
	}
}
