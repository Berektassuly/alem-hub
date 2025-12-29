// Package activity contains domain entities and business logic
// for tracking student activities, sessions, and task completions.
package activity

import (
	"context"
	"time"
)

// Repository defines the interface for activity data persistence.
// This interface is implemented by the infrastructure layer.
// The domain layer has no knowledge of the actual storage mechanism.
type Repository interface {
	// Session operations

	// SaveSession persists a session (create or update).
	SaveSession(ctx context.Context, session *Session) error

	// GetActiveSession returns the current active session for a student, if any.
	GetActiveSession(ctx context.Context, studentID StudentID) (*Session, error)

	// GetSessionsByStudent returns all sessions for a student within a time range.
	GetSessionsByStudent(ctx context.Context, studentID StudentID, from, to time.Time) ([]*Session, error)

	// EndExpiredSessions marks sessions as expired if they've been inactive too long.
	// Returns the number of sessions expired.
	EndExpiredSessions(ctx context.Context, inactiveThreshold time.Duration) (int, error)

	// Task completion operations

	// SaveTaskCompletion persists a task completion record.
	SaveTaskCompletion(ctx context.Context, completion *TaskCompletion) error

	// GetTaskCompletion returns a specific task completion by ID.
	GetTaskCompletion(ctx context.Context, id string) (*TaskCompletion, error)

	// GetTaskCompletionsByStudent returns all task completions for a student.
	GetTaskCompletionsByStudent(ctx context.Context, studentID StudentID, limit int) ([]*TaskCompletion, error)

	// GetStudentsWhoCompletedTask returns student IDs who completed a specific task.
	// This is the core query for the "find helper" feature.
	// Results are ordered by completion time (most recent first).
	GetStudentsWhoCompletedTask(ctx context.Context, taskID TaskID, limit int) ([]StudentID, error)

	// HasStudentCompletedTask checks if a student has completed a specific task.
	HasStudentCompletedTask(ctx context.Context, studentID StudentID, taskID TaskID) (bool, error)

	// Activity aggregate operations

	// SaveActivity persists the activity aggregate.
	SaveActivity(ctx context.Context, activity *Activity) error

	// GetActivity returns the activity aggregate for a student.
	GetActivity(ctx context.Context, studentID StudentID) (*Activity, error)

	// GetActivitiesByStudents returns activities for multiple students.
	// Useful for batch operations and leaderboard building.
	GetActivitiesByStudents(ctx context.Context, studentIDs []StudentID) ([]*Activity, error)

	// Online tracking operations

	// GetOnlineStudents returns all currently online students.
	GetOnlineStudents(ctx context.Context) ([]OnlineStatus, error)

	// GetRecentlyActiveStudents returns students active within the given duration.
	// Ordered by last activity time (most recent first).
	GetRecentlyActiveStudents(ctx context.Context, within time.Duration, limit int) ([]OnlineStatus, error)

	// UpdateOnlineStatus updates the online status for a student.
	UpdateOnlineStatus(ctx context.Context, status OnlineStatus) error

	// Daily progress operations

	// GetDailyProgress returns the progress for a student on a specific day.
	GetDailyProgress(ctx context.Context, studentID StudentID, date time.Time) (*DailyProgress, error)

	// SaveDailyProgress persists daily progress.
	SaveDailyProgress(ctx context.Context, progress *DailyProgress) error

	// GetDailyProgressRange returns daily progress for a date range.
	GetDailyProgressRange(ctx context.Context, studentID StudentID, from, to time.Time) ([]*DailyProgress, error)

	// Analytics and reporting operations

	// GetInactiveStudents returns students who haven't been active for the given duration.
	// Used for sending encouragement notifications.
	GetInactiveStudents(ctx context.Context, inactiveDuration time.Duration) ([]StudentID, error)

	// GetTopHelpers returns students who have helped others the most.
	// Used for recognizing helpful community members.
	GetTopHelpers(ctx context.Context, limit int) ([]StudentID, error)

	// GetStudentStreak returns the current and longest streak for a student.
	GetStudentStreak(ctx context.Context, studentID StudentID) (current int, longest int, err error)
}

// OnlineTracker defines a specialized interface for real-time online tracking.
// This is typically implemented using Redis or an in-memory cache for low latency.
type OnlineTracker interface {
	// MarkOnline marks a student as online.
	// The entry should automatically expire after the given TTL if not refreshed.
	MarkOnline(ctx context.Context, studentID StudentID, ttl time.Duration) error

	// MarkOffline explicitly marks a student as offline.
	MarkOffline(ctx context.Context, studentID StudentID) error

	// RefreshOnline refreshes the TTL for an online student.
	// Called periodically to keep the online status alive.
	RefreshOnline(ctx context.Context, studentID StudentID, ttl time.Duration) error

	// IsOnline checks if a student is currently online.
	IsOnline(ctx context.Context, studentID StudentID) (bool, error)

	// GetAllOnline returns all currently online student IDs.
	GetAllOnline(ctx context.Context) ([]StudentID, error)

	// GetOnlineCount returns the count of online students.
	GetOnlineCount(ctx context.Context) (int, error)
}

// TaskIndex defines an interface for efficient task-to-student lookups.
// This supports the "who solved this task?" feature.
type TaskIndex interface {
	// IndexTaskCompletion adds a student to the index for a task.
	IndexTaskCompletion(ctx context.Context, taskID TaskID, studentID StudentID, completedAt time.Time) error

	// GetSolvers returns students who solved a task, ordered by completion time.
	GetSolvers(ctx context.Context, taskID TaskID, limit int) ([]StudentID, error)

	// GetSolversOnline returns only online students who solved a task.
	GetSolversOnline(ctx context.Context, taskID TaskID, onlineTracker OnlineTracker) ([]StudentID, error)

	// GetTasksSolvedBy returns all tasks solved by a student.
	GetTasksSolvedBy(ctx context.Context, studentID StudentID) ([]TaskID, error)

	// GetRecentSolvers returns students who recently solved any task.
	// Useful for showing "who's actively working" in the UI.
	GetRecentSolvers(ctx context.Context, within time.Duration, limit int) ([]StudentID, error)
}

// HelperFinder combines multiple sources to find the best helpers for a task.
// This encapsulates the core "find help" business logic.
type HelperFinder interface {
	// FindHelpers returns potential helpers for a given task, ranked by:
	// 1. Has completed the task
	// 2. Currently online (or recently active)
	// 3. Has high helper rating
	// 4. Has helped this student before (existing connection)
	FindHelpers(ctx context.Context, forStudent StudentID, taskID TaskID, limit int) ([]HelperSuggestion, error)
}

// HelperSuggestion represents a potential helper with ranking metadata.
type HelperSuggestion struct {
	StudentID        StudentID
	IsOnline         bool
	LastSeenAt       time.Time
	CompletedTaskAt  time.Time
	HelperRating     float64 // Average rating as a helper (1-5)
	TimesHelpedOther int     // Total times this student helped others
	HasPriorContact  bool    // Has helped the requesting student before
}
