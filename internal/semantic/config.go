package semantic

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultMode                  = "auto"
	DefaultModel                 = "qwen3-embedding:8b"
	DefaultDimensions            = 1024
	DefaultMaxChunkTokens        = 700
	DefaultChunkOverlapTokens    = 120
	DefaultMaxFileBytes          = 1 * 1024 * 1024
	DefaultMaxRecords            = 200000
	DefaultBatchSize             = 32
	DefaultTopKRecords           = 40
	DefaultTopKFiles             = 12
	DefaultMaxContextBytes       = 256 * 1024
	DefaultLightTopKRecords      = 4
	DefaultLightTopKFiles        = 1
	DefaultLightMaxContextBytes  = 8 * 1024
	DefaultLightDeadlineMS       = 1200
	DefaultFullTopKRecords       = 12
	DefaultFullTopKFiles         = 4
	DefaultFullMaxContextBytes   = 64 * 1024
	DefaultFullDeadlineMS        = 3000
	DefaultDeepTopKRecords       = 40
	DefaultDeepTopKFiles         = 12
	DefaultDeepMaxContextBytes   = 256 * 1024
	DefaultDeepDeadlineMS        = 3000
	DefaultHybridLexicalWeight   = 0.20
	DefaultFrecencyWeight        = 0.10
	DefaultPromptRefreshMaxFiles = 8
	DefaultPromptRefreshTimeout  = 1500 * time.Millisecond
	DefaultKeepAlive             = "10m"
	DefaultQueryKeepAlive        = "30s"
	DefaultBuildKeepAlive        = "10m"
)

func DefaultConfig() Config {
	return Config{
		Enabled:               true,
		Mode:                  DefaultMode,
		AutoBuild:             false,
		Model:                 DefaultModel,
		Dimensions:            DefaultDimensions,
		MaxChunkTokens:        DefaultMaxChunkTokens,
		ChunkOverlapTokens:    DefaultChunkOverlapTokens,
		MaxFileBytes:          DefaultMaxFileBytes,
		MaxRecords:            DefaultMaxRecords,
		BatchSize:             DefaultBatchSize,
		TopKRecords:           DefaultTopKRecords,
		TopKFiles:             DefaultTopKFiles,
		MaxContextBytes:       DefaultMaxContextBytes,
		LightTopKRecords:      DefaultLightTopKRecords,
		LightTopKFiles:        DefaultLightTopKFiles,
		LightMaxContextBytes:  DefaultLightMaxContextBytes,
		LightDeadlineMS:       DefaultLightDeadlineMS,
		FullTopKRecords:       DefaultFullTopKRecords,
		FullTopKFiles:         DefaultFullTopKFiles,
		FullMaxContextBytes:   DefaultFullMaxContextBytes,
		FullDeadlineMS:        DefaultFullDeadlineMS,
		DeepTopKRecords:       DefaultDeepTopKRecords,
		DeepTopKFiles:         DefaultDeepTopKFiles,
		DeepMaxContextBytes:   DefaultDeepMaxContextBytes,
		DeepDeadlineMS:        DefaultDeepDeadlineMS,
		HybridLexicalWeight:   DefaultHybridLexicalWeight,
		FrecencyWeight:        DefaultFrecencyWeight,
		PromptRefreshMaxFiles: DefaultPromptRefreshMaxFiles,
		PromptRefreshTimeout:  DefaultPromptRefreshTimeout,
		KeepAlive:             DefaultKeepAlive,
		QueryKeepAlive:        DefaultQueryKeepAlive,
		BuildKeepAlive:        DefaultBuildKeepAlive,
		StorePreviews:         true,
	}
}

func NormalizeConfig(cfg Config) (Config, []string) {
	def := DefaultConfig()
	out := cfg
	warnings := make([]string, 0, 8)
	out.Mode = strings.ToLower(strings.TrimSpace(out.Mode))
	switch out.Mode {
	case "off", "explicit", "auto":
	default:
		out.Mode = def.Mode
		warnings = append(warnings, "semantic_index.mode must be off|explicit|auto; using default")
	}

	if strings.TrimSpace(out.Model) == "" {
		out.Model = def.Model
		warnings = append(warnings, "semantic_index.model is empty; using default")
	}
	if out.Dimensions <= 0 {
		out.Dimensions = def.Dimensions
		warnings = append(warnings, "semantic_index.dimensions must be > 0; using default")
	}
	if out.MaxChunkTokens <= 0 {
		out.MaxChunkTokens = def.MaxChunkTokens
		warnings = append(warnings, "semantic_index.max_chunk_tokens must be > 0; using default")
	}
	if out.ChunkOverlapTokens < 0 {
		out.ChunkOverlapTokens = def.ChunkOverlapTokens
		warnings = append(warnings, "semantic_index.chunk_overlap_tokens must be >= 0; using default")
	}
	if out.ChunkOverlapTokens >= out.MaxChunkTokens {
		out.ChunkOverlapTokens = out.MaxChunkTokens / 2
		warnings = append(warnings, "semantic_index.chunk_overlap_tokens must be less than max_chunk_tokens; clamped")
	}
	if out.MaxFileBytes <= 0 {
		out.MaxFileBytes = def.MaxFileBytes
		warnings = append(warnings, "semantic_index.max_file_bytes must be > 0; using default")
	}
	if out.MaxRecords <= 0 {
		out.MaxRecords = def.MaxRecords
		warnings = append(warnings, "semantic_index.max_records must be > 0; using default")
	}
	if out.BatchSize <= 0 {
		out.BatchSize = def.BatchSize
		warnings = append(warnings, "semantic_index.batch_size must be > 0; using default")
	}
	if out.TopKRecords <= 0 {
		out.TopKRecords = def.TopKRecords
		warnings = append(warnings, "semantic_index.top_k_records must be > 0; using default")
	}
	if out.TopKFiles <= 0 {
		out.TopKFiles = def.TopKFiles
		warnings = append(warnings, "semantic_index.top_k_files must be > 0; using default")
	}
	if out.MaxContextBytes <= 0 {
		out.MaxContextBytes = def.MaxContextBytes
		warnings = append(warnings, "semantic_index.max_context_bytes must be > 0; using default")
	}
	if out.LightTopKRecords <= 0 {
		out.LightTopKRecords = def.LightTopKRecords
	}
	if out.LightTopKFiles <= 0 {
		out.LightTopKFiles = def.LightTopKFiles
	}
	if out.LightMaxContextBytes <= 0 {
		out.LightMaxContextBytes = def.LightMaxContextBytes
	}
	if out.LightDeadlineMS <= 0 {
		out.LightDeadlineMS = def.LightDeadlineMS
	}
	if out.FullTopKRecords <= 0 {
		out.FullTopKRecords = out.TopKRecords
		if out.FullTopKRecords <= 0 {
			out.FullTopKRecords = def.FullTopKRecords
		}
	}
	if out.FullTopKFiles <= 0 {
		out.FullTopKFiles = out.TopKFiles
		if out.FullTopKFiles <= 0 {
			out.FullTopKFiles = def.FullTopKFiles
		}
	}
	if out.FullMaxContextBytes <= 0 {
		out.FullMaxContextBytes = out.MaxContextBytes
		if out.FullMaxContextBytes <= 0 {
			out.FullMaxContextBytes = def.FullMaxContextBytes
		}
	}
	if out.FullDeadlineMS <= 0 {
		out.FullDeadlineMS = def.FullDeadlineMS
	}
	if out.DeepTopKRecords <= 0 {
		out.DeepTopKRecords = def.DeepTopKRecords
	}
	if out.DeepTopKFiles <= 0 {
		out.DeepTopKFiles = def.DeepTopKFiles
	}
	if out.DeepMaxContextBytes <= 0 {
		out.DeepMaxContextBytes = def.DeepMaxContextBytes
	}
	if out.DeepDeadlineMS <= 0 {
		out.DeepDeadlineMS = def.DeepDeadlineMS
	}
	if out.HybridLexicalWeight < 0 || out.HybridLexicalWeight > 1 {
		out.HybridLexicalWeight = def.HybridLexicalWeight
		warnings = append(warnings, "semantic_index.hybrid_lexical_weight must be in [0,1]; using default")
	}
	if out.FrecencyWeight < 0 || out.FrecencyWeight > 1 {
		out.FrecencyWeight = def.FrecencyWeight
		warnings = append(warnings, "semantic_index.frecency_weight must be in [0,1]; using default")
	}
	if out.PromptRefreshMaxFiles <= 0 {
		out.PromptRefreshMaxFiles = def.PromptRefreshMaxFiles
		warnings = append(warnings, "semantic_index.prompt_refresh_max_files must be > 0; using default")
	}
	if out.PromptRefreshTimeout <= 0 {
		out.PromptRefreshTimeout = def.PromptRefreshTimeout
		warnings = append(warnings, "semantic_index.prompt_refresh_timeout must be > 0; using default")
	}
	if strings.TrimSpace(out.KeepAlive) == "" {
		out.KeepAlive = def.KeepAlive
		warnings = append(warnings, "semantic_index.keep_alive is empty; using default")
	}
	if strings.TrimSpace(out.QueryKeepAlive) == "" {
		out.QueryKeepAlive = def.QueryKeepAlive
	}
	if strings.TrimSpace(out.BuildKeepAlive) == "" {
		out.BuildKeepAlive = out.KeepAlive
		if strings.TrimSpace(out.BuildKeepAlive) == "" {
			out.BuildKeepAlive = def.BuildKeepAlive
		}
	}
	return out, warnings
}

func ValidateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("semantic config: model must not be empty")
	}
	if cfg.Dimensions <= 0 {
		return fmt.Errorf("semantic config: dimensions must be > 0")
	}
	if cfg.MaxChunkTokens <= 0 {
		return fmt.Errorf("semantic config: max_chunk_tokens must be > 0")
	}
	if cfg.ChunkOverlapTokens < 0 || cfg.ChunkOverlapTokens >= cfg.MaxChunkTokens {
		return fmt.Errorf("semantic config: chunk_overlap_tokens must be in [0,max_chunk_tokens)")
	}
	if cfg.MaxFileBytes <= 0 || cfg.MaxRecords <= 0 || cfg.BatchSize <= 0 {
		return fmt.Errorf("semantic config: max_file_bytes/max_records/batch_size must be > 0")
	}
	if cfg.TopKRecords <= 0 || cfg.TopKFiles <= 0 || cfg.MaxContextBytes <= 0 {
		return fmt.Errorf("semantic config: retrieval limits must be > 0")
	}
	if cfg.LightTopKRecords <= 0 || cfg.LightTopKFiles <= 0 || cfg.LightMaxContextBytes <= 0 || cfg.LightDeadlineMS <= 0 {
		return fmt.Errorf("semantic config: light retrieval limits must be > 0")
	}
	if cfg.FullTopKRecords <= 0 || cfg.FullTopKFiles <= 0 || cfg.FullMaxContextBytes <= 0 || cfg.FullDeadlineMS <= 0 {
		return fmt.Errorf("semantic config: full retrieval limits must be > 0")
	}
	if cfg.DeepTopKRecords <= 0 || cfg.DeepTopKFiles <= 0 || cfg.DeepMaxContextBytes <= 0 || cfg.DeepDeadlineMS <= 0 {
		return fmt.Errorf("semantic config: deep retrieval limits must be > 0")
	}
	if cfg.HybridLexicalWeight < 0 || cfg.HybridLexicalWeight > 1 {
		return fmt.Errorf("semantic config: hybrid_lexical_weight must be in [0,1]")
	}
	if cfg.FrecencyWeight < 0 || cfg.FrecencyWeight > 1 {
		return fmt.Errorf("semantic config: frecency_weight must be in [0,1]")
	}
	if cfg.PromptRefreshMaxFiles <= 0 || cfg.PromptRefreshTimeout <= 0 {
		return fmt.Errorf("semantic config: refresh controls must be > 0")
	}
	if strings.TrimSpace(cfg.KeepAlive) == "" {
		return fmt.Errorf("semantic config: keep_alive must not be empty")
	}
	if strings.TrimSpace(cfg.QueryKeepAlive) == "" {
		return fmt.Errorf("semantic config: query_keep_alive must not be empty")
	}
	if strings.TrimSpace(cfg.BuildKeepAlive) == "" {
		return fmt.Errorf("semantic config: build_keep_alive must not be empty")
	}
	return nil
}
