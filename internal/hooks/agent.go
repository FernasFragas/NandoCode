package hooks

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

func runAgentHook(ctx context.Context, client llm.Client, h Hook, env Envelope, cfg Config) Result {
	if client == nil {
		return Result{Warning: "agent hook skipped: llm client is nil"}
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
			{Role: llm.RoleSystem, Content: "Return JSON only: {\"decision\":\"allow|deny|ask\",\"reason\":\"...\"}"},
			{Role: llm.RoleUser, Content: h.Prompt + "\n\nHook input JSON:\n" + string(payload)},
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
		return Result{Decision: DecisionAllow, Warning: "agent hook failed open: " + sanitize(err.Error())}
	}
	var b strings.Builder
	for evt := range stream {
		b.WriteString(evt.Message.Content)
	}
	res := parseHookJSON(b.String())
	if !res.Decision.Valid() || res.Decision == DecisionNone {
		return Result{Decision: DecisionAllow, Warning: "agent hook parse failed open"}
	}
	return res
}
