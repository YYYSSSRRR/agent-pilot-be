package model

import "time"

// WSRuntimeDoc 持久化 websocket 会话的 eino 图检查点与可恢复中断元数据（同一 compose CheckPointID / session_id）。
type WSRuntimeDoc struct {
	ID string `bson:"_id"` //session_id
	//当前图状态（checkpoint）
	Graph []byte `bson:"graph,omitempty"`
	// 当前 checkpoint id
	CheckpointID string `bson:"checkpoint_id,omitempty"`
	// 当前 plan
	PlanID string `bson:"plan_id,omitempty"`
	// 当前 step
	StepID string `bson:"step_id,omitempty"`
	//interrupt info
	InterruptKind string `bson:"interrupt_kind,omitempty"`
	// running / interrupted / completed / pending_plan_approval / approved / pending_tool_approval
	Status    string `bson:"status,omitempty"`
	UpdatedAt time.Time     `bson:"updated_at"`
	// 外部中断请求标记
	InterruptRequested bool `bson:"interrupt_requested,omitempty"`
	// 待审批的工具调用命令
	PendingToolCall string `bson:"pending_tool_call,omitempty"`
	// 工具是否已审批
	PendingToolApproved bool `bson:"pending_tool_approved,omitempty"`
	// plan 审批操作: "approved" / "rejected"
	PlanAction string `bson:"plan_action,omitempty"`
}
