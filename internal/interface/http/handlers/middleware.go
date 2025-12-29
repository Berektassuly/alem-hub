// Package handlers contains HTTP handler interfaces and implementations.
package handlers

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// AUTHENTICATION MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// APIKeyAuth provides API key authentication.
type APIKeyAuth struct {
	headerName string
	validKeys  map[string]bool
	mu         sync.RWMutex
}

// NewAPIKeyAuth creates a new API key authenticator.
func NewAPIKeyAuth(headerName string, keys []string) *APIKeyAuth {
	validKeys := make(map[string]bool, len(keys))
	for _, key := range keys {
		if key != "" {
			validKeys[key] = true
		}
	}

	return &APIKeyAuth{
		headerName: headerName,
		validKeys:  validKeys,
	}
}

// AddKey adds a valid API key.
func (a *APIKeyAuth) AddKey(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.validKeys[key] = true
}

// RemoveKey removes an API key.
func (a *APIKeyAuth) RemoveKey(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.validKeys, key)
}

// IsValid checks if an API key is valid.
func (a *APIKeyAuth) IsValid(key string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.validKeys[key]
}

// Middleware returns an HTTP middleware that checks for valid API keys.
func (a *APIKeyAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(a.headerName)

		// Also check Authorization header with Bearer scheme
		if key == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if key == "" {
			http.Error(w, `{"error":"missing_api_key","message":"API key is required"}`, http.StatusUnauthorized)
			return
		}

		if !a.IsValid(key) {
			http.Error(w, `{"error":"invalid_api_key","message":"Invalid API key"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// TIMEOUT MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// TimeoutMiddleware adds a timeout to request contexts.
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			// Use a channel to track completion
			done := make(chan struct{})

			go func() {
				next.ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				// Request completed normally
			case <-ctx.Done():
				// Timeout exceeded
				if ctx.Err() == context.DeadlineExceeded {
					http.Error(w, `{"error":"timeout","message":"Request timeout exceeded"}`, http.StatusGatewayTimeout)
				}
			}
		})
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// COMPRESSION MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// Note: For production, use a proper compression middleware like
// github.com/klauspost/compress/gzhttp

// CompressionMiddleware adds gzip compression hints.
// For full compression, use a dedicated library.
func CompressionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Vary", "Accept-Encoding")
		}
		next.ServeHTTP(w, r)
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// CACHE CONTROL MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// CacheControlMiddleware adds cache control headers.
func CacheControlMiddleware(maxAge time.Duration, private bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only cache GET requests
			if r.Method == http.MethodGet {
				directive := "public"
				if private {
					directive = "private"
				}
				w.Header().Set("Cache-Control",
					directive+", max-age="+formatSeconds(maxAge))
			} else {
				w.Header().Set("Cache-Control", "no-store")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NoCacheMiddleware prevents caching.
func NoCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// SECURITY HEADERS MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// SecurityHeadersMiddleware adds security-related headers.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// XSS protection (legacy, but still useful)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content security policy for API
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		next.ServeHTTP(w, r)
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// REQUEST SIZE LIMIT MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// RequestSizeLimitMiddleware limits the size of request bodies.
func RequestSizeLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				http.Error(w, `{"error":"payload_too_large","message":"Request body too large"}`,
					http.StatusRequestEntityTooLarge)
				return
			}

			// Also limit the actual body reading
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

			next.ServeHTTP(w, r)
		})
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// METHOD OVERRIDE MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// MethodOverrideMiddleware allows HTTP method override via header or query param.
func MethodOverrideMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only override for POST requests
		if r.Method == http.MethodPost {
			// Check header first
			override := r.Header.Get("X-HTTP-Method-Override")
			if override == "" {
				// Check query parameter
				override = r.URL.Query().Get("_method")
			}

			if override != "" {
				override = strings.ToUpper(override)
				switch override {
				case http.MethodPut, http.MethodPatch, http.MethodDelete:
					r.Method = override
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// CONTEXT INJECTION MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// ContextKey is a type for context keys.
type ContextKey string

const (
	// ContextKeyUserID is the context key for user ID.
	ContextKeyUserID ContextKey = "user_id"
	// ContextKeyRequestStart is the context key for request start time.
	ContextKeyRequestStart ContextKey = "request_start"
)

// InjectContextMiddleware injects common values into request context.
func InjectContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Inject request start time
		ctx = context.WithValue(ctx, ContextKeyRequestStart, time.Now())

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// formatSeconds formats a duration as seconds for cache headers.
func formatSeconds(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 0 {
		secs = 0
	}

	// Convert to string manually
	if secs == 0 {
		return "0"
	}

	digits := make([]byte, 0, 20)
	for secs > 0 {
		digits = append([]byte{byte('0' + secs%10)}, digits...)
		secs /= 10
	}
	return string(digits)
}

// ══════════════════════════════════════════════════════════════════════════════
// MIDDLEWARE CHAIN BUILDER
// ══════════════════════════════════════════════════════════════════════════════

// MiddlewareFunc is a function that wraps an http.Handler.
type MiddlewareFunc func(http.Handler) http.Handler

// Chain chains multiple middleware functions.
func Chain(middlewares ...MiddlewareFunc) MiddlewareFunc {
	return func(final http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// ChainHandler chains middleware and wraps a final handler.
func ChainHandler(handler http.Handler, middlewares ...MiddlewareFunc) http.Handler {
	return Chain(middlewares...)(handler)
}
