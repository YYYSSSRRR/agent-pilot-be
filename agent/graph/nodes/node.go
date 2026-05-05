package nodes

import (
	"context"

	"github.com/agent-pilot/agent-pilot-be/agent/scheduler"
	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
)

type Node interface {
	Invoke(ctx context.Context, state *State) (*State, error)
}

type State struct {
	Request  *atype.Request
	Plan     *atype.Plan
	Runtime  *atype.Runtime
	Decision *scheduler.Decision
	Result   *atype.Result
}
