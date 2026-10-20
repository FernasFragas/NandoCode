package bootstrap

// Overrides are command-line values that win over the loaded config. Empty
// strings and a non-positive NumCtx mean "not set".
type Overrides struct {
	Model     string
	OllamaURL string
	NumCtx    int
}

// ApplyOverrides applies command-line overrides on top of ApplyConfig.
func ApplyOverrides(initial *Initial, o Overrides) {
	if o.Model != "" {
		initial.DefaultModel = o.Model
	}
	if o.OllamaURL != "" {
		initial.OllamaBaseURL = o.OllamaURL
		initial.LLMBaseURL = o.OllamaURL
	}
	if o.NumCtx > 0 {
		initial.NumCtx = o.NumCtx
	}
}
