package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pixell07/canopy/config"
	"github.com/pixell07/canopy/internal/api/handlers"
	"github.com/pixell07/canopy/internal/auth"
	"github.com/pixell07/canopy/internal/middleware"
	"github.com/pixell07/canopy/internal/notify"
	"github.com/pixell07/canopy/internal/observability"
	"github.com/pixell07/canopy/internal/repository"
	"github.com/pixell07/canopy/internal/router"
	"github.com/pixell07/canopy/internal/service"
	"go.uber.org/zap"
)

func main() {
	// Logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Config
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("config error", zap.Error(err))
	}

	// MongoDB
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connectCancel()

	db, err := repository.NewMongoClient(connectCtx, cfg.MongoURI, cfg.DBName)
	if err != nil {
		logger.Fatal("mongodb connect failed", zap.Error(err))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = db.Disconnect(ctx)
	}()
	logger.Info("mongodb connected", zap.String("db", cfg.DBName))

	// Redis (non-critical)
	var redis *repository.RedisClient
	if r, err := repository.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword); err != nil {
		logger.Warn("redis unavailable — rate limiting disabled", zap.Error(err))
	} else {
		redis = r
		defer redis.Close()
		logger.Info("redis connected", zap.String("addr", cfg.RedisAddr))
	}

	// Repositories
	userRepo := repository.NewUserRepo(db)
	deployRepo := repository.NewDeploymentRepo(db)
	serverRepo := repository.NewServerRepo(db)
	metricsRepo := repository.NewMetricsRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	webhookRepo := repository.NewWebhookRepo(db)

	// Auth
	authSvc := auth.NewService(cfg.JWTSecret, cfg.JWTTokenTTL)

	// Observability
	metrics := observability.NewMetrics()

	// Webhook pool (10 workers, 200 job buffer)
	notifier := notify.NewWebhookNotifier(webhookRepo, logger)
	pool := notify.NewPool(notifier, 10, 200, logger)
	defer pool.Stop() // drains on shutdown

	// Services
	userSvc := service.NewUserService(userRepo, auditRepo, authSvc, logger)
	canarySvc := service.NewCanaryService(deployRepo, serverRepo, metricsRepo, auditRepo, logger)
	healthSvc := service.NewHealthService(metricsRepo, logger)
	watcherSvc := service.NewWatcherService(
		canarySvc, healthSvc,
		deployRepo, serverRepo, auditRepo,
		pool, logger, cfg.HeartbeatStale,
	)

	// Background watcher
	watcherCtx, watcherCancel := context.WithCancel(context.Background())
	defer watcherCancel()
	go watcherSvc.Run(watcherCtx)

	// Middleware
	mw := middleware.New(authSvc, userRepo, redis, metrics, logger, cfg.RateLimitRPM)

	// Handlers
	authHandler := handlers.NewAuthHandler(userSvc, metrics, logger)
	deployHandler := handlers.NewDeploymentHandler(canarySvc, metrics, logger)
	serverHandler := handlers.NewServerHandler(canarySvc, logger)
	metricsHandler := handlers.NewMetricsHandler(canarySvc, healthSvc, logger)
	auditHandler := handlers.NewAuditHandler(auditRepo, logger)
	webhookHandler := handlers.NewWebhookHandler(webhookRepo, logger)
	statusHandler := handlers.NewStatusHandler(deployRepo, serverRepo, logger)
	healthHandler := handlers.NewHealthHandler()

	// Router
	r := router.New(mw,
		authHandler, deployHandler, serverHandler,
		metricsHandler, auditHandler, webhookHandler,
		statusHandler, healthHandler,
	)

	// HTTP Server
	srv := &http.Server{
		Addr:         cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("canopy started",
			zap.String("addr", cfg.Port),
			zap.String("env", cfg.Env),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("shutdown signal received", zap.String("signal", sig.String()))
	watcherCancel() // stop watcher first

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Fatal("forced shutdown", zap.Error(err))
	}
	// pool.Stop() called via defer — drains in-flight webhooks
	logger.Info("canopy stopped cleanly")
}
