package handlers

import (
	"encoding/json"
	"net/http"
)

const (
	ErrCodeInvalidInput   = "invalid_input"
	ErrCodeNotFound       = "not_found"
	ErrCodeQueryFailed    = "query_failed"
	ErrCodeInternal       = "internal_error"
	ErrCodeNotImplemented = "not_implemented"
)

// APIErrorResponse is the standard error body returned by the viewer API.
type APIErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// WriteJSON writes a JSON response body.
func WriteJSON(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(data)
}

// WriteAPIError writes a consistently shaped JSON API error response.
func WriteAPIError(w http.ResponseWriter, errorMsg, code, details string, statusCode int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	data, err := json.Marshal(APIErrorResponse{
		Error:   errorMsg,
		Code:    code,
		Details: details,
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(statusCode)
	if _, err := w.Write(data); err != nil {
		return
	}
}
