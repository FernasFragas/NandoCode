package tui

import "testing"

func TestParseSlashCommand(t *testing.T) {
	tests := []struct {
		input string
		cmd   string
		args  []string
	}{
		{"/help", "help", []string{}},
		{"/clear", "clear", []string{}},
		{"/exit", "exit", []string{}},
		{"/model qwen3:14b", "model", []string{"qwen3:14b"}},
		{"/model my-model:7b", "model", []string{"my-model:7b"}},
		{"prompt", "", nil},
		{"", "", nil},
	}

	for _, tt := range tests {
		cmd, args := ParseSlashCommand(tt.input)
		if cmd != tt.cmd {
			t.Errorf("ParseSlashCommand(%q) cmd = %q, want %q", tt.input, cmd, tt.cmd)
		}
		if len(args) != len(tt.args) {
			t.Errorf("ParseSlashCommand(%q) args length = %d, want %d", tt.input, len(args), len(tt.args))
		}
		for i, arg := range args {
			if arg != tt.args[i] {
				t.Errorf("ParseSlashCommand(%q) args[%d] = %q, want %q", tt.input, i, arg, tt.args[i])
			}
		}
	}
}

func TestTranscriptHelpers(t *testing.T) {
	// Test AppendAssistantDelta
	var items []TranscriptItem
	items = AppendAssistantDelta(items, "Hello ")
	items = AppendAssistantDelta(items, "World")

	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}
	if items[0].Content != "Hello World" {
		t.Errorf("Expected 'Hello World', got %q", items[0].Content)
	}

	// Test AppendThinkingDelta
	var thinkItems []TranscriptItem
	thinkItems = AppendThinkingDelta(thinkItems, "thinking ")
	thinkItems = AppendThinkingDelta(thinkItems, "continues")

	if len(thinkItems) != 1 {
		t.Errorf("Expected 1 item, got %d", len(thinkItems))
	}
	if thinkItems[0].Content != "thinking continues" {
		t.Errorf("Expected 'thinking continues', got %q", thinkItems[0].Content)
	}
	if !thinkItems[0].Collapsed {
		t.Error("Expected thinking item to be collapsed")
	}

	// Test CreateToolItem
	toolItem := CreateToolItem("tool1", "Bash")
	if toolItem.Kind != TranscriptTool {
		t.Errorf("Expected TranscriptTool, got %v", toolItem.Kind)
	}
	if toolItem.ToolID != "tool1" {
		t.Errorf("Expected ToolID 'tool1', got %q", toolItem.ToolID)
	}

	// Test CreateUserItem
	userItem := CreateUserItem("user input")
	if userItem.Kind != TranscriptUser {
		t.Errorf("Expected TranscriptUser, got %v", userItem.Kind)
	}

	// Test CreateSystemItem
	sysItem := CreateSystemItem("system message")
	if sysItem.Kind != TranscriptSystem {
		t.Errorf("Expected TranscriptSystem, got %v", sysItem.Kind)
	}
}

func TestAppendAssistantDeltaStartsNewResponseAfterUser(t *testing.T) {
	items := []TranscriptItem{
		CreateUserItem("first prompt"),
		{Kind: TranscriptAssistant, Content: "first response"},
		CreateUserItem("second prompt"),
	}

	items = AppendAssistantDelta(items, "second response")

	if len(items) != 4 {
		t.Fatalf("expected 4 transcript items, got %d", len(items))
	}
	if items[1].Content != "first response" {
		t.Fatalf("previous assistant content = %q, want unchanged", items[1].Content)
	}
	if items[3].Kind != TranscriptAssistant || items[3].Content != "second response" {
		t.Fatalf("new assistant item = %#v, want second response after latest user prompt", items[3])
	}
}

func TestAppendThinkingDeltaStartsNewBlockAfterUser(t *testing.T) {
	items := []TranscriptItem{
		CreateUserItem("first prompt"),
		{Kind: TranscriptThinking, Content: "old thinking", Collapsed: true},
		CreateUserItem("second prompt"),
	}

	items = AppendThinkingDelta(items, "new thinking")

	if len(items) != 4 {
		t.Fatalf("expected 4 transcript items, got %d", len(items))
	}
	if items[1].Content != "old thinking" {
		t.Fatalf("previous thinking content = %q, want unchanged", items[1].Content)
	}
	if items[3].Kind != TranscriptThinking || items[3].Content != "new thinking" {
		t.Fatalf("new thinking item = %#v, want new thinking after latest user prompt", items[3])
	}
}

func TestVimMode(t *testing.T) {
	vim := NewVimState()

	// Should start in insert mode
	if !vim.IsInsert() {
		t.Error("Expected to start in insert mode")
	}

	// Test transition to normal
	vim.EnterNormal()
	if !vim.IsNormal() {
		t.Error("Expected to be in normal mode")
	}
	if vim.IsInsert() {
		t.Error("Should not be in insert mode")
	}

	// Test transition to insert
	vim.EnterInsert()
	if !vim.IsInsert() {
		t.Error("Expected to be in insert mode")
	}

	// Test transition to visual
	vim.EnterVisual()
	if !vim.IsVisual() {
		t.Error("Expected to be in visual mode")
	}
}

func TestMarkdownRenderer(t *testing.T) {
	renderer, err := NewMarkdownRenderer(80)
	if err != nil {
		t.Fatalf("Failed to create renderer: %v", err)
	}

	// Test rendering simple text
	result, err := renderer.Render("Hello **world**")
	if err != nil {
		t.Errorf("Failed to render: %v", err)
	}
	if result == "" {
		t.Error("Expected non-empty result")
	}

	// Test empty string
	result, err = renderer.Render("")
	if err != nil {
		t.Errorf("Failed to render empty string: %v", err)
	}
	if result != "" {
		t.Error("Expected empty result for empty input")
	}

	// Test resize
	err = renderer.Resize(120)
	if err != nil {
		t.Errorf("Failed to resize: %v", err)
	}

	// Test resize to same width (should be no-op)
	err = renderer.Resize(120)
	if err != nil {
		t.Errorf("Failed to resize to same width: %v", err)
	}
}
