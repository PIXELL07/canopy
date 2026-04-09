package service

import (
	"context"
	"time"

	"github.com/pixell07/canopy/internal/models"
	"github.com/pixell07/canopy/internal/notify"
	"github.com/pixell07/canopy/internal/repository"
	"go.uber.org/zap"
)

// WatcherService runs two background loops:
//  1. Every 30s — evaluate active canary deployments
//  2. Every 60s — mark stale servers offline
type WatcherService struct {
	canarySvc    *CanaryService
	healthSvc    *HealthService
	deployRepo   *repository.DeploymentRepo
	serverRepo   *repository.ServerRepo
	auditRepo    *repository.AuditRepo
	pool         *notify.Pool // bounded worker pool for webhook delivery
	log          *zap.Logger
	evalInterval time.Duration
	hbInterval   time.Duration
	hbThreshold  time.Duration
}

func NewWatcherService(
	cs *CanaryService,
	hs *HealthService,
	dr *repository.DeploymentRepo,
	sr *repository.ServerRepo,
	ar *repository.AuditRepo,
	pool *notify.Pool,
	log *zap.Logger,
	hbThreshold time.Duration,
) *WatcherService {
	return &WatcherService{
		canarySvc:    cs,
		healthSvc:    hs,
		deployRepo:   dr,
		serverRepo:   sr,
		auditRepo:    ar,
		pool:         pool,
		log:          log,
		evalInterval: 30 * time.Second,
		hbInterval:   60 * time.Second,
		hbThreshold:  hbThreshold,
	}
}

// Run starts both background loops. Cancel ctx to stop cleanly.
func (w *WatcherService) Run(ctx context.Context) {
	w.log.Info("watcher started",
		zap.Duration("eval_interval", w.evalInterval),
		zap.Duration("heartbeat_threshold", w.hbThreshold),
	)

	evalTicker := time.NewTicker(w.evalInterval)
	hbTicker := time.NewTicker(w.hbInterval)
	defer evalTicker.Stop()
	defer hbTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("watcher stopped")
			return
		case <-evalTicker.C:
			w.evaluateDeployments(ctx)
		case <-hbTicker.C:
			w.checkHeartbeats(ctx)
		}
	}
}

func (w *WatcherService) evaluateDeployments(ctx context.Context) {
	active, err := w.deployRepo.GetActive(ctx)
	if err != nil {
		w.log.Error("watcher: list active failed", zap.Error(err))
		return
	}

	for _, deploy := range active {
		id := deploy.ID.Hex()
		age := time.Since(deploy.CreatedAt)

		if age < deploy.MonitorDuration {
			w.log.Debug("watcher: still in monitor window",
				zap.String("deployment", id),
				zap.Duration("remaining", deploy.MonitorDuration-age),
			)
			continue
		}

		report, err := w.healthSvc.EvaluateDeployment(ctx, deploy, deploy.MonitorDuration)
		if err != nil {
			w.log.Error("watcher: health eval failed", zap.String("deployment", id), zap.Error(err))
			continue
		}

		w.log.Info("watcher: evaluation complete",
			zap.String("deployment", id),
			zap.String("version", deploy.Version),
			zap.Float64("avg_error_rate", report.AvgErrorRate),
			zap.Float64("avg_latency_ms", report.AvgLatencyMs),
			zap.Int("crashes", report.TotalCrashes),
			zap.String("recommendation", report.Recommendation),
			zap.Strings("reasons", report.Reasons),
		)

		switch report.Recommendation {
		case "promote":
			result, err := w.canarySvc.Promote(ctx, id, "system", "canopy-watcher", "internal")
			if err != nil {
				w.log.Error("watcher: auto-promote failed", zap.String("deployment", id), zap.Error(err))
				continue
			}
			w.log.Info("watcher: auto-promoted", zap.String("deployment", id))
			w.pool.Enqueue(context.Background(), models.EventDeployDone, map[string]interface{}{
				"deployment_id": id,
				"name":          result.Name,
				"version":       result.Version,
				"triggered_by":  "auto",
			})

		case "rollback":
			result, err := w.canarySvc.Rollback(ctx, id, "system", "canopy-watcher", "internal")
			if err != nil {
				w.log.Error("watcher: auto-rollback failed", zap.String("deployment", id), zap.Error(err))
				continue
			}
			w.log.Warn("watcher: auto-rolled back",
				zap.String("deployment", id),
				zap.Strings("reasons", report.Reasons),
			)
			w.pool.Enqueue(context.Background(), models.EventDeployRolledBack, map[string]interface{}{
				"deployment_id":  id,
				"name":           result.Name,
				"version":        result.Version,
				"rolled_back_to": result.PrevVersion,
				"reasons":        report.Reasons,
				"triggered_by":   "auto",
			})
		}
	}
}

func (w *WatcherService) checkHeartbeats(ctx context.Context) {
	staleThreshold := time.Now().Add(-w.hbThreshold)
	stale, err := w.serverRepo.GetStale(ctx, staleThreshold)
	if err != nil {
		w.log.Error("watcher: heartbeat check failed", zap.Error(err))
		return
	}

	for _, srv := range stale {
		sid := srv.ID.Hex()
		if err := w.serverRepo.UpdateStatus(ctx, sid, models.ServerOffline); err != nil {
			w.log.Error("watcher: mark offline failed", zap.String("server", sid), zap.Error(err))
			continue
		}

		w.log.Warn("watcher: server offline",
			zap.String("server", sid),
			zap.String("name", srv.Name),
			zap.Duration("silent_for", time.Since(srv.LastHeartbeat)),
		)

		_ = w.auditRepo.Append(ctx, &models.AuditEntry{
			Action:       models.AuditServerOffline,
			ActorID:      "system",
			ActorName:    "canopy-watcher",
			ResourceType: "server",
			ResourceID:   sid,
			Meta: map[string]interface{}{
				"host":        srv.Host,
				"last_seen":   srv.LastHeartbeat,
				"silent_secs": int(time.Since(srv.LastHeartbeat).Seconds()),
			},
			IPAddress: "internal",
		})

		w.pool.Enqueue(context.Background(), models.EventServerOffline, map[string]interface{}{
			"server_id":   sid,
			"server_name": srv.Name,
			"host":        srv.Host,
			"region":      srv.Region,
			"last_seen":   srv.LastHeartbeat,
		})
	}
}
