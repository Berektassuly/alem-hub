// Package alem implements Alem Platform API client.
// This package handles all communication with the Alem School platform,
// including fetching student data, XP, tasks, and online status.
package alem

import (
	"encoding/json"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// API RESPONSE WRAPPERS
// ══════════════════════════════════════════════════════════════════════════════

// APIResponse represents a generic API response wrapper.
type APIResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Error   string `json:"error,omitempty"`
	Meta    *Meta  `json:"meta,omitempty"`
}

// Meta contains pagination and additional metadata.
type Meta struct {
	Total      int    `json:"total,omitempty"`
	Page       int    `json:"page,omitempty"`
	PerPage    int    `json:"per_page,omitempty"`
	TotalPages int    `json:"total_pages,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT DTOs
// ══════════════════════════════════════════════════════════════════════════════

// StudentDTO represents a student as returned by Alem API.
// This is the external representation that needs to be mapped to our domain model.
type StudentDTO struct {
	// ID is the unique identifier in Alem platform
	ID string `json:"id"`

	// Login is the student's username on the platform
	Login string `json:"login"`

	// FirstName is the student's first name
	FirstName string `json:"first_name,omitempty"`

	// LastName is the student's last name
	LastName string `json:"last_name,omitempty"`

	// Email is the student's email (may not always be available)
	Email string `json:"email,omitempty"`

	// AvatarURL is the URL to the student's avatar
	AvatarURL string `json:"avatar_url,omitempty"`

	// Campus is the campus name (e.g., "Almaty")
	Campus string `json:"campus,omitempty"`

	// Cohort is the student's cohort/batch identifier
	Cohort string `json:"cohort,omitempty"`

	// Pool is the pool/wave the student belongs to
	Pool string `json:"pool,omitempty"`

	// Level is the current level of the student
	Level int `json:"level"`

	// XP represents total experience points
	XP int `json:"xp"`

	// Wallet is the amount of available currency/points
	Wallet int `json:"wallet,omitempty"`

	// IsActive indicates if the student is currently active in the program
	IsActive bool `json:"is_active"`

	// IsOnline indicates if the student is currently online
	IsOnline bool `json:"is_online,omitempty"`

	// LastActivityAt is the timestamp of last activity
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`

	// CreatedAt is when the student account was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the student account was last updated
	UpdatedAt time.Time `json:"updated_at"`

	// Progress contains additional progress information
	Progress *ProgressDTO `json:"progress,omitempty"`

	// Stats contains statistical data
	Stats *StudentStatsDTO `json:"stats,omitempty"`
}

// FullName returns the student's full name.
func (s *StudentDTO) FullName() string {
	if s.FirstName == "" && s.LastName == "" {
		return s.Login
	}
	if s.LastName == "" {
		return s.FirstName
	}
	if s.FirstName == "" {
		return s.LastName
	}
	return s.FirstName + " " + s.LastName
}

// ProgressDTO contains student's progress information.
type ProgressDTO struct {
	// CurrentProject is the project the student is currently working on
	CurrentProject string `json:"current_project,omitempty"`

	// CompletedProjects is the number of completed projects
	CompletedProjects int `json:"completed_projects"`

	// FailedProjects is the number of failed projects
	FailedProjects int `json:"failed_projects"`

	// InProgressProjects is the number of projects in progress
	InProgressProjects int `json:"in_progress_projects"`

	// TotalSkills is the total number of acquired skills
	TotalSkills int `json:"total_skills"`

	// Streak is the current activity streak in days
	Streak int `json:"streak"`

	// LongestStreak is the longest activity streak ever achieved
	LongestStreak int `json:"longest_streak"`
}

// StudentStatsDTO contains statistical data about a student.
type StudentStatsDTO struct {
	// TotalTasks is the total number of tasks attempted
	TotalTasks int `json:"total_tasks"`

	// CompletedTasks is the number of successfully completed tasks
	CompletedTasks int `json:"completed_tasks"`

	// TotalTime is the total time spent in seconds
	TotalTime int64 `json:"total_time"`

	// AverageGrade is the average grade/score
	AverageGrade float64 `json:"average_grade"`

	// Rank is the student's position in the leaderboard
	Rank int `json:"rank,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// TASK DTOs
// ══════════════════════════════════════════════════════════════════════════════

// TaskDTO represents a task/project as returned by Alem API.
type TaskDTO struct {
	// ID is the unique task identifier
	ID string `json:"id"`

	// Slug is the URL-friendly identifier (e.g., "go-intro", "graph-01")
	Slug string `json:"slug"`

	// Name is the human-readable task name
	Name string `json:"name"`

	// Description is the task description
	Description string `json:"description,omitempty"`

	// Type indicates the task type (project, exam, checkpoint, etc.)
	Type string `json:"type"`

	// Difficulty is the difficulty level (1-5 or similar)
	Difficulty int `json:"difficulty"`

	// XPReward is the XP awarded upon completion
	XPReward int `json:"xp_reward"`

	// EstimatedTime is the estimated time to complete in hours
	EstimatedTime int `json:"estimated_time,omitempty"`

	// Skills are the skills this task teaches or requires
	Skills []string `json:"skills,omitempty"`

	// Prerequisites are task IDs that must be completed first
	Prerequisites []string `json:"prerequisites,omitempty"`

	// IsAvailable indicates if the task is currently available
	IsAvailable bool `json:"is_available"`
}

// TaskCompletionDTO represents a student's completion of a task.
type TaskCompletionDTO struct {
	// ID is the unique completion record identifier
	ID string `json:"id"`

	// StudentID is the ID of the student who completed the task
	StudentID string `json:"student_id"`

	// TaskID is the ID of the completed task
	TaskID string `json:"task_id"`

	// TaskSlug is the slug of the completed task
	TaskSlug string `json:"task_slug,omitempty"`

	// Status is the completion status (passed, failed, in_progress)
	Status string `json:"status"`

	// Grade is the grade/score received (0-100 or similar)
	Grade int `json:"grade,omitempty"`

	// XPEarned is the XP earned for this completion
	XPEarned int `json:"xp_earned"`

	// Attempts is the number of attempts made
	Attempts int `json:"attempts"`

	// StartedAt is when the student started the task
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the task was completed
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// TimeSpent is the time spent in seconds
	TimeSpent int64 `json:"time_spent,omitempty"`
}

// IsSuccessful returns true if the task was successfully completed.
func (tc *TaskCompletionDTO) IsSuccessful() bool {
	return tc.Status == "passed" || tc.Status == "completed" || tc.Status == "success"
}

// Duration returns the time spent as a time.Duration.
func (tc *TaskCompletionDTO) Duration() time.Duration {
	return time.Duration(tc.TimeSpent) * time.Second
}

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD DTOs
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardEntryDTO represents an entry in the leaderboard.
type LeaderboardEntryDTO struct {
	// Rank is the position in the leaderboard
	Rank int `json:"rank"`

	// StudentID is the student's ID
	StudentID string `json:"student_id"`

	// Login is the student's username
	Login string `json:"login"`

	// DisplayName is the student's display name
	DisplayName string `json:"display_name,omitempty"`

	// XP is the total experience points
	XP int `json:"xp"`

	// Level is the current level
	Level int `json:"level"`

	// TasksCompleted is the number of completed tasks
	TasksCompleted int `json:"tasks_completed"`

	// IsOnline indicates if the student is currently online
	IsOnline bool `json:"is_online,omitempty"`

	// Change is the rank change since last update (+2, -1, 0)
	Change int `json:"change,omitempty"`
}

// LeaderboardDTO represents the full leaderboard response.
type LeaderboardDTO struct {
	// Entries are the leaderboard entries
	Entries []LeaderboardEntryDTO `json:"entries"`

	// UpdatedAt is when the leaderboard was last calculated
	UpdatedAt time.Time `json:"updated_at"`

	// Cohort is the cohort this leaderboard is for (if filtered)
	Cohort string `json:"cohort,omitempty"`

	// Total is the total number of students
	Total int `json:"total"`
}

// ══════════════════════════════════════════════════════════════════════════════
// ACTIVITY DTOs
// ══════════════════════════════════════════════════════════════════════════════

// ActivityDTO represents a student's activity event.
type ActivityDTO struct {
	// ID is the unique activity identifier
	ID string `json:"id"`

	// StudentID is the student who performed the activity
	StudentID string `json:"student_id"`

	// Type is the activity type (login, task_start, task_complete, etc.)
	Type string `json:"type"`

	// Description is a human-readable description
	Description string `json:"description,omitempty"`

	// Metadata contains additional activity-specific data
	Metadata json.RawMessage `json:"metadata,omitempty"`

	// Timestamp is when the activity occurred
	Timestamp time.Time `json:"timestamp"`

	// XPGained is the XP gained from this activity (if any)
	XPGained int `json:"xp_gained,omitempty"`
}

// OnlineStatusDTO represents a student's online status.
type OnlineStatusDTO struct {
	// StudentID is the student's ID
	StudentID string `json:"student_id"`

	// IsOnline indicates if currently online
	IsOnline bool `json:"is_online"`

	// LastSeenAt is when the student was last seen
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`

	// CurrentTask is the task they're currently working on (if any)
	CurrentTask string `json:"current_task,omitempty"`

	// SessionDuration is how long they've been online this session (in seconds)
	SessionDuration int64 `json:"session_duration,omitempty"`
}

// OnlineStudentsDTO represents the list of online students.
type OnlineStudentsDTO struct {
	// Students is the list of online students
	Students []OnlineStatusDTO `json:"students"`

	// Total is the total count of online students
	Total int `json:"total"`

	// UpdatedAt is when this data was fetched
	UpdatedAt time.Time `json:"updated_at"`
}

// ══════════════════════════════════════════════════════════════════════════════
// AUTHENTICATION DTOs
// ══════════════════════════════════════════════════════════════════════════════

// TokenDTO represents an authentication token.
type TokenDTO struct {
	// AccessToken is the JWT access token
	AccessToken string `json:"access_token"`

	// TokenType is the token type (usually "Bearer")
	TokenType string `json:"token_type"`

	// ExpiresIn is the token validity in seconds
	ExpiresIn int `json:"expires_in"`

	// RefreshToken is the token used to refresh the access token
	RefreshToken string `json:"refresh_token,omitempty"`

	// ExpiresAt is the calculated expiration timestamp
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// IsExpired checks if the token has expired.
func (t *TokenDTO) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	// Add 60 second buffer for safety
	return time.Now().After(t.ExpiresAt.Add(-60 * time.Second))
}

// ══════════════════════════════════════════════════════════════════════════════
// ERROR DTOs
// ══════════════════════════════════════════════════════════════════════════════

// APIErrorDTO represents an error response from the API.
type APIErrorDTO struct {
	// Code is the error code
	Code string `json:"code"`

	// Message is the human-readable error message
	Message string `json:"message"`

	// Details contains additional error details
	Details map[string]interface{} `json:"details,omitempty"`

	// RequestID is the ID of the failed request (for debugging)
	RequestID string `json:"request_id,omitempty"`
}

// Error implements the error interface.
func (e *APIErrorDTO) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

// ══════════════════════════════════════════════════════════════════════════════
// SYNC DTOs (For efficient syncing)
// ══════════════════════════════════════════════════════════════════════════════

// SyncDeltaDTO represents changes since the last sync.
type SyncDeltaDTO struct {
	// Students contains updated student records
	Students []StudentDTO `json:"students"`

	// TaskCompletions contains new task completions
	TaskCompletions []TaskCompletionDTO `json:"task_completions"`

	// OnlineChanges contains online status changes
	OnlineChanges []OnlineStatusDTO `json:"online_changes"`

	// DeletedStudentIDs contains IDs of deleted/deactivated students
	DeletedStudentIDs []string `json:"deleted_student_ids,omitempty"`

	// SyncTimestamp is the timestamp of this sync
	SyncTimestamp time.Time `json:"sync_timestamp"`

	// NextSyncToken is the token to use for the next delta sync
	NextSyncToken string `json:"next_sync_token,omitempty"`

	// FullSyncRequired indicates if a full sync is needed
	FullSyncRequired bool `json:"full_sync_required,omitempty"`
}

// HasChanges returns true if there are any changes in this delta.
func (d *SyncDeltaDTO) HasChanges() bool {
	return len(d.Students) > 0 ||
		len(d.TaskCompletions) > 0 ||
		len(d.OnlineChanges) > 0 ||
		len(d.DeletedStudentIDs) > 0
}

// ══════════════════════════════════════════════════════════════════════════════
// WEBHOOK DTOs (If Alem supports webhooks)
// ══════════════════════════════════════════════════════════════════════════════

// WebhookEventDTO represents a webhook event from Alem.
type WebhookEventDTO struct {
	// EventType is the type of event (student.updated, task.completed, etc.)
	EventType string `json:"event_type"`

	// EventID is the unique event identifier
	EventID string `json:"event_id"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Payload contains the event-specific data
	Payload json.RawMessage `json:"payload"`

	// Signature is the webhook signature for verification
	Signature string `json:"signature,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// REQUEST DTOs
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardRequestDTO represents parameters for fetching leaderboard.
type LeaderboardRequestDTO struct {
	// Cohort filters by cohort (optional)
	Cohort string `json:"cohort,omitempty"`

	// Limit is the maximum number of entries to return
	Limit int `json:"limit,omitempty"`

	// Offset is the number of entries to skip
	Offset int `json:"offset,omitempty"`

	// SortBy is the field to sort by (xp, level, tasks_completed)
	SortBy string `json:"sort_by,omitempty"`

	// Order is the sort order (asc, desc)
	Order string `json:"order,omitempty"`
}

// StudentsRequestDTO represents parameters for fetching students.
type StudentsRequestDTO struct {
	// IDs filters by specific student IDs
	IDs []string `json:"ids,omitempty"`

	// Cohort filters by cohort
	Cohort string `json:"cohort,omitempty"`

	// IsActive filters by active status
	IsActive *bool `json:"is_active,omitempty"`

	// IsOnline filters by online status
	IsOnline *bool `json:"is_online,omitempty"`

	// Search is a search query for login/name
	Search string `json:"search,omitempty"`

	// ModifiedSince filters students modified after this time
	ModifiedSince *time.Time `json:"modified_since,omitempty"`

	// Page is the page number for pagination
	Page int `json:"page,omitempty"`

	// PerPage is the number of items per page
	PerPage int `json:"per_page,omitempty"`
}

// TaskCompletionsRequestDTO represents parameters for fetching task completions.
type TaskCompletionsRequestDTO struct {
	// StudentID filters by student
	StudentID string `json:"student_id,omitempty"`

	// TaskID filters by task
	TaskID string `json:"task_id,omitempty"`

	// Status filters by completion status
	Status string `json:"status,omitempty"`

	// Since filters completions after this time
	Since *time.Time `json:"since,omitempty"`

	// Page is the page number
	Page int `json:"page,omitempty"`

	// PerPage is the number of items per page
	PerPage int `json:"per_page,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// BOOTCAMP DTOs
// ══════════════════════════════════════════════════════════════════════════════

// BootcampDTO represents the root bootcamp data structure.
type BootcampDTO struct {
	ID       string            `json:"id"`
	Status   string            `json:"status"`
	StartAt  time.Time         `json:"start_at"`
	EndAt    time.Time         `json:"end_at"`
	Title    string            `json:"title"`
	Type     string            `json:"type"`
	TotalXP  int               `json:"total_xp"`
	UserXP   int               `json:"user_xp"`
	Children []BootcampNodeDTO `json:"children"`
}

// BootcampNodeDTO represents a node in the bootcamp graph (week, story, etc.).
type BootcampNodeDTO struct {
	ID       string            `json:"id,omitempty"`
	Status   string            `json:"status"`
	StartAt  time.Time         `json:"start_at"`
	EndAt    time.Time         `json:"end_at"`
	Index    int               `json:"index"`
	Title    string            `json:"title"`
	Type     string            `json:"type"`
	TotalXP  int               `json:"total_xp"`
	UserXP   int               `json:"user_xp"`
	Children []BootcampNodeDTO `json:"children,omitempty"`
}
