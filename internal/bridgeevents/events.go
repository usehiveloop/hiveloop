package bridgeevents

const (
	EventAgentError              = "agent_error"
	EventConversationCreated     = "conversation_created"
	EventConversationEnded       = "conversation_ended"
	EventDone                    = "done"
	EventMessageReceived         = "message_received"
	EventReasoningDelta          = "reasoning_delta"
	EventResponseChunk           = "response_chunk"
	EventResponseCompleted       = "response_completed"
	EventResponseStarted         = "response_started"
	EventTodoUpdated             = "todo_updated"
	EventToolApprovalRequired    = "tool_approval_required"
	EventToolApprovalResolved    = "tool_approval_resolved"
	EventToolCallCompleted       = "tool_call_completed"
	EventToolCallStarted         = "tool_call_started"
	EventTurnCompleted           = "turn_completed"
	EventBackgroundTaskCompleted = "background_task_completed"
	EventReasoningStarted        = "reasoning_started"
	EventReasoningCompleted      = "reasoning_completed"
	EventSubAgentStarted         = "sub_agent_started"
	EventSubAgentCompleted       = "sub_agent_completed"
)

var legacyEventAliases = map[string]string{
	"AgentError":              EventAgentError,
	"ConversationCreated":     EventConversationCreated,
	"ConversationEnded":       EventConversationEnded,
	"Done":                    EventDone,
	"MessageReceived":         EventMessageReceived,
	"ReasoningDelta":          EventReasoningDelta,
	"ResponseChunk":           EventResponseChunk,
	"ResponseCompleted":       EventResponseCompleted,
	"ResponseStarted":         EventResponseStarted,
	"TodoUpdated":             EventTodoUpdated,
	"ToolApprovalRequired":    EventToolApprovalRequired,
	"ToolApprovalResolved":    EventToolApprovalResolved,
	"ToolCallCompleted":       EventToolCallCompleted,
	"ToolCallResult":          EventToolCallCompleted,
	"ToolCallStarted":         EventToolCallStarted,
	"ToolCallStart":           EventToolCallStarted,
	"TurnCompleted":           EventTurnCompleted,
	"BackgroundTaskCompleted": EventBackgroundTaskCompleted,
	"ReasoningStarted":        EventReasoningStarted,
	"ReasoningCompleted":      EventReasoningCompleted,
	"SubAgentStarted":         EventSubAgentStarted,
	"SubAgentCompleted":       EventSubAgentCompleted,
	"tool_call_result":        EventToolCallCompleted,
	"tool_call_start":         EventToolCallStarted,
}

func NormalizeEventType(eventType string) string {
	if canonical, ok := legacyEventAliases[eventType]; ok {
		return canonical
	}
	return eventType
}

func IsTerminalEventType(eventType string) bool {
	switch NormalizeEventType(eventType) {
	case EventAgentError, EventConversationEnded, EventDone, EventTurnCompleted:
		return true
	default:
		return false
	}
}
