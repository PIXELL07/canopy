package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
)

// ServerStatusRow is returned by the aggregation that counts servers per status.
type ServerStatusRow struct {
	Status   string `bson:"_id"`
	Count    int64  `bson:"count"`
	IsCanary bool   `bson:"is_canary"`
}

// GetStatusCounts runs a MongoDB aggregation to count servers grouped by status.
// Much cheaper than fetching all servers and counting in Go on large fleets.
func (r *ServerRepo) GetStatusCounts(ctx context.Context) ([]*ServerStatusRow, error) {
	pipeline := bson.A{
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$status"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "is_canary", Value: bson.D{{Key: "$max", Value: "$is_canary"}}},
		}}},
	}
	cur, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []*ServerStatusRow
	return results, cur.All(ctx, &results)
}
