package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

type MCPTool struct {
	serverName string
	serverTool ToolDescriptor
	client     *Client
}

func NewMCPTool(serverName string, desc ToolDescriptor, client *Client) *MCPTool {
	return &MCPTool{
		serverName: serverName,
		serverTool: desc,
		client:     client,
	}
}

func (a *MCPTool) Name() string {
	return toolName(a.serverName, a.serverTool.Name)
}

func (a *MCPTool) Description() string {
	return mcpToolDescription(a.serverTool.Description, a.serverName)
}

func (a *MCPTool) Aliases() []string { return nil }

func (a *MCPTool) JSONSchema() map[string]any {
	if a.serverTool.InputSchema == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return a.serverTool.InputSchema
}

func (a *MCPTool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var v map[string]any
	if len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func (a *MCPTool) IsEnabled(ctx tools.Context) bool { return true }
func (a *MCPTool) IsReadOnly(input any) bool        { return false }
func (a *MCPTool) IsConcurrencySafe(input any) bool { return false }
func (a *MCPTool) IsDestructive(input any) bool     { return true }

func (a *MCPTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAsk, Reason: "mcp tool requires permission", UpdatedInput: input}
}

func (a *MCPTool) Call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	args, _ := input.(map[string]any)
	raw, err := json.Marshal(args)
	if err != nil {
		return tools.Result{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx.EffectiveContext(), 45*time.Second)
	defer cancel()
	result, err := a.client.CallTool(callCtx, a.serverTool.Name, raw)
	if err != nil {
		return tools.Result{}, err
	}
	lines := renderMCPContent(result.Content)
	if len(lines) == 0 {
		lines = append(lines, "MCP tool completed with no content.")
	}
	display := strings.Join(lines, "\n")
	if result.IsError {
		return tools.Result{Display: display}, fmt.Errorf("mcp tool reported error")
	}
	return tools.Result{Display: display}, nil
}

func (a *MCPTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{
		Title:   a.Name(),
		Summary: a.serverTool.Name,
	}
}

func renderMCPContent(content []map[string]any) []string {
	lines := make([]string, 0, len(content)+1)
	for _, item := range content {
		typ, _ := item["type"].(string)
		switch typ {
		case "text":
			if text, _ := item["text"].(string); strings.TrimSpace(text) != "" {
				lines = append(lines, text)
			}
		case "image":
			mime, _ := item["mimeType"].(string)
			if strings.TrimSpace(mime) == "" {
				mime = "unknown"
			}
			lines = append(lines, "[MCP image content omitted: "+mime+"]")
		case "resource":
			uri, _ := item["uri"].(string)
			if strings.TrimSpace(uri) == "" {
				if res, ok := item["resource"].(map[string]any); ok {
					uri, _ = res["uri"].(string)
				}
			}
			if strings.TrimSpace(uri) == "" {
				uri = "unknown"
			}
			lines = append(lines, "[MCP resource content omitted: "+uri+"]")
		default:
			j, _ := json.Marshal(item)
			lines = append(lines, "[MCP non-text content omitted: "+string(j)+"]")
		}
	}
	return lines
}

func mcpToolDescription(raw, serverName string) string {
	desc := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if desc == "" {
		desc = "MCP tool from server " + serverName
	}
	if len(desc) > 96 {
		desc = strings.TrimSpace(desc[:96]) + "..."
	}
	if !strings.HasSuffix(desc, ".") {
		desc += "."
	}
	return desc
}
