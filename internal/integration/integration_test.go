package integration

import (
	"fmt"
	"net/http"
	"testing"
)

// Auth tests

func TestAuth_HealthPublic(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.get("/health", "")
	assertStatus(t, resp, http.StatusOK)
}

func TestAuth_LoginSuccess(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.post("/auth/login", "", map[string]string{
		"email": "admin@test.com", "password": "password123",
	})
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Token string `json:"token"`
	}
	mustDecode(t, resp, &body)
	if body.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestAuth_LoginWrongPassword(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.post("/auth/login", "", map[string]string{
		"email": "admin@test.com", "password": "wrong",
	})
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestAuth_Me(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.get("/auth/me", env.adminToken)
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	mustDecode(t, resp, &body)
	if body.Email != "admin@test.com" {
		t.Errorf("expected email admin@test.com, got %s", body.Email)
	}
	if body.Role != "admin" {
		t.Errorf("expected role admin, got %s", body.Role)
	}
}

func TestAuth_MeUnauthenticated(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.get("/auth/me", "")
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestAuth_Refresh(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.post("/auth/refresh", env.adminToken, nil)
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Token string `json:"token"`
	}
	mustDecode(t, resp, &body)
	if body.Token == "" {
		t.Error("expected refreshed token")
	}
}

// RBAC tests

func TestRBAC_ViewerCannotRegisterServer(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.post("/servers", env.viewerToken, map[string]interface{}{
		"name": "test-server", "host": "10.0.0.1",
	})
	assertStatus(t, resp, http.StatusForbidden)
}

func TestRBAC_ViewerCannotStartDeployment(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.post("/deployments", env.viewerToken, map[string]interface{}{
		"name": "test", "version": "v2.0",
	})
	assertStatus(t, resp, http.StatusForbidden)
}

func TestRBAC_ViewerCanListDeployments(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.get("/deployments", env.viewerToken)
	assertStatus(t, resp, http.StatusOK)
}

func TestRBAC_DeployerCannotAccessAuditLog(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.get("/audit?resource_id=test", env.deployToken)
	assertStatus(t, resp, http.StatusForbidden)
}

func TestRBAC_UnauthenticatedCannotAccessDeployments(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.get("/deployments", "")
	assertStatus(t, resp, http.StatusUnauthorized)
}

// Server tests

func TestServers_RegisterAndList(t *testing.T) {
	env := setupTestEnv(t)

	// Register a server as admin
	resp := env.post("/servers", env.adminToken, map[string]interface{}{
		"name": "server-01", "host": "10.0.0.1", "region": "us-east-1",
		"tags": []string{"app"}, "version": "v1.0",
	})
	assertStatus(t, resp, http.StatusCreated)

	var created struct {
		ID string `json:"id"`
	}
	mustDecode(t, resp, &created)
	if created.ID == "" {
		t.Fatal("expected server ID in response")
	}

	// List as viewer
	listResp := env.get("/servers", env.viewerToken)
	assertStatus(t, listResp, http.StatusOK)

	var servers []map[string]interface{}
	mustDecode(t, listResp, &servers)
	if len(servers) == 0 {
		t.Error("expected at least one server in list")
	}
}

func TestServers_Heartbeat(t *testing.T) {
	env := setupTestEnv(t)

	// Register first
	resp := env.post("/servers", env.adminToken, map[string]interface{}{
		"name": "hb-server", "host": "10.0.0.2", "version": "v1.0",
	})
	assertStatus(t, resp, http.StatusCreated)

	var srv struct {
		ID string `json:"id"`
	}
	mustDecode(t, resp, &srv)

	// Send heartbeat
	hbResp := env.post(fmt.Sprintf("/servers/%s/heartbeat", srv.ID), env.viewerToken, nil)
	assertStatus(t, hbResp, http.StatusOK)
}

func TestServers_RegisterValidation(t *testing.T) {
	env := setupTestEnv(t)

	// Missing host — should fail validation
	resp := env.post("/servers", env.adminToken, map[string]interface{}{
		"name": "bad-server",
	})
	assertStatus(t, resp, http.StatusBadRequest)

	var body struct {
		Err struct {
			Code    string `json:"code"`
			Details []struct {
				Field string `json:"field"`
			} `json:"details"`
		} `json:"error"`
	}
	mustDecode(t, resp, &body)
	if body.Err.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %s", body.Err.Code)
	}
}

// Deployment tests

func TestDeployments_StartRequiresServers(t *testing.T) {
	env := setupTestEnv(t)

	// No servers registered — should fail with 422
	resp := env.post("/deployments", env.deployToken, map[string]interface{}{
		"name": "test-release", "version": "v2.0", "prev_version": "v1.0",
		"canary_percent": 5, "monitor_seconds": 60,
	})
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

func TestDeployments_FullLifecycle(t *testing.T) {
	env := setupTestEnv(t)

	// Register 5 servers
	for i := 1; i <= 5; i++ {
		resp := env.post("/servers", env.adminToken, map[string]interface{}{
			"name":    fmt.Sprintf("server-%02d", i),
			"host":    fmt.Sprintf("10.0.1.%d", i),
			"version": "v1.0",
		})
		assertStatus(t, resp, http.StatusCreated)
	}

	// Start canary deployment
	resp := env.post("/deployments", env.deployToken, map[string]interface{}{
		"name": "lifecycle-test", "version": "v2.0", "prev_version": "v1.0",
		"canary_percent": 20, "monitor_seconds": 30,
		"max_error_rate": 0.05, "max_latency_ms": 500,
	})
	assertStatus(t, resp, http.StatusCreated)

	var deploy struct {
		ID            string   `json:"id"`
		Status        string   `json:"status"`
		CanaryPercent int      `json:"canary_percent"`
		CanaryServers []string `json:"canary_server_ids"`
	}
	mustDecode(t, resp, &deploy)

	if deploy.Status != "canary" {
		t.Errorf("expected status 'canary', got '%s'", deploy.Status)
	}
	if deploy.CanaryPercent != 20 {
		t.Errorf("expected canary_percent 20, got %d", deploy.CanaryPercent)
	}
	if len(deploy.CanaryServers) == 0 {
		t.Error("expected at least one canary server")
	}

	// Cannot start another while one is active
	resp2 := env.post("/deployments", env.deployToken, map[string]interface{}{
		"name": "concurrent", "version": "v3.0",
	})
	assertStatus(t, resp2, http.StatusConflict)

	// Get the deployment
	getResp := env.get("/deployments/"+deploy.ID, env.viewerToken)
	assertStatus(t, getResp, http.StatusOK)

	// Promote it
	promoteResp := env.post("/deployments/"+deploy.ID+"/promote", env.deployToken, nil)
	assertStatus(t, promoteResp, http.StatusOK)

	var promoted struct {
		Status string `json:"status"`
	}
	mustDecode(t, promoteResp, &promoted)
	if promoted.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", promoted.Status)
	}
}

func TestDeployments_Rollback(t *testing.T) {
	env := setupTestEnv(t)

	// Register servers
	for i := 1; i <= 3; i++ {
		env.post("/servers", env.adminToken, map[string]interface{}{
			"name":    fmt.Sprintf("rb-server-%d", i),
			"host":    fmt.Sprintf("10.0.2.%d", i),
			"version": "v1.0",
		})
	}

	// Start canary
	resp := env.post("/deployments", env.deployToken, map[string]interface{}{
		"name": "rollback-test", "version": "v2.0", "prev_version": "v1.0",
		"canary_percent": 33, "monitor_seconds": 30,
	})
	assertStatus(t, resp, http.StatusCreated)

	var deploy struct {
		ID string `json:"id"`
	}
	mustDecode(t, resp, &deploy)

	// Rollback
	rbResp := env.post("/deployments/"+deploy.ID+"/rollback", env.deployToken, nil)
	assertStatus(t, rbResp, http.StatusOK)

	var rolled struct {
		Status string `json:"status"`
	}
	mustDecode(t, rbResp, &rolled)
	if rolled.Status != "rolled_back" {
		t.Errorf("expected 'rolled_back', got '%s'", rolled.Status)
	}
}

func TestDeployments_ValidationErrors(t *testing.T) {
	env := setupTestEnv(t)

	tests := []struct {
		name      string
		body      map[string]interface{}
		wantField string
	}{
		{
			name:      "missing name",
			body:      map[string]interface{}{"version": "v2.0"},
			wantField: "name",
		},
		{
			name:      "canary percent too high",
			body:      map[string]interface{}{"name": "test", "version": "v2.0", "canary_percent": 99},
			wantField: "canary_percent",
		},
		{
			name:      "invalid error rate",
			body:      map[string]interface{}{"name": "test", "version": "v2.0", "max_error_rate": 2.0},
			wantField: "max_error_rate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := env.post("/deployments", env.deployToken, tt.body)
			assertStatus(t, resp, http.StatusBadRequest)

			var body struct {
				Err struct {
					Code    string `json:"code"`
					Details []struct {
						Field string `json:"field"`
					} `json:"details"`
				} `json:"error"`
			}
			mustDecode(t, resp, &body)
			if body.Err.Code != "VALIDATION_ERROR" {
				t.Errorf("expected VALIDATION_ERROR, got %s", body.Err.Code)
			}
			found := false
			for _, d := range body.Err.Details {
				if d.Field == tt.wantField {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected field '%s' in error details, got %+v", tt.wantField, body.Err.Details)
			}
		})
	}
}

// Metrics tests

func TestMetrics_Ingest(t *testing.T) {
	env := setupTestEnv(t)

	// Register a server and start a deployment so we have valid IDs
	srvResp := env.post("/servers", env.adminToken, map[string]interface{}{
		"name": "metrics-server", "host": "10.0.3.1", "version": "v1.0",
	})
	var srv struct {
		ID string `json:"id"`
	}
	mustDecode(t, srvResp, &srv)

	for i := 0; i < 4; i++ {
		env.post("/servers", env.adminToken, map[string]interface{}{
			"name": fmt.Sprintf("ms-%d", i), "host": fmt.Sprintf("10.0.3.%d", i+2), "version": "v1.0",
		})
	}

	depResp := env.post("/deployments", env.deployToken, map[string]interface{}{
		"name": "metrics-test", "version": "v2.0", "prev_version": "v1.0",
		"canary_percent": 20, "monitor_seconds": 60,
	})
	var dep struct {
		ID string `json:"id"`
	}
	mustDecode(t, depResp, &dep)

	// Ingest valid metrics
	resp := env.post("/metrics", env.viewerToken, map[string]interface{}{
		"server_id": srv.ID, "deployment_id": dep.ID,
		"version": "v2.0", "error_rate": 0.02,
		"latency_ms": 150, "request_count": 1000,
		"crash_count": 0,
	})
	assertStatus(t, resp, http.StatusAccepted)

	// Invalid error_rate
	badResp := env.post("/metrics", env.viewerToken, map[string]interface{}{
		"server_id": srv.ID, "deployment_id": dep.ID,
		"error_rate": 1.5, "latency_ms": 100,
	})
	assertStatus(t, badResp, http.StatusBadRequest)
}

// Status tests

func TestStatus_Get(t *testing.T) {
	env := setupTestEnv(t)

	// Register some servers so stats are non-trivial
	for i := 0; i < 3; i++ {
		env.post("/servers", env.adminToken, map[string]interface{}{
			"name":    fmt.Sprintf("stat-srv-%d", i),
			"host":    fmt.Sprintf("10.0.4.%d", i),
			"version": "v1.0",
		})
	}

	resp := env.get("/status", env.viewerToken)
	assertStatus(t, resp, http.StatusOK)

	var status struct {
		Fleet struct {
			Total   int64 `json:"total"`
			Healthy int64 `json:"healthy"`
		} `json:"fleet"`
		ActiveDeployments []interface{} `json:"active_deployments"`
	}
	mustDecode(t, resp, &status)

	if status.Fleet.Total < 3 {
		t.Errorf("expected at least 3 total servers, got %d", status.Fleet.Total)
	}
}

// Webhook tests

func TestWebhooks_CreateAndList(t *testing.T) {
	env := setupTestEnv(t)

	// Create webhook (admin)
	resp := env.post("/webhooks", env.adminToken, map[string]interface{}{
		"name":   "test-hook",
		"url":    "https://example.com/webhook",
		"secret": "test-secret",
		"events": []string{"deployment.started", "deployment.rolled_back"},
	})
	assertStatus(t, resp, http.StatusCreated)

	// List (viewer can read)
	listResp := env.get("/webhooks", env.viewerToken)
	assertStatus(t, listResp, http.StatusOK)

	var hooks []map[string]interface{}
	mustDecode(t, listResp, &hooks)
	if len(hooks) == 0 {
		t.Error("expected at least one webhook")
	}
}

func TestWebhooks_DeployerCannotCreate(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.post("/webhooks", env.deployToken, map[string]interface{}{
		"name": "test", "url": "https://example.com/wh",
		"events": []string{"deployment.started"},
	})
	assertStatus(t, resp, http.StatusForbidden)
}
