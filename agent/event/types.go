package event

type EventType string

const (
	EventPlanCreated   EventType = "plan_created"
	EventStepStarted   EventType = "step_started"
	EventStepCompleted EventType = "step_completed"
	EventStepPaused    EventType = "step_paused"
	EventExecutionDone EventType = "execution_done"
	EventMessage       EventType = "message"
	EventToolCall      EventType = "tool_call"
	EventToolResult    EventType = "tool_result"
)

type Event struct {
	SessionID string
	Type      EventType
	Data      any
}
