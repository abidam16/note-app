package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appauth "note-app/internal/infrastructure/auth"
)

func TestAuthenticateMiddleware(t *testing.T) {
	tm := appauth.NewTokenManager("secret", "issuer", 15*time.Minute)
	token, _, err := tm.GenerateAccessToken("user-1", "u@example.com", time.Now().UTC())
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	okHandler := Authenticate(tm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value(ContextUserIDKey).(string)
		if userID != "user-1" {
			t.Fatalf("expected user id in context, got %q", userID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	okHandler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}

	for _, tc := range []struct {
		name   string
		header string
	}{
		{name: "missing", header: ""},
		{name: "invalid", header: "Bearer bad-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := Authenticate(tm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", res.Code)
			}
		})
	}
}

func TestLoggerMiddleware(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))

	h := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/log", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	line, err := io.ReadAll(&output)
	if err != nil {
		t.Fatalf("read logger output: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(line, &payload); err != nil {
		t.Fatalf("parse logger output: %v", err)
	}
	if payload["msg"] != "http request" {
		t.Fatalf("unexpected log msg: %v", payload["msg"])
	}
}

func TestLoggerMiddlewareSanitizesSensitiveQueryAndRefererValues(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))

	h := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh?refresh_token=rt-secret&password=Password1&safe=ok", nil)
	req.Header.Set("Authorization", "Bearer bearer-secret")
	req.Header.Set("Referer", "https://app.example.com/review?access_token=at-secret&safe=1")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}

	logs := output.String()
	for _, secret := range []string{"rt-secret", "Password1", "at-secret", "bearer-secret"} {
		if bytes.Contains(output.Bytes(), []byte(secret)) {
			t.Fatalf("expected logs to redact %q, got %s", secret, logs)
		}
	}
	if !bytes.Contains(output.Bytes(), []byte(`"query":"password=%5BREDACTED%5D&refresh_token=%5BREDACTED%5D&safe=ok"`)) {
		t.Fatalf("expected sanitized query in logs, got %s", logs)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"referer":"https://app.example.com/review?access_token=%5BREDACTED%5D&safe=1"`)) {
		t.Fatalf("expected sanitized referer in logs, got %s", logs)
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	h := SecurityHeaders()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if got := res.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
	if got := res.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected deny frame header, got %q", got)
	}
	if got := res.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("expected no-referrer policy, got %q", got)
	}
}

func TestNoStoreMiddleware(t *testing.T) {
	h := NoStore()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected no-store cache control, got %q", got)
	}
	if got := res.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("expected pragma no-cache, got %q", got)
	}
}

func TestResolveClientIPIgnoresSpoofedForwardedHeadersByDefault(t *testing.T) {
	h := ResolveClientIP(ClientIPConfig{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestClientIP(r); got != "203.0.113.10" {
			t.Fatalf("expected direct peer ip, got %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.77")
	req.Header.Set("X-Real-IP", "198.51.100.88")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
}

func TestResolveClientIPUsesForwardedHeadersFromTrustedProxy(t *testing.T) {
	h := ResolveClientIP(ClientIPConfig{
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []string{"10.0.0.0/8"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestClientIP(r); got != "198.51.100.77" {
			t.Fatalf("expected forwarded client ip, got %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.77, 10.9.8.7")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
}

func TestStatusRecorderWriteHeader(t *testing.T) {
	r := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	r.WriteHeader(http.StatusAccepted)
	if r.status != http.StatusAccepted {
		t.Fatalf("expected status to be recorded, got %d", r.status)
	}
}

func TestRateLimitMiddlewareRejectsRequestsOverLimit(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	h := RateLimit(RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 2,
		Now: func() time.Time {
			return now
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	req.RemoteAddr = "203.0.113.10:1234"

	for i := 0; i < 2; i++ {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req.Clone(req.Context()))
		if res.Code != http.StatusNoContent {
			t.Fatalf("request %d: expected 204, got %d", i+1, res.Code)
		}
	}

	blocked := httptest.NewRecorder()
	h.ServeHTTP(blocked, req.Clone(req.Context()))
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", blocked.Code)
	}
	if got := blocked.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}
	if got := blocked.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("expected retry-after 60, got %q", got)
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(blocked.Body.Bytes(), &payload); err != nil {
		t.Fatalf("parse rate limit body: %v", err)
	}
	if payload["error"]["code"] != "rate_limited" || payload["error"]["message"] != "too many requests" {
		t.Fatalf("unexpected rate limit payload: %+v", payload)
	}
}

func TestRateLimitMiddlewareResetsAfterWindow(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	h := RateLimit(RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 1,
		Now: func() time.Time {
			return now
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	req.RemoteAddr = "203.0.113.20:1234"

	first := httptest.NewRecorder()
	h.ServeHTTP(first, req.Clone(req.Context()))
	if first.Code != http.StatusNoContent {
		t.Fatalf("expected first request to pass, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, req.Clone(req.Context()))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be limited, got %d", second.Code)
	}
	if got := second.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("expected retry-after 60, got %q", got)
	}

	now = now.Add(45 * time.Second)

	thirdBlocked := httptest.NewRecorder()
	h.ServeHTTP(thirdBlocked, req.Clone(req.Context()))
	if thirdBlocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected request inside window to stay limited, got %d", thirdBlocked.Code)
	}
	if got := thirdBlocked.Header().Get("Retry-After"); got != "15" {
		t.Fatalf("expected retry-after 15 near window end, got %q", got)
	}

	now = now.Add(16 * time.Second)

	afterReset := httptest.NewRecorder()
	h.ServeHTTP(afterReset, req.Clone(req.Context()))
	if afterReset.Code != http.StatusNoContent {
		t.Fatalf("expected request after window reset to pass, got %d", afterReset.Code)
	}
}

func TestRateLimitMiddlewareTracksClientsSeparately(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	h := RateLimit(RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 1,
		Now: func() time.Time {
			return now
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	firstClient := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	firstClient.RemoteAddr = "203.0.113.30:1234"
	secondClient := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	secondClient.RemoteAddr = "203.0.113.31:5678"

	firstRes := httptest.NewRecorder()
	h.ServeHTTP(firstRes, firstClient)
	if firstRes.Code != http.StatusNoContent {
		t.Fatalf("expected first client request to pass, got %d", firstRes.Code)
	}

	secondRes := httptest.NewRecorder()
	h.ServeHTTP(secondRes, secondClient)
	if secondRes.Code != http.StatusNoContent {
		t.Fatalf("expected second client request to pass, got %d", secondRes.Code)
	}
}

func TestRateLimitMiddlewareIgnoresSpoofedForwardedHeadersWithoutTrustedProxy(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	h := ResolveClientIP(ClientIPConfig{})(RateLimit(RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 1,
		Now: func() time.Time {
			return now
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	firstReq.RemoteAddr = "203.0.113.40:1234"
	firstReq.Header.Set("X-Forwarded-For", "198.51.100.1")
	firstRes := httptest.NewRecorder()
	h.ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusNoContent {
		t.Fatalf("expected first request to pass, got %d", firstRes.Code)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	secondReq.RemoteAddr = "203.0.113.40:5678"
	secondReq.Header.Set("X-Forwarded-For", "198.51.100.2")
	secondRes := httptest.NewRecorder()
	h.ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusTooManyRequests {
		t.Fatalf("expected spoofed forwarded header not to bypass limiter, got %d", secondRes.Code)
	}
}

func TestRateLimitMiddlewareUsesTrustedProxyForwardedClientIP(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	h := ResolveClientIP(ClientIPConfig{
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []string{"10.0.0.0/8"},
	})(RateLimit(RateLimitConfig{
		Window:      time.Minute,
		MaxRequests: 1,
		Now: func() time.Time {
			return now
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	firstReq.RemoteAddr = "10.1.1.1:1234"
	firstReq.Header.Set("X-Forwarded-For", "198.51.100.10")
	firstRes := httptest.NewRecorder()
	h.ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusNoContent {
		t.Fatalf("expected first proxied request to pass, got %d", firstRes.Code)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	secondReq.RemoteAddr = "10.1.1.1:5678"
	secondReq.Header.Set("X-Forwarded-For", "198.51.100.11")
	secondRes := httptest.NewRecorder()
	h.ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusNoContent {
		t.Fatalf("expected second proxied client to get separate bucket, got %d", secondRes.Code)
	}
}

func TestConcurrencyLimitMiddlewareRejectsRequestsOverCapacity(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	h := ConcurrencyLimit(ConcurrencyLimitConfig{
		MaxConcurrent: 1,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))

	firstReq := httptest.NewRequest(http.MethodGet, "/heavy", nil)
	firstRes := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(firstRes, firstReq)
		close(done)
	}()

	<-started

	secondReq := httptest.NewRequest(http.MethodGet, "/heavy", nil)
	secondRes := httptest.NewRecorder()
	h.ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", secondRes.Code)
	}
	if got := secondRes.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}
	if got := secondRes.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("expected retry-after 1, got %q", got)
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(secondRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("parse overload body: %v", err)
	}
	if payload["error"]["code"] != "overloaded" || payload["error"]["message"] != "server is handling too many expensive requests" {
		t.Fatalf("unexpected overload payload: %+v", payload)
	}

	close(release)
	<-done
}

func TestConcurrencyLimitMiddlewareReleasesCapacityAfterCompletion(t *testing.T) {
	h := ConcurrencyLimit(ConcurrencyLimitConfig{
		MaxConcurrent: 1,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	firstReq := httptest.NewRequest(http.MethodGet, "/heavy", nil)
	firstRes := httptest.NewRecorder()
	h.ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusNoContent {
		t.Fatalf("expected first request 204, got %d", firstRes.Code)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/heavy", nil)
	secondRes := httptest.NewRecorder()
	h.ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusNoContent {
		t.Fatalf("expected second request 204 after release, got %d", secondRes.Code)
	}
}
