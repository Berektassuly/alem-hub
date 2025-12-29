// Package circuitbreaker implements the Circuit Breaker pattern for fault tolerance.
// It protects the system from cascading failures when external services (Alem API) fail.
// No external dependencies - uses only standard library.
package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State represents the current state of the circuit breaker.
type State int

const (
	// StateClosed is the normal state - requests are allowed through.
	StateClosed State = iota
	// StateOpen is the failure state - requests are blocked.
	StateOpen
	// StateHalfOpen is the recovery state - limited requests allowed to test recovery.
	StateHalfOpen
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Common errors.
var (
	// ErrCircuitOpen is returned when the circuit is open and requests are blocked.
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests is returned when too many requests are made in half-open state.
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// Config holds circuit breaker configuration.
type Config struct {
	// Name identifies this circuit breaker (for logging/metrics).
	Name string

	// FailureThreshold is the number of failures before opening the circuit.
	// Default: 5
	FailureThreshold int

	// SuccessThreshold is the number of successes in half-open state
	// before closing the circuit.
	// Default: 2
	SuccessThreshold int

	// Timeout is how long to wait in open state before transitioning to half-open.
	// Default: 30s
	Timeout time.Duration

	// MaxHalfOpenRequests is the maximum number of requests allowed in half-open state.
	// Default: 1
	MaxHalfOpenRequests int

	// OnStateChange is called when the circuit state changes.
	OnStateChange func(name string, from, to State)

	// IsFailure determines if an error should be counted as a failure.
	// If nil, all non-nil errors are counted as failures.
	IsFailure func(error) bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(name string) Config {
	return Config{
		Name:                name,
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		MaxHalfOpenRequests: 1,
		OnStateChange:       nil,
		IsFailure:           nil,
	}
}

// Option is a functional option for configuring the circuit breaker.
type Option func(*Config)

// WithFailureThreshold sets the failure threshold.
func WithFailureThreshold(n int) Option {
	return func(c *Config) {
		if n > 0 {
			c.FailureThreshold = n
		}
	}
}

// WithSuccessThreshold sets the success threshold.
func WithSuccessThreshold(n int) Option {
	return func(c *Config) {
		if n > 0 {
			c.SuccessThreshold = n
		}
	}
}

// WithTimeout sets the timeout duration.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) {
		if d > 0 {
			c.Timeout = d
		}
	}
}

// WithMaxHalfOpenRequests sets the max requests allowed in half-open state.
func WithMaxHalfOpenRequests(n int) Option {
	return func(c *Config) {
		if n > 0 {
			c.MaxHalfOpenRequests = n
		}
	}
}

// WithOnStateChange sets the state change callback.
func WithOnStateChange(fn func(name string, from, to State)) Option {
	return func(c *Config) {
		c.OnStateChange = fn
	}
}

// WithIsFailure sets the failure detection function.
func WithIsFailure(fn func(error) bool) Option {
	return func(c *Config) {
		c.IsFailure = fn
	}
}

// Counts holds the current counts for the circuit breaker.
type Counts struct {
	Requests             int
	TotalSuccesses       int
	TotalFailures        int
	ConsecutiveSuccesses int
	ConsecutiveFailures  int
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	config Config

	mu               sync.Mutex
	state            State
	counts           Counts
	lastFailureTime  time.Time
	halfOpenRequests int
}

// New creates a new CircuitBreaker with the given name and options.
func New(name string, opts ...Option) *CircuitBreaker {
	config := DefaultConfig(name)
	for _, opt := range opts {
		opt(&config)
	}

	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Execute runs the given function if the circuit allows it.
// It handles state transitions based on the result.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	// Check if we can proceed
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	// Execute the function
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	// Record the result
	cb.afterRequest(err, duration)

	return err
}

// ExecuteWithFallback runs the function with a fallback if the circuit is open.
func (cb *CircuitBreaker) ExecuteWithFallback(ctx context.Context, fn func(context.Context) error, fallback func(error) error) error {
	err := cb.Execute(ctx, fn)
	if errors.Is(err, ErrCircuitOpen) || errors.Is(err, ErrTooManyRequests) {
		return fallback(err)
	}
	return err
}

// beforeRequest checks if a request should be allowed.
func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		// Allow the request
		return nil

	case StateOpen:
		// Check if timeout has passed
		if now.Sub(cb.lastFailureTime) >= cb.config.Timeout {
			// Transition to half-open
			cb.setState(StateHalfOpen)
			cb.halfOpenRequests = 1
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		// Allow limited requests
		if cb.halfOpenRequests < cb.config.MaxHalfOpenRequests {
			cb.halfOpenRequests++
			return nil
		}
		return ErrTooManyRequests

	default:
		return ErrCircuitOpen
	}
}

// afterRequest records the result of a request.
func (cb *CircuitBreaker) afterRequest(err error, duration time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.counts.Requests++

	// Determine if this is a failure
	isFailure := err != nil
	if cb.config.IsFailure != nil && err != nil {
		isFailure = cb.config.IsFailure(err)
	}

	if isFailure {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

// onSuccess handles a successful request.
func (cb *CircuitBreaker) onSuccess() {
	cb.counts.TotalSuccesses++
	cb.counts.ConsecutiveSuccesses++
	cb.counts.ConsecutiveFailures = 0

	switch cb.state {
	case StateHalfOpen:
		if cb.counts.ConsecutiveSuccesses >= cb.config.SuccessThreshold {
			cb.setState(StateClosed)
		}
	}
}

// onFailure handles a failed request.
func (cb *CircuitBreaker) onFailure() {
	cb.counts.TotalFailures++
	cb.counts.ConsecutiveFailures++
	cb.counts.ConsecutiveSuccesses = 0
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.counts.ConsecutiveFailures >= cb.config.FailureThreshold {
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		// Any failure in half-open state opens the circuit
		cb.setState(StateOpen)
	}
}

// setState transitions to a new state.
func (cb *CircuitBreaker) setState(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState

	// Reset counts on state change
	cb.counts.ConsecutiveSuccesses = 0
	cb.counts.ConsecutiveFailures = 0
	cb.halfOpenRequests = 0

	// Call the callback
	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.config.Name, oldState, newState)
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Counts returns the current counts.
func (cb *CircuitBreaker) Counts() Counts {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.counts
}

// Reset resets the circuit breaker to its initial state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.counts = Counts{}
	cb.halfOpenRequests = 0
}

// Name returns the name of the circuit breaker.
func (cb *CircuitBreaker) Name() string {
	return cb.config.Name
}

// IsOpen returns true if the circuit is open.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state == StateOpen
}

// IsClosed returns true if the circuit is closed.
func (cb *CircuitBreaker) IsClosed() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state == StateClosed
}

// Common circuit breaker configurations for Alem Hub.

// AlemAPIBreaker returns a circuit breaker configured for Alem API.
// Uses conservative settings because Alem API is critical but potentially unstable.
func AlemAPIBreaker(onStateChange func(name string, from, to State)) *CircuitBreaker {
	return New(
		"alem-api",
		WithFailureThreshold(3),     // Open after 3 consecutive failures
		WithSuccessThreshold(2),     // Need 2 successes to close
		WithTimeout(60*time.Second), // Wait 1 minute before trying again
		WithMaxHalfOpenRequests(1),  // Only allow 1 test request
		WithOnStateChange(onStateChange),
	)
}

// TelegramAPIBreaker returns a circuit breaker configured for Telegram API.
func TelegramAPIBreaker(onStateChange func(name string, from, to State)) *CircuitBreaker {
	return New(
		"telegram-api",
		WithFailureThreshold(5),     // More tolerant - Telegram is usually stable
		WithSuccessThreshold(1),     // Recover quickly
		WithTimeout(30*time.Second), // Shorter timeout
		WithMaxHalfOpenRequests(2),  // Allow more test requests
		WithOnStateChange(onStateChange),
	)
}

// DatabaseBreaker returns a circuit breaker configured for database operations.
func DatabaseBreaker(onStateChange func(name string, from, to State)) *CircuitBreaker {
	return New(
		"database",
		WithFailureThreshold(3),
		WithSuccessThreshold(1),
		WithTimeout(10*time.Second), // Short timeout for DB
		WithMaxHalfOpenRequests(1),
		WithOnStateChange(onStateChange),
	)
}
