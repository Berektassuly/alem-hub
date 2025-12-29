// Package handlers contains HTTP handler interfaces and implementations.
package handlers

import (
	"context"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// HEALTH CHECK INTERFACES
// ══════════════════════════════════════════════════════════════════════════════

// HealthChecker defines the interface for health checking.
type HealthChecker interface {
	// Check performs a health check and returns the status.
	Check(ctx context.Context) HealthStatus

	// AddCheck adds a named health check function.
	AddCheck(name string, check HealthCheckFunc)

	// RemoveCheck removes a named health check.
	RemoveCheck(name string)
}

// HealthCheckFunc is a function that performs a single health check.
// It returns an error if the check fails.
type HealthCheckFunc func(ctx context.Context) error

// HealthStatus represents the overall health status of the service.
type HealthStatus struct {
	// Healthy indicates if the service is healthy overall.
	Healthy bool `json:"healthy"`

	// Ready indicates if the service is ready to accept requests.
	Ready bool `json:"ready"`

	// Message provides additional context about the health status.
	Message string `json:"message,omitempty"`

	// Checks contains individual health check results.
	Checks map[string]CheckResult `json:"checks,omitempty"`

	// Uptime is how long the service has been running.
	Uptime string `json:"uptime,omitempty"`

	// Timestamp is when the check was performed.
	Timestamp time.Time `json:"timestamp"`

	// Version is the service version.
	Version string `json:"version,omitempty"`
}

// CheckResult represents the result of a single health check.
type CheckResult struct {
	// Healthy indicates if this specific check passed.
	Healthy bool `json:"healthy"`

	// Message provides details about the check result.
	Message string `json:"message,omitempty"`

	// Duration is how long the check took.
	Duration string `json:"duration,omitempty"`

	// LastChecked is when this check was last performed.
	LastChecked time.Time `json:"last_checked,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// COMPOSITE HEALTH CHECKER
// ══════════════════════════════════════════════════════════════════════════════

// CompositeHealthChecker aggregates multiple health checks.
type CompositeHealthChecker struct {
	mu        sync.RWMutex
	checks    map[string]HealthCheckFunc
	startTime time.Time
	version   string
	timeout   time.Duration
}

// NewCompositeHealthChecker creates a new composite health checker.
func NewCompositeHealthChecker(version string) *CompositeHealthChecker {
	return &CompositeHealthChecker{
		checks:    make(map[string]HealthCheckFunc),
		startTime: time.Now(),
		version:   version,
		timeout:   5 * time.Second,
	}
}

// SetTimeout sets the timeout for individual health checks.
func (c *CompositeHealthChecker) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// AddCheck adds a named health check function.
func (c *CompositeHealthChecker) AddCheck(name string, check HealthCheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = check
}

// RemoveCheck removes a named health check.
func (c *CompositeHealthChecker) RemoveCheck(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.checks, name)
}

// Check performs all health checks and returns the aggregated status.
func (c *CompositeHealthChecker) Check(ctx context.Context) HealthStatus {
	c.mu.RLock()
	checks := make(map[string]HealthCheckFunc, len(c.checks))
	for name, check := range c.checks {
		checks[name] = check
	}
	c.mu.RUnlock()

	status := HealthStatus{
		Healthy:   true,
		Ready:     true,
		Checks:    make(map[string]CheckResult),
		Uptime:    time.Since(c.startTime).Round(time.Second).String(),
		Timestamp: time.Now().UTC(),
		Version:   c.version,
	}

	// If no checks are registered, just return healthy
	if len(checks) == 0 {
		status.Message = "No health checks registered"
		return status
	}

	// Run all checks
	var wg sync.WaitGroup
	results := make(chan struct {
		name   string
		result CheckResult
	}, len(checks))

	for name, check := range checks {
		wg.Add(1)
		go func(name string, check HealthCheckFunc) {
			defer wg.Done()

			// Create context with timeout
			checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
			defer cancel()

			start := time.Now()
			err := check(checkCtx)
			duration := time.Since(start)

			result := CheckResult{
				Healthy:     err == nil,
				Duration:    duration.Round(time.Millisecond).String(),
				LastChecked: time.Now().UTC(),
			}

			if err != nil {
				result.Message = err.Error()
			} else {
				result.Message = "OK"
			}

			results <- struct {
				name   string
				result CheckResult
			}{name, result}
		}(name, check)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var unhealthyChecks []string
	for r := range results {
		status.Checks[r.name] = r.result
		if !r.result.Healthy {
			status.Healthy = false
			status.Ready = false
			unhealthyChecks = append(unhealthyChecks, r.name)
		}
	}

	// Set message based on results
	if status.Healthy {
		status.Message = "All checks passed"
	} else {
		status.Message = "Some checks failed: " + joinStrings(unhealthyChecks, ", ")
	}

	return status
}

// ══════════════════════════════════════════════════════════════════════════════
// PREDEFINED HEALTH CHECKS
// ══════════════════════════════════════════════════════════════════════════════

// DatabaseChecker creates a health check for database connectivity.
type DatabaseChecker interface {
	Ping(ctx context.Context) error
}

// NewDatabaseCheck creates a database health check function.
func NewDatabaseCheck(db DatabaseChecker) HealthCheckFunc {
	return func(ctx context.Context) error {
		return db.Ping(ctx)
	}
}

// CacheChecker creates a health check for cache connectivity.
type CacheChecker interface {
	Ping(ctx context.Context) error
}

// NewCacheCheck creates a cache health check function.
func NewCacheCheck(cache CacheChecker) HealthCheckFunc {
	return func(ctx context.Context) error {
		return cache.Ping(ctx)
	}
}

// ExternalAPIChecker creates a health check for external API connectivity.
type ExternalAPIChecker interface {
	HealthCheck(ctx context.Context) error
}

// NewExternalAPICheck creates an external API health check function.
func NewExternalAPICheck(api ExternalAPIChecker) HealthCheckFunc {
	return func(ctx context.Context) error {
		return api.HealthCheck(ctx)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// WEBHOOK HANDLER INTERFACE
// ══════════════════════════════════════════════════════════════════════════════

// WebhookHandler defines the interface for handling webhooks.
type WebhookHandler interface {
	// HandleTelegramUpdate processes a Telegram webhook update.
	HandleTelegramUpdate(ctx context.Context, payload []byte) error
}

// ══════════════════════════════════════════════════════════════════════════════
// NOOP IMPLEMENTATIONS (for testing/default)
// ══════════════════════════════════════════════════════════════════════════════

// NoopHealthChecker always returns healthy status.
type NoopHealthChecker struct {
	startTime time.Time
}

// NewNoopHealthChecker creates a new noop health checker.
func NewNoopHealthChecker() *NoopHealthChecker {
	return &NoopHealthChecker{
		startTime: time.Now(),
	}
}

// Check always returns healthy status.
func (n *NoopHealthChecker) Check(ctx context.Context) HealthStatus {
	return HealthStatus{
		Healthy:   true,
		Ready:     true,
		Message:   "OK",
		Uptime:    time.Since(n.startTime).Round(time.Second).String(),
		Timestamp: time.Now().UTC(),
	}
}

// AddCheck is a no-op.
func (n *NoopHealthChecker) AddCheck(name string, check HealthCheckFunc) {}

// RemoveCheck is a no-op.
func (n *NoopHealthChecker) RemoveCheck(name string) {}

// NoopWebhookHandler discards all webhooks.
type NoopWebhookHandler struct{}

// NewNoopWebhookHandler creates a new noop webhook handler.
func NewNoopWebhookHandler() *NoopWebhookHandler {
	return &NoopWebhookHandler{}
}

// HandleTelegramUpdate is a no-op.
func (n *NoopWebhookHandler) HandleTelegramUpdate(ctx context.Context, payload []byte) error {
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// joinStrings joins a slice of strings with a separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
