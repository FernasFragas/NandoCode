package tools

import (
	"encoding/json"
	"testing"
)

func TestBuildToolDefaultsFailClosed(t *testing.T) {
	tool := BuildTool(Spec{
		Name:        "Example",
		Description: "Read example data.",
		Schema:      map[string]any{"type": "object"},
		Unmarshal: func(raw json.RawMessage) (any, error) {
			return map[string]any{}, nil
		},
		CallFunc: func(ctx Context, input any, progress chan<- ProgressEvent) (Result, error) {
			return Result{Display: "ok"}, nil
		},
	})

	if !tool.IsEnabled(Context{}) {
		t.Fatal("tool should be enabled by default")
	}
	if tool.IsReadOnly(nil) {
		t.Fatal("tool should not be read-only by default")
	}
	if tool.IsConcurrencySafe(nil) {
		t.Fatal("tool should not be concurrency-safe by default")
	}
	if !tool.IsDestructive(nil) {
		t.Fatal("tool should be destructive by default")
	}
	if got := tool.CheckPermissions(Context{}, nil).Decision; got != PermAsk {
		t.Fatalf("default permission = %s, want ask", got)
	}
	if got := tool.CheckPermissions(Context{PermissionMode: PermissionPlan}, nil).Decision; got != PermDeny {
		t.Fatalf("plan permission = %s, want deny", got)
	}
}

func TestToLLMToolDef(t *testing.T) {
	tool := BuildTool(Spec{
		Name:        "Example",
		Description: "Read example data.",
		Schema:      map[string]any{"type": "object"},
		Unmarshal: func(raw json.RawMessage) (any, error) {
			return nil, nil
		},
		CallFunc: func(ctx Context, input any, progress chan<- ProgressEvent) (Result, error) {
			return Result{}, nil
		},
	})

	def, err := ToLLMToolDef(tool)
	if err != nil {
		t.Fatal(err)
	}
	if def.Type != "function" {
		t.Fatalf("Type = %q", def.Type)
	}
	if def.Function.Name != "Example" {
		t.Fatalf("Name = %q", def.Function.Name)
	}
}

func TestValidateToolRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		tool Tool
	}{
		{name: "nil", tool: nil},
		{name: "empty name", tool: BuildTool(Spec{Description: "Valid.", Schema: map[string]any{"type": "object"}})},
		{name: "nil schema", tool: BuildTool(Spec{Name: "Bad", Description: "Valid."})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTool(tt.tool); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTruncateDisplay(t *testing.T) {
	got, truncated := TruncateDisplay("abcdef", 5)
	if !truncated || len(got) != 5 {
		t.Fatalf("got %q truncated=%v", got, truncated)
	}
	got, truncated = TruncateDisplay("abc", 5)
	if truncated || got != "abc" {
		t.Fatalf("got %q truncated=%v", got, truncated)
	}
}
