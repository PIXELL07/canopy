package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/pixell07/canopy/internal/auth"
	"github.com/pixell07/canopy/internal/models"
	"github.com/pixell07/canopy/internal/repository"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailTaken     = errors.New("email already registered")
	ErrBadCredentials = errors.New("invalid email or password")
	ErrUserInactive   = errors.New("account is inactive")
)

type UserService struct {
	userRepo    *repository.UserRepo
	auditRepo   *repository.AuditRepo
	authService *auth.Service
	log         *zap.Logger
}

func NewUserService(
	ur *repository.UserRepo,
	ar *repository.AuditRepo,
	as *auth.Service,
	log *zap.Logger,
) *UserService {
	return &UserService{userRepo: ur, auditRepo: ar, authService: as, log: log}
}

type RegisterRequest struct {
	Name     string
	Email    string
	Password string
	Role     models.Role
}

// Register creates a new user with hashed password and a generated API key.
func (s *UserService) Register(ctx context.Context, req RegisterRequest, actorID, actorName, ip string) (*models.User, error) {
	if _, err := s.userRepo.GetByEmail(ctx, req.Email); err == nil {
		return nil, ErrEmailTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, err
	}

	if req.Role == "" {
		req.Role = models.RoleViewer
	}

	user := &models.User{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hash),
		APIKey:       apiKey,
		Role:         req.Role,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	_ = s.auditRepo.Append(ctx, &models.AuditEntry{
		Action:       models.AuditUserCreate,
		ActorID:      actorID,
		ActorName:    actorName,
		ResourceType: "user",
		ResourceID:   user.ID.Hex(),
		Meta:         map[string]interface{}{"email": user.Email, "role": user.Role},
		IPAddress:    ip,
	})

	s.log.Info("user registered", zap.String("email", user.Email), zap.String("role", string(user.Role)))
	return user, nil
}

type LoginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
	User      *models.User `json:"user"`
}

// Login authenticates a user and returns a signed JWT.
func (s *UserService) Login(ctx context.Context, email, password, ip string) (*LoginResponse, error) {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, ErrBadCredentials
	}
	if !user.Active {
		return nil, ErrUserInactive
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrBadCredentials
	}

	token, err := s.authService.GenerateToken(user)
	if err != nil {
		return nil, err
	}

	_ = s.userRepo.UpdateLastLogin(ctx, user.ID.Hex())
	_ = s.auditRepo.Append(ctx, &models.AuditEntry{
		Action:       models.AuditUserLogin,
		ActorID:      user.ID.Hex(),
		ActorName:    user.Name,
		ResourceType: "user",
		ResourceID:   user.ID.Hex(),
		IPAddress:    ip,
	})

	return &LoginResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User:      user,
	}, nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "cpy_" + hex.EncodeToString(b), nil
}

// GetByID returns a user by their MongoDB ID string.
func (s *UserService) GetByID(ctx context.Context, id string) (*models.User, error) {
	return s.userRepo.GetByID(ctx, id)
}

// IssueToken signs a new JWT for the given user without any login side effects.
func (s *UserService) IssueToken(user *models.User) (string, error) {
	return s.authService.GenerateToken(user)
}
