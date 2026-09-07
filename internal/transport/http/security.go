package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter limits requests by key.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// MemoryRateLimiter is a simple in-process fixed-window limiter.
type MemoryRateLimiter struct {
	mu   sync.Mutex
	hits map[string]bucket
}

type bucket struct {
	count int
	reset time.Time
}

// NewMemoryRateLimiter creates an in-memory rate limiter.
func NewMemoryRateLimiter() *MemoryRateLimiter {
	return &MemoryRateLimiter{hits: make(map[string]bucket)}
}

func (m *MemoryRateLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.hits[key]
	if !ok || now.After(b.reset) {
		m.hits[key] = bucket{count: 1, reset: now.Add(window)}
		return true, nil
	}
	if b.count >= limit {
		return false, nil
	}
	b.count++
	m.hits[key] = b
	return true, nil
}

func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	allowAll := false
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAll = true
		}
		if o != "" {
			allowed[o] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || len(allowed) == 0 || containsOrigin(allowed, origin)) {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if len(allowed) == 0 {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Grafana-Signature")
				w.Header().Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func containsOrigin(allowed map[string]struct{}, origin string) bool {
	_, ok := allowed[origin]
	return ok
}

func apiKeyMiddleware(apiKey string) func(http.Handler) http.Handler {
	apiKey = strings.TrimSpace(apiKey)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}
			path := r.URL.Path
			if path == "/api/v1/health/live" || path == "/api/v1/health" || path == "/api/v1/metrics" {
				next.ServeHTTP(w, r)
				return
			}
			provided := r.Header.Get("X-API-Key")
			if provided == "" {
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
					provided = strings.TrimSpace(auth[7:])
				}
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid API key required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func rateLimitMiddleware(limiter RateLimiter, bucket string, limitPerMinute int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || limitPerMinute <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			ip := r.RemoteAddr
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = strings.TrimSpace(strings.Split(fwd, ",")[0])
			}
			ok, err := limiter.Allow(r.Context(), bucket+":"+ip, limitPerMinute, time.Minute)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			if !ok {
				writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func grafanaWebhookAuthMiddleware(secret string) func(http.Handler) http.Handler {
	secret = strings.TrimSpace(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				next.ServeHTTP(w, r)
				return
			}
			sig := r.Header.Get("X-Grafana-Signature")
			if sig == "" {
				sig = r.Header.Get("X-Signature")
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_BODY", "Unable to read request body")
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write(body)
			expected := hex.EncodeToString(mac.Sum(nil))
			if subtle.ConstantTimeCompare([]byte(strings.ToLower(sig)), []byte(strings.ToLower(expected))) != 1 {
				writeError(w, http.StatusUnauthorized, "INVALID_SIGNATURE", "Invalid webhook signature")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
