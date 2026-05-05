package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agent-pilot/agent-pilot-be/agent/tool/skill"
	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Planner interface {
	Plan(ctx context.Context, req atype.Request) (*atype.Plan, error)
}

type LLMPlanner struct {
	model  model.ToolCallingChatModel
	skills []*skill.Skill
	now    func() time.Time
}

func NewLLMPlanner(chatModel model.ToolCallingChatModel, skillReg *skill.Registry) *LLMPlanner {
	var skills []*skill.Skill
	if skillReg != nil {
		skills = skillReg.List()
	}

	return &LLMPlanner{
		model:  chatModel,
		skills: skills,
		now:    time.Now,
	}
}

func (p *LLMPlanner) Plan(ctx context.Context, req atype.Request) (*atype.Plan, error) {
	if p == nil || p.model == nil {
		return nil, fmt.Errorf("planner model is nil")
	}
	if strings.TrimSpace(req.UserInput) == "" {
		return nil, fmt.Errorf("user input is required")
	}

	messages := []*schema.Message{
		schema.SystemMessage(p.systemPrompt()),
		schema.UserMessage(p.userPrompt(req)),
	}

	resp, err := p.model.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return nil, fmt.Errorf("planner returned empty response")
	}

	out, err := parseOutput(resp.Content)
	if err != nil {
		return nil, err
	}

	now := p.now()
	plan := &atype.Plan{
		ID:        NewID("plan"),
		SessionID: req.SessionID,
		Goal:      strings.TrimSpace(out.Goal),
		Steps:     normalizeSteps(out.Steps),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if strings.TrimSpace(plan.Goal) == "" {
		plan.Goal = strings.TrimSpace(req.UserInput)
	}

	return plan, nil
}

func (p *LLMPlanner) systemPrompt() string {
	var sb strings.Builder
	sb.WriteString(`You are the planning layer for a plan-execute agent.

Create a compact, executable plan before any action is taken. The executor will later use ReAct and skill tools, so your job is to decide intent, skill choices, and a practical step breakdown.

Rules:
- Return JSON only. No markdown fences.
- Do not execute tools.
- Prefer available skills when they match a step.
- A step should be small enough for one ReAct execution loop.
- Keep plans practical and short unless the user asks for a broad project.

Required JSON shape:
{
  "goal": "string",
  "steps": [
    {
      "title": "string",
      "description": "string"
    }
  ]
}

Available skills:
`)

	for _, s := range p.skills {
		if s == nil || s.DisableModelInvocation {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(s.Name)
		if s.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(strings.ReplaceAll(s.Description, "\n", " "))
		}
		if s.WhenToUse != "" {
			sb.WriteString(" When to use: ")
			sb.WriteString(strings.ReplaceAll(s.WhenToUse, "\n", " "))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (p *LLMPlanner) userPrompt(req atype.Request) string {
	var sb strings.Builder
	sb.WriteString("User request:\n")
	sb.WriteString(req.UserInput)
	sb.WriteString("\n\nRecent conversation context:\n")

	for _, msg := range lastMessages(req.History, 8) {
		if msg == nil || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(string(msg.Role))
		sb.WriteString(": ")
		sb.WriteString(trimForPrompt(msg.Content, 1200))
		sb.WriteString("\n")
	}

	return sb.String()
}

type plannerOutput struct {
	Goal  string       `json:"goal"`
	Steps []outputStep `json:"steps"`
}

type outputStep struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func parseOutput(content string) (*plannerOutput, error) {
	raw := strings.TrimSpace(content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var out plannerOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("decode planner output: %w", err)
	}
	return &out, nil
}

func normalizeSteps(steps []outputStep) []atype.Step {
	out := make([]atype.Step, 0, len(steps))
	for i, step := range steps {
		title := strings.TrimSpace(step.Title)
		desc := strings.TrimSpace(step.Description)
		if title == "" {
			title = fmt.Sprintf("Step %d", i+1)
		}
		out = append(out, atype.Step{
			ID:          fmt.Sprintf("step_%02d", i+1),
			Title:       title,
			Description: desc,
		})
	}
	return out
}

func lastMessages(in []*schema.Message, n int) []*schema.Message {
	if len(in) <= n {
		return in
	}
	return in[len(in)-n:]
}

func trimForPrompt(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
