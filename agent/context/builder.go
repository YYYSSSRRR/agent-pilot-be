package context

import (
	"fmt"
	"strings"

	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/cloudwego/eino/schema"
)

type Options struct {
	SystemPrompt     string
	MaxMessages      int
	MaxContentChars  int
	DropEmptyContent bool
	MaxPlans         int
}

type Builder struct {
	opt Options
}

func NewBuilder(opt Options) *Builder {
	return &Builder{opt: opt}
}

func (b *Builder) BuildExecutionContext(execCtx *atype.ExecutionContext) ([]*schema.Message, error) {
	var out []*schema.Message

	var ctx strings.Builder
	ctx.WriteString("Execution context:\n\n")
	ctx.WriteString("Goal:\n")
	ctx.WriteString(execCtx.Plan.Goal)
	ctx.WriteString("\n\n")
	ctx.WriteString("Current Step:\n")
	ctx.WriteString(execCtx.Step.Title)
	ctx.WriteString("\n")
	ctx.WriteString("Step Description:\n")
	ctx.WriteString(execCtx.Step.Description)
	ctx.WriteString("\n\n")

	out = append(out, schema.UserMessage(ctx.String()))
	msgs, err := b.build(execCtx.Messages)
	if err != nil {
		return nil, err
	}
	out = append(out, msgs...)
	return out, nil
}

func (b *Builder) BuildPlanContext(req atype.Request, plans []*atype.Plan) ([]*schema.Message, error) {
	out := make([]*schema.Message, 0, 2)
	if strings.TrimSpace(b.opt.SystemPrompt) != "" {
		out = append(out, schema.SystemMessage(strings.TrimSpace(b.opt.SystemPrompt)))
	}
	out = append(out, schema.UserMessage(b.planContext(req, plans)))
	return out, nil
}

func (b *Builder) planContext(req atype.Request, plans []*atype.Plan) string {
	var sb strings.Builder
	sb.WriteString("User request:\n")
	sb.WriteString(strings.TrimSpace(req.UserInput))
	sb.WriteString("\n\nHistorical plans (most recent first):\n")

	limit := b.opt.MaxPlans
	if limit <= 0 {
		limit = 5
	}
	if len(plans) > limit {
		plans = plans[:limit]
	}
	if len(plans) == 0 {
		sb.WriteString("- (none)\n")
		return sb.String()
	}

	for _, p := range plans {
		sb.WriteString("- ")
		sb.WriteString(p.ID)
		if strings.TrimSpace(p.Goal) != "" {
			sb.WriteString(" goal=")
			sb.WriteString(trimForPrompt(p.Goal, 160))
		}
		if !p.UpdatedAt.IsZero() {
			sb.WriteString(" updated_at=")
			sb.WriteString(p.UpdatedAt.Format("2006-01-02 15:04:05"))
		}
		sb.WriteString("\n")

		// Steps summary (no messages)
		for i := range p.Steps {
			st := p.Steps[i]
			sb.WriteString("  - step ")
			sb.WriteString(st.ID)
			if strings.TrimSpace(st.Title) != "" {
				sb.WriteString(" title=")
				sb.WriteString(trimForPrompt(st.Title, 120))
			}
			if strings.TrimSpace(st.Result) != "" {
				sb.WriteString(" result=")
				sb.WriteString(trimForPrompt(st.Result, 160))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// 把atype.Message转换为schema.Message
func (b *Builder) build(messages []*atype.Message) ([]*schema.Message, error) {
	out := make([]*schema.Message, 0, len(messages)+1)
	if strings.TrimSpace(b.opt.SystemPrompt) != "" {
		out = append(out, schema.SystemMessage(strings.TrimSpace(b.opt.SystemPrompt)))
	}

	start := 0
	if b.opt.MaxMessages > 0 && len(messages) > b.opt.MaxMessages {
		start = len(messages) - b.opt.MaxMessages
	}

	for i := start; i < len(messages); i++ {
		m := messages[i]
		sm, err := toSchemaMessage(*m, b.opt)
		if err != nil {
			return nil, fmt.Errorf("build ctx: message[%d] (%s): %w", i, m.ID, err)
		}
		if sm == nil {
			continue
		}
		out = append(out, sm)
	}

	return out, nil
}

func toSchemaMessage(m atype.Message, opt Options) (*schema.Message, error) {
	content := normalizeContent(m.Content, opt.MaxContentChars)
	if opt.DropEmptyContent && strings.TrimSpace(content) == "" &&
		m.Role != atype.RoleToolCall && m.Role != atype.RoleToolResult {
		return nil, nil
	}

	switch m.Role {
	case atype.RoleUser:
		if opt.DropEmptyContent && strings.TrimSpace(content) == "" {
			return nil, nil
		}
		return schema.UserMessage(content), nil

	case atype.RoleAssistant:
		return schema.AssistantMessage(content, nil), nil

	case atype.RoleToolCall:
		if strings.TrimSpace(content) == "" {
			name, _ := stringMeta(m.Metadata, "tool_name")
			if strings.TrimSpace(name) == "" {
				name, _ = stringMeta(m.Metadata, "name")
			}
			args, _ := stringMeta(m.Metadata, "arguments")
			if strings.TrimSpace(name) != "" {
				if strings.TrimSpace(args) != "" {
					content = "TOOL_CALL " + name + " args=" + args
				} else {
					content = "TOOL_CALL " + name
				}
			}
		}
		if opt.DropEmptyContent && strings.TrimSpace(content) == "" {
			return nil, nil
		}
		return schema.AssistantMessage(content, nil), nil

	case atype.RoleToolResult:
		callID, _ := stringMeta(m.Metadata, "call_id")
		if strings.TrimSpace(callID) == "" {
			callID, _ = stringMeta(m.Metadata, "tool_call_id")
		}
		if strings.TrimSpace(callID) == "" {
			return nil, fmt.Errorf("tool_result missing call_id in metadata")
		}

		toolName, _ := stringMeta(m.Metadata, "tool_name")
		if strings.TrimSpace(toolName) == "" {
			toolName, _ = stringMeta(m.Metadata, "name")
		}

		if strings.TrimSpace(toolName) != "" {
			return schema.ToolMessage(content, callID, schema.WithToolName(toolName)), nil
		}
		return schema.ToolMessage(content, callID), nil

	default:
		if opt.DropEmptyContent && strings.TrimSpace(content) == "" {
			return nil, nil
		}
		return schema.AssistantMessage(content, nil), nil
	}
}

func normalizeContent(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}

func stringMeta(meta map[string]any, key string) (string, bool) {
	if meta == nil {
		return "", false
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case fmt.Stringer:
		return t.String(), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

func trimForPrompt(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
