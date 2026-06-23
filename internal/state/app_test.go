package state

import (
	"context"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/types"
)

func TestDefaultAppInitialization(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	initial.DefaultModel = "test-model"
	initial.BashTimeout = 10 * time.Minute
	state := bootstrap.New(initial)
	snap := state.Snapshot()

	app := DefaultApp(snap)

	if app.ActiveModel != "test-model" {
		t.Errorf("expected model test-model, got %s", app.ActiveModel)
	}
	if app.LLMProvider == "" || app.LLMBaseURL == "" {
		t.Fatalf("expected provider metadata to be initialized, got provider=%q base=%q", app.LLMProvider, app.LLMBaseURL)
	}
	if app.ToolSettings.BashTimeout != 10*time.Minute {
		t.Errorf("expected bash timeout 10m, got %v", app.ToolSettings.BashTimeout)
	}
	if app.ToolSettings.WorkingDir != snap.WorkingDir {
		t.Errorf("expected working dir from snapshot")
	}
	if app.PermissionMode != snap.PermissionMode {
		t.Errorf("expected permission mode from snapshot")
	}
	if len(app.Messages) != 0 {
		t.Error("expected empty messages")
	}
	if len(app.QueuedPrompts) != 0 {
		t.Error("expected empty queued prompts")
	}
	if app.ActiveRun {
		t.Error("expected active run to be false")
	}
}

func TestAppCloneDeepCopiesMessages(t *testing.T) {
	app := App{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "test"},
		},
	}

	cloned := app.Clone()
	cloned.Messages = append(cloned.Messages, llm.Message{Role: llm.RoleAssistant, Content: "response"})

	if len(app.Messages) != 1 {
		t.Error("modifying clone should not affect original messages")
	}
}

func TestAppCloneDeepCopiesQueuedPrompts(t *testing.T) {
	app := App{
		QueuedPrompts: []string{"prompt1", "prompt2"},
	}

	cloned := app.Clone()
	cloned.QueuedPrompts = append(cloned.QueuedPrompts, "prompt3")

	if len(app.QueuedPrompts) != 2 {
		t.Error("modifying clone should not affect original queued prompts")
	}
}

func TestAppCloneDeepCopiesToolSettings(t *testing.T) {
	app := App{
		ToolSettings: ToolSettings{
			WorkingDir:            "/work",
			AdditionalWorkingDirs: []string{"/extra1", "/extra2"},
			Env:                   []string{"VAR=val"},
			BashTimeout:           5 * time.Minute,
			MaxResultChars:        1024,
			MaxReadChars:          2048,
			MaxDirFiles:           200,
			MaxPromptFiles:        400,
			MaxDirBytes:           512 * 1024,
			MaxPromptBytes:        2 * 1024 * 1024,
			MaxDirDepth:           8,
		},
	}

	cloned := app.Clone()
	cloned.ToolSettings.AdditionalWorkingDirs = append(cloned.ToolSettings.AdditionalWorkingDirs, "/extra3")
	cloned.ToolSettings.Env = append(cloned.ToolSettings.Env, "VAR2=val2")

	if len(app.ToolSettings.AdditionalWorkingDirs) != 2 {
		t.Error("modifying clone tool settings should not affect original")
	}
	if len(app.ToolSettings.Env) != 1 {
		t.Error("modifying clone env should not affect original")
	}
}

func TestAppCloneDeepCopiesActiveTools(t *testing.T) {
	app := App{
		ActiveTools: map[string]ToolUse{
			"tool1": {ID: "tool1", Name: "bash", Summary: "echo hello"},
		},
	}

	cloned := app.Clone()
	cloned.ActiveTools["tool2"] = ToolUse{ID: "tool2", Name: "read", Summary: "read file"}

	if len(app.ActiveTools) != 1 {
		t.Error("modifying clone active tools should not affect original")
	}
}

func TestAppCloneDeepCopiesTasks(t *testing.T) {
	app := App{
		Tasks: map[string]types.TaskSummary{
			"task1": {ID: "task1", Kind: types.KindBash, Status: types.StatusPending},
		},
	}

	cloned := app.Clone()
	cloned.Tasks["task2"] = types.TaskSummary{ID: "task2", Kind: types.KindAgent}

	if len(app.Tasks) != 1 {
		t.Error("modifying clone tasks should not affect original")
	}
}

func TestAppCloneDeepCopiesPermissionPrompt(t *testing.T) {
	prompt := &PermissionPrompt{ID: "perm1", ToolName: "bash", Target: "rm *"}
	app := App{
		PermissionPrompt: prompt,
	}

	cloned := app.Clone()
	if cloned.PermissionPrompt == nil {
		t.Error("expected cloned permission prompt to be set")
	}
	if cloned.PermissionPrompt == app.PermissionPrompt {
		t.Error("permission prompt should be deep copied")
	}
	if cloned.PermissionPrompt.ID != "perm1" {
		t.Errorf("expected prompt ID perm1, got %s", cloned.PermissionPrompt.ID)
	}

	cloned.PermissionPrompt.ID = "modified"
	if app.PermissionPrompt.ID != "perm1" {
		t.Error("modifying cloned prompt should not affect original")
	}
}

func TestAppCloneDeepCopiesRules(t *testing.T) {
	app := App{
		PermissionRules: permissions.Rules{
			AlwaysAllow: []permissions.Rule{{Pattern: "bash(safe)", Source: permissions.SourceUser}},
		},
	}

	cloned := app.Clone()
	cloned.PermissionRules.AlwaysAllow = append(cloned.PermissionRules.AlwaysAllow,
		permissions.Rule{Pattern: "bash(risky)", Source: permissions.SourceSession})

	if len(app.PermissionRules.AlwaysAllow) != 1 {
		t.Error("modifying cloned rules should not affect original")
	}
}

func TestAppCloneZeroValue(t *testing.T) {
	var app App
	cloned := app.Clone()

	if len(cloned.Messages) != 0 {
		t.Error("expected empty messages slice in cloned zero app")
	}
	if cloned.ActiveTools == nil {
		t.Error("expected non-nil active tools map even for zero app")
	}
	if len(cloned.ActiveTools) != 0 {
		t.Error("expected empty active tools map")
	}
	if cloned.Tasks == nil {
		t.Error("expected non-nil tasks map even for zero app")
	}
	if len(cloned.Tasks) != 0 {
		t.Error("expected empty tasks map")
	}
}

func TestToolContextBuilding(t *testing.T) {
	app := App{
		ToolSettings: ToolSettings{
			WorkingDir:            "/work",
			AdditionalWorkingDirs: []string{"/extra"},
			Env:                   []string{"VAR=val"},
			BashTimeout:           10 * time.Minute,
			MaxResultChars:        1024,
			MaxReadChars:          2048,
			MaxDirFiles:           200,
			MaxPromptFiles:        400,
			MaxDirBytes:           512 * 1024,
			MaxPromptBytes:        2 * 1024 * 1024,
			MaxDirDepth:           8,
		},
	}

	ctx := context.Background()
	toolCtx := app.ToolContext(ctx)

	if toolCtx.WorkingDir != "/work" {
		t.Errorf("expected working dir /work, got %s", toolCtx.WorkingDir)
	}
	if len(toolCtx.AdditionalWorkingDirs) != 1 {
		t.Errorf("expected 1 additional dir, got %d", len(toolCtx.AdditionalWorkingDirs))
	}
	if toolCtx.BashTimeout != 10*time.Minute {
		t.Errorf("expected bash timeout 10m, got %v", toolCtx.BashTimeout)
	}
	if toolCtx.MaxResultChars != 1024 {
		t.Errorf("expected max result chars 1024, got %d", toolCtx.MaxResultChars)
	}
	if toolCtx.MaxDirFiles != 200 || toolCtx.MaxPromptFiles != 400 {
		t.Fatalf("unexpected file caps: dir=%d prompt=%d", toolCtx.MaxDirFiles, toolCtx.MaxPromptFiles)
	}
}

func TestToolContextDoesNotStoreContextInApp(t *testing.T) {
	app := App{}
	ctx := context.Background()

	_ = app.ToolContext(ctx)

	// Verify app state wasn't modified - just ensure Clone works without panic
	app2 := app.Clone()
	if len(app2.Messages) != 0 || len(app2.QueuedPrompts) != 0 {
		t.Error("app should be unchanged after ToolContext")
	}
}

func TestVimModeValues(t *testing.T) {
	if VimInsert != "insert" || VimNormal != "normal" || VimVisual != "visual" {
		t.Error("vim mode constants have unexpected values")
	}
}

func TestToolUseStructure(t *testing.T) {
	use := ToolUse{
		ID:        "id1",
		Name:      "bash",
		Summary:   "echo test",
		StartedAt: time.Now(),
		Done:      false,
		Error:     "",
	}

	if use.ID != "id1" || use.Name != "bash" {
		t.Error("tool use fields not set correctly")
	}
}

func TestPermissionPromptStructure(t *testing.T) {
	prompt := PermissionPrompt{
		ID:       "perm1",
		ToolName: "bash",
		Target:   "/etc/passwd",
		Reason:   "user requested",
	}

	if prompt.ToolName != "bash" {
		t.Errorf("expected tool name bash, got %s", prompt.ToolName)
	}
}

func TestDefaultAppWithPermissionRules(t *testing.T) {
	initial := bootstrap.DefaultInitial("")
	initial.PermissionRules = permissions.Rules{
		AlwaysAllow: []permissions.Rule{
			{Pattern: "bash(safe)", Source: permissions.SourceUser},
		},
		AlwaysDeny: []permissions.Rule{
			{Pattern: "bash(rm -rf /)", Source: permissions.SourcePolicy},
		},
	}
	state := bootstrap.New(initial)
	snap := state.Snapshot()

	app := DefaultApp(snap)

	if len(app.PermissionRules.AlwaysAllow) != 1 {
		t.Error("expected allow rules to be copied")
	}
	if len(app.PermissionRules.AlwaysDeny) != 1 {
		t.Error("expected deny rules to be copied")
	}

	// Verify deep copy by modifying the app's rules
	app.PermissionRules.AlwaysAllow = append(app.PermissionRules.AlwaysAllow,
		permissions.Rule{Pattern: "modified", Source: permissions.SourceSession})

	snap2 := state.Snapshot()
	if len(snap2.PermissionRules.AlwaysAllow) != 1 {
		t.Error("modifying app rules should not affect bootstrap snapshot")
	}
}

func TestAppUsageTracking(t *testing.T) {
	app := App{
		Usage: agent.Usage{
			PromptEvalCount: 100,
			EvalCount:       200,
			TotalDuration:   1000000000,
			Turns:           5,
			ToolCalls:       2,
		},
	}

	cloned := app.Clone()
	if cloned.Usage.Turns != 5 {
		t.Errorf("expected turns 5, got %d", cloned.Usage.Turns)
	}
	if cloned.Usage.ToolCalls != 2 {
		t.Errorf("expected tool calls 2, got %d", cloned.Usage.ToolCalls)
	}
}
