package tool

import "context"

type ctxKey string

const (
	ctxKeyApprovedCmd ctxKey = "approved_shell_cmd"
	ctxKeyApprovedID  ctxKey = "approved_tool_call_id"
)

// WithApprovedCommand 在 context 中标记某条 shell 命令已被用户批准。
func WithApprovedCommand(ctx context.Context, cmd string) context.Context {
	return context.WithValue(ctx, ctxKeyApprovedCmd, cmd)
}

// GetApprovedCommand 检查 context 中是否包含已批准的 shell 命令。
// 返回命令原文和 true（如已批准）。
func GetApprovedCommand(ctx context.Context) (string, bool) {
	cmd, ok := ctx.Value(ctxKeyApprovedCmd).(string)
	return cmd, ok
}
