package state

import (
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
)

func TestOnChangeMirrorsModel(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	initial.DefaultModel = "old-model"
	bootstrap.ResetGlobalForTest(initial)

	prev := App{ActiveModel: "old-model"}
	next := App{ActiveModel: "new-model"}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if snap.DefaultModel != "new-model" {
		t.Errorf("expected DefaultModel to mirror new-model, got %s", snap.DefaultModel)
	}
}

func TestOnChangeMirrorsProviderFields(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	initial.LLMProvider = "ollama_local"
	initial.LLMBaseURL = "http://localhost:11434"
	bootstrap.ResetGlobalForTest(initial)

	prev := App{LLMProvider: "ollama_local", LLMBaseURL: "http://localhost:11434"}
	next := App{LLMProvider: "ollama_cloud_api", LLMBaseURL: llm.OllamaCloudBaseURL}
	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if snap.LLMProvider != "ollama_cloud_api" || snap.LLMBaseURL != llm.OllamaCloudBaseURL {
		t.Fatalf("snapshot provider=%q base=%q", snap.LLMProvider, snap.LLMBaseURL)
	}
}

func TestOnChangeMirrorsWorkingDir(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	initial.WorkingDir = "/old/work"
	bootstrap.ResetGlobalForTest(initial)

	prev := App{ToolSettings: ToolSettings{WorkingDir: "/old/work"}}
	next := App{ToolSettings: ToolSettings{WorkingDir: "/new/work"}}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if snap.WorkingDir != "/new/work" {
		t.Errorf("expected WorkingDir to mirror /new/work, got %s", snap.WorkingDir)
	}
}

func TestOnChangeMirrorsBashTimeout(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	initial.BashTimeout = 5 * time.Minute
	bootstrap.ResetGlobalForTest(initial)

	prev := App{ToolSettings: ToolSettings{BashTimeout: 5 * time.Minute}}
	next := App{ToolSettings: ToolSettings{BashTimeout: 10 * time.Minute}}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if snap.BashTimeout != 10*time.Minute {
		t.Errorf("expected BashTimeout to mirror 10m, got %v", snap.BashTimeout)
	}
}

func TestOnChangeMirrorsMaxResultChars(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	bootstrap.ResetGlobalForTest(initial)

	prev := App{ToolSettings: ToolSettings{MaxResultChars: 1000}}
	next := App{ToolSettings: ToolSettings{MaxResultChars: 2000}}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if snap.MaxResultChars != 2000 {
		t.Errorf("expected MaxResultChars to mirror 2000, got %d", snap.MaxResultChars)
	}
}

func TestOnChangeMirrorsMaxReadChars(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	bootstrap.ResetGlobalForTest(initial)

	prev := App{ToolSettings: ToolSettings{MaxReadChars: 10000}}
	next := App{ToolSettings: ToolSettings{MaxReadChars: 20000}}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if snap.MaxReadChars != 20000 {
		t.Errorf("expected MaxReadChars to mirror 20000, got %d", snap.MaxReadChars)
	}
}

func TestOnChangeMirrorsDirectoryCaps(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	bootstrap.ResetGlobalForTest(initial)

	prev := App{ToolSettings: ToolSettings{
		MaxDirFiles:    10,
		MaxPromptFiles: 20,
		MaxDirBytes:    1000,
		MaxPromptBytes: 2000,
		MaxDirDepth:    2,
	}}
	next := App{ToolSettings: ToolSettings{
		MaxDirFiles:    200,
		MaxPromptFiles: 400,
		MaxDirBytes:    512 * 1024,
		MaxPromptBytes: 2 * 1024 * 1024,
		MaxDirDepth:    8,
	}}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if snap.MaxDirFiles != 200 || snap.MaxPromptFiles != 400 {
		t.Fatalf("unexpected file caps: dir=%d prompt=%d", snap.MaxDirFiles, snap.MaxPromptFiles)
	}
	if snap.MaxDirBytes != 512*1024 || snap.MaxPromptBytes != 2*1024*1024 {
		t.Fatalf("unexpected byte caps: dir=%d prompt=%d", snap.MaxDirBytes, snap.MaxPromptBytes)
	}
	if snap.MaxDirDepth != 8 {
		t.Fatalf("unexpected depth cap: %d", snap.MaxDirDepth)
	}
}

func TestOnChangeMirrorsPermissionMode(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	initial.PermissionMode = permissions.ModeDefault
	bootstrap.ResetGlobalForTest(initial)

	prev := App{PermissionMode: permissions.ModeDefault}
	next := App{PermissionMode: permissions.ModePlan}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if snap.PermissionMode != permissions.ModePlan {
		t.Errorf("expected PermissionMode to mirror ModePlan, got %s", snap.PermissionMode)
	}
}

func TestOnChangeMirrorsPermissionRules(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	bootstrap.ResetGlobalForTest(initial)

	prevRules := permissions.Rules{
		AlwaysAllow: []permissions.Rule{{Pattern: "bash(old)", Source: permissions.SourceUser}},
	}
	nextRules := permissions.Rules{
		AlwaysAllow: []permissions.Rule{{Pattern: "bash(new)", Source: permissions.SourceUser}},
	}

	prev := App{PermissionRules: prevRules}
	next := App{PermissionRules: nextRules}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if len(snap.PermissionRules.AlwaysAllow) != 1 {
		t.Errorf("expected 1 allow rule, got %d", len(snap.PermissionRules.AlwaysAllow))
	}
	if snap.PermissionRules.AlwaysAllow[0].Pattern != "bash(new)" {
		t.Errorf("expected pattern bash(new), got %s", snap.PermissionRules.AlwaysAllow[0].Pattern)
	}
}

func TestOnChangeDoesNotMirrorMessages(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	bootstrap.ResetGlobalForTest(initial)

	prev := App{}
	next := App{}

	// Messages field exists only in App, not in bootstrap.Snapshot
	// This is verified by the fact that bootstrap.Snapshot has no Messages field
	// We can't directly test the mirroring, but we verify bootstrap Snapshot doesn't have Messages
	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	// If we try to access snap.Messages, it won't exist (compile error)
	// This is verified by the fact that bootstrap.Snapshot has no Messages field
	_ = snap
}

func TestOnChangeDoesNotMirrorActiveTools(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	bootstrap.ResetGlobalForTest(initial)

	prev := App{ActiveTools: map[string]ToolUse{}}
	next := App{ActiveTools: map[string]ToolUse{"tool1": {ID: "tool1", Name: "bash"}}}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	// bootstrap.Snapshot should not have an ActiveTools field
	// This is implicitly verified; if it did, we'd need to explicitly test it doesn't get mirrored
	_ = snap
}

func TestOnChangeSkipsUnchangedFields(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	initial.DefaultModel = "test-model"
	initial.WorkingDir = "/work"
	bootstrap.ResetGlobalForTest(initial)

	// prev and next have same values for model, different for working dir
	prev := App{
		ActiveModel:  "test-model",
		ToolSettings: ToolSettings{WorkingDir: "/work"},
	}
	next := App{
		ActiveModel:  "test-model",
		ToolSettings: ToolSettings{WorkingDir: "/new-work"},
	}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if snap.DefaultModel != "test-model" {
		t.Errorf("expected DefaultModel unchanged")
	}
	if snap.WorkingDir != "/new-work" {
		t.Errorf("expected WorkingDir updated to /new-work")
	}
}

func TestOnChangeZeroValueApp(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	bootstrap.ResetGlobalForTest(initial)

	var prev App
	var next App

	// Should not panic with zero-value apps
	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	// Should have bootstrap defaults, not panic
	if snap.DefaultModel == "" {
		t.Error("expected DefaultModel to have a value after OnChange")
	}
}

func TestOnChangeWithMultipleRuleTypes(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	bootstrap.ResetGlobalForTest(initial)

	prevRules := permissions.Rules{
		AlwaysAllow: []permissions.Rule{},
		AlwaysDeny:  []permissions.Rule{},
		AlwaysAsk:   []permissions.Rule{},
	}
	nextRules := permissions.Rules{
		AlwaysAllow: []permissions.Rule{{Pattern: "bash(ls)", Source: permissions.SourceUser}},
		AlwaysDeny:  []permissions.Rule{{Pattern: "bash(rm -rf /)", Source: permissions.SourcePolicy}},
		AlwaysAsk:   []permissions.Rule{{Pattern: "filewrite(/etc/)", Source: permissions.SourceProject}},
	}

	prev := App{PermissionRules: prevRules}
	next := App{PermissionRules: nextRules}

	OnChange(prev, next)

	snap := bootstrap.Global().Snapshot()
	if len(snap.PermissionRules.AlwaysAllow) != 1 {
		t.Errorf("expected 1 allow rule, got %d", len(snap.PermissionRules.AlwaysAllow))
	}
	if len(snap.PermissionRules.AlwaysDeny) != 1 {
		t.Errorf("expected 1 deny rule, got %d", len(snap.PermissionRules.AlwaysDeny))
	}
	if len(snap.PermissionRules.AlwaysAsk) != 1 {
		t.Errorf("expected 1 ask rule, got %d", len(snap.PermissionRules.AlwaysAsk))
	}
}

func TestRulesEqualFunc(t *testing.T) {
	rule1 := permissions.Rule{Pattern: "bash(ls)", Source: permissions.SourceUser}
	rule2 := permissions.Rule{Pattern: "bash(ls)", Source: permissions.SourceUser}
	rule3 := permissions.Rule{Pattern: "bash(cat)", Source: permissions.SourceUser}

	rules1 := permissions.Rules{AlwaysAllow: []permissions.Rule{rule1}}
	rules2 := permissions.Rules{AlwaysAllow: []permissions.Rule{rule2}}
	rules3 := permissions.Rules{AlwaysAllow: []permissions.Rule{rule3}}

	if !rulesEqual(rules1, rules2) {
		t.Error("expected identical rules to be equal")
	}
	if rulesEqual(rules1, rules3) {
		t.Error("expected different patterns to not be equal")
	}
	if rulesEqual(rules1, permissions.Rules{}) {
		t.Error("expected non-empty and empty rules to not be equal")
	}
}
