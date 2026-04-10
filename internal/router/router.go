package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/pixell07/canopy/internal/api/handlers"
	"github.com/pixell07/canopy/internal/middleware"
	"github.com/pixell07/canopy/internal/models"
)

func New(
	mw *middleware.Middleware,
	auth *handlers.AuthHandler,
	deploy *handlers.DeploymentHandler,
	server *handlers.ServerHandler,
	metrics *handlers.MetricsHandler,
	audit *handlers.AuditHandler,
	webhook *handlers.WebhookHandler,
	status *handlers.StatusHandler,
	health *handlers.HealthHandler,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.Recoverer)
	r.Use(chimw.CleanPath)
	r.Use(chimw.StripSlashes)
	r.Use(mw.RequestID)
	r.Use(mw.Logger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining"},
		AllowCredentials: false,
	}))

	// Public
	r.Get("/health", health.Check)
	r.Handle("/metrics", promhttp.Handler())
	r.Post("/auth/login", auth.Login)

	// Authenticated
	r.Group(func(r chi.Router) {
		r.Use(mw.Authenticate)
		r.Use(mw.RateLimit)

		// Auth
		r.Get("/auth/me", auth.Me)
		r.Post("/auth/refresh", auth.Refresh)
		r.With(mw.RequireRole(models.RoleAdmin)).Post("/auth/register", auth.Register)

		// System status — viewer+
		r.With(mw.RequireRole(models.RoleViewer)).Get("/status", status.Get)

		// Deployments
		r.Route("/deployments", func(r chi.Router) {
			r.With(mw.RequireRole(models.RoleViewer)).Get("/", deploy.List)
			r.With(mw.RequireRole(models.RoleDeployer)).Post("/", deploy.Start)
			r.Route("/{id}", func(r chi.Router) {
				r.With(mw.RequireRole(models.RoleViewer)).Get("/", deploy.Get)
				r.With(mw.RequireRole(models.RoleDeployer)).Post("/promote", deploy.Promote)
				r.With(mw.RequireRole(models.RoleDeployer)).Post("/rollback", deploy.Rollback)
			})
		})

		// Servers
		r.Route("/servers", func(r chi.Router) {
			r.With(mw.RequireRole(models.RoleViewer)).Get("/", server.List)
			r.With(mw.RequireRole(models.RoleAdmin)).Post("/", server.Register)
			r.Post("/{id}/heartbeat", server.Heartbeat)
		})

		// Metrics
		r.Route("/metrics", func(r chi.Router) {
			r.Post("/", metrics.Ingest)
			r.With(mw.RequireRole(models.RoleViewer)).Get("/server/{serverID}", metrics.GetForServer)
			r.With(mw.RequireRole(models.RoleViewer)).Get("/deployment/{id}/report", metrics.GetReport)
		})

		// Audit log (admin only)
		r.With(mw.RequireRole(models.RoleAdmin)).Get("/audit", audit.List)

		// Webhooks
		r.Route("/webhooks", func(r chi.Router) {
			r.With(mw.RequireRole(models.RoleViewer)).Get("/", webhook.List)
			r.With(mw.RequireRole(models.RoleAdmin)).Post("/", webhook.Create)
			r.With(mw.RequireRole(models.RoleAdmin)).Delete("/{id}", webhook.Delete)
		})
	})

	return r
}
