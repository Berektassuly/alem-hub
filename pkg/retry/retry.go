// Package retry provides retry functionality with exponential backoff and jitter.
// Designed for resilient external service calls (Alem API, Telegram API).
// No external dependencies - uses only standard library.
package retry

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// init seeds the random number generator for jitter.
func init() {
	rand.Seed(time.Now().UnixNano())
}

// RetryableError indicates that an error is retryable.
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// Retryable wraps an error to indicate it should be retried.
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &RetryableError{Err: err}
}

// IsRetryable checks if an error is retryable.
func IsRetryable(err error) bool {
	var retryableErr *RetryableError
	return errors.As(err, &retryableErr)
}

// PermanentError indicates that an error should not be retried.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string {
	return e.Err.Error()
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

// Permanent wraps an error to indicate it should not be retried.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{Err: err}
}

// IsPermanent checks if an error is permanent (should not be retried).
func IsPermanent(err error) bool {
	var permanentErr *PermanentError
	return errors.As(err, &permanentErr)
}

// Config holds retry configuration.
type Config struct {
	// MaxAttempts is the maximum number of attempts (including first attempt).
	// Default: 3
	MaxAttempts int

	// InitialDelay is the initial delay before first retry.
	// Default: 100ms
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	// Default: 30s
	MaxDelay time.Duration

	// Multiplier is the factor by which delay increases after each attempt.
	// Default: 2.0
	Multiplier float64

	// JitterFactor adds randomness to delays (0.0 = no jitter, 1.0 = full jitter).
	// Default: 0.1 (10% jitter)
	JitterFactor float64

	// RetryIf is a function that determines if an error should be retried.
	// If nil, only RetryableError errors are retried.
	RetryIf func(error) bool

	// OnRetry is called before each retry attempt.
	// Useful for logging or metrics.
	OnRetry func(attempt int, err error, delay time.Duration)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.1,
		RetryIf:      nil,
		OnRetry:      nil,
	}
}

// Option is a functional option for configuring retries.
type Option func(*Config)

// WithMaxAttempts sets the maximum number of attempts.
func WithMaxAttempts(n int) Option {
	return func(c *Config) {
		if n > 0 {
			c.MaxAttempts = n
		}
	}
}

// WithInitialDelay sets the initial delay before first retry.
func WithInitialDelay(d time.Duration) Option {
	return func(c *Config) {
		if d > 0 {
			c.InitialDelay = d
		}
	}
}

// WithMaxDelay sets the maximum delay between retries.
func WithMaxDelay(d time.Duration) Option {
	return func(c *Config) {
		if d > 0 {
			c.MaxDelay = d
		}
	}
}

// WithMultiplier sets the backoff multiplier.
func WithMultiplier(m float64) Option {
	return func(c *Config) {
		if m >= 1.0 {
			c.Multiplier = m
		}
	}
}

// WithJitter sets the jitter factor (0.0 to 1.0).
func WithJitter(j float64) Option {
	return func(c *Config) {
		if j >= 0 && j <= 1.0 {
			c.JitterFactor = j
		}
	}
}

// WithRetryIf sets a custom function to determine if an error should be retried.
func WithRetryIf(fn func(error) bool) Option {
	return func(c *Config) {
		c.RetryIf = fn
	}
}

// WithOnRetry sets a callback function called before each retry.
func WithOnRetry(fn func(attempt int, err error, delay time.Duration)) Option {
	return func(c *Config) {
		c.OnRetry = fn
	}
}

// Retrier manages retry operations.
type Retrier struct {
	config Config
}

// New creates a new Retrier with the given options.
func New(opts ...Option) *Retrier {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(&config)
	}
	return &Retrier{config: config}
}

// Do executes the operation with retries.
// The operation should return a RetryableError if it should be retried,
// or a PermanentError if it should not be retried.
func (r *Retrier) Do(ctx context.Context, operation func(ctx context.Context) error) error {
	var lastErr error

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		// Check context before each attempt
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}

		// Execute operation
		err := operation(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is permanent (should not be retried)
		if IsPermanent(err) {
			return errors.Unwrap(err)
		}

		// Check if we should retry this error
		shouldRetry := false
		if r.config.RetryIf != nil {
			shouldRetry = r.config.RetryIf(err)
		} else {
			// Default: only retry RetryableError
			shouldRetry = IsRetryable(err)
		}

		if !shouldRetry {
			return err
		}

		// Last attempt - don't sleep, just return the error
		if attempt == r.config.MaxAttempts {
			// Unwrap RetryableError if present
			if IsRetryable(err) {
				return errors.Unwrap(err)
			}
			return err
		}

		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)

		// Call OnRetry callback if set
		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt, err, delay)
		}

		// Wait before next attempt
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(delay):
		}
	}

	return lastErr
}

// calculateDelay calculates the delay for a given attempt with jitter.
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	// Base delay with exponential backoff: initialDelay * multiplier^(attempt-1)
	baseDelay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt-1))

	// Cap at max delay
	if baseDelay > float64(r.config.MaxDelay) {
		baseDelay = float64(r.config.MaxDelay)
	}

	// Add jitter
	if r.config.JitterFactor > 0 {
		jitter := baseDelay * r.config.JitterFactor * (rand.Float64()*2 - 1) // -jitter to +jitter
		baseDelay += jitter
	}

	// Ensure non-negative
	if baseDelay < 0 {
		baseDelay = 0
	}

	return time.Duration(baseDelay)
}

// Do is a convenience function that creates a Retrier and executes the operation.
func Do(ctx context.Context, operation func(ctx context.Context) error, opts ...Option) error {
	return New(opts...).Do(ctx, operation)
}

// DoWithData is a helper for operations that return data.
func DoWithData[T any](ctx context.Context, operation func(ctx context.Context) (T, error), opts ...Option) (T, error) {
	var result T
	err := New(opts...).Do(ctx, func(ctx context.Context) error {
		var opErr error
		result, opErr = operation(ctx)
		return opErr
	})
	return result, err
}

// Common retry configurations for Alem Hub use cases.

// AlemAPIRetrier returns a Retrier configured for Alem API calls.
// Uses conservative settings to avoid being rate-limited.
func AlemAPIRetrier() *Retrier {
	return New(
		WithMaxAttempts(3),
		WithInitialDelay(500*time.Millisecond),
		WithMaxDelay(10*time.Second),
		WithMultiplier(2.0),
		WithJitter(0.2),
	)
}

// TelegramRetrier returns a Retrier configured for Telegram API calls.
func TelegramRetrier() *Retrier {
	return New(
		WithMaxAttempts(5),
		WithInitialDelay(100*time.Millisecond),
		WithMaxDelay(5*time.Second),
		WithMultiplier(1.5),
		WithJitter(0.1),
	)
}

// DatabaseRetrier returns a Retrier configured for database operations.
func DatabaseRetrier() *Retrier {
	return New(
		WithMaxAttempts(3),
		WithInitialDelay(50*time.Millisecond),
		WithMaxDelay(1*time.Second),
		WithMultiplier(2.0),
		WithJitter(0.05),
	)
}
