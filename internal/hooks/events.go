package hooks

// Event is a lifecycle event name.
type Event string

const (
	EventSessionStart     Event = "SessionStart"
	EventSessionEnd       Event = "SessionEnd"
	EventUserPromptSubmit Event = "UserPromptSubmit"
	EventUserMessageRecv  Event = "UserMessageReceived"
	EventPreToolUse       Event = "PreToolUse"
	EventPostToolUse      Event = "PostToolUse"
	EventPostToolUseFail  Event = "PostToolUseFailure"
	EventPermissionDenied Event = "PermissionDenied"
	EventStop             Event = "Stop"
	EventSetup            Event = "Setup"
	EventSubagentStart    Event = "SubagentStart"
	EventSubagentStop     Event = "SubagentStop"
	EventPreCompact       Event = "PreCompact"
	EventPostCompact      Event = "PostCompact"
	EventMemoryRead       Event = "MemoryRead"
	EventMemoryWrite      Event = "MemoryWrite"
	EventNotification     Event = "Notification"
	EventConfigChange     Event = "ConfigChange"
	EventInstructionsLoad Event = "InstructionsLoaded"
	EventCwdChanged       Event = "CwdChanged"
	EventFileChanged      Event = "FileChanged"
	EventTaskCreated      Event = "TaskCreated"
	EventTaskCompleted    Event = "TaskCompleted"
)

func (e Event) Valid() bool {
	switch e {
	case EventSessionStart, EventSessionEnd, EventUserPromptSubmit, EventUserMessageRecv,
		EventPreToolUse, EventPostToolUse, EventPostToolUseFail, EventPermissionDenied,
		EventStop, EventSetup, EventSubagentStart, EventSubagentStop, EventPreCompact,
		EventPostCompact, EventMemoryRead, EventMemoryWrite, EventNotification,
		EventConfigChange, EventInstructionsLoad, EventCwdChanged, EventFileChanged,
		EventTaskCreated, EventTaskCompleted:
		return true
	default:
		return false
	}
}

func (e Event) Blocks() bool {
	switch e {
	case EventPreToolUse, EventUserPromptSubmit, EventStop:
		return true
	default:
		return false
	}
}
