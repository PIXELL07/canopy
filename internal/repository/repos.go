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

// Deployment Repo

type DeploymentRepo struct {
	col *mongo.Collection
}

func NewDeploymentRepo(db *DB) *DeploymentRepo {
	return &DeploymentRepo{col: db.Database.Collection("deployments")}
}

func (r *DeploymentRepo) Create(ctx context.Context, d *models.Deployment) error {
	d.ID = primitive.NewObjectID()
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()
	_, err := r.col.InsertOne(ctx, d)
	return err
}

func (r *DeploymentRepo) GetByID(ctx context.Context, id string) (*models.Deployment, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	var d models.Deployment
	if err := r.col.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (r *DeploymentRepo) List(ctx context.Context, limit, skip int64) ([]*models.Deployment, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit).SetSkip(skip)
	cur, err := r.col.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.Deployment
	return results, cur.All(ctx, &results)
}

func (r *DeploymentRepo) GetActive(ctx context.Context) ([]*models.Deployment, error) {
	filter := bson.M{"status": bson.M{"$in": []models.DeploymentStatus{
		models.StatusCanary, models.StatusMonitoring, models.StatusRollingOut,
	}}}
	cur, err := r.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.Deployment
	return results, cur.All(ctx, &results)
}

func (r *DeploymentRepo) UpdateStatus(ctx context.Context, id string, status models.DeploymentStatus) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	res, err := r.col.UpdateOne(ctx, bson.M{"_id": oid},
		bson.M{"$set": bson.M{"status": status, "updated_at": time.Now()}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *DeploymentRepo) UpdateCanaryServers(ctx context.Context, id string, serverIDs []string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": oid},
		bson.M{"$set": bson.M{"canary_server_ids": serverIDs, "updated_at": time.Now()}})
	return err
}

func (r *DeploymentRepo) MarkCompleted(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": oid},
		bson.M{"$set": bson.M{"status": models.StatusCompleted, "completed_at": now, "updated_at": now}})
	return err
}

func (r *DeploymentRepo) MarkRolledBack(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": oid},
		bson.M{"$set": bson.M{"status": models.StatusRolledBack, "rollback_at": now, "updated_at": now}})
	return err
}

// Server Repo

type ServerRepo struct {
	col *mongo.Collection
}

func NewServerRepo(db *DB) *ServerRepo {
	return &ServerRepo{col: db.Database.Collection("servers")}
}

func (r *ServerRepo) Create(ctx context.Context, s *models.Server) error {
	s.ID = primitive.NewObjectID()
	s.CreatedAt = time.Now()
	s.LastHeartbeat = time.Now()
	_, err := r.col.InsertOne(ctx, s)
	return err
}

func (r *ServerRepo) GetByID(ctx context.Context, id string) (*models.Server, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	var s models.Server
	if err := r.col.FindOne(ctx, bson.M{"_id": oid}).Decode(&s); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *ServerRepo) List(ctx context.Context) ([]*models.Server, error) {
	cur, err := r.col.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.Server
	return results, cur.All(ctx, &results)
}

func (r *ServerRepo) CountAll(ctx context.Context) (int64, error) {
	return r.col.CountDocuments(ctx, bson.M{})
}

func (r *ServerRepo) GetNHealthyServers(ctx context.Context, n int) ([]*models.Server, error) {
	cur, err := r.col.Find(ctx, bson.M{"status": models.ServerHealthy, "is_canary": false})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var all []*models.Server
	if err := cur.All(ctx, &all); err != nil {
		return nil, err
	}
	if len(all) > n {
		return all[:n], nil
	}
	return all, nil
}

func (r *ServerRepo) SetCanary(ctx context.Context, serverID, deploymentID, version string, isCanary bool) error {
	oid, err := primitive.ObjectIDFromHex(serverID)
	if err != nil {
		return err
	}
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": oid},
		bson.M{"$set": bson.M{
			"is_canary": isCanary, "deployment_id": deploymentID,
			"current_version": version, "last_heartbeat": time.Now(),
		}})
	return err
}

func (r *ServerRepo) PromoteAll(ctx context.Context, deploymentID, version string) error {
	_, err := r.col.UpdateMany(ctx, bson.M{},
		bson.M{"$set": bson.M{
			"current_version": version, "is_canary": false,
			"deployment_id": deploymentID, "last_heartbeat": time.Now(),
		}})
	return err
}

func (r *ServerRepo) RollbackCanaries(ctx context.Context, deploymentID, prevVersion string) error {
	_, err := r.col.UpdateMany(ctx,
		bson.M{"deployment_id": deploymentID, "is_canary": true},
		bson.M{"$set": bson.M{
			"current_version": prevVersion, "is_canary": false, "last_heartbeat": time.Now(),
		}})
	return err
}

func (r *ServerRepo) UpdateStatus(ctx context.Context, serverID string, status models.ServerStatus) error {
	oid, err := primitive.ObjectIDFromHex(serverID)
	if err != nil {
		return err
	}
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": oid},
		bson.M{"$set": bson.M{"status": status}})
	return err
}

func (r *ServerRepo) RecordHeartbeat(ctx context.Context, serverID string) error {
	oid, err := primitive.ObjectIDFromHex(serverID)
	if err != nil {
		return err
	}
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": oid},
		bson.M{"$set": bson.M{"last_heartbeat": time.Now(), "status": models.ServerHealthy}})
	return err
}

// GetStale returns servers whose last_heartbeat is older than the given time.
func (r *ServerRepo) GetStale(ctx context.Context, olderThan time.Time) ([]*models.Server, error) {
	filter := bson.M{
		"last_heartbeat": bson.M{"$lt": olderThan},
		"status":         bson.M{"$ne": models.ServerOffline},
	}
	cur, err := r.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.Server
	return results, cur.All(ctx, &results)
}

// Metrics Repo
type MetricsRepo struct {
	col *mongo.Collection
}

func NewMetricsRepo(db *DB) *MetricsRepo {
	return &MetricsRepo{col: db.Database.Collection("metrics")}
}

func (r *MetricsRepo) Record(ctx context.Context, m *models.Metrics) error {
	m.ID = primitive.NewObjectID()
	m.RecordedAt = time.Now()
	_, err := r.col.InsertOne(ctx, m)
	return err
}

func (r *MetricsRepo) GetSince(ctx context.Context, deploymentID string, since time.Time) ([]*models.Metrics, error) {
	cur, err := r.col.Find(ctx, bson.M{
		"deployment_id": deploymentID,
		"recorded_at":   bson.M{"$gte": since},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.Metrics
	return results, cur.All(ctx, &results)
}

func (r *MetricsRepo) GetForServer(ctx context.Context, serverID string, limit int64) ([]*models.Metrics, error) {
	opts := options.Find().SetSort(bson.D{{Key: "recorded_at", Value: -1}}).SetLimit(limit)
	cur, err := r.col.Find(ctx, bson.M{"server_id": serverID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.Metrics
	return results, cur.All(ctx, &results)
}
