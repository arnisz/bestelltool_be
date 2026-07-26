package httpadapter

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
)

func TestRateLimiterSourceIPDoesNotTrustHeadersByDefault_SEC05(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute, false, time.Now)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "198.51.100.10:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.8")
	req.Header.Set("X-Real-IP", "203.0.113.9")

	if got := limiter.sourceIP(req); got != "198.51.100.10" {
		t.Fatalf("sourceIP() = %q, want socket peer", got)
	}
}

func TestRateLimiterSourceIPTrustsProxyHeadersWhenEnabled_SEC05(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute, true, time.Now)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "198.51.100.10:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.8, 198.51.100.10")

	if got := limiter.sourceIP(req); got != "203.0.113.8" {
		t.Fatalf("sourceIP() = %q, want trusted forwarded client", got)
	}
}

func TestRateLimiterLoginReturns429WithRetryAfter_SEC05(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(1, time.Minute, false, func() time.Time { return now })
	var calls int
	h := limiter.limitLogin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	first := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"secret"}`))
	first.RemoteAddr = "198.51.100.10:4321"
	firstRec := httptest.NewRecorder()
	h.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want 204", firstRec.Code)
	}

	second := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"secret"}`))
	second.RemoteAddr = "198.51.100.10:4321"
	secondRec := httptest.NewRecorder()
	h.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", secondRec.Code)
	}
	if got := secondRec.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
	if calls != 1 {
		t.Fatalf("next handler calls = %d, want 1", calls)
	}
	if _, err := limiter.allow("login:ip:198.51.100.10"); !errors.Is(err, ports.ErrThrottled) {
		t.Fatalf("allow() error = %v, want ErrThrottled", err)
	}
}
