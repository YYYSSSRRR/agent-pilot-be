package memory

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/compose"

	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/agent-pilot/agent-pilot-be/repository/dao"
)

type MemoryService interface {
	//session
	CreateChatSession(ctx context.Context, userID string) (*atype.Session, error)
	GetChatSession(ctx context.Context, chatSessionID string) (*atype.Session, error)

	//plan（仅用于展示，不维护状态）
	SavePlan(ctx context.Context, p *atype.Plan) error
	GetPlan(ctx context.Context, planID string) (*atype.Plan, error)
	ListPlansBySession(ctx context.Context, sessionID string, limit int) ([]*atype.Plan, error)

	//messages
	AppendMessage(ctx context.Context, msg []*atype.Message) error
	GetMessages(ctx context.Context, sessionID string) ([]*atype.Message, error)

	//runtime（唯一执行状态）
	GetRuntime(ctx context.Context, sessionID string) (*atype.Runtime, bool, error)
	SaveRuntime(ctx context.Context, rt *atype.Runtime) error
	DeleteRuntime(ctx context.Context, sessionID string) error

	// WebSocket / eino：图检查点与中断恢复元数据（需 Mongo AgentDao）。
	GraphCheckPointStore() compose.CheckPointStore

	//buildContext
	GetExecutionContext(ctx context.Context, sessionID string) (*atype.ExecutionContext, error)
}

type memoryService struct {
	dao dao.AgentDao
}

func NewMemoryService(d dao.AgentDao) MemoryService {
	return &memoryService{dao: d}
}

// session
func (s *memoryService) CreateChatSession(ctx context.Context, userID string) (*atype.Session, error) {
	return s.dao.CreateChatSession(ctx, userID)
}

func (s *memoryService) GetChatSession(ctx context.Context, chatSessionID string) (*atype.Session, error) {
	return s.dao.GetChatSession(ctx, chatSessionID)
}

// plan
func (s *memoryService) SavePlan(ctx context.Context, p *atype.Plan) error {
	return s.dao.SavePlan(ctx, p)
}

func (s *memoryService) GetPlan(ctx context.Context, planID string) (*atype.Plan, error) {
	return s.dao.GetPlan(ctx, planID)
}

func (s *memoryService) ListPlansBySession(ctx context.Context, sessionID string, limit int) ([]*atype.Plan, error) {
	return s.dao.ListPlansBySession(ctx, sessionID, limit)
}

// messages
func (s *memoryService) AppendMessage(ctx context.Context, msg []*atype.Message) error {
	return s.dao.AppendMessage(ctx, msg)
}

func (s *memoryService) GetMessages(ctx context.Context, sessionID string) ([]*atype.Message, error) {
	return s.dao.GetMessages(ctx, sessionID)
}

// runtime
func (s *memoryService) GetRuntime(ctx context.Context, sessionID string) (*atype.Runtime, bool, error) {
	return s.dao.GetRuntime(ctx, sessionID)
}

func (s *memoryService) SaveRuntime(ctx context.Context, rt *atype.Runtime) error {
	if rt == nil {
		return errors.New("runtime is nil")
	}
	return s.dao.SaveRuntime(ctx, rt)

}

func (s *memoryService) DeleteRuntime(ctx context.Context, sessionID string) error {
	return s.dao.DeleteRuntime(ctx, sessionID)
}

func (s *memoryService) GetExecutionContext(ctx context.Context, sessionID string) (*atype.ExecutionContext, error) {
	//获取runtime运行状态
	rt, ok, err := s.dao.GetRuntime(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("runtime not found for session %s", sessionID)
	}

	//plan
	plan, err := s.dao.GetPlan(ctx, rt.PlanID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, fmt.Errorf("plan not found: %s", rt.PlanID)
	}

	//step
	step := &atype.Step{}
	if rt.StepID != "" {
		step = findStep(plan.Steps, rt.StepID)
		if step == nil {
			return nil, fmt.Errorf("step not found: %s", rt.StepID)
		}
	} else {
		step = &plan.Steps[0]
	}

	// 5. messages（只取当前 plan 或 session 级）
	msgs, err := s.dao.GetPlanMessages(ctx, plan.ID)
	if err != nil {
		return nil, err
	}

	isResume := rt.Status == atype.RuntimeInterrupted

	return &atype.ExecutionContext{
		SessionID:          sessionID,
		Plan:               plan,
		Step:               step,
		Messages:           msgs,
		IsResume:           isResume,
		InterruptRequested: rt.InterruptRequested,
	}, nil
}

func findStep(steps []atype.Step, stepID string) *atype.Step {
	for i := range steps {
		if steps[i].ID == stepID {
			return &steps[i]
		}
	}
	return nil
}
