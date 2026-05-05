package graph

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/agent-pilot/agent-pilot-be/agent/graph/nodes"
	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	"github.com/agent-pilot/agent-pilot-be/agent/scheduler"
	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ctxKey string

const ctxKeySessionID ctxKey = "session_id"

type AgentGraph struct {
	schedulerNode  nodes.Node
	plannerNode    nodes.Node
	executorNode   nodes.Node
	memory         memory.MemoryService
	interruptFuncs sync.Map // map[sessionID]func(opts ...compose.GraphInterruptOption)
}

func NewAgentGraph(
	schedulerNode nodes.Node,
	plannerNode nodes.Node,
	executorNode nodes.Node,
	memory memory.MemoryService,
) *AgentGraph {
	return &AgentGraph{
		schedulerNode: schedulerNode,
		plannerNode:   plannerNode,
		executorNode:  executorNode,
		memory:        memory,
	}
}

func (ag *AgentGraph) InterruptExecution(sessionID string) {
	if v, ok := ag.interruptFuncs.Load(sessionID); ok {
		if interrupt, ok := v.(func(opts ...compose.GraphInterruptOption)); ok {
			interrupt()
		}
	}
}

func (ag *AgentGraph) BuildGraph(opts ...compose.GraphCompileOption) (compose.Runnable[*nodes.State, *atype.Result], error) {
	g := compose.NewGraph[*nodes.State, *atype.Result]()

	if err := g.AddLambdaNode("scheduler",
		compose.InvokableLambda(func(ctx context.Context, state *nodes.State) (*nodes.State, error) {
			return ag.schedulerNode.Invoke(ctx, state)
		}),
		compose.WithNodeName("Scheduler"),
	); err != nil {
		return nil, err
	}

	if err := g.AddLambdaNode("planner",
		compose.InvokableLambda(func(ctx context.Context, state *nodes.State) (*nodes.State, error) {
			return ag.plannerNode.Invoke(ctx, state)
		}),
		compose.WithNodeName("Planner"),
	); err != nil {
		return nil, err
	}

	if err := g.AddLambdaNode("executor",
		compose.InvokableLambda(func(ctx context.Context, state *nodes.State) (*nodes.State, error) {
			return ag.executorNode.Invoke(ctx, state)
		}),
		compose.WithNodeName("Executor"),
	); err != nil {
		return nil, err
	}

	if err := g.AddLambdaNode("finisher",
		compose.InvokableLambda(func(ctx context.Context, state *nodes.State) (*atype.Result, error) {
			if state == nil || state.Result == nil {
				return &atype.Result{}, nil
			}
			return state.Result, nil
		}),
		compose.WithNodeName("Finisher"),
	); err != nil {
		return nil, err
	}

	if err := g.AddEdge(compose.START, "scheduler"); err != nil {
		return nil, err
	}

	if err := g.AddBranch("scheduler",
		compose.NewGraphBranch(
			func(ctx context.Context, state *nodes.State) (string, error) {
				if state == nil || state.Decision == nil {
					return "finisher", nil
				}
				switch state.Decision.Action {
				case scheduler.ActionPlan:
					return "planner", nil
				case scheduler.ActionExecute:
					return "executor", nil
				case scheduler.ActionResume:
					return "executor", nil
				default:
					return "finisher", nil
				}
			},
			map[string]bool{"planner": true, "executor": true, "finisher": true},
		),
	); err != nil {
		return nil, err
	}

	if err := g.AddBranch("executor",
		compose.NewGraphBranch(
			func(ctx context.Context, state *nodes.State) (string, error) {
				if state == nil || state.Decision == nil {
					return "finisher", nil
				}
				switch state.Decision.Action {
				case scheduler.ActionExecute:
					return "executor", nil
				case scheduler.ActionPause:
					return "finisher", nil
				case scheduler.ActionComplete:
					return "finisher", nil
				default:
					return "finisher", nil
				}
			},
			map[string]bool{"executor": true, "finisher": true},
		),
	); err != nil {
		return nil, err
	}

	if err := g.AddEdge("planner", "executor"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("finisher", compose.END); err != nil {
		return nil, err
	}

	if ag.memory != nil {
		if store := ag.memory.GraphCheckPointStore(); store != nil {
			opts = append(opts, compose.WithCheckPointStore(store))
		}
	}

	runnable, err := g.Compile(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("compile graph failed: %w", err)
	}
	return &graphRunnable{inner: runnable, memory: ag.memory, graph: ag}, nil
}

type graphRunnable struct {
	inner  compose.Runnable[*nodes.State, *atype.Result]
	memory memory.MemoryService
	graph  *AgentGraph
}

func (r *graphRunnable) Invoke(ctx context.Context, input *nodes.State, opts ...compose.Option) (*atype.Result, error) {
	opts = append(opts, withDefaultCheckPointID(input)...)

	sessionID := ""
	if input != nil && input.Request != nil {
		sessionID = strings.TrimSpace(input.Request.SessionID)
	}

	state := input
	if state == nil {
		state = &nodes.State{}
	}

	// 从memory中恢复runtime
	if r.memory != nil && sessionID != "" {
		rt, ok, err := r.memory.GetRuntime(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if ok && rt != nil {
			state.Runtime = rt
			if state.Request == nil {
				state.Request = &atype.Request{
					SessionID: sessionID,
				}
			}
		}
	}

	// 创建可中断的运行上下文
	runCtx, interrupt := compose.WithGraphInterrupt(ctx)
	if sessionID != "" {
		r.graph.interruptFuncs.Store(sessionID, interrupt)
		defer r.graph.interruptFuncs.Delete(sessionID)
	}

	// 决定走resume还是scheduler
	if state.Runtime != nil {
		rt := state.Runtime
		if rt.Status == atype.RuntimeApproved || rt.Status == atype.RuntimeCompleted {
			opts = append(opts, compose.WithForceNewRun())
		} else if rt.Status != atype.RuntimeCompleted && strings.TrimSpace(rt.CheckpointID) != "" {
			runCtx = compose.ResumeWithData(runCtx, rt.CheckpointID, sessionID)
		}
	}

	out, err := r.inner.Invoke(runCtx, state, opts...)
	if err == nil {
		return out, nil
	}

	info, ok := compose.ExtractInterruptInfo(err)
	if !ok {
		return nil, err
	}

	pause, interruptID := extractPauseInfo(info)
	rt := &atype.Runtime{SessionID: sessionID, Status: atype.RuntimeInterrupted}
	if pause != nil {
		rt.PlanID = pause.PlanID
		rt.StepID = pause.StepID
		rt.InterruptKind = pause.InterruptKind
	}
	if interruptID != "" {
		rt.CheckpointID = interruptID
	}
	if r.memory != nil && sessionID != "" {
		if saveErr := r.memory.SaveRuntime(ctx, rt); saveErr != nil {
			return nil, saveErr
		}
	}

	result := &atype.Result{}
	if state != nil {
		result.Plan = state.Plan
	}
	if pause != nil {
		result.Summary = pause.Summary
	}
	return result, nil
}

func (r *graphRunnable) Stream(ctx context.Context, input *nodes.State, opts ...compose.Option) (*schema.StreamReader[*atype.Result], error) {
	opts = append(opts, withDefaultCheckPointID(input)...)
	return r.inner.Stream(ctx, input, opts...)
}

func (r *graphRunnable) Collect(ctx context.Context, input *schema.StreamReader[*nodes.State], opts ...compose.Option) (*atype.Result, error) {
	return r.inner.Collect(ctx, input, opts...)
}

func (r *graphRunnable) Transform(ctx context.Context, input *schema.StreamReader[*nodes.State], opts ...compose.Option) (*schema.StreamReader[*atype.Result], error) {
	return r.inner.Transform(ctx, input, opts...)
}

func withDefaultCheckPointID(input *nodes.State) []compose.Option {
	if input == nil || input.Request == nil || strings.TrimSpace(input.Request.SessionID) == "" {
		return nil
	}
	return []compose.Option{compose.WithCheckPointID(strings.TrimSpace(input.Request.SessionID))}
}

func extractPauseInfo(info *compose.InterruptInfo) (*nodes.ExecutorPauseInfo, string) {
	if info == nil || len(info.InterruptContexts) == 0 {
		return nil, ""
	}
	ictx := info.InterruptContexts[0]
	if ictx == nil {
		return nil, ""
	}
	if pause, ok := ictx.Info.(*nodes.ExecutorPauseInfo); ok {
		return pause, ictx.ID
	}
	if raw, ok := ictx.Info.(map[string]any); ok {
		return &nodes.ExecutorPauseInfo{
			Summary:       stringFromAny(raw["summary"]),
			PlanID:        stringFromAny(raw["plan_id"]),
			StepID:        stringFromAny(raw["step_id"]),
			InterruptKind: stringFromAny(raw["interrupt_kind"]),
		}, ictx.ID
	}
	return nil, ictx.ID
}

func stringFromAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
