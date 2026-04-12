package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	appauth "note-app/internal/infrastructure/auth"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type contextKey string

const ContextUserIDKey contextKey = "user_id"

type RateLimitConfig struct {
	Window      time.Duration
	MaxRequests int
	Now         func() time.Time
}

type ConcurrencyLimitConfig struct {
	MaxConcurrent int
}

func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			next.ServeHTTP(w, r)
		})
	}
}

func NoStore() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
			next.ServeHTTP(w, r)
		})
	}
}

type rateLimitBucket struct {
	windowStartedAt time.Time
	requestCount    int
}

type rateLimiter struct {
	mu            sync.Mutex
	window        time.Duration
	maxRequests   int
	now           func() time.Time
	buckets       map[string]rateLimitBucket
	lastCleanupAt time.Time
}

func RateLimit(config RateLimitConfig) func(http.Handler) http.Handler {
	if config.Window <= 0 || config.MaxRequests <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	nowFn := config.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	limiter := &rateLimiter{
		window:      config.Window,
		maxRequests: config.MaxRequests,
		now:         nowFn,
		buckets:     make(map[string]rateLimitBucket),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := limiter.allow(clientKey(r))
			if !allowed {
				writeRateLimitExceeded(w, retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ConcurrencyLimit(config ConcurrencyLimitConfig) func(http.Handler) http.Handler {
	if config.MaxConcurrent <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	slots := make(chan struct{}, config.MaxConcurrent)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				next.ServeHTTP(w, r)
			default:
				writeOverloaded(w)
			}
		})
	}
}

func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	now := l.now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastCleanupAt.IsZero() || now.Sub(l.lastCleanupAt) >= l.window {
		l.cleanup(now)
		l.lastCleanupAt = now
	}

	bucket, ok := l.buckets[key]
	if !ok || now.Sub(bucket.windowStartedAt) >= l.window {
		l.buckets[key] = rateLimitBucket{
			windowStartedAt: now,
			requestCount:    1,
		}
		return true, 0
	}

	if bucket.requestCount >= l.maxRequests {
		retryAfter := l.window - now.Sub(bucket.windowStartedAt)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter
	}

	bucket.requestCount++
	l.buckets[key] = bucket
	return true, 0
}

func (l *rateLimiter) cleanup(now time.Time) {
	for key, bucket := range l.buckets {
		if now.Sub(bucket.windowStartedAt) >= l.window {
			delete(l.buckets, key)
		}
	}
}

func clientKey(r *http.Request) string {
	return RequestClientIP(r)
}

func writeRateLimitExceeded(w http.ResponseWriter, window time.Duration) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", formatRetryAfter(window))
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    "rate_limited",
			"message": "too many requests",
		},
	})
}

func writeOverloaded(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "1")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    "overloaded",
			"message": "server is handling too many expensive requests",
		},
	})
}

func formatRetryAfter(window time.Duration) string {
	seconds := int(window / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}

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
				slog.String("query", SanitizeLogQuery(r.URL.RawQuery)),
				slog.Int("status", wrapped.status),
				slog.Duration("duration", time.Since(startedAt)),
				slog.Int("bytes_written", wrapped.bytesWritten),
				slog.String("client_ip", RequestClientIP(r)),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
				slog.String("referer", SanitizeLogReferer(r.Referer())),
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
