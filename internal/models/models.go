package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Roles

type Role string

const (
	RoleAdmin    Role = "admin"    // full access
	RoleDeployer Role = "deployer" // start/promote/rollback
	RoleViewer   Role = "viewer"   // read-only
)

// User

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"  json:"id"`
	Name         string             `bson:"name"           json:"name"`
	Email        string             `bson:"email"          json:"email"`
	PasswordHash string             `bson:"password_hash"  json:"-"`
	Role         Role               `bson:"role"           json:"role"`
	APIKey       string             `bson:"api_key"        json:"-"`
	Active       bool               `bson:"active"         json:"active"`
	CreatedAt    time.Time          `bson:"created_at"     json:"created_at"`
	LastLoginAt  *time.Time         `bson:"last_login_at"  json:"last_login_at,omitempty"`
}

// Deployment

type DeploymentStatus string

const (
	StatusPending    DeploymentStatus = "pending"
	StatusCanary     DeploymentStatus = "canary"
	StatusMonitoring DeploymentStatus = "monitoring"
	StatusRollingOut DeploymentStatus = "rolling_out"
	StatusCompleted  DeploymentStatus = "completed"
	StatusRolledBack DeploymentStatus = "rolled_back"
	StatusFailed     DeploymentStatus = "failed"
)

type Deployment struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"     json:"id"`
	Name            string             `bson:"name"              json:"name"`
	Version         string             `bson:"version"           json:"version"`
	PrevVersion     string             `bson:"prev_version"      json:"prev_version"`
	Status          DeploymentStatus   `bson:"status"            json:"status"`
	CanaryPercent   int                `bson:"canary_percent"    json:"canary_percent"`
	CanaryServerIDs []string           `bson:"canary_server_ids" json:"canary_server_ids"`
	MonitorDuration time.Duration      `bson:"monitor_duration"  json:"monitor_duration"`
	MaxErrorRate    float64            `bson:"max_error_rate"    json:"max_error_rate"`
	MaxLatencyMs    int64              `bson:"max_latency_ms"    json:"max_latency_ms"`
	CreatedByID     string             `bson:"created_by_id"     json:"created_by_id"`
	CreatedByName   string             `bson:"created_by_name"   json:"created_by_name"`
	CreatedAt       time.Time          `bson:"created_at"        json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at"        json:"updated_at"`
	CompletedAt     *time.Time         `bson:"completed_at"      json:"completed_at,omitempty"`
	RollbackAt      *time.Time         `bson:"rollback_at"       json:"rollback_at,omitempty"`
	Notes           string             `bson:"notes"             json:"notes,omitempty"`
}

// Server

type ServerStatus string

const (
	ServerHealthy   ServerStatus = "healthy"
	ServerUnhealthy ServerStatus = "unhealthy"
	ServerDraining  ServerStatus = "draining"
	ServerOffline   ServerStatus = "offline"
)

type Server struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"   json:"id"`
	Name           string             `bson:"name"            json:"name"`
	Host           string             `bson:"host"            json:"host"`
	Region         string             `bson:"region"          json:"region"`
	Tags           []string           `bson:"tags"            json:"tags"`
	CurrentVersion string             `bson:"current_version" json:"current_version"`
	Status         ServerStatus       `bson:"status"          json:"status"`
	IsCanary       bool               `bson:"is_canary"       json:"is_canary"`
	DeploymentID   string             `bson:"deployment_id"   json:"deployment_id,omitempty"`
	LastHeartbeat  time.Time          `bson:"last_heartbeat"  json:"last_heartbeat"`
	CreatedAt      time.Time          `bson:"created_at"      json:"created_at"`
}

// IsStale returns true if the server hasn't reported within the given threshold.
func (s *Server) IsStale(threshold time.Duration) bool {
	return time.Since(s.LastHeartbeat) > threshold
}

// Metrics

type Metrics struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ServerID     string             `bson:"server_id"     json:"server_id"`
	DeploymentID string             `bson:"deployment_id" json:"deployment_id"`
	Version      string             `bson:"version"       json:"version"`
	ErrorRate    float64            `bson:"error_rate"    json:"error_rate"`
	LatencyMs    int64              `bson:"latency_ms"    json:"latency_ms"`
	RequestCount int64              `bson:"request_count" json:"request_count"`
	CrashCount   int                `bson:"crash_count"   json:"crash_count"`
	MemUsageMB   float64            `bson:"mem_usage_mb"  json:"mem_usage_mb"`
	CPUPercent   float64            `bson:"cpu_percent"   json:"cpu_percent"`
	RecordedAt   time.Time          `bson:"recorded_at"   json:"recorded_at"`
}

// Health Report

type HealthReport struct {
	DeploymentID   string    `json:"deployment_id"`
	Version        string    `json:"version"`
	AvgErrorRate   float64   `json:"avg_error_rate"`
	AvgLatencyMs   float64   `json:"avg_latency_ms"`
	TotalRequests  int64     `json:"total_requests"`
	TotalCrashes   int       `json:"total_crashes"`
	SampleCount    int       `json:"sample_count"`
	IsHealthy      bool      `json:"is_healthy"`
	Recommendation string    `json:"recommendation"` // "promote" | "rollback" | "wait"
	Reasons        []string  `json:"reasons,omitempty"`
	GeneratedAt    time.Time `json:"generated_at"`
}

// Audit Log

type AuditAction string

const (
	AuditDeployStart    AuditAction = "deploy.start"
	AuditDeployPromote  AuditAction = "deploy.promote"
	AuditDeployRollback AuditAction = "deploy.rollback"
	AuditServerRegister AuditAction = "server.register"
	AuditServerOffline  AuditAction = "server.offline"
	AuditAutoPromote    AuditAction = "auto.promote"
	AuditAutoRollback   AuditAction = "auto.rollback"
	AuditUserLogin      AuditAction = "user.login"
	AuditUserCreate     AuditAction = "user.create"
)

type AuditEntry struct {
	ID           primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	Action       AuditAction            `bson:"action"        json:"action"`
	ActorID      string                 `bson:"actor_id"      json:"actor_id"`
	ActorName    string                 `bson:"actor_name"    json:"actor_name"`
	ResourceType string                 `bson:"resource_type" json:"resource_type"`
	ResourceID   string                 `bson:"resource_id"   json:"resource_id"`
	Meta         map[string]interface{} `bson:"meta"          json:"meta,omitempty"`
	IPAddress    string                 `bson:"ip_address"    json:"ip_address"`
	CreatedAt    time.Time              `bson:"created_at"    json:"created_at"`
}

// Webhook

type WebhookEvent string

const (
	EventDeployStarted    WebhookEvent = "deployment.started"
	EventDeployDone       WebhookEvent = "deployment.completed"
	EventDeployRolledBack WebhookEvent = "deployment.rolled_back"
	EventServerOffline    WebhookEvent = "server.offline"
)

type Webhook struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name"          json:"name"`
	URL       string             `bson:"url"           json:"url"`
	Secret    string             `bson:"secret"        json:"-"`
	Events    []WebhookEvent     `bson:"events"        json:"events"`
	Active    bool               `bson:"active"        json:"active"`
	CreatedAt time.Time          `bson:"created_at"    json:"created_at"`
}
