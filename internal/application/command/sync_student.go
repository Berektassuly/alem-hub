// Package command contains write operations (CQRS - Commands).
// Commands are responsible for changing the state of the system.
// They follow the philosophy "From Competition to Collaboration".
package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
)

// ══════════════════════════════════════════════════════════════════════════════
// SYNC STUDENT COMMAND
// Synchronizes student data with Alem Platform API.
// This is a core command that keeps local data in sync with the source of truth.
// ══════════════════════════════════════════════════════════════════════════════

// SyncStudentCommand contains the data needed to sync a student.
type SyncStudentCommand struct {
	// StudentID is the internal ID of the student to sync.
	// If empty, AlemLogin must be provided.
	StudentID string

	// Email is the student's email.
	// Used when syncing by email instead of internal ID.
	Email string

	// ForceSync bypasses the sync interval check.
	ForceSync bool

	// CorrelationID for tracing across services.
	CorrelationID string
}

// Validate validates the command.
func (c SyncStudentCommand) Validate() error {
	if c.StudentID == "" && c.Email == "" {
		return errors.New("sync_student: either student_id or email must be provided")
	}
	return nil
}

// SyncStudentResult contains the result of synchronization.
type SyncStudentResult struct {
	// StudentID is the internal ID of the synced student.
	StudentID string

	// WasUpdated indicates if any data was changed.
	WasUpdated bool

	// XPDelta is the XP change (can be negative for corrections).
	XPDelta int

	// OldXP is the XP before sync.
	OldXP int

	// NewXP is the XP after sync.
	NewXP int

	// OldRank is the rank before sync (0 if unknown).
	OldRank int

	// NewRank is the rank after sync (0 if unknown).
	NewRank int

	// RankChanged indicates if the rank position changed.
	RankChanged bool

	// TasksCompleted contains IDs of newly completed tasks.
	TasksCompleted []string

	// SyncedAt is when the sync was performed.
	SyncedAt time.Time

	// Events contains domain events generated during sync.
	Events []shared.Event
}

// ══════════════════════════════════════════════════════════════════════════════
// DEPENDENCIES (Interfaces)
// ══════════════════════════════════════════════════════════════════════════════

// AlemStudentData represents data fetched from Alem Platform.
type AlemStudentData struct {
	Login           string
	DisplayName     string
	XP              int
	Level           int
	Cohort          string
	CompletedTasks  []string
	LastActivityAt  time.Time
	IsOnline        bool
	ProfileImageURL string
}

// AlemAPIClient defines the interface for Alem Platform API.
type AlemAPIClient interface {
	// GetStudentByLogin fetches student data by login.
	GetStudentByLogin(ctx context.Context, login string) (*AlemStudentData, error)

	// GetAllStudents fetches all students (for bulk sync).
	GetAllStudents(ctx context.Context) ([]AlemStudentData, error)

	// GetStudentTasks fetches completed tasks for a student.
	GetStudentTasks(ctx context.Context, login string) ([]string, error)
}

// LeaderboardService provides rank information.
type LeaderboardService interface {
	// GetStudentRank returns the current rank of a student.
	GetStudentRank(ctx context.Context, studentID string) (int, error)

	// InvalidateCache invalidates the leaderboard cache after updates.
	InvalidateCache(ctx context.Context) error
}

// ══════════════════════════════════════════════════════════════════════════════
// HANDLER
// ══════════════════════════════════════════════════════════════════════════════

// SyncStudentHandler handles the SyncStudentCommand.
type SyncStudentHandler struct {
	studentRepo        student.Repository
	progressRepo       student.ProgressRepository
	alemClient         AlemAPIClient
	leaderboardService LeaderboardService
	eventPublisher     shared.EventPublisher

	// Configuration
	minSyncInterval time.Duration // Minimum interval between syncs
}

// SyncStudentHandlerConfig contains configuration for the handler.
type SyncStudentHandlerConfig struct {
	MinSyncInterval time.Duration
}

// DefaultSyncStudentHandlerConfig returns default configuration.
func DefaultSyncStudentHandlerConfig() SyncStudentHandlerConfig {
	return SyncStudentHandlerConfig{
		MinSyncInterval: 5 * time.Minute,
	}
}

// NewSyncStudentHandler creates a new SyncStudentHandler.
func NewSyncStudentHandler(
	studentRepo student.Repository,
	progressRepo student.ProgressRepository,
	alemClient AlemAPIClient,
	leaderboardService LeaderboardService,
	eventPublisher shared.EventPublisher,
	config SyncStudentHandlerConfig,
) *SyncStudentHandler {
	if config.MinSyncInterval == 0 {
		config = DefaultSyncStudentHandlerConfig()
	}

	return &SyncStudentHandler{
		studentRepo:        studentRepo,
		progressRepo:       progressRepo,
		alemClient:         alemClient,
		leaderboardService: leaderboardService,
		eventPublisher:     eventPublisher,
		minSyncInterval:    config.MinSyncInterval,
	}
}

// Handle executes the sync student command.
func (h *SyncStudentHandler) Handle(ctx context.Context, cmd SyncStudentCommand) (*SyncStudentResult, error) {
	// Validate command
	if err := cmd.Validate(); err != nil {
		return nil, fmt.Errorf("sync_student: validation failed: %w", err)
	}

	// Find the student
	existingStudent, err := h.findStudent(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("sync_student: failed to find student: %w", err)
	}

	// Check sync interval (unless forced)
	if !cmd.ForceSync && !h.shouldSync(existingStudent) {
		return &SyncStudentResult{
			StudentID:  existingStudent.ID,
			WasUpdated: false,
			OldXP:      int(existingStudent.CurrentXP),
			NewXP:      int(existingStudent.CurrentXP),
			SyncedAt:   existingStudent.LastSyncedAt,
			Events:     nil,
		}, nil
	}

	// Fetch data from Alem API
	// Derive login from email
	parts := strings.Split(existingStudent.Email, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("sync_student: invalid email format: %s", existingStudent.Email)
	}
	login := parts[0]

	alemData, err := h.alemClient.GetStudentByLogin(ctx, login)
	if err != nil {
		return nil, fmt.Errorf("sync_student: failed to fetch from Alem API: %w", err)
	}

	// Get current rank before sync
	oldRank, _ := h.leaderboardService.GetStudentRank(ctx, existingStudent.ID)

	// Perform synchronization
	result, err := h.syncStudentData(ctx, existingStudent, alemData, oldRank, cmd.CorrelationID)
	if err != nil {
		return nil, fmt.Errorf("sync_student: failed to sync data: %w", err)
	}

	// Publish domain events
	for _, event := range result.Events {
		if err := h.eventPublisher.Publish(event); err != nil {
			// Log error but don't fail the sync
			// Events can be retried via outbox pattern
			continue
		}
	}

	// Invalidate leaderboard cache if XP changed
	if result.WasUpdated && result.XPDelta != 0 {
		_ = h.leaderboardService.InvalidateCache(ctx)
	}

	return result, nil
}

// findStudent finds the student by ID or login.
func (h *SyncStudentHandler) findStudent(ctx context.Context, cmd SyncStudentCommand) (*student.Student, error) {
	if cmd.StudentID != "" {
		return h.studentRepo.GetByID(ctx, cmd.StudentID)
	}
	return h.studentRepo.GetByEmail(ctx, cmd.Email)
}

// shouldSync determines if a sync should be performed based on the interval.
func (h *SyncStudentHandler) shouldSync(s *student.Student) bool {
	if s.LastSyncedAt.IsZero() {
		return true
	}
	return time.Since(s.LastSyncedAt) >= h.minSyncInterval
}

// syncStudentData performs the actual synchronization.
func (h *SyncStudentHandler) syncStudentData(
	ctx context.Context,
	existingStudent *student.Student,
	alemData *AlemStudentData,
	oldRank int,
	correlationID string,
) (*SyncStudentResult, error) {
	result := &SyncStudentResult{
		StudentID:  existingStudent.ID,
		WasUpdated: false,
		OldXP:      int(existingStudent.CurrentXP),
		NewXP:      int(existingStudent.CurrentXP),
		OldRank:    oldRank,
		SyncedAt:   time.Now().UTC(),
		Events:     make([]shared.Event, 0),
	}

	// Track changes
	hasChanges := false

	// Sync XP
	newXP := student.XP(alemData.XP)
	if newXP != existingStudent.CurrentXP {
		delta, err := existingStudent.UpdateXP(newXP)
		if err != nil {
			return nil, fmt.Errorf("failed to update XP: %w", err)
		}

		result.XPDelta = int(delta)
		result.NewXP = int(newXP)
		hasChanges = true

		// Save XP history
		xpEntry := student.XPHistoryEntry{
			Timestamp: result.SyncedAt,
			OldXP:     student.XP(result.OldXP),
			NewXP:     newXP,
			Delta:     delta,
			Reason:    "sync",
		}
		if err := h.progressRepo.SaveXPChange(ctx, xpEntry); err != nil {
			// Log but don't fail
		}

		// Emit XPGained event if positive
		if delta > 0 {
			event := shared.NewXPGainedEvent(
				existingStudent.ID,
				int(delta),
				int(newXP),
				"sync",
				"",
			)
			if correlationID != "" {
				event.BaseEvent = event.BaseEvent.WithCorrelationID(correlationID)
			}
			result.Events = append(result.Events, event)
		}
	}

	// Sync display name if changed
	if alemData.DisplayName != "" && alemData.DisplayName != existingStudent.DisplayName {
		existingStudent.DisplayName = alemData.DisplayName
		hasChanges = true
	}

	// Sync online state
	if alemData.IsOnline {
		existingStudent.MarkOnline()
	} else {
		existingStudent.MarkOffline()
	}

	// Update sync timestamp
	existingStudent.SyncedWith(result.SyncedAt)

	// Persist changes
	if hasChanges {
		if err := h.studentRepo.Update(ctx, existingStudent); err != nil {
			return nil, fmt.Errorf("failed to save student: %w", err)
		}
		result.WasUpdated = true

		// Check for rank changes after update
		newRank, err := h.leaderboardService.GetStudentRank(ctx, existingStudent.ID)
		if err == nil && newRank > 0 {
			result.NewRank = newRank
			if oldRank > 0 && newRank != oldRank {
				result.RankChanged = true

				// Emit RankChanged event
				cohort := string(existingStudent.Cohort)
				rankEvent := shared.NewRankChangedEvent(
					existingStudent.ID,
					oldRank,
					newRank,
					cohort,
				)
				if correlationID != "" {
					rankEvent.BaseEvent = rankEvent.BaseEvent.WithCorrelationID(correlationID)
				}
				result.Events = append(result.Events, rankEvent)

				// Check for top N entry
				if newRank <= 50 && oldRank > 50 {
					topEvent := shared.NewEnteredTopNEvent(existingStudent.ID, 50, newRank, cohort)
					result.Events = append(result.Events, topEvent)
				}
				if newRank <= 10 && oldRank > 10 {
					topEvent := shared.NewEnteredTopNEvent(existingStudent.ID, 10, newRank, cohort)
					result.Events = append(result.Events, topEvent)
				}
			}
		}
	}

	// Sync completed tasks (separate process to detect new completions)
	if len(alemData.CompletedTasks) > 0 {
		newTasks := h.detectNewTasks(ctx, existingStudent.ID, alemData.CompletedTasks)
		result.TasksCompleted = newTasks

		// Emit TaskCompleted events
		for _, taskID := range newTasks {
			taskEvent := shared.NewTaskCompletedEvent(
				existingStudent.ID,
				taskID,
				0, // XP earned per task unknown from sync
				0, // Time spent unknown from sync
			)
			result.Events = append(result.Events, taskEvent)
		}
	}

	return result, nil
}

// detectNewTasks compares current tasks with known completions.
func (h *SyncStudentHandler) detectNewTasks(
	ctx context.Context,
	studentID string,
	currentTasks []string,
) []string {
	// This would query the progress repository for known completions
	// and return only tasks that are new
	// For now, return empty as this requires task completion tracking
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// BULK SYNC COMMAND
// ══════════════════════════════════════════════════════════════════════════════

// SyncAllStudentsCommand triggers synchronization of all students.
type SyncAllStudentsCommand struct {
	// ForceSync bypasses the sync interval check for all students.
	ForceSync bool

	// Concurrency controls how many students to sync in parallel.
	Concurrency int

	// CorrelationID for tracing.
	CorrelationID string
}

// SyncAllStudentsResult contains the result of bulk synchronization.
type SyncAllStudentsResult struct {
	// TotalStudents is the count of students processed.
	TotalStudents int

	// UpdatedCount is the count of students that had changes.
	UpdatedCount int

	// FailedCount is the count of students that failed to sync.
	FailedCount int

	// Errors contains sync errors by student ID.
	Errors map[string]error

	// Duration is the total sync duration.
	Duration time.Duration

	// StartedAt is when the sync started.
	StartedAt time.Time

	// CompletedAt is when the sync completed.
	CompletedAt time.Time
}

// SyncAllStudentsHandler handles bulk synchronization.
type SyncAllStudentsHandler struct {
	studentRepo    student.Repository
	syncHandler    *SyncStudentHandler
	eventPublisher shared.EventPublisher
}

// NewSyncAllStudentsHandler creates a new bulk sync handler.
func NewSyncAllStudentsHandler(
	studentRepo student.Repository,
	syncHandler *SyncStudentHandler,
	eventPublisher shared.EventPublisher,
) *SyncAllStudentsHandler {
	return &SyncAllStudentsHandler{
		studentRepo:    studentRepo,
		syncHandler:    syncHandler,
		eventPublisher: eventPublisher,
	}
}

// Handle executes the bulk sync command.
func (h *SyncAllStudentsHandler) Handle(ctx context.Context, cmd SyncAllStudentsCommand) (*SyncAllStudentsResult, error) {
	startedAt := time.Now()
	result := &SyncAllStudentsResult{
		Errors:    make(map[string]error),
		StartedAt: startedAt,
	}

	// Get all active students
	opts := student.DefaultListOptions().WithInactive()
	students, err := h.studentRepo.GetAll(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("sync_all: failed to get students: %w", err)
	}

	result.TotalStudents = len(students)

	// Set default concurrency
	concurrency := cmd.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}

	// Create semaphore for concurrency control
	sem := make(chan struct{}, concurrency)

	// Results channel
	type syncResultItem struct {
		studentID string
		updated   bool
		err       error
	}
	results := make(chan syncResultItem, len(students))

	// Sync each student
	for _, s := range students {
		sem <- struct{}{} // Acquire semaphore

		go func(st *student.Student) {
			defer func() { <-sem }() // Release semaphore

			syncCmd := SyncStudentCommand{
				StudentID:     st.ID,
				ForceSync:     cmd.ForceSync,
				CorrelationID: cmd.CorrelationID,
			}

			syncRes, syncErr := h.syncHandler.Handle(ctx, syncCmd)
			if syncErr != nil {
				results <- syncResultItem{st.ID, false, syncErr}
				return
			}

			results <- syncResultItem{st.ID, syncRes.WasUpdated, nil}
		}(s)
	}

	// Collect results
	for i := 0; i < len(students); i++ {
		r := <-results
		if r.err != nil {
			result.FailedCount++
			result.Errors[r.studentID] = r.err
		} else if r.updated {
			result.UpdatedCount++
		}
	}

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(startedAt)

	return result, nil
}
