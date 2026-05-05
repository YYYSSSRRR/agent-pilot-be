package nodes

import (
	"context"

	"github.com/agent-pilot/agent-pilot-be/agent/scheduler"
	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
)

type SchedulerNode struct {
}

func NewSchedulerNode() Node {
	return &SchedulerNode{}
}

func (n *SchedulerNode) Invoke(ctx context.Context, state *State) (*State, error) {
	if state == nil {
		state = &State{}
	}

	state.Decision = &scheduler.Decision{}

	rt := state.Runtime

	// 恢复后的决策
	if rt != nil && rt.Status == atype.RuntimeApproved {
		switch rt.InterruptKind {
		case "tool_approval":
			if !rt.PendingToolApproved {
				// 用户拒绝了工具调用 → 重新规划
				state.Decision.Action = scheduler.ActionPlan
				return state, nil
			}
			state.Decision.Action = scheduler.ActionExecute
			return state, nil
		case "plan_approval":
			if rt.PlanAction == "rejected" {
				state.Decision.Action = scheduler.ActionPlan
				return state, nil
			}
			state.Decision.Action = scheduler.ActionExecute
			return state, nil
		}
	}

	//  正常流程
	if state.Runtime == nil || state.Runtime.PlanID == "" {
		state.Decision.Action = scheduler.ActionPlan
	} else {
		state.Decision.Action = scheduler.ActionExecute
	}
	return state, nil
}
