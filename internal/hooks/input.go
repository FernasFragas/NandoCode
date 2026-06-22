package hooks

import "time"

type ToolInput struct {
	Name          string `json:"name,omitempty"`
	Target        string `json:"target,omitempty"`
	InputSummary  string `json:"input_summary,omitempty"`
	ResultSummary string `json:"result_summary,omitempty"`
	Error         string `json:"error,omitempty"`
}

type Envelope struct {
	Event               Event             `json:"event"`
	Timestamp           time.Time         `json:"timestamp"`
	SessionID           string            `json:"session_id,omitempty"`
	WorkingDir          string            `json:"working_dir,omitempty"`
	Tool                *ToolInput        `json:"tool,omitempty"`
	ConversationSummary string            `json:"conversation_summary,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}
