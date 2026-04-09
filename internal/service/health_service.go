package service

import (
	"context"
	"time"

	"github.com/pixell07/canopy/internal/models"
	"github.com/pixell07/canopy/internal/repository"
	"go.uber.org/zap"
)

type HealthService struct {
	metricsRepo *repository.MetricsRepo
	log         *zap.Logger
}

func NewHealthService(mr *repository.MetricsRepo, log *zap.Logger) *HealthService {
	return &HealthService{metricsRepo: mr, log: log}
}

func (h *HealthService) EvaluateDeployment(ctx context.Context, deploy *models.Deployment, window time.Duration) (*models.HealthReport, error) {
	since := time.Now().Add(-window)
	metrics, err := h.metricsRepo.GetSince(ctx, deploy.ID.Hex(), since)
	if err != nil {
		return nil, err
	}

	report := &models.HealthReport{
		DeploymentID: deploy.ID.Hex(),
		Version:      deploy.Version,
		GeneratedAt:  time.Now(),
		SampleCount:  len(metrics),
	}

	if len(metrics) == 0 {
		report.Recommendation = "wait"
		report.IsHealthy = true
		return report, nil
	}

	var totalError float64
	var totalLatency int64
	var totalRequests int64
	var totalCrashes int

	for _, m := range metrics {
		totalError += m.ErrorRate
		totalLatency += m.LatencyMs
		totalRequests += m.RequestCount
		totalCrashes += m.CrashCount
	}

	n := float64(len(metrics))
	report.AvgErrorRate = totalError / n
	report.AvgLatencyMs = float64(totalLatency) / n
	report.TotalRequests = totalRequests
	report.TotalCrashes = totalCrashes

	var reasons []string

	if report.TotalCrashes > 0 {
		reasons = append(reasons, "crashes detected")
	}
	if report.AvgErrorRate > deploy.MaxErrorRate {
		reasons = append(reasons, "error rate above threshold")
	}
	if report.AvgLatencyMs > float64(deploy.MaxLatencyMs) {
		reasons = append(reasons, "latency above threshold")
	}

	report.IsHealthy = len(reasons) == 0
	report.Reasons = reasons

	if report.IsHealthy {
		report.Recommendation = "promote"
	} else {
		report.Recommendation = "rollback"
	}

	h.log.Info("health evaluated",
		zap.String("deployment", deploy.ID.Hex()),
		zap.Bool("healthy", report.IsHealthy),
		zap.String("recommendation", report.Recommendation),
		zap.Strings("reasons", reasons),
	)
	return report, nil
}

func (h *HealthService) GetServerMetrics(ctx context.Context, serverID string) ([]*models.Metrics, error) {
	return h.metricsRepo.GetForServer(ctx, serverID, 100)
}
