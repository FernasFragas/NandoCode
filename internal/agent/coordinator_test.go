package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

type coordinatorTestTool struct {
	name string
}

func (t coordinatorTestTool) Name() string                                      { return t.name }
func (t coordinatorTestTool) Description() string                               { return "test" }
func (t coordinatorTestTool) JSONSchema() map[string]any                        { return tools.ObjectSchema(nil, nil) }
func (t coordinatorTestTool) UnmarshalInput(raw json.RawMessage) (any, error)   { return struct{}{}, nil }
func (t coordinatorTestTool) IsEnabled(ctx tools.Context) bool                  { return true }
func (t coordinatorTestTool) IsReadOnly(input any) bool                         { return true }
func (t coordinatorTestTool) IsConcurrencySafe(input any) bool                  { return true }
func (t coordinatorTestTool) IsDestructive(input any) bool                      { return false }
func (t coordinatorTestTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}
func (t coordinatorTestTool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	return tools.Result{Display: "ok"}, nil
}
func (t coordinatorTestTool) Render(input any, result tools.Result) tools.RenderHints { return tools.RenderHints{} }
func (t coordinatorTestTool) Aliases() []string                                       { return nil }

func TestIsCoordinatorMode(t *testing.T) {
	t.Setenv("NANDOCODEGO_COORDINATOR", "1")
	if !IsCoordinatorMode() {
		t.Fatal("expected coordinator mode enabled")
	}
	t.Setenv("NANDOCODEGO_COORDINATOR", "false")
	if IsCoordinatorMode() {
		t.Fatal("expected coordinator mode disabled")
	}
}

func TestBuildCoordinatorRegistryHasThreeTools(t *testing.T) {
	r := BuildCoordinatorRegistry(
		coordinatorTestTool{name: "Agent"},
		coordinatorTestTool{name: "SendMessage"},
		coordinatorTestTool{name: "TaskStop"},
	)
	if got := len(r.All()); got != 3 {
		t.Fatalf("len(registry)=%d want=3", got)
	}
}

func TestBuildWorkerRegistryExcludesCoordinatorInternalTools(t *testing.T) {
	full := tools.NewRegistry()
	_ = full.Register(coordinatorTestTool{name: "Agent"})
	_ = full.Register(coordinatorTestTool{name: "SendMessage"})
	_ = full.Register(coordinatorTestTool{name: "TaskStop"})
	_ = full.Register(coordinatorTestTool{name: "FileRead"})
	workers := BuildWorkerRegistry(full)
	if _, ok := workers.Lookup("Agent"); ok {
		t.Fatal("Agent should be excluded")
	}
	if _, ok := workers.Lookup("SendMessage"); ok {
		t.Fatal("SendMessage should be excluded")
	}
	if _, ok := workers.Lookup("FileRead"); !ok {
		t.Fatal("FileRead should remain available")
	}
}

func TestBuildCoordinatorSystemPromptContainsRequiredPhrases(t *testing.T) {
	prompt := BuildCoordinatorSystemPrompt([]string{"Bash", "FileRead"}, "/tmp/scratch")
	if !strings.Contains(strings.ToLower(prompt), "never delegate understanding") {
		t.Fatal("missing required phrase")
	}
	if !strings.Contains(prompt, "Research") || !strings.Contains(prompt, "Synthesize") || !strings.Contains(prompt, "Implement") || !strings.Contains(prompt, "Verify") {
		t.Fatal("missing required workflow sections")
	}
	if !strings.Contains(prompt, "Bash, FileRead") {
		t.Fatal("missing worker tool list")
	}
}

func TestReadCoordinatorConfigMaxWorkersClamp(t *testing.T) {
	t.Setenv("NANDOCODEGO_COORDINATOR_MAX_WORKERS", "10")
	cfg := ReadCoordinatorConfig()
	if cfg.MaxWorkers != maxCoordinatorMaxWorkers {
		t.Fatalf("max workers=%d want=%d", cfg.MaxWorkers, maxCoordinatorMaxWorkers)
	}
}
