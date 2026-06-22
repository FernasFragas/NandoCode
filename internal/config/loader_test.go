package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadHierarchy(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	proj := filepath.Join(td, "proj.toml")
	if err := os.WriteFile(user, []byte(`default_model="user-model"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proj, []byte(`default_model="proj-model"`), 0o644); err != nil {
		t.Fatal(err)
	}
	flagModel := "flag-model"
	got, err := Load(user, proj, FlagOverrides{Model: &flagModel})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.DefaultModel != "flag-model" {
		t.Fatalf("default_model=%q", got.Config.DefaultModel)
	}
	if got.Sources.DefaultModel != "flag" {
		t.Fatalf("source=%q", got.Sources.DefaultModel)
	}
}

func TestLoadMissingFiles(t *testing.T) {
	t.Parallel()
	got, err := Load("/no/user.toml", "/no/proj.toml", FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.DefaultModel == "" {
		t.Fatal("expected defaults")
	}
	if got.Sources.DefaultModel != "default" {
		t.Fatalf("expected default source, got %q", got.Sources.DefaultModel)
	}
	if got.Config.MaxTurns != 200 {
		t.Fatalf("expected default max_turns=200, got %d", got.Config.MaxTurns)
	}
}

func TestLoadUserConfigParseErrorWarnsAndContinues(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	if err := os.WriteFile(user, []byte(`default_model = "`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.DefaultModel != DefaultConfig().DefaultModel {
		t.Fatalf("expected default model fallback, got %q", got.Config.DefaultModel)
	}
	if len(got.Warnings) == 0 || !strings.Contains(strings.ToLower(strings.Join(got.Warnings, "\n")), "parse error") {
		t.Fatalf("expected parse warning, got %v", got.Warnings)
	}
}

func TestLoadUnknownKeyWarnsAndContinues(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	if err := os.WriteFile(user, []byte(`default_model="user-model"`+"\n"+`mystery = "x"`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.DefaultModel != "user-model" {
		t.Fatalf("expected user override, got %q", got.Config.DefaultModel)
	}
	joined := strings.ToLower(strings.Join(got.Warnings, "\n"))
	if !strings.Contains(joined, "unknown top-level key") {
		t.Fatalf("expected unknown key warning, got %v", got.Warnings)
	}
}

func TestLoadSourcesForSkillsDirs(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	if err := os.WriteFile(user, []byte(`skills_dir="/tmp/a"`+"\n"+`project_skills_dir="/tmp/b"`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources.SkillsDir != "user" || got.Sources.ProjectSkillsDir != "user" {
		t.Fatalf("unexpected sources: %+v", got.Sources)
	}
}

func TestLoadConcurrencyMaxBatchSize(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	if err := os.WriteFile(user, []byte("[concurrency]\nmax_batch_size=7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.MaxConcurrentTools != 7 {
		t.Fatalf("expected max concurrent tools=7, got %d", got.Config.MaxConcurrentTools)
	}
	if got.Sources.MaxConcurrentTools != "user" {
		t.Fatalf("expected source=user, got %q", got.Sources.MaxConcurrentTools)
	}
}

func TestLoadDirectoryMentionCaps(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	body := strings.Join([]string{
		"max_dir_files=201",
		"max_prompt_files=401",
		"max_dir_bytes=600000",
		"max_prompt_bytes=3000000",
		"max_dir_depth=9",
	}, "\n")
	if err := os.WriteFile(user, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.MaxDirFiles != 201 || got.Config.MaxPromptFiles != 401 {
		t.Fatalf("unexpected file caps: %+v", got.Config)
	}
	if got.Config.MaxDirBytes != 600000 || got.Config.MaxPromptBytes != 3000000 {
		t.Fatalf("unexpected byte caps: %+v", got.Config)
	}
	if got.Config.MaxDirDepth != 9 {
		t.Fatalf("unexpected depth cap: %d", got.Config.MaxDirDepth)
	}
	if got.Sources.MaxDirFiles != "user" || got.Sources.MaxDirDepth != "user" {
		t.Fatalf("unexpected sources: %+v", got.Sources)
	}
}

func TestLoadSlowStageThresholdSourceDefault(t *testing.T) {
	t.Parallel()
	got, err := Load("/no/user.toml", "/no/proj.toml", FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources.SlowStageNoticeThreshold != "default" {
		t.Fatalf("expected default source, got %q", got.Sources.SlowStageNoticeThreshold)
	}
}

func TestLoadSlowStageThresholdSourceUserAndProject(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	proj := filepath.Join(td, "proj.toml")

	if err := os.WriteFile(user, []byte(`slow_stage_notice_threshold="900ms"`), 0o644); err != nil {
		t.Fatal(err)
	}
	gotUser, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if gotUser.Sources.SlowStageNoticeThreshold != "user" {
		t.Fatalf("expected user source, got %q", gotUser.Sources.SlowStageNoticeThreshold)
	}
	if gotUser.Config.SlowStageNoticeThreshold != 900*time.Millisecond {
		t.Fatalf("unexpected threshold: %s", gotUser.Config.SlowStageNoticeThreshold)
	}

	if err := os.WriteFile(proj, []byte(`slow_stage_notice_threshold="1400ms"`), 0o644); err != nil {
		t.Fatal(err)
	}
	gotProject, err := Load(user, proj, FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if gotProject.Sources.SlowStageNoticeThreshold != "project" {
		t.Fatalf("expected project source, got %q", gotProject.Sources.SlowStageNoticeThreshold)
	}
	if gotProject.Config.SlowStageNoticeThreshold != 1400*time.Millisecond {
		t.Fatalf("unexpected threshold: %s", gotProject.Config.SlowStageNoticeThreshold)
	}
}

func TestLoadPromptDumpConfig(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	body := strings.Join([]string{
		`prompt_dump_mode="metadata"`,
		`prompt_dump_keep=7`,
		`prompt_preview_chars=321`,
	}, "\n")
	if err := os.WriteFile(user, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.PromptDumpMode != "metadata" {
		t.Fatalf("prompt_dump_mode=%q", got.Config.PromptDumpMode)
	}
	if got.Config.PromptDumpKeep != 7 {
		t.Fatalf("prompt_dump_keep=%d", got.Config.PromptDumpKeep)
	}
	if got.Config.PromptPreviewChars != 321 {
		t.Fatalf("prompt_preview_chars=%d", got.Config.PromptPreviewChars)
	}
}

func TestLoadOllamaCloudEnabledEnvOverride(t *testing.T) {
	t.Setenv("NANDOCODEGO_OLLAMA_CLOUD", "0")
	got, err := Load("/no/user.toml", "/no/proj.toml", FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.OllamaCloudEnabled {
		t.Fatalf("expected cloud disabled by env override")
	}
	if got.Sources.OllamaCloudEnabled != "env" {
		t.Fatalf("source=%q", got.Sources.OllamaCloudEnabled)
	}
}

func TestLoadWarnsOnPlaintextAPIKeyConfig(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	if err := os.WriteFile(user, []byte(`api_key="dont-use"`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.ToLower(strings.Join(got.Warnings, "\n"))
	if !strings.Contains(joined, "plaintext api key") {
		t.Fatalf("expected plaintext key warning, got %v", got.Warnings)
	}
}

func TestLoadWatchdogTimeoutOverridesAndSources(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	proj := filepath.Join(td, "proj.toml")
	if err := os.WriteFile(user, []byte(`llm_stream_idle_timeout="110s"`+"\n"+`cloud_llm_stream_idle_timeout="6m"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proj, []byte(`llm_stream_idle_timeout="130s"`+"\n"+`cloud_llm_stream_idle_timeout="7m"`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, proj, FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.LLMStreamIdleTimeout != 130*time.Second {
		t.Fatalf("llm_stream_idle_timeout=%s", got.Config.LLMStreamIdleTimeout)
	}
	if got.Config.CloudLLMStreamIdleTimeout != 7*time.Minute {
		t.Fatalf("cloud_llm_stream_idle_timeout=%s", got.Config.CloudLLMStreamIdleTimeout)
	}
	if got.Sources.LLMStreamIdleTimeout != "project" {
		t.Fatalf("LLMStreamIdleTimeout source=%q", got.Sources.LLMStreamIdleTimeout)
	}
	if got.Sources.CloudLLMStreamIdleTimeout != "project" {
		t.Fatalf("CloudLLMStreamIdleTimeout source=%q", got.Sources.CloudLLMStreamIdleTimeout)
	}
}

func TestLoadWatchdogTimeoutFlagsOverride(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	if err := os.WriteFile(user, []byte(`llm_stream_idle_timeout="110s"`+"\n"+`cloud_llm_stream_idle_timeout="6m"`), 0o644); err != nil {
		t.Fatal(err)
	}
	localIdle := "95s"
	cloudIdle := "8m"
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{
		LLMStreamIdleTimeout:      &localIdle,
		CloudLLMStreamIdleTimeout: &cloudIdle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.LLMStreamIdleTimeout != 95*time.Second {
		t.Fatalf("llm_stream_idle_timeout=%s", got.Config.LLMStreamIdleTimeout)
	}
	if got.Config.CloudLLMStreamIdleTimeout != 8*time.Minute {
		t.Fatalf("cloud_llm_stream_idle_timeout=%s", got.Config.CloudLLMStreamIdleTimeout)
	}
	if got.Sources.LLMStreamIdleTimeout != "flag" {
		t.Fatalf("LLMStreamIdleTimeout source=%q", got.Sources.LLMStreamIdleTimeout)
	}
	if got.Sources.CloudLLMStreamIdleTimeout != "flag" {
		t.Fatalf("CloudLLMStreamIdleTimeout source=%q", got.Sources.CloudLLMStreamIdleTimeout)
	}
}

func TestLoadSemanticIndexConfigAndSources(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	body := strings.Join([]string{
		`chat_keep_alive = "7m"`,
		"[semantic_index]",
		`enabled = true`,
		`mode = "explicit"`,
		`auto_build = true`,
		`model = "qwen3-embedding:8b"`,
		`dimensions = 768`,
		`max_chunk_tokens = 600`,
		`chunk_overlap_tokens = 100`,
		`max_file_bytes = 222222`,
		`max_records = 12345`,
		`batch_size = 16`,
		`top_k_records = 33`,
		`top_k_files = 9`,
		`max_context_bytes = 123456`,
		`light_top_k_records = 5`,
		`light_top_k_files = 2`,
		`light_max_context_bytes = 9100`,
		`light_deadline_ms = 1300`,
		`full_top_k_records = 15`,
		`full_top_k_files = 5`,
		`full_max_context_bytes = 70000`,
		`full_deadline_ms = 3100`,
		`deep_top_k_records = 45`,
		`deep_top_k_files = 13`,
		`deep_max_context_bytes = 300000`,
		`deep_deadline_ms = 3200`,
		`hybrid_lexical_weight = 0.35`,
		`frecency_weight = 0.25`,
		`prompt_refresh_max_files = 5`,
		`prompt_refresh_timeout_ms = 1800`,
		`keep_alive = "15m"`,
		`query_keep_alive = "45s"`,
		`build_keep_alive = "12m"`,
		`store_previews = false`,
	}, "\n")
	if err := os.WriteFile(user, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.ChatKeepAlive != "7m" {
		t.Fatalf("chat_keep_alive=%q", got.Config.ChatKeepAlive)
	}
	if got.Config.SemanticIndex.Mode != "explicit" {
		t.Fatalf("semantic mode=%q", got.Config.SemanticIndex.Mode)
	}
	if got.Config.SemanticIndex.Dimensions != 768 {
		t.Fatalf("dimensions=%d", got.Config.SemanticIndex.Dimensions)
	}
	if got.Config.SemanticIndex.LightTopKRecords != 5 || got.Config.SemanticIndex.FullTopKRecords != 15 || got.Config.SemanticIndex.DeepTopKRecords != 45 {
		t.Fatalf("semantic top-k tiers not loaded: %+v", got.Config.SemanticIndex)
	}
	if got.Config.SemanticIndex.QueryKeepAlive != "45s" || got.Config.SemanticIndex.BuildKeepAlive != "12m" {
		t.Fatalf("semantic keepalive split not loaded: %+v", got.Config.SemanticIndex)
	}
	if got.Config.SemanticIndex.PromptRefreshTimeout != 1800*time.Millisecond {
		t.Fatalf("prompt_refresh_timeout=%s", got.Config.SemanticIndex.PromptRefreshTimeout)
	}
	if got.Config.SemanticIndex.StorePreviews {
		t.Fatalf("store_previews should be false")
	}
	if got.Sources.ChatKeepAlive != "user" || got.Sources.SemanticIndexModel != "user" || got.Sources.SemanticIndexPromptRefreshTimeout != "user" {
		t.Fatalf("unexpected semantic sources: %+v", got.Sources)
	}
}

func TestLoadSemanticIndexNormalization(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	body := strings.Join([]string{
		"[semantic_index]",
		`model = ""`,
		`dimensions = 0`,
		`batch_size = 0`,
		`prompt_refresh_timeout_ms = 0`,
	}, "\n")
	if err := os.WriteFile(user, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.SemanticIndex.Model == "" || got.Config.SemanticIndex.Dimensions <= 0 {
		t.Fatalf("semantic config should be normalized: %+v", got.Config.SemanticIndex)
	}
	if got.Config.SemanticIndex.PromptRefreshTimeout <= 0 {
		t.Fatalf("prompt_refresh_timeout not normalized")
	}
	if len(got.Warnings) == 0 {
		t.Fatalf("expected warnings for invalid semantic config values")
	}
}

func TestLoadWatchdogTimeoutClamp(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	if err := os.WriteFile(user, []byte(`llm_stream_idle_timeout="500ms"`+"\n"+`cloud_llm_stream_idle_timeout="500ms"`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.LLMStreamIdleTimeout != time.Second {
		t.Fatalf("llm_stream_idle_timeout=%s", got.Config.LLMStreamIdleTimeout)
	}
	if got.Config.CloudLLMStreamIdleTimeout != time.Second {
		t.Fatalf("cloud_llm_stream_idle_timeout=%s", got.Config.CloudLLMStreamIdleTimeout)
	}
}

func TestLoadWatchdogTimeoutInvalidDurationFails(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	user := filepath.Join(td, "user.toml")
	if err := os.WriteFile(user, []byte(`llm_stream_idle_timeout="abc"`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(user, filepath.Join(td, "missing.toml"), FlagOverrides{})
	if err == nil {
		t.Fatal("expected load error for invalid duration")
	}
}
