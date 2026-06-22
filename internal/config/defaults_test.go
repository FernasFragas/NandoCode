package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.DefaultModel == "" || cfg.OllamaBaseURL == "" || cfg.MaxTurns < 1 {
		t.Fatal("default config not populated")
	}
	if cfg.MaxTurns != 200 {
		t.Fatalf("MaxTurns=%d, want 200", cfg.MaxTurns)
	}
	if cfg.LLMStreamIdleTimeout != 90*time.Second {
		t.Fatalf("LLMStreamIdleTimeout=%s, want 90s", cfg.LLMStreamIdleTimeout)
	}
	if cfg.CloudLLMStreamIdleTimeout != 5*time.Minute {
		t.Fatalf("CloudLLMStreamIdleTimeout=%s, want 5m", cfg.CloudLLMStreamIdleTimeout)
	}
}

func TestDefaultConfigTOML(t *testing.T) {
	t.Parallel()
	body := DefaultConfigTOML()
	for _, key := range []string{"default_model", "ollama_base_url", "chat_keep_alive", "llm_stream_idle_timeout", "cloud_llm_stream_idle_timeout", "permission_mode", "max_batch_size", "max_dir_files", "semantic_index", "prompt_refresh_timeout_ms"} {
		if !strings.Contains(body, key) {
			t.Fatalf("template missing key %s", key)
		}
	}
	if !strings.Contains(body, "# max_turns = 200") {
		t.Fatal("template should document max_turns = 200")
	}
}
