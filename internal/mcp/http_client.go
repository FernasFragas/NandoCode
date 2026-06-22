package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

type httpClient struct {
	baseURL     string
	client      *http.Client
	nextID      int64
	tokenGetter func(context.Context) (string, error)
}

func startHTTPClient(cfg ServerConfig, tokenGetter func(context.Context) (string, error)) (*httpClient, error) {
	if err := ValidateHTTPDestination(cfg.URL, false); err != nil {
		return nil, err
	}
	return &httpClient{
		baseURL:     cfg.URL,
		tokenGetter: tokenGetter,
		client:      NewSafeHTTPClient(false, 45*time.Second),
	}, nil
}

func (c *httpClient) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "nandocodego",
			"version": "dev",
		},
	}
	_, err := c.request(ctx, "initialize", params)
	return err
}

func (c *httpClient) listTools(ctx context.Context) ([]ToolDescriptor, error) {
	raw, err := c.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Tools []ToolDescriptor `json:"tools"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse tools/list: %w", err)
	}
	return payload.Tools, nil
}

func (c *httpClient) callTool(ctx context.Context, name string, args map[string]any) (CallResult, error) {
	raw, err := c.request(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return CallResult{}, err
	}
	var result CallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return CallResult{}, fmt.Errorf("parse tools/call: %w", err)
	}
	return result, nil
}

func (c *httpClient) request(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.tokenGetter != nil {
		token, err := c.tokenGetter(ctx)
		if err != nil {
			return nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	rpcResp, err := decodeHTTPRPCResponse(resp)
	if err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

func (c *httpClient) Close() error { return nil }

func (c *httpClient) Initialize(ctx context.Context) error {
	return c.initialize(ctx)
}

func (c *httpClient) ListTools(ctx context.Context) ([]ToolDescriptor, error) {
	return c.listTools(ctx)
}

func (c *httpClient) CallTool(ctx context.Context, name string, input json.RawMessage) (CallResult, error) {
	var args map[string]any
	if len(bytes.TrimSpace(input)) > 0 && string(bytes.TrimSpace(input)) != "null" {
		if err := json.Unmarshal(input, &args); err != nil {
			return CallResult{}, err
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	return c.callTool(ctx, name, args)
}

func decodeHTTPRPCResponse(resp *http.Response) (rpcResponse, error) {
	ct := resp.Header.Get("Content-Type")
	if !containsToken(ct, "text/event-stream") {
		var out rpcResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return rpcResponse{}, err
		}
		return out, nil
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return rpcResponse{}, err
	}
	lines := bytes.Split(raw, []byte{'\n'})
	var payload []byte
	for _, ln := range lines {
		ln = bytes.TrimSpace(ln)
		if len(ln) == 0 || !bytes.HasPrefix(ln, []byte("data:")) {
			continue
		}
		payload = bytes.TrimSpace(bytes.TrimPrefix(ln, []byte("data:")))
	}
	if len(payload) == 0 {
		return rpcResponse{}, fmt.Errorf("empty sse payload")
	}
	var out rpcResponse
	if err := json.Unmarshal(payload, &out); err != nil {
		return rpcResponse{}, err
	}
	return out, nil
}

func containsToken(header, tok string) bool {
	for _, p := range bytes.Split([]byte(header), []byte{';'}) {
		if string(bytes.TrimSpace(bytes.ToLower(p))) == tok {
			return true
		}
	}
	return false
}
