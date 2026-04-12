# Contributing to Canopy

Thank you for your interest in contributing. This document covers how to get set up, the conventions we follow, and how to submit changes.

---

## Getting started

```bash
git clone https://github.com/yourname/canopy
cd canopy
cp .env.example .env
make up       # start MongoDB + Redis
make tidy     # download dependencies
make run      # start the server
```

---

## Running tests

```bash
# Unit tests (no external dependencies)
make test

# Integration tests (requires MongoDB on localhost:27017)
MONGO_URI=mongodb://localhost:27017 go test ./internal/integration/... -v

# With race detector (always use this before opening a PR)
go test ./... -race -count=1
```

---

## Code conventions

**Package structure** — we use a strict layered architecture. Imports only flow inward:

```
handlers → services → repositories → models
                    ↘ auth / notify / observability
```

A handler must never import a repository directly. A repository must never import a service. Circular imports are a compile error — use this to your advantage.

**Errors** — use `apierr.X()` in handlers for structured HTTP errors. Use sentinel errors (`var ErrX = errors.New(...)`) in services. Never return raw MongoDB errors to handlers.

**Validation** — all input validation goes through `internal/validate`. Add new helpers there rather than validating inline in handlers.

**Logging** — use structured `zap` fields, never `fmt.Printf`. Every significant action (deployment started, server offline) should emit an `Info` or `Warn` log with relevant field context.

**Audit log** — any action that modifies state (create, update, delete) should write an `AuditEntry`. The repo's `Append` method is append-only by design — never update or delete audit entries.

**Context** — every function that touches the database takes a `context.Context` as its first argument. Never use `context.Background()` inside a request handler — propagate the request context.

---

## Opening a pull request

1. Fork the repo and create a branch: `git checkout -b feat/your-feature`
2. Write tests for new code. Integration tests for any new endpoint.
3. Ensure `make test` and `make lint` pass locally.
4. Open a PR against `main` with a clear description of what changed and why.
5. CI must be green before merge.

---

## Project layout

```
canopy/
├── cmd/server/main.go          Wire everything together
├── config/                     Environment-driven config
├── internal/
│   ├── apierr/                 Structured HTTP error types
│   ├── auth/                   JWT + RBAC
│   ├── api/handlers/           HTTP handlers (thin — no business logic)
│   ├── middleware/             Auth, rate limit, logging, request ID
│   ├── models/                 Domain types (no logic)
│   ├── notify/                 Webhook delivery pool
│   ├── observability/          Prometheus metrics
│   ├── repository/             MongoDB + Redis data access
│   ├── router/                 Route declarations + RBAC assignments
│   ├── service/                Business logic (canary, health, user, watcher)
│   ├── validate/               Input validation helpers
│   └── integration/            End-to-end tests against real MongoDB
├── scripts/                    seed.sh, deploy.sh, rollback.sh
├── docs/openapi.yaml           Full OpenAPI 3.0 spec
└── .github/workflows/ci.yml   GitHub Actions CI
```
