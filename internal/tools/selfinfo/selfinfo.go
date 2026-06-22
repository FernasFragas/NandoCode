// Package selfinfo provides a tool for the LLM to query its own runtime configuration.
package selfinfo

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

// Input is the GetConfig tool input.
type Input struct {
	// IncludeModels fetches the full list of available models from Ollama when true.
	IncludeModels bool `json:"include_models,omitempty"`
}

// Tool retrieves live agent configuration and model details from Ollama.
type Tool struct {
	client   llm.Client
	getModel func() string // reads the current active model from the live state store
	maxTurns int
}

// New creates the GetConfig tool.
// getModel is called at each tool invocation so it always reflects the current active model,
// even after a /model switch. maxTurns is an agent-level policy value.
func New(client llm.Client, getModel func() string, maxTurns int) *Tool {
	return &Tool{
		client:   client,
		getModel: getModel,
		maxTurns: maxTurns,
	}
}

func (t *Tool) Name() string              { return "GetConfig" }
func (t *Tool) Aliases() []string         { return []string{"get_config", "self_info"} }
func (t *Tool) IsEnabled(_ tools.Context) bool { return true }
func (t *Tool) IsReadOnly(_ any) bool          { return true }
func (t *Tool) IsConcurrencySafe(_ any) bool   { return true }
func (t *Tool) IsDestructive(_ any) bool       { return false }

func (t *Tool) Description() string {
	return "Return live model configuration from Ollama. Includes context length, family, quantization, and optionally the list of available models."
}

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"include_models": map[string]any{
				"type":        "boolean",
				"description": "When true, also fetch the full list of available models from Ollama.",
			},
		},
	}
}

func (t *Tool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var in Input
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
	}
	return in, nil
}

func (t *Tool) CheckPermissions(_ tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}

func (t *Tool) Call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in, _ := input.(Input)
	currentModel := t.getModel()
	lines := []string{}

	lines = append(lines, fmt.Sprintf("current_model: %s", currentModel))
	lines = append(lines, fmt.Sprintf("max_turns: %d", t.maxTurns))
	lines = append(lines, fmt.Sprintf("permission_mode: %s", string(ctx.PermissionMode)))
	lines = append(lines, fmt.Sprintf("max_result_chars: %d", ctx.EffectiveMaxResultChars()))
	lines = append(lines, fmt.Sprintf("max_read_chars: %d", ctx.EffectiveMaxReadChars()))

	// Fetch live model details from Ollama
	if t.client != nil {
		details, err := t.client.ShowModel(ctx.EffectiveContext(), currentModel)
		if err != nil {
			lines = append(lines, fmt.Sprintf("model_details_error: %v", err))
		} else {
			if details.Family != "" {
				lines = append(lines, fmt.Sprintf("model_family: %s", details.Family))
			}
			if details.ParameterSize != "" {
				lines = append(lines, fmt.Sprintf("model_parameter_size: %s", details.ParameterSize))
			}
			if details.QuantizationLevel != "" {
				lines = append(lines, fmt.Sprintf("model_quantization: %s", details.QuantizationLevel))
			}
			if details.ContextLength > 0 {
				lines = append(lines, fmt.Sprintf("context_length (num_ctx): %d tokens", details.ContextLength))
			}
			// Show additional parameters from the model (stop tokens, temperature, etc.)
			for k, v := range details.Parameters {
				if k == "num_ctx" {
					continue // already shown above as context_length
				}
				lines = append(lines, fmt.Sprintf("model_param.%s: %v", k, v))
			}
		}
	}

	// Optionally fetch available models
	if in.IncludeModels && t.client != nil {
		models, err := t.client.ListModels(ctx.EffectiveContext())
		if err != nil {
			lines = append(lines, fmt.Sprintf("available_models_error: %v", err))
		} else {
			names := make([]string, len(models))
			for i, m := range models {
				names[i] = m.Name
			}
			lines = append(lines, "available_models: "+strings.Join(names, ", "))
		}
	}

	return tools.Result{
		Display: strings.Join(lines, "\n"),
	}, nil
}

func (t *Tool) Render(_ any, _ tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "GetConfig", Summary: t.getModel()}
}
