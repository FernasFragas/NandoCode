package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeTransport struct {
	initErr   error
	listErr   error
	callErr   error
	tools     []ToolDescriptor
	callValue CallResult
	closed    bool
}

func (f *fakeTransport) Initialize(ctx context.Context) error { return f.initErr }
func (f *fakeTransport) ListTools(ctx context.Context) ([]ToolDescriptor, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]ToolDescriptor, len(f.tools))
	copy(out, f.tools)
	return out, nil
}
func (f *fakeTransport) CallTool(ctx context.Context, name string, input json.RawMessage) (CallResult, error) {
	if f.callErr != nil {
		return CallResult{}, f.callErr
	}
	return f.callValue, nil
}
func (f *fakeTransport) Close() error {
	f.closed = true
	return nil
}

func TestClientToolsBeforeConnectEmpty(t *testing.T) {
	t.Parallel()
	c, err := NewClient(ServerConfig{Name: "s", Transport: TransportStdio, Command: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tools()) != 0 {
		t.Fatalf("expected no tools before connect")
	}
}

func TestClientConnectWithFakeTransport(t *testing.T) {
	t.Parallel()
	c, err := NewClient(ServerConfig{Name: "s", Transport: TransportStdio, Command: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	ft := &fakeTransport{tools: []ToolDescriptor{{Name: "list", Description: "List files."}}}
	c.factory = func(context.Context, ServerConfig, func(context.Context) (string, error)) (ClientTransport, error) {
		return ft, nil
	}
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c.Tools()) != 1 || c.Tools()[0].Name != "list" {
		t.Fatalf("unexpected tools after connect: %#v", c.Tools())
	}
}

func TestClientConnectFailure(t *testing.T) {
	t.Parallel()
	c, err := NewClient(ServerConfig{Name: "s", Transport: TransportStdio, Command: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	c.factory = func(context.Context, ServerConfig, func(context.Context) (string, error)) (ClientTransport, error) {
		return &fakeTransport{initErr: errors.New("boom")}, nil
	}
	if err := c.Connect(context.Background()); err == nil {
		t.Fatalf("expected connect error")
	}
}

func TestClientCloseAfterConnect(t *testing.T) {
	t.Parallel()
	c, err := NewClient(ServerConfig{Name: "s", Transport: TransportStdio, Command: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	ft := &fakeTransport{}
	c.factory = func(context.Context, ServerConfig, func(context.Context) (string, error)) (ClientTransport, error) {
		return ft, nil
	}
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if !ft.closed {
		t.Fatalf("expected transport close to be called")
	}
}
