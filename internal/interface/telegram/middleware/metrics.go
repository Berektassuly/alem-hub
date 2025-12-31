// Package middleware contains Telegram bot middlewares for request processing.
package middleware

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// METRICS MIDDLEWARE
// Collects and exposes metrics about bot usage, performance, and errors.
// Philosophy: "You can't improve what you don't measure." This middleware
// provides insights into how students use the bot and where bottlenecks exist.
// ══════════════════════════════════════════════════════════════════════════════

// MetricsConfig holds configuration for the metrics middleware.
type MetricsConfig struct {
	// EnableDetailedTiming enables per-command latency percentile tracking.
	EnableDetailedTiming bool

	// HistogramBuckets defines the latency histogram buckets in milliseconds.
	HistogramBuckets []float64

	// RetentionPeriod is how long to keep historical metrics.
	RetentionPeriod time.Duration

	// AggregationInterval is how often to aggregate metrics.
	AggregationInterval time.Duration

	// OnSlowRequest is called when a request exceeds the slow threshold.
	OnSlowRequest func(command string, duration time.Duration, telegramID int64)

	// SlowRequestThreshold defines what's considered a slow request.
	SlowRequestThreshold time.Duration
}

// DefaultMetricsConfig returns sensible defaults for metrics middleware.
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		EnableDetailedTiming: true,
		HistogramBuckets:     []float64{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
		RetentionPeriod:      24 * time.Hour,
		AggregationInterval:  time.Minute,
		SlowRequestThreshold: 2 * time.Second,
		OnSlowRequest:        nil,
	}
}

// MetricsMiddleware collects and exposes metrics.
type MetricsMiddleware struct {
	config MetricsConfig

	// Global counters
	totalRequests  atomic.Int64
	totalErrors    atomic.Int64
	activeRequests atomic.Int64

	// Per-command metrics
	commandMetrics sync.Map // map[string]*CommandMetrics

	// Error tracking
	errorCounts sync.Map // map[string]*atomic.Int64

	// User activity tracking
	uniqueUsers sync.Map // map[int64]time.Time

	// Time-series data
	timeSeries *TimeSeries

	// Latency histogram
	latencyHistogram *LatencyHistogram
}

// CommandMetrics holds metrics for a specific command.
type CommandMetrics struct {
	mu sync.RWMutex

	// Name is the command name.
	Name string

	// Total number of invocations.
	TotalCount atomic.Int64

	// Number of successful invocations.
	SuccessCount atomic.Int64

	// Number of failed invocations.
	ErrorCount atomic.Int64

	// Timing metrics (in nanoseconds).
	TotalDuration atomic.Int64
	MinDuration   atomic.Int64
	MaxDuration   atomic.Int64

	// Latencies for percentile calculation.
	latencies []time.Duration

	// Last invocation time.
	LastInvoked atomic.Value // time.Time

	// Unique users who used this command.
	uniqueUsers map[int64]struct{}
}

// NewMetricsMiddleware creates a new metrics middleware.
func NewMetricsMiddleware(config MetricsConfig) *MetricsMiddleware {
	m := &MetricsMiddleware{
		config:           config,
		timeSeries:       NewTimeSeries(config.RetentionPeriod, config.AggregationInterval),
		latencyHistogram: NewLatencyHistogram(config.HistogramBuckets),
	}

	// Start background aggregation
	go m.aggregationLoop()

	return m
}

// RequestContext holds context for metrics collection.
type RequestContext struct {
	// Command being executed.
	Command string

	// TelegramID of the user.
	TelegramID int64

	// StartTime when the request started.
	StartTime time.Time

	// middleware reference for recording metrics
	middleware *MetricsMiddleware
}

// Start begins tracking a new request.
func (m *MetricsMiddleware) Start(command string, telegramID int64) *RequestContext {
	m.totalRequests.Add(1)
	m.activeRequests.Add(1)

	// Track unique user
	m.uniqueUsers.Store(telegramID, time.Now())

	// Record start time in time series
	m.timeSeries.RecordRequest()

	return &RequestContext{
		Command:    command,
		TelegramID: telegramID,
		StartTime:  time.Now(),
		middleware: m,
	}
}

// End completes tracking for a request.
func (rc *RequestContext) End(err error) {
	m := rc.middleware
	duration := time.Since(rc.StartTime)

	m.activeRequests.Add(-1)

	// Get or create command metrics
	metrics := m.getCommandMetrics(rc.Command)

	// Update counters
	metrics.TotalCount.Add(1)
	if err != nil {
		metrics.ErrorCount.Add(1)
		m.totalErrors.Add(1)
		m.recordError(err.Error())
	} else {
		metrics.SuccessCount.Add(1)
	}

	// Update timing
	durationNanos := duration.Nanoseconds()
	metrics.TotalDuration.Add(durationNanos)

	// Update min/max (using CAS)
	for {
		currentMin := metrics.MinDuration.Load()
		if currentMin != 0 && currentMin <= durationNanos {
			break
		}
		if metrics.MinDuration.CompareAndSwap(currentMin, durationNanos) {
			break
		}
	}

	for {
		currentMax := metrics.MaxDuration.Load()
		if currentMax >= durationNanos {
			break
		}
		if metrics.MaxDuration.CompareAndSwap(currentMax, durationNanos) {
			break
		}
	}

	// Record latency for percentiles
	metrics.mu.Lock()
	metrics.latencies = append(metrics.latencies, duration)
	if metrics.uniqueUsers == nil {
		metrics.uniqueUsers = make(map[int64]struct{})
	}
	metrics.uniqueUsers[rc.TelegramID] = struct{}{}
	metrics.mu.Unlock()

	metrics.LastInvoked.Store(time.Now())

	// Record in histogram
	m.latencyHistogram.Record(rc.Command, duration)

	// Record in time series
	m.timeSeries.RecordLatency(duration)
	if err != nil {
		m.timeSeries.RecordError()
	}

	// Check for slow request
	if m.config.OnSlowRequest != nil && duration > m.config.SlowRequestThreshold {
		m.config.OnSlowRequest(rc.Command, duration, rc.TelegramID)
	}
}

// EndWithError is a convenience method for ending with an error.
func (rc *RequestContext) EndWithError(err error) {
	rc.End(err)
}

// EndSuccess is a convenience method for ending successfully.
func (rc *RequestContext) EndSuccess() {
	rc.End(nil)
}

// getCommandMetrics returns metrics for a command, creating if needed.
func (m *MetricsMiddleware) getCommandMetrics(command string) *CommandMetrics {
	if val, ok := m.commandMetrics.Load(command); ok {
		return val.(*CommandMetrics)
	}

	metrics := &CommandMetrics{
		Name:      command,
		latencies: make([]time.Duration, 0, 1000),
	}

	actual, _ := m.commandMetrics.LoadOrStore(command, metrics)
	return actual.(*CommandMetrics)
}

// recordError records an error occurrence.
func (m *MetricsMiddleware) recordError(errType string) {
	// Simplify error type for grouping
	simplified := simplifyError(errType)

	val, _ := m.errorCounts.LoadOrStore(simplified, &atomic.Int64{})
	val.(*atomic.Int64).Add(1)
}

// ══════════════════════════════════════════════════════════════════════════════
// METRICS SNAPSHOT
// Point-in-time view of all collected metrics.
// ══════════════════════════════════════════════════════════════════════════════

// MetricsSnapshot represents a point-in-time snapshot of all metrics.
type MetricsSnapshot struct {
	// Timestamp when the snapshot was taken.
	Timestamp time.Time

	// Global metrics.
	TotalRequests  int64
	TotalErrors    int64
	ActiveRequests int64
	ErrorRate      float64

	// Per-command metrics.
	Commands map[string]*CommandSnapshot

	// Unique users in the last hour/day.
	UniqueUsersLastHour int
	UniqueUsersLastDay  int

	// Top errors.
	TopErrors []ErrorCount

	// Latency percentiles (overall).
	LatencyP50 time.Duration
	LatencyP95 time.Duration
	LatencyP99 time.Duration
	LatencyAvg time.Duration
	LatencyMax time.Duration

	// Time-series data.
	RequestsPerMinute []TimeSeriesPoint
	ErrorsPerMinute   []TimeSeriesPoint
	LatencyPerMinute  []TimeSeriesPoint
}

// CommandSnapshot represents metrics for a single command.
type CommandSnapshot struct {
	Name         string
	TotalCount   int64
	SuccessCount int64
	ErrorCount   int64
	ErrorRate    float64
	AvgDuration  time.Duration
	MinDuration  time.Duration
	MaxDuration  time.Duration
	P50Duration  time.Duration
	P95Duration  time.Duration
	P99Duration  time.Duration
	UniqueUsers  int
	LastInvoked  time.Time
}

// ErrorCount represents an error type and its count.
type ErrorCount struct {
	Error string
	Count int64
}

// TimeSeriesPoint represents a single point in time-series data.
type TimeSeriesPoint struct {
	Timestamp time.Time
	Value     float64
}

// Snapshot returns a point-in-time snapshot of all metrics.
func (m *MetricsMiddleware) Snapshot() *MetricsSnapshot {
	snap := &MetricsSnapshot{
		Timestamp:      time.Now(),
		TotalRequests:  m.totalRequests.Load(),
		TotalErrors:    m.totalErrors.Load(),
		ActiveRequests: m.activeRequests.Load(),
		Commands:       make(map[string]*CommandSnapshot),
	}

	// Calculate error rate
	if snap.TotalRequests > 0 {
		snap.ErrorRate = float64(snap.TotalErrors) / float64(snap.TotalRequests)
	}

	// Collect command metrics
	m.commandMetrics.Range(func(key, value interface{}) bool {
		cmd := value.(*CommandMetrics)
		snap.Commands[key.(string)] = cmd.snapshot()
		return true
	})

	// Count unique users
	now := time.Now()
	hourAgo := now.Add(-time.Hour)
	dayAgo := now.Add(-24 * time.Hour)

	m.uniqueUsers.Range(func(key, value interface{}) bool {
		lastSeen := value.(time.Time)
		if lastSeen.After(dayAgo) {
			snap.UniqueUsersLastDay++
			if lastSeen.After(hourAgo) {
				snap.UniqueUsersLastHour++
			}
		}
		return true
	})

	// Collect top errors
	errorList := make([]ErrorCount, 0)
	m.errorCounts.Range(func(key, value interface{}) bool {
		errorList = append(errorList, ErrorCount{
			Error: key.(string),
			Count: value.(*atomic.Int64).Load(),
		})
		return true
	})
	sort.Slice(errorList, func(i, j int) bool {
		return errorList[i].Count > errorList[j].Count
	})
	if len(errorList) > 10 {
		errorList = errorList[:10]
	}
	snap.TopErrors = errorList

	// Get overall latency percentiles
	snap.LatencyP50, snap.LatencyP95, snap.LatencyP99 = m.latencyHistogram.Percentiles()
	snap.LatencyAvg, snap.LatencyMax = m.latencyHistogram.Summary()

	// Get time series data
	snap.RequestsPerMinute = m.timeSeries.GetRequests()
	snap.ErrorsPerMinute = m.timeSeries.GetErrors()
	snap.LatencyPerMinute = m.timeSeries.GetLatencies()

	return snap
}

// snapshot creates a snapshot of command metrics.
func (cm *CommandMetrics) snapshot() *CommandSnapshot {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	total := cm.TotalCount.Load()
	errors := cm.ErrorCount.Load()
	totalDuration := cm.TotalDuration.Load()

	snap := &CommandSnapshot{
		Name:         cm.Name,
		TotalCount:   total,
		SuccessCount: cm.SuccessCount.Load(),
		ErrorCount:   errors,
		MinDuration:  time.Duration(cm.MinDuration.Load()),
		MaxDuration:  time.Duration(cm.MaxDuration.Load()),
		UniqueUsers:  len(cm.uniqueUsers),
	}

	if total > 0 {
		snap.ErrorRate = float64(errors) / float64(total)
		snap.AvgDuration = time.Duration(totalDuration / total)
	}

	// Calculate percentiles
	if len(cm.latencies) > 0 {
		sorted := make([]time.Duration, len(cm.latencies))
		copy(sorted, cm.latencies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

		snap.P50Duration = percentile(sorted, 0.50)
		snap.P95Duration = percentile(sorted, 0.95)
		snap.P99Duration = percentile(sorted, 0.99)
	}

	if lastInvoked, ok := cm.LastInvoked.Load().(time.Time); ok {
		snap.LastInvoked = lastInvoked
	}

	return snap
}

// ══════════════════════════════════════════════════════════════════════════════
// LATENCY HISTOGRAM
// Efficient storage for latency distribution.
// ══════════════════════════════════════════════════════════════════════════════

// LatencyHistogram tracks latency distribution.
type LatencyHistogram struct {
	mu      sync.RWMutex
	buckets []float64
	counts  map[string][]atomic.Int64
	totals  map[string]*atomic.Int64
	maxes   map[string]*atomic.Int64
}

// NewLatencyHistogram creates a new latency histogram.
func NewLatencyHistogram(buckets []float64) *LatencyHistogram {
	return &LatencyHistogram{
		buckets: buckets,
		counts:  make(map[string][]atomic.Int64),
		totals:  make(map[string]*atomic.Int64),
		maxes:   make(map[string]*atomic.Int64),
	}
}

// Record records a latency value.
func (h *LatencyHistogram) Record(command string, duration time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Initialize if needed
	if _, ok := h.counts[command]; !ok {
		h.counts[command] = make([]atomic.Int64, len(h.buckets)+1)
		h.totals[command] = &atomic.Int64{}
		h.maxes[command] = &atomic.Int64{}
	}

	ms := float64(duration.Milliseconds())

	// Find bucket
	bucketIdx := len(h.buckets)
	for i, bucket := range h.buckets {
		if ms <= bucket {
			bucketIdx = i
			break
		}
	}

	h.counts[command][bucketIdx].Add(1)
	h.totals[command].Add(duration.Nanoseconds())

	// Update max
	for {
		current := h.maxes[command].Load()
		if current >= duration.Nanoseconds() {
			break
		}
		if h.maxes[command].CompareAndSwap(current, duration.Nanoseconds()) {
			break
		}
	}
}

// Percentiles returns p50, p95, p99 latencies.
func (h *LatencyHistogram) Percentiles() (p50, p95, p99 time.Duration) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Aggregate all commands
	totalCounts := make([]int64, len(h.buckets)+1)
	var total int64

	for _, counts := range h.counts {
		for i := range counts {
			c := counts[i].Load()
			totalCounts[i] += c
			total += c
		}
	}

	if total == 0 {
		return 0, 0, 0
	}

	p50 = h.percentileFromBuckets(totalCounts, total, 0.50)
	p95 = h.percentileFromBuckets(totalCounts, total, 0.95)
	p99 = h.percentileFromBuckets(totalCounts, total, 0.99)

	return
}

// Summary returns average and max latencies.
func (h *LatencyHistogram) Summary() (avg, max time.Duration) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var totalNanos int64
	var count int64
	var maxNanos int64

	for cmd := range h.totals {
		totalNanos += h.totals[cmd].Load()
		m := h.maxes[cmd].Load()
		if m > maxNanos {
			maxNanos = m
		}
		for i := range h.counts[cmd] {
			count += h.counts[cmd][i].Load()
		}
	}

	if count > 0 {
		avg = time.Duration(totalNanos / count)
	}
	max = time.Duration(maxNanos)

	return
}

func (h *LatencyHistogram) percentileFromBuckets(counts []int64, total int64, p float64) time.Duration {
	target := int64(float64(total) * p)
	var cumulative int64

	for i, count := range counts {
		cumulative += count
		if cumulative >= target {
			if i < len(h.buckets) {
				return time.Duration(h.buckets[i]) * time.Millisecond
			}
			// Beyond highest bucket
			return time.Duration(h.buckets[len(h.buckets)-1]) * time.Millisecond * 2
		}
	}

	return 0
}

// ══════════════════════════════════════════════════════════════════════════════
// TIME SERIES
// Track metrics over time for graphing and analysis.
// ══════════════════════════════════════════════════════════════════════════════

// TimeSeries tracks metrics over time.
type TimeSeries struct {
	mu        sync.RWMutex
	retention time.Duration
	interval  time.Duration

	// Current minute's data
	currentMinute time.Time
	requestCount  atomic.Int64
	errorCount    atomic.Int64
	latencySum    atomic.Int64
	latencyCount  atomic.Int64

	// Historical data
	requests  []TimeSeriesPoint
	errors    []TimeSeriesPoint
	latencies []TimeSeriesPoint
}

// NewTimeSeries creates a new time series tracker.
func NewTimeSeries(retention, interval time.Duration) *TimeSeries {
	return &TimeSeries{
		retention:     retention,
		interval:      interval,
		currentMinute: time.Now().Truncate(time.Minute),
		requests:      make([]TimeSeriesPoint, 0),
		errors:        make([]TimeSeriesPoint, 0),
		latencies:     make([]TimeSeriesPoint, 0),
	}
}

// RecordRequest records a request.
func (ts *TimeSeries) RecordRequest() {
	ts.maybeRotate()
	ts.requestCount.Add(1)
}

// RecordError records an error.
func (ts *TimeSeries) RecordError() {
	ts.maybeRotate()
	ts.errorCount.Add(1)
}

// RecordLatency records a latency value.
func (ts *TimeSeries) RecordLatency(d time.Duration) {
	ts.maybeRotate()
	ts.latencySum.Add(d.Nanoseconds())
	ts.latencyCount.Add(1)
}

// maybeRotate rotates data if minute has changed.
func (ts *TimeSeries) maybeRotate() {
	now := time.Now().Truncate(time.Minute)

	ts.mu.Lock()
	defer ts.mu.Unlock()

	if now.After(ts.currentMinute) {
		// Save current minute's data
		ts.requests = append(ts.requests, TimeSeriesPoint{
			Timestamp: ts.currentMinute,
			Value:     float64(ts.requestCount.Load()),
		})
		ts.errors = append(ts.errors, TimeSeriesPoint{
			Timestamp: ts.currentMinute,
			Value:     float64(ts.errorCount.Load()),
		})

		latencyCount := ts.latencyCount.Load()
		if latencyCount > 0 {
			avgLatency := float64(ts.latencySum.Load()) / float64(latencyCount) / float64(time.Millisecond)
			ts.latencies = append(ts.latencies, TimeSeriesPoint{
				Timestamp: ts.currentMinute,
				Value:     avgLatency,
			})
		}

		// Reset counters
		ts.currentMinute = now
		ts.requestCount.Store(0)
		ts.errorCount.Store(0)
		ts.latencySum.Store(0)
		ts.latencyCount.Store(0)

		// Prune old data
		ts.prune()
	}
}

// prune removes data older than retention period.
func (ts *TimeSeries) prune() {
	threshold := time.Now().Add(-ts.retention)

	ts.requests = prunePoints(ts.requests, threshold)
	ts.errors = prunePoints(ts.errors, threshold)
	ts.latencies = prunePoints(ts.latencies, threshold)
}

func prunePoints(points []TimeSeriesPoint, threshold time.Time) []TimeSeriesPoint {
	idx := 0
	for i, p := range points {
		if p.Timestamp.After(threshold) {
			idx = i
			break
		}
	}
	if idx > 0 {
		return points[idx:]
	}
	return points
}

// GetRequests returns request time series.
func (ts *TimeSeries) GetRequests() []TimeSeriesPoint {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	result := make([]TimeSeriesPoint, len(ts.requests))
	copy(result, ts.requests)
	return result
}

// GetErrors returns error time series.
func (ts *TimeSeries) GetErrors() []TimeSeriesPoint {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	result := make([]TimeSeriesPoint, len(ts.errors))
	copy(result, ts.errors)
	return result
}

// GetLatencies returns latency time series.
func (ts *TimeSeries) GetLatencies() []TimeSeriesPoint {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	result := make([]TimeSeriesPoint, len(ts.latencies))
	copy(result, ts.latencies)
	return result
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// percentile calculates the percentile value from sorted data.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// simplifyError simplifies an error message for grouping.
func simplifyError(err string) string {
	// Truncate long errors
	if len(err) > 100 {
		return err[:100] + "..."
	}
	return err
}

// aggregationLoop runs periodic aggregation.
func (m *MetricsMiddleware) aggregationLoop() {
	ticker := time.NewTicker(m.config.AggregationInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Rotate time series
		m.timeSeries.maybeRotate()

		// Clean up old unique users
		threshold := time.Now().Add(-24 * time.Hour)
		m.uniqueUsers.Range(func(key, value interface{}) bool {
			if value.(time.Time).Before(threshold) {
				m.uniqueUsers.Delete(key)
			}
			return true
		})
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// FORMATTED OUTPUT
// Human-readable metrics output.
// ══════════════════════════════════════════════════════════════════════════════

// String returns a formatted string of the metrics snapshot.
func (s *MetricsSnapshot) String() string {
	return fmt.Sprintf(`=== Bot Metrics ===
Time: %s

Global:
  Total Requests:  %d
  Total Errors:    %d
  Active Requests: %d
  Error Rate:      %.2f%%
  
Users:
  Last Hour: %d
  Last Day:  %d

Latency:
  P50: %v
  P95: %v
  P99: %v
  Avg: %v
  Max: %v

Commands: %d tracked
`,
		s.Timestamp.Format(time.RFC3339),
		s.TotalRequests,
		s.TotalErrors,
		s.ActiveRequests,
		s.ErrorRate*100,
		s.UniqueUsersLastHour,
		s.UniqueUsersLastDay,
		s.LatencyP50,
		s.LatencyP95,
		s.LatencyP99,
		s.LatencyAvg,
		s.LatencyMax,
		len(s.Commands),
	)
}
