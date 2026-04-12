package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var notificationStreamHeartbeatInterval = 25 * time.Second

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

func writeSSEEvent(w http.ResponseWriter, eventType string, data any) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", eventType); err != nil {
		return err
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return nil
}

func writeSSEComment(w http.ResponseWriter, comment string) error {
	if comment == "" {
		comment = "keep-alive"
	}
	_, err := fmt.Fprintf(w, ": %s\n\n", comment)
	return err
}
