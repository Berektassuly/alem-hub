// Package alem implements Alem Platform API client.
package alem

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// RATE LIMITER - Token Bucket implementation
// ══════════════════════════════════════════════════════════════════════════════

// RateLimiter implements the Token Bucket algorithm to control request rate.
// This is essential for protecting against API blocking when syncing with Alem.
type RateLimiter struct {
	mu sync.Mutex

	// Configuration
	maxTokens        float64       // Maximum tokens in the bucket
	refillRate       float64       // Tokens added per second
	tokens           float64       // Current token count
	lastRefill       time.Time     // Last time tokens were added
	minInterval      time.Duration // Minimum interval between requests
	lastRequest      time.Time     // Time of last request
	waitTimeout      time.Duration // Maximum time to wait for a token
	retryAfter       time.Duration // How long to wait after rate limit hit
	consecutiveWaits int           // Track consecutive waits for adaptive backoff
}

// RateLimiterConfig contains configuration for the rate limiter.
type RateLimiterConfig struct {
	// RequestsPerSecond is the maximum sustained request rate
	RequestsPerSecond float64

	// BurstSize is the maximum number of requests that can be made in a burst
	BurstSize int

	// MinInterval is the minimum time between requests (even with tokens available)
	MinInterval time.Duration

	// WaitTimeout is the maximum time to wait for a token
	WaitTimeout time.Duration

	// RetryAfter is the default retry time when rate limited
	RetryAfter time.Duration
}

// DefaultRateLimiterConfig returns conservative defaults for Alem API.
// These values are designed to be safe for an unofficial API client.
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerSecond: 2.0,                    // 2 requests per second sustained
		BurstSize:         5,                      // Allow small bursts
		MinInterval:       200 * time.Millisecond, // At least 200ms between requests
		WaitTimeout:       30 * time.Second,       // Wait up to 30 seconds for a token
		RetryAfter:        60 * time.Second,       // Wait 60 seconds if rate limited
	}
}

// AggressiveRateLimiterConfig returns more aggressive settings.
// Use only if you're sure the API can handle it.
func AggressiveRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerSecond: 5.0,
		BurstSize:         10,
		MinInterval:       100 * time.Millisecond,
		WaitTimeout:       15 * time.Second,
		RetryAfter:        30 * time.Second,
	}
}

// ConservativeRateLimiterConfig returns very conservative settings.
// Use when you want to minimize the risk of being blocked.
func ConservativeRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerSecond: 0.5,
		BurstSize:         2,
		MinInterval:       1 * time.Second,
		WaitTimeout:       60 * time.Second,
		RetryAfter:        120 * time.Second,
	}
}

// NewRateLimiter creates a new RateLimiter with the given configuration.
func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	now := time.Now()
	return &RateLimiter{
		maxTokens:   float64(config.BurstSize),
		refillRate:  config.RequestsPerSecond,
		tokens:      float64(config.BurstSize), // Start with full bucket
		lastRefill:  now,
		minInterval: config.MinInterval,
		lastRequest: now.Add(-config.MinInterval), // Allow immediate first request
		waitTimeout: config.WaitTimeout,
		retryAfter:  config.RetryAfter,
	}
}

// RateLimitError is returned when rate limit is exceeded.
type RateLimitError struct {
	// RetryAfter is the suggested time to wait before retrying
	RetryAfter time.Duration

	// Message provides additional context
	Message string
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	return e.Message
}

// Is implements errors.Is interface.
func (e *RateLimitError) Is(target error) bool {
	_, ok := target.(*RateLimitError)
	return ok
}

var (
	// ErrRateLimitExceeded is returned when rate limit is exceeded and timeout is reached.
	ErrRateLimitExceeded = &RateLimitError{Message: "rate limit exceeded"}

	// ErrRateLimitWaitTimeout is returned when waiting for rate limit times out.
	ErrRateLimitWaitTimeout = &RateLimitError{Message: "timeout waiting for rate limit"}
)

// Allow checks if a request is allowed and blocks until it is or timeout.
// Returns nil if the request can proceed, or an error if rate limited.
func (rl *RateLimiter) Allow(ctx context.Context) error {
	deadline := time.Now().Add(rl.waitTimeout)

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if we can proceed
		waitTime, ok := rl.tryAcquire()
		if ok {
			return nil
		}

		// Check timeout
		if time.Now().Add(waitTime).After(deadline) {
			return &RateLimitError{
				RetryAfter: waitTime,
				Message:    "rate limit exceeded, retry after " + waitTime.String(),
			}
		}

		// Wait and retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue to retry
		}
	}
}

// TryAllow attempts to get permission for a request without blocking.
// Returns true if the request can proceed, false otherwise.
func (rl *RateLimiter) TryAllow() bool {
	_, ok := rl.tryAcquire()
	return ok
}

// WaitTime returns how long to wait before the next request can be made.
func (rl *RateLimiter) WaitTime() time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refillTokens()

	// Check minimum interval
	timeSinceLastRequest := time.Since(rl.lastRequest)
	if timeSinceLastRequest < rl.minInterval {
		return rl.minInterval - timeSinceLastRequest
	}

	// Check token availability
	if rl.tokens >= 1.0 {
		return 0
	}

	// Calculate time until next token
	tokensNeeded := 1.0 - rl.tokens
	return time.Duration(tokensNeeded / rl.refillRate * float64(time.Second))
}

// tryAcquire attempts to acquire a token without blocking.
// Returns (waitTime, success). If success is false, waitTime indicates
// how long to wait before retrying.
func (rl *RateLimiter) tryAcquire() (time.Duration, bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refillTokens()

	// Check minimum interval
	timeSinceLastRequest := time.Since(rl.lastRequest)
	if timeSinceLastRequest < rl.minInterval {
		waitTime := rl.minInterval - timeSinceLastRequest
		return waitTime, false
	}

	// Check token availability
	if rl.tokens < 1.0 {
		// Calculate time until next token with adaptive backoff
		tokensNeeded := 1.0 - rl.tokens
		baseWait := time.Duration(tokensNeeded / rl.refillRate * float64(time.Second))

		// Apply adaptive backoff for consecutive waits
		if rl.consecutiveWaits > 0 {
			backoffMultiplier := 1 << uint(min(rl.consecutiveWaits, 5)) // Cap at 32x
			baseWait = time.Duration(float64(baseWait) * float64(backoffMultiplier))
		}
		rl.consecutiveWaits++

		return baseWait, false
	}

	// Consume token
	rl.tokens--
	rl.lastRequest = time.Now()
	rl.consecutiveWaits = 0 // Reset on successful acquisition

	return 0, true
}

// refillTokens adds tokens based on time elapsed since last refill.
// Must be called with lock held.
func (rl *RateLimiter) refillTokens() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()

	if elapsed > 0 {
		// Add tokens based on elapsed time
		rl.tokens += elapsed * rl.refillRate

		// Cap at maximum
		if rl.tokens > rl.maxTokens {
			rl.tokens = rl.maxTokens
		}

		rl.lastRefill = now
	}
}

// RecordRateLimitHit records that the API returned a rate limit response.
// This adjusts internal state to be more conservative.
func (rl *RateLimiter) RecordRateLimitHit(retryAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Empty the bucket when rate limited
	rl.tokens = 0

	// Use the retry-after from the API if provided, otherwise use default
	if retryAfter > 0 {
		rl.retryAfter = retryAfter
	}

	// Reduce the refill rate temporarily
	rl.refillRate *= 0.8

	// Update last request time to enforce wait
	rl.lastRequest = time.Now()

	// Increase consecutive waits for backoff
	rl.consecutiveWaits++
}

// Reset resets the rate limiter to initial state.
// Useful after a period of inactivity or configuration change.
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.tokens = rl.maxTokens
	rl.lastRefill = time.Now()
	rl.lastRequest = time.Now().Add(-rl.minInterval)
	rl.consecutiveWaits = 0
}

// SetRefillRate dynamically adjusts the refill rate.
func (rl *RateLimiter) SetRefillRate(rate float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refillRate = rate
}

// GetStatus returns the current status of the rate limiter.
type RateLimiterStatus struct {
	AvailableTokens  float64
	MaxTokens        float64
	RefillRate       float64
	LastRefill       time.Time
	LastRequest      time.Time
	ConsecutiveWaits int
}

// Status returns the current status of the rate limiter.
func (rl *RateLimiter) Status() RateLimiterStatus {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refillTokens()

	return RateLimiterStatus{
		AvailableTokens:  rl.tokens,
		MaxTokens:        rl.maxTokens,
		RefillRate:       rl.refillRate,
		LastRefill:       rl.lastRefill,
		LastRequest:      rl.lastRequest,
		ConsecutiveWaits: rl.consecutiveWaits,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// CIRCUIT BREAKER - Protection against failing external service
// ══════════════════════════════════════════════════════════════════════════════

// CircuitState represents the state of the circuit breaker.
type CircuitState int

const (
	// CircuitClosed - Normal operation, requests pass through.
	CircuitClosed CircuitState = iota

	// CircuitOpen - Circuit is open, requests fail fast.
	CircuitOpen

	// CircuitHalfOpen - Testing if service recovered.
	CircuitHalfOpen
)

// String returns the string representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the Circuit Breaker pattern.
type CircuitBreaker struct {
	mu sync.RWMutex

	// Configuration
	failureThreshold   int           // Number of failures before opening
	successThreshold   int           // Number of successes in half-open before closing
	timeout            time.Duration // How long to wait before moving to half-open
	halfOpenMaxRetries int           // Max requests allowed in half-open state

	// State
	state            CircuitState
	failures         int
	successes        int
	lastFailureTime  time.Time
	lastStateChange  time.Time
	halfOpenRequests int
}

// CircuitBreakerConfig contains configuration for the circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures before the circuit opens
	FailureThreshold int

	// SuccessThreshold is the number of successes needed to close the circuit
	SuccessThreshold int

	// Timeout is how long to wait before trying again
	Timeout time.Duration

	// HalfOpenMaxRetries is the number of test requests in half-open state
	HalfOpenMaxRetries int
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:   5,
		SuccessThreshold:   2,
		Timeout:            30 * time.Second,
		HalfOpenMaxRetries: 3,
	}
}

// NewCircuitBreaker creates a new CircuitBreaker.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold:   config.FailureThreshold,
		successThreshold:   config.SuccessThreshold,
		timeout:            config.Timeout,
		halfOpenMaxRetries: config.HalfOpenMaxRetries,
		state:              CircuitClosed,
		lastStateChange:    time.Now(),
	}
}

// ErrCircuitOpen is returned when the circuit is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Allow checks if a request should be allowed through.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return nil

	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(cb.lastStateChange) > cb.timeout {
			cb.toHalfOpen()
			return nil
		}
		return ErrCircuitOpen

	case CircuitHalfOpen:
		// Allow limited requests in half-open
		if cb.halfOpenRequests < cb.halfOpenMaxRetries {
			cb.halfOpenRequests++
			return nil
		}
		return ErrCircuitOpen
	}

	return nil
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.successThreshold {
			cb.toClosed()
		}
	case CircuitClosed:
		// Reset failure count on success
		cb.failures = 0
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		if cb.failures >= cb.failureThreshold {
			cb.toOpen()
		}
	case CircuitHalfOpen:
		cb.toOpen()
	}
}

// State returns the current state of the circuit.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.toClosed()
}

// Internal state transitions (must be called with lock held)

func (cb *CircuitBreaker) toClosed() {
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
}

func (cb *CircuitBreaker) toOpen() {
	cb.state = CircuitOpen
	cb.lastStateChange = time.Now()
}

func (cb *CircuitBreaker) toHalfOpen() {
	cb.state = CircuitHalfOpen
	cb.successes = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()
}

// CircuitBreakerStatus contains the current status.
type CircuitBreakerStatus struct {
	State           CircuitState
	Failures        int
	Successes       int
	LastFailureTime time.Time
	LastStateChange time.Time
}

// Status returns the current status.
func (cb *CircuitBreaker) Status() CircuitBreakerStatus {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStatus{
		State:           cb.state,
		Failures:        cb.failures,
		Successes:       cb.successes,
		LastFailureTime: cb.lastFailureTime,
		LastStateChange: cb.lastStateChange,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// RETRY HELPER
// ══════════════════════════════════════════════════════════════════════════════

// RetryConfig contains configuration for retry behavior.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int

	// InitialBackoff is the initial wait time between retries
	InitialBackoff time.Duration

	// MaxBackoff is the maximum wait time between retries
	MaxBackoff time.Duration

	// BackoffMultiplier is the factor by which backoff increases
	BackoffMultiplier float64

	// Jitter adds randomness to backoff (0.0 to 1.0)
	Jitter float64
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.1,
	}
}

// CalculateBackoff calculates the backoff duration for a given attempt.
func (c RetryConfig) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return c.InitialBackoff
	}

	backoff := float64(c.InitialBackoff)
	for i := 0; i < attempt; i++ {
		backoff *= c.BackoffMultiplier
	}

	if backoff > float64(c.MaxBackoff) {
		backoff = float64(c.MaxBackoff)
	}

	// Apply jitter
	if c.Jitter > 0 {
		jitterAmount := backoff * c.Jitter
		// Simple deterministic jitter based on attempt number
		adjustment := jitterAmount * float64((attempt*37)%100) / 100.0
		backoff = backoff - jitterAmount/2 + adjustment
	}

	return time.Duration(backoff)
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
