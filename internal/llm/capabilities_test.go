package llm

import (
	"testing"
)

func TestModelCapabilities(t *testing.T) {
	tests := []struct {
		name              string
		modelName         string
		expectTools       bool
		expectThinking    bool
		expectImages      bool
		minRecommendedCtx int
	}{
		{
			name:              "qwen3.6 35b",
			modelName:         DefaultModel,
			expectTools:       true,
			expectThinking:    true,
			expectImages:      true,
			minRecommendedCtx: 65536,
		},
		{
			name:              "qwen3",
			modelName:         "qwen3",
			expectTools:       true,
			expectThinking:    false,
			expectImages:      false,
			minRecommendedCtx: 32768,
		},
		{
			name:              "qwen3 with size tag",
			modelName:         "qwen3:7b",
			expectTools:       true,
			expectThinking:    false,
			expectImages:      false,
			minRecommendedCtx: 32768,
		},
		{
			name:              "gpt-oss",
			modelName:         "gpt-oss",
			expectTools:       true,
			expectThinking:    true,
			expectImages:      true,
			minRecommendedCtx: 32768,
		},
		{
			name:              "gemma3 - poor tool support",
			modelName:         "gemma3",
			expectTools:       false,
			expectThinking:    false,
			expectImages:      false,
			minRecommendedCtx: 8192,
		},
		{
			name:              "unknown model",
			modelName:         "unknown-model",
			expectTools:       false,
			expectThinking:    false,
			expectImages:      false,
			minRecommendedCtx: 4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := ModelCapabilities(tt.modelName)

			if caps.SupportsTools != tt.expectTools {
				t.Errorf("SupportsTools = %v, want %v", caps.SupportsTools, tt.expectTools)
			}
			if caps.SupportsThinking != tt.expectThinking {
				t.Errorf("SupportsThinking = %v, want %v", caps.SupportsThinking, tt.expectThinking)
			}
			if caps.SupportsImages != tt.expectImages {
				t.Errorf("SupportsImages = %v, want %v", caps.SupportsImages, tt.expectImages)
			}
			if caps.RecommendedNumCtx < tt.minRecommendedCtx {
				t.Errorf("RecommendedNumCtx = %d, want >= %d", caps.RecommendedNumCtx, tt.minRecommendedCtx)
			}
		})
	}
}

func TestNormalizeModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"qwen3", "qwen3"},
		{"qwen3.6:35b", "qwen3.6"},
		{"qwen3:7b", "qwen3"},
		{"qwen3:13b", "qwen3"},
		{"llama3.2:latest", "llama3.2"},
		{"QWEN3", "qwen3"},
		{"GPT-OSS", "gpt-oss"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeModelName(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeModelName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
