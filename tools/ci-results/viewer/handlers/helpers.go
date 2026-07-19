package handlers

import (
	"encoding/json"
	"net/http"
)

const (
	ErrCodeInvalidJSON    = "invalid_json"
	ErrCodeInvalidInput   = "invalid_input"
	ErrCodeNotFound       = "not_found"
	ErrCodeQueryFailed    = "query_failed"
	ErrCodeInternal       = "internal_error"
	ErrCodeNotImplemented = "not_implemented"
)

type APIErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

func WriteJSON(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(data)
}

func DecodeJSON[T any](w http.ResponseWriter, r *http.Request, value *T) bool {
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		WriteAPIError(w, "invalid request body", ErrCodeInvalidJSON, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

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
