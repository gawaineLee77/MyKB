package apierror

import (
	"encoding/json"
	"net/http"
)

// Problem is the stable, client-facing error shape returned by MindCreek.
type Problem struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

type envelope struct {
	Error Problem `json:"error"`
}

// Write sends a structured JSON error and never exposes internal details.
func Write(w http.ResponseWriter, status int, code, message, requestID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Error: Problem{
		Code:      code,
		Message:   message,
		RequestID: requestID,
	}})
}
