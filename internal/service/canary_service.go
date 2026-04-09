package service

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/pixell07/canopy/internal/models"
	"github.com/pixell07/canopy/internal/repository"
	"go.uber.org/zap"
)

var (
	ErrNotEnoughServers = errors.New("not enough healthy servers to start canary")
	ErrDeploymentActive = errors.New("a deployment is already in progress")
	ErrInvalidPercent   = errors.New("canary_percent must be between 1 and 50")
)

type StartCanaryRequest struct {
	Name            string
	Version         string
	PrevVersion     string
	CanaryPercent   int
	MonitorDuration time.Duration
	MaxErrorRate    float64
	MaxLatencyMs    int64
	Notes           string
	ActorID         string
	ActorName       string
	IPAddress       string
}

type CanaryService struct {
	deployRepo  *repository.DeploymentRepo
	serverRepo  *repository.ServerRepo
	metricsRepo *repository.MetricsRepo
	auditRepo   *repository.AuditRepo
	log         *zap.Logger
}

func NewCanaryService(
	dr *repository.DeploymentRepo,
	sr *repository.ServerRepo,
	mr *repository.MetricsRepo,
	ar *repository.AuditRepo,
	log *zap.Logger,
) *CanaryService {
	return &CanaryService{deployRepo: dr, serverRepo: sr, metricsRepo: mr, auditRepo: ar, log: log}
}

func (s *CanaryService) DeployRepo() *repository.DeploymentRepo { return s.deployRepo }

func (s *CanaryService) StartCanary(ctx context.Context, req StartCanaryRequest) (*models.Deployment, error) {
	if req.CanaryPercent < 1 || req.CanaryPercent > 50 {
		return nil, ErrInvalidPercent
	}

	active, err := s.deployRepo.GetActive(ctx)
	if err != nil {
		return nil, err
	}
	if len(active) > 0 {
		return nil, ErrDeploymentActive
	}

	total, err := s.serverRepo.CountAll(ctx)
	if err != nil {
		return nil, err
	}
	canaryCount := int(math.Ceil(float64(total) * float64(req.CanaryPercent) / 100.0))
	if canaryCount < 1 {
		canaryCount = 1
	}

	canaryServers, err := s.serverRepo.GetNHealthyServers(ctx, canaryCount)
	if err != nil {
		return nil, err
	}
	if len(canaryServers) == 0 {
		return nil, ErrNotEnoughServers
	}

	deploy := &models.Deployment{
		Name:            req.Name,
		Version:         req.Version,
		PrevVersion:     req.PrevVersion,
		Status:          models.StatusCanary,
		CanaryPercent:   req.CanaryPercent,
		MonitorDuration: req.MonitorDuration,
		MaxErrorRate:    req.MaxErrorRate,
		MaxLatencyMs:    req.MaxLatencyMs,
		Notes:           req.Notes,
		CreatedByID:     req.ActorID,
		CreatedByName:   req.ActorName,
	}

	if err := s.deployRepo.Create(ctx, deploy); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(canaryServers))
	for _, srv := range canaryServers {
		sid := srv.ID.Hex()
		ids = append(ids, sid)
		if err := s.serverRepo.SetCanary(ctx, sid, deploy.ID.Hex(), req.Version, true); err != nil {
			s.log.Error("failed tagging canary server", zap.String("server", sid), zap.Error(err))
		}
	}
	_ = s.deployRepo.UpdateCanaryServers(ctx, deploy.ID.Hex(), ids)
	deploy.CanaryServerIDs = ids

	_ = s.auditRepo.Append(ctx, &models.AuditEntry{
		Action:       models.AuditDeployStart,
		ActorID:      req.ActorID,
		ActorName:    req.ActorName,
		ResourceType: "deployment",
		ResourceID:   deploy.ID.Hex(),
		Meta: map[string]interface{}{
			"version":        req.Version,
			"canary_percent": req.CanaryPercent,
			"canary_servers": ids,
		},
		IPAddress: req.IPAddress,
	})

	s.log.Info("canary started",
		zap.String("deployment", deploy.ID.Hex()),
		zap.String("version", req.Version),
		zap.Int("servers", len(ids)),
	)
	return deploy, nil
}

func (s *CanaryService) Promote(ctx context.Context, deploymentID, actorID, actorName, ip string) (*models.Deployment, error) {
	deploy, err := s.deployRepo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if err := s.serverRepo.PromoteAll(ctx, deploymentID, deploy.Version); err != nil {
		return nil, err
	}
	if err := s.deployRepo.MarkCompleted(ctx, deploymentID); err != nil {
		return nil, err
	}

	_ = s.auditRepo.Append(ctx, &models.AuditEntry{
		Action:       models.AuditDeployPromote,
		ActorID:      actorID,
		ActorName:    actorName,
		ResourceType: "deployment",
		ResourceID:   deploymentID,
		Meta:         map[string]interface{}{"version": deploy.Version},
		IPAddress:    ip,
	})

	deploy.Status = models.StatusCompleted
	s.log.Info("deployment promoted", zap.String("id", deploymentID), zap.String("version", deploy.Version))
	return deploy, nil
}

func (s *CanaryService) Rollback(ctx context.Context, deploymentID, actorID, actorName, ip string) (*models.Deployment, error) {
	deploy, err := s.deployRepo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if err := s.serverRepo.RollbackCanaries(ctx, deploymentID, deploy.PrevVersion); err != nil {
		return nil, err
	}
	if err := s.deployRepo.MarkRolledBack(ctx, deploymentID); err != nil {
		return nil, err
	}

	_ = s.auditRepo.Append(ctx, &models.AuditEntry{
		Action:       models.AuditDeployRollback,
		ActorID:      actorID,
		ActorName:    actorName,
		ResourceType: "deployment",
		ResourceID:   deploymentID,
		Meta:         map[string]interface{}{"rolled_back_to": deploy.PrevVersion},
		IPAddress:    ip,
	})

	deploy.Status = models.StatusRolledBack
	s.log.Warn("deployment rolled back", zap.String("id", deploymentID), zap.String("to", deploy.PrevVersion))
	return deploy, nil
}

func (s *CanaryService) GetDeployment(ctx context.Context, id string) (*models.Deployment, error) {
	return s.deployRepo.GetByID(ctx, id)
}

func (s *CanaryService) ListDeployments(ctx context.Context, limit, skip int64) ([]*models.Deployment, error) {
	return s.deployRepo.List(ctx, limit, skip)
}

func (s *CanaryService) RegisterServer(ctx context.Context, srv *models.Server, actorID, actorName, ip string) error {
	srv.Status = models.ServerHealthy
	if err := s.serverRepo.Create(ctx, srv); err != nil {
		return err
	}
	_ = s.auditRepo.Append(ctx, &models.AuditEntry{
		Action:       models.AuditServerRegister,
		ActorID:      actorID,
		ActorName:    actorName,
		ResourceType: "server",
		ResourceID:   srv.ID.Hex(),
		Meta:         map[string]interface{}{"host": srv.Host, "region": srv.Region},
		IPAddress:    ip,
	})
	return nil
}

func (s *CanaryService) ListServers(ctx context.Context) ([]*models.Server, error) {
	return s.serverRepo.List(ctx)
}

func (s *CanaryService) RecordHeartbeat(ctx context.Context, serverID string) error {
	return s.serverRepo.RecordHeartbeat(ctx, serverID)
}

func (s *CanaryService) RecordMetrics(ctx context.Context, m *models.Metrics) error {
	return s.metricsRepo.Record(ctx, m)
}
