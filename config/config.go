package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	MongoURI       string
	DBName         string
	RedisAddr      string
	RedisPassword  string
	JWTSecret      string
	JWTTokenTTL    time.Duration
	Env            string
	HeartbeatStale time.Duration // how long before server is considered offline
	RateLimitRPM   int           // requests per minute per API key
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:           getEnv("PORT", ":8080"),
		MongoURI:       getEnv("MONGO_URI", "mongodb://localhost:27017"),
		DBName:         getEnv("DB_NAME", "canopy"),
		RedisAddr:      getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:  getEnv("REDIS_PASSWORD", ""),
		JWTSecret:      getEnv("JWT_SECRET", "change-me-in-production-use-32-chars"),
		Env:            getEnv("APP_ENV", "development"),
		HeartbeatStale: 90 * time.Second,
		RateLimitRPM:   120,
	}

	if ttlStr := os.Getenv("JWT_TTL_HOURS"); ttlStr != "" {
		var h int
		if _, err := fmt.Sscanf(ttlStr, "%d", &h); err == nil {
			cfg.JWTTokenTTL = time.Duration(h) * time.Hour
		}
	}
	if cfg.JWTTokenTTL == 0 {
		cfg.JWTTokenTTL = 24 * time.Hour
	}

	if cfg.MongoURI == "" {
		return nil, fmt.Errorf("MONGO_URI is required")
	}
	if cfg.JWTSecret == "change-me-in-production-use-32-chars" && cfg.Env == "production" {
		return nil, fmt.Errorf("JWT_SECRET must be set in production")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
