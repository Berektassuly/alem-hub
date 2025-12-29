// Package middleware contains Telegram bot middlewares for request processing.
package middleware

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// RECOVERY MIDDLEWARE
// Catches panics in handlers and converts them to user-friendly error messages.
// Philosophy: Never show scary stack traces to users, but make sure we log
// everything for debugging. The bot must stay responsive even if handlers crash.
// ══════════════════════════════════════════════════════════════════════════════

// RecoveryConfig holds configuration for the recovery middleware.
type RecoveryConfig struct {
	// EnableStackTrace enables capturing stack traces (can be memory intensive).
	EnableStackTrace bool

	// StackTraceDepth is the maximum depth of stack trace to capture.
	StackTraceDepth int

	// OnPanic is called when a panic is recovered.
	// This is where you would send alerts to monitoring systems.
	OnPanic func(ctx context.Context, panicInfo *PanicInfo)

	// UserErrorMessage is the message sent to users when a panic occurs.
	UserErrorMessage string

	// LogPanics enables logging panics to stdout (useful for debugging).
	LogPanics bool

	// MaxPanicsPerMinute limits how many panics to process per minute
	// to prevent cascading failures.
	MaxPanicsPerMinute int
}

// DefaultRecoveryConfig returns sensible defaults for recovery middleware.
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		EnableStackTrace: true,
		StackTraceDepth:  64,
		OnPanic:          nil, // Set your own handler
		UserErrorMessage: "😔 Что-то пошло не так.\n\n" +
			"Наша команда уже знает о проблеме и работает над её решением.\n" +
			"Попробуй ещё раз через несколько минут.",
		LogPanics:          true,
		MaxPanicsPerMinute: 100,
	}
}

// PanicInfo contains information about a recovered panic.
type PanicInfo struct {
	// Error is the panic value converted to error.
	Error error

	// PanicValue is the raw panic value.
	PanicValue interface{}

	// StackTrace is the formatted stack trace.
	StackTrace string

	// RequestID is the request ID from context (if available).
	RequestID string

	// TelegramID is the Telegram user ID (if available).
	TelegramID int64

	// Command is the command that was being processed (if available).
	Command string

	// Timestamp is when the panic occurred.
	Timestamp time.Time

	// Goroutine is the ID of the goroutine that panicked.
	Goroutine int
}

// String returns a formatted string representation of the panic info.
func (p *PanicInfo) String() string {
	var buf bytes.Buffer
	buf.WriteString("=== PANIC RECOVERED ===\n")
	buf.WriteString(fmt.Sprintf("Time:       %s\n", p.Timestamp.Format(time.RFC3339)))
	buf.WriteString(fmt.Sprintf("Goroutine:  %d\n", p.Goroutine))
	if p.RequestID != "" {
		buf.WriteString(fmt.Sprintf("RequestID:  %s\n", p.RequestID))
	}
	if p.TelegramID != 0 {
		buf.WriteString(fmt.Sprintf("TelegramID: %d\n", p.TelegramID))
	}
	if p.Command != "" {
		buf.WriteString(fmt.Sprintf("Command:    %s\n", p.Command))
	}
	buf.WriteString(fmt.Sprintf("Error:      %v\n", p.PanicValue))
	if p.StackTrace != "" {
		buf.WriteString("\nStack Trace:\n")
		buf.WriteString(p.StackTrace)
	}
	buf.WriteString("========================\n")
	return buf.String()
}

// RecoveryMiddleware recovers from panics and provides error handling.
type RecoveryMiddleware struct {
	config       RecoveryConfig
	panicCounter *panicRateLimiter
}

// NewRecoveryMiddleware creates a new recovery middleware.
func NewRecoveryMiddleware(config RecoveryConfig) *RecoveryMiddleware {
	return &RecoveryMiddleware{
		config:       config,
		panicCounter: newPanicRateLimiter(config.MaxPanicsPerMinute),
	}
}

// RecoveryResult represents the result of handling a panic.
type RecoveryResult struct {
	// Recovered indicates if a panic was recovered.
	Recovered bool

	// PanicInfo contains panic details (if recovered).
	PanicInfo *PanicInfo

	// UserMessage is the message to show to the user.
	UserMessage string

	// ShouldNotify indicates if external systems should be notified.
	ShouldNotify bool
}

// Wrap wraps a function with panic recovery.
// Returns a function that will catch any panics and return a RecoveryResult.
func (m *RecoveryMiddleware) Wrap(fn func() error) func() (*RecoveryResult, error) {
	return m.WrapWithContext(context.Background(), fn)
}

// WrapWithContext wraps a function with panic recovery and context.
func (m *RecoveryMiddleware) WrapWithContext(ctx context.Context, fn func() error) func() (*RecoveryResult, error) {
	return func() (result *RecoveryResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				result = m.handlePanic(ctx, r)
			}
		}()

		err = fn()
		return &RecoveryResult{Recovered: false}, err
	}
}

// RecoverWithHandler executes a handler and recovers from any panics.
// This is the main entry point for the middleware.
func (m *RecoveryMiddleware) RecoverWithHandler(
	ctx context.Context,
	telegramID int64,
	command string,
	handler func() error,
) *RecoveryResult {
	// Add metadata to context for better panic info
	ctx = context.WithValue(ctx, TelegramIDContextKey, telegramID)

	var result *RecoveryResult
	var handlerErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				result = m.handlePanicWithMeta(ctx, r, telegramID, command)
			}
		}()
		handlerErr = handler()
	}()

	// If panic occurred, return the result
	if result != nil {
		return result
	}

	// No panic - check if handler returned an error
	if handlerErr != nil {
		return &RecoveryResult{
			Recovered:   false,
			UserMessage: "", // Let the caller handle the error
		}
	}

	return &RecoveryResult{
		Recovered: false,
	}
}

// handlePanic processes a recovered panic.
func (m *RecoveryMiddleware) handlePanic(ctx context.Context, panicValue interface{}) *RecoveryResult {
	return m.handlePanicWithMeta(ctx, panicValue, 0, "")
}

// handlePanicWithMeta processes a recovered panic with additional metadata.
func (m *RecoveryMiddleware) handlePanicWithMeta(
	ctx context.Context,
	panicValue interface{},
	telegramID int64,
	command string,
) *RecoveryResult {
	// Rate limit panic processing
	if !m.panicCounter.allow() {
		return &RecoveryResult{
			Recovered:    true,
			UserMessage:  m.config.UserErrorMessage,
			ShouldNotify: false, // Too many panics, skip notification
		}
	}

	// Build panic info
	panicInfo := &PanicInfo{
		Error:      toError(panicValue),
		PanicValue: panicValue,
		Timestamp:  time.Now(),
		Goroutine:  getGoroutineID(),
		TelegramID: telegramID,
		Command:    command,
	}

	// Get request ID from context
	if requestID, ok := ctx.Value(RequestIDContextKey).(string); ok {
		panicInfo.RequestID = requestID
	}

	// Capture stack trace if enabled
	if m.config.EnableStackTrace {
		panicInfo.StackTrace = string(debug.Stack())
	}

	// Log if enabled
	if m.config.LogPanics {
		fmt.Println(panicInfo.String())
	}

	// Call custom panic handler
	if m.config.OnPanic != nil {
		m.config.OnPanic(ctx, panicInfo)
	}

	return &RecoveryResult{
		Recovered:    true,
		PanicInfo:    panicInfo,
		UserMessage:  m.config.UserErrorMessage,
		ShouldNotify: true,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// toError converts a panic value to an error.
func toError(panicValue interface{}) error {
	switch v := panicValue.(type) {
	case error:
		return v
	case string:
		return fmt.Errorf("%s", v)
	default:
		return fmt.Errorf("panic: %v", v)
	}
}

// getGoroutineID returns the current goroutine ID (for debugging only).
// Note: This is not officially supported by Go and should only be used for debugging.
func getGoroutineID() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	var id int
	fmt.Sscanf(string(buf[:n]), "goroutine %d ", &id)
	return id
}

// ══════════════════════════════════════════════════════════════════════════════
// PANIC RATE LIMITER
// Prevents cascading failures by limiting how many panics we process.
// ══════════════════════════════════════════════════════════════════════════════

type panicRateLimiter struct {
	mu        sync.Mutex
	count     int
	maxPerMin int
	window    time.Time
}

func newPanicRateLimiter(maxPerMin int) *panicRateLimiter {
	return &panicRateLimiter{
		maxPerMin: maxPerMin,
		window:    time.Now(),
	}
}

func (p *panicRateLimiter) allow() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	// Reset counter if minute has passed
	if now.Sub(p.window) > time.Minute {
		p.count = 0
		p.window = now
	}

	// Check limit
	if p.count >= p.maxPerMin {
		return false
	}

	p.count++
	return true
}

// ══════════════════════════════════════════════════════════════════════════════
// PANIC AGGREGATOR
// Collects and groups similar panics for better alerting.
// ══════════════════════════════════════════════════════════════════════════════

// PanicAggregator groups and tracks similar panics.
type PanicAggregator struct {
	mu       sync.RWMutex
	panics   map[string]*AggregatedPanic
	maxAge   time.Duration
	maxItems int
}

// AggregatedPanic represents a group of similar panics.
type AggregatedPanic struct {
	// Key is the unique identifier for this panic type.
	Key string

	// Count is the number of times this panic occurred.
	Count int

	// FirstSeen is when this panic was first observed.
	FirstSeen time.Time

	// LastSeen is when this panic was last observed.
	LastSeen time.Time

	// SampleError is a sample of the error message.
	SampleError string

	// SampleStack is a sample of the stack trace.
	SampleStack string

	// AffectedUsers is a set of affected user IDs.
	AffectedUsers map[int64]bool

	// AffectedCommands is a set of affected commands.
	AffectedCommands map[string]bool
}

// NewPanicAggregator creates a new panic aggregator.
func NewPanicAggregator(maxAge time.Duration, maxItems int) *PanicAggregator {
	pa := &PanicAggregator{
		panics:   make(map[string]*AggregatedPanic),
		maxAge:   maxAge,
		maxItems: maxItems,
	}

	// Start cleanup goroutine
	go pa.cleanupLoop()

	return pa
}

// Add adds a panic to the aggregator.
func (pa *PanicAggregator) Add(info *PanicInfo) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	// Generate key from error message (simplified grouping)
	key := generatePanicKey(info.Error.Error())

	agg, ok := pa.panics[key]
	if !ok {
		agg = &AggregatedPanic{
			Key:              key,
			FirstSeen:        info.Timestamp,
			SampleError:      info.Error.Error(),
			SampleStack:      info.StackTrace,
			AffectedUsers:    make(map[int64]bool),
			AffectedCommands: make(map[string]bool),
		}
		pa.panics[key] = agg
	}

	agg.Count++
	agg.LastSeen = info.Timestamp

	if info.TelegramID != 0 {
		agg.AffectedUsers[info.TelegramID] = true
	}
	if info.Command != "" {
		agg.AffectedCommands[info.Command] = true
	}

	// Enforce max items
	if len(pa.panics) > pa.maxItems {
		pa.evictOldest()
	}
}

// GetStats returns current panic statistics.
func (pa *PanicAggregator) GetStats() []*AggregatedPanic {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	result := make([]*AggregatedPanic, 0, len(pa.panics))
	for _, agg := range pa.panics {
		// Create a copy
		copy := &AggregatedPanic{
			Key:              agg.Key,
			Count:            agg.Count,
			FirstSeen:        agg.FirstSeen,
			LastSeen:         agg.LastSeen,
			SampleError:      agg.SampleError,
			SampleStack:      agg.SampleStack,
			AffectedUsers:    make(map[int64]bool),
			AffectedCommands: make(map[string]bool),
		}
		for k, v := range agg.AffectedUsers {
			copy.AffectedUsers[k] = v
		}
		for k, v := range agg.AffectedCommands {
			copy.AffectedCommands[k] = v
		}
		result = append(result, copy)
	}
	return result
}

// Clear clears all aggregated panics.
func (pa *PanicAggregator) Clear() {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	pa.panics = make(map[string]*AggregatedPanic)
}

func (pa *PanicAggregator) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		pa.cleanup()
	}
}

func (pa *PanicAggregator) cleanup() {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	threshold := time.Now().Add(-pa.maxAge)
	for key, agg := range pa.panics {
		if agg.LastSeen.Before(threshold) {
			delete(pa.panics, key)
		}
	}
}

func (pa *PanicAggregator) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, agg := range pa.panics {
		if oldestKey == "" || agg.LastSeen.Before(oldestTime) {
			oldestKey = key
			oldestTime = agg.LastSeen
		}
	}

	if oldestKey != "" {
		delete(pa.panics, oldestKey)
	}
}

// generatePanicKey generates a grouping key from an error message.
func generatePanicKey(errMsg string) string {
	// Simple key generation - in production, you might want something smarter
	// that groups similar errors even if they have different dynamic values
	if len(errMsg) > 100 {
		return errMsg[:100]
	}
	return errMsg
}
