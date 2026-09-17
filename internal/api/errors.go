package api

import (
	"encoding/json"
	"net/http"
)

// APIError represents the standard TinyVM API error payload.
type APIError struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the specific machine-readable code and human message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Common error codes
const (
	ErrCodeInvalidInput    = "INVALID_INPUT"
	ErrCodeNotFound        = "VM_NOT_FOUND"
	ErrCodeConflict        = "VM_STATE_CONFLICT"
	ErrCodeInternal        = "INTERNAL_ERROR"
	ErrCodeKVMUnavailable  = "KVM_UNAVAILABLE"
)

// WriteJSONError sends a structured JSON error response.
func WriteJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIError{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}
