package builtin

import (
	"encoding/json"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

func TestNewRegistry(t *testing.T) {
	reg, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tool := range reg.All() {
		names = append(names, tool.Name())
		if _, err := tools.ToLLMToolDef(tool); err != nil {
			t.Fatalf("ToLLMToolDef(%s): %v", tool.Name(), err)
		}
	}
	want := []string{"Bash", "FileEdit", "FileRead", "FileWrite", "Glob", "Grep", "TodoRead", "TodoWrite", "WebFetch"}
	if len(names) != len(want) {
		t.Fatalf("expected %d tools, got %d: %v", len(want), len(names), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names[%d]: want %q, got %q (all: %v)", i, want[i], names[i], names)
		}
	}
}

type fakeTool struct{}

func (f fakeTool) Name() string                                    { return "FakeSkill" }
func (f fakeTool) Description() string                             { return "Fake skill tool." }
func (f fakeTool) Aliases() []string                               { return nil }
func (f fakeTool) JSONSchema() map[string]any                      { return map[string]any{"type": "object"} }
func (f fakeTool) UnmarshalInput(raw json.RawMessage) (any, error) { return map[string]any{}, nil }
func (f fakeTool) IsEnabled(ctx tools.Context) bool                { return true }
func (f fakeTool) IsReadOnly(input any) bool                       { return true }
func (f fakeTool) IsConcurrencySafe(input any) bool                { return true }
func (f fakeTool) IsDestructive(input any) bool                    { return false }
func (f fakeTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow}
}
func (f fakeTool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	return tools.Result{Display: "ok"}, nil
}
func (f fakeTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "Fake", Summary: "fake"}
}

func TestNewRegistryWithTools(t *testing.T) {
	reg, err := NewRegistryWithTools(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Lookup("FakeSkill"); !ok {
		t.Fatal("expected FakeSkill to be registered")
	}
}

func TestNewReadOnlyRegistry(t *testing.T) {
	reg, err := NewReadOnlyRegistry()
	if err != nil {
		t.Fatal(err)
	}

	expectPresent := []string{"FileRead", "Glob", "Grep"}
	for _, name := range expectPresent {
		if _, ok := reg.Lookup(name); !ok {
			t.Fatalf("expected %s in read-only registry", name)
		}
	}

	expectAbsent := []string{"Bash", "FileWrite", "FileEdit", "WebFetch", "TodoWrite", "TodoRead"}
	for _, name := range expectAbsent {
		if _, ok := reg.Lookup(name); ok {
			t.Fatalf("did not expect %s in read-only registry", name)
		}
	}
}
