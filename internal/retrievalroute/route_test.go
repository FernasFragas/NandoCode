package retrievalroute

import "testing"

func TestDecideExplicitAccessSkipsEmbedding(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:         "@docs/plan.md can you access this?",
		ShouldQuery:       true,
		AttachmentPolicy:  "content",
		AttachedFileCount: 1,
		CurrentTurnPaths: []string{
			"docs/plan.md",
		},
		SemanticEnabled: true,
		SemanticMode:    "auto",
	}, Config{Mode: "auto"})
	if d.Action != ActionExplicitContextOnly || d.AllowEmbedding {
		t.Fatalf("decision=%+v", d)
	}
	if d.Reason != ReasonSkipExplicitContext {
		t.Fatalf("reason=%q", d.Reason)
	}
}

func TestDecideListingSkipsEmbedding(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:        "list files in @docs/",
		ShouldQuery:      true,
		AttachmentPolicy: "listing_tree_only",
		SemanticEnabled:  true,
		SemanticMode:     "auto",
	}, Config{Mode: "auto"})
	if d.Action != ActionLocalSearchOnly || d.AllowEmbedding {
		t.Fatalf("decision=%+v", d)
	}
	if d.Reason != ReasonSkipListingIntent {
		t.Fatalf("reason=%q", d.Reason)
	}
}

func TestDecideBroadPromptUsesSemanticFull(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:       "fix the authentication bug",
		ShouldQuery:     true,
		SemanticEnabled: true,
		SemanticMode:    "auto",
	}, Config{Mode: "auto"})
	if d.Action != ActionSemanticFull || !d.AllowEmbedding {
		t.Fatalf("decision=%+v", d)
	}
	if d.Profile != "full" {
		t.Fatalf("profile=%q", d.Profile)
	}
	if d.ToolMode != ToolModeDefault {
		t.Fatalf("tool mode=%q", d.ToolMode)
	}
}

func TestDecideBroadPromptSkipsWhenKnownIndexMissing(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:       "fix the authentication bug",
		ShouldQuery:     true,
		IndexKnown:      true,
		HasIndex:        false,
		SemanticEnabled: true,
		SemanticMode:    "auto",
	}, Config{Mode: "auto"})
	if d.AllowEmbedding || d.Action != ActionSkipAllRetrieval {
		t.Fatalf("decision=%+v", d)
	}
	if d.Reason != ReasonSkipIndexMissing {
		t.Fatalf("reason=%q", d.Reason)
	}
}

func TestDecideRelatedPromptSkipsSemanticWhenKnownIndexIncompatible(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:         "@internal/server/session.go find related callers",
		ShouldQuery:       true,
		AttachmentPolicy:  "content",
		AttachedFileCount: 1,
		CurrentTurnPaths:  []string{"internal/server/session.go"},
		IndexKnown:        true,
		HasIndex:          true,
		IndexCompatible:   false,
		SemanticEnabled:   true,
		SemanticMode:      "auto",
	}, Config{Mode: "auto"})
	if d.AllowEmbedding || d.Action != ActionExplicitContextOnly {
		t.Fatalf("decision=%+v", d)
	}
	if d.Reason != ReasonSkipDimensionsMismatch {
		t.Fatalf("reason=%q", d.Reason)
	}
}

func TestDecideGeneralQuestionSkipsEmbedding(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:       "how is the weather",
		ShouldQuery:     true,
		SemanticEnabled: true,
		SemanticMode:    "auto",
	}, Config{Mode: "auto"})
	if d.Action != ActionSkipAllRetrieval || d.AllowEmbedding {
		t.Fatalf("decision=%+v", d)
	}
	if d.Reason != ReasonSkipGeneralPrompt {
		t.Fatalf("reason=%q", d.Reason)
	}
	if d.ToolMode != ToolModeNone {
		t.Fatalf("tool mode=%q", d.ToolMode)
	}
	if d.RequestProfile != "general_prompt" {
		t.Fatalf("request profile=%q", d.RequestProfile)
	}
}

func TestDecideMemoryRecallSkipsEmbedding(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:       "recall the previous context about auth",
		ShouldQuery:     true,
		SemanticEnabled: true,
		SemanticMode:    "auto",
	}, Config{Mode: "auto"})
	if d.Action != ActionSkipAllRetrieval || d.AllowEmbedding {
		t.Fatalf("decision=%+v", d)
	}
	if d.Reason != ReasonSkipMemoryRecall {
		t.Fatalf("reason=%q", d.Reason)
	}
	if d.ToolMode != ToolModeDefault {
		t.Fatalf("tool mode=%q", d.ToolMode)
	}
	if d.RequestProfile != "memory_recall" {
		t.Fatalf("request profile=%q", d.RequestProfile)
	}
}

func TestDecideRelatedPromptUsesSemanticLight(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:         "@internal/server/session.go find related callers",
		ShouldQuery:       true,
		AttachmentPolicy:  "content",
		AttachedFileCount: 1,
		CurrentTurnPaths:  []string{"internal/server/session.go"},
		SemanticEnabled:   true,
		SemanticMode:      "auto",
	}, Config{Mode: "auto"})
	if d.Action != ActionSemanticLight || !d.AllowEmbedding {
		t.Fatalf("decision=%+v", d)
	}
	if !d.UseCurrentPathWeight {
		t.Fatalf("expected UseCurrentPathWeight=true")
	}
}

func TestDecideSemanticOffSkipsBroadPrompt(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:       "fix the authentication bug",
		ShouldQuery:     true,
		SemanticEnabled: false,
		SemanticMode:    "off",
	}, Config{Mode: "auto"})
	if d.Action != ActionExplicitContextOnly || d.AllowEmbedding {
		t.Fatalf("decision=%+v", d)
	}
	if d.Reason != ReasonSkipExplicitContext {
		t.Fatalf("reason=%q", d.Reason)
	}
	if d.ToolMode != ToolModeDefault {
		t.Fatalf("tool mode=%q", d.ToolMode)
	}
	if d.RequestProfile != "semantic_off" {
		t.Fatalf("request profile=%q", d.RequestProfile)
	}
}

func TestDecideDeepForcesDeepProfile(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:       "fix the authentication bug",
		ShouldQuery:     true,
		ForceDeep:       true,
		SemanticEnabled: true,
		SemanticMode:    "auto",
	}, Config{Mode: "auto"})
	if d.Action != ActionSemanticFull || !d.AllowEmbedding {
		t.Fatalf("decision=%+v", d)
	}
	if d.Profile != "deep" {
		t.Fatalf("profile=%q", d.Profile)
	}
}

func TestDecideNoQuerySkipsAll(t *testing.T) {
	t.Parallel()
	d := Decide(Input{
		RawPrompt:       "/index build",
		ShouldQuery:     false,
		SemanticEnabled: true,
	}, Config{Mode: "auto"})
	if d.Action != ActionSkipAllRetrieval || d.AllowEmbedding {
		t.Fatalf("decision=%+v", d)
	}
	if d.Reason != ReasonSkipLocalCommand {
		t.Fatalf("reason=%q", d.Reason)
	}
}

func TestDecideToolModeAndAction(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name         string
		prompt       string
		semantic     bool
		wantAction   Action
		wantToolMode ToolMode
	}{
		{"agent event loop", "explain the agent event loop", true, ActionSemanticFull, ToolModeDefault},
		{"endpoint location", "where is the endpoint registered", true, ActionSemanticFull, ToolModeDefault},
		{"capital is not api", "what is the capital of France", true, ActionSkipAllRetrieval, ToolModeNone},
		{"author is not auth", "who is the author of this novel", true, ActionSkipAllRetrieval, ToolModeNone},
		{"auth bug", "fix the authentication bug", true, ActionSemanticFull, ToolModeDefault},
		{"weather", "how is the weather", true, ActionSkipAllRetrieval, ToolModeNone},
		{"code question semantic off", "explain the session model", false, ActionExplicitContextOnly, ToolModeDefault},
		{"chitchat semantic off keeps tools", "tell me a joke", false, ActionExplicitContextOnly, ToolModeDefault},
		{"verify route handler", "verify the route handler", true, ActionSemanticFull, ToolModeDefault},
		{"camelCase identifier", "what does parseConfig do", true, ActionSkipAllRetrieval, ToolModeDefault},
		{"exact reply", "Respond with exactly: ok", true, ActionSkipAllRetrieval, ToolModeNone},
	}
	for _, row := range rows {
		row := row
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			d := Decide(Input{RawPrompt: row.prompt, ShouldQuery: true, SemanticEnabled: row.semantic, SemanticMode: "auto"}, Config{Mode: "auto"})
			if d.Action != row.wantAction {
				t.Errorf("action=%q want %q", d.Action, row.wantAction)
			}
			if d.ToolMode != row.wantToolMode {
				t.Errorf("tool mode=%q want %q", d.ToolMode, row.wantToolMode)
			}
		})
	}
}

func TestDecideCodeIdentifiersNeverDisableTools(t *testing.T) {
	t.Parallel()
	prompts := []string{
		"what does the endpoint do",
		"how does the server start",
		"describe the session lifecycle",
		"which model does it use",
		"trace the route for chat",
		"verify this works",
		"explain the code",
		"what is in main.go",
		"look at internal/server/handler.go",
		"why does build_prompt fail",
		"read README", "update the README", "edit the readme", "run ls", "git status",
		"run npm install", "grep for TODO", "show me the logs", "what changed",
		"summarize this", "add logging", "delete the temp folder",
	}
	for _, semantic := range []bool{true, false} {
		for _, p := range prompts {
			d := Decide(Input{RawPrompt: p, ShouldQuery: true, SemanticEnabled: semantic, SemanticMode: "auto"}, Config{Mode: "auto"})
			if d.ToolMode == ToolModeNone {
				t.Errorf("prompt %q (semantic=%v) got ToolModeNone", p, semantic)
			}
		}
	}
}
