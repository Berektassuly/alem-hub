// Package http implements REST API and Webhook endpoints for Alem Community Hub.
// This package provides an optional HTTP interface for external integrations,
// health checks, and administrative APIs.
package http

import (
	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"github.com/alem-hub/alem-community-hub/internal/interface/http/handlers"
	"github.com/alem-hub/alem-community-hub/pkg/logger"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// SERVER CONFIGURATION
// ══════════════════════════════════════════════════════════════════════════════

// Config contains HTTP server configuration.
type Config struct {
	// Host - address to bind (default: "0.0.0.0").
	Host string

	// Port - port to listen on (default: 8080).
	Port int

	// ReadTimeout - maximum duration for reading the entire request.
	ReadTimeout time.Duration

	// WriteTimeout - maximum duration for writing the response.
	WriteTimeout time.Duration

	// IdleTimeout - maximum duration for idle connections.
	IdleTimeout time.Duration

	// MaxHeaderBytes - maximum size of request headers.
	MaxHeaderBytes int

	// EnableCORS - enable CORS headers.
	EnableCORS bool

	// AllowedOrigins - allowed origins for CORS.
	AllowedOrigins []string

	// EnableMetrics - enable Prometheus metrics endpoint.
	EnableMetrics bool

	// EnablePprof - enable pprof debug endpoints.
	EnablePprof bool

	// RateLimitPerMinute - requests per minute per IP (0 = disabled).
	RateLimitPerMinute int

	// TrustedProxies - list of trusted proxy IPs for X-Forwarded-For.
	TrustedProxies []string

	// APIKeyHeader - header name for API key authentication.
	APIKeyHeader string

	// APIKeys - valid API keys for authenticated endpoints.
	APIKeys []string

	// WebhookSecret - secret for validating webhook requests.
	WebhookSecret string
}

// DefaultConfig returns default server configuration.
func DefaultConfig() Config {
	return Config{
		Host:               "0.0.0.0",
		Port:               8080,
		ReadTimeout:        15 * time.Second,
		WriteTimeout:       15 * time.Second,
		IdleTimeout:        60 * time.Second,
		MaxHeaderBytes:     1 << 20, // 1 MB
		EnableCORS:         true,
		AllowedOrigins:     []string{"*"},
		EnableMetrics:      true,
		EnablePprof:        false,
		RateLimitPerMinute: 100,
		APIKeyHeader:       "X-API-Key",
		APIKeys:            []string{},
	}
}

// Address returns the server address string.
func (c Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// ══════════════════════════════════════════════════════════════════════════════
// DEPENDENCIES
// ══════════════════════════════════════════════════════════════════════════════

// Dependencies contains all dependencies required by HTTP handlers.
type Dependencies struct {
	// Query Handlers (CQRS Read Side)
	GetLeaderboardHandler   *query.GetLeaderboardHandler
	GetStudentRankHandler   *query.GetStudentRankHandler
	GetOnlineNowHandler     *query.GetOnlineNowHandler
	GetNeighborsHandler     *query.GetNeighborsHandler
	GetDailyProgressHandler *query.GetDailyProgressHandler
	FindHelpersHandler      *query.FindHelpersHandler

	// Logger
	Logger *logger.Logger

	// Health Check Dependencies
	HealthChecker handlers.HealthChecker

	// Webhook Handler (for Telegram)
	WebhookHandler handlers.WebhookHandler
}

// ══════════════════════════════════════════════════════════════════════════════
// SERVER
// ══════════════════════════════════════════════════════════════════════════════

// Server represents the HTTP server.
type Server struct {
	config     Config
	deps       Dependencies
	httpServer *http.Server
	router     *http.ServeMux
	logger     *logger.Logger

	// Middleware state
	rateLimiter *rateLimiter

	// Server state
	mu        sync.RWMutex
	running   bool
	startedAt time.Time
}

// NewServer creates a new HTTP server with the given configuration and dependencies.
func NewServer(config Config, deps Dependencies) *Server {
	s := &Server{
		config: config,
		deps:   deps,
		router: http.NewServeMux(),
		logger: deps.Logger,
	}

	if s.logger == nil {
		s.logger = logger.Default()
	}

	// Initialize rate limiter
	if config.RateLimitPerMinute > 0 {
		s.rateLimiter = newRateLimiter(config.RateLimitPerMinute, time.Minute)
	}

	// Setup routes
	s.setupRoutes()

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:           config.Address(),
		Handler:        s.buildMiddlewareChain(s.router),
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		IdleTimeout:    config.IdleTimeout,
		MaxHeaderBytes: config.MaxHeaderBytes,
	}

	return s
}

// ══════════════════════════════════════════════════════════════════════════════
// ROUTING
// ══════════════════════════════════════════════════════════════════════════════

// setupRoutes configures all HTTP routes.
func (s *Server) setupRoutes() {
	// ─────────────────────────────────────────────────────────────────────────
	// Health & Status Endpoints
	// ─────────────────────────────────────────────────────────────────────────
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("GET /healthz", s.handleHealth) // Kubernetes alias
	s.router.HandleFunc("GET /ready", s.handleReady)
	s.router.HandleFunc("GET /live", s.handleLive)
	s.router.HandleFunc("GET /", s.handleRoot)

	// ─────────────────────────────────────────────────────────────────────────
	// API v1 - Public Endpoints
	// ─────────────────────────────────────────────────────────────────────────
	s.router.HandleFunc("GET /api/v1/leaderboard", s.handleGetLeaderboard)
	s.router.HandleFunc("GET /api/v1/leaderboard/{cohort}", s.handleGetLeaderboardByCohort)
	s.router.HandleFunc("GET /api/v1/students/online", s.handleGetOnline)
	s.router.HandleFunc("GET /api/v1/students/{id}", s.handleGetStudent)
	s.router.HandleFunc("GET /api/v1/students/{id}/rank", s.handleGetStudentRank)
	s.router.HandleFunc("GET /api/v1/students/{id}/neighbors", s.handleGetStudentNeighbors)
	s.router.HandleFunc("GET /api/v1/students/{id}/progress", s.handleGetStudentProgress)
	s.router.HandleFunc("GET /api/v1/helpers", s.handleFindHelpers)
	s.router.HandleFunc("GET /api/v1/stats", s.handleGetStats)

	// ─────────────────────────────────────────────────────────────────────────
	// Webhook Endpoints (Telegram)
	// ─────────────────────────────────────────────────────────────────────────
	s.router.HandleFunc("POST /webhook/telegram", s.handleTelegramWebhook)
	s.router.HandleFunc("POST /webhook/telegram/{token}", s.handleTelegramWebhookWithToken)

	// ─────────────────────────────────────────────────────────────────────────
	// Metrics (if enabled)
	// ─────────────────────────────────────────────────────────────────────────
	if s.config.EnableMetrics {
		s.router.HandleFunc("GET /metrics", s.handleMetrics)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// MIDDLEWARE CHAIN
// ══════════════════════════════════════════════════════════════════════════════

// buildMiddlewareChain wraps the router with all middleware.
func (s *Server) buildMiddlewareChain(handler http.Handler) http.Handler {
	// Apply middleware in reverse order (last middleware wraps first)
	h := handler

	// Request ID middleware
	h = s.requestIDMiddleware(h)

	// Logging middleware
	h = s.loggingMiddleware(h)

	// Recovery middleware (must be early to catch panics)
	h = s.recoveryMiddleware(h)

	// CORS middleware
	if s.config.EnableCORS {
		h = s.corsMiddleware(h)
	}

	// Rate limiting middleware
	if s.rateLimiter != nil {
		h = s.rateLimitMiddleware(h)
	}

	return h
}

// requestIDMiddleware adds a unique request ID to each request.
func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), contextKeyRequestID, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingMiddleware logs all HTTP requests.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)

		s.logger.Info("http request",
			logger.String("method", r.Method),
			logger.String("path", r.URL.Path),
			logger.Int("status", rw.statusCode),
			logger.Int64("duration_ms", duration.Milliseconds()),
			logger.String("ip", getClientIP(r)),
			logger.String("user_agent", r.UserAgent()),
			logger.String("request_id", getRequestID(r.Context())),
		)
	})
}

// recoveryMiddleware recovers from panics and returns 500.
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()
				s.logger.Error("panic recovered",
					logger.Any("error", err),
					logger.String("stack", string(stack)),
					logger.String("path", r.URL.Path),
					logger.String("request_id", getRequestID(r.Context())),
				)
				writeJSONError(w, http.StatusInternalServerError, "internal_server_error", "An unexpected error occurred")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, o := range s.config.AllowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware implements per-IP rate limiting.
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		if !s.rateLimiter.Allow(ip) {
			w.Header().Set("Retry-After", "60")
			writeJSONError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many requests, please try again later")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// SERVER LIFECYCLE
// ══════════════════════════════════════════════════════════════════════════════

// Start starts the HTTP server.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.startedAt = time.Now()
	s.mu.Unlock()

	s.logger.Info("starting HTTP server", logger.String("address", s.config.Address()))

	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// StartAsync starts the server in a goroutine.
func (s *Server) StartAsync() <-chan error {
	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errCh <- err
		}
		close(errCh)
	}()
	return errCh
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}

// IsRunning returns true if the server is running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Uptime returns the server uptime.
func (s *Server) Uptime() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.running {
		return 0
	}
	return time.Since(s.startedAt)
}

// Address returns the server address.
func (s *Server) Address() string {
	return s.config.Address()
}

// ══════════════════════════════════════════════════════════════════════════════
// RESPONSE HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// JSONResponse represents a standard JSON response.
type JSONResponse struct {
	Success   bool          `json:"success"`
	Data      interface{}   `json:"data,omitempty"`
	Error     *APIError     `json:"error,omitempty"`
	Meta      *ResponseMeta `json:"meta,omitempty"`
	RequestID string        `json:"request_id,omitempty"`
}

// APIError represents an API error.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// ResponseMeta contains response metadata.
type ResponseMeta struct {
	Timestamp  time.Time `json:"timestamp"`
	Version    string    `json:"version,omitempty"`
	TotalCount int       `json:"total_count,omitempty"`
	Page       int       `json:"page,omitempty"`
	PageSize   int       `json:"page_size,omitempty"`
	HasMore    bool      `json:"has_more,omitempty"`
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	response := JSONResponse{
		Success: status >= 200 && status < 300,
		Data:    data,
		Meta: &ResponseMeta{
			Timestamp: time.Now().UTC(),
			Version:   "v1",
		},
	}

	_ = json.NewEncoder(w).Encode(response)
}

// writeJSONWithMeta writes a JSON response with custom metadata.
func writeJSONWithMeta(w http.ResponseWriter, r *http.Request, status int, data interface{}, meta *ResponseMeta) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if meta == nil {
		meta = &ResponseMeta{}
	}
	meta.Timestamp = time.Now().UTC()
	meta.Version = "v1"

	response := JSONResponse{
		Success:   status >= 200 && status < 300,
		Data:      data,
		Meta:      meta,
		RequestID: getRequestID(r.Context()),
	}

	_ = json.NewEncoder(w).Encode(response)
}

// writeJSONError writes an error JSON response.
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	response := JSONResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
		Meta: &ResponseMeta{
			Timestamp: time.Now().UTC(),
		},
	}

	_ = json.NewEncoder(w).Encode(response)
}

// writeJSONErrorWithDetails writes an error JSON response with details.
func writeJSONErrorWithDetails(w http.ResponseWriter, status int, code, message, details string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	response := JSONResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: &ResponseMeta{
			Timestamp: time.Now().UTC(),
		},
	}

	_ = json.NewEncoder(w).Encode(response)
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER TYPES AND FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

type contextKey string

const contextKeyRequestID contextKey = "request_id"

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// getRequestID extracts the request ID from context.
func getRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(contextKeyRequestID).(string); ok {
		return id
	}
	return ""
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond()%1000)
}

// getQueryParam extracts a query parameter with a default value.
func getQueryParam(r *http.Request, key, defaultValue string) string {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getQueryParamInt extracts an integer query parameter with a default value.
func getQueryParamInt(r *http.Request, key string, defaultValue int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}

	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

// getQueryParamBool extracts a boolean query parameter.
func getQueryParamBool(r *http.Request, key string) bool {
	value := strings.ToLower(r.URL.Query().Get(key))
	return value == "true" || value == "1" || value == "yes"
}

// ══════════════════════════════════════════════════════════════════════════════
// RATE LIMITER
// ══════════════════════════════════════════════════════════════════════════════

type rateLimiter struct {
	mu       sync.RWMutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

func (rl *rateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Get current requests for this key
	requests := rl.requests[key]

	// Filter out old requests
	var valid []time.Time
	for _, t := range requests {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}

	// Check if under limit
	if len(valid) >= rl.limit {
		rl.requests[key] = valid
		return false
	}

	// Add new request
	rl.requests[key] = append(valid, now)
	return true
}

func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		windowStart := now.Add(-rl.window)

		for key, requests := range rl.requests {
			var valid []time.Time
			for _, t := range requests {
				if t.After(windowStart) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = valid
			}
		}
		rl.mu.Unlock()
	}
}
