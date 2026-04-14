package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/pixell07/canopy/internal/apierr"
	"github.com/pixell07/canopy/internal/repository"
	"go.uber.org/zap"
)

// StatusHandler provides a single endpoint that gives operators a full
// system snapshot: fleet health, active deployments, recent activity.
type StatusHandler struct {
	deployRepo *repository.DeploymentRepo
	serverRepo *repository.ServerRepo
	log        *zap.Logger
}

func NewStatusHandler(dr *repository.DeploymentRepo, sr *repository.ServerRepo, log *zap.Logger) *StatusHandler {
	return &StatusHandler{deployRepo: dr, serverRepo: sr, log: log}
}

// FleetSummary is the response shape for GET /status.
type FleetSummary struct {
	GeneratedAt       time.Time            `json:"generated_at"`
	Fleet             FleetStats           `json:"fleet"`
	ActiveDeployments []*DeploymentSummary `json:"active_deployments"`
}

type FleetStats struct {
	Total     int64 `json:"total"`
	Healthy   int64 `json:"healthy"`
	Unhealthy int64 `json:"unhealthy"`
	Offline   int64 `json:"offline"`
	Canary    int64 `json:"canary"`
}

type DeploymentSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Status        string `json:"status"`
	CanaryPercent int    `json:"canary_percent"`
	CanaryServers int    `json:"canary_servers"`
	AgeSeconds    int64  `json:"age_seconds"`
	CreatedBy     string `json:"created_by"`
}

// GET /status  (viewer+)
func (h *StatusHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Fetch all servers and active deployments concurrently
	type serverResult struct {
		servers []*repository.ServerStatusRow
		err     error
	}
	type deployResult struct {
		active []*repository.DeploymentRow // fix
		err    error
	}

	serverCh := make(chan serverResult, 1)
	deployCh := make(chan deployResult, 1)

	go func() {
		rows, err := h.serverRepo.GetStatusCounts(ctx)
		serverCh <- serverResult{servers: rows, err: err}
	}()

	go func() {
		active, err := h.deployRepo.GetActive(ctx)
		if err != nil {
			deployCh <- deployResult{err: err}
			return
		}
		rows := make([]*repository.DeploymentRow, 0, len(active))
		for _, d := range active {
			rows = append(rows, &repository.DeploymentRow{Deployment: d}) // fix
		}
		deployCh <- deployResult{active: rows}
	}()

	sr := <-serverCh
	dr := <-deployCh

	if sr.err != nil || dr.err != nil {
		h.log.Error("status fetch failed", zap.Any("server_err", sr.err), zap.Any("deploy_err", dr.err))
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}

	// Aggregate fleet stats
	stats := FleetStats{}
	for _, row := range sr.servers {
		stats.Total += row.Count
		switch row.Status {
		case "healthy":
			stats.Healthy = row.Count
		case "unhealthy":
			stats.Unhealthy = row.Count
		case "offline":
			stats.Offline = row.Count
		}
		if row.IsCanary {
			stats.Canary += row.Count
		}
	}

	// Build deployment summaries
	summaries := make([]*DeploymentSummary, 0, len(dr.active))
	for _, row := range dr.active {
		d := row.Deployment
		summaries = append(summaries, &DeploymentSummary{
			ID:            d.ID.Hex(),
			Name:          d.Name,
			Version:       d.Version,
			Status:        string(d.Status),
			CanaryPercent: d.CanaryPercent,
			CanaryServers: len(d.CanaryServerIDs),
			AgeSeconds:    int64(time.Since(d.CreatedAt).Seconds()),
			CreatedBy:     d.CreatedByName,
		})
	}

	respond(w, http.StatusOK, FleetSummary{
		GeneratedAt:       time.Now().UTC(),
		Fleet:             stats,
		ActiveDeployments: summaries,
	})
}
