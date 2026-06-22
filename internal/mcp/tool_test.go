package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

func TestMCPToolName(t *testing.T) {
	t.Parallel()
	tool := NewMCPTool("File System", ToolDescriptor{Name: "Read-File"}, &Client{})
	if got, want := tool.Name(), "mcp__file_system__read_file"; got != want {
		t.Fatalf("name mismatch: got=%q want=%q", got, want)
	}
}

func TestMCPToolCallTextResult(t *testing.T) {
	t.Parallel()
	client := &Client{
		transport: &fakeTransport{
			callValue: CallResult{
				Content: []map[string]any{{"type": "text", "text": "ok"}},
			},
		},
	}
	tool := NewMCPTool("fs", ToolDescriptor{Name: "list"}, client)
	result, err := tool.Call(tools.Context{Context: context.Background()}, map[string]any{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Display != "ok" {
		t.Fatalf("unexpected display: %q", result.Display)
	}
}

func TestMCPToolCallErrorResult(t *testing.T) {
	t.Parallel()
	client := &Client{
		transport: &fakeTransport{
			callValue: CallResult{
				IsError: true,
				Content: []map[string]any{{"type": "text", "text": "failed"}},
			},
		},
	}
	tool := NewMCPTool("fs", ToolDescriptor{Name: "list"}, client)
	_, err := tool.Call(tools.Context{Context: context.Background()}, map[string]any{}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestMCPToolCallTransportError(t *testing.T) {
	t.Parallel()
	client := &Client{
		transport: &fakeTransport{callErr: errors.New("boom")},
	}
	tool := NewMCPTool("fs", ToolDescriptor{Name: "list"}, client)
	_, err := tool.Call(tools.Context{Context: context.Background()}, map[string]any{}, nil)
	if err == nil {
		t.Fatalf("expected transport error")
	}
}

func TestMCPToolUnmarshalInput(t *testing.T) {
	t.Parallel()
	tool := NewMCPTool("fs", ToolDescriptor{Name: "list"}, &Client{})
	got, err := tool.UnmarshalInput(json.RawMessage(`{"path":"."}`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got.(map[string]any)
	if !ok || m["path"] != "." {
		t.Fatalf("unexpected parsed input: %#v", got)
	}
}
