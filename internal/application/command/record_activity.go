// Package command contains write operations (CQRS - Commands).
package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/domain/activity"
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
)

// ══════════════════════════════════════════════════════════════════════════════
// RECORD ACTIVITY COMMAND
// Records student activities: going online/offline, completing tasks, sessions.
// This is essential for the "who is online now" and "Daily Grind" features.
// ══════════════════════════════════════════════════════════════════════════════

// ActivityType defines the type of activity being recorded.
type ActivityType string

const (
	// ActivityTypeOnline - student went online.
	ActivityTypeOnline ActivityType = "online"

	// ActivityTypeOffline - student went offline.
	ActivityTypeOffline ActivityType = "offline"

	// ActivityTypeHeartbeat - periodic heartbeat to maintain online status.
	ActivityTypeHeartbeat ActivityType = "heartbeat"

	// ActivityTypeTaskCompleted - student completed a task.
	ActivityTypeTaskCompleted ActivityType = "task_completed"

	// ActivityTypeSessionStart - explicit session start.
	ActivityTypeSessionStart ActivityType = "session_start"

	// ActivityTypeSessionEnd - explicit session end.
	ActivityTypeSessionEnd ActivityType = "session_end"
)

// RecordActivityCommand contains the data to record an activity.
type RecordActivityCommand struct {
	// StudentID is the internal ID of the student.
	StudentID string

	// Type is the type of activity.
	Type ActivityType

	// TaskID is the ID of the completed task (for task_completed type).
	TaskID string

	// XPEarned is the XP earned from this activity (for task_completed).
	XPEarned int

	// SessionID is the session identifier (for session operations).
	SessionID string

	// Timestamp is when the activity occurred (defaults to now if zero).
	Timestamp time.Time

	// Metadata contains additional activity-specific data.
	Metadata map[string]interface{}

	// CorrelationID for tracing.
	CorrelationID string
}

// Validate validates the command.
func (c RecordActivityCommand) Validate() error {
	if c.StudentID == "" {
		return errors.New("record_activity: student_id is required")
	}

	switch c.Type {
	case ActivityTypeOnline, ActivityTypeOffline, ActivityTypeHeartbeat,
		ActivityTypeSessionStart, ActivityTypeSessionEnd:
		// Valid types without additional requirements
	case ActivityTypeTaskCompleted:
		if c.TaskID == "" {
			return errors.New("record_activity: task_id is required for task_completed")
		}
	default:
		return fmt.Errorf("record_activity: unknown activity type: %s", c.Type)
	}

	return nil
}

// RecordActivityResult contains the result of recording an activity.
type RecordActivityResult struct {
	// Success indicates if the activity was recorded.
	Success bool

	// StudentID is the internal ID of the student.
	StudentID string

	// ActivityType is the type of activity recorded.
	ActivityType ActivityType

	// IsOnline indicates the current online status.
	IsOnline bool

	// CurrentStreak is the current daily streak.
	CurrentStreak int

	// StreakUpdated indicates if the streak was updated.
	StreakUpdated bool

	// StreakBroken indicates if the streak was broken.
	StreakBroken bool

	// PreviousStreak is the streak before it was broken (if applicable).
	PreviousStreak int

	// SessionID is the ID of the current/new session.
	SessionID string

	// DailyProgress is the updated daily progress.
	DailyProgress *student.DailyGrind

	// Events contains domain events generated.
	Events []shared.Event

	// RecordedAt is when the activity was recorded.
	RecordedAt time.Time
}

// ══════════════════════════════════════════════════════════════════════════════
// HANDLER
// ══════════════════════════════════════════════════════════════════════════════

// RecordActivityHandler handles the RecordActivityCommand.
type RecordActivityHandler struct {
	studentRepo    student.Repository
	progressRepo   student.ProgressRepository
	activityRepo   activity.Repository
	onlineTracker  activity.OnlineTracker
	eventPublisher shared.EventPublisher

	// Configuration
	onlineTTL         time.Duration // How long to consider someone online without heartbeat
	sessionExpiration time.Duration // When to auto-expire sessions
}

// RecordActivityHandlerConfig contains configuration for the handler.
type RecordActivityHandlerConfig struct {
	OnlineTTL         time.Duration
	SessionExpiration time.Duration
}

// DefaultRecordActivityHandlerConfig returns default configuration.
func DefaultRecordActivityHandlerConfig() RecordActivityHandlerConfig {
	return RecordActivityHandlerConfig{
		OnlineTTL:         10 * time.Minute,
		SessionExpiration: 30 * time.Minute,
	}
}

// NewRecordActivityHandler creates a new RecordActivityHandler.
func NewRecordActivityHandler(
	studentRepo student.Repository,
	progressRepo student.ProgressRepository,
	activityRepo activity.Repository,
	onlineTracker activity.OnlineTracker,
	eventPublisher shared.EventPublisher,
	config RecordActivityHandlerConfig,
) *RecordActivityHandler {
	if config.OnlineTTL == 0 {
		config = DefaultRecordActivityHandlerConfig()
	}

	return &RecordActivityHandler{
		studentRepo:       studentRepo,
		progressRepo:      progressRepo,
		activityRepo:      activityRepo,
		onlineTracker:     onlineTracker,
		eventPublisher:    eventPublisher,
		onlineTTL:         config.OnlineTTL,
		sessionExpiration: config.SessionExpiration,
	}
}

// Handle executes the record activity command.
func (h *RecordActivityHandler) Handle(ctx context.Context, cmd RecordActivityCommand) (*RecordActivityResult, error) {
	// Validate command
	if err := cmd.Validate(); err != nil {
		return nil, fmt.Errorf("record_activity: validation failed: %w", err)
	}

	// Set timestamp if not provided
	timestamp := cmd.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	// Get student
	stud, err := h.studentRepo.GetByID(ctx, cmd.StudentID)
	if err != nil {
		return nil, fmt.Errorf("record_activity: failed to get student: %w", err)
	}

	// Initialize result
	result := &RecordActivityResult{
		Success:      true,
		StudentID:    cmd.StudentID,
		ActivityType: cmd.Type,
		RecordedAt:   timestamp,
		Events:       make([]shared.Event, 0),
	}

	// Handle activity by type
	switch cmd.Type {
	case ActivityTypeOnline:
		if err := h.handleGoOnline(ctx, stud, cmd, result); err != nil {
			return nil, err
		}
	case ActivityTypeOffline:
		if err := h.handleGoOffline(ctx, stud, cmd, result); err != nil {
			return nil, err
		}
	case ActivityTypeHeartbeat:
		if err := h.handleHeartbeat(ctx, stud, cmd, result); err != nil {
			return nil, err
		}
	case ActivityTypeTaskCompleted:
		if err := h.handleTaskCompleted(ctx, stud, cmd, result); err != nil {
			return nil, err
		}
	case ActivityTypeSessionStart:
		if err := h.handleSessionStart(ctx, stud, cmd, result); err != nil {
			return nil, err
		}
	case ActivityTypeSessionEnd:
		if err := h.handleSessionEnd(ctx, stud, cmd, result); err != nil {
			return nil, err
		}
	}

	// Update streak
	if err := h.updateStreak(ctx, stud, result, timestamp); err != nil {
		// Log but don't fail
	}

	// Update daily progress
	if err := h.updateDailyProgress(ctx, cmd.StudentID, cmd.Type, cmd.XPEarned, timestamp); err != nil {
		// Log but don't fail
	}

	// Save student changes
	if err := h.studentRepo.Update(ctx, stud); err != nil {
		return nil, fmt.Errorf("record_activity: failed to update student: %w", err)
	}

	// Publish events
	for _, event := range result.Events {
		_ = h.eventPublisher.Publish(event)
	}

	// Update result with current state
	result.IsOnline = stud.OnlineState == student.OnlineStateOnline

	return result, nil
}

// handleGoOnline handles the online activity type.
func (h *RecordActivityHandler) handleGoOnline(
	ctx context.Context,
	stud *student.Student,
	cmd RecordActivityCommand,
	result *RecordActivityResult,
) error {
	// Mark student as online
	stud.MarkOnline()

	// Update online tracker
	if err := h.onlineTracker.MarkOnline(ctx, activity.StudentID(cmd.StudentID), h.onlineTTL); err != nil {
		return fmt.Errorf("failed to mark online: %w", err)
	}

	// Create or get session
	sessionID := cmd.SessionID
	if sessionID == "" {
		sessionID = generateSessionID(cmd.StudentID)
	}

	// Check if there's an existing active session
	existingSession, err := h.activityRepo.GetActiveSession(ctx, activity.StudentID(cmd.StudentID))
	if err == nil && existingSession != nil {
		// Reuse existing session
		sessionID = string(existingSession.ID)
	} else {
		// Create new session
		timestamp := cmd.Timestamp
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}

		newSession, err := activity.NewSession(
			activity.SessionID(sessionID),
			activity.StudentID(cmd.StudentID),
			timestamp,
		)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}

		if err := h.activityRepo.SaveSession(ctx, newSession); err != nil {
			return fmt.Errorf("failed to save session: %w", err)
		}

		// Emit event
		event := shared.NewStudentWentOnlineEvent(cmd.StudentID, sessionID)
		if cmd.CorrelationID != "" {
			event.BaseEvent = event.BaseEvent.WithCorrelationID(cmd.CorrelationID)
		}
		result.Events = append(result.Events, event)
	}

	result.SessionID = sessionID
	result.IsOnline = true

	return nil
}

// handleGoOffline handles the offline activity type.
func (h *RecordActivityHandler) handleGoOffline(
	ctx context.Context,
	stud *student.Student,
	cmd RecordActivityCommand,
	result *RecordActivityResult,
) error {
	// Mark student as offline
	stud.MarkOffline()

	// Update online tracker
	if err := h.onlineTracker.MarkOffline(ctx, activity.StudentID(cmd.StudentID)); err != nil {
		// Log but don't fail
	}

	// End active session
	session, err := h.activityRepo.GetActiveSession(ctx, activity.StudentID(cmd.StudentID))
	if err == nil && session != nil {
		timestamp := cmd.Timestamp
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}

		if err := session.End(timestamp); err == nil {
			_ = h.activityRepo.SaveSession(ctx, session)

			// Emit event
			event := shared.NewStudentWentOfflineEvent(
				cmd.StudentID,
				string(session.ID),
				session.Duration(),
				session.TasksCompletedDuringSession,
				session.XPGainedDuringSession,
			)
			if cmd.CorrelationID != "" {
				event.BaseEvent = event.BaseEvent.WithCorrelationID(cmd.CorrelationID)
			}
			result.Events = append(result.Events, event)
		}

		result.SessionID = string(session.ID)
	}

	result.IsOnline = false

	return nil
}

// handleHeartbeat handles the heartbeat activity type.
func (h *RecordActivityHandler) handleHeartbeat(
	ctx context.Context,
	stud *student.Student,
	cmd RecordActivityCommand,
	result *RecordActivityResult,
) error {
	// Refresh online status
	if err := h.onlineTracker.RefreshOnline(ctx, activity.StudentID(cmd.StudentID), h.onlineTTL); err != nil {
		// If refresh fails, try to mark as online
		_ = h.onlineTracker.MarkOnline(ctx, activity.StudentID(cmd.StudentID), h.onlineTTL)
	}

	// Update last seen
	stud.MarkOnline()

	// Get current session
	session, err := h.activityRepo.GetActiveSession(ctx, activity.StudentID(cmd.StudentID))
	if err == nil && session != nil {
		result.SessionID = string(session.ID)
	}

	result.IsOnline = true

	return nil
}

// handleTaskCompleted handles the task_completed activity type.
func (h *RecordActivityHandler) handleTaskCompleted(
	ctx context.Context,
	stud *student.Student,
	cmd RecordActivityCommand,
	result *RecordActivityResult,
) error {
	timestamp := cmd.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	// Create task completion record
	completionID := generateCompletionID(cmd.StudentID, cmd.TaskID)
	completion, err := activity.NewTaskCompletion(
		completionID,
		activity.StudentID(cmd.StudentID),
		activity.TaskID(cmd.TaskID),
		timestamp,
		cmd.XPEarned,
	)
	if err != nil {
		return fmt.Errorf("failed to create task completion: %w", err)
	}

	// Get time spent from metadata if available
	if timeSpent, ok := cmd.Metadata["time_spent"].(time.Duration); ok {
		_ = completion.SetTimeSpent(timeSpent)
	}

	// Get attempts from metadata if available
	if attempts, ok := cmd.Metadata["attempts"].(int); ok {
		_ = completion.SetAttempts(attempts)
	}

	// Attach to current session if exists
	session, _ := h.activityRepo.GetActiveSession(ctx, activity.StudentID(cmd.StudentID))
	if session != nil {
		_ = completion.AttachToSession(session.ID)
		_ = session.RecordTaskCompletion(cmd.XPEarned)
		_ = h.activityRepo.SaveSession(ctx, session)
		result.SessionID = string(session.ID)
	}

	// Save task completion
	if err := h.activityRepo.SaveTaskCompletion(ctx, completion); err != nil {
		return fmt.Errorf("failed to save task completion: %w", err)
	}

	// Update student's last seen
	stud.MarkOnline()

	// Emit event
	taskEvent := shared.NewTaskCompletedEvent(
		cmd.StudentID,
		cmd.TaskID,
		cmd.XPEarned,
		completion.TimeSpent,
	)
	if cmd.CorrelationID != "" {
		taskEvent.BaseEvent = taskEvent.BaseEvent.WithCorrelationID(cmd.CorrelationID)
	}
	result.Events = append(result.Events, taskEvent)

	return nil
}

// handleSessionStart handles explicit session start.
func (h *RecordActivityHandler) handleSessionStart(
	ctx context.Context,
	stud *student.Student,
	cmd RecordActivityCommand,
	result *RecordActivityResult,
) error {
	// Delegate to handleGoOnline but with specific session ID
	return h.handleGoOnline(ctx, stud, cmd, result)
}

// handleSessionEnd handles explicit session end.
func (h *RecordActivityHandler) handleSessionEnd(
	ctx context.Context,
	stud *student.Student,
	cmd RecordActivityCommand,
	result *RecordActivityResult,
) error {
	// End specific session if provided
	if cmd.SessionID != "" {
		// Find and end the specific session
		session, err := h.activityRepo.GetActiveSession(ctx, activity.StudentID(cmd.StudentID))
		if err == nil && session != nil && string(session.ID) == cmd.SessionID {
			timestamp := cmd.Timestamp
			if timestamp.IsZero() {
				timestamp = time.Now().UTC()
			}
			_ = session.End(timestamp)
			_ = h.activityRepo.SaveSession(ctx, session)
		}
	} else {
		// End any active session
		return h.handleGoOffline(ctx, stud, cmd, result)
	}

	return nil
}

// updateStreak updates the student's daily streak.
func (h *RecordActivityHandler) updateStreak(
	ctx context.Context,
	stud *student.Student,
	result *RecordActivityResult,
	activityTime time.Time,
) error {
	// Get current streak
	streak, err := h.progressRepo.GetStreak(ctx, stud.ID)
	if err != nil {
		// Create new streak if not exists
		streak = student.NewStreak(stud.ID)
	}

	previousStreak := streak.CurrentStreak

	// Check if streak is broken before recording
	wasBroken := streak.IsBroken()

	// Record activity
	streak.RecordActivity(activityTime)

	// Save streak
	if err := h.progressRepo.SaveStreak(ctx, streak); err != nil {
		return fmt.Errorf("failed to save streak: %w", err)
	}

	result.CurrentStreak = streak.CurrentStreak
	result.StreakUpdated = streak.CurrentStreak != previousStreak

	// Check if streak was broken
	if wasBroken && previousStreak > 1 {
		result.StreakBroken = true
		result.PreviousStreak = previousStreak

		// Emit streak broken event
		daysMissed := int(activityTime.Sub(streak.LastActiveDate).Hours() / 24)
		event := shared.NewDailyStreakBrokenEvent(stud.ID, previousStreak, daysMissed)
		result.Events = append(result.Events, event)
	}

	return nil
}

// updateDailyProgress updates the daily grind progress.
func (h *RecordActivityHandler) updateDailyProgress(
	ctx context.Context,
	studentID string,
	activityType ActivityType,
	xpEarned int,
	timestamp time.Time,
) error {
	// Get or create today's daily grind
	grind, err := h.progressRepo.GetTodayDailyGrind(ctx, studentID)
	if err != nil {
		// Get current XP and rank for new daily grind
		stud, err := h.studentRepo.GetByID(ctx, studentID)
		if err != nil {
			return err
		}
		grind = student.NewDailyGrind(studentID, stud.CurrentXP, 0)
	}

	// Update based on activity type
	switch activityType {
	case ActivityTypeTaskCompleted:
		grind.RecordTaskCompletion()
		if xpEarned > 0 {
			grind.RecordXPGain(grind.XPCurrent.Add(student.XP(xpEarned)))
		}
	case ActivityTypeSessionEnd:
		// Session duration would be tracked separately
	}

	// Save updated daily grind
	return h.progressRepo.SaveDailyGrind(ctx, grind)
}

// Helper functions

func generateSessionID(studentID string) string {
	return fmt.Sprintf("session_%s_%d", studentID, time.Now().UnixNano())
}

func generateCompletionID(studentID, taskID string) string {
	return fmt.Sprintf("completion_%s_%s_%d", studentID, taskID, time.Now().UnixNano())
}

// ══════════════════════════════════════════════════════════════════════════════
// BATCH ACTIVITY COMMAND
// For recording multiple activities at once (e.g., from batch sync).
// ══════════════════════════════════════════════════════════════════════════════

// RecordBatchActivityCommand contains multiple activities to record.
type RecordBatchActivityCommand struct {
	Activities    []RecordActivityCommand
	CorrelationID string
}

// RecordBatchActivityResult contains results for batch recording.
type RecordBatchActivityResult struct {
	TotalCount   int
	SuccessCount int
	FailedCount  int
	Results      []*RecordActivityResult
	Errors       map[string]error
}

// RecordBatchActivityHandler handles batch activity recording.
type RecordBatchActivityHandler struct {
	handler *RecordActivityHandler
}

// NewRecordBatchActivityHandler creates a new batch handler.
func NewRecordBatchActivityHandler(handler *RecordActivityHandler) *RecordBatchActivityHandler {
	return &RecordBatchActivityHandler{handler: handler}
}

// Handle executes the batch record activity command.
func (h *RecordBatchActivityHandler) Handle(
	ctx context.Context,
	cmd RecordBatchActivityCommand,
) (*RecordBatchActivityResult, error) {
	result := &RecordBatchActivityResult{
		TotalCount: len(cmd.Activities),
		Results:    make([]*RecordActivityResult, 0, len(cmd.Activities)),
		Errors:     make(map[string]error),
	}

	for i, activity := range cmd.Activities {
		// Set correlation ID if not set
		if activity.CorrelationID == "" {
			activity.CorrelationID = cmd.CorrelationID
		}

		actResult, err := h.handler.Handle(ctx, activity)
		if err != nil {
			result.FailedCount++
			result.Errors[fmt.Sprintf("%d:%s", i, activity.StudentID)] = err
			continue
		}

		result.SuccessCount++
		result.Results = append(result.Results, actResult)
	}

	return result, nil
}
