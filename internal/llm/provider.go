package llm

// Provider identifies the active model backend.
type Provider string

const (
	ProviderOllamaLocal    Provider = "ollama_local"
	ProviderOllamaCloudAPI Provider = "ollama_cloud_api"
)

// ModelOrigin identifies where a model name is resolved from.
type ModelOrigin string

const (
	ModelOriginLocal          ModelOrigin = "local"
	ModelOriginOllamaCloudAPI ModelOrigin = "ollama_cloud_api"
)

const (
	// OllamaCloudBaseURL is the direct Ollama Cloud API host.
	OllamaCloudBaseURL = "https://ollama.com"
)

// ResolvedModel captures model resolution details.
type ResolvedModel struct {
	RequestedName string
	Model         string
	Origin        ModelOrigin
	Provider      Provider
	BaseURL       string
	AliasUsed     bool
	AliasReason   string
}
