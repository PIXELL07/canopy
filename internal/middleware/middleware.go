package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pixell07/canopy/internal/apierr"
	"github.com/pixell07/canopy/internal/auth"
	"github.com/pixell07/canopy/internal/models"
	"github.com/pixell07/canopy/internal/observability"
	"github.com/pixell07/canopy/internal/repository"
	"go.uber.org/zap"
)

type Middleware struct {
	authSvc      *auth.Service
	userRepo     *repository.UserRepo
	redis        *repository.RedisClient // may be nil — rate limiting fails open
	metrics      *observability.Metrics
	log          *zap.Logger
	rateLimitRPM int
}

func New(
	authSvc *auth.Service,
	userRepo *repository.UserRepo,
	redis *repository.RedisClient,
	metrics *observability.Metrics,
	log *zap.Logger,
	rateLimitRPM int,
) *Middleware {
	return &Middleware{
		authSvc: authSvc, userRepo: userRepo, redis: redis,
		metrics: metrics, log: log, rateLimitRPM: rateLimitRPM,
	}
}

// injects a unique X-Request-ID into every response.
func (m *Middleware) RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

// Logger logs every request with structured fields.
func (m *Middleware) Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		duration := time.Since(start)

		m.log.Info("http",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", rw.status),
			zap.Duration("duration", duration),
			zap.String("ip", realIP(r)),
			zap.String("request_id", w.Header().Get("X-Request-ID")),
		)
		m.metrics.HTTPRequestsTotal.WithLabelValues(
			r.Method, r.URL.Path, strconv.Itoa(rw.status),
		).Inc()
		m.metrics.HTTPRequestDuration.WithLabelValues(
			r.Method, r.URL.Path,
		).Observe(duration.Seconds())
	})
}

// validates Bearer JWT or X-API-Key. Injects claims into context.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, authType := auth.ExtractFromRequest(r)
		if token == "" {
			apierr.Unauthorized("missing authentication — provide Authorization: Bearer <token> or X-API-Key").
				Write(w, http.StatusUnauthorized)
			return
		}

		var claims *auth.Claims

		switch authType {
		case "bearer":
			c, err := m.authSvc.ValidateToken(token)
			if err != nil {
				apierr.Unauthorized("invalid or expired token").Write(w, http.StatusUnauthorized)
				return
			}
			claims = c

		case "apikey":
			user, err := m.userRepo.GetByAPIKey(r.Context(), token)
			if err != nil {
				apierr.Unauthorized("invalid API key").Write(w, http.StatusUnauthorized)
				return
			}
			claims = &auth.Claims{
				UserID: user.ID.Hex(),
				Email:  user.Email,
				Name:   user.Name,
				Role:   user.Role,
			}
		}

		ctx := auth.WithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// returns middleware that enforces a minimum RBAC role.
func (m *Middleware) RequireRole(role models.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok {
				apierr.Unauthorized("not authenticated").Write(w, http.StatusUnauthorized)
				return
			}
			if err := auth.RequireRole(claims, role); err != nil {
				apierr.Forbidden(fmt.Sprintf("requires role: %s", role)).Write(w, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit enforces per-identity request rate limiting via Redis.
// Fails open if Redis is unavailable.
func (m *Middleware) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If Redis is not configured, skip rate limiting entirely
		if m.redis == nil {
			next.ServeHTTP(w, r)
			return
		}

		key := realIP(r)
		if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
			key = "user:" + claims.UserID
		}

		allowed, remaining, err := m.redis.RateLimit(r.Context(), key, m.rateLimitRPM)
		if err != nil {
			// Fail open — Redis outage must not block users
			m.log.Warn("rate limit check failed, failing open", zap.Error(err))
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(m.rateLimitRPM))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if !allowed {
			apierr.RateLimited().Write(w, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// helpers

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
