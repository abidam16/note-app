package http

import (
	"encoding/json"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type successEnvelope struct {
	Data any `json:"data"`
}

type errorEnvelope struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func NewAPIError(code, message string) APIError {
	return APIError{Code: code, Message: message}
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(successEnvelope{Data: data})
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, apiErr APIError) {
	apiErr.RequestID = chimiddleware.GetReqID(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{Error: apiErr})
}

func DecodeJSON(r *http.Request, dest any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}
