package tools

import (
	"encoding/json"
	"testing"
)

func testTool(name string, aliases ...string) Tool {
	return BuildTool(Spec{
		Name:        name,
		Description: "Read test data.",
		Aliases:     aliases,
		Schema:      map[string]any{"type": "object"},
		Unmarshal: func(raw json.RawMessage) (any, error) {
			return nil, nil
		},
		CallFunc: func(ctx Context, input any, progress chan<- ProgressEvent) (Result, error) {
			return Result{}, nil
		},
	})
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(testTool("B", "Bee")); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(testTool("A")); err != nil {
		t.Fatal(err)
	}
	if got, ok := reg.Lookup("Bee"); !ok || got.Name() != "B" {
		t.Fatalf("alias lookup failed: %v %v", got, ok)
	}
	all := reg.All()
	if len(all) != 2 || all[0].Name() != "A" || all[1].Name() != "B" {
		t.Fatalf("unexpected order: %#v", all)
	}
	if err := reg.Register(testTool("A")); err == nil {
		t.Fatal("expected duplicate name error")
	}
	if err := reg.Register(testTool("C", "Bee")); err == nil {
		t.Fatal("expected duplicate alias error")
	}
}
