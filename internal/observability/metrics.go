package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus instruments for Canopy.
// Use promauto so they register themselves on creation.
type Metrics struct {
	// HTTP
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// Deployments
	DeploymentsStarted    prometheus.Counter
	DeploymentsCompleted  prometheus.Counter
	DeploymentsRolledBack prometheus.Counter
	ActiveDeployments     prometheus.Gauge

	// Canary health
	CanaryErrorRate *prometheus.GaugeVec
	CanaryLatencyMs *prometheus.GaugeVec

	// Servers
	ServersTotal   prometheus.Gauge
	ServersOffline prometheus.Gauge

	// Webhooks
	WebhooksDelivered prometheus.Counter
	WebhooksFailed    prometheus.Counter

	// Auth
	LoginAttempts *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		HTTPRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "canopy_http_requests_total",
			Help: "Total HTTP requests by method, path, and status.",
		}, []string{"method", "path", "status"}),

		HTTPRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "canopy_http_request_duration_seconds",
			Help:    "HTTP request latency.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),

		DeploymentsStarted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "canopy_deployments_started_total",
			Help: "Total canary deployments started.",
		}),

		DeploymentsCompleted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "canopy_deployments_completed_total",
			Help: "Total deployments successfully promoted to 100%.",
		}),

		DeploymentsRolledBack: promauto.NewCounter(prometheus.CounterOpts{
			Name: "canopy_deployments_rolled_back_total",
			Help: "Total deployments rolled back.",
		}),

		ActiveDeployments: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "canopy_active_deployments",
			Help: "Number of deployments currently in canary/monitoring/rolling_out state.",
		}),

		CanaryErrorRate: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "canopy_canary_error_rate",
			Help: "Current average error rate for active canary deployments.",
		}, []string{"deployment_id", "version"}),

		CanaryLatencyMs: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "canopy_canary_latency_ms",
			Help: "Current average latency (ms) for active canary deployments.",
		}, []string{"deployment_id", "version"}),

		ServersTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "canopy_servers_total",
			Help: "Total registered servers in the fleet.",
		}),

		ServersOffline: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "canopy_servers_offline",
			Help: "Servers currently marked offline (missed heartbeat).",
		}),

		WebhooksDelivered: promauto.NewCounter(prometheus.CounterOpts{
			Name: "canopy_webhooks_delivered_total",
			Help: "Total webhook deliveries that succeeded.",
		}),

		WebhooksFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "canopy_webhooks_failed_total",
			Help: "Total webhook deliveries that exhausted retries.",
		}),

		LoginAttempts: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "canopy_login_attempts_total",
			Help: "Login attempts by result (success/failure).",
		}, []string{"result"}),
	}
}
