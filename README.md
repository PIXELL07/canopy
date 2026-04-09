# Canopy — Production Deployment Platform

A self-hosted canary deployment platform built in Go. Ship new versions to a percentage of your fleet, monitor health metrics in real time, and automatically promote or roll back — with full audit logging, RBAC, webhook notifications, and Prometheus observability.

---

## Architecture

```
                        ┌─────────────────────────────────┐
                        │           API Gateway            │
                        │  JWT auth · RBAC · Rate limit    │
                        └──────────────┬──────────────────┘
                                       │
          ┌──────────────┬─────────────┼──────────────┬──────────────┐
          │              │             │              │              │
    ┌─────▼────┐  ┌──────▼─────┐ ┌────▼─────┐ ┌─────▼────┐ ┌──────▼───┐
    │Deployment│  │  Health    │ │  User    │ │  Audit   │ │ Webhook  │
    │ Service  │  │  Service   │ │ Service  │ │ Service  │ │ Notifier │
    └─────┬────┘  └──────┬─────┘ └────┬─────┘ └─────┬────┘ └──────┬───┘
          │              │             │              │              │
          └──────────────┴─────────────┼──────────────┴──────────────┘
                                       │
          ┌──────────────┬─────────────┴──────────────┐
          │              │                             │
     ┌────▼────┐   ┌─────▼─────┐              ┌───────▼──────┐
     │ MongoDB │   │   Redis   │              │  Background  │
     │ (data)  │   │(rate lim) │              │   Watcher    │
     └─────────┘   └───────────┘              └──────────────┘
```

### Background workers (always running)
- **Canary watcher** — every 30s: evaluates active deployments, auto-promotes or rolls back
- **Heartbeat checker** — every 60s: marks servers offline if silent for 90s

---

## Features

| Feature | Details |
|---|---|
| Canary deployments | Route % of traffic to new version by selecting N servers |
| Auto promote/rollback | Watcher evaluates error rate, latency, crash count |
| JWT authentication | HS256 tokens, configurable TTL |
| API key auth | Per-user `cpy_...` keys for machine-to-machine |
| RBAC | `admin` / `deployer` / `viewer` role hierarchy |
| Redis rate limiting | Per-identity sliding window, fails open |
| Audit log | Append-only, every action recorded with actor + IP |
| Webhook notifications | HMAC-SHA256 signed, 3-retry with backoff |
| Prometheus metrics | `/metrics` endpoint with counters, gauges, histograms |
| MongoDB indexes | 13 indexes including TTL (metrics auto-deleted after 30d) |
| Graceful shutdown | Drains in-flight requests on SIGINT/SIGTERM |

---

## Quick Start

```bash
# 1. Copy and configure
cp .env.example .env

# 2. Start MongoDB + Redis + server
make up

# 3. Seed: creates admin user + 10 servers + webhook
bash scripts/seed.sh

# 4. Run a canary deployment (auto-promotes after 60s of healthy metrics)
export TOKEN=<token from seed output>
bash scripts/deploy.sh
```

---

## API Reference

All endpoints except `/health`, `/metrics`, and `POST /auth/login` require:
```
Authorization: Bearer <jwt>
# or
X-API-Key: cpy_<your api key>
```

### Auth
| Method | Path | Role | Description |
|---|---|---|---|
| POST | `/auth/login` | public | Get JWT token |
| GET | `/auth/me` | any | Current user info |
| POST | `/auth/register` | admin | Create user |

### Deployments
| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/deployments` | viewer | List deployments (paginated) |
| POST | `/deployments` | deployer | Start canary deployment |
| GET | `/deployments/{id}` | viewer | Get deployment |
| POST | `/deployments/{id}/promote` | deployer | Roll out to 100% |
| POST | `/deployments/{id}/rollback` | deployer | Revert canaries |

### Servers
| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/servers` | viewer | List all servers |
| POST | `/servers` | admin | Register server |
| POST | `/servers/{id}/heartbeat` | any | Agent heartbeat |

### Metrics
| Method | Path | Role | Description |
|---|---|---|---|
| POST | `/metrics` | any | Ingest metrics snapshot |
| GET | `/metrics/server/{id}` | viewer | Per-server metrics |
| GET | `/metrics/deployment/{id}/report` | viewer | Health report + recommendation |

### Webhooks
| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/webhooks` | viewer | List webhooks |
| POST | `/webhooks` | admin | Create webhook |
| DELETE | `/webhooks/{id}` | admin | Delete webhook |

### Audit
| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/audit?resource_id=<id>` | admin | Audit log for resource |
| GET | `/audit?actor_id=<id>` | admin | Audit log for actor |

---

## Webhook Payload

```json
{
  "event": "deployment.rolled_back",
  "timestamp": "2025-01-15T10:30:00Z",
  "data": {
    "deployment_id": "...",
    "version": "v2.0",
    "rolled_back_to": "v1.0",
    "reasons": ["error rate above threshold"],
    "triggered_by": "auto"
  }
}
```

Verify the signature: `X-Canopy-Signature: sha256=<hmac-sha256 of body>`

---

## Prometheus Metrics

Scraped at `GET /metrics` (no auth required for Prometheus).

```
canopy_http_requests_total{method, path, status}
canopy_http_request_duration_seconds{method, path}
canopy_deployments_started_total
canopy_deployments_completed_total
canopy_deployments_rolled_back_total
canopy_active_deployments
canopy_canary_error_rate{deployment_id, version}
canopy_canary_latency_ms{deployment_id, version}
canopy_servers_total
canopy_servers_offline
canopy_webhooks_delivered_total
canopy_webhooks_failed_total
canopy_login_attempts_total{result}
```

---

## Project Structure

```
canopy/                                          44 files · 4,412 Go lines
│
├── cmd/server/main.go                           Entry point — wires all deps, graceful shutdown
├── config/config.go                             Env-based config with production validation
├── go.mod                                       10 dependencies declared
│
├── internal/
│   ├── models/models.go                         All domain types: User, Deployment, Server,
│   │                                            Metrics, AuditEntry, Webhook
│   │
│   ├── auth/auth.go                             JWT HS256 sign/validate, RBAC role hierarchy,
│   │                                            context helpers (WithClaims/ClaimsFromContext)
│   │
│   ├── apierr/
│   │   ├── apierr.go                            Typed error envelope: code + message + field details
│   │   └── apierr_test.go                       3 tests: status codes, JSON shape, validation details
│   │
│   ├── validate/
│   │   ├── validate.go                          Required, MinLen, MaxLen, Email, InRange,
│   │   │                                        FloatRange, ValidURL, Collect
│   │   └── validate_test.go                     7 tests covering every validator
│   │
│   ├── repository/
│   │   ├── errors.go                            ErrNotFound sentinel
│   │   ├── mongo.go                             DB wrapper + Disconnect + 13 indexes + TTL
│   │   ├── redis.go                             RateLimit (sliding window) + Set/Get/Del
│   │   ├── repos.go                             DeploymentRepo, ServerRepo, MetricsRepo
│   │   ├── user_repo.go                         GetByEmail, GetByAPIKey, UpdateLastLogin
│   │   ├── audit_webhook_repo.go                AuditRepo (append-only) + WebhookRepo
│   │   └── aggregations.go                      MongoDB $group pipeline for fleet stats
│   │
│   ├── service/
│   │   ├── canary_service.go                    StartCanary, Promote, Rollback + audit logging
│   │   ├── health_service.go                    EvaluateDeployment with reasons array
│   │   ├── user_service.go                      Register (bcrypt), Login (JWT), GetByID, IssueToken
│   │   ├── watcher_service.go                   Eval loop (30s) + heartbeat loop (60s), uses pool
│   │   └── canopy_test.go                       10 unit tests: health eval + RBAC
│   │
│   ├── notify/
│   │   ├── webhook.go                           HMAC-SHA256 signed delivery, 3 retries + backoff
│   │   └── pool.go                              Bounded worker pool (10 workers, 200-job buffer)
│   │
│   ├── observability/metrics.go                 13 Prometheus instruments (counters/gauges/histograms)
│   │
│   ├── middleware/middleware.go                  RequestID, Logger, Authenticate (JWT+APIKey),
│   │                                            RequireRole, RateLimit (nil-safe Redis)
│   │
│   ├── api/handlers/
│   │   ├── handlers.go                          All handlers — Auth, Deployment, Server,
│   │   │                                        Metrics, Audit, Webhook, Health
│   │   ├── auth_refresh.go                      POST /auth/refresh — re-issue JWT
│   │   └── status_handler.go                    GET /status — concurrent fleet snapshot
│   │
│   ├── router/router.go                         Chi routes with per-route RBAC assignments
│   │
│   └── integration/
│       ├── helpers_test.go                      Full httptest.Server setup, HTTP helpers
│       └── integration_test.go                  20 end-to-end tests against real MongoDB
│
├── scripts/
│   ├── seed.sh                                  Create admin + 10 servers + Slack webhook
│   ├── deploy.sh                                Start canary + stream healthy metrics
│   ├── rollback.sh                              Manual rollback + audit trail
│   └── status.sh                               Fleet health summary in terminal
│
├── docs/openapi.yaml                            Full OpenAPI 3.0 spec — every endpoint
├── .github/workflows/ci.yml                     GitHub Actions: test + lint + docker build
├── .golangci.yml                                11 linters (bodyclose, noctx, errcheck...)
├── Dockerfile                                   Two-stage scratch build (~10MB image)
├── docker-compose.yml                           MongoDB + Redis + Canopy with healthchecks
├── Makefile                                     run, build, test, test-integration, lint, up, down
├── .env.example
├── README.md
└── CONTRIBUTING.md
```