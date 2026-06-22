package tui

// TranscriptKind identifies the type of transcript entry.
type TranscriptKind string

const (
	TranscriptUser      TranscriptKind = "user"
	TranscriptAssistant TranscriptKind = "assistant"
	TranscriptThinking  TranscriptKind = "thinking"
	TranscriptTool      TranscriptKind = "tool"
	TranscriptSystem    TranscriptKind = "system"
)

// TranscriptItem represents a single entry in the chat transcript.
type TranscriptItem struct {
	Kind      TranscriptKind
	ToolID    string
	ToolName  string
	Content   string
	Collapsed bool
	Error     string
	Rendered  string // cached rendered markdown
	CharCount int    // total chars accumulated for thinking blocks
	Streaming bool   // true while thinking deltas are still arriving
}

// AppendAssistantDelta appends text to the last assistant item or creates one.
func AppendAssistantDelta(items []TranscriptItem, content string) []TranscriptItem {
	if len(items) > 0 && items[len(items)-1].Kind == TranscriptAssistant {
		items[len(items)-1].Content += content
		items[len(items)-1].Rendered = "" // clear cache
		return items
	}

	return append(items, TranscriptItem{
		Kind:    TranscriptAssistant,
		Content: content,
	})
}

// AppendThinkingDelta appends to the active thinking item or creates one.
func AppendThinkingDelta(items []TranscriptItem, content string) []TranscriptItem {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Kind != TranscriptThinking {
			continue
		}
		if items[i].Streaming {
			items[i].Content += content
			items[i].CharCount += len(content)
			items[i].Rendered = ""
			return items
		}
		break
	}

	return append(items, TranscriptItem{
		Kind:      TranscriptThinking,
		Content:   content,
		Collapsed: true,
		CharCount: len(content),
		Streaming: true,
	})
}

// FinalizeThinkingItem marks the most recent active thinking item as complete.
func FinalizeThinkingItem(items []TranscriptItem) []TranscriptItem {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Kind == TranscriptThinking {
			items[i].Streaming = false
			return items
		}
	}
	return items
}

// CreateToolItem creates a new tool transcript item.
func CreateToolItem(toolID, toolName string) TranscriptItem {
	return TranscriptItem{
		Kind:     TranscriptTool,
		ToolID:   toolID,
		ToolName: toolName,
		Content:  "[started]",
	}
}

// CreateSystemItem creates a new system transcript item.
func CreateSystemItem(content string) TranscriptItem {
	return TranscriptItem{
		Kind:    TranscriptSystem,
		Content: content,
	}
}

// CreateUserItem creates a new user transcript item.
func CreateUserItem(content string) TranscriptItem {
	return TranscriptItem{
		Kind:    TranscriptUser,
		Content: content,
	}
}
