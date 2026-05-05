package dao

import (
	"context"
	"time"

	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/agent-pilot/agent-pilot-be/repository/model"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AgentDao interface {
	//session
	CreateChatSession(ctx context.Context, userID string) (*atype.Session, error)
	GetChatSession(ctx context.Context, chatSessionID string) (*atype.Session, error)

	//plan（仅用于展示，不维护状态）
	SavePlan(ctx context.Context, p *atype.Plan) error
	GetPlan(ctx context.Context, planID string) (*atype.Plan, error)
	ListPlansBySession(ctx context.Context, sessionID string, limit int) ([]*atype.Plan, error)

	//message
	AppendMessage(ctx context.Context, msg []*atype.Message) error
	GetMessages(ctx context.Context, sessionID string) ([]*atype.Message, error)
	GetPlanMessages(ctx context.Context, planID string) ([]*atype.Message, error)
	GetStepMessages(ctx context.Context, planID string, stepID string) ([]*atype.Message, error)
	InsertMessages(ctx context.Context, docs []any) error //runtime(唯一执行状态)
	GetRuntime(ctx context.Context, sessionID string) (*atype.Runtime, bool, error)
	SaveRuntime(ctx context.Context, rt *atype.Runtime) error
	DeleteRuntime(ctx context.Context, sessionID string) error

	//graph checkpoint store,给 compose.CheckPointStore 用
	WSRuntimeGraphGet(ctx context.Context, sessionID string) ([]byte, bool, error)
	WSRuntimeGraphSet(ctx context.Context, sessionID string, graph []byte) error

}

type agentDao struct {
	chatSessionCol *mongo.Collection
	planCol        *mongo.Collection
	messageCol     *mongo.Collection
	wsRuntimeCol   *mongo.Collection
}

func NewAgentDao(db *mongo.Database) AgentDao {
	return &agentDao{
		chatSessionCol: db.Collection("agent_sessions"),
		planCol:        db.Collection("agent_plans"),
		messageCol:     db.Collection("agent_messages"),
		wsRuntimeCol:   db.Collection("agent_ws_runtime"),
	}
}

// session
func (d *agentDao) CreateChatSession(ctx context.Context, userID string) (*atype.Session, error) {
	now := time.Now()
	m := model.ChatSession{
		ID:        uuid.New().String(),
		UserID:    userID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := d.chatSessionCol.InsertOne(ctx, &m)
	if err != nil {
		return nil, err
	}
	return sessionFromChatSession(&m), nil
}

func (d *agentDao) GetChatSession(ctx context.Context, chatSessionID string) (*atype.Session, error) {
	var m model.ChatSession
	err := d.chatSessionCol.FindOne(ctx, bson.M{"_id": chatSessionID}).Decode(&m)
	if err != nil {
		return nil, err
	}
	return sessionFromChatSession(&m), nil
}

// plan
func (d *agentDao) SavePlan(ctx context.Context, plan *atype.Plan) error {
	if plan == nil {
		return nil
	}
	now := time.Now()
	//如果新建：重新生成id
	if plan.ID == "" {
		plan.ID = primitive.NewObjectID().Hex()
	}
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = now
	}
	plan.UpdatedAt = now
	doc := modelFromPlan(plan)
	_, err := d.planCol.UpdateOne(
		ctx,
		bson.M{
			"_id": doc.ID,
		},
		bson.M{
			"$set": doc,
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (d *agentDao) GetPlan(ctx context.Context, planID string) (*atype.Plan, error) {
	var r model.Plan
	err := d.planCol.FindOne(ctx, bson.M{"_id": planID}).Decode(&r)
	if err != nil {
		return nil, err
	}
	return planFromModel(&r), nil
}

func (d *agentDao) ListPlansBySession(ctx context.Context, sessionID string, limit int) ([]*atype.Plan, error) {
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	cur, err := d.planCol.Find(
		ctx,
		bson.M{"session_id": sessionID},
		options.Find().
			SetSort(bson.D{{Key: "updated_at", Value: -1}}).
			SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	out := make([]*atype.Plan, 0, limit)
	for cur.Next(ctx) {
		var r model.Plan
		if err := cur.Decode(&r); err != nil {
			return nil, err
		}
		p := planFromModel(&r)
		if p == nil {
			continue
		}
		out = append(out, p)
	}
	return out, cur.Err()
}

// message
func (d *agentDao) AppendMessage(ctx context.Context, messages []*atype.Message) error {
	if len(messages) == 0 {
		return nil
	}
	var docs []any
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.ID == "" {
			msg.ID = uuid.New().String()
		}
		if msg.CreatedAt.IsZero() {
			msg.CreatedAt = time.Now()
		}
		docs = append(docs, agentMessageFromPlan(msg))
	}

	_, err := d.messageCol.InsertMany(ctx, docs)
	return err
}

func (d *agentDao) InsertMessages(ctx context.Context, docs []any) error {
	if len(docs) == 0 {
		return nil
	}
	_, err := d.messageCol.InsertMany(ctx, docs)
	return err
}

func (d *agentDao) GetPlanMessages(ctx context.Context, planID string) ([]*atype.Message, error) {
	return d.findMessages(ctx, bson.M{"plan_id": planID})
}

func (d *agentDao) GetStepMessages(ctx context.Context, planID string, stepID string) ([]*atype.Message, error) {
	return d.findMessages(ctx, bson.M{"plan_id": planID, "step_id": stepID})
}

func (d *agentDao) GetMessages(ctx context.Context, sessionID string) ([]*atype.Message, error) {
	return d.findMessages(ctx, bson.M{"session_id": sessionID})
}

func (d *agentDao) findMessages(ctx context.Context, filter bson.M) ([]*atype.Message, error) {
	cur, err := d.messageCol.Find(
		ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []*atype.Message
	for cur.Next(ctx) {
		var m model.AgentMessage
		if err := cur.Decode(&m); err != nil {
			return nil, err
		}
		out = append(out, messageFromAgent(&m))
	}
	return out, cur.Err()
}
