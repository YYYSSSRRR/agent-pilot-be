package nodes

import (
	"context"

	actx "github.com/agent-pilot/agent-pilot-be/agent/context"
	"github.com/agent-pilot/agent-pilot-be/agent/event"
	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	"github.com/agent-pilot/agent-pilot-be/agent/plan"
	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
)

type PlannerNode struct {
	memory  memory.MemoryService
	planner plan.Planner
	ctxb    *actx.Builder
}

func NewPlannerNode(memory memory.MemoryService, planner plan.Planner, ctxb *actx.Builder) Node {
	return &PlannerNode{
		memory:  memory,
		planner: planner,
		ctxb:    ctxb,
	}
}

func (n *PlannerNode) Invoke(ctx context.Context, state *State) (*State, error) {
	plans, err := n.memory.ListPlansBySession(ctx, state.Request.SessionID, 5)
	if err != nil {
		return nil, err
	}

	ctxMsgs, err := n.ctxb.BuildPlanContext(*state.Request, plans)
	if err != nil {
		return nil, err
	}

	req := state.Request
	req.History = ctxMsgs

	newPlan, err := n.planner.Plan(ctx, *req)
	if err != nil {
		return nil, err
	}

	if err := n.memory.SavePlan(ctx, newPlan); err != nil {
		return nil, err
	}

	rt := &atype.Runtime{
		SessionID: state.Request.SessionID,
		PlanID:    newPlan.ID,
		Status:    atype.RuntimePendingPlanApproval,
	}
	if len(newPlan.Steps) > 0 {
		rt.StepID = newPlan.Steps[0].ID
	}
	if err := n.memory.SaveRuntime(ctx, rt); err != nil {
		return nil, err
	}

	state.Plan = newPlan
	state.Runtime = rt
	state.Result = &atype.Result{
		Plan:    newPlan,
		Summary: "plan created",
	}

	if bus := event.FromBus(ctx); bus != nil {
		bus.Publish(state.Request.SessionID, event.EventPlanCreated, map[string]any{
			"plan_id": newPlan.ID,
			"goal":    newPlan.Goal,
			"steps":   len(newPlan.Steps),
		})
	}

	return state, nil
}
