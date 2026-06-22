package hooks

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

func runPromptHook(ctx context.Context, client llm.Client, h Hook, env Envelope, cfg Config) Result {
	if client == nil {
		return Result{Warning: "prompt hook skipped: llm client is nil"}
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return Result{Warning: "failed to encode hook input: " + err.Error()}
	}
	runCtx, cancel := context.WithTimeout(ctx, h.Timeout(cfg.DefaultTimeout))
	defer cancel()

	req := &llm.ChatRequest{
		Model: cfg.Model,
		Messages: []llm.Message{
			{
				Role:    llm.RoleSystem,
				Content: "You are a hook policy checker. Return JSON with decision allow, deny, ask, or omit decision for no decision.",
			},
			{
				Role:    llm.RoleUser,
				Content: h.Prompt + "\n\nHook input JSON:\n" + string(payload),
			},
		},
		Format: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"decision": map[string]any{"type": "string"},
				"reason":   map[string]any{"type": "string"},
			},
		},
		Stream: false,
	}
	stream, err := client.Chat(runCtx, req)
	if err != nil {
		return Result{Warning: "prompt hook failed: " + err.Error()}
	}
	var body strings.Builder
	for evt := range stream {
		body.WriteString(evt.Message.Content)
	}
	return parseHookJSON(body.String())
}

func parseHookJSON(raw string) Result {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Result{}
	}
	var res Result
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return Result{Warning: "failed to parse hook JSON: " + err.Error()}
	}
	if !res.Decision.Valid() {
		return Result{Warning: "invalid hook decision: " + string(res.Decision)}
	}
	res.Reason = sanitize(res.Reason)
	res.Warning = sanitize(res.Warning)
	res.AdditionalContext = sanitize(res.AdditionalContext)
	if len(res.UpdatedInput) > 0 && strings.TrimSpace(string(res.UpdatedInput)) != "null" {
		if res.Warning == "" {
			res.Warning = "hook updated_input ignored in Phase 9"
		} else {
			res.Warning += "; hook updated_input ignored in Phase 9"
		}
		res.UpdatedInput = nil
	}
	return res
}
