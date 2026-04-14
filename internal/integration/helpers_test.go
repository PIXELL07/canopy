// Package integration contains end-to-end tests that spin up a real HTTP
// server backed by a real MongoDB instance (provided via MONGO_URI env var).
// Run with: go test ./internal/integration/... -v -tags integration
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/pixell07/canopy/config"
	"github.com/pixell07/canopy/internal/api/handlers"
	"github.com/pixell07/canopy/internal/auth"
	"github.com/pixell07/canopy/internal/middleware"
	"github.com/pixell07/canopy/internal/models"
	"github.com/pixell07/canopy/internal/notify"
	"github.com/pixell07/canopy/internal/observability"
	"github.com/pixell07/canopy/internal/repository"
	"github.com/pixell07/canopy/internal/router"
	"github.com/pixell07/canopy/internal/service"
	"go.uber.org/zap"
)

// testEnv holds everything needed for integration tests.
type testEnv struct {
	server      *httptest.Server
	db          *repository.DB
	adminToken  string
	deployToken string
	viewerToken string
	t           *testing.T
}

// setupTestEnv creates a full in-process HTTP server wired to a real MongoDB.
// The database is named canopy_test_{timestamp} to avoid collisions.
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	dbName := "canopy_test_" + time.Now().Format("20060102150405")

	cfg := &config.Config{
		Port:           ":0",
		MongoURI:       mongoURI,
		DBName:         dbName,
		JWTSecret:      "integration-test-secret-32chars!!",
		JWTTokenTTL:    time.Hour,
		HeartbeatStale: 90 * time.Second,
		RateLimitRPM:   1000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := repository.NewMongoClient(ctx, cfg.MongoURI, cfg.DBName)
	if err != nil {
		t.Skipf("MongoDB unavailable (%v) — skipping integration tests", err)
	}

	logger := zap.NewNop()

	userRepo := repository.NewUserRepo(db)
	deployRepo := repository.NewDeploymentRepo(db)
	serverRepo := repository.NewServerRepo(db)
	metricsRepo := repository.NewMetricsRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	webhookRepo := repository.NewWebhookRepo(db)

	authSvc := auth.NewService(cfg.JWTSecret, cfg.JWTTokenTTL)
	obs := observability.NewMetrics()
	notifier := notify.NewWebhookNotifier(webhookRepo, logger)
	pool := notify.NewPool(notifier, 2, 10, logger)

	userSvc := service.NewUserService(userRepo, auditRepo, authSvc, logger)
	canarySvc := service.NewCanaryService(deployRepo, serverRepo, metricsRepo, auditRepo, logger)
	healthSvc := service.NewHealthService(metricsRepo, logger)

	mw := middleware.New(authSvc, userRepo, nil, obs, logger, cfg.RateLimitRPM)

	authH := handlers.NewAuthHandler(userSvc, obs, logger)
	deployH := handlers.NewDeploymentHandler(canarySvc, obs, logger)
	serverH := handlers.NewServerHandler(canarySvc, logger)
	metricsH := handlers.NewMetricsHandler(canarySvc, healthSvc, logger)
	auditH := handlers.NewAuditHandler(auditRepo, logger)
	webhookH := handlers.NewWebhookHandler(webhookRepo, logger)
	statusH := handlers.NewStatusHandler(deployRepo, serverRepo, logger)
	healthH := handlers.NewHealthHandler()

	r := router.New(mw, authH, deployH, serverH, metricsH, auditH, webhookH, statusH, healthH)
	srv := httptest.NewServer(r)

	env := &testEnv{server: srv, db: db, t: t}

	// Create users for each role
	env.adminToken = env.mustCreateUserAndLogin("Admin User", "admin@test.com", "password123", models.RoleAdmin)
	env.deployToken = env.mustCreateUserAndLogin("Deploy User", "deploy@test.com", "password123", models.RoleDeployer)
	env.viewerToken = env.mustCreateUserAndLogin("View User", "viewer@test.com", "password123", models.RoleViewer)

	t.Cleanup(func() {
		srv.Close()
		pool.Stop()
		dropCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = db.Database.Drop(dropCtx)
		_ = db.Disconnect(dropCtx)
	})

	return env
}

// mustCreateUserAndLogin creates a user via the service layer and returns a JWT.
func (e *testEnv) mustCreateUserAndLogin(name, email, password string, role models.Role) string {
	e.t.Helper()

	// Register directly via service to bypass HTTP (avoids chicken-and-egg auth problem)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(e.db)
	auditRepo := repository.NewAuditRepo(e.db)
	authSvc := auth.NewService("integration-test-secret-32chars!!", time.Hour)
	userSvc := service.NewUserService(userRepo, auditRepo, authSvc, zap.NewNop())

	_, err := userSvc.Register(ctx, service.RegisterRequest{
		Name: name, Email: email, Password: password, Role: role,
	}, "test", "test", "127.0.0.1")
	if err != nil && err != service.ErrEmailTaken {
		e.t.Fatalf("failed to create user %s: %v", email, err)
	}

	// Login via HTTP to get a real JWT
	resp := e.post("/auth/login", "", map[string]string{
		"email": email, "password": password,
	})
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("login failed for %s: status %d", email, resp.StatusCode)
	}

	var body struct {
		Token string `json:"token"`
	}
	mustDecode(e.t, resp, &body)
	return body.Token
}

// HTTP helpers

func (e *testEnv) get(path, token string) *http.Response {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("GET %s failed: %v", path, err)
	}
	return resp
}

func (e *testEnv) post(path, token string, body interface{}) *http.Response {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, e.server.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("POST %s failed: %v", path, err)
	}
	return resp
}

func (e *testEnv) delete(path, token string) *http.Response {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, e.server.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("DELETE %s failed: %v", path, err)
	}
	return resp
}

func mustDecode(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Errorf("expected status %d, got %d", want, resp.StatusCode)
	}
}
