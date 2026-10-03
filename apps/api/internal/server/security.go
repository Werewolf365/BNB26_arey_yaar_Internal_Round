// Package server security: API auth, rate limiting, and HTTP hardening.
//
// Posture is env-driven (see ConfigFromEnv). The zero Config is open local-dev
// mode: no keys required, no rate limiting, no CORS. Production sets
// QUORUM_API_KEYS (and optionally rate/CORS knobs); admin writes then require
// a key. Reads stay open locally per docs/api.md. Secrets are never logged.
package server

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config controls the API security posture.
type Config struct {
	// APIKeys gates administrative writes (builders POST/PATCH, policies
	// POST). Empty = open (local dev only; startup logs a warning).
	APIKeys []string
	// RateLimitRPS caps requests per client IP. <= 0 = disabled.
	RateLimitRPS float64
	// RateBurst is the per-IP bucket size. <= 0 defaults to 10 when limiting.
	RateBurst int
	// CORSOrigins allowlists origins for CORS headers. Empty = none sent.
	CORSOrigins []string
	// MaxBodyBytes caps JSON request bodies. <= 0 defaults to 1 MiB.
	MaxBodyBytes int64
}

// DefaultMaxBodyBytes bounds every JSON request body.
const DefaultMaxBodyBytes = 1 << 20

// ConfigFromEnv reads the security posture from the environment:
//
//	QUORUM_API_KEYS        comma-separated keys for admin writes (empty = open)
//	QUORUM_RATE_LIMIT_RPS  requests/sec per IP (empty/0 = disabled)
//	QUORUM_RATE_BURST      bucket size (default 10)
//	QUORUM_CORS_ORIGINS    comma-separated origins (empty = no CORS headers)
func ConfigFromEnv() Config {
	var c Config
	if v := os.Getenv("QUORUM_API_KEYS"); v != "" {
		for _, k := range strings.Split(v, ",") {
			if k = strings.TrimSpace(k); k != "" {
				c.APIKeys = append(c.APIKeys, k)
			}
		}
	}
	if v := os.Getenv("QUORUM_RATE_LIMIT_RPS"); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 {
			c.RateLimitRPS = f
		}
	}
	if v := os.Getenv("QUORUM_RATE_BURST"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			c.RateBurst = n
		}
	}
	if v := os.Getenv("QUORUM_CORS_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.CORSOrigins = append(c.CORSOrigins, o)
			}
		}
	}
	return c
}

func (c Config) maxBody() int64 {
	if c.MaxBodyBytes > 0 {
		return c.MaxBodyBytes
	}
	return DefaultMaxBodyBytes
}

func (c Config) burst() int {
	if c.RateBurst > 0 {
		return c.RateBurst
	}
	return 10
}

// adminWrite reports whether this request mutates administrative state
// (docs/api.md: auth for administrative writes; reads stay open).
func adminWrite(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && p == "/api/v1/builders":
		return true
	case r.Method == http.MethodPatch && strings.HasPrefix(p, "/api/v1/builders/"):
		return true
	case r.Method == http.MethodPost && p == "/api/v1/policies":
		return true
	default:
		return false
	}
}

// checkAuth validates the presented key without leaking it into errors/logs.
func (c Config) checkAuth(r *http.Request) error {
	if len(c.APIKeys) == 0 {
		return nil // open local-dev mode
	}
	got := r.Header.Get("X-Api-Key")
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		got = strings.TrimPrefix(auth, "Bearer ")
	}
	if got == "" {
		return fmt.Errorf("UNAUTHENTICATED: admin writes require an API key")
	}
	for _, k := range c.APIKeys {
		if subtle.ConstantTimeCompare([]byte(got), []byte(k)) == 1 {
			return nil
		}
	}
	return fmt.Errorf("FORBIDDEN: presented API key is not authorized")
}

// --- per-IP token-bucket rate limiter (stdlib only) ---

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rps     float64
	burst   int
}

func newRateLimiter(rps float64, burst int) *rateLimiter {
	return &rateLimiter{buckets: map[string]*bucket{}, rps: rps, burst: burst}
}

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// allow consumes one token; false means the caller is over budget.
func (l *rateLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Opportunistic cleanup so the map cannot grow without bound.
	if len(l.buckets) > 4096 {
		for k, b := range l.buckets {
			if now.Sub(b.lastSeen) > time.Minute {
				delete(l.buckets, k)
			}
		}
	}
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: float64(l.burst), lastSeen: now}
		l.buckets[ip] = b
	}
	elapsed := now.Sub(b.lastSeen).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rps
		if b.tokens > float64(l.burst) {
			b.tokens = float64(l.burst)
		}
		b.lastSeen = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// --- middleware chain ---

// secure wraps the mux with headers, CORS, rate limiting, and auth.
// Order matters: cheap observable headers first, auth last before dispatch.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if origin := r.Header.Get("Origin"); origin != "" && originAllowed(s.cfg.CORSOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, X-Api-Key, Authorization")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if s.limiter != nil && !s.limiter.allow(clientIP(r), time.Now()) {
			writeErr(w, r, fmt.Errorf("RATE_LIMITED: too many requests"))
			return
		}
		if adminWrite(r) {
			if err := s.cfg.checkAuth(r); err != nil {
				writeErr(w, r, err)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func originAllowed(allowed []string, origin string) bool {
	for _, o := range allowed {
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}
