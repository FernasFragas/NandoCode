package agent

import (
	"context"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

func runFork(
	ctx context.Context,
	parentConversation []llm.Message,
	parentInput Input,
	forkPrompt string,
	registry *tools.Registry,
	client llm.Client,
	baseConfig Config,
) (string, error) {
	prompt := strings.TrimSpace(forkPrompt)
	if prompt == "" {
		prompt = "You are reviewing the conversation above. Provide analysis and suggestions."
	}
	params := SubagentParams{
		Mode:         SpawnFork,
		SystemPrompt: prompt,
		Task:         "Review the conversation and provide concise recommendations.",
		Messages:     append([]llm.Message(nil), parentConversation...),
	}
	p := parentInput
	p.StopHook = nil
	return runSubagent(ctx, p, params, client, registry, baseConfig)
}
