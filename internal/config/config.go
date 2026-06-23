package config

import (
	"time"

	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/semantic"
)

type Config struct {
	DefaultModel                       string           `koanf:"default_model"`
	OllamaBaseURL                      string           `koanf:"ollama_base_url"`
	OllamaCloudEnabled                 bool             `koanf:"ollama_cloud_enabled"`
	ChatKeepAlive                      string           `koanf:"chat_keep_alive"`
	LLMStreamIdleTimeout               time.Duration    `koanf:"llm_stream_idle_timeout"`
	CloudLLMStreamIdleTimeout          time.Duration    `koanf:"cloud_llm_stream_idle_timeout"`
	PermissionMode                     permissions.Mode `koanf:"permission_mode"`
	MaxTurns                           int              `koanf:"max_turns"`
	MaxConcurrentTools                 int              `koanf:"-"`
	BashTimeout                        time.Duration    `koanf:"bash_timeout"`
	MaxReadChars                       int              `koanf:"max_read_chars"`
	MaxResultChars                     int              `koanf:"max_result_chars"`
	MaxDirFiles                        int              `koanf:"max_dir_files"`
	MaxPromptFiles                     int              `koanf:"max_prompt_files"`
	MaxDirBytes                        int64            `koanf:"max_dir_bytes"`
	MaxPromptBytes                     int64            `koanf:"max_prompt_bytes"`
	MaxDirDepth                        int              `koanf:"max_dir_depth"`
	MentionDirectorySource             string           `koanf:"mention_directory_source"`
	MentionIncludeGitignoredOnExplicit bool             `koanf:"mention_include_gitignored_on_explicit"`
	PromptDumpMode                     string           `koanf:"prompt_dump_mode"`
	PromptDumpKeep                     int              `koanf:"prompt_dump_keep"`
	PromptPreviewChars                 int              `koanf:"prompt_preview_chars"`
	LogLevel                           string           `koanf:"log_level"`
	LogFormat                          string           `koanf:"log_format"`
	SlowStageNoticeThreshold           time.Duration    `koanf:"slow_stage_notice_threshold"`
	ContextMode                        string           `koanf:"context_mode"`
	MemoryRecallMode                   string           `koanf:"memory_recall_mode"`
	MemoryEnabled                      bool             `koanf:"memory_enabled"`
	SkillsDir                          string           `koanf:"skills_dir"`
	ProjectSkillsDir                   string           `koanf:"project_skills_dir"`
	SemanticIndex                      semantic.Config  `koanf:"semantic_index"`
}

type ConfigSources struct {
	DefaultModel                       string
	OllamaBaseURL                      string
	OllamaCloudEnabled                 string
	ChatKeepAlive                      string
	LLMStreamIdleTimeout               string
	CloudLLMStreamIdleTimeout          string
	PermissionMode                     string
	MaxTurns                           string
	MaxConcurrentTools                 string
	BashTimeout                        string
	MaxReadChars                       string
	MaxResultChars                     string
	MaxDirFiles                        string
	MaxPromptFiles                     string
	MaxDirBytes                        string
	MaxPromptBytes                     string
	MaxDirDepth                        string
	MentionDirectorySource             string
	MentionIncludeGitignoredOnExplicit string
	PromptDumpMode                     string
	PromptDumpKeep                     string
	PromptPreviewChars                 string
	LogLevel                           string
	LogFormat                          string
	SlowStageNoticeThreshold           string
	ContextMode                        string
	MemoryRecallMode                   string
	MemoryEnabled                      string
	SkillsDir                          string
	ProjectSkillsDir                   string
	SemanticIndexEnabled               string
	SemanticIndexAutoBuild             string
	SemanticIndexModel                 string
	SemanticIndexDimensions            string
	SemanticIndexMaxChunkTokens        string
	SemanticIndexChunkOverlapTokens    string
	SemanticIndexMaxFileBytes          string
	SemanticIndexMaxRecords            string
	SemanticIndexBatchSize             string
	SemanticIndexTopKRecords           string
	SemanticIndexTopKFiles             string
	SemanticIndexMaxContextBytes       string
	SemanticIndexHybridLexicalWeight   string
	SemanticIndexFrecencyWeight        string
	SemanticIndexPromptRefreshMaxFiles string
	SemanticIndexPromptRefreshTimeout  string
	SemanticIndexKeepAlive             string
	SemanticIndexStorePreviews         string
	SemanticIndexMode                  string
	SemanticIndexLightTopKRecords      string
	SemanticIndexLightTopKFiles        string
	SemanticIndexLightMaxContextBytes  string
	SemanticIndexLightDeadlineMS       string
	SemanticIndexFullTopKRecords       string
	SemanticIndexFullTopKFiles         string
	SemanticIndexFullMaxContextBytes   string
	SemanticIndexFullDeadlineMS        string
	SemanticIndexDeepTopKRecords       string
	SemanticIndexDeepTopKFiles         string
	SemanticIndexDeepMaxContextBytes   string
	SemanticIndexDeepDeadlineMS        string
	SemanticIndexQueryKeepAlive        string
	SemanticIndexBuildKeepAlive        string
}

type LoadResult struct {
	Config   Config
	Sources  ConfigSources
	Warnings []string
}

type FlagOverrides struct {
	Model                     *string
	OllamaURL                 *string
	LogLevel                  *string
	LogFormat                 *string
	LLMStreamIdleTimeout      *string
	CloudLLMStreamIdleTimeout *string
}
