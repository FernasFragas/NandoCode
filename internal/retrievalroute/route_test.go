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
