// Package scheduler implements background job scheduling for Alem Community Hub.
// It provides cron-like scheduling for periodic tasks such as data synchronization,
// leaderboard rebuilding, and sending notifications.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// JOB INTERFACE
// ══════════════════════════════════════════════════════════════════════════════

// Job defines the interface that all scheduled jobs must implement.
type Job interface {
	// Name returns the unique name of the job.
	Name() string

	// Run executes the job.
	// The context is cancelled when the scheduler is stopping.
	Run(ctx context.Context) error

	// Description returns a human-readable description of the job.
	Description() string
}

// Schedule defines when a job should run.
type Schedule interface {
	// Next returns the next time the job should run after the given time.
	Next(t time.Time) time.Time

	// String returns a human-readable representation of the schedule.
	String() string
}

// JobResult contains the result of a job execution.
type JobResult struct {
	JobName     string
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
	Success     bool
	Error       error
	Metadata    map[string]interface{}
}

// ══════════════════════════════════════════════════════════════════════════════
// SCHEDULER
// ══════════════════════════════════════════════════════════════════════════════

// Scheduler manages and executes scheduled jobs.
type Scheduler struct {
	mu sync.RWMutex

	// Configuration
	logger   *slog.Logger
	timezone *time.Location

	// State
	jobs      map[string]*scheduledJob
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startedAt time.Time

	// Metrics and history
	metrics    *SchedulerMetrics
	lastRuns   map[string]*JobResult
	runHistory []JobResult

	// Hooks
	onJobStart    func(jobName string)
	onJobComplete func(result JobResult)
	onJobError    func(jobName string, err error)
}

// scheduledJob wraps a Job with scheduling information.
type scheduledJob struct {
	job       Job
	schedule  Schedule
	enabled   bool
	lastRun   time.Time
	nextRun   time.Time
	runCount  int64
	failCount int64
}

// SchedulerConfig contains configuration for the Scheduler.
type SchedulerConfig struct {
	// Logger for structured logging.
	Logger *slog.Logger

	// Timezone for schedule calculations (default: UTC).
	Timezone *time.Location

	// MaxHistorySize is the maximum number of job results to keep in history.
	MaxHistorySize int

	// EnableMetrics enables metrics collection.
	EnableMetrics bool
}

// DefaultSchedulerConfig returns sensible defaults.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		Logger:         slog.Default(),
		Timezone:       time.UTC,
		MaxHistorySize: 1000,
		EnableMetrics:  true,
	}
}

// NewScheduler creates a new Scheduler with the given configuration.
func NewScheduler(config SchedulerConfig) *Scheduler {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Timezone == nil {
		config.Timezone = time.UTC
	}
	if config.MaxHistorySize <= 0 {
		config.MaxHistorySize = 1000
	}

	s := &Scheduler{
		logger:     config.Logger,
		timezone:   config.Timezone,
		jobs:       make(map[string]*scheduledJob),
		lastRuns:   make(map[string]*JobResult),
		runHistory: make([]JobResult, 0, config.MaxHistorySize),
	}

	if config.EnableMetrics {
		s.metrics = NewSchedulerMetrics()
	}

	return s
}

// ══════════════════════════════════════════════════════════════════════════════
// JOB REGISTRATION
// ══════════════════════════════════════════════════════════════════════════════

// Register adds a job to the scheduler with the given schedule.
func (s *Scheduler) Register(job Job, schedule Schedule) error {
	if job == nil {
		return ErrNilJob
	}
	if schedule == nil {
		return ErrNilSchedule
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	name := job.Name()
	if _, exists := s.jobs[name]; exists {
		return fmt.Errorf("%w: %s", ErrJobAlreadyExists, name)
	}

	now := time.Now().In(s.timezone)
	sj := &scheduledJob{
		job:      job,
		schedule: schedule,
		enabled:  true,
		nextRun:  schedule.Next(now),
	}

	s.jobs[name] = sj

	s.logger.Info("job registered",
		"job", name,
		"description", job.Description(),
		"next_run", sj.nextRun.Format(time.RFC3339),
	)

	return nil
}

// Unregister removes a job from the scheduler.
func (s *Scheduler) Unregister(jobName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[jobName]; !exists {
		return fmt.Errorf("%w: %s", ErrJobNotFound, jobName)
	}

	delete(s.jobs, jobName)
	s.logger.Info("job unregistered", "job", jobName)

	return nil
}

// EnableJob enables a job by name.
func (s *Scheduler) EnableJob(jobName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sj, exists := s.jobs[jobName]
	if !exists {
		return fmt.Errorf("%w: %s", ErrJobNotFound, jobName)
	}

	sj.enabled = true
	sj.nextRun = sj.schedule.Next(time.Now().In(s.timezone))
	s.logger.Info("job enabled", "job", jobName, "next_run", sj.nextRun)

	return nil
}

// DisableJob disables a job by name.
func (s *Scheduler) DisableJob(jobName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sj, exists := s.jobs[jobName]
	if !exists {
		return fmt.Errorf("%w: %s", ErrJobNotFound, jobName)
	}

	sj.enabled = false
	s.logger.Info("job disabled", "job", jobName)

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// LIFECYCLE
// ══════════════════════════════════════════════════════════════════════════════

// Start begins the scheduler loop.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrSchedulerAlreadyRunning
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true
	s.startedAt = time.Now()
	s.mu.Unlock()

	s.logger.Info("scheduler started", "jobs_count", len(s.jobs))

	s.wg.Add(1)
	go s.runLoop()

	return nil
}

// Stop gracefully stops the scheduler.
// It waits for all currently running jobs to complete.
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return ErrSchedulerNotRunning
	}
	s.running = false
	s.cancel()
	s.mu.Unlock()

	// Wait for the run loop and all jobs to finish
	s.wg.Wait()

	s.logger.Info("scheduler stopped",
		"uptime", time.Since(s.startedAt).String(),
	)

	return nil
}

// IsRunning returns true if the scheduler is running.
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// ══════════════════════════════════════════════════════════════════════════════
// SCHEDULER LOOP
// ══════════════════════════════════════════════════════════════════════════════

// runLoop is the main scheduler loop that checks and runs due jobs.
func (s *Scheduler) runLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkAndRunJobs()
		}
	}
}

// checkAndRunJobs checks all jobs and runs those that are due.
func (s *Scheduler) checkAndRunJobs() {
	now := time.Now().In(s.timezone)

	s.mu.RLock()
	jobsToRun := make([]*scheduledJob, 0)
	for _, sj := range s.jobs {
		if sj.enabled && !sj.nextRun.IsZero() && now.After(sj.nextRun) {
			jobsToRun = append(jobsToRun, sj)
		}
	}
	s.mu.RUnlock()

	// Run due jobs
	for _, sj := range jobsToRun {
		s.wg.Add(1)
		go s.runJob(sj)
	}
}

// runJob executes a single job and records the result.
func (s *Scheduler) runJob(sj *scheduledJob) {
	defer s.wg.Done()

	jobName := sj.job.Name()
	startedAt := time.Now()

	// Call onJobStart hook
	if s.onJobStart != nil {
		s.onJobStart(jobName)
	}

	s.logger.Info("job started", "job", jobName)

	// Update next run time before executing
	s.mu.Lock()
	sj.lastRun = startedAt
	sj.nextRun = sj.schedule.Next(startedAt.In(s.timezone))
	sj.runCount++
	s.mu.Unlock()

	// Execute the job
	err := sj.job.Run(s.ctx)
	completedAt := time.Now()
	duration := completedAt.Sub(startedAt)

	// Build result
	result := JobResult{
		JobName:     jobName,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		Duration:    duration,
		Success:     err == nil,
		Error:       err,
		Metadata:    make(map[string]interface{}),
	}

	// Update metrics
	if s.metrics != nil {
		s.metrics.RecordExecution(jobName, duration, err == nil)
	}

	// Update state
	s.mu.Lock()
	if err != nil {
		sj.failCount++
	}
	s.lastRuns[jobName] = &result
	s.addToHistory(result)
	s.mu.Unlock()

	// Log result
	if err != nil {
		s.logger.Error("job failed",
			"job", jobName,
			"duration", duration.String(),
			"error", err,
		)

		// Call onJobError hook
		if s.onJobError != nil {
			s.onJobError(jobName, err)
		}
	} else {
		s.logger.Info("job completed",
			"job", jobName,
			"duration", duration.String(),
		)
	}

	// Call onJobComplete hook
	if s.onJobComplete != nil {
		s.onJobComplete(result)
	}
}

// addToHistory adds a result to the run history with size limit.
func (s *Scheduler) addToHistory(result JobResult) {
	s.runHistory = append(s.runHistory, result)

	// Trim history if needed
	maxSize := 1000
	if len(s.runHistory) > maxSize {
		s.runHistory = s.runHistory[len(s.runHistory)-maxSize:]
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// MANUAL EXECUTION
// ══════════════════════════════════════════════════════════════════════════════

// RunNow immediately executes a job by name, ignoring its schedule.
func (s *Scheduler) RunNow(ctx context.Context, jobName string) (*JobResult, error) {
	s.mu.RLock()
	sj, exists := s.jobs[jobName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, jobName)
	}

	startedAt := time.Now()
	s.logger.Info("manual job execution started", "job", jobName)

	err := sj.job.Run(ctx)
	completedAt := time.Now()
	duration := completedAt.Sub(startedAt)

	result := &JobResult{
		JobName:     jobName,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		Duration:    duration,
		Success:     err == nil,
		Error:       err,
		Metadata:    map[string]interface{}{"manual": true},
	}

	// Update metrics
	if s.metrics != nil {
		s.metrics.RecordExecution(jobName, duration, err == nil)
	}

	// Update state
	s.mu.Lock()
	s.lastRuns[jobName] = result
	s.addToHistory(*result)
	s.mu.Unlock()

	if err != nil {
		s.logger.Error("manual job execution failed",
			"job", jobName,
			"duration", duration.String(),
			"error", err,
		)
	} else {
		s.logger.Info("manual job execution completed",
			"job", jobName,
			"duration", duration.String(),
		)
	}

	return result, err
}

// ══════════════════════════════════════════════════════════════════════════════
// STATUS & INFO
// ══════════════════════════════════════════════════════════════════════════════

// JobInfo contains information about a registered job.
type JobInfo struct {
	Name        string
	Description string
	Enabled     bool
	Schedule    string
	LastRun     time.Time
	NextRun     time.Time
	RunCount    int64
	FailCount   int64
	LastResult  *JobResult
}

// ListJobs returns information about all registered jobs.
func (s *Scheduler) ListJobs() []JobInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	infos := make([]JobInfo, 0, len(s.jobs))
	for name, sj := range s.jobs {
		info := JobInfo{
			Name:        name,
			Description: sj.job.Description(),
			Enabled:     sj.enabled,
			Schedule:    sj.schedule.String(),
			LastRun:     sj.lastRun,
			NextRun:     sj.nextRun,
			RunCount:    sj.runCount,
			FailCount:   sj.failCount,
			LastResult:  s.lastRuns[name],
		}
		infos = append(infos, info)
	}

	return infos
}

// GetJobInfo returns information about a specific job.
func (s *Scheduler) GetJobInfo(jobName string) (*JobInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sj, exists := s.jobs[jobName]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, jobName)
	}

	info := &JobInfo{
		Name:        jobName,
		Description: sj.job.Description(),
		Enabled:     sj.enabled,
		Schedule:    sj.schedule.String(),
		LastRun:     sj.lastRun,
		NextRun:     sj.nextRun,
		RunCount:    sj.runCount,
		FailCount:   sj.failCount,
		LastResult:  s.lastRuns[jobName],
	}

	return info, nil
}

// GetHistory returns the recent job execution history.
func (s *Scheduler) GetHistory(limit int) []JobResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.runHistory) {
		limit = len(s.runHistory)
	}

	// Return most recent results
	start := len(s.runHistory) - limit
	result := make([]JobResult, limit)
	copy(result, s.runHistory[start:])

	return result
}

// GetMetrics returns scheduler metrics.
func (s *Scheduler) GetMetrics() *SchedulerMetrics {
	return s.metrics
}

// ══════════════════════════════════════════════════════════════════════════════
// HOOKS
// ══════════════════════════════════════════════════════════════════════════════

// OnJobStart sets a callback to be called when a job starts.
func (s *Scheduler) OnJobStart(fn func(jobName string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onJobStart = fn
}

// OnJobComplete sets a callback to be called when a job completes.
func (s *Scheduler) OnJobComplete(fn func(result JobResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onJobComplete = fn
}

// OnJobError sets a callback to be called when a job fails.
func (s *Scheduler) OnJobError(fn func(jobName string, err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onJobError = fn
}

// ══════════════════════════════════════════════════════════════════════════════
// METRICS
// ══════════════════════════════════════════════════════════════════════════════

// SchedulerMetrics tracks scheduler performance metrics.
type SchedulerMetrics struct {
	mu sync.RWMutex

	TotalExecutions int64
	TotalSuccesses  int64
	TotalFailures   int64
	TotalDuration   time.Duration

	ExecutionsByJob map[string]int64
	FailuresByJob   map[string]int64
	DurationsByJob  map[string]time.Duration
	LastExecutions  map[string]time.Time
}

// NewSchedulerMetrics creates a new metrics tracker.
func NewSchedulerMetrics() *SchedulerMetrics {
	return &SchedulerMetrics{
		ExecutionsByJob: make(map[string]int64),
		FailuresByJob:   make(map[string]int64),
		DurationsByJob:  make(map[string]time.Duration),
		LastExecutions:  make(map[string]time.Time),
	}
}

// RecordExecution records a job execution.
func (m *SchedulerMetrics) RecordExecution(jobName string, duration time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalExecutions++
	m.TotalDuration += duration
	m.ExecutionsByJob[jobName]++
	m.DurationsByJob[jobName] += duration
	m.LastExecutions[jobName] = time.Now()

	if success {
		m.TotalSuccesses++
	} else {
		m.TotalFailures++
		m.FailuresByJob[jobName]++
	}
}

// Snapshot returns a point-in-time snapshot of metrics.
func (m *SchedulerMetrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var avgDuration time.Duration
	if m.TotalExecutions > 0 {
		avgDuration = m.TotalDuration / time.Duration(m.TotalExecutions)
	}

	var successRate float64
	if m.TotalExecutions > 0 {
		successRate = float64(m.TotalSuccesses) / float64(m.TotalExecutions)
	}

	return MetricsSnapshot{
		TotalExecutions: m.TotalExecutions,
		TotalSuccesses:  m.TotalSuccesses,
		TotalFailures:   m.TotalFailures,
		SuccessRate:     successRate,
		AverageDuration: avgDuration,
	}
}

// MetricsSnapshot is a point-in-time snapshot of scheduler metrics.
type MetricsSnapshot struct {
	TotalExecutions int64
	TotalSuccesses  int64
	TotalFailures   int64
	SuccessRate     float64
	AverageDuration time.Duration
}

// ══════════════════════════════════════════════════════════════════════════════
// ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrNilJob is returned when trying to register a nil job.
	ErrNilJob = fmt.Errorf("job cannot be nil")

	// ErrNilSchedule is returned when trying to register a job with nil schedule.
	ErrNilSchedule = fmt.Errorf("schedule cannot be nil")

	// ErrJobAlreadyExists is returned when a job with the same name already exists.
	ErrJobAlreadyExists = fmt.Errorf("job already exists")

	// ErrJobNotFound is returned when a job is not found.
	ErrJobNotFound = fmt.Errorf("job not found")

	// ErrSchedulerAlreadyRunning is returned when Start is called on a running scheduler.
	ErrSchedulerAlreadyRunning = fmt.Errorf("scheduler is already running")

	// ErrSchedulerNotRunning is returned when Stop is called on a stopped scheduler.
	ErrSchedulerNotRunning = fmt.Errorf("scheduler is not running")
)
