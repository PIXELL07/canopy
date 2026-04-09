package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pixell07/canopy/internal/apierr"
	"github.com/pixell07/canopy/internal/auth"
	"github.com/pixell07/canopy/internal/models"
	"github.com/pixell07/canopy/internal/observability"
	"github.com/pixell07/canopy/internal/repository"
	"github.com/pixell07/canopy/internal/service"
	"github.com/pixell07/canopy/internal/validate"
	"go.uber.org/zap"
)

// Auth

type AuthHandler struct {
	userSvc *service.UserService
	metrics *observability.Metrics
	log     *zap.Logger
}

func NewAuthHandler(us *service.UserService, m *observability.Metrics, log *zap.Logger) *AuthHandler {
	return &AuthHandler{userSvc: us, metrics: m, log: log}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		apierr.BadRequest("invalid JSON body").Write(w, http.StatusBadRequest)
		return
	}
	errs := validate.Collect(
		validate.Required("email", body.Email),
		validate.Required("password", body.Password),
	)
	if len(errs) > 0 {
		apierr.Validation(errs...).Write(w, http.StatusBadRequest)
		return
	}
	result, err := h.userSvc.Login(r.Context(), body.Email, body.Password, realIP(r))
	if err != nil {
		h.metrics.LoginAttempts.WithLabelValues("failure").Inc()
		apierr.Unauthorized("invalid email or password").Write(w, http.StatusUnauthorized)
		return
	}
	h.metrics.LoginAttempts.WithLabelValues("success").Inc()
	respond(w, http.StatusOK, result)
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	var body struct {
		Name     string      `json:"name"`
		Email    string      `json:"email"`
		Password string      `json:"password"`
		Role     models.Role `json:"role"`
	}
	if err := decode(r, &body); err != nil {
		apierr.BadRequest("invalid JSON body").Write(w, http.StatusBadRequest)
		return
	}
	errs := validate.Collect(
		validate.Required("name", body.Name),
		validate.MaxLen("name", body.Name, 100),
		validate.Required("email", body.Email),
		validate.Email("email", body.Email),
		validate.Required("password", body.Password),
		validate.MinLen("password", body.Password, 8),
	)
	if len(errs) > 0 {
		apierr.Validation(errs...).Write(w, http.StatusBadRequest)
		return
	}
	actorID, actorName := "system", "system"
	if claims != nil {
		actorID, actorName = claims.UserID, claims.Name
	}
	user, err := h.userSvc.Register(r.Context(), service.RegisterRequest{
		Name: body.Name, Email: body.Email, Password: body.Password, Role: body.Role,
	}, actorID, actorName, realIP(r))
	if err != nil {
		switch err {
		case service.ErrEmailTaken:
			apierr.Conflict("email already registered").Write(w, http.StatusConflict)
		default:
			h.log.Error("register failed", zap.Error(err))
			apierr.Internal().Write(w, http.StatusInternalServerError)
		}
		return
	}
	respond(w, http.StatusCreated, user)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w, http.StatusUnauthorized)
		return
	}
	respond(w, http.StatusOK, map[string]interface{}{
		"user_id": claims.UserID, "email": claims.Email,
		"name": claims.Name, "role": claims.Role,
	})
}

// Deployment

type DeploymentHandler struct {
	svc     *service.CanaryService
	metrics *observability.Metrics
	log     *zap.Logger
}

func NewDeploymentHandler(svc *service.CanaryService, m *observability.Metrics, log *zap.Logger) *DeploymentHandler {
	return &DeploymentHandler{svc: svc, metrics: m, log: log}
}

func (h *DeploymentHandler) Start(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	var body struct {
		Name           string  `json:"name"`
		Version        string  `json:"version"`
		PrevVersion    string  `json:"prev_version"`
		CanaryPercent  int     `json:"canary_percent"`
		MonitorSeconds int     `json:"monitor_seconds"`
		MaxErrorRate   float64 `json:"max_error_rate"`
		MaxLatencyMs   int64   `json:"max_latency_ms"`
		Notes          string  `json:"notes"`
	}
	if err := decode(r, &body); err != nil {
		apierr.BadRequest("invalid JSON body").Write(w, http.StatusBadRequest)
		return
	}
	if body.CanaryPercent == 0 {
		body.CanaryPercent = 5
	}
	if body.MonitorSeconds == 0 {
		body.MonitorSeconds = 600
	}
	if body.MaxErrorRate == 0 {
		body.MaxErrorRate = 0.05
	}
	if body.MaxLatencyMs == 0 {
		body.MaxLatencyMs = 500
	}

	errs := validate.Collect(
		validate.Required("name", body.Name),
		validate.MaxLen("name", body.Name, 100),
		validate.Required("version", body.Version),
		validate.MaxLen("version", body.Version, 50),
		validate.InRange("canary_percent", body.CanaryPercent, 1, 50),
		validate.InRange("monitor_seconds", body.MonitorSeconds, 30, 86400),
		validate.FloatRange("max_error_rate", body.MaxErrorRate, 0.001, 1.0),
		validate.Positive("max_latency_ms", body.MaxLatencyMs),
	)
	if len(errs) > 0 {
		apierr.Validation(errs...).Write(w, http.StatusBadRequest)
		return
	}
	deploy, err := h.svc.StartCanary(r.Context(), service.StartCanaryRequest{
		Name: body.Name, Version: body.Version, PrevVersion: body.PrevVersion,
		CanaryPercent:   body.CanaryPercent,
		MonitorDuration: time.Duration(body.MonitorSeconds) * time.Second,
		MaxErrorRate:    body.MaxErrorRate, MaxLatencyMs: body.MaxLatencyMs,
		Notes: body.Notes, ActorID: claims.UserID,
		ActorName: claims.Name, IPAddress: realIP(r),
	})
	if err != nil {
		switch err {
		case service.ErrInvalidPercent:
			apierr.BadRequest(err.Error()).Write(w, http.StatusBadRequest)
		case service.ErrDeploymentActive:
			apierr.Conflict(err.Error()).Write(w, http.StatusConflict)
		case service.ErrNotEnoughServers:
			apierr.Unprocessable(err.Error()).Write(w, http.StatusUnprocessableEntity)
		default:
			h.log.Error("start canary failed", zap.Error(err))
			apierr.Internal().Write(w, http.StatusInternalServerError)
		}
		return
	}
	h.metrics.DeploymentsStarted.Inc()
	h.metrics.ActiveDeployments.Inc()
	respond(w, http.StatusCreated, deploy)
}

func (h *DeploymentHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	skip, _ := strconv.ParseInt(r.URL.Query().Get("skip"), 10, 64)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	deploys, err := h.svc.ListDeployments(r.Context(), limit, skip)
	if err != nil {
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	respond(w, http.StatusOK, map[string]interface{}{"deployments": deploys, "limit": limit, "skip": skip})
}

func (h *DeploymentHandler) Get(w http.ResponseWriter, r *http.Request) {
	deploy, err := h.svc.GetDeployment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		apierr.NotFound("deployment not found").Write(w, http.StatusNotFound)
		return
	}
	respond(w, http.StatusOK, deploy)
}

func (h *DeploymentHandler) Promote(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	id := chi.URLParam(r, "id")
	deploy, err := h.svc.Promote(r.Context(), id, claims.UserID, claims.Name, realIP(r))
	if err != nil {
		if err == repository.ErrNotFound {
			apierr.NotFound("deployment not found").Write(w, http.StatusNotFound)
			return
		}
		h.log.Error("promote failed", zap.String("id", id), zap.Error(err))
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	h.metrics.DeploymentsCompleted.Inc()
	h.metrics.ActiveDeployments.Dec()
	respond(w, http.StatusOK, deploy)
}

func (h *DeploymentHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	id := chi.URLParam(r, "id")
	deploy, err := h.svc.Rollback(r.Context(), id, claims.UserID, claims.Name, realIP(r))
	if err != nil {
		if err == repository.ErrNotFound {
			apierr.NotFound("deployment not found").Write(w, http.StatusNotFound)
			return
		}
		h.log.Error("rollback failed", zap.String("id", id), zap.Error(err))
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	h.metrics.DeploymentsRolledBack.Inc()
	h.metrics.ActiveDeployments.Dec()
	respond(w, http.StatusOK, deploy)
}

// Server

type ServerHandler struct {
	svc *service.CanaryService
	log *zap.Logger
}

func NewServerHandler(svc *service.CanaryService, log *zap.Logger) *ServerHandler {
	return &ServerHandler{svc: svc, log: log}
}

func (h *ServerHandler) Register(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	var body struct {
		Name    string   `json:"name"`
		Host    string   `json:"host"`
		Region  string   `json:"region"`
		Tags    []string `json:"tags"`
		Version string   `json:"version"`
	}
	if err := decode(r, &body); err != nil {
		apierr.BadRequest("invalid JSON body").Write(w, http.StatusBadRequest)
		return
	}
	errs := validate.Collect(
		validate.Required("name", body.Name),
		validate.MaxLen("name", body.Name, 100),
		validate.Required("host", body.Host),
	)
	if len(errs) > 0 {
		apierr.Validation(errs...).Write(w, http.StatusBadRequest)
		return
	}
	srv := &models.Server{
		Name: body.Name, Host: body.Host, Region: body.Region,
		Tags: body.Tags, CurrentVersion: body.Version,
	}
	if err := h.svc.RegisterServer(r.Context(), srv, claims.UserID, claims.Name, realIP(r)); err != nil {
		h.log.Error("register server failed", zap.Error(err))
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	respond(w, http.StatusCreated, srv)
}

func (h *ServerHandler) List(w http.ResponseWriter, r *http.Request) {
	servers, err := h.svc.ListServers(r.Context())
	if err != nil {
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	respond(w, http.StatusOK, servers)
}

func (h *ServerHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RecordHeartbeat(r.Context(), chi.URLParam(r, "id")); err != nil {
		apierr.NotFound("server not found").Write(w, http.StatusNotFound)
		return
	}
	respond(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Metrics

type MetricsHandler struct {
	canarySvc *service.CanaryService
	healthSvc *service.HealthService
	log       *zap.Logger
}

func NewMetricsHandler(cs *service.CanaryService, hs *service.HealthService, log *zap.Logger) *MetricsHandler {
	return &MetricsHandler{canarySvc: cs, healthSvc: hs, log: log}
}

func (h *MetricsHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var m models.Metrics
	if err := decode(r, &m); err != nil {
		apierr.BadRequest("invalid metrics payload").Write(w, http.StatusBadRequest)
		return
	}
	errs := validate.Collect(
		validate.Required("server_id", m.ServerID),
		validate.Required("deployment_id", m.DeploymentID),
		validate.FloatRange("error_rate", m.ErrorRate, 0, 1),
		validate.Positive("latency_ms", m.LatencyMs),
	)
	if len(errs) > 0 {
		apierr.Validation(errs...).Write(w, http.StatusBadRequest)
		return
	}
	m.RecordedAt = time.Now()
	if err := h.canarySvc.RecordMetrics(r.Context(), &m); err != nil {
		h.log.Error("failed to record metrics", zap.Error(err))
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"status": "recorded"})
}

func (h *MetricsHandler) GetForServer(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.healthSvc.GetServerMetrics(r.Context(), chi.URLParam(r, "serverID"))
	if err != nil {
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	respond(w, http.StatusOK, metrics)
}

func (h *MetricsHandler) GetReport(w http.ResponseWriter, r *http.Request) {
	deploy, err := h.canarySvc.GetDeployment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		apierr.NotFound("deployment not found").Write(w, http.StatusNotFound)
		return
	}
	report, err := h.healthSvc.EvaluateDeployment(r.Context(), deploy, deploy.MonitorDuration)
	if err != nil {
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	respond(w, http.StatusOK, report)
}

// Audit

type AuditHandler struct {
	auditRepo *repository.AuditRepo
	log       *zap.Logger
}

func NewAuditHandler(ar *repository.AuditRepo, log *zap.Logger) *AuditHandler {
	return &AuditHandler{auditRepo: ar, log: log}
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	if id := r.URL.Query().Get("resource_id"); id != "" {
		entries, err := h.auditRepo.ListForResource(r.Context(), id, 100)
		if err != nil {
			apierr.Internal().Write(w, http.StatusInternalServerError)
			return
		}
		respond(w, http.StatusOK, entries)
		return
	}
	if id := r.URL.Query().Get("actor_id"); id != "" {
		entries, err := h.auditRepo.ListForActor(r.Context(), id, 100)
		if err != nil {
			apierr.Internal().Write(w, http.StatusInternalServerError)
			return
		}
		respond(w, http.StatusOK, entries)
		return
	}
	apierr.BadRequest("query param resource_id or actor_id is required").Write(w, http.StatusBadRequest)
}

// Webhook

type WebhookHandler struct {
	webhookRepo *repository.WebhookRepo
	log         *zap.Logger
}

func NewWebhookHandler(wr *repository.WebhookRepo, log *zap.Logger) *WebhookHandler {
	return &WebhookHandler{webhookRepo: wr, log: log}
}

func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string                `json:"name"`
		URL    string                `json:"url"`
		Secret string                `json:"secret"`
		Events []models.WebhookEvent `json:"events"`
	}
	if err := decode(r, &body); err != nil {
		apierr.BadRequest("invalid JSON body").Write(w, http.StatusBadRequest)
		return
	}
	errs := validate.Collect(
		validate.Required("name", body.Name),
		validate.MaxLen("name", body.Name, 100),
		validate.Required("url", body.URL),
		validate.ValidURL("url", body.URL),
	)
	if len(body.Events) == 0 {
		errs = append(errs, apierr.Field("events", "must contain at least one event type"))
	}
	if len(errs) > 0 {
		apierr.Validation(errs...).Write(w, http.StatusBadRequest)
		return
	}
	wh := &models.Webhook{Name: body.Name, URL: body.URL, Secret: body.Secret, Events: body.Events}
	if err := h.webhookRepo.Create(r.Context(), wh); err != nil {
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	respond(w, http.StatusCreated, wh)
}

func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	webhooks, err := h.webhookRepo.List(r.Context())
	if err != nil {
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}
	respond(w, http.StatusOK, webhooks)
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.webhookRepo.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		apierr.NotFound("webhook not found").Write(w, http.StatusNotFound)
		return
	}
	respond(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Health

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler { return &HealthHandler{} }

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, map[string]string{
		"status": "ok", "service": "canopy",
		"time": time.Now().UTC().Format(time.RFC3339),
	})
}

// Shared

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(r.ResponseWriter, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
