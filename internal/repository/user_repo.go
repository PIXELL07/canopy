package repository

import (
	"context"
	"time"

	"github.com/pixell07/canopy/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type UserRepo struct {
	col *mongo.Collection
}

func NewUserRepo(db *DB) *UserRepo {
	return &UserRepo{col: db.Database.Collection("users")}
}

func (r *UserRepo) Create(ctx context.Context, u *models.User) error {
	u.ID = primitive.NewObjectID()
	u.CreatedAt = time.Now()
	u.Active = true
	_, err := r.col.InsertOne(ctx, u)
	return err
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (*models.User, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	var u models.User
	if err := r.col.FindOne(ctx, bson.M{"_id": oid}).Decode(&u); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	if err := r.col.FindOne(ctx, bson.M{"email": email}).Decode(&u); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) GetByAPIKey(ctx context.Context, key string) (*models.User, error) {
	var u models.User
	if err := r.col.FindOne(ctx, bson.M{"api_key": key, "active": true}).Decode(&u); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) UpdateLastLogin(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = r.col.UpdateOne(ctx,
		bson.M{"_id": oid},
		bson.M{"$set": bson.M{"last_login_at": now}},
	)
	return err
}

func (r *UserRepo) List(ctx context.Context) ([]*models.User, error) {
	cur, err := r.col.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*models.User
	return results, cur.All(ctx, &results)
}
