package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CronExpression represents a parsed cron expression.
// Supports standard 5-field format: minute hour day-of-month month day-of-week
// Examples:
//   - "*/5 * * * *"  - every 5 minutes
//   - "0 */1 * * *"  - every hour
//   - "0 21 * * *"   - every day at 21:00
//   - "0 0 * * 0"    - every Sunday at midnight
type CronExpression struct {
	raw      string
	minutes  []int // 0-59
	hours    []int // 0-23
	days     []int // 1-31
	months   []int // 1-12
	weekdays []int // 0-6 (0 = Sunday)
}

// CronJob represents a scheduled job with its cron expression.
type CronJob struct {
	Name       string
	Expression *CronExpression
	Job        Job
	LastRun    time.Time
	NextRun    time.Time
	RunCount   int64
	Enabled    bool
}

// CronScheduler manages cron-based job scheduling.
type CronScheduler struct {
	jobs     map[string]*CronJob
	mu       sync.RWMutex
	logger   *slog.Logger
	location *time.Location
	running  bool
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// CronOption configures the CronScheduler.
type CronOption func(*CronScheduler)

// WithLocation sets the timezone for cron expressions.
func WithLocation(loc *time.Location) CronOption {
	return func(cs *CronScheduler) {
		cs.location = loc
	}
}

// WithCronLogger sets the logger for the cron scheduler.
func WithCronLogger(logger *slog.Logger) CronOption {
	return func(cs *CronScheduler) {
		cs.logger = logger
	}
}

// NewCronScheduler creates a new cron-based scheduler.
func NewCronScheduler(opts ...CronOption) *CronScheduler {
	cs := &CronScheduler{
		jobs:     make(map[string]*CronJob),
		logger:   slog.Default(),
		location: time.Local,
		stopCh:   make(chan struct{}),
	}

	for _, opt := range opts {
		opt(cs)
	}

	return cs
}

// ParseCronExpression parses a cron expression string.
// Format: minute hour day-of-month month day-of-week
// Supports: *, */n, n, n-m, n,m,o
func ParseCronExpression(expr string) (*CronExpression, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron expression: expected 5 fields, got %d", len(fields))
	}

	ce := &CronExpression{raw: expr}
	var err error

	ce.minutes, err = parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}

	ce.hours, err = parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}

	ce.days, err = parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day field: %w", err)
	}

	ce.months, err = parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}

	ce.weekdays, err = parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("invalid weekday field: %w", err)
	}

	return ce, nil
}

// parseField parses a single cron field.
func parseField(field string, min, max int) ([]int, error) {
	var result []int

	// Handle wildcard
	if field == "*" {
		for i := min; i <= max; i++ {
			result = append(result, i)
		}
		return result, nil
	}

	// Handle step values (*/n or n-m/s)
	if strings.Contains(field, "/") {
		parts := strings.Split(field, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid step format: %s", field)
		}

		step, err := strconv.Atoi(parts[1])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step value: %s", parts[1])
		}

		var start, end int
		if parts[0] == "*" {
			start, end = min, max
		} else if strings.Contains(parts[0], "-") {
			rangeParts := strings.Split(parts[0], "-")
			start, _ = strconv.Atoi(rangeParts[0])
			end, _ = strconv.Atoi(rangeParts[1])
		} else {
			start, _ = strconv.Atoi(parts[0])
			end = max
		}

		for i := start; i <= end; i += step {
			if i >= min && i <= max {
				result = append(result, i)
			}
		}
		return result, nil
	}

	// Handle ranges (n-m)
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range format: %s", field)
		}

		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid range start: %s", parts[0])
		}

		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid range end: %s", parts[1])
		}

		for i := start; i <= end; i++ {
			if i >= min && i <= max {
				result = append(result, i)
			}
		}
		return result, nil
	}

	// Handle lists (n,m,o)
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, p := range parts {
			v, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				return nil, fmt.Errorf("invalid list value: %s", p)
			}
			if v >= min && v <= max {
				result = append(result, v)
			}
		}
		sort.Ints(result)
		return result, nil
	}

	// Handle single value
	v, err := strconv.Atoi(field)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %s", field)
	}
	if v < min || v > max {
		return nil, fmt.Errorf("value out of range [%d-%d]: %d", min, max, v)
	}
	return []int{v}, nil
}

// String returns the original cron expression.
func (ce *CronExpression) String() string {
	return ce.raw
}

// Next calculates the next time the cron expression matches after the given time.
func (ce *CronExpression) Next(after time.Time) time.Time {
	// Start from the next minute
	t := after.Add(time.Minute).Truncate(time.Minute)

	// Maximum iterations to prevent infinite loops
	const maxIterations = 366 * 24 * 60 // One year in minutes

	for i := 0; i < maxIterations; i++ {
		// Check if current time matches
		if ce.matches(t) {
			return t
		}

		// Advance by one minute
		t = t.Add(time.Minute)
	}

	// Should never reach here with valid expressions
	return time.Time{}
}

// matches checks if the given time matches the cron expression.
func (ce *CronExpression) matches(t time.Time) bool {
	return contains(ce.minutes, t.Minute()) &&
		contains(ce.hours, t.Hour()) &&
		contains(ce.days, t.Day()) &&
		contains(ce.months, int(t.Month())) &&
		contains(ce.weekdays, int(t.Weekday()))
}

// contains checks if a slice contains a value.
func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// AddJob adds a job with a cron expression.
func (cs *CronScheduler) AddJob(name string, cronExpr string, job Job) error {
	expr, err := ParseCronExpression(cronExpr)
	if err != nil {
		return fmt.Errorf("failed to parse cron expression: %w", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	now := time.Now().In(cs.location)
	cs.jobs[name] = &CronJob{
		Name:       name,
		Expression: expr,
		Job:        job,
		NextRun:    expr.Next(now),
		Enabled:    true,
	}

	cs.logger.Info("cron job added",
		"job", name,
		"expression", cronExpr,
		"next_run", cs.jobs[name].NextRun.Format(time.RFC3339),
	)

	return nil
}

// RemoveJob removes a job by name.
func (cs *CronScheduler) RemoveJob(name string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.jobs, name)
	cs.logger.Info("cron job removed", "job", name)
}

// EnableJob enables a job.
func (cs *CronScheduler) EnableJob(name string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	job, exists := cs.jobs[name]
	if !exists {
		return fmt.Errorf("job not found: %s", name)
	}

	job.Enabled = true
	job.NextRun = job.Expression.Next(time.Now().In(cs.location))
	return nil
}

// DisableJob disables a job.
func (cs *CronScheduler) DisableJob(name string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	job, exists := cs.jobs[name]
	if !exists {
		return fmt.Errorf("job not found: %s", name)
	}

	job.Enabled = false
	return nil
}

// GetJobStatus returns the status of a job.
func (cs *CronScheduler) GetJobStatus(name string) (*CronJob, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	job, exists := cs.jobs[name]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent race conditions
	jobCopy := *job
	return &jobCopy, true
}

// ListJobs returns all registered jobs.
func (cs *CronScheduler) ListJobs() []*CronJob {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	jobs := make([]*CronJob, 0, len(cs.jobs))
	for _, job := range cs.jobs {
		jobCopy := *job
		jobs = append(jobs, &jobCopy)
	}

	// Sort by next run time
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].NextRun.Before(jobs[j].NextRun)
	})

	return jobs
}

// Start begins the cron scheduler loop.
func (cs *CronScheduler) Start(ctx context.Context) error {
	cs.mu.Lock()
	if cs.running {
		cs.mu.Unlock()
		return fmt.Errorf("cron scheduler already running")
	}
	cs.running = true
	cs.stopCh = make(chan struct{})
	cs.mu.Unlock()

	cs.logger.Info("cron scheduler started", "timezone", cs.location.String())

	cs.wg.Add(1)
	go cs.run(ctx)

	return nil
}

// Stop gracefully stops the cron scheduler.
func (cs *CronScheduler) Stop() {
	cs.mu.Lock()
	if !cs.running {
		cs.mu.Unlock()
		return
	}
	cs.running = false
	close(cs.stopCh)
	cs.mu.Unlock()

	cs.wg.Wait()
	cs.logger.Info("cron scheduler stopped")
}

// run is the main scheduler loop.
func (cs *CronScheduler) run(ctx context.Context) {
	defer cs.wg.Done()

	// Tick every minute at the start of each minute
	ticker := time.NewTicker(cs.timeUntilNextMinute())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cs.logger.Info("cron scheduler context cancelled")
			return

		case <-cs.stopCh:
			return

		case <-ticker.C:
			// Reset ticker to next minute
			ticker.Reset(cs.timeUntilNextMinute())

			// Check and run due jobs
			cs.checkAndRunJobs(ctx)
		}
	}
}

// timeUntilNextMinute returns the duration until the start of the next minute.
func (cs *CronScheduler) timeUntilNextMinute() time.Duration {
	now := time.Now().In(cs.location)
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	return time.Until(nextMinute)
}

// checkAndRunJobs checks for due jobs and runs them.
func (cs *CronScheduler) checkAndRunJobs(ctx context.Context) {
	now := time.Now().In(cs.location)

	cs.mu.RLock()
	var dueJobs []*CronJob
	for _, job := range cs.jobs {
		if job.Enabled && !job.NextRun.After(now) {
			dueJobs = append(dueJobs, job)
		}
	}
	cs.mu.RUnlock()

	for _, job := range dueJobs {
		cs.runJob(ctx, job, now)
	}
}

// runJob executes a single job.
func (cs *CronScheduler) runJob(ctx context.Context, job *CronJob, now time.Time) {
	cs.mu.Lock()
	// Update job metadata before running
	job.LastRun = now
	job.NextRun = job.Expression.Next(now)
	job.RunCount++
	cs.mu.Unlock()

	cs.logger.Info("running cron job",
		"job", job.Name,
		"run_count", job.RunCount,
	)

	// Run job in a goroutine to not block other jobs
	cs.wg.Add(1)
	go func(j *CronJob) {
		defer cs.wg.Done()

		startTime := time.Now()
		err := j.Job.Run(ctx)
		duration := time.Since(startTime)

		if err != nil {
			cs.logger.Error("cron job failed",
				"job", j.Name,
				"duration", duration,
				"error", err,
			)
		} else {
			cs.logger.Info("cron job completed",
				"job", j.Name,
				"duration", duration,
			)
		}
	}(job)
}

// Common cron expression presets.
const (
	EveryMinute      = "* * * * *"
	Every5Minutes    = "*/5 * * * *"
	Every10Minutes   = "*/10 * * * *"
	Every15Minutes   = "*/15 * * * *"
	Every30Minutes   = "*/30 * * * *"
	EveryHour        = "0 * * * *"
	EveryDay9AM      = "0 9 * * *"
	EveryDay21PM     = "0 21 * * *"
	EveryDayMidnight = "0 0 * * *"
	EverySunday      = "0 0 * * 0"
	EveryMonday      = "0 0 * * 1"
	FirstOfMonth     = "0 0 1 * *"
)

// MustParseCronExpression parses a cron expression or panics.
// Use only for compile-time constants.
func MustParseCronExpression(expr string) *CronExpression {
	ce, err := ParseCronExpression(expr)
	if err != nil {
		panic(fmt.Sprintf("invalid cron expression %q: %v", expr, err))
	}
	return ce
}
