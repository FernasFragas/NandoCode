package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/semantic"
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

func Load(userConfigPath, projConfigPath string, flags FlagOverrides) (LoadResult, error) {
	k := koanf.New(".")
	d := DefaultConfig()
	base := map[string]any{
		"default_model":                            d.DefaultModel,
		"ollama_base_url":                          d.OllamaBaseURL,
		"ollama_cloud_enabled":                     d.OllamaCloudEnabled,
		"chat_keep_alive":                          d.ChatKeepAlive,
		"llm_stream_idle_timeout":                  d.LLMStreamIdleTimeout.String(),
		"cloud_llm_stream_idle_timeout":            d.CloudLLMStreamIdleTimeout.String(),
		"permission_mode":                          d.PermissionMode.String(),
		"max_turns":                                d.MaxTurns,
		"concurrency.max_batch_size":               d.MaxConcurrentTools,
		"bash_timeout":                             d.BashTimeout.String(),
		"max_read_chars":                           d.MaxReadChars,
		"max_result_chars":                         d.MaxResultChars,
		"max_dir_files":                            d.MaxDirFiles,
		"max_prompt_files":                         d.MaxPromptFiles,
		"max_dir_bytes":                            d.MaxDirBytes,
		"max_prompt_bytes":                         d.MaxPromptBytes,
		"max_dir_depth":                            d.MaxDirDepth,
		"mention_directory_source":                 d.MentionDirectorySource,
		"mention_include_gitignored_on_explicit":   d.MentionIncludeGitignoredOnExplicit,
		"prompt_dump_mode":                         d.PromptDumpMode,
		"prompt_dump_keep":                         d.PromptDumpKeep,
		"prompt_preview_chars":                     d.PromptPreviewChars,
		"log_level":                                d.LogLevel,
		"log_format":                               d.LogFormat,
		"slow_stage_notice_threshold":              d.SlowStageNoticeThreshold.String(),
		"context_mode":                             d.ContextMode,
		"memory_recall_mode":                       d.MemoryRecallMode,
		"memory_enabled":                           d.MemoryEnabled,
		"skills_dir":                               d.SkillsDir,
		"project_skills_dir":                       d.ProjectSkillsDir,
		"semantic_index.enabled":                   d.SemanticIndex.Enabled,
		"semantic_index.mode":                      d.SemanticIndex.Mode,
		"semantic_index.auto_build":                d.SemanticIndex.AutoBuild,
		"semantic_index.model":                     d.SemanticIndex.Model,
		"semantic_index.dimensions":                d.SemanticIndex.Dimensions,
		"semantic_index.max_chunk_tokens":          d.SemanticIndex.MaxChunkTokens,
		"semantic_index.chunk_overlap_tokens":      d.SemanticIndex.ChunkOverlapTokens,
		"semantic_index.max_file_bytes":            d.SemanticIndex.MaxFileBytes,
		"semantic_index.max_records":               d.SemanticIndex.MaxRecords,
		"semantic_index.batch_size":                d.SemanticIndex.BatchSize,
		"semantic_index.top_k_records":             d.SemanticIndex.TopKRecords,
		"semantic_index.top_k_files":               d.SemanticIndex.TopKFiles,
		"semantic_index.max_context_bytes":         d.SemanticIndex.MaxContextBytes,
		"semantic_index.light_top_k_records":       d.SemanticIndex.LightTopKRecords,
		"semantic_index.light_top_k_files":         d.SemanticIndex.LightTopKFiles,
		"semantic_index.light_max_context_bytes":   d.SemanticIndex.LightMaxContextBytes,
		"semantic_index.light_deadline_ms":         d.SemanticIndex.LightDeadlineMS,
		"semantic_index.full_top_k_records":        d.SemanticIndex.FullTopKRecords,
		"semantic_index.full_top_k_files":          d.SemanticIndex.FullTopKFiles,
		"semantic_index.full_max_context_bytes":    d.SemanticIndex.FullMaxContextBytes,
		"semantic_index.full_deadline_ms":          d.SemanticIndex.FullDeadlineMS,
		"semantic_index.deep_top_k_records":        d.SemanticIndex.DeepTopKRecords,
		"semantic_index.deep_top_k_files":          d.SemanticIndex.DeepTopKFiles,
		"semantic_index.deep_max_context_bytes":    d.SemanticIndex.DeepMaxContextBytes,
		"semantic_index.deep_deadline_ms":          d.SemanticIndex.DeepDeadlineMS,
		"semantic_index.hybrid_lexical_weight":     d.SemanticIndex.HybridLexicalWeight,
		"semantic_index.frecency_weight":           d.SemanticIndex.FrecencyWeight,
		"semantic_index.prompt_refresh_max_files":  d.SemanticIndex.PromptRefreshMaxFiles,
		"semantic_index.prompt_refresh_timeout_ms": int(d.SemanticIndex.PromptRefreshTimeout / time.Millisecond),
		"semantic_index.keep_alive":                d.SemanticIndex.KeepAlive,
		"semantic_index.query_keep_alive":          d.SemanticIndex.QueryKeepAlive,
		"semantic_index.build_keep_alive":          d.SemanticIndex.BuildKeepAlive,
		"semantic_index.store_previews":            d.SemanticIndex.StorePreviews,
	}
	if err := k.Load(confmap.Provider(base, "."), nil); err != nil {
		return LoadResult{}, err
	}
	src := ConfigSources{
		DefaultModel:                       "default",
		OllamaBaseURL:                      "default",
		OllamaCloudEnabled:                 "default",
		ChatKeepAlive:                      "default",
		LLMStreamIdleTimeout:               "default",
		CloudLLMStreamIdleTimeout:          "default",
		PermissionMode:                     "default",
		MaxTurns:                           "default",
		MaxConcurrentTools:                 "default",
		BashTimeout:                        "default",
		MaxReadChars:                       "default",
		MaxResultChars:                     "default",
		MaxDirFiles:                        "default",
		MaxPromptFiles:                     "default",
		MaxDirBytes:                        "default",
		MaxPromptBytes:                     "default",
		MaxDirDepth:                        "default",
		MentionDirectorySource:             "default",
		MentionIncludeGitignoredOnExplicit: "default",
		PromptDumpMode:                     "default",
		PromptDumpKeep:                     "default",
		PromptPreviewChars:                 "default",
		LogLevel:                           "default",
		LogFormat:                          "default",
		SlowStageNoticeThreshold:           "default",
		ContextMode:                        "default",
		MemoryRecallMode:                   "default",
		MemoryEnabled:                      "default",
		SkillsDir:                          "default",
		ProjectSkillsDir:                   "default",
		SemanticIndexEnabled:               "default",
		SemanticIndexMode:                  "default",
		SemanticIndexAutoBuild:             "default",
		SemanticIndexModel:                 "default",
		SemanticIndexDimensions:            "default",
		SemanticIndexMaxChunkTokens:        "default",
		SemanticIndexChunkOverlapTokens:    "default",
		SemanticIndexMaxFileBytes:          "default",
		SemanticIndexMaxRecords:            "default",
		SemanticIndexBatchSize:             "default",
		SemanticIndexTopKRecords:           "default",
		SemanticIndexTopKFiles:             "default",
		SemanticIndexMaxContextBytes:       "default",
		SemanticIndexLightTopKRecords:      "default",
		SemanticIndexLightTopKFiles:        "default",
		SemanticIndexLightMaxContextBytes:  "default",
		SemanticIndexLightDeadlineMS:       "default",
		SemanticIndexFullTopKRecords:       "default",
		SemanticIndexFullTopKFiles:         "default",
		SemanticIndexFullMaxContextBytes:   "default",
		SemanticIndexFullDeadlineMS:        "default",
		SemanticIndexDeepTopKRecords:       "default",
		SemanticIndexDeepTopKFiles:         "default",
		SemanticIndexDeepMaxContextBytes:   "default",
		SemanticIndexDeepDeadlineMS:        "default",
		SemanticIndexHybridLexicalWeight:   "default",
		SemanticIndexFrecencyWeight:        "default",
		SemanticIndexPromptRefreshMaxFiles: "default",
		SemanticIndexPromptRefreshTimeout:  "default",
		SemanticIndexKeepAlive:             "default",
		SemanticIndexQueryKeepAlive:        "default",
		SemanticIndexBuildKeepAlive:        "default",
		SemanticIndexStorePreviews:         "default",
	}
	warnings := make([]string, 0, 4)

	if st, err := os.Stat(userConfigPath); err == nil && !st.IsDir() {
		loadConfigLayer(k, userConfigPath, "user", &src, &warnings)
	}
	if st, err := os.Stat(projConfigPath); err == nil && !st.IsDir() {
		loadConfigLayer(k, projConfigPath, "project", &src, &warnings)
	}
	if flags.Model != nil {
		_ = k.Set("default_model", *flags.Model)
		src.DefaultModel = "flag"
	}
	if flags.OllamaURL != nil {
		_ = k.Set("ollama_base_url", *flags.OllamaURL)
		src.OllamaBaseURL = "flag"
	}
	if flags.LogLevel != nil {
		_ = k.Set("log_level", *flags.LogLevel)
		src.LogLevel = "flag"
	}
	if flags.LogFormat != nil {
		_ = k.Set("log_format", *flags.LogFormat)
		src.LogFormat = "flag"
	}
	if flags.LLMStreamIdleTimeout != nil {
		_ = k.Set("llm_stream_idle_timeout", *flags.LLMStreamIdleTimeout)
		src.LLMStreamIdleTimeout = "flag"
	}
	if flags.CloudLLMStreamIdleTimeout != nil {
		_ = k.Set("cloud_llm_stream_idle_timeout", *flags.CloudLLMStreamIdleTimeout)
		src.CloudLLMStreamIdleTimeout = "flag"
	}

	cfg := DefaultConfig()
	if err := k.Unmarshal("", &cfg); err != nil {
		return LoadResult{}, err
	}
	if v := strings.TrimSpace(os.Getenv("NANDOCODEGO_OLLAMA_CLOUD")); v != "" {
		if b, ok := parseBoolEnv(v); ok {
			cfg.OllamaCloudEnabled = b
			src.OllamaCloudEnabled = "env"
		} else {
			warnings = append(warnings, fmt.Sprintf("invalid NANDOCODEGO_OLLAMA_CLOUD=%q (expected 0|1|true|false)", v))
		}
	}
	cfg.MaxConcurrentTools = k.Int("concurrency.max_batch_size")
	cfg.PermissionMode = permissions.Mode(k.String("permission_mode")).Normalize()
	if cfg.MaxTurns < 1 {
		cfg.MaxTurns = 1
	}
	if cfg.MaxConcurrentTools < 1 {
		cfg.MaxConcurrentTools = 1
	}
	if cfg.BashTimeout < time.Second {
		cfg.BashTimeout = time.Second
	}
	if cfg.LLMStreamIdleTimeout < time.Second {
		cfg.LLMStreamIdleTimeout = time.Second
	}
	if cfg.CloudLLMStreamIdleTimeout < time.Second {
		cfg.CloudLLMStreamIdleTimeout = cfg.LLMStreamIdleTimeout
	}
	cfg.ContextMode = strings.ToLower(strings.TrimSpace(cfg.ContextMode))
	switch cfg.ContextMode {
	case "auto", "small", "large", "max":
	default:
		cfg.ContextMode = "auto"
	}
	cfg.MemoryRecallMode = strings.ToLower(strings.TrimSpace(cfg.MemoryRecallMode))
	switch cfg.MemoryRecallMode {
	case "off", "fast", "llm":
	default:
		cfg.MemoryRecallMode = "fast"
	}
	cfg.MentionDirectorySource = strings.ToLower(strings.TrimSpace(cfg.MentionDirectorySource))
	switch cfg.MentionDirectorySource {
	case "auto", "git", "filesystem":
	default:
		cfg.MentionDirectorySource = "auto"
	}
	cfg.PromptDumpMode = strings.ToLower(strings.TrimSpace(cfg.PromptDumpMode))
	switch cfg.PromptDumpMode {
	case "off", "metadata", "full":
	default:
		cfg.PromptDumpMode = "off"
	}
	if cfg.PromptDumpKeep < 1 {
		cfg.PromptDumpKeep = 10
	}
	if cfg.PromptPreviewChars < 1 {
		cfg.PromptPreviewChars = 600
	}
	promptRefreshMS := k.Int("semantic_index.prompt_refresh_timeout_ms")
	if promptRefreshMS <= 0 {
		promptRefreshMS = int(semantic.DefaultPromptRefreshTimeout / time.Millisecond)
	}
	cfg.SemanticIndex.PromptRefreshTimeout = time.Duration(promptRefreshMS) * time.Millisecond
	normSemantic, semanticWarnings := semantic.NormalizeConfig(cfg.SemanticIndex)
	cfg.SemanticIndex = normSemantic
	warnings = append(warnings, semanticWarnings...)
	return LoadResult{Config: cfg, Sources: src, Warnings: warnings}, nil
}

func loadConfigLayer(k *koanf.Koanf, path, label string, src *ConfigSources, warnings *[]string) {
	tmp := koanf.New(".")
	if err := tmp.Load(file.Provider(path), toml.Parser()); err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s config parse error at %s: %v", label, path, err))
		return
	}
	if err := k.Load(file.Provider(path), toml.Parser()); err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s config load error at %s: %v", label, path, err))
		return
	}
	markUnknownKeys(tmp, label, warnings)
	markSourcesFromKoanf(tmp, src, label)
}

func markSourcesFromKoanf(tmp *koanf.Koanf, src *ConfigSources, label string) {
	if tmp.Exists("default_model") {
		src.DefaultModel = label
	}
	if tmp.Exists("ollama_base_url") {
		src.OllamaBaseURL = label
	}
	if tmp.Exists("ollama_cloud_enabled") {
		src.OllamaCloudEnabled = label
	}
	if tmp.Exists("chat_keep_alive") {
		src.ChatKeepAlive = label
	}
	if tmp.Exists("llm_stream_idle_timeout") {
		src.LLMStreamIdleTimeout = label
	}
	if tmp.Exists("cloud_llm_stream_idle_timeout") {
		src.CloudLLMStreamIdleTimeout = label
	}
	if tmp.Exists("permission_mode") {
		src.PermissionMode = label
	}
	if tmp.Exists("max_turns") {
		src.MaxTurns = label
	}
	if tmp.Exists("concurrency.max_batch_size") {
		src.MaxConcurrentTools = label
	}
	if tmp.Exists("bash_timeout") {
		src.BashTimeout = label
	}
	if tmp.Exists("max_read_chars") {
		src.MaxReadChars = label
	}
	if tmp.Exists("max_result_chars") {
		src.MaxResultChars = label
	}
	if tmp.Exists("max_dir_files") {
		src.MaxDirFiles = label
	}
	if tmp.Exists("max_prompt_files") {
		src.MaxPromptFiles = label
	}
	if tmp.Exists("max_dir_bytes") {
		src.MaxDirBytes = label
	}
	if tmp.Exists("max_prompt_bytes") {
		src.MaxPromptBytes = label
	}
	if tmp.Exists("max_dir_depth") {
		src.MaxDirDepth = label
	}
	if tmp.Exists("mention_directory_source") {
		src.MentionDirectorySource = label
	}
	if tmp.Exists("mention_include_gitignored_on_explicit") {
		src.MentionIncludeGitignoredOnExplicit = label
	}
	if tmp.Exists("prompt_dump_mode") {
		src.PromptDumpMode = label
	}
	if tmp.Exists("prompt_dump_keep") {
		src.PromptDumpKeep = label
	}
	if tmp.Exists("prompt_preview_chars") {
		src.PromptPreviewChars = label
	}
	if tmp.Exists("log_level") {
		src.LogLevel = label
	}
	if tmp.Exists("log_format") {
		src.LogFormat = label
	}
	if tmp.Exists("slow_stage_notice_threshold") {
		src.SlowStageNoticeThreshold = label
	}
	if tmp.Exists("context_mode") {
		src.ContextMode = label
	}
	if tmp.Exists("memory_recall_mode") {
		src.MemoryRecallMode = label
	}
	if tmp.Exists("memory_enabled") {
		src.MemoryEnabled = label
	}
	if tmp.Exists("skills_dir") {
		src.SkillsDir = label
	}
	if tmp.Exists("project_skills_dir") {
		src.ProjectSkillsDir = label
	}
	if tmp.Exists("semantic_index.enabled") {
		src.SemanticIndexEnabled = label
	}
	if tmp.Exists("semantic_index.mode") {
		src.SemanticIndexMode = label
	}
	if tmp.Exists("semantic_index.auto_build") {
		src.SemanticIndexAutoBuild = label
	}
	if tmp.Exists("semantic_index.model") {
		src.SemanticIndexModel = label
	}
	if tmp.Exists("semantic_index.dimensions") {
		src.SemanticIndexDimensions = label
	}
	if tmp.Exists("semantic_index.max_chunk_tokens") {
		src.SemanticIndexMaxChunkTokens = label
	}
	if tmp.Exists("semantic_index.chunk_overlap_tokens") {
		src.SemanticIndexChunkOverlapTokens = label
	}
	if tmp.Exists("semantic_index.max_file_bytes") {
		src.SemanticIndexMaxFileBytes = label
	}
	if tmp.Exists("semantic_index.max_records") {
		src.SemanticIndexMaxRecords = label
	}
	if tmp.Exists("semantic_index.batch_size") {
		src.SemanticIndexBatchSize = label
	}
	if tmp.Exists("semantic_index.top_k_records") {
		src.SemanticIndexTopKRecords = label
	}
	if tmp.Exists("semantic_index.top_k_files") {
		src.SemanticIndexTopKFiles = label
	}
	if tmp.Exists("semantic_index.max_context_bytes") {
		src.SemanticIndexMaxContextBytes = label
	}
	if tmp.Exists("semantic_index.light_top_k_records") {
		src.SemanticIndexLightTopKRecords = label
	}
	if tmp.Exists("semantic_index.light_top_k_files") {
		src.SemanticIndexLightTopKFiles = label
	}
	if tmp.Exists("semantic_index.light_max_context_bytes") {
		src.SemanticIndexLightMaxContextBytes = label
	}
	if tmp.Exists("semantic_index.light_deadline_ms") {
		src.SemanticIndexLightDeadlineMS = label
	}
	if tmp.Exists("semantic_index.full_top_k_records") {
		src.SemanticIndexFullTopKRecords = label
	}
	if tmp.Exists("semantic_index.full_top_k_files") {
		src.SemanticIndexFullTopKFiles = label
	}
	if tmp.Exists("semantic_index.full_max_context_bytes") {
		src.SemanticIndexFullMaxContextBytes = label
	}
	if tmp.Exists("semantic_index.full_deadline_ms") {
		src.SemanticIndexFullDeadlineMS = label
	}
	if tmp.Exists("semantic_index.deep_top_k_records") {
		src.SemanticIndexDeepTopKRecords = label
	}
	if tmp.Exists("semantic_index.deep_top_k_files") {
		src.SemanticIndexDeepTopKFiles = label
	}
	if tmp.Exists("semantic_index.deep_max_context_bytes") {
		src.SemanticIndexDeepMaxContextBytes = label
	}
	if tmp.Exists("semantic_index.deep_deadline_ms") {
		src.SemanticIndexDeepDeadlineMS = label
	}
	if tmp.Exists("semantic_index.hybrid_lexical_weight") {
		src.SemanticIndexHybridLexicalWeight = label
	}
	if tmp.Exists("semantic_index.frecency_weight") {
		src.SemanticIndexFrecencyWeight = label
	}
	if tmp.Exists("semantic_index.prompt_refresh_max_files") {
		src.SemanticIndexPromptRefreshMaxFiles = label
	}
	if tmp.Exists("semantic_index.prompt_refresh_timeout_ms") {
		src.SemanticIndexPromptRefreshTimeout = label
	}
	if tmp.Exists("semantic_index.keep_alive") {
		src.SemanticIndexKeepAlive = label
	}
	if tmp.Exists("semantic_index.query_keep_alive") {
		src.SemanticIndexQueryKeepAlive = label
	}
	if tmp.Exists("semantic_index.build_keep_alive") {
		src.SemanticIndexBuildKeepAlive = label
	}
	if tmp.Exists("semantic_index.store_previews") {
		src.SemanticIndexStorePreviews = label
	}
}

func markUnknownKeys(tmp *koanf.Koanf, label string, warnings *[]string) {
	knownTop := map[string]struct{}{
		"default_model":                          {},
		"ollama_base_url":                        {},
		"ollama_cloud_enabled":                   {},
		"chat_keep_alive":                        {},
		"llm_stream_idle_timeout":                {},
		"cloud_llm_stream_idle_timeout":          {},
		"permission_mode":                        {},
		"max_turns":                              {},
		"concurrency":                            {},
		"bash_timeout":                           {},
		"max_read_chars":                         {},
		"max_result_chars":                       {},
		"max_dir_files":                          {},
		"max_prompt_files":                       {},
		"max_dir_bytes":                          {},
		"max_prompt_bytes":                       {},
		"max_dir_depth":                          {},
		"mention_directory_source":               {},
		"mention_include_gitignored_on_explicit": {},
		"prompt_dump_mode":                       {},
		"prompt_dump_keep":                       {},
		"prompt_preview_chars":                   {},
		"log_level":                              {},
		"log_format":                             {},
		"slow_stage_notice_threshold":            {},
		"context_mode":                           {},
		"memory_recall_mode":                     {},
		"memory_enabled":                         {},
		"skills_dir":                             {},
		"project_skills_dir":                     {},
		"semantic_index":                         {},
		"mcp":                                    {},
	}
	if tmp.Exists("api_key") || tmp.Exists("ollama_api_key") {
		*warnings = append(*warnings, fmt.Sprintf("%s config contains unsupported plaintext API key fields; use OLLAMA_API_KEY or OS keychain", label))
	}
	seenUnknown := map[string]struct{}{}
	for _, key := range tmp.Keys() {
		top := key
		if idx := strings.IndexRune(key, '.'); idx > 0 {
			top = key[:idx]
		}
		if _, ok := knownTop[top]; ok {
			continue
		}
		if _, dup := seenUnknown[top]; dup {
			continue
		}
		seenUnknown[top] = struct{}{}
		*warnings = append(*warnings, fmt.Sprintf("%s config contains unknown top-level key %q", label, top))
	}
}

func parseBoolEnv(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}
