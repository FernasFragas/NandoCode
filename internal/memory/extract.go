package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

type extractResponse struct {
	Drafts []struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	} `json:"drafts"`
}

// ExtractDrafts generates pending memory draft files from conversation history.
func ExtractDrafts(ctx context.Context, client llm.Client, cfg Config, conversation []llm.Message, manifest []Entry) ([]Draft, error) {
	if client == nil || len(conversation) == 0 {
		return nil, nil
	}
	var convo strings.Builder
	for _, m := range conversation {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		convo.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}
	if convo.Len() == 0 {
		return nil, nil
	}
	var known strings.Builder
	for _, e := range manifest {
		known.WriteString(fmt.Sprintf("- %s: %s\n", e.Filename, e.Description))
	}
	req := &llm.ChatRequest{
		Model: cfg.Model,
		Messages: []llm.Message{
			{
				Role: llm.RoleSystem,
				Content: "Propose durable memories only. Use types user|feedback|project|reference. " +
					"Return JSON {\"drafts\":[{\"filename\":\"...md\",\"content\":\"markdown with yaml frontmatter\"}]}. " +
					"Do not include secrets. Return empty drafts when nothing durable exists.",
			},
			{
				Role:    llm.RoleUser,
				Content: "Known memories:\n" + known.String() + "\nConversation:\n" + convo.String(),
			},
		},
		Format: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"drafts": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"filename": map[string]any{"type": "string"},
							"content":  map[string]any{"type": "string"},
						},
						"required": []string{"filename", "content"},
					},
				},
			},
			"required": []string{"drafts"},
		},
		Stream: false,
	}

	stream, err := client.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	var raw strings.Builder
	for evt := range stream {
		raw.WriteString(evt.Message.Content)
		if evt.Done && strings.HasPrefix(evt.DoneReason, "stream_error:") {
			return nil, errors.New(evt.DoneReason)
		}
	}

	var parsed extractResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw.String())), &parsed); err != nil {
		return nil, nil
	}
	drafts := make([]Draft, 0, len(parsed.Drafts))
	for _, d := range parsed.Drafts {
		name := strings.TrimSpace(d.Filename)
		if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
			name = time.Now().UTC().Format("20060102T150405Z") + "-memory.md"
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			name += ".md"
		}
		content := strings.TrimSpace(d.Content)
		if content == "" {
			continue
		}
		drafts = append(drafts, Draft{Filename: name, Content: content + "\n"})
	}
	return drafts, nil
}
