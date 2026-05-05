package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/agent-pilot/agent-pilot-be/agent/event"
	"github.com/agent-pilot/agent-pilot-be/agent/graph"
	"github.com/agent-pilot/agent-pilot-be/agent/graph/nodes"
	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	"github.com/agent-pilot/agent-pilot-be/agent/tool"
	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/agent-pilot/agent-pilot-be/pkg/jwt"
	"github.com/agent-pilot/agent-pilot-be/transport"
	"github.com/cloudwego/eino/compose"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"

	"github.com/agent-pilot/agent-pilot-be/agent/tool/skill"
	pkgmodel "github.com/agent-pilot/agent-pilot-be/model"
)

type ControllerInterface interface {
	Chat(ctx *gin.Context)
	ChatSync(ctx *gin.Context)
	CreateSession(ctx *gin.Context)
	Plan(ctx *gin.Context)
	Execute(ctx *gin.Context)
}

type Controller struct {
	SkillReg  *skill.Registry
	SystemMsg string
	Graph     *graph.AgentGraph
	Runnable  compose.Runnable[*nodes.State, *atype.Result]
	Memory    memory.MemoryService
	Bus       *event.Bus
	mu        sync.Mutex
}

func NewController(
	ctx context.Context,
	skillReg *skill.Registry,
	systemMsg string,
	memory memory.MemoryService,
	graph *graph.AgentGraph,
) *Controller {
	runnable, err := graph.BuildGraph()
	if err != nil {
		panic(err)
	}
	return &Controller{
		SkillReg:  skillReg,
		SystemMsg: systemMsg,
		Graph:     graph,
		Runnable:  runnable,
		Memory:    memory,
		Bus:       event.NewBus(),
	}
}

//// Chat 处理流式聊天请求
//// @Summary      流式对话（SSE）
//// @Description  Server-Sent Events：event:message / event:done / event:error
//// @Tags         chat
//// @Accept       json
//// @Produce      text/event-stream
//// @Param body body request true "用户输入"
//// @Success      200   {string}  string      "SSE 流"
//// @Failure      400   {object}  model.Response
//// @Router       /chat/stream [post]
//// @Security     BearerAuth
//func (c *Controller) Chat(ctx *gin.Context) {
//	var req request
//	if err := ctx.ShouldBindJSON(&req); err != nil {
//		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
//			Code:    400,
//			Message: err.Error(),
//			Data:    nil,
//		})
//		return
//	}
//
//	if req.Message == "" {
//		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
//			Code:    400,
//			Message: "Message is required",
//			Data:    nil,
//		})
//		return
//	}
//
//	// 设置 SSE headers
//	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
//	ctx.Writer.Header().Set("Cache-Control", "no-cache")
//	ctx.Writer.Header().Set("Connection", "keep-alive")
//	ctx.Writer.Flush()
//
//	sessionID := "mock-session"
//
//	result, err := c.executeGraph(ctx.Request.Context(), sessionID, req.Message)
//	if err != nil {
//		c.sendEventGin(ctx, "error", fmt.Sprintf("Graph error: %v", err))
//		return
//	}
//
//	if result != nil && result.Summary != "" {
//		c.sendEventGin(ctx, "message", result.Summary)
//	}
//	c.sendEventGin(ctx, "done", "")
//}

// ChatSync 处理非流式聊天请求
// @Summary      非流式对话
// @Description  普通HTTP响应，不使用SSE
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param body body request true "用户输入"
// @Success      200   {object}  pkgmodel.Response "data.message"
// @Failure      400   {object}  pkgmodel.Response
// @Failure      500   {object}  pkgmodel.Response
// @Router       /chat/sync [post]
// @Security     BearerAuth
func (c *Controller) ChatSync(ctx *gin.Context, req request) (pkgmodel.Response, error) {

	if req.Message == "" {
		return pkgmodel.Response{}, errors.New("输入为空")
	}

	request := atype.Request{
		SessionID: req.SessionID,
		UserInput: req.Message,
	}
	ctx2 := event.WithBus(ctx.Request.Context(), c.Bus)
	result, err := c.Runnable.Invoke(ctx2, &nodes.State{Request: &request})
	if err != nil {
		return pkgmodel.Response{}, err
	}

	message := ""
	if result != nil {
		message = result.Summary
	}

	return pkgmodel.Response{
		Code:    200,
		Message: "ok",
		Data: map[string]interface{}{
			"message": message,
			"plan":    result.Plan,
		},
	}, nil

}

//// Plan 生成执行计划
//// @Summary      生成 Plan
//// @Tags         chat
//// @Accept       json
//// @Produce      json
//// @Param        body body request true "用户输入（message 必填）"
//// @Success      200   {object}  model.Response "data.plan / data.checkpoint_id"
//// @Failure      400   {object}  model.Response
//// @Failure      500   {object}  model.Response
//// @Router       /chat/plan [post]
//// @Security     BearerAuth
//func (c *Controller) Plan(ctx *gin.Context) {
//	var req request
//	if err := ctx.ShouldBindJSON(&req); err != nil {
//		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
//			Code:    400,
//			Message: err.Error(),
//			Data:    nil,
//		})
//		return
//	}
//
//	if req.Message == "" {
//		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
//			Code:    400,
//			Message: "Message is required",
//			Data:    nil,
//		})
//		return
//	}
//	if c.Planner == nil {
//		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
//			Code:    500,
//			Message: "planner is not configured",
//			Data:    nil,
//		})
//		return
//	}
//
//	sessionID := "mock"
//	history := c.getHistory(sessionID)
//	p, err := c.Planner.Plan(ctx.Request.Context(), atype.Request{
//		SessionID: sessionID,
//		UserInput: req.Message,
//		History:   history,
//	})
//	if err != nil {
//		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
//			Code:    500,
//			Message: err.Error(),
//			Data:    nil,
//		})
//		return
//	}
//
//	var checkpointID string
//	if c.Checkpointer != nil {
//		cp, err := c.Checkpointer.Save(ctx.Request.Context(), p, "plan_created")
//		if err != nil {
//			ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
//				Code:    500,
//				Message: err.Error(),
//				Data:    nil,
//			})
//			return
//		}
//		checkpointID = cp.ID
//	}
//
//	ctx.JSON(http.StatusOK, pkgmodel.Response{
//		Code:    0,
//		Message: "ok",
//		Data: gin.H{
//			"plan":          p,
//			"checkpoint_id": checkpointID,
//		},
//	})
//}
//
//// Execute 按 Plan 执行（ReAct）
//// @Summary      执行 Plan
//// @Tags         chat
//// @Accept       json
//// @Produce      json
//// @Param        body body request true "checkpoint_id 或 message（二选一逻辑见实现）"
//// @Success      200   {object}  model.Response
//// @Failure      400   {object}  model.Response
//// @Failure      500   {object}  model.Response
//// @Router       /chat/execute [post]
//// @Security     BearerAuth
//func (c *Controller) Execute(ctx *gin.Context) {
//	var req request
//	if err := ctx.ShouldBindJSON(&req); err != nil {
//		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
//			Code:    400,
//			Message: err.Error(),
//			Data:    nil,
//		})
//		return
//	}
//
//	if c.Executor == nil {
//		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
//			Code:    500,
//			Message: "react executor is not configured",
//			Data:    nil,
//		})
//		return
//	}
//
//	//p, err := c.planForExecution(ctx, req)
//	//if err != nil {
//	//	ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
//	//		Code:    400,
//	//		Message: err.Error(),
//	//		Data:    nil,
//	//	})
//	//	return
//	//}
//
//	//result, err := c.Executor.ExecuteStep(ctx.Request.Context(), p)
//	//if err != nil {
//	//	ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
//	//		Code:    500,
//	//		Message: err.Error(),
//	//		Data:    result,
//	//	})
//	//	return
//	//}
//
//	ctx.JSON(http.StatusOK, pkgmodel.Response{
//		Code:    0,
//		Message: "ok",
//		Data:    nil,
//	})
//}
//
//func (c *Controller) planForExecution(ctx *gin.Context, req request) (*atype.Plan, error) {
//	if req.CheckpointID != "" {
//		if c.Checkpointer == nil {
//			return nil, fmt.Errorf("checkpointer is not configured")
//		}
//		cp, err := c.Checkpointer.Load(ctx.Request.Context(), req.CheckpointID)
//		if err != nil {
//			return nil, err
//		}
//		return cp.Plan, nil
//	}
//
//	if strings.TrimSpace(req.Message) == "" {
//		return nil, fmt.Errorf("message or checkpoint_id is required")
//	}
//	if c.Planner == nil {
//		return nil, fmt.Errorf("planner is not configured")
//	}
//
//	sessionID := "mock"
//	return c.Planner.Plan(ctx.Request.Context(), atype.Request{
//		SessionID: sessionID,
//		UserInput: req.Message,
//		History:   c.getHistory(sessionID),
//	})
//}
//
//// streamFromEvents 从事件流中提取内容并发送给客户端
//func (c *Controller) streamFromEvents(
//	ginCtx *gin.Context,
//	events *adk.AsyncIterator[*adk.AgentEvent],
//	sessionID string,
//	history []*schema.Message,
//) {
//	var fullReply strings.Builder
//	for {
//		event, ok := events.Next()
//		if !ok {
//			break
//		}
//		if event.Err != nil {
//			c.sendEventGin(ginCtx, "error", fmt.Sprintf("Stream error: %v", event.Err))
//			return
//		}
//		if event.Output == nil || event.Output.MessageOutput == nil {
//			continue
//		}
//
//		mv := event.Output.MessageOutput
//		//if mv.Role != schema.Assistant {
//		//	continue
//		//}
//
//		// 处理流式内容
//		if mv.IsStreaming {
//			mv.MessageStream.SetAutomaticClose()
//			for {
//				frame, err := mv.MessageStream.Recv()
//				if errors.Is(err, io.EOF) {
//					break
//				}
//				if err != nil {
//					c.sendEventGin(ginCtx, "error", fmt.Sprintf("Stream recv error: %v", err))
//					return
//				}
//
//				if frame != nil && frame.Content != "" {
//					fullReply.WriteString(frame.Content)
//					c.sendEventGin(ginCtx, "message", frame.Content)
//				}
//			}
//			continue
//		}
//
//		// 非流式内容
//		if mv.Message != nil && mv.Message.Content != "" {
//			fullReply.WriteString(mv.Message.Content)
//			c.sendEventGin(ginCtx, "message", mv.Message.Content)
//		}
//	}
//
//	if fullReply.Len() > 0 {
//		history = append(history, schema.AssistantMessage(fullReply.String(), nil))
//	}
//
//	c.saveHistory(sessionID, history)
//	fmt.Printf("%+v", history)
//	// done
//	c.sendEventGin(ginCtx, "done", "")
//}
//
//// sendEventGin 发送 SSE 事件
//func (c *Controller) sendEventGin(ctx *gin.Context, event, data string) {
//	fmt.Fprintf(ctx.Writer, "event: %s\n", event)
//	// 如果 data 包含换行符，拆分为多行
//	lines := strings.Split(data, "\n")
//	for _, line := range lines {
//		fmt.Fprintf(ctx.Writer, "data: %s\n", line)
//	}
//	fmt.Fprintf(ctx.Writer, "\n")
//	ctx.Writer.Flush()
//}
//

// BuildSystemPrompt 构建系统提示
func BuildSystemPrompt(reg []*skill.Skill) string {
	var sb strings.Builder

	sb.WriteString(`
You are an 智能协作助手 CLI assistant.

You have access to the following skills.

When a user's request matches a skill:
- Say: USING_SKILL: <name> and use load skill tool
- The system will load the skill content for you
- Then follow its instructions strictly

Available skills:
`)

	for _, s := range reg {
		if s.DisableModelInvocation {
			continue
		}

		sb.WriteString("\n---\n")
		sb.WriteString("Name: " + s.Name + "\n")
		sb.WriteString("Description: " + s.Description + "\n")

		if s.WhenToUse != "" {
			sb.WriteString("WhenToUse: " + s.WhenToUse + "\n")
		}
	}

	sb.WriteString(`
When you decide to use a skill:
1. Output EXACTLY: USING_SKILL: <name>
2. Do NOT output any command yet
3. Wait for the system to load the skill content
`)

	return sb.String()
}

// ChatWS 处理 WebSocket 连接。通过 session_id 查询参数区分会话。
func (c *Controller) ChatWS(ctx *gin.Context) {
	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		sessionID = "default"
	}

	wsHandler := transport.NewHandler(c.Bus, func(ctx context.Context, sid string, msg transport.WSMsg) {
		switch msg.Type {
		case "execute":
			if rt, ok, err := c.Memory.GetRuntime(ctx, sid); err == nil && ok && rt.PlanID != "" {
				c.Memory.AppendMessage(ctx, []*atype.Message{{
					SessionID: sid, PlanID: rt.PlanID, StepID: rt.StepID,
					Role: atype.RoleUser, Content: msg.Data, CreatedAt: time.Now(),
				}})
			}
			req := atype.Request{SessionID: sid, UserInput: msg.Data}
			ctx2 := event.WithBus(ctx, c.Bus)
			_, _ = c.Runnable.Invoke(ctx2, &nodes.State{Request: &req})

		case "approve":
			rt, ok, err := c.Memory.GetRuntime(ctx, sid)
			if err == nil && ok && rt != nil {
				rt.PendingToolApproved = true
				rt.Status = atype.RuntimeApproved
				_ = c.Memory.SaveRuntime(ctx, rt)
			}
			ctx2 := tool.WithApprovedCommand(event.WithBus(ctx, c.Bus), msg.Data)
			_, _ = c.Runnable.Invoke(ctx2, &nodes.State{Request: &atype.Request{SessionID: sid}})

		case "reject":
			rt, ok, err := c.Memory.GetRuntime(ctx, sid)
			if err == nil && ok && rt != nil {
				rt.Status = atype.RuntimeApproved
				_ = c.Memory.SaveRuntime(ctx, rt)
			}
			ctx2 := event.WithBus(ctx, c.Bus)
			_, _ = c.Runnable.Invoke(ctx2, &nodes.State{Request: &atype.Request{SessionID: sid}})

		case "plan_approve":
			rt, ok, err := c.Memory.GetRuntime(ctx, sid)
			if err == nil && ok && rt != nil {
				rt.Status = atype.RuntimeApproved
				rt.PlanAction = "approved"
				_ = c.Memory.SaveRuntime(ctx, rt)
			}
			ctx2 := event.WithBus(ctx, c.Bus)
			_, _ = c.Runnable.Invoke(ctx2, &nodes.State{Request: &atype.Request{SessionID: sid}})

		case "plan_reject":
			rt, ok, err := c.Memory.GetRuntime(ctx, sid)
			if err == nil && ok && rt != nil {
				rt.Status = atype.RuntimeApproved
				rt.PlanAction = "rejected"
				_ = c.Memory.SaveRuntime(ctx, rt)
			}
			ctx2 := event.WithBus(ctx, c.Bus)
			_, _ = c.Runnable.Invoke(ctx2, &nodes.State{Request: &atype.Request{SessionID: sid}})

		case "interrupt":
			rt, ok, err := c.Memory.GetRuntime(ctx, sid)
			if err == nil && ok && rt != nil {
				rt.InterruptRequested = true
				_ = c.Memory.SaveRuntime(ctx, rt)
			}
			c.Graph.SignalInterrupt(sid)
		}
	})

	websocket.Server{Handler: wsHandler.ServeWS}.ServeHTTP(ctx.Writer, ctx.Request)
}

// CreateSession 创建新对话
// @Summary      创建新对话
// @Description  创建新对话
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        Authorization header string true "Bearer Token" default(Bearer )
// @Success      200   {object}  pkgmodel.Response "data.message"
// @Failure      400   {object}  pkgmodel.Response
// @Failure      500   {object}  pkgmodel.Response
// @Router       /chat/session [post]
// @Security     BearerAuth
func (c *Controller) CreateSession(ctx *gin.Context, claim jwt.UserClaims) (pkgmodel.Response, error) {
	userID := claim.ID
	if userID == "" {
		userID = "anonymous"
	}

	session, err := c.Memory.CreateChatSession(ctx.Request.Context(), userID)
	if err != nil {
		return pkgmodel.Response{}, err
	}
	return pkgmodel.Response{
		Code:    200,
		Message: "ok",
		Data: map[string]interface{}{
			"session_id": session.ID,
			"user_id":    session.UserID,
			"created_at": session.CreatedAt,
		},
	}, nil
}
