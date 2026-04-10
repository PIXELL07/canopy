package service_test

import (
	"testing"
	"time"

	"github.com/pixell07/canopy/internal/auth"
	"github.com/pixell07/canopy/internal/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Health evaluation (pure, no DB)

func evaluateHealth(metrics []*models.Metrics, deploy *models.Deployment) *models.HealthReport {
	report := &models.HealthReport{
		DeploymentID: deploy.ID.Hex(),
		Version:      deploy.Version,
		GeneratedAt:  time.Now(),
		SampleCount:  len(metrics),
	}

	if len(metrics) == 0 {
		report.Recommendation = "wait"
		report.IsHealthy = true
		return report
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
	return report
}

func testDeploy() *models.Deployment {
	return &models.Deployment{
		ID:           primitive.NewObjectID(),
		Version:      "v2.0",
		MaxErrorRate: 0.05,
		MaxLatencyMs: 500,
	}
}

func metric(errRate float64, latencyMs int64, crashes int) *models.Metrics {
	return &models.Metrics{ErrorRate: errRate, LatencyMs: latencyMs, CrashCount: crashes, RequestCount: 1000}
}

func TestHealthy_ShouldPromote(t *testing.T) {
	report := evaluateHealth([]*models.Metrics{
		metric(0.01, 200, 0), metric(0.02, 210, 0), metric(0.01, 195, 0),
	}, testDeploy())
	if !report.IsHealthy {
		t.Errorf("expected healthy; error=%.3f latency=%.0f", report.AvgErrorRate, report.AvgLatencyMs)
	}
	if report.Recommendation != "promote" {
		t.Errorf("expected promote, got %s", report.Recommendation)
	}
}

func TestHighErrorRate_ShouldRollback(t *testing.T) {
	report := evaluateHealth([]*models.Metrics{metric(0.10, 200, 0), metric(0.12, 210, 0)}, testDeploy())
	if report.IsHealthy {
		t.Error("expected unhealthy (high error rate)")
	}
	if report.Recommendation != "rollback" {
		t.Errorf("expected rollback, got %s", report.Recommendation)
	}
}

func TestHighLatency_ShouldRollback(t *testing.T) {
	report := evaluateHealth([]*models.Metrics{metric(0.01, 800, 0), metric(0.01, 900, 0)}, testDeploy())
	if report.IsHealthy {
		t.Error("expected unhealthy (high latency)")
	}
}

func TestCrash_ShouldRollback(t *testing.T) {
	report := evaluateHealth([]*models.Metrics{metric(0.01, 200, 1)}, testDeploy())
	if report.IsHealthy {
		t.Error("expected unhealthy (crash)")
	}
	if len(report.Reasons) == 0 {
		t.Error("expected reasons to be populated")
	}
}

func TestNoMetrics_ShouldWait(t *testing.T) {
	report := evaluateHealth([]*models.Metrics{}, testDeploy())
	if report.Recommendation != "wait" {
		t.Errorf("expected wait, got %s", report.Recommendation)
	}
}

func TestAtExactThreshold_ShouldPromote(t *testing.T) {
	report := evaluateHealth([]*models.Metrics{metric(0.05, 500, 0)}, testDeploy())
	if !report.IsHealthy {
		t.Errorf("exact threshold should pass, got %s reasons=%v", report.Recommendation, report.Reasons)
	}
}

func TestAveragesCorrectly(t *testing.T) {
	report := evaluateHealth([]*models.Metrics{metric(0.02, 300, 0), metric(0.08, 300, 0)}, testDeploy())
	if report.AvgErrorRate != 0.05 {
		t.Errorf("expected avg 0.05, got %.4f", report.AvgErrorRate)
	}
	if report.SampleCount != 2 {
		t.Errorf("expected 2 samples, got %d", report.SampleCount)
	}
}

// RBAC tests

func makeClaims(role models.Role) *auth.Claims {
	return &auth.Claims{UserID: "test", Email: "test@test.com", Name: "Test", Role: role}
}

func TestRBAC_AdminCanDoEverything(t *testing.T) {
	claims := makeClaims(models.RoleAdmin)
	for _, role := range []models.Role{models.RoleAdmin, models.RoleDeployer, models.RoleViewer} {
		if err := auth.RequireRole(claims, role); err != nil {
			t.Errorf("admin should satisfy %s, got error: %v", role, err)
		}
	}
}

func TestRBAC_DeployerCannotAccessAdmin(t *testing.T) {
	claims := makeClaims(models.RoleDeployer)
	if err := auth.RequireRole(claims, models.RoleAdmin); err == nil {
		t.Error("deployer should not satisfy admin requirement")
	}
}

func TestRBAC_ViewerCannotDeploy(t *testing.T) {
	claims := makeClaims(models.RoleViewer)
	if err := auth.RequireRole(claims, models.RoleDeployer); err == nil {
		t.Error("viewer should not satisfy deployer requirement")
	}
	if err := auth.RequireRole(claims, models.RoleViewer); err != nil {
		t.Errorf("viewer should satisfy viewer requirement: %v", err)
	}
}

func TestRBAC_UnknownRoleHasNoAccess(t *testing.T) {
	claims := makeClaims(models.Role("ghost"))
	if err := auth.RequireRole(claims, models.RoleViewer); err == nil {
		t.Error("unknown role should not satisfy viewer")
	}
}
