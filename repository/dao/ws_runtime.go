package dao

import (
	"context"
	"errors"
	"time"

	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/agent-pilot/agent-pilot-be/repository/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//type WSRuntimeResume struct {
//	StepID        string
//	Message       string
//	PlanID        string
//	InterruptKind string
//}

func (d *agentDao) GetRuntime(ctx context.Context, sessionID string) (*atype.Runtime, bool, error) {
	var doc model.WSRuntimeDoc
	err := d.wsRuntimeCol.FindOne(
		ctx,
		bson.M{
			"_id": sessionID,
		},
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return runtimeFromModel(&doc), true, nil
}

func (d *agentDao) SaveRuntime(

	ctx context.Context,
	rt *atype.Runtime,

) error {

	if rt == nil {
		return nil
	}
	now := time.Now()
	rt.UpdatedAt = now
	doc := runtimeToModel(rt)
	_, err := d.wsRuntimeCol.UpdateOne(
		ctx,
		bson.M{
			"_id": rt.SessionID,
		},
		bson.M{
			"$set": doc,
		},
		options.Update().SetUpsert(true),
	)
	return err

}

func (d *agentDao) DeleteRuntime(ctx context.Context, sessionID string) error {
	_, err := d.wsRuntimeCol.DeleteOne(
		ctx,
		bson.M{
			"_id": sessionID,
		},
	)
	return err

}

func (d *agentDao) WSRuntimeGraphGet(ctx context.Context, sessionID string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	var doc model.WSRuntimeDoc
	err := d.wsRuntimeCol.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	if len(doc.Graph) == 0 {
		return nil, false, nil
	}
	return append([]byte(nil), doc.Graph...), true, nil
}

func (d *agentDao) WSRuntimeGraphSet(ctx context.Context, sessionID string, graph []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	_, err := d.wsRuntimeCol.UpdateOne(
		ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$set": bson.M{
				"graph":      append([]byte(nil), graph...),
				"updated_at": now,
			},
			"$setOnInsert": bson.M{"_id": sessionID},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

//func (ad *agentDao) WSRuntimeResumeGet(ctx context.Context, sessionID string) (WSRuntimeResume, bool, error) {
//	var zero WSRuntimeResume
//	if err := ctx.Err(); err != nil {
//		return zero, false, err
//	}
//
//	var doc model.WSRuntimeDoc
//	err := ad.wsRuntimeCol.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
//	if errors.Is(err, mongo.ErrNoDocuments) {
//		return zero, false, nil
//	}
//	if err != nil {
//		return zero, false, err
//	}
//
//	if doc.StepID == "" {
//		return zero, false, nil
//	}
//	return WSRuntimeResume{
//		StepID:        doc.StepID,
//		Message:       "",
//		PlanID:        doc.PlanID,
//		InterruptKind: doc.InterruptKind,
//	}, true, nil
//}
//
//func (ad *agentDao) WSRuntimeResumeSet(ctx context.Context, sessionID string, rec WSRuntimeResume) error {
//	if err := ctx.Err(); err != nil {
//		return err
//	}
//	now := time.Now()
//	_, err := ad.wsRuntimeCol.UpdateOne(
//		ctx,
//		bson.M{"_id": sessionID},
//		bson.M{
//			"$set": bson.M{
//				"step_id":        rec.StepID,
//				"plan_id":        rec.PlanID,
//				"interrupt_kind": rec.InterruptKind,
//				"updated_at":     now,
//			},
//			"$setOnInsert": bson.M{"_id": sessionID},
//		},
//		options.Update().SetUpsert(true),
//	)
//	return err
//}
//
//func (ad *agentDao) WSRuntimeResumeClear(ctx context.Context, sessionID string) error {
//	if err := ctx.Err(); err != nil {
//		return err
//	}
//	_, err := ad.wsRuntimeCol.UpdateOne(
//		ctx,
//		bson.M{"_id": sessionID},
//		bson.M{
//			"$unset": bson.M{
//				"step_id":        "",
//				"plan_id":        "",
//				"interrupt_kind": "",
//			},
//			"$set": bson.M{"updated_at": time.Now()},
//		},
//	)
//	return err
//}
