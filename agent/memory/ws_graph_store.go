package memory

import (
	"context"

	"github.com/agent-pilot/agent-pilot-be/repository/dao"
	"github.com/cloudwego/eino/compose"
)

type graphCheckPointStore struct {
	d dao.AgentDao
}

func (s *graphCheckPointStore) Get(ctx context.Context, id string) ([]byte, bool, error) {
	if s == nil || s.d == nil {
		return nil, false, nil
	}
	return s.d.WSRuntimeGraphGet(ctx, id)
}

func (s *graphCheckPointStore) Set(ctx context.Context, id string, graph []byte) error {
	if s == nil || s.d == nil {
		return nil
	}
	return s.d.WSRuntimeGraphSet(ctx, id, graph)
}

// GraphCheckPointStore 返回基于 Mongo 的 eino compose 检查点存储，与 websocket session_id（compose CheckPointID）对齐。
func (s *memoryService) GraphCheckPointStore() compose.CheckPointStore {
	if s == nil || s.dao == nil {
		return nil
	}
	return &graphCheckPointStore{d: s.dao}
}
