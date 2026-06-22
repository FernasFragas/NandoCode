package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type stdioClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID int64
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func startStdioClient(ctx context.Context, cfg ServerConfig) (*stdioClient, error) {
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Env = buildProcessEnv(cfg.Env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mcp server %q: %w", cfg.Name, err)
	}
	c := &stdioClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}
	return c, nil
}

func (c *stdioClient) initialize(ctx context.Context) error {
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

func (c *stdioClient) listTools(ctx context.Context) ([]ToolDescriptor, error) {
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

func (c *stdioClient) callTool(ctx context.Context, name string, args map[string]any) (CallResult, error) {
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

func (c *stdioClient) request(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
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

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := writeFrame(c.stdin, payload); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(45 * time.Second)
	}
	for {
		if time.Now().After(deadline) {
			return nil, context.DeadlineExceeded
		}
		frame, err := readFrame(c.stdout)
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		var resp rpcResponse
		if err := json.Unmarshal(frame, &resp); err != nil {
			continue
		}
		if resp.ID == nil {
			continue
		}
		respID, ok := normalizeID(resp.ID)
		if !ok || respID != id {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func writeFrame(w io.Writer, payload []byte) error {
	header := []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "content-length:") {
			v := strings.TrimSpace(line[len("Content-Length:"):])
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length %q", v)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(payload), nil
}

func normalizeID(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

func (c *stdioClient) Close() error {
	if c == nil {
		return nil
	}
	_ = c.sendNotification("shutdown", map[string]any{})
	_ = c.stdin.Close()
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			return fmt.Errorf("stdio mcp process did not exit after kill")
		}
	}
}

func (c *stdioClient) Initialize(ctx context.Context) error {
	return c.initialize(ctx)
}

func (c *stdioClient) ListTools(ctx context.Context) ([]ToolDescriptor, error) {
	return c.listTools(ctx)
}

func (c *stdioClient) CallTool(ctx context.Context, name string, input json.RawMessage) (CallResult, error) {
	var args map[string]any
	if len(strings.TrimSpace(string(input))) > 0 && string(input) != "null" {
		if err := json.Unmarshal(input, &args); err != nil {
			return CallResult{}, err
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	return c.callTool(ctx, name, args)
}

func (c *stdioClient) sendNotification(method string, params map[string]any) error {
	if c == nil || c.stdin == nil {
		return nil
	}
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeFrame(c.stdin, payload)
}

func buildProcessEnv(overrides map[string]string) []string {
	keys := []string{"PATH", "HOME", "TMPDIR", "USER", "SHELL", "LANG", "TERM"}
	out := make(map[string]string, len(keys)+len(overrides))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			out[key] = value
		}
	}
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	ordered := make([]string, 0, len(out))
	for key := range out {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	env := make([]string, 0, len(ordered))
	for _, key := range ordered {
		env = append(env, key+"="+out[key])
	}
	return env
}
