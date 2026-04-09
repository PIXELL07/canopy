package repository

import (
	"context"
	"time"

	"github.com/pixell07/canopy/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Audit Repo

type AuditRepo struct {
	col *mongo.Collection
}

func NewAuditRepo(db *DB) *AuditRepo {
	return &AuditRepo{col: db.Database.Collection("audit_log")}
}

// Append writes a new audit entry. This collection is append-only — never update or delete.
func (r *AuditRepo) Append(ctx context.Context, entry *models.AuditEntry) error {
	entry.ID = primitive.NewObjectID()
	entry.CreatedAt = time.Now()
	_, err := r.col.InsertOne(ctx, entry)
	return err
}

func (r *AuditRepo) ListForResource(ctx context.Context, resourceID string, limit int64) ([]*models.AuditEntry, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit)
	cur, err := r.col.Find(ctx, bson.M{"resource_id": resourceID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.AuditEntry
	return results, cur.All(ctx, &results)
}

func (r *AuditRepo) ListForActor(ctx context.Context, actorID string, limit int64) ([]*models.AuditEntry, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit)
	cur, err := r.col.Find(ctx, bson.M{"actor_id": actorID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.AuditEntry
	return results, cur.All(ctx, &results)
}

// Webhook Repo

type WebhookRepo struct {
	col *mongo.Collection
}

func NewWebhookRepo(db *DB) *WebhookRepo {
	return &WebhookRepo{col: db.Database.Collection("webhooks")}
}

func (r *WebhookRepo) Create(ctx context.Context, wh *models.Webhook) error {
	wh.ID = primitive.NewObjectID()
	wh.CreatedAt = time.Now()
	wh.Active = true
	_, err := r.col.InsertOne(ctx, wh)
	return err
}

func (r *WebhookRepo) List(ctx context.Context) ([]*models.Webhook, error) {
	cur, err := r.col.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.Webhook
	return results, cur.All(ctx, &results)
}

func (r *WebhookRepo) GetActiveForEvent(ctx context.Context, event models.WebhookEvent) ([]*models.Webhook, error) {
	cur, err := r.col.Find(ctx, bson.M{
		"active": true,
		"events": bson.M{"$in": []models.WebhookEvent{event}},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.Webhook
	return results, cur.All(ctx, &results)
}

func (r *WebhookRepo) Delete(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	res, err := r.col.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}
