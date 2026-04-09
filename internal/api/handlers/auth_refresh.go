package handlers

import (
	"net/http"
	"time"

	"github.com/pixell07/canopy/internal/apierr"
	"github.com/pixell07/canopy/internal/auth"
	"go.uber.org/zap"
)

// POST /auth/refresh
// Issues a new JWT for an already-authenticated user without re-entering password.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		apierr.Unauthorized("not authenticated").Write(w, http.StatusUnauthorized)
		return
	}

	user, err := h.userSvc.GetByID(r.Context(), claims.UserID)
	if err != nil {
		apierr.Unauthorized("user no longer exists").Write(w, http.StatusUnauthorized)
		return
	}
	if !user.Active {
		apierr.Unauthorized("account is inactive").Write(w, http.StatusUnauthorized)
		return
	}

	token, err := h.userSvc.IssueToken(user)
	if err != nil {
		h.log.Error("token refresh failed", zap.Error(err))
		apierr.Internal().Write(w, http.StatusInternalServerError)
		return
	}

	respond(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"expires_at": time.Now().Add(24 * time.Hour),
	})
}
