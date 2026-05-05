package scheduler

type Action string

const (
	ActionStart    Action = "start"
	ActionResume   Action = "resume"
	ActionRunning  Action = "running"
	ActionDone     Action = "done"
	ActionPlan     Action = "plan"
	ActionExecute  Action = "execute"
	ActionPause    Action = "pause"
	ActionComplete Action = "complete"
)

type Decision struct {
	Action Action
	Reason string
}
