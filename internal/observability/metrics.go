package observability

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	HTTPRequestsTotal     *prometheus.CounterVec
	HTTPRequestDuration   *prometheus.HistogramVec
	DeploymentsStarted    prometheus.Counter
	DeploymentsCompleted  prometheus.Counter
	DeploymentsRolledBack prometheus.Counter
	ActiveDeployments     prometheus.Gauge
	CanaryErrorRate       *prometheus.GaugeVec
	CanaryLatencyMs       *prometheus.GaugeVec
	ServersTotal          prometheus.Gauge
	ServersOffline        prometheus.Gauge
	WebhooksDelivered     prometheus.Counter
	WebhooksFailed        prometheus.Counter
	LoginAttempts         *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	// Use a fresh registry each time — safe for tests
	reg := prometheus.NewRegistry()

	m := &Metrics{
		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "canopy_http_requests_total",
			Help: "Total HTTP requests by method, path, and status.",
		}, []string{"method", "path", "status"}),

		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "canopy_http_request_duration_seconds",
			Help:    "HTTP request latency.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),

		DeploymentsStarted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "canopy_deployments_started_total",
			Help: "Total canary deployments started.",
		}),

		DeploymentsCompleted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "canopy_deployments_completed_total",
			Help: "Total deployments promoted to 100%.",
		}),

		DeploymentsRolledBack: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "canopy_deployments_rolled_back_total",
			Help: "Total deployments rolled back.",
		}),

		ActiveDeployments: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "canopy_active_deployments",
			Help: "Deployments currently in canary/monitoring/rolling_out state.",
		}),

		CanaryErrorRate: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "canopy_canary_error_rate",
			Help: "Current average error rate for active canary deployments.",
		}, []string{"deployment_id", "version"}),

		CanaryLatencyMs: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "canopy_canary_latency_ms",
			Help: "Current average latency (ms) for active canary deployments.",
		}, []string{"deployment_id", "version"}),

		ServersTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "canopy_servers_total",
			Help: "Total registered servers.",
		}),

		ServersOffline: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "canopy_servers_offline",
			Help: "Servers currently offline.",
		}),

		WebhooksDelivered: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "canopy_webhooks_delivered_total",
			Help: "Total webhook deliveries that succeeded.",
		}),

		WebhooksFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "canopy_webhooks_failed_total",
			Help: "Total webhook deliveries that failed.",
		}),

		LoginAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "canopy_login_attempts_total",
			Help: "Login attempts by result.",
		}, []string{"result"}),
	}

	// Register all — ignore errors in tests (duplicate registration is safe now)
	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.DeploymentsStarted,
		m.DeploymentsCompleted,
		m.DeploymentsRolledBack,
		m.ActiveDeployments,
		m.CanaryErrorRate,
		m.CanaryLatencyMs,
		m.ServersTotal,
		m.ServersOffline,
		m.WebhooksDelivered,
		m.WebhooksFailed,
		m.LoginAttempts,
	)

	return m
}
