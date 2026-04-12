package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

const (
	defaultJSONBodyLimitBytes  int64 = 1 << 20
	documentJSONBodyLimitBytes int64 = 8 << 20
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

func DecodeJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	return DecodeJSONWithLimit(w, r, dest, defaultJSONBodyLimitBytes)
}

func DecodeJSONWithLimit(w http.ResponseWriter, r *http.Request, dest any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = defaultJSONBodyLimitBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	return errors.New("request body must contain a single JSON value")
}
