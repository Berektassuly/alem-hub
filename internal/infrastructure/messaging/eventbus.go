// Package messaging implements event bus functionality for the Alem Community Hub.
// It provides both in-memory and Redis-based event buses for event-driven architecture.
package messaging

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// IN-MEMORY EVENT BUS
// ══════════════════════════════════════════════════════════════════════════════

// InMemoryEventBus is a simple in-memory implementation of EventBus.
// Suitable for single-instance deployments and testing.
type InMemoryEventBus struct {
	mu          sync.RWMutex
	handlers    map[shared.EventType][]shared.EventHandler
	allHandlers []shared.EventHandler
	asyncMode   bool
	workerPool  chan struct{}
	logger      *slog.Logger
	metrics     *EventBusMetrics
	closed      bool
	closeCh     chan struct{}
	wg          sync.WaitGroup
}

// InMemoryEventBusConfig contains configuration for InMemoryEventBus.
type InMemoryEventBusConfig struct {
	// AsyncMode enables asynchronous event processing
	AsyncMode bool

	// WorkerPoolSize is the number of concurrent workers for async processing
	WorkerPoolSize int

	// Logger for structured logging
	Logger *slog.Logger

	// EnableMetrics enables metrics collection
	EnableMetrics bool
}

// DefaultInMemoryEventBusConfig returns sensible defaults.
func DefaultInMemoryEventBusConfig() InMemoryEventBusConfig {
	return InMemoryEventBusConfig{
		AsyncMode:      true,
		WorkerPoolSize: 10,
		EnableMetrics:  true,
	}
}

// NewInMemoryEventBus creates a new in-memory event bus.
func NewInMemoryEventBus(config InMemoryEventBusConfig) *InMemoryEventBus {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.WorkerPoolSize <= 0 {
		config.WorkerPoolSize = 10
	}

	bus := &InMemoryEventBus{
		handlers:    make(map[shared.EventType][]shared.EventHandler),
		allHandlers: make([]shared.EventHandler, 0),
		asyncMode:   config.AsyncMode,
		workerPool:  make(chan struct{}, config.WorkerPoolSize),
		logger:      config.Logger,
		closeCh:     make(chan struct{}),
	}

	if config.EnableMetrics {
		bus.metrics = NewEventBusMetrics()
	}

	return bus
}

// Subscribe registers a handler for a specific event type.
func (b *InMemoryEventBus) Subscribe(eventType shared.EventType, handler shared.EventHandler) error {
	if handler == nil {
		return errors.New("handler cannot be nil")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrEventBusClosed
	}

	b.handlers[eventType] = append(b.handlers[eventType], handler)
	b.logger.Debug("subscribed handler", "event_type", eventType)

	return nil
}

// SubscribeAll registers a handler for all events.
func (b *InMemoryEventBus) SubscribeAll(handler shared.EventHandler) error {
	if handler == nil {
		return errors.New("handler cannot be nil")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrEventBusClosed
	}

	b.allHandlers = append(b.allHandlers, handler)
	b.logger.Debug("subscribed global handler")

	return nil
}

// Publish sends an event to all subscribed handlers.
func (b *InMemoryEventBus) Publish(event shared.Event) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrEventBusClosed
	}

	// Collect handlers to call
	handlers := make([]shared.EventHandler, 0)
	handlers = append(handlers, b.handlers[event.EventType()]...)
	handlers = append(handlers, b.allHandlers...)
	b.mu.RUnlock()

	if len(handlers) == 0 {
		b.logger.Debug("no handlers for event", "event_type", event.EventType())
		return nil
	}

	// Track metrics
	if b.metrics != nil {
		b.metrics.RecordPublish(event.EventType())
	}

	// Execute handlers
	if b.asyncMode {
		for _, handler := range handlers {
			b.executeAsync(event, handler)
		}
	} else {
		for _, handler := range handlers {
			if err := b.executeSync(event, handler); err != nil {
				b.logger.Error("handler error", "event_type", event.EventType(), "error", err)
			}
		}
	}

	return nil
}

// executeAsync executes a handler asynchronously using the worker pool.
func (b *InMemoryEventBus) executeAsync(event shared.Event, handler shared.EventHandler) {
	b.wg.Add(1)

	go func() {
		defer b.wg.Done()

		// Acquire worker slot
		select {
		case b.workerPool <- struct{}{}:
			defer func() { <-b.workerPool }()
		case <-b.closeCh:
			return
		}

		start := time.Now()
		err := handler(event)
		duration := time.Since(start)

		if b.metrics != nil {
			b.metrics.RecordHandlerExecution(event.EventType(), duration, err == nil)
		}

		if err != nil {
			b.logger.Error("async handler error",
				"event_type", event.EventType(),
				"duration", duration,
				"error", err,
			)
		}
	}()
}

// executeSync executes a handler synchronously.
func (b *InMemoryEventBus) executeSync(event shared.Event, handler shared.EventHandler) error {
	start := time.Now()
	err := handler(event)
	duration := time.Since(start)

	if b.metrics != nil {
		b.metrics.RecordHandlerExecution(event.EventType(), duration, err == nil)
	}

	return err
}

// Close gracefully shuts down the event bus.
func (b *InMemoryEventBus) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	close(b.closeCh)
	b.mu.Unlock()

	// Wait for pending handlers to complete
	b.wg.Wait()

	b.logger.Info("event bus closed")
	return nil
}

// Metrics returns the current metrics.
func (b *InMemoryEventBus) Metrics() *EventBusMetrics {
	return b.metrics
}

// ══════════════════════════════════════════════════════════════════════════════
// REDIS EVENT BUS
// ══════════════════════════════════════════════════════════════════════════════

// RedisEventBus is a Redis Pub/Sub based implementation of EventBus.
// Suitable for distributed deployments where multiple instances need to share events.
type RedisEventBus struct {
	client      RedisClient
	localBus    *InMemoryEventBus
	channelName string
	instanceID  string
	logger      *slog.Logger
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.RWMutex
	closed      bool
}

// RedisClient defines the interface for Redis operations.
// This allows for easy mocking and different Redis client implementations.
type RedisClient interface {
	Publish(ctx context.Context, channel string, message interface{}) error
	Subscribe(ctx context.Context, channels ...string) (<-chan RedisMessage, error)
	Close() error
}

// RedisMessage represents a message received from Redis Pub/Sub.
type RedisMessage struct {
	Channel string
	Payload string
	Err     error
}

// RedisEventBusConfig contains configuration for RedisEventBus.
type RedisEventBusConfig struct {
	// Client is the Redis client to use
	Client RedisClient

	// ChannelName is the Redis channel for events (default: "alem-hub:events")
	ChannelName string

	// InstanceID uniquely identifies this instance (for filtering self-published events)
	InstanceID string

	// LocalBusConfig is the config for the local in-memory bus
	LocalBusConfig InMemoryEventBusConfig

	// Logger for structured logging
	Logger *slog.Logger
}

// NewRedisEventBus creates a new Redis-based event bus.
func NewRedisEventBus(config RedisEventBusConfig) (*RedisEventBus, error) {
	if config.Client == nil {
		return nil, errors.New("redis client is required")
	}
	if config.ChannelName == "" {
		config.ChannelName = "alem-hub:events"
	}
	if config.InstanceID == "" {
		config.InstanceID = generateInstanceID()
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	bus := &RedisEventBus{
		client:      config.Client,
		localBus:    NewInMemoryEventBus(config.LocalBusConfig),
		channelName: config.ChannelName,
		instanceID:  config.InstanceID,
		logger:      config.Logger,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start subscription listener
	if err := bus.startSubscriber(); err != nil {
		cancel()
		return nil, fmt.Errorf("start subscriber: %w", err)
	}

	return bus, nil
}

// Subscribe registers a handler for a specific event type.
func (b *RedisEventBus) Subscribe(eventType shared.EventType, handler shared.EventHandler) error {
	return b.localBus.Subscribe(eventType, handler)
}

// SubscribeAll registers a handler for all events.
func (b *RedisEventBus) SubscribeAll(handler shared.EventHandler) error {
	return b.localBus.SubscribeAll(handler)
}

// Publish sends an event to Redis Pub/Sub and local handlers.
func (b *RedisEventBus) Publish(event shared.Event) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrEventBusClosed
	}
	b.mu.RUnlock()

	// Serialize event for Redis
	envelope := eventEnvelope{
		InstanceID:  b.instanceID,
		EventType:   event.EventType(),
		AggregateID: event.AggregateID(),
		OccurredAt:  event.OccurredAt(),
		Payload:     event.Payload(),
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// Publish to Redis
	if err := b.client.Publish(b.ctx, b.channelName, string(data)); err != nil {
		b.logger.Error("failed to publish to redis", "error", err)
		// Fall back to local processing
	}

	// Also publish locally for handlers in this instance
	return b.localBus.Publish(event)
}

// startSubscriber starts the Redis subscription listener.
func (b *RedisEventBus) startSubscriber() error {
	messages, err := b.client.Subscribe(b.ctx, b.channelName)
	if err != nil {
		return err
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.subscriptionLoop(messages)
	}()

	return nil
}

// subscriptionLoop processes messages from Redis.
func (b *RedisEventBus) subscriptionLoop(messages <-chan RedisMessage) {
	for {
		select {
		case <-b.ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}
			if msg.Err != nil {
				b.logger.Error("redis subscription error", "error", msg.Err)
				continue
			}

			b.handleRedisMessage(msg)
		}
	}
}

// handleRedisMessage processes a message from Redis.
func (b *RedisEventBus) handleRedisMessage(msg RedisMessage) {
	var envelope eventEnvelope
	if err := json.Unmarshal([]byte(msg.Payload), &envelope); err != nil {
		b.logger.Error("failed to unmarshal event", "error", err)
		return
	}

	// Skip events from self (already processed locally)
	if envelope.InstanceID == b.instanceID {
		return
	}

	// Create a reconstructed event
	event := &reconstructedEvent{
		eventType:   envelope.EventType,
		aggregateID: envelope.AggregateID,
		occurredAt:  envelope.OccurredAt,
		payload:     envelope.Payload,
	}

	// Process through local handlers
	if err := b.localBus.Publish(event); err != nil {
		b.logger.Error("failed to process remote event", "error", err)
	}
}

// Close gracefully shuts down the Redis event bus.
func (b *RedisEventBus) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	b.cancel()
	b.wg.Wait()

	if err := b.localBus.Close(); err != nil {
		b.logger.Error("failed to close local bus", "error", err)
	}

	b.logger.Info("redis event bus closed")
	return nil
}

// Metrics returns the current metrics from the local bus.
func (b *RedisEventBus) Metrics() *EventBusMetrics {
	return b.localBus.Metrics()
}

// ══════════════════════════════════════════════════════════════════════════════
// EVENT ENVELOPE (for serialization)
// ══════════════════════════════════════════════════════════════════════════════

type eventEnvelope struct {
	InstanceID  string                 `json:"instance_id"`
	EventType   shared.EventType       `json:"event_type"`
	AggregateID string                 `json:"aggregate_id"`
	OccurredAt  time.Time              `json:"occurred_at"`
	Payload     map[string]interface{} `json:"payload"`
}

// reconstructedEvent is used to recreate events from Redis messages.
type reconstructedEvent struct {
	eventType   shared.EventType
	aggregateID string
	occurredAt  time.Time
	payload     map[string]interface{}
}

func (e *reconstructedEvent) EventType() shared.EventType {
	return e.eventType
}

func (e *reconstructedEvent) AggregateID() string {
	return e.aggregateID
}

func (e *reconstructedEvent) OccurredAt() time.Time {
	return e.occurredAt
}

func (e *reconstructedEvent) Payload() map[string]interface{} {
	return e.payload
}

// ══════════════════════════════════════════════════════════════════════════════
// METRICS
// ══════════════════════════════════════════════════════════════════════════════

// EventBusMetrics tracks event bus performance metrics.
type EventBusMetrics struct {
	mu sync.RWMutex

	// Publish metrics
	PublishedTotal    map[shared.EventType]int64
	PublishedLastHour map[shared.EventType]int64

	// Handler execution metrics
	HandlerExecutions      int64
	HandlerSuccesses       int64
	HandlerFailures        int64
	HandlerTotalDuration   time.Duration
	HandlersByType         map[shared.EventType]int64
	HandlerDurationsByType map[shared.EventType]time.Duration

	// Last reset time
	LastReset time.Time
}

// NewEventBusMetrics creates new metrics tracker.
func NewEventBusMetrics() *EventBusMetrics {
	return &EventBusMetrics{
		PublishedTotal:         make(map[shared.EventType]int64),
		PublishedLastHour:      make(map[shared.EventType]int64),
		HandlersByType:         make(map[shared.EventType]int64),
		HandlerDurationsByType: make(map[shared.EventType]time.Duration),
		LastReset:              time.Now(),
	}
}

// RecordPublish records a publish event.
func (m *EventBusMetrics) RecordPublish(eventType shared.EventType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.PublishedTotal[eventType]++
	m.PublishedLastHour[eventType]++
}

// RecordHandlerExecution records a handler execution.
func (m *EventBusMetrics) RecordHandlerExecution(eventType shared.EventType, duration time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.HandlerExecutions++
	m.HandlerTotalDuration += duration
	m.HandlersByType[eventType]++
	m.HandlerDurationsByType[eventType] += duration

	if success {
		m.HandlerSuccesses++
	} else {
		m.HandlerFailures++
	}
}

// Snapshot returns a copy of current metrics.
func (m *EventBusMetrics) Snapshot() EventBusMetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgDuration := time.Duration(0)
	if m.HandlerExecutions > 0 {
		avgDuration = m.HandlerTotalDuration / time.Duration(m.HandlerExecutions)
	}

	return EventBusMetricsSnapshot{
		TotalPublished:         m.sumMap(m.PublishedTotal),
		TotalHandlerExecs:      m.HandlerExecutions,
		HandlerSuccessRate:     m.successRate(),
		AverageHandlerDuration: avgDuration,
		LastReset:              m.LastReset,
	}
}

func (m *EventBusMetrics) sumMap(mp map[shared.EventType]int64) int64 {
	var sum int64
	for _, v := range mp {
		sum += v
	}
	return sum
}

func (m *EventBusMetrics) successRate() float64 {
	if m.HandlerExecutions == 0 {
		return 1.0
	}
	return float64(m.HandlerSuccesses) / float64(m.HandlerExecutions)
}

// Reset resets hourly metrics.
func (m *EventBusMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.PublishedLastHour = make(map[shared.EventType]int64)
	m.LastReset = time.Now()
}

// EventBusMetricsSnapshot is a point-in-time snapshot of metrics.
type EventBusMetricsSnapshot struct {
	TotalPublished         int64
	TotalHandlerExecs      int64
	HandlerSuccessRate     float64
	AverageHandlerDuration time.Duration
	LastReset              time.Time
}

// ══════════════════════════════════════════════════════════════════════════════
// ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrEventBusClosed is returned when operations are attempted on a closed bus.
	ErrEventBusClosed = errors.New("event bus is closed")

	// ErrHandlerPanic is returned when a handler panics.
	ErrHandlerPanic = errors.New("handler panicked")

	// ErrEventNotSupported is returned for unknown event types.
	ErrEventNotSupported = errors.New("event type not supported")
)

// ══════════════════════════════════════════════════════════════════════════════
// HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// generateInstanceID generates a unique instance identifier.
func generateInstanceID() string {
	return fmt.Sprintf("instance-%d", time.Now().UnixNano())
}

// ══════════════════════════════════════════════════════════════════════════════
// BUFFERED EVENT BUS (for batch processing)
// ══════════════════════════════════════════════════════════════════════════════

// BufferedEventBus buffers events and flushes them in batches.
type BufferedEventBus struct {
	inner       shared.EventBus
	buffer      []shared.Event
	bufferSize  int
	flushTicker *time.Ticker
	mu          sync.Mutex
	logger      *slog.Logger
	closed      bool
	closeCh     chan struct{}
	wg          sync.WaitGroup
}

// BufferedEventBusConfig contains configuration for BufferedEventBus.
type BufferedEventBusConfig struct {
	// Inner is the underlying event bus
	Inner shared.EventBus

	// BufferSize is the maximum events to buffer before flushing
	BufferSize int

	// FlushInterval is how often to flush regardless of buffer size
	FlushInterval time.Duration

	// Logger for structured logging
	Logger *slog.Logger
}

// NewBufferedEventBus creates a new buffered event bus.
func NewBufferedEventBus(config BufferedEventBusConfig) *BufferedEventBus {
	if config.BufferSize <= 0 {
		config.BufferSize = 100
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = time.Second
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	bus := &BufferedEventBus{
		inner:       config.Inner,
		buffer:      make([]shared.Event, 0, config.BufferSize),
		bufferSize:  config.BufferSize,
		flushTicker: time.NewTicker(config.FlushInterval),
		logger:      config.Logger,
		closeCh:     make(chan struct{}),
	}

	bus.wg.Add(1)
	go bus.flushLoop()

	return bus
}

// Subscribe delegates to inner bus.
func (b *BufferedEventBus) Subscribe(eventType shared.EventType, handler shared.EventHandler) error {
	return b.inner.Subscribe(eventType, handler)
}

// SubscribeAll delegates to inner bus.
func (b *BufferedEventBus) SubscribeAll(handler shared.EventHandler) error {
	return b.inner.SubscribeAll(handler)
}

// Publish buffers the event for batch processing.
func (b *BufferedEventBus) Publish(event shared.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrEventBusClosed
	}

	b.buffer = append(b.buffer, event)

	// Flush if buffer is full
	if len(b.buffer) >= b.bufferSize {
		b.flushLocked()
	}

	return nil
}

// Flush manually flushes the buffer.
func (b *BufferedEventBus) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked()
}

func (b *BufferedEventBus) flushLocked() error {
	if len(b.buffer) == 0 {
		return nil
	}

	events := b.buffer
	b.buffer = make([]shared.Event, 0, b.bufferSize)

	// Publish all buffered events
	var lastErr error
	for _, event := range events {
		if err := b.inner.Publish(event); err != nil {
			b.logger.Error("failed to publish buffered event", "error", err)
			lastErr = err
		}
	}

	return lastErr
}

func (b *BufferedEventBus) flushLoop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.closeCh:
			return
		case <-b.flushTicker.C:
			b.mu.Lock()
			b.flushLocked()
			b.mu.Unlock()
		}
	}
}

// Close flushes remaining events and closes the bus.
func (b *BufferedEventBus) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.flushTicker.Stop()
	close(b.closeCh)
	b.flushLocked()
	b.mu.Unlock()

	b.wg.Wait()
	return nil
}
