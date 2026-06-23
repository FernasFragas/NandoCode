package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/mcp/auth"
)

type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type CallResult struct {
	Content []map[string]any `json:"content"`
	IsError bool             `json:"isError"`
}

type ClientTransport interface {
	Initialize(ctx context.Context) error
	ListTools(ctx context.Context) ([]ToolDescriptor, error)
	CallTool(ctx context.Context, name string, input json.RawMessage) (CallResult, error)
	Close() error
}

type transportFactory func(context.Context, ServerConfig, func(context.Context) (string, error)) (ClientTransport, error)

type Client struct {
	cfg       ServerConfig
	transport ClientTransport
	tools     []ToolDescriptor
	factory   transportFactory
}

func NewClient(cfg ServerConfig) (*Client, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("server name is required")
	}
	switch cfg.Transport {
	case TransportStdio:
		if strings.TrimSpace(cfg.Command) == "" {
			return nil, fmt.Errorf("stdio server %q missing command", cfg.Name)
		}
	case TransportHTTP:
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, fmt.Errorf("http server %q missing url", cfg.Name)
		}
	default:
		return nil, fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
	return &Client{
		cfg:     cfg,
		factory: defaultTransportFactory,
	}, nil
}

func (c *Client) Connect(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("nil mcp client")
	}
	if c.transport == nil {
		t, err := c.factory(ctx, c.cfg, c.tokenGetter())
		if err != nil {
			return err
		}
		c.transport = t
	}
	if err := c.transport.Initialize(ctx); err != nil {
		return err
	}
	tools, err := c.transport.ListTools(ctx)
	if err != nil {
		return err
	}
	c.tools = append(c.tools[:0], tools...)
	return nil
}

func (c *Client) Tools() []ToolDescriptor {
	if c == nil {
		return nil
	}
	out := make([]ToolDescriptor, len(c.tools))
	copy(out, c.tools)
	return out
}

func (c *Client) CallTool(ctx context.Context, name string, input json.RawMessage) (CallResult, error) {
	if c == nil || c.transport == nil {
		return CallResult{}, fmt.Errorf("mcp client is not connected")
	}
	return c.transport.CallTool(ctx, name, input)
}

func (c *Client) Close() error {
	if c == nil || c.transport == nil {
		return nil
	}
	return c.transport.Close()
}

func (c *Client) tokenGetter() func(context.Context) (string, error) {
	if strings.ToLower(strings.TrimSpace(c.cfg.Auth)) != "oauth" {
		return nil
	}
	flow := &auth.PKCEFlow{
		ServerID:    c.cfg.Name,
		ResourceURL: c.cfg.URL,
		Store:       auth.NewKeyringStore(),
	}
	return flow.EnsureToken
}

func defaultTransportFactory(ctx context.Context, cfg ServerConfig, tokenGetter func(context.Context) (string, error)) (ClientTransport, error) {
	switch cfg.Transport {
	case TransportStdio:
		return startStdioClient(ctx, cfg)
	case TransportHTTP:
		return startHTTPClient(cfg, tokenGetter)
	default:
		return nil, fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
}
