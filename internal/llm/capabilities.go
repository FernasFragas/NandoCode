// Package llm provides the LLM client interface and types for interacting with Ollama.
package llm

import (
	"strings"
)

// Capabilities represents the capabilities of a specific model.
type Capabilities struct {
	SupportsTools     bool
	SupportsThinking  bool
	SupportsImages    bool
	RecommendedNumCtx int
}

// ModelCapabilities returns the capabilities for a given model name.
// This is a hardcoded matrix that should be updated quarterly.
func ModelCapabilities(modelName string) Capabilities {
	// Normalize model name (remove version tags, size variants)
	normalized := normalizeModelName(modelName)

	// Hardcoded capability matrix based on Ollama model testing
	switch normalized {
	case "qwen3.6":
		return Capabilities{
			SupportsTools:     true,
			SupportsThinking:  true,
			SupportsImages:    true,
			RecommendedNumCtx: 65536,
		}

	case "qwen3":
		return Capabilities{
			SupportsTools:     true,
			SupportsThinking:  false,
			SupportsImages:    false,
			RecommendedNumCtx: 32768,
		}

	case "qwen3-thinking":
		return Capabilities{
			SupportsTools:     true,
			SupportsThinking:  true,
			SupportsImages:    false,
			RecommendedNumCtx: 32768,
		}

	case "llama3.1", "llama3.2":
		return Capabilities{
			SupportsTools:     true,
			SupportsThinking:  false,
			SupportsImages:    false,
			RecommendedNumCtx: 8192,
		}

	case "mistral":
		return Capabilities{
			SupportsTools:     true,
			SupportsThinking:  false,
			SupportsImages:    false,
			RecommendedNumCtx: 8192,
		}

	case "gpt-oss":
		return Capabilities{
			SupportsTools:     true,
			SupportsThinking:  true,
			SupportsImages:    true,
			RecommendedNumCtx: 32768,
		}

	case "gemma3":
		// Poor tool calling reliability - use JSON-format prompting fallback
		return Capabilities{
			SupportsTools:     false,
			SupportsThinking:  false,
			SupportsImages:    false,
			RecommendedNumCtx: 8192,
		}

	default:
		// Conservative defaults for unknown models
		return Capabilities{
			SupportsTools:     false,
			SupportsThinking:  false,
			SupportsImages:    false,
			RecommendedNumCtx: 4096,
		}
	}
}

// normalizeModelName extracts the base model name, removing version suffixes and size variants.
func normalizeModelName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Remove size variants (e.g., "7b", "13b", "70b")
	name = strings.ReplaceAll(name, ":7b", "")
	name = strings.ReplaceAll(name, ":13b", "")
	name = strings.ReplaceAll(name, ":70b", "")
	name = strings.ReplaceAll(name, ":latest", "")

	// Extract base name before colon
	if idx := strings.Index(name, ":"); idx != -1 {
		name = name[:idx]
	}

	return name
}
