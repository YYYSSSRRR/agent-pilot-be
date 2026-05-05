package react

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	context2 "github.com/agent-pilot/agent-pilot-be/agent/context"
	"github.com/agent-pilot/agent-pilot-be/agent/event"
	"github.com/agent-pilot/agent-pilot-be/agent/tool"
	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	einomodel "github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const defaultMaxTurns = 8

type Executor struct {
	model          einomodel.ToolCallingChatModel
	tools          []einotool.BaseTool
	maxTurns       int
	now            func() time.Time
	contextBuilder *context2.Builder
}

func NewExecutor(
	model einomodel.ToolCallingChatModel,
	tools []einotool.BaseTool,
	contextBuilder *context2.Builder,
) *Executor {
	return &Executor{
		model:          model,
		tools:          tools,
		maxTurns:       defaultMaxTurns,
		now:            time.Now,
		contextBuilder: contextBuilder,
	}
}

func (e *Executor) ExecuteStep(ctx context.Context, exeCtx *atype.ExecutionContext, interruptCh <-chan struct{}) (*atype.StepResult, error) {
	var executionMessages []*atype.Message
	toolInfos, invokables, err := e.prepareTools(ctx)
	if err != nil {
		return nil, err
	}

	modelWithTools, err := e.model.WithTools(toolInfos)
	if err != nil {
		return nil, err
	}

	messages, err := e.contextBuilder.BuildExecutionContext(exeCtx)
	if err != nil {
		return nil, err
	}

	messages = append([]*schema.Message{
		schema.SystemMessage(e.systemPrompt()),
	}, messages...)

	bus := event.FromBus(ctx)

	//如果已经有审批完成的工具，直接执行，否则再进入llm会死循环
	if approvedCmd, ok := tool.GetApprovedCommand(ctx); ok && approvedCmd != "" {
		if shellTool, ok := invokables["shell"]; ok {
			args := fmt.Sprintf(`{"cmd":"%s"}`, strings.ReplaceAll(approvedCmd, `"`, `\"`))
			result, err := shellTool.InvokableRun(ctx, args)
			if err != nil {
				result = "tool execution error: " + err.Error()
			}

			messages = append(messages, schema.ToolMessage(result, "approved_cmd", schema.WithToolName("shell")))
			executionMessages = append(executionMessages, &atype.Message{
				SessionID: exeCtx.SessionID,
				PlanID:    exeCtx.Plan.ID,
				StepID:    exeCtx.Step.ID,
				Role:      atype.RoleToolResult,
				Content:   result,
				Metadata: map[string]any{
					"tool_name": "shell",
					"call_id":   "approved_cmd",
				},
				CreatedAt: time.Now(),
			})

			if bus != nil {
				bus.Publish(exeCtx.SessionID, event.EventToolResult, map[string]any{
					"plan_id": exeCtx.Plan.ID, "step_id": exeCtx.Step.ID,
					"tool_name": "shell", "result": result,
				})
			}
		}
	}

	for turn := 0; turn < e.maxTurns; turn++ {
		select {
		case <-interruptCh:
			return &atype.StepResult{
				StepID:     exeCtx.Step.ID,
				Paused:     true,
				Completed:  false,
				Output:     "execution interrupted by user",
				Messages:   executionMessages,
				PausedKind: "user_interrupt",
			}, nil
		default:
		}

		msg, err := modelWithTools.Generate(ctx, messages)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			return nil, fmt.Errorf("model returned nil message")
		}

		if msg.Content != "" {
			executionMessages = append(executionMessages, &atype.Message{
				SessionID: exeCtx.Plan.SessionID,
				PlanID:    exeCtx.Plan.ID,
				StepID:    exeCtx.Step.ID,
				Role:      atype.RoleAssistant,
				Content:   msg.Content,
				CreatedAt: time.Now(),
			})
			messages = append(messages, msg)

			if bus != nil {
				bus.Publish(exeCtx.Plan.SessionID, event.EventMessage, map[string]any{
					"plan_id": exeCtx.Plan.ID,
					"step_id": exeCtx.Step.ID,
					"content": msg.Content,
					"role":    "assistant",
					"type":    "plan",
				})
			}
		}

		if len(msg.ToolCalls) == 0 {
			return nil, fmt.Errorf("model returned no tool call; expected complete_step or request_human_input")
		}

		for _, call := range msg.ToolCalls {
			toolName := call.Function.Name
			executionMessages = append(executionMessages, &atype.Message{
				SessionID: exeCtx.Plan.SessionID,
				PlanID:    exeCtx.Plan.ID,
				StepID:    exeCtx.Step.ID,
				Role:      atype.RoleToolCall,
				Content:   fmt.Sprintf("tool_name:%s,arguments:%s", call.Function.Name, call.Function.Arguments),
				Metadata: map[string]any{
					"tool_name": call.Function.Name,
					"call_id":   call.ID,
					"arguments": call.Function.Arguments,
				},
				CreatedAt: time.Now(),
			})

			// 发射工具调用事件
			if bus != nil {
				bus.Publish(exeCtx.Plan.SessionID, event.EventToolCall, map[string]any{
					"plan_id": exeCtx.Plan.ID, "step_id": exeCtx.Step.ID,
					"tool_name": toolName, "arguments": call.Function.Arguments,
				})
			}

			switch call.Function.Name {

			case "request_human_input":
				question, kind, err := e.extractHumanInput(call.Function.Arguments)
				if err != nil {
					return nil, fmt.Errorf("invalid request_human_input args: %w", err)
				}
				executionMessages = append(executionMessages, &atype.Message{
					SessionID: exeCtx.Plan.SessionID,
					PlanID:    exeCtx.Plan.ID,
					StepID:    exeCtx.Step.ID,
					Role:      atype.RoleToolResult,
					Content:   question,
					Metadata: map[string]any{
						"tool_name": call.Function.Name,
						"call_id":   call.ID,
					},
					CreatedAt: time.Now(),
				})

				if bus != nil {
					bus.Publish(exeCtx.Plan.SessionID, event.EventStepPaused, map[string]any{
						"plan_id": exeCtx.Plan.ID, "step_id": exeCtx.Step.ID,
						"kind": kind, "question": question,
					})
				}

				return &atype.StepResult{
					StepID:     exeCtx.Step.ID,
					Paused:     true,
					Completed:  false,
					Output:     question,
					Messages:   executionMessages,
					PausedKind: kind,
				}, nil

			case "complete_step":
				summary, err := e.extractCompleteResult(call.Function.Arguments)
				if err != nil {
					return nil, err
				}

				executionMessages = append(executionMessages, &atype.Message{
					SessionID: exeCtx.Plan.SessionID,
					PlanID:    exeCtx.Plan.ID,
					StepID:    exeCtx.Step.ID,
					Role:      atype.RoleToolResult,
					Content:   summary,
					Metadata: map[string]any{
						"tool_name": toolName,
						"call_id":   call.ID,
					},
					CreatedAt: time.Now(),
				})

				return &atype.StepResult{
					StepID:    exeCtx.Step.ID,
					Output:    summary,
					Paused:    false,
					Completed: true,
					Messages:  executionMessages,
				}, nil

			default:
				t, ok := invokables[toolName]
				if !ok {
					messages = append(messages, schema.ToolMessage("unknown tool: "+toolName, call.ID, schema.WithToolName(toolName)))
					continue
				}

				result, err := t.InvokableRun(ctx, call.Function.Arguments)
				if err != nil {
					result = "tool execution error: " + err.Error()
				}

				// 发射工具结果事件
				if bus != nil {
					bus.Publish(exeCtx.Plan.SessionID, event.EventToolResult, map[string]any{
						"plan_id": exeCtx.Plan.ID, "step_id": exeCtx.Step.ID,
						"tool_name": toolName, "result": result,
					})
				}

				if strings.HasPrefix(result, "SHELL_NEEDS_APPROVAL:") {
					cmd := strings.TrimPrefix(result, "SHELL_NEEDS_APPROVAL:")
					return &atype.StepResult{
						StepID:          exeCtx.Step.ID,
						Paused:          true,
						PausedKind:      "tool_approval",
						Output:          "Shell command requires approval: " + cmd,
						Messages:        executionMessages,
						PendingToolCall: cmd,
					}, nil
				}

				messages = append(messages, schema.ToolMessage(result, call.ID, schema.WithToolName(toolName)))
				executionMessages = append(executionMessages, &atype.Message{
					SessionID: exeCtx.Plan.SessionID,
					PlanID:    exeCtx.Plan.ID,
					StepID:    exeCtx.Step.ID,
					Role:      atype.RoleToolResult,
					Content:   result,
					Metadata: map[string]any{
						"tool_name": call.Function.Name,
						"call_id":   call.ID,
					},
				})
			}
		}
	}
	return nil, fmt.Errorf("react executor exceeded max turns for step %s", exeCtx.Step.ID)
}

func (e *Executor) prepareTools(ctx context.Context) ([]*schema.ToolInfo, map[string]einotool.InvokableTool, error) {
	infos := make([]*schema.ToolInfo, 0, len(e.tools))
	invokables := make(map[string]einotool.InvokableTool, len(e.tools))

	for _, baseTool := range e.tools {
		info, err := baseTool.Info(ctx)
		if err != nil {
			return nil, nil, err
		}
		infos = append(infos, info)

		if invokable, ok := baseTool.(einotool.InvokableTool); ok {
			invokables[info.Name] = invokable
		}
	}

	return infos, invokables, nil
}

func (e *Executor) systemPrompt() string {
	return `You are the execution layer of a plan-execute agent.

Core Rules:
- Execute ONLY the current step. Never execute future steps.
- Use ReAct: think silently about the next action before acting.
- Use load_skill before following a skill.
- Use load_skill_references only when the loaded skill requires reference files.
- Use shell for lark-cli commands.
- The Lark user access token is provided by the runtime environment. DO NOT ask for it or print it.
- Prefer --as user for user-owned Lark resources unless bot identity is explicitly required.
- Stop immediately after completing the current step.

Required Function Calls:
- When the current step is finished: you MUST explicitly call complete_step.
- When required information is missing, ambiguous, or needs confirmation:
  you MUST explicitly call request_human_input, then stop execution immediately.
  Examples: missing document ID, unknown target user, ambiguous instruction, confirmation before destructive action.

When requesting human input: ask a concise question and stop execution.
When the step is completed: return a concise result and do not proceed to other steps.
`
}

func (e *Executor) extractHumanInput(args string) (string, string, error) {
	var payload struct {
		Question string `json:"question"`
		Kind     string `json:"kind"`
	}
	err := json.Unmarshal([]byte(args), &payload)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(payload.Question), strings.TrimSpace(payload.Kind), nil
}

func (e *Executor) extractCompleteResult(args string) (string, error) {
	type finishArgs struct {
		Summary string `json:"summary"`
	}
	var req finishArgs
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return "", fmt.Errorf("invalid complete_step args: %w", err)
	}
	req.Summary = strings.TrimSpace(req.Summary)
	if req.Summary == "" {
		return "", fmt.Errorf("complete_step summary is empty")
	}
	return req.Summary, nil
}
