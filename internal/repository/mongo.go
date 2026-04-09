package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DB struct {
	Client   *mongo.Client
	Database *mongo.Database
}

func (db *DB) Disconnect(ctx context.Context) error {
	return db.Client.Disconnect(ctx)
}

func NewMongoClient(ctx context.Context, uri, dbName string) (*DB, error) {
	opts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := &DB{Client: client, Database: client.Database(dbName)}
	return db, ensureIndexes(ctx, db.Database)
}

func ensureIndexes(ctx context.Context, db *mongo.Database) error {
	type spec struct {
		col   string
		model mongo.IndexModel
	}

	indexes := []spec{
		// users
		{col: "users", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetName("email_unique").SetUnique(true),
		}},
		{col: "users", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "api_key", Value: 1}},
			Options: options.Index().SetName("api_key_unique").SetUnique(true).SetSparse(true),
		}},
		// deployments
		{col: "deployments", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "status", Value: 1}},
			Options: options.Index().SetName("status_1"),
		}},
		{col: "deployments", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("created_desc"),
		}},
		// servers
		{col: "servers", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "is_canary", Value: 1}},
			Options: options.Index().SetName("status_canary"),
		}},
		{col: "servers", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "deployment_id", Value: 1}, {Key: "is_canary", Value: 1}},
			Options: options.Index().SetName("deployment_canary"),
		}},
		{col: "servers", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "last_heartbeat", Value: 1}},
			Options: options.Index().SetName("heartbeat_1"),
		}},
		// metrics — compound + TTL
		{col: "metrics", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "deployment_id", Value: 1}, {Key: "recorded_at", Value: -1}},
			Options: options.Index().SetName("deploy_recorded"),
		}},
		{col: "metrics", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "server_id", Value: 1}, {Key: "recorded_at", Value: -1}},
			Options: options.Index().SetName("server_recorded"),
		}},
		{col: "metrics", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "recorded_at", Value: 1}},
			Options: options.Index().SetName("ttl_30d").SetExpireAfterSeconds(30 * 24 * 3600),
		}},
		// audit log — append-only, query by resource or actor
		{col: "audit_log", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "resource_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("resource_time"),
		}},
		{col: "audit_log", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "actor_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("actor_time"),
		}},
		// webhooks
		{col: "webhooks", model: mongo.IndexModel{
			Keys:    bson.D{{Key: "active", Value: 1}},
			Options: options.Index().SetName("active_1"),
		}},
	}

	for _, idx := range indexes {
		if _, err := db.Collection(idx.col).Indexes().CreateOne(ctx, idx.model); err != nil {
			return err
		}
	}
	return nil
}
