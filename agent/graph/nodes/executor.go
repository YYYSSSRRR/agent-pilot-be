package nodes

import (
	"context"
	"fmt"
	"sync"

	"github.com/agent-pilot/agent-pilot-be/agent/event"
	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	"github.com/agent-pilot/agent-pilot-be/agent/react"
	"github.com/agent-pilot/agent-pilot-be/agent/tool"
	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/cloudwego/eino/compose"
)

type ctxKey string

const ctxKeySessionID ctxKey = "session_id"

type ExecutorNode struct {
	memory       memory.MemoryService
	executor     *react.Executor
	interruptChs sync.Map // map[sessionID]chan struct{}
}

func (n *ExecutorNode) SignalInterrupt(sessionID string) {
	v, _ := n.interruptChs.LoadOrStore(sessionID, make(chan struct{}, 1))
	ch := v.(chan struct{})
	select {
	case ch <- struct{}{}:
	default:
	}
}

type ExecutorPauseInfo struct {
	Summary       string `json:"summary"`
	PlanID        string `json:"plan_id"`
	StepID        string `json:"step_id"`
	InterruptKind string `json:"interrupt_kind"`
}

func NewExecutorNode(memory memory.MemoryService, executor *react.Executor) Node {
	return &ExecutorNode{
		memory:   memory,
		executor: executor,
	}
}

func (n *ExecutorNode) Invoke(ctx context.Context, state *State) (*State, error) {
	// eino resume from checkpoint calls lambda with nil state
	if state == nil {
		wasInterrupted, hasState, sid := compose.GetInterruptState[string](ctx)
		if !wasInterrupted {
			return nil, fmt.Errorf("executor: state nil and not in resume flow")
		}
		if !hasState || sid == "" {
			return nil, fmt.Errorf("executor: no session id in interrupt state")
		}
		rt, ok, err := n.memory.GetRuntime(ctx, sid)
		if err != nil {
			return nil, fmt.Errorf("executor: get runtime on resume: %w", err)
		}
		if !ok || rt == nil {
			return nil, fmt.Errorf("executor: runtime not found for session %s", sid)
		}
		state = &State{
			Runtime: rt,
			Request: &atype.Request{SessionID: sid},
		}
	}

	rt := state.Runtime
	if rt == nil {
		return nil, fmt.Errorf("runtime nil")
	}

	sessionID := state.Request.SessionID

	// Create per-session interrupt channel, shared with SignalInterrupt
	v, _ := n.interruptChs.LoadOrStore(sessionID, make(chan struct{}, 1))
	interruptCh := v.(chan struct{})
	defer func() {
		n.interruptChs.Delete(sessionID)
		close(interruptCh)
	}()

	// 处理用户中断请求
	if rt.InterruptRequested {
		rt.InterruptRequested = false
		if err := n.memory.SaveRuntime(ctx, rt); err != nil {
			return nil, err
		}
		state.Runtime = rt
		state.Result = &atype.Result{
			Plan:    state.Plan,
			Summary: "execution interrupted by user",
		}

		if bus := event.FromBus(ctx); bus != nil {
			bus.Publish(state.Request.SessionID, event.EventStepPaused, map[string]any{
				"kind": "user_interrupt",
			})
		}

		return state, compose.Interrupt(ctx, &ExecutorPauseInfo{
			Summary:       "execution interrupted by user",
			PlanID:        rt.PlanID,
			StepID:        rt.StepID,
			InterruptKind: "user_interrupt",
		}, sessionID)
	}

	// plan 审批中断：等待用户审批
	if rt.Status == atype.RuntimePendingPlanApproval {
		state.Result = &atype.Result{
			Plan:    state.Plan,
			Summary: "plan pending approval",
		}

		if bus := event.FromBus(ctx); bus != nil {
			bus.Publish(state.Request.SessionID, event.EventStepPaused, map[string]any{
				"kind":    "plan_approval",
				"plan_id": rt.PlanID,
				"step_id": rt.StepID,
			})
		}

		return state, compose.Interrupt(ctx, &ExecutorPauseInfo{
			Summary:       "plan pending approval",
			PlanID:        rt.PlanID,
			StepID:        rt.StepID,
			InterruptKind: "plan_approval",
		}, sessionID)
	}

	// 工具审批恢复：将已批准的 pending tool 注入 context，供 ExecuteStep 直接执行
	if rt.Status == atype.RuntimeApproved && rt.PendingToolApproved && rt.PendingToolCall != "" {
		ctx = tool.WithApprovedCommand(ctx, rt.PendingToolCall)
		//批准后把pending状态恢复，否则会重复执行工具
		rt.PendingToolCall = ""
		rt.PendingToolApproved = false
	}

	exeCtx, err := n.memory.GetExecutionContext(ctx, state.Request.SessionID)
	if err != nil {
		return nil, err
	}

	p := exeCtx.Plan
	step := exeCtx.Step
	if p == nil {
		return nil, fmt.Errorf("plan nil from runtime")
	}
	if step == nil {
		return nil, fmt.Errorf("step nil from runtime")
	}

	bus := event.FromBus(ctx)

	// 发射 step 开始事件
	if bus != nil {
		bus.Publish(state.Request.SessionID, event.EventStepStarted, map[string]any{
			"plan_id":     p.ID,
			"step_id":     step.ID,
			"title":       step.Title,
			"description": step.Description,
		})
	}

	stepResult, err := n.executor.ExecuteStep(ctx, exeCtx, interruptCh)
	if err != nil {
		return nil, err
	}

	if len(stepResult.Messages) > 0 {
		if err := n.memory.AppendMessage(ctx, stepResult.Messages); err != nil {
			return nil, err
		}
	}

	if stepResult.Paused {
		rt.Status = atype.RuntimeInterrupted
		rt.StepID = step.ID
		rt.PlanID = p.ID
		rt.InterruptKind = stepResult.PausedKind
		if stepResult.PausedKind == "tool_approval" {
			rt.PendingToolCall = stepResult.PendingToolCall
		}
		state.Runtime = rt
		state.Result = &atype.Result{
			Plan:    p,
			Steps:   []atype.StepResult{*stepResult},
			Summary: stepResult.Output,
		}

		if bus != nil {
			bus.Publish(state.Request.SessionID, event.EventStepPaused, map[string]any{
				"plan_id": p.ID, "step_id": step.ID,
				"kind": stepResult.PausedKind, "output": stepResult.Output,
			})
		}

		return state, compose.Interrupt(ctx, &ExecutorPauseInfo{
			Summary:       stepResult.Output,
			PlanID:        p.ID,
			StepID:        step.ID,
			InterruptKind: stepResult.PausedKind,
		}, sessionID)
	}

	if stepResult.Completed {
		if bus != nil {
			bus.Publish(state.Request.SessionID, event.EventStepCompleted, map[string]any{
				"plan_id": p.ID, "step_id": step.ID, "output": stepResult.Output,
			})
		}

		next := n.findNextStep(p, step)
		if next == nil {
			rt.Status = atype.RuntimeCompleted
			rt.PlanID = ""
			rt.StepID = ""
			rt.CheckpointID = "" // 清除 checkpoint
			_ = n.memory.SaveRuntime(ctx, rt)
			state.Runtime = rt
			state.Result = &atype.Result{
				Plan:    p,
				Steps:   []atype.StepResult{*stepResult},
				Summary: stepResult.Output,
			}

			if bus != nil {
				bus.Publish(state.Request.SessionID, event.EventExecutionDone, nil)
			}
			return state, nil
		}

		rt.StepID = next.ID
		rt.Status = atype.RuntimeRunning
		_ = n.memory.SaveRuntime(ctx, rt)
		state.Runtime = rt
		state.Result = &atype.Result{
			Plan:    p,
			Steps:   []atype.StepResult{*stepResult},
			Summary: stepResult.Output,
		}
		return state, nil
	}

	return nil, fmt.Errorf("invalid step result")
}

func (n *ExecutorNode) findNextStep(plan *atype.Plan, currentStep *atype.Step) *atype.Step {
	for i := range plan.Steps {
		if plan.Steps[i].ID == currentStep.ID {
			if i >= len(plan.Steps)-1 {
				return nil
			}
			return &plan.Steps[i+1]
		}
	}
	return nil
}
