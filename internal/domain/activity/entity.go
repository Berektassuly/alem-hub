// Package activity contains domain entities and business logic
// for tracking student activities, sessions, and task completions.
// This is a pure domain layer with zero external dependencies.
package activity

import (
	"errors"
	"time"
)

// Domain errors for activity package.
var (
	ErrInvalidStudentID    = errors.New("activity: invalid student ID")
	ErrInvalidTaskID       = errors.New("activity: invalid task ID")
	ErrSessionAlreadyEnded = errors.New("activity: session already ended")
	ErrSessionNotStarted   = errors.New("activity: session not started")
	ErrNegativeDuration    = errors.New("activity: duration cannot be negative")
	ErrFutureTimestamp     = errors.New("activity: timestamp cannot be in the future")
	ErrInvalidXP           = errors.New("activity: XP must be non-negative")
)

// StudentID represents a unique identifier for a student.
type StudentID string

// IsValid checks if the student ID is valid.
func (s StudentID) IsValid() bool {
	return s != ""
}

// String returns the string representation of StudentID.
func (s StudentID) String() string {
	return string(s)
}

// TaskID represents a unique identifier for a task (e.g., "graph-01", "go-intro-03").
type TaskID string

// IsValid checks if the task ID is valid.
func (t TaskID) IsValid() bool {
	return t != ""
}

// String returns the string representation of TaskID.
func (t TaskID) String() string {
	return string(t)
}

// SessionID represents a unique identifier for a session.
type SessionID string

// IsValid checks if the session ID is valid.
func (s SessionID) IsValid() bool {
	return s != ""
}

// String returns the string representation of SessionID.
func (s SessionID) String() string {
	return string(s)
}

// SessionStatus represents the current state of a session.
type SessionStatus string

const (
	SessionStatusActive  SessionStatus = "active"
	SessionStatusEnded   SessionStatus = "ended"
	SessionStatusExpired SessionStatus = "expired" // Auto-expired due to inactivity
)

// Session represents a working session of a student.
// A session starts when a student goes online and ends when they go offline.
// Sessions are used to track "who is online now" for the help-finding feature.
type Session struct {
	ID        SessionID
	StudentID StudentID
	StartedAt time.Time
	EndedAt   *time.Time // nil if session is still active
	Status    SessionStatus

	// Metadata for analytics
	TasksCompletedDuringSession int
	XPGainedDuringSession       int
}

// NewSession creates a new active session for a student.
func NewSession(id SessionID, studentID StudentID, startedAt time.Time) (*Session, error) {
	if !id.IsValid() {
		return nil, errors.New("activity: invalid session ID")
	}
	if !studentID.IsValid() {
		return nil, ErrInvalidStudentID
	}
	if startedAt.After(time.Now().Add(time.Minute)) { // Allow 1 minute tolerance
		return nil, ErrFutureTimestamp
	}

	return &Session{
		ID:        id,
		StudentID: studentID,
		StartedAt: startedAt,
		Status:    SessionStatusActive,
	}, nil
}

// End marks the session as ended.
func (s *Session) End(endedAt time.Time) error {
	if s.Status != SessionStatusActive {
		return ErrSessionAlreadyEnded
	}
	if endedAt.Before(s.StartedAt) {
		return errors.New("activity: end time cannot be before start time")
	}

	s.EndedAt = &endedAt
	s.Status = SessionStatusEnded
	return nil
}

// Expire marks the session as expired due to inactivity.
func (s *Session) Expire(expiredAt time.Time) error {
	if s.Status != SessionStatusActive {
		return ErrSessionAlreadyEnded
	}

	s.EndedAt = &expiredAt
	s.Status = SessionStatusExpired
	return nil
}

// Duration returns the duration of the session.
// For active sessions, it returns duration until now.
func (s *Session) Duration() time.Duration {
	if s.EndedAt != nil {
		return s.EndedAt.Sub(s.StartedAt)
	}
	return time.Since(s.StartedAt)
}

// IsActive returns true if the session is currently active.
func (s *Session) IsActive() bool {
	return s.Status == SessionStatusActive
}

// RecordTaskCompletion increments the tasks completed counter for this session.
func (s *Session) RecordTaskCompletion(xpGained int) error {
	if s.Status != SessionStatusActive {
		return ErrSessionAlreadyEnded
	}
	if xpGained < 0 {
		return ErrInvalidXP
	}

	s.TasksCompletedDuringSession++
	s.XPGainedDuringSession += xpGained
	return nil
}

// TaskCompletion represents the completion of a specific task by a student.
// This is crucial for the "find helper" feature - we need to know who solved which task.
type TaskCompletion struct {
	ID          string
	StudentID   StudentID
	TaskID      TaskID
	CompletedAt time.Time
	XPEarned    int

	// Time spent on this specific task (if trackable)
	TimeSpent time.Duration

	// Number of attempts before completion (for difficulty analysis)
	Attempts int

	// Was help received from another student?
	ReceivedHelpFrom *StudentID

	// Session during which the task was completed (optional)
	SessionID *SessionID
}

// NewTaskCompletion creates a new task completion record.
func NewTaskCompletion(
	id string,
	studentID StudentID,
	taskID TaskID,
	completedAt time.Time,
	xpEarned int,
) (*TaskCompletion, error) {
	if id == "" {
		return nil, errors.New("activity: invalid completion ID")
	}
	if !studentID.IsValid() {
		return nil, ErrInvalidStudentID
	}
	if !taskID.IsValid() {
		return nil, ErrInvalidTaskID
	}
	if xpEarned < 0 {
		return nil, ErrInvalidXP
	}
	if completedAt.After(time.Now().Add(time.Minute)) {
		return nil, ErrFutureTimestamp
	}

	return &TaskCompletion{
		ID:          id,
		StudentID:   studentID,
		TaskID:      taskID,
		CompletedAt: completedAt,
		XPEarned:    xpEarned,
		Attempts:    1, // Default to 1 attempt
	}, nil
}

// SetTimeSpent sets the time spent on the task.
func (tc *TaskCompletion) SetTimeSpent(duration time.Duration) error {
	if duration < 0 {
		return ErrNegativeDuration
	}
	tc.TimeSpent = duration
	return nil
}

// SetAttempts sets the number of attempts.
func (tc *TaskCompletion) SetAttempts(attempts int) error {
	if attempts < 1 {
		return errors.New("activity: attempts must be at least 1")
	}
	tc.Attempts = attempts
	return nil
}

// MarkHelpReceived records that help was received from another student.
// This is important for building the social graph and endorsement system.
func (tc *TaskCompletion) MarkHelpReceived(helperID StudentID) error {
	if !helperID.IsValid() {
		return ErrInvalidStudentID
	}
	tc.ReceivedHelpFrom = &helperID
	return nil
}

// AttachToSession links this completion to a session.
func (tc *TaskCompletion) AttachToSession(sessionID SessionID) error {
	if !sessionID.IsValid() {
		return errors.New("activity: invalid session ID")
	}
	tc.SessionID = &sessionID
	return nil
}

// Activity represents the aggregated activity data for a student.
// This is the main aggregate root for the activity domain.
type Activity struct {
	StudentID StudentID

	// Current state
	IsOnline       bool
	LastSeenAt     time.Time
	CurrentSession *Session

	// Aggregated statistics
	TotalTasksCompleted int
	TotalXPEarned       int
	TotalSessionTime    time.Duration
	TotalSessions       int

	// Streak tracking (for Daily Grind feature)
	CurrentStreak int       // Days in a row with activity
	LongestStreak int       // Best streak ever
	LastActiveDay time.Time // Last day with recorded activity (date only, no time)

	// Recent activity for quick access
	RecentTasks []TaskCompletion // Last N completed tasks

	// Helping statistics (for the "good helper" rating)
	TimesHelpedOthers int
	TimesReceivedHelp int
}

// NewActivity creates a new activity record for a student.
func NewActivity(studentID StudentID) (*Activity, error) {
	if !studentID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	return &Activity{
		StudentID:   studentID,
		IsOnline:    false,
		LastSeenAt:  time.Now(),
		RecentTasks: make([]TaskCompletion, 0),
	}, nil
}

// GoOnline marks the student as online and starts a new session.
func (a *Activity) GoOnline(session *Session) error {
	if session == nil {
		return ErrSessionNotStarted
	}
	if session.StudentID != a.StudentID {
		return errors.New("activity: session belongs to different student")
	}

	a.IsOnline = true
	a.CurrentSession = session
	a.LastSeenAt = session.StartedAt
	return nil
}

// GoOffline marks the student as offline and ends the current session.
func (a *Activity) GoOffline(endedAt time.Time) error {
	if !a.IsOnline || a.CurrentSession == nil {
		return ErrSessionNotStarted
	}

	if err := a.CurrentSession.End(endedAt); err != nil {
		return err
	}

	a.TotalSessionTime += a.CurrentSession.Duration()
	a.TotalSessions++
	a.IsOnline = false
	a.LastSeenAt = endedAt
	a.CurrentSession = nil

	return nil
}

// RecordTaskCompletion records a new task completion for this student.
func (a *Activity) RecordTaskCompletion(completion *TaskCompletion) error {
	if completion == nil {
		return errors.New("activity: completion cannot be nil")
	}
	if completion.StudentID != a.StudentID {
		return errors.New("activity: completion belongs to different student")
	}

	a.TotalTasksCompleted++
	a.TotalXPEarned += completion.XPEarned
	a.LastSeenAt = completion.CompletedAt

	// Update streak
	a.updateStreak(completion.CompletedAt)

	// Track if help was received
	if completion.ReceivedHelpFrom != nil {
		a.TimesReceivedHelp++
	}

	// Update current session if active
	if a.CurrentSession != nil && a.CurrentSession.IsActive() {
		_ = a.CurrentSession.RecordTaskCompletion(completion.XPEarned)
	}

	// Add to recent tasks (keep last 10)
	a.RecentTasks = append([]TaskCompletion{*completion}, a.RecentTasks...)
	if len(a.RecentTasks) > 10 {
		a.RecentTasks = a.RecentTasks[:10]
	}

	return nil
}

// updateStreak updates the daily streak based on the activity timestamp.
func (a *Activity) updateStreak(activityTime time.Time) {
	activityDay := truncateToDay(activityTime)
	lastDay := truncateToDay(a.LastActiveDay)

	if a.LastActiveDay.IsZero() {
		// First activity ever
		a.CurrentStreak = 1
		a.LongestStreak = 1
		a.LastActiveDay = activityDay
		return
	}

	daysDiff := int(activityDay.Sub(lastDay).Hours() / 24)

	switch {
	case daysDiff == 0:
		// Same day, streak unchanged
	case daysDiff == 1:
		// Consecutive day, increment streak
		a.CurrentStreak++
		if a.CurrentStreak > a.LongestStreak {
			a.LongestStreak = a.CurrentStreak
		}
		a.LastActiveDay = activityDay
	default:
		// Streak broken (more than 1 day gap)
		a.CurrentStreak = 1
		a.LastActiveDay = activityDay
	}
}

// RecordHelpGiven records that this student helped another student.
func (a *Activity) RecordHelpGiven() {
	a.TimesHelpedOthers++
}

// IsRecentlyActive returns true if the student was active within the given duration.
func (a *Activity) IsRecentlyActive(within time.Duration) bool {
	return time.Since(a.LastSeenAt) <= within
}

// HasCompletedTask checks if the student has completed a specific task.
// This is used for the "find helper" feature.
func (a *Activity) HasCompletedTask(taskID TaskID) bool {
	for _, task := range a.RecentTasks {
		if task.TaskID == taskID {
			return true
		}
	}
	return false
}

// DaysSinceLastActivity returns the number of days since last activity.
// Used for detecting inactive students who need encouragement.
func (a *Activity) DaysSinceLastActivity() int {
	if a.LastSeenAt.IsZero() {
		return -1 // Never active
	}
	return int(time.Since(a.LastSeenAt).Hours() / 24)
}

// truncateToDay returns the date portion of a time (midnight UTC).
func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// OnlineStatus represents the online status with additional context.
type OnlineStatus struct {
	StudentID      StudentID
	IsOnline       bool
	LastSeenAt     time.Time
	SessionStarted *time.Time // When current session started (if online)
}

// NewOnlineStatus creates an online status snapshot.
func NewOnlineStatus(studentID StudentID, isOnline bool, lastSeenAt time.Time) OnlineStatus {
	return OnlineStatus{
		StudentID:  studentID,
		IsOnline:   isOnline,
		LastSeenAt: lastSeenAt,
	}
}

// TimeSinceLastSeen returns human-readable time since last seen.
func (s OnlineStatus) TimeSinceLastSeen() time.Duration {
	if s.IsOnline {
		return 0
	}
	return time.Since(s.LastSeenAt)
}

// DailyProgress represents a student's progress for a specific day.
// Used for the Daily Grind feature and daily digest notifications.
type DailyProgress struct {
	StudentID      StudentID
	Date           time.Time // The day this progress represents
	TasksCompleted int
	XPEarned       int
	TimeSpent      time.Duration
	SessionCount   int
}

// NewDailyProgress creates a new daily progress record.
func NewDailyProgress(studentID StudentID, date time.Time) *DailyProgress {
	return &DailyProgress{
		StudentID: studentID,
		Date:      truncateToDay(date),
	}
}

// AddTaskCompletion adds a task completion to the daily progress.
func (dp *DailyProgress) AddTaskCompletion(xp int, timeSpent time.Duration) {
	dp.TasksCompleted++
	dp.XPEarned += xp
	dp.TimeSpent += timeSpent
}

// AddSession adds a session to the daily progress.
func (dp *DailyProgress) AddSession(duration time.Duration) {
	dp.SessionCount++
	dp.TimeSpent += duration
}
