package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps go-redis with helpers for rate limiting and caching.
type RedisClient struct {
	client *redis.Client
}

func NewRedisClient(addr, password string) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	return &RedisClient{client: rdb}, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

// RateLimit checks a sliding window rate limit for a given key.
// Returns (allowed bool, remaining int, err).
// Uses a simple fixed-window counter in Redis with a 1-minute TTL.
func (r *RedisClient) RateLimit(ctx context.Context, key string, limitPerMinute int) (bool, int, error) {
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, "rl:"+key)
	pipe.Expire(ctx, "rl:"+key, time.Minute)

	if _, err := pipe.Exec(ctx); err != nil {
		// Fail open — don't block requests if Redis is down
		return true, limitPerMinute, nil
	}

	count := int(incr.Val())
	remaining := limitPerMinute - count
	if remaining < 0 {
		remaining = 0
	}

	return count <= limitPerMinute, remaining, nil
}

// Set stores a key-value pair with optional TTL. Pass 0 for no expiry.
func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

// Get retrieves a cached value. Returns ("", redis.Nil) if not found.
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

// Del removes a key.
func (r *RedisClient) Del(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}
