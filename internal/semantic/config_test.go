package semantic

import (
	"testing"
	"time"
)

func TestDefaultConfigIsValid(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if cfg.Model != DefaultModel {
		t.Fatalf("model=%q", cfg.Model)
	}
	if cfg.Dimensions != DefaultDimensions {
		t.Fatalf("dimensions=%d", cfg.Dimensions)
	}
}

func TestNormalizeConfigRepairsInvalidValues(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Enabled:               true,
		Model:                 "",
		Dimensions:            0,
		MaxChunkTokens:        0,
		ChunkOverlapTokens:    1000,
		MaxFileBytes:          -1,
		MaxRecords:            -1,
		BatchSize:             -1,
		TopKRecords:           -1,
		TopKFiles:             -1,
		MaxContextBytes:       -1,
		HybridLexicalWeight:   99,
		FrecencyWeight:        -2,
		PromptRefreshMaxFiles: 0,
		PromptRefreshTimeout:  0,
		KeepAlive:             "",
	}
	got, warnings := NormalizeConfig(cfg)
	if len(warnings) == 0 {
		t.Fatalf("expected warnings for invalid config")
	}
	if err := ValidateConfig(got); err != nil {
		t.Fatalf("normalized config invalid: %v", err)
	}
	if got.ChunkOverlapTokens >= got.MaxChunkTokens {
		t.Fatalf("overlap must be < max chunk tokens")
	}
	if got.PromptRefreshTimeout < time.Millisecond {
		t.Fatalf("unexpected timeout %s", got.PromptRefreshTimeout)
	}
}
