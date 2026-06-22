package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

type recallResponse struct {
	Selected []string `json:"selected"`
}

// Recall chooses relevant memories with an LLM side-query over metadata only.
func Recall(ctx context.Context, client llm.Client, cfg Config, query Query, entries []Entry, alreadyLoaded map[string]bool) (RecallResult, error) {
	if len(entries) == 0 || client == nil {
		return RecallResult{}, nil
	}

	var manifest strings.Builder
	index := make(map[string]Entry, len(entries))
	for _, e := range entries {
		if alreadyLoaded != nil && alreadyLoaded[e.Filename] {
			continue
		}
		index[e.Filename] = e
		manifest.WriteString(fmt.Sprintf("- %s | type=%s | name=%s | updated=%s | description=%s\n",
			e.Filename, e.Type, e.Name, e.UpdatedAt.UTC().Format("2006-01-02"), e.Description))
	}
	if manifest.Len() == 0 {
		return RecallResult{}, nil
	}

	userPrompt := "User request:\n" + query.LatestUser + "\n\nMemory manifest:\n" + manifest.String()
	req := &llm.ChatRequest{
		Model: cfg.Model,
		Messages: []llm.Message{
			{
				Role:    llm.RoleSystem,
				Content: "Select only useful memory filenames for this request. Return JSON {\"selected\":[...]} with at most 5 entries. Skip if uncertain.",
			},
			{Role: llm.RoleUser, Content: userPrompt},
		},
		Format: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"selected": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
				},
			},
			"required": []string{"selected"},
		},
		Stream: false,
	}

	events, err := client.Chat(ctx, req)
	if err != nil {
		return RecallResult{}, err
	}
	var out strings.Builder
	for evt := range events {
		out.WriteString(evt.Message.Content)
		if evt.Done && strings.HasPrefix(evt.DoneReason, "stream_error:") {
			return RecallResult{}, errors.New(evt.DoneReason)
		}
	}

	var parsed recallResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &parsed); err != nil {
		return RecallResult{
			Warnings: []string{fmt.Sprintf("recall parse failed: %v", err)},
		}, nil
	}
	max := cfg.MaxSelected
	if max <= 0 {
		max = 5
	}

	selected := make([]Entry, 0, max)
	warnings := []string{}
	seen := map[string]bool{}
	for _, name := range parsed.Selected {
		if len(selected) >= max {
			break
		}
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		entry, ok := index[name]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("ignored unknown recalled filename %q", name))
			continue
		}
		selected = append(selected, entry)
	}
	return RecallResult{
		Selected: selected,
		Warnings: warnings,
	}, nil
}
