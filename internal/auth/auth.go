package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pixell07/canopy/internal/models"
)

// Errors

var (
	ErrInvalidToken     = errors.New("invalid or expired token")
	ErrInsufficientRole = errors.New("insufficient permissions for this action")
)

// Claims
type Claims struct {
	UserID string      `json:"user_id"`
	Email  string      `json:"email"`
	Name   string      `json:"name"`
	Role   models.Role `json:"role"`
	jwt.RegisteredClaims
}

// Service

type Service struct {
	jwtSecret []byte
	ttl       time.Duration
}

func NewService(secret string, ttl time.Duration) *Service {
	return &Service{
		jwtSecret: []byte(secret),
		ttl:       ttl,
	}
}

// GenerateToken signs a JWT for the given user.
func (s *Service) GenerateToken(user *models.User) (string, error) {
	claims := Claims{
		UserID: user.ID.Hex(),
		Email:  user.Email,
		Name:   user.Name,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "canopy",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ValidateToken parses and verifies a token string.
func (s *Service) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ExtractFromRequest pulls the Bearer token or X-API-Key from a request.
// Returns the token string and the auth type ("bearer" or "apikey").
func ExtractFromRequest(r *http.Request) (string, string) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer "), "bearer"
	}
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key, "apikey"
	}
	return "", ""
}

// RBAC

// RequireRole checks that the claims role meets the minimum required role.
// Role hierarchy: admin > deployer > viewer
func RequireRole(claims *Claims, required models.Role) error {
	if rankOf(claims.Role) < rankOf(required) {
		return ErrInsufficientRole
	}
	return nil
}

func rankOf(r models.Role) int {
	switch r {
	case models.RoleAdmin:
		return 3
	case models.RoleDeployer:
		return 2
	case models.RoleViewer:
		return 1
	}
	return 0
}

// Context helpers

type contextKey string

const claimsKey contextKey = "claims"

// WithClaims stores claims in the request context.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// ClaimsFromContext retrieves claims from the context.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}
