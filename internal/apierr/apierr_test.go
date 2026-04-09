package apierr_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pixell07/canopy/internal/apierr"
)

func TestErrorWrite_Status(t *testing.T) {
	tests := []struct {
		name       string
		err        *apierr.Error
		wantStatus int
	}{
		{"bad request", apierr.BadRequest("bad"), http.StatusBadRequest},
		{"unauthorized", apierr.Unauthorized("no"), http.StatusUnauthorized},
		{"forbidden", apierr.Forbidden("no"), http.StatusForbidden},
		{"not found", apierr.NotFound("gone"), http.StatusNotFound},
		{"conflict", apierr.Conflict("dup"), http.StatusConflict},
		{"internal", apierr.Internal(), http.StatusInternalServerError},
		{"rate limited", apierr.RateLimited(), http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.err.Write(w, tt.wantStatus)
			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", ct)
			}
		})
	}
}

func TestErrorWrite_JSONShape(t *testing.T) {
	w := httptest.NewRecorder()
	apierr.BadRequest("something went wrong").Write(w, http.StatusBadRequest)

	var envelope struct {
		Err struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if envelope.Err.Code != "BAD_REQUEST" {
		t.Errorf("expected code BAD_REQUEST, got %s", envelope.Err.Code)
	}
	if envelope.Err.Message != "something went wrong" {
		t.Errorf("unexpected message: %s", envelope.Err.Message)
	}
}

func TestValidationError_IncludesDetails(t *testing.T) {
	w := httptest.NewRecorder()
	apierr.Validation(
		apierr.Field("email", "must be a valid email"),
		apierr.Field("password", "must be at least 8 characters"),
	).Write(w, http.StatusBadRequest)

	var envelope struct {
		Err struct {
			Code    string `json:"code"`
			Details []struct {
				Field string `json:"field"`
				Issue string `json:"issue"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if envelope.Err.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %s", envelope.Err.Code)
	}
	if len(envelope.Err.Details) != 2 {
		t.Errorf("expected 2 field errors, got %d", len(envelope.Err.Details))
	}
	if envelope.Err.Details[0].Field != "email" {
		t.Errorf("expected first field to be 'email', got %s", envelope.Err.Details[0].Field)
	}
}
