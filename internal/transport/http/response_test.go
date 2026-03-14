package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAPIError(t *testing.T) {
	err := NewAPIError("code", "message")
	if err.Code != "code" || err.Message != "message" {
		t.Fatalf("unexpected api error: %+v", err)
	}
}

func TestWriteJSON(t *testing.T) {
	res := httptest.NewRecorder()
	WriteJSON(res, http.StatusCreated, map[string]string{"ok": "yes"})

	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}
	if ct := res.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("unexpected content-type: %s", ct)
	}

	var body map[string]map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if body["data"]["ok"] != "yes" {
		t.Fatalf("unexpected payload: %+v", body)
	}
}

func TestWriteError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	WriteError(res, req, http.StatusBadRequest, NewAPIError("bad", "invalid"))

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.Code)
	}

	var payload map[string]map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("parse error body: %v", err)
	}
	if payload["error"]["code"] != "bad" || payload["error"]["message"] != "invalid" {
		t.Fatalf("unexpected error payload: %+v", payload)
	}
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	var ok payload
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"name":"john"}`))
	if err := DecodeJSON(req, &ok); err != nil {
		t.Fatalf("decode valid json: %v", err)
	}
	if ok.Name != "john" {
		t.Fatalf("unexpected name: %s", ok.Name)
	}

	var bad payload
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"name":"john","extra":1}`))
	if err := DecodeJSON(req, &bad); err == nil {
		t.Fatal("expected decode to fail on unknown field")
	}
}
