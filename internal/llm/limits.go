package llm

const (
	// charsPerToken is the average characters per token used for limit calculations.
	charsPerToken = 4

	// fallbackMaxOutputTokens is used when the model details provide no usable value.
	fallbackMaxOutputTokens = 8192
)

// ModelLimits holds system limits derived from a model's actual capabilities.
type ModelLimits struct {
	// MaxOutputTokens is the maximum tokens the model generates per turn (num_predict).
	MaxOutputTokens int
	// MaxResultChars is the tool-result char budget fed back to the LLM.
	MaxResultChars int
	// NumCtx is the full context window size to request from Ollama.
	NumCtx int
}

// ComputeLimits derives system limits from live model details returned by ShowModel.
// Priority for MaxOutputTokens: num_predict parameter > half of context_length > fallback.
// MaxResultChars is set to MaxOutputTokens * charsPerToken so tool output never exceeds
// what the model can read back in a single response.
func ComputeLimits(d ModelDetails) ModelLimits {
	maxOutput := 0

	// Prefer num_predict from model parameters (the model's configured generation cap).
	if v, ok := d.Parameters["num_predict"]; ok {
		switch n := v.(type) {
		case float64:
			maxOutput = int(n)
		case int64:
			maxOutput = int(n)
		}
	}

	// Fall back to half the context window when num_predict is absent or zero.
	if maxOutput <= 0 && d.ContextLength > 0 {
		maxOutput = int(d.ContextLength / 2)
	}

	if maxOutput <= 0 {
		maxOutput = fallbackMaxOutputTokens
	}

	numCtx := int(d.ContextLength)

	return ModelLimits{
		MaxOutputTokens: maxOutput,
		MaxResultChars:  maxOutput * charsPerToken,
		NumCtx:          numCtx,
	}
}
