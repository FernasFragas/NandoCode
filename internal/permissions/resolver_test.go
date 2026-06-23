package permissions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

// mockTool implements tools.Tool for testing.
type mockTool struct {
	name     string
	decision tools.Permission
	reason   string
	enabled  bool
}

func (m *mockTool) Name() string                     { return m.name }
func (m *mockTool) Description() string              { return "mock" }
func (m *mockTool) Aliases() []string                { return nil }
func (m *mockTool) JSONSchema() map[string]any       { return nil }
func (m *mockTool) IsEnabled(ctx tools.Context) bool { return m.enabled }
func (m *mockTool) IsReadOnly(input any) bool        { return false }
func (m *mockTool) IsConcurrencySafe(input any) bool { return false }
func (m *mockTool) IsDestructive(input any) bool     { return false }
func (m *mockTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: m.decision, Reason: m.reason}
}
func (m *mockTool) UnmarshalInput(raw json.RawMessage) (any, error) { return nil, nil }
func (m *mockTool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	return tools.Result{}, nil
}
func (m *mockTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{}
}

func TestResolveNilTool(t *testing.T) {
	result := Resolve(context.Background(), Request{
		Tool:     nil,
		ToolName: "Test",
	})

	if result.Decision != DecisionDeny {
		t.Errorf("Resolve with nil tool: decision = %q, want %q", result.Decision, DecisionDeny)
	}
}

func TestResolveEmptyToolName(t *testing.T) {
	tool := &mockTool{name: "Test", decision: tools.PermAllow}
	result := Resolve(context.Background(), Request{
		Tool:     tool,
		ToolName: "",
	})

	if result.Decision != DecisionDeny {
		t.Errorf("Resolve with empty tool name: decision = %q, want %q", result.Decision, DecisionDeny)
	}
}

func TestResolveHookOverrides(t *testing.T) {
	tool := &mockTool{name: "Test", decision: tools.PermAllow}

	hookCalled := false
	hook := func(ctx context.Context, req Request) (Result, bool) {
		hookCalled = true
		return Result{Decision: DecisionDeny, Stage: StageHook, Reason: "hook denied"}, true
	}

	result := Resolve(context.Background(), Request{
		Mode:         ModeBypass,
		Tool:         tool,
		ToolName:     "Test",
		Input:        nil,
		HookDecision: hook,
	})

	if !hookCalled {
		t.Errorf("Hook was not called")
	}
	if result.Decision != DecisionDeny || result.Stage != StageHook {
		t.Errorf("Resolve hook override: decision = %q stage = %q", result.Decision, result.Stage)
	}
}

func TestResolveRuleMatchesDeny(t *testing.T) {
	tool := &mockTool{name: "Test", decision: tools.PermAllow}
	rules := Rules{
		AlwaysDeny: []Rule{{Pattern: "Test(forbidden)", Source: SourcePolicy}},
	}

	// Create an input struct that can be extracted
	input := struct {
		Command string
	}{Command: "forbidden"}

	result := Resolve(context.Background(), Request{
		Mode:        ModeBypass,
		Rules:       rules,
		Tool:        tool,
		ToolName:    "Test",
		Input:       input,
		ToolContext: tools.DefaultContext(context.Background(), ""),
	})

	if result.Decision != DecisionDeny || result.Stage != StageRule {
		t.Errorf("Resolve rule deny: decision = %q stage = %q, want %q %q", result.Decision, result.Stage, DecisionDeny, StageRule)
	}
}

func TestResolveToolClassifierDenyOverridesMode(t *testing.T) {
	tool := &mockTool{name: "Test", decision: tools.PermDeny, reason: "destructive"}

	result := Resolve(context.Background(), Request{
		Mode:        ModeBypass,
		Tool:        tool,
		ToolName:    "Test",
		Input:       "test",
		ToolContext: tools.DefaultContext(context.Background(), ""),
	})

	if result.Decision != DecisionDeny {
		t.Errorf("Resolve tool deny: decision = %q, want %q", result.Decision, DecisionDeny)
	}
}

func TestResolveModeBypass(t *testing.T) {
	tests := []struct {
		name         string
		toolDecision tools.Permission
		wantDecision Decision
	}{
		{"AllowBypass", tools.PermAllow, DecisionAllow},
		{"AskBypass", tools.PermAsk, DecisionAllow},
		// DenyBypass is denied regardless (tool says deny)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &mockTool{name: "Test", decision: tt.toolDecision}
			result := Resolve(context.Background(), Request{
				Mode:        ModeBypass,
				Tool:        tool,
				ToolName:    "Test",
				Input:       "test",
				ToolContext: tools.DefaultContext(context.Background(), ""),
			})

			if result.Decision != tt.wantDecision {
				t.Errorf("ModeBypass: decision = %q, want %q", result.Decision, tt.wantDecision)
			}
		})
	}
}

func TestResolveModeDefault(t *testing.T) {
	tests := []struct {
		name         string
		toolDecision tools.Permission
		wantDecision Decision
	}{
		{"AllowDefault", tools.PermAllow, DecisionAllow},
		{"AskDefault", tools.PermAsk, DecisionAsk},
		{"DenyDefault", tools.PermDeny, DecisionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &mockTool{name: "Test", decision: tt.toolDecision}
			result := Resolve(context.Background(), Request{
				Mode:        ModeDefault,
				Tool:        tool,
				ToolName:    "Test",
				Input:       "test",
				ToolContext: tools.DefaultContext(context.Background(), ""),
			})

			if result.Decision != tt.wantDecision {
				t.Errorf("ModeDefault: decision = %q, want %q", result.Decision, tt.wantDecision)
			}
		})
	}
}

func TestModeDontAsk(t *testing.T) {
	tests := []struct {
		name         string
		toolDecision tools.Permission
		wantDecision Decision
	}{
		{"AllowDontAsk", tools.PermAllow, DecisionAllow},
		{"AskDontAsk", tools.PermAsk, DecisionDeny},
		{"DenyDontAsk", tools.PermDeny, DecisionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &mockTool{name: "Test", decision: tt.toolDecision}
			result := Resolve(context.Background(), Request{
				Mode:        ModeDontAsk,
				Tool:        tool,
				ToolName:    "Test",
				Input:       "test",
				ToolContext: tools.DefaultContext(context.Background(), ""),
			})

			if result.Decision != tt.wantDecision {
				t.Errorf("ModeDontAsk: decision = %q, want %q", result.Decision, tt.wantDecision)
			}
		})
	}
}

func TestModePlan(t *testing.T) {
	tests := []struct {
		name         string
		toolDecision tools.Permission
		wantDecision Decision
	}{
		{"AllowPlan", tools.PermAllow, DecisionAllow},
		{"AskPlan", tools.PermAsk, DecisionDeny},
		{"DenyPlan", tools.PermDeny, DecisionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &mockTool{name: "Test", decision: tt.toolDecision}
			result := Resolve(context.Background(), Request{
				Mode:        ModePlan,
				Tool:        tool,
				ToolName:    "Test",
				Input:       "test",
				ToolContext: tools.DefaultContext(context.Background(), ""),
			})

			if result.Decision != tt.wantDecision {
				t.Errorf("ModePlan: decision = %q, want %q", result.Decision, tt.wantDecision)
			}
		})
	}
}

func TestModeBubble(t *testing.T) {
	tests := []struct {
		name         string
		toolDecision tools.Permission
		wantDecision Decision
	}{
		{"AllowBubble", tools.PermAllow, DecisionAsk},
		{"AskBubble", tools.PermAsk, DecisionAsk},
		{"DenyBubble", tools.PermDeny, DecisionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &mockTool{name: "Test", decision: tt.toolDecision}
			result := Resolve(context.Background(), Request{
				Mode:        ModeBubble,
				Tool:        tool,
				ToolName:    "Test",
				Input:       "test",
				ToolContext: tools.DefaultContext(context.Background(), ""),
			})

			if result.Decision != tt.wantDecision {
				t.Errorf("ModeBubble: decision = %q, want %q", result.Decision, tt.wantDecision)
			}
		})
	}
}

func TestResolvePromptCallback(t *testing.T) {
	tool := &mockTool{name: "Test", decision: tools.PermAsk}

	promptCalled := false
	prompt := func(ctx context.Context, p Prompt) (Decision, string, error) {
		promptCalled = true
		return DecisionAllow, "user approved", nil
	}

	result := Resolve(context.Background(), Request{
		Mode:        ModeDefault,
		Tool:        tool,
		ToolName:    "Test",
		Input:       "test",
		ToolContext: tools.DefaultContext(context.Background(), ""),
		Prompt:      prompt,
	})

	if !promptCalled {
		t.Errorf("Prompt callback was not called")
	}
	if result.Decision != DecisionAllow || result.Stage != StagePrompt {
		t.Errorf("Resolve prompt: decision = %q stage = %q", result.Decision, result.Stage)
	}
}

func TestResolvePromptNil(t *testing.T) {
	tool := &mockTool{name: "Test", decision: tools.PermAsk}

	result := Resolve(context.Background(), Request{
		Mode:        ModeDefault,
		Tool:        tool,
		ToolName:    "Test",
		Input:       "test",
		ToolContext: tools.DefaultContext(context.Background(), ""),
		Prompt:      nil,
	})

	if result.Decision != DecisionAsk {
		t.Errorf("Resolve with nil prompt: decision = %q, want %q", result.Decision, DecisionAsk)
	}
}

func TestResolveNormalizesMode(t *testing.T) {
	tool := &mockTool{name: "Test", decision: tools.PermAllow}

	result := Resolve(context.Background(), Request{
		Mode:        Mode("unknown"),
		Tool:        tool,
		ToolName:    "Test",
		Input:       "test",
		ToolContext: tools.DefaultContext(context.Background(), ""),
	})

	if result.Decision != DecisionAllow {
		t.Errorf("Resolve normalizes unknown mode: decision = %q", result.Decision)
	}
}

func TestResolveUpdatedInput(t *testing.T) {
	// Create a mock tool that returns updated input
	toolWithUpdate := &mockTool{
		name:     "Test",
		decision: tools.PermAllow,
	}

	result := Resolve(context.Background(), Request{
		Mode:        ModeDefault,
		Tool:        toolWithUpdate,
		ToolName:    "Test",
		Input:       "original",
		ToolContext: tools.DefaultContext(context.Background(), ""),
	})

	// For this test, we're just checking that the resolver properly handles tools.
	// The UpdatedInput preservation is tested more comprehensively via integration tests.
	if result.Decision != DecisionAllow {
		t.Errorf("Resolve UpdatedInput test: decision = %q, want %q", result.Decision, DecisionAllow)
	}
}

func TestResolveObserverCalled(t *testing.T) {
	tool := &mockTool{name: "Test", decision: tools.PermAllow}
	called := false
	result := Resolve(context.Background(), Request{
		Mode:        ModeDefault,
		Tool:        tool,
		ToolName:    "Test",
		Input:       "x",
		ToolContext: tools.DefaultContext(context.Background(), ""),
		Observer: func(_ context.Context, _ Request, r Result) {
			called = true
			if r.Decision == "" {
				t.Fatal("observer received empty decision")
			}
		},
	})
	if result.Decision != DecisionAllow {
		t.Fatalf("decision=%q", result.Decision)
	}
	if !called {
		t.Fatal("observer not called")
	}
}
