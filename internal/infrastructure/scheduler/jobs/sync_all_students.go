// Package jobs contains implementations of scheduled jobs for Alem Community Hub.
// Each job follows the philosophy "From Competition to Collaboration",
// focusing on keeping data fresh and students engaged.
package jobs

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// SYNC ALL STUDENTS JOB
// ══════════════════════════════════════════════════════════════════════════════

// SyncAllStudentsJob synchronizes all student data with the Alem Platform API.
// This is the core job that keeps local data in sync with the source of truth.
//
// Philosophy: Fresh data enables accurate leaderboards and timely notifications,
// which are essential for the "From Competition to Collaboration" experience.
type SyncAllStudentsJob struct {
	// Dependencies
	studentRepo    student.Repository
	progressRepo   student.ProgressRepository
	syncRepo       student.SyncRepository
	alemClient     AlemClient
	eventPublisher shared.EventPublisher
	logger         *slog.Logger

	// Configuration
	config SyncAllStudentsConfig

	// State (for metrics)
	lastSyncStats atomic.Value // *SyncStats
}

// SyncAllStudentsConfig contains configuration for the sync job.
type SyncAllStudentsConfig struct {
	// Concurrency is the number of students to sync in parallel.
	Concurrency int

	// BatchSize is the number of students to process per batch.
	BatchSize int

	// MinSyncInterval is the minimum interval between syncs for the same student.
	MinSyncInterval time.Duration

	// Timeout is the maximum duration for the entire sync operation.
	Timeout time.Duration

	// RetryAttempts is the number of retry attempts for failed syncs.
	RetryAttempts int

	// SkipRecentlySynced skips students synced within MinSyncInterval.
	SkipRecentlySynced bool
}

// DefaultSyncAllStudentsConfig returns sensible defaults.
func DefaultSyncAllStudentsConfig() SyncAllStudentsConfig {
	return SyncAllStudentsConfig{
		Concurrency:        5,
		BatchSize:          50,
		MinSyncInterval:    5 * time.Minute,
		Timeout:            10 * time.Minute,
		RetryAttempts:      2,
		SkipRecentlySynced: true,
	}
}

// SyncStats contains statistics from a sync run.
type SyncStats struct {
	StartedAt     time.Time
	CompletedAt   time.Time
	Duration      time.Duration
	TotalStudents int
	SyncedCount   int
	SkippedCount  int
	UpdatedCount  int
	FailedCount   int
	TotalXPDelta  int
	Errors        []SyncError
}

// SyncError represents an error during sync.
type SyncError struct {
	StudentID  string
	Email      string
	Error      error
	OccurredAt time.Time
	RetryCount int
}

// AlemClient defines the interface for fetching data from Alem Platform.
type AlemClient interface {
	// GetAllStudents fetches all students from the Alem Platform.
	GetAllStudents(ctx context.Context) ([]AlemStudentData, error)

	// GetStudentByLogin fetches a single student by login.
	GetStudentByLogin(ctx context.Context, login string) (*AlemStudentData, error)
}

// AlemStudentData represents student data from Alem Platform.
type AlemStudentData struct {
	Login          string
	DisplayName    string
	XP             int
	Level          int
	Cohort         string
	CompletedTasks []string
	LastActivityAt time.Time
	IsOnline       bool
}

// NewSyncAllStudentsJob creates a new sync job.
func NewSyncAllStudentsJob(
	studentRepo student.Repository,
	progressRepo student.ProgressRepository,
	syncRepo student.SyncRepository,
	alemClient AlemClient,
	eventPublisher shared.EventPublisher,
	logger *slog.Logger,
	config SyncAllStudentsConfig,
) *SyncAllStudentsJob {
	if logger == nil {
		logger = slog.Default()
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 5
	}

	return &SyncAllStudentsJob{
		studentRepo:    studentRepo,
		progressRepo:   progressRepo,
		syncRepo:       syncRepo,
		alemClient:     alemClient,
		eventPublisher: eventPublisher,
		logger:         logger,
		config:         config,
	}
}

// Name returns the job name.
func (j *SyncAllStudentsJob) Name() string {
	return "sync_all_students"
}

// Description returns a human-readable description.
func (j *SyncAllStudentsJob) Description() string {
	return "Synchronizes all student data with Alem Platform API"
}

// Run executes the sync job.
func (j *SyncAllStudentsJob) Run(ctx context.Context) error {
	startedAt := time.Now()
	stats := &SyncStats{
		StartedAt: startedAt,
		Errors:    make([]SyncError, 0),
	}

	j.logger.Info("starting sync_all_students job")

	// Apply timeout
	if j.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, j.config.Timeout)
		defer cancel()
	}

	// Get all students to sync
	students, err := j.getStudentsToSync(ctx)
	if err != nil {
		return fmt.Errorf("failed to get students to sync: %w", err)
	}

	stats.TotalStudents = len(students)
	j.logger.Info("found students to sync", "count", stats.TotalStudents)

	if stats.TotalStudents == 0 {
		stats.CompletedAt = time.Now()
		stats.Duration = stats.CompletedAt.Sub(startedAt)
		j.lastSyncStats.Store(stats)
		return nil
	}

	// Fetch all students from Alem API
	alemStudents, err := j.alemClient.GetAllStudents(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch students from Alem API: %w", err)
	}

	// Create a map for quick lookup
	alemStudentMap := make(map[string]AlemStudentData, len(alemStudents))
	for _, as := range alemStudents {
		alemStudentMap[as.Login] = as
	}

	// Sync students concurrently
	j.syncStudentsConcurrently(ctx, students, alemStudentMap, stats)

	// Update last sync time
	if err := j.syncRepo.SetLastSyncTime(ctx, time.Now()); err != nil {
		j.logger.Error("failed to set last sync time", "error", err)
	}

	// Finalize stats
	stats.CompletedAt = time.Now()
	stats.Duration = stats.CompletedAt.Sub(startedAt)
	j.lastSyncStats.Store(stats)

	// Emit sync completed event
	j.emitSyncCompletedEvent(stats)

	j.logger.Info("sync_all_students job completed",
		"duration", stats.Duration.String(),
		"total", stats.TotalStudents,
		"synced", stats.SyncedCount,
		"updated", stats.UpdatedCount,
		"failed", stats.FailedCount,
		"skipped", stats.SkippedCount,
	)

	// Return error if too many failures
	failureRate := float64(stats.FailedCount) / float64(stats.TotalStudents)
	if failureRate > 0.5 {
		return fmt.Errorf("sync failed for more than 50%% of students (%d/%d)",
			stats.FailedCount, stats.TotalStudents)
	}

	return nil
}

// getStudentsToSync returns the list of students that need syncing.
func (j *SyncAllStudentsJob) getStudentsToSync(ctx context.Context) ([]*student.Student, error) {
	opts := student.DefaultListOptions().WithInactive()
	students, err := j.studentRepo.GetAll(ctx, opts)
	if err != nil {
		return nil, err
	}

	if !j.config.SkipRecentlySynced {
		return students, nil
	}

	// Filter out recently synced students
	threshold := time.Now().Add(-j.config.MinSyncInterval)
	filtered := make([]*student.Student, 0, len(students))
	for _, s := range students {
		if s.LastSyncedAt.Before(threshold) {
			filtered = append(filtered, s)
		}
	}

	return filtered, nil
}

// syncStudentsConcurrently syncs students using a worker pool.
func (j *SyncAllStudentsJob) syncStudentsConcurrently(
	ctx context.Context,
	students []*student.Student,
	alemData map[string]AlemStudentData,
	stats *SyncStats,
) {
	var (
		wg        sync.WaitGroup
		semaphore = make(chan struct{}, j.config.Concurrency)
		mu        sync.Mutex
	)

	for _, s := range students {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wg.Add(1)
		semaphore <- struct{}{} // Acquire

		go func(st *student.Student) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release

			// Find Alem data for this student
            // Derive login from email (assuming email is login@alem.school or similar)
            login := ""
            if idx := strings.Index(st.Email, "@"); idx > 0 {
                login = st.Email[:idx]
            }
            
			alemStudent, found := alemData[login]
			if !found {
				mu.Lock()
				stats.SkippedCount++
				mu.Unlock()
				return
			}

			// Sync the student
			updated, xpDelta, err := j.syncStudent(ctx, st, &alemStudent)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				stats.FailedCount++
				stats.Errors = append(stats.Errors, SyncError{
					StudentID:  st.ID,
					Email:      st.Email,
					Error:      err,
					OccurredAt: time.Now(),
				})
				j.logger.Error("failed to sync student",
					"student_id", st.ID,
					"email", st.Email,
					"error", err,
				)
			} else {
				stats.SyncedCount++
				if updated {
					stats.UpdatedCount++
					stats.TotalXPDelta += xpDelta
				}
			}
		}(s)
	}

	wg.Wait()
}

// syncStudent synchronizes a single student with Alem data.
func (j *SyncAllStudentsJob) syncStudent(
	ctx context.Context,
	s *student.Student,
	alemData *AlemStudentData,
) (updated bool, xpDelta int, err error) {
	oldXP := int(s.CurrentXP)
	newXP := student.XP(alemData.XP)

	// Check if XP changed
	if newXP != s.CurrentXP {
		delta, err := s.UpdateXP(newXP)
		if err != nil {
			return false, 0, fmt.Errorf("failed to update XP: %w", err)
		}
		xpDelta = int(delta)
		updated = true

		// Save XP history
		xpEntry := student.XPHistoryEntry{
			Timestamp: time.Now(),
			OldXP:     student.XP(oldXP),
			NewXP:     newXP,
			Delta:     delta,
			Reason:    "sync",
		}
		if err := j.progressRepo.SaveXPChange(ctx, xpEntry); err != nil {
			j.logger.Warn("failed to save XP history",
				"student_id", s.ID,
				"error", err,
			)
		}

		// Emit XPGained event if positive
		if delta > 0 {
			event := shared.NewXPGainedEvent(s.ID, int(delta), int(newXP), "sync", "")
			if err := j.eventPublisher.Publish(event); err != nil {
				j.logger.Warn("failed to publish XPGained event",
					"student_id", s.ID,
					"error", err,
				)
			}
		}
	}

	// Update display name if changed
	if alemData.DisplayName != "" && alemData.DisplayName != s.DisplayName {
		s.DisplayName = alemData.DisplayName
		updated = true
	}

	// Update online state
	if alemData.IsOnline {
		s.MarkOnline()
	} else {
		s.MarkOffline()
	}

	// Update sync timestamp
	s.SyncedWith(time.Now())

	// Persist changes
	if err := j.studentRepo.Update(ctx, s); err != nil {
		return false, 0, fmt.Errorf("failed to save student: %w", err)
	}

	// Mark as synced
	if err := j.syncRepo.MarkSynced(ctx, s.ID, time.Now()); err != nil {
		j.logger.Warn("failed to mark student as synced",
			"student_id", s.ID,
			"error", err,
		)
	}

	return updated, xpDelta, nil
}

// emitSyncCompletedEvent publishes a sync completed event.
func (j *SyncAllStudentsJob) emitSyncCompletedEvent(stats *SyncStats) {
	// Create a generic event for sync completion
	event := shared.BaseEvent{
		Type:        shared.EventSyncCompleted,
		Timestamp:   time.Now(),
		AggregateId: "system",
	}

	// We need to wrap this in a proper event type
	// For now, we'll just log it
	j.logger.Info("sync completed event",
		"total", stats.TotalStudents,
		"synced", stats.SyncedCount,
		"updated", stats.UpdatedCount,
		"failed", stats.FailedCount,
		"duration", stats.Duration.String(),
	)

	_ = event // Use the event when proper infrastructure is in place
}

// LastSyncStats returns statistics from the last sync run.
func (j *SyncAllStudentsJob) LastSyncStats() *SyncStats {
	stats := j.lastSyncStats.Load()
	if stats == nil {
		return nil
	}
	return stats.(*SyncStats)
}

// ══════════════════════════════════════════════════════════════════════════════
// SYNC SINGLE STUDENT (for on-demand sync)
// ══════════════════════════════════════════════════════════════════════════════

// SyncSingleStudent syncs a single student by ID or login.
// This can be called on-demand, for example when a student uses the bot.
func (j *SyncAllStudentsJob) SyncSingleStudent(ctx context.Context, studentID string) error {
	s, err := j.studentRepo.GetByID(ctx, studentID)
	if err != nil {
		return fmt.Errorf("student not found: %w", err)
	}

	// Fetch fresh data from Alem
    login := ""
    if idx := strings.Index(s.Email, "@"); idx > 0 {
        login = s.Email[:idx]
    }
    
	alemData, err := j.alemClient.GetStudentByLogin(ctx, login)
	if err != nil {
		return fmt.Errorf("failed to fetch from Alem API: %w", err)
	}

	_, _, err = j.syncStudent(ctx, s, alemData)
	return err
}
