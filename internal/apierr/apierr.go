package apierr

import (
	"encoding/json"
	"net/http"
)

// represents a machine-readable error code the client can switch on.
type Code string

const (
	CodeBadRequest    Code = "BAD_REQUEST"
	CodeUnauthorized  Code = "UNAUTHORIZED"
	CodeForbidden     Code = "FORBIDDEN"
	CodeNotFound      Code = "NOT_FOUND"
	CodeConflict      Code = "CONFLICT"
	CodeUnprocessable Code = "UNPROCESSABLE"
	CodeRateLimited   Code = "RATE_LIMITED"
	CodeInternal      Code = "INTERNAL_ERROR"
	CodeValidation    Code = "VALIDATION_ERROR"
)

// Error is the canonical error envelope returned by every failing endpoint.
//
//	{
//	  "error": {
//	    "code":    "VALIDATION_ERROR",
//	    "message": "request validation failed",
//	    "details": [
//	      {"field": "canary_percent", "issue": "must be between 1 and 50"}
//	    ]
//	  }
//	}
type Error struct {
	Code    Code       `json:"code"`
	Message string     `json:"message"`
	Details []FieldErr `json:"details,omitempty"`
}

// FieldErr describes a validation failure for a specific field.
type FieldErr struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// envelope wraps Error for JSON encoding.
type envelope struct {
	Err Error `json:"error"`
}

// Write serialises the error envelope to the response writer.
func (e *Error) Write(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Err: *e})
}

// Constructors

func BadRequest(msg string) *Error {
	return &Error{Code: CodeBadRequest, Message: msg}
}

func Unauthorized(msg string) *Error {
	return &Error{Code: CodeUnauthorized, Message: msg}
}

func Forbidden(msg string) *Error {
	return &Error{Code: CodeForbidden, Message: msg}
}

func NotFound(msg string) *Error {
	return &Error{Code: CodeNotFound, Message: msg}
}

func Conflict(msg string) *Error {
	return &Error{Code: CodeConflict, Message: msg}
}

func Unprocessable(msg string) *Error {
	return &Error{Code: CodeUnprocessable, Message: msg}
}

func RateLimited() *Error {
	return &Error{Code: CodeRateLimited, Message: "rate limit exceeded — try again in 60 seconds"}
}

func Internal() *Error {
	return &Error{Code: CodeInternal, Message: "an internal error occurred"}
}

// returns a validation error with one or more field-level details.
func Validation(fields ...FieldErr) *Error {
	return &Error{
		Code:    CodeValidation,
		Message: "request validation failed",
		Details: fields,
	}
}

// Field is a shorthand for FieldErr.
func Field(name, issue string) FieldErr {
	return FieldErr{Field: name, Issue: issue}
}
