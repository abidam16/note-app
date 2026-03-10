package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appauth "note-app/internal/infrastructure/auth"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type contextKey string

const ContextUserIDKey contextKey = "user_id"

func Authenticate(tokenManager appauth.TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := strings.TrimSpace(r.Header.Get("Authorization"))
			if header == "" || !strings.HasPrefix(header, "Bearer ") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"missing bearer token"}}`))
				return
			}

			token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			claims, err := tokenManager.ParseAccessToken(token)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"invalid access token"}}`))
				return
			}

			ctx := context.WithValue(r.Context(), ContextUserIDKey, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			wrapped := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			logMethod := logger.Info
			if wrapped.status >= http.StatusInternalServerError {
				logMethod = logger.Error
			} else if wrapped.status >= http.StatusBadRequest {
				logMethod = logger.Warn
			}

			logMethod("http request",
				slog.String("request_id", chimiddleware.GetReqID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", r.URL.RawQuery),
				slog.Int("status", wrapped.status),
				slog.Duration("duration", time.Since(startedAt)),
				slog.Int("bytes_written", wrapped.bytesWritten),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
				slog.String("referer", r.Referer()),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status       int
	bytesWritten int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.bytesWritten += n
	return n, err
}
