package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type InterruptArgs struct {
	Question string `json:"question"`
}

// InterruptTool 用一个工具来让llm自己决定是否应该暂停
type InterruptTool struct{}

func (t *InterruptTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "request_human_input",
		Desc: `
Pause current execution and request additional input from the user.
Use this tool when:
- required information is missing
- confirmation is required
- the instruction is ambiguous
- a user decision is needed
After calling this tool the agent MUST stop execution immediately.
`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"question": {
				Type:     schema.String,
				Required: true,
			},
			"kind": {
				Type:     schema.String,
				Required: true,
				Enum: []string{
					"missing_info",
					"approval",
				},
			},
		}),
	}, nil
}

func (t *InterruptTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var req InterruptArgs
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return "", fmt.Errorf(
			"invalid request_human_input args: %w",
			err,
		)
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		return "", fmt.Errorf("question is required")
	}
	return req.Question, nil
}
