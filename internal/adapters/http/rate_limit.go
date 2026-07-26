package httpadapter

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bestelltool_be/internal/application/ports"
)

const maxRateLimitEntries = 10_000

type rateLimitEntry struct {
	windowStartedAt time.Time
	requests        int
}

// RateLimiter limits authentication attempts within fixed time windows.
// It is deliberately in-memory: limits are per server instance.
type RateLimiter struct {
	mu                sync.Mutex
	entries           map[string]rateLimitEntry
	maxRequests       int
	window            time.Duration
	trustProxyHeaders bool
	now               func() time.Time
}

func NewRateLimiter(maxRequests int, window time.Duration, trustProxyHeaders bool, now func() time.Time) *RateLimiter {
	if now == nil {
		now = time.Now
	}
	return &RateLimiter{
		entries:           make(map[string]rateLimitEntry),
		maxRequests:       maxRequests,
		window:            window,
		trustProxyHeaders: trustProxyHeaders,
		now:               now,
	}
}

func (l *RateLimiter) allow(key string) (time.Duration, error) {
	if l == nil || l.maxRequests <= 0 || l.window <= 0 {
		return 0, nil
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok || !now.Before(entry.windowStartedAt.Add(l.window)) {
		if len(l.entries) >= maxRateLimitEntries {
			for existingKey, existing := range l.entries {
				if !now.Before(existing.windowStartedAt.Add(l.window)) {
					delete(l.entries, existingKey)
				}
			}
		}
		l.entries[key] = rateLimitEntry{windowStartedAt: now, requests: 1}
		return 0, nil
	}
	if entry.requests >= l.maxRequests {
		return entry.windowStartedAt.Add(l.window).Sub(now), ports.ErrThrottled
	}
	entry.requests++
	l.entries[key] = entry
	return 0, nil
}

func (l *RateLimiter) sourceIP(r *http.Request) string {
	if l != nil && l.trustProxyHeaders {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			if ip := strings.TrimSpace(strings.SplitN(forwarded, ",", 2)[0]); net.ParseIP(ip) != nil {
				return ip
			}
		}
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (l *RateLimiter) limitRefresh(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retryAfter, err := l.allow("refresh:ip:" + l.sourceIP(r)); err != nil {
			writeThrottled(w, retryAfter)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) limitLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeBadRequest(w)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))

		keys := []string{"login:ip:" + l.sourceIP(r)}
		var payload loginPayload
		if json.Unmarshal(body, &payload) == nil && strings.TrimSpace(payload.Username) != "" {
			keys = append(keys, "login:account:"+strings.ToLower(strings.TrimSpace(payload.Username)))
		}
		for _, key := range keys {
			if retryAfter, err := l.allow(key); err != nil {
				writeThrottled(w, retryAfter)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeThrottled(w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Round(time.Second)/time.Second), 10))
	writeJSON(w, http.StatusTooManyRequests, errorEnvelope{Error: errorBody{Code: "throttled", Message: "too many requests"}})
}
