package atype

import (
	"time"

	"github.com/cloudwego/eino/schema"
)

type Session struct {
	ID            string    `json:"id" bson:"_id"`
	UserID        string    `json:"user_id" bson:"user_id"`
	CurrentPlanID string    `json:"current_plan_id,omitempty" bson:"current_plan_id,omitempty"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" bson:"updated_at"`
}

type Plan struct {
	ID            string    `json:"id" bson:"_id"`
	SessionID     string    `json:"session_id,omitempty" bson:"session_id,omitempty"`
	Goal          string    `json:"goal" bson:"goal"`
	Steps         []Step    `json:"steps" bson:"steps"`
	CurrentStepID string    `json:"current_step_id,omitempty" bson:"current_step_id,omitempty"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" bson:"updated_at"`
}

type Step struct {
	ID          string `json:"id" bson:"id"`
	Title       string `json:"title" bson:"title"`
	Description string `json:"description" bson:"description"`
	Result      string `json:"result,omitempty" bson:"result,omitempty"`
}

type MessageRole string

const (
	RoleUser       MessageRole = "user"
	RoleAssistant  MessageRole = "assistant"
	RoleSystem     MessageRole = "system"
	RoleToolCall   MessageRole = "tool_call"
	RoleToolResult MessageRole = "tool_result"
)

type Message struct {
	ID        string         `json:"id" bson:"_id"`
	SessionID string         `json:"session_id" bson:"session_id"`
	PlanID    string         `json:"plan_id" bson:"plan_id"`
	StepID    string         `json:"step_id,omitempty" bson:"step_id,omitempty"`
	Role      MessageRole    `json:"role" bson:"role"`
	Content   string         `json:"content" bson:"content"`
	Metadata  map[string]any `json:"metadata,omitempty" bson:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at" bson:"created_at"`
}

type ExecutionContext struct {
	SessionID string
	Plan      *Plan
	Step      *Step
	Messages  []*Message
	IsResume  bool
	InterruptRequested bool
}

type Request struct {
	SessionID string
	UserInput string
	History   []*schema.Message
}

type Result struct {
	Plan    *Plan        `json:"plan"`
	Steps   []StepResult `json:"steps"`
	Summary string       `json:"summary"`
}

type StepResult struct {
	StepID          string     `json:"step_id"`
	Output          string     `json:"output"`
	Paused          bool       `json:"paused"`
	Completed       bool       `json:"completed"`
	Messages        []*Message `json:"messages"`
	PausedKind      string     `json:"pause_kind"`
	PendingToolCall string     `json:"pending_tool_call,omitempty"`
}

type RuntimeStatus string

const (
	RuntimeInterrupted          RuntimeStatus = "interrupted"
	RuntimeCompleted            RuntimeStatus = "completed"
	RuntimeRunning              RuntimeStatus = "running"
	RuntimePendingPlanApproval  RuntimeStatus = "pending_plan_approval"
	RuntimeApproved             RuntimeStatus = "approved"
	RuntimePendingToolApproval  RuntimeStatus = "pending_tool_approval"
)

type Runtime struct {
	SessionID          string
	Graph              []byte
	CheckpointID       string
	PlanID             string
	StepID             string
	InterruptKind      string
	Status             RuntimeStatus `json:"status"`
	UpdatedAt          time.Time
	InterruptRequested bool `json:"interrupt_requested,omitempty"`
	PendingToolCall    string `json:"pending_tool_call,omitempty"`
	PendingToolApproved bool  `json:"pending_tool_approved,omitempty"`
	PlanAction         string `json:"plan_action,omitempty"`
}

// PendingToolCallInfo 序列化到 Runtime.PendingToolCall
type PendingToolCallInfo struct {
	ToolName  string `json:"tool_name"`
	Arguments string `json:"arguments"`
}
