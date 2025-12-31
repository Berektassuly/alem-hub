// Package messaging implements event bus functionality.
package messaging

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// DISPATCHER
// ══════════════════════════════════════════════════════════════════════════════

// Dispatcher routes events to appropriate handlers with support for:
// - Middleware (logging, metrics, error handling)
// - Retry logic with exponential backoff
// - Concurrent processing
// - Dead letter queue for failed events
type Dispatcher struct {
	eventBus    shared.EventBus
	handlers    map[shared.EventType][]HandlerRegistration
	middlewares []Middleware
	retryConfig RetryConfig
	deadLetterQ *DeadLetterQueue
	logger      *slog.Logger
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	workerPool  chan struct{}
	metrics     *DispatcherMetrics
}

// HandlerRegistration contains handler metadata.
type HandlerRegistration struct {
	Name       string
	Handler    shared.EventHandler
	Priority   int
	Async      bool
	MaxRetries int
	Timeout    time.Duration
}

// DispatcherConfig contains configuration for the Dispatcher.
type DispatcherConfig struct {
	// EventBus is the underlying event bus
	EventBus shared.EventBus

	// WorkerPoolSize is the number of concurrent workers
	WorkerPoolSize int

	// RetryConfig configures retry behavior
	RetryConfig RetryConfig

	// EnableDeadLetterQueue enables DLQ for failed events
	EnableDeadLetterQueue bool

	// DeadLetterQueueSize is the max size of the DLQ
	DeadLetterQueueSize int

	// Logger for structured logging
	Logger *slog.Logger
}

// RetryConfig contains retry configuration.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int

	// InitialBackoff is the initial wait between retries
	InitialBackoff time.Duration

	// MaxBackoff is the maximum wait between retries
	MaxBackoff time.Duration

	// BackoffMultiplier is the factor for exponential backoff
	BackoffMultiplier float64
}

// DefaultRetryConfig returns sensible retry defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        5 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// DefaultDispatcherConfig returns sensible defaults.
func DefaultDispatcherConfig(eventBus shared.EventBus) DispatcherConfig {
	return DispatcherConfig{
		EventBus:              eventBus,
		WorkerPoolSize:        10,
		RetryConfig:           DefaultRetryConfig(),
		EnableDeadLetterQueue: true,
		DeadLetterQueueSize:   1000,
	}
}

// NewDispatcher creates a new event dispatcher.
func NewDispatcher(config DispatcherConfig) *Dispatcher {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.WorkerPoolSize <= 0 {
		config.WorkerPoolSize = 10
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &Dispatcher{
		eventBus:    config.EventBus,
		handlers:    make(map[shared.EventType][]HandlerRegistration),
		middlewares: make([]Middleware, 0),
		retryConfig: config.RetryConfig,
		logger:      config.Logger,
		ctx:         ctx,
		cancel:      cancel,
		workerPool:  make(chan struct{}, config.WorkerPoolSize),
		metrics:     NewDispatcherMetrics(),
	}

	if config.EnableDeadLetterQueue {
		d.deadLetterQ = NewDeadLetterQueue(config.DeadLetterQueueSize)
	}

	return d
}

// ══════════════════════════════════════════════════════════════════════════════
// HANDLER REGISTRATION
// ══════════════════════════════════════════════════════════════════════════════

// RegisterHandler registers a handler for an event type.
func (d *Dispatcher) RegisterHandler(eventType shared.EventType, reg HandlerRegistration) error {
	if reg.Handler == nil {
		return errors.New("handler cannot be nil")
	}
	if reg.Name == "" {
		reg.Name = fmt.Sprintf("handler-%d", time.Now().UnixNano())
	}
	if reg.MaxRetries <= 0 {
		reg.MaxRetries = d.retryConfig.MaxRetries
	}
	if reg.Timeout <= 0 {
		reg.Timeout = 30 * time.Second
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.handlers[eventType] = append(d.handlers[eventType], reg)
	d.logger.Debug("registered handler",
		"event_type", eventType,
		"handler_name", reg.Name,
		"async", reg.Async,
	)

	return nil
}

// Register is a convenience method for simple handler registration.
func (d *Dispatcher) Register(eventType shared.EventType, name string, handler shared.EventHandler) error {
	return d.RegisterHandler(eventType, HandlerRegistration{
		Name:    name,
		Handler: handler,
		Async:   true,
	})
}

// RegisterSync registers a synchronous handler.
func (d *Dispatcher) RegisterSync(eventType shared.EventType, name string, handler shared.EventHandler) error {
	return d.RegisterHandler(eventType, HandlerRegistration{
		Name:    name,
		Handler: handler,
		Async:   false,
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// MIDDLEWARE
// ══════════════════════════════════════════════════════════════════════════════

// Middleware wraps handler execution.
type Middleware func(shared.EventHandler) shared.EventHandler

// Use adds middleware to the dispatcher.
func (d *Dispatcher) Use(middleware Middleware) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.middlewares = append(d.middlewares, middleware)
}

// RecoveryMiddleware recovers from panics in handlers.
func RecoveryMiddleware(logger *slog.Logger) Middleware {
	return func(next shared.EventHandler) shared.EventHandler {
		return func(event shared.Event) (err error) {
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					logger.Error("handler panic recovered",
						"event_type", event.EventType(),
						"panic", r,
						"stack", stack,
					)
					err = fmt.Errorf("handler panic: %v", r)
				}
			}()
			return next(event)
		}
	}
}

// LoggingMiddleware logs handler execution.
func LoggingMiddleware(logger *slog.Logger) Middleware {
	return func(next shared.EventHandler) shared.EventHandler {
		return func(event shared.Event) error {
			start := time.Now()
			err := next(event)
			duration := time.Since(start)

			if err != nil {
				logger.Error("handler failed",
					"event_type", event.EventType(),
					"aggregate_id", event.AggregateID(),
					"duration", duration,
					"error", err,
				)
			} else {
				logger.Debug("handler completed",
					"event_type", event.EventType(),
					"aggregate_id", event.AggregateID(),
					"duration", duration,
				)
			}

			return err
		}
	}
}

// MetricsMiddleware collects handler metrics.
func MetricsMiddleware(metrics *DispatcherMetrics) Middleware {
	return func(next shared.EventHandler) shared.EventHandler {
		return func(event shared.Event) error {
			start := time.Now()
			err := next(event)
			duration := time.Since(start)

			metrics.RecordExecution(event.EventType(), duration, err == nil)

			return err
		}
	}
}

// TimeoutMiddleware adds timeout to handler execution.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next shared.EventHandler) shared.EventHandler {
		return func(event shared.Event) error {
			done := make(chan error, 1)

			go func() {
				done <- next(event)
			}()

			select {
			case err := <-done:
				return err
			case <-time.After(timeout):
				return fmt.Errorf("handler timeout after %v", timeout)
			}
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// EVENT DISPATCHING
// ══════════════════════════════════════════════════════════════════════════════

// Start begins listening for events and dispatching them.
func (d *Dispatcher) Start() error {
	// Subscribe to all events
	return d.eventBus.SubscribeAll(func(event shared.Event) error {
		return d.dispatch(event)
	})
}

// Dispatch manually dispatches an event to registered handlers.
func (d *Dispatcher) Dispatch(event shared.Event) error {
	return d.dispatch(event)
}

func (d *Dispatcher) dispatch(event shared.Event) error {
	d.mu.RLock()
	handlers := d.handlers[event.EventType()]
	middlewares := d.middlewares
	d.mu.RUnlock()

	if len(handlers) == 0 {
		return nil
	}

	d.metrics.RecordDispatch(event.EventType())

	var wg sync.WaitGroup
	var syncErrors []error
	var syncMu sync.Mutex

	for _, reg := range handlers {
		if reg.Async {
			wg.Add(1)
			go func(r HandlerRegistration) {
				defer wg.Done()
				d.executeHandler(event, r, middlewares)
			}(reg)
		} else {
			err := d.executeHandler(event, reg, middlewares)
			if err != nil {
				syncMu.Lock()
				syncErrors = append(syncErrors, err)
				syncMu.Unlock()
			}
		}
	}

	wg.Wait()

	if len(syncErrors) > 0 {
		return fmt.Errorf("sync handler errors: %v", syncErrors)
	}

	return nil
}

func (d *Dispatcher) executeHandler(event shared.Event, reg HandlerRegistration, middlewares []Middleware) error {
	// Acquire worker slot
	select {
	case d.workerPool <- struct{}{}:
		defer func() { <-d.workerPool }()
	case <-d.ctx.Done():
		return d.ctx.Err()
	}

	// Build handler chain with middleware
	handler := reg.Handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	// Execute with retry
	var lastErr error
	for attempt := 0; attempt <= reg.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := d.calculateBackoff(attempt)
			d.logger.Debug("retrying handler",
				"handler", reg.Name,
				"attempt", attempt,
				"backoff", backoff,
			)

			select {
			case <-d.ctx.Done():
				return d.ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Execute with timeout
		err := d.executeWithTimeout(handler, event, reg.Timeout)
		if err == nil {
			if attempt > 0 {
				d.metrics.RecordRetrySuccess(event.EventType())
			}
			return nil
		}

		lastErr = err
		d.logger.Warn("handler attempt failed",
			"handler", reg.Name,
			"attempt", attempt,
			"error", err,
		)
	}

	// All retries exhausted - send to dead letter queue
	if d.deadLetterQ != nil {
		d.deadLetterQ.Add(DeadLetterEntry{
			Event:       event,
			HandlerName: reg.Name,
			Error:       lastErr,
			Attempts:    reg.MaxRetries + 1,
			FailedAt:    time.Now(),
		})
	}

	d.metrics.RecordFailure(event.EventType())
	return fmt.Errorf("handler %s failed after %d retries: %w", reg.Name, reg.MaxRetries+1, lastErr)
}

func (d *Dispatcher) executeWithTimeout(handler shared.EventHandler, event shared.Event, timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		done <- handler(event)
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("handler timeout after %v", timeout)
	case <-d.ctx.Done():
		return d.ctx.Err()
	}
}

func (d *Dispatcher) calculateBackoff(attempt int) time.Duration {
	backoff := float64(d.retryConfig.InitialBackoff)
	for i := 1; i < attempt; i++ {
		backoff *= d.retryConfig.BackoffMultiplier
	}

	if backoff > float64(d.retryConfig.MaxBackoff) {
		backoff = float64(d.retryConfig.MaxBackoff)
	}

	return time.Duration(backoff)
}

// ══════════════════════════════════════════════════════════════════════════════
// LIFECYCLE
// ══════════════════════════════════════════════════════════════════════════════

// Stop gracefully stops the dispatcher.
func (d *Dispatcher) Stop() error {
	d.cancel()
	d.wg.Wait()
	d.logger.Info("dispatcher stopped")
	return nil
}

// Metrics returns dispatcher metrics.
func (d *Dispatcher) Metrics() *DispatcherMetrics {
	return d.metrics
}

// DeadLetterQueue returns the dead letter queue.
func (d *Dispatcher) DeadLetterQueue() *DeadLetterQueue {
	return d.deadLetterQ
}

// ══════════════════════════════════════════════════════════════════════════════
// DEAD LETTER QUEUE
// ══════════════════════════════════════════════════════════════════════════════

// DeadLetterEntry represents a failed event.
type DeadLetterEntry struct {
	Event       shared.Event
	HandlerName string
	Error       error
	Attempts    int
	FailedAt    time.Time
}

// DeadLetterQueue stores events that failed processing.
type DeadLetterQueue struct {
	mu      sync.RWMutex
	entries []DeadLetterEntry
	maxSize int
}

// NewDeadLetterQueue creates a new dead letter queue.
func NewDeadLetterQueue(maxSize int) *DeadLetterQueue {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &DeadLetterQueue{
		entries: make([]DeadLetterEntry, 0),
		maxSize: maxSize,
	}
}

// Add adds an entry to the queue.
func (q *DeadLetterQueue) Add(entry DeadLetterEntry) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Remove oldest if at capacity
	if len(q.entries) >= q.maxSize {
		q.entries = q.entries[1:]
	}

	q.entries = append(q.entries, entry)
}

// Entries returns all entries.
func (q *DeadLetterQueue) Entries() []DeadLetterEntry {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]DeadLetterEntry, len(q.entries))
	copy(result, q.entries)
	return result
}

// Size returns the current queue size.
func (q *DeadLetterQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.entries)
}

// Clear removes all entries.
func (q *DeadLetterQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries = make([]DeadLetterEntry, 0)
}

// Pop removes and returns the oldest entry.
func (q *DeadLetterQueue) Pop() (DeadLetterEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.entries) == 0 {
		return DeadLetterEntry{}, false
	}

	entry := q.entries[0]
	q.entries = q.entries[1:]
	return entry, true
}

// ══════════════════════════════════════════════════════════════════════════════
// DISPATCHER METRICS
// ══════════════════════════════════════════════════════════════════════════════

// DispatcherMetrics tracks dispatcher performance.
type DispatcherMetrics struct {
	mu sync.RWMutex

	// Dispatch counts
	DispatchedTotal    map[shared.EventType]int64
	DispatchedLastHour map[shared.EventType]int64

	// Execution metrics
	ExecutionsTotal int64
	SuccessTotal    int64
	FailuresTotal   int64
	RetriesTotal    int64
	RetrySuccesses  int64

	// Duration tracking
	TotalDuration    time.Duration
	DurationByType   map[shared.EventType]time.Duration
	ExecutionsByType map[shared.EventType]int64

	// Last reset
	LastReset time.Time
}

// NewDispatcherMetrics creates new dispatcher metrics.
func NewDispatcherMetrics() *DispatcherMetrics {
	return &DispatcherMetrics{
		DispatchedTotal:    make(map[shared.EventType]int64),
		DispatchedLastHour: make(map[shared.EventType]int64),
		DurationByType:     make(map[shared.EventType]time.Duration),
		ExecutionsByType:   make(map[shared.EventType]int64),
		LastReset:          time.Now(),
	}
}

// RecordDispatch records an event dispatch.
func (m *DispatcherMetrics) RecordDispatch(eventType shared.EventType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DispatchedTotal[eventType]++
	m.DispatchedLastHour[eventType]++
}

// RecordExecution records a handler execution.
func (m *DispatcherMetrics) RecordExecution(eventType shared.EventType, duration time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ExecutionsTotal++
	m.TotalDuration += duration
	m.DurationByType[eventType] += duration
	m.ExecutionsByType[eventType]++

	if success {
		m.SuccessTotal++
	} else {
		m.FailuresTotal++
	}
}

// RecordRetrySuccess records a successful retry.
func (m *DispatcherMetrics) RecordRetrySuccess(eventType shared.EventType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.RetriesTotal++
	m.RetrySuccesses++
}

// RecordFailure records a handler failure (after all retries).
func (m *DispatcherMetrics) RecordFailure(eventType shared.EventType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.FailuresTotal++
}

// Snapshot returns a point-in-time snapshot.
func (m *DispatcherMetrics) Snapshot() DispatcherMetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgDuration := time.Duration(0)
	if m.ExecutionsTotal > 0 {
		avgDuration = m.TotalDuration / time.Duration(m.ExecutionsTotal)
	}

	successRate := 1.0
	if m.ExecutionsTotal > 0 {
		successRate = float64(m.SuccessTotal) / float64(m.ExecutionsTotal)
	}

	var totalDispatched int64
	for _, v := range m.DispatchedTotal {
		totalDispatched += v
	}

	return DispatcherMetricsSnapshot{
		TotalDispatched: totalDispatched,
		TotalExecutions: m.ExecutionsTotal,
		TotalFailures:   m.FailuresTotal,
		TotalRetries:    m.RetriesTotal,
		SuccessRate:     successRate,
		AverageDuration: avgDuration,
		LastReset:       m.LastReset,
	}
}

// Reset resets hourly counters.
func (m *DispatcherMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DispatchedLastHour = make(map[shared.EventType]int64)
	m.LastReset = time.Now()
}

// DispatcherMetricsSnapshot is a point-in-time snapshot.
type DispatcherMetricsSnapshot struct {
	TotalDispatched int64
	TotalExecutions int64
	TotalFailures   int64
	TotalRetries    int64
	SuccessRate     float64
	AverageDuration time.Duration
	LastReset       time.Time
}

// ══════════════════════════════════════════════════════════════════════════════
// CONVENIENCE BUILDER
// ══════════════════════════════════════════════════════════════════════════════

// DispatcherBuilder provides fluent API for building a dispatcher.
type DispatcherBuilder struct {
	config DispatcherConfig
}

// NewDispatcherBuilder creates a new builder.
func NewDispatcherBuilder(eventBus shared.EventBus) *DispatcherBuilder {
	return &DispatcherBuilder{
		config: DefaultDispatcherConfig(eventBus),
	}
}

// WithWorkerPoolSize sets the worker pool size.
func (b *DispatcherBuilder) WithWorkerPoolSize(size int) *DispatcherBuilder {
	b.config.WorkerPoolSize = size
	return b
}

// WithRetryConfig sets the retry configuration.
func (b *DispatcherBuilder) WithRetryConfig(config RetryConfig) *DispatcherBuilder {
	b.config.RetryConfig = config
	return b
}

// WithDeadLetterQueue enables the dead letter queue.
func (b *DispatcherBuilder) WithDeadLetterQueue(size int) *DispatcherBuilder {
	b.config.EnableDeadLetterQueue = true
	b.config.DeadLetterQueueSize = size
	return b
}

// WithLogger sets the logger.
func (b *DispatcherBuilder) WithLogger(logger *slog.Logger) *DispatcherBuilder {
	b.config.Logger = logger
	return b
}

// Build creates the dispatcher.
func (b *DispatcherBuilder) Build() *Dispatcher {
	return NewDispatcher(b.config)
}
