package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			reqID = hex.EncodeToString(buf)
		}

		w.Header().Set("X-Request-ID", reqID)
		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CORS(allowedOrigins string) func(http.Handler) http.Handler {
	origins := strings.Split(allowedOrigins, ",")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			allow := "*"

			if allowedOrigins != "*" && allowedOrigins != "" {
				allow = ""
				for _, o := range origins {
					o = strings.TrimSpace(o)
					if o == origin {
						allow = origin
						break
					}
				}
			}

			if allow != "" {
				w.Header().Set("Access-Control-Allow-Origin", allow)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, Accept")
			}

			if r.Method == http.MethodOptions {
				if allow == "" {
					http.Error(w, "origin not allowed", http.StatusForbidden)
					return
				}
				w.Header().Add("Vary", "Origin")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rl.limit <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		ip := r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			ip = ip[:idx]
		}

		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)

		// Filter out old timestamps
		var valid []time.Time
		for _, t := range rl.requests[ip] {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= rl.limit {
			rl.requests[ip] = valid
			rl.mu.Unlock()
			http.Error(w, `{"error":{"code":"RATE_LIMIT_EXCEEDED","message":"too many requests"}}`, http.StatusTooManyRequests)
			return
		}

		rl.requests[ip] = append(valid, now)
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID, _ := r.Context().Value(RequestIDKey).(string)

			next.ServeHTTP(w, r)

			logger.Debug("http request",
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}
