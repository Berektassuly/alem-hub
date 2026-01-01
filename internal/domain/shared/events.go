// Package shared contains common domain types, errors, events, and value objects
// that are used across all domain packages.
package shared

import (
	"encoding/json"
	"time"
)

// EventType represents the type of domain event.
type EventType string

// Domain event types - these drive the event-driven architecture.
// Each event represents something significant that happened in the domain.
const (
	// Student events
	EventStudentRegistered  EventType = "student.registered"
	EventStudentUpdated     EventType = "student.updated"
	EventStudentDeactivated EventType = "student.deactivated"
	EventStudentReactivated EventType = "student.reactivated"

	// Progress events
	EventXPGained           EventType = "progress.xp_gained"
	EventLevelUp            EventType = "progress.level_up"
	EventTaskCompleted      EventType = "progress.task_completed"
	EventDailyStreakUpdated EventType = "progress.streak_updated"
	EventDailyStreakBroken  EventType = "progress.streak_broken"

	// Leaderboard events
	EventRankChanged        EventType = "leaderboard.rank_changed"
	EventEnteredTopN        EventType = "leaderboard.entered_top_n"
	EventLeftTopN           EventType = "leaderboard.left_top_n"
	EventLeaderboardUpdated EventType = "leaderboard.updated"

	// Activity events
	EventStudentWentOnline  EventType = "activity.went_online"
	EventStudentWentOffline EventType = "activity.went_offline"
	EventSessionStarted     EventType = "activity.session_started"
	EventSessionEnded       EventType = "activity.session_ended"

	// Social events
	EventHelpRequested    EventType = "social.help_requested"
	EventHelpProvided     EventType = "social.help_provided"
	EventConnectionMade   EventType = "social.connection_made"
	EventEndorsementGiven EventType = "social.endorsement_given"
	EventMentorMatched    EventType = "social.mentor_matched"

	// Notification events
	EventNotificationSent   EventType = "notification.sent"
	EventNotificationFailed EventType = "notification.failed"

	// System events
	EventSyncCompleted   EventType = "system.sync_completed"
	EventStudentInactive EventType = "system.student_inactive"
)

// Event is the base interface for all domain events.
type Event interface {
	// EventType returns the type of the event.
	EventType() EventType

	// OccurredAt returns when the event occurred.
	OccurredAt() time.Time

	// AggregateID returns the ID of the aggregate that produced this event.
	AggregateID() string

	// Payload returns the event data as a map for serialization.
	Payload() map[string]interface{}
}

// BaseEvent provides common event functionality.
type BaseEvent struct {
	Type          EventType `json:"type"`
	Timestamp     time.Time `json:"timestamp"`
	AggregateId   string    `json:"aggregate_id"`
	Version       int       `json:"version"`
	CorrelationID string    `json:"correlation_id,omitempty"`
}

// EventType implements Event interface.
func (e BaseEvent) EventType() EventType {
	return e.Type
}

// OccurredAt implements Event interface.
func (e BaseEvent) OccurredAt() time.Time {
	return e.Timestamp
}

// AggregateID implements Event interface.
func (e BaseEvent) AggregateID() string {
	return e.AggregateId
}

// NewBaseEvent creates a new base event.
func NewBaseEvent(eventType EventType, aggregateID string) BaseEvent {
	return BaseEvent{
		Type:        eventType,
		Timestamp:   time.Now(),
		AggregateId: aggregateID,
		Version:     1,
	}
}

// WithCorrelationID sets the correlation ID for tracing.
func (e BaseEvent) WithCorrelationID(id string) BaseEvent {
	e.CorrelationID = id
	return e
}

// ═══════════════════════════════════════════════════════════════════════════
// Student Events
// ═══════════════════════════════════════════════════════════════════════════

// StudentRegisteredEvent is emitted when a new student registers.
type StudentRegisteredEvent struct {
	BaseEvent
	TelegramID  int64  `json:"telegram_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Cohort      string `json:"cohort"`
}

// Payload implements Event interface.
func (e StudentRegisteredEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"telegram_id":  e.TelegramID,
		"email":        e.Email,
		"display_name": e.DisplayName,
		"cohort":       e.Cohort,
	}
}

// NewStudentRegisteredEvent creates a new StudentRegisteredEvent.
func NewStudentRegisteredEvent(studentID string, telegramID int64, email, displayName, cohort string) StudentRegisteredEvent {
	return StudentRegisteredEvent{
		BaseEvent:   NewBaseEvent(EventStudentRegistered, studentID),
		TelegramID:  telegramID,
		Email:       email,
		DisplayName: displayName,
		Cohort:      cohort,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Progress Events
// ═══════════════════════════════════════════════════════════════════════════

// XPGainedEvent is emitted when a student gains XP.
type XPGainedEvent struct {
	BaseEvent
	StudentID string `json:"student_id"`
	Amount    int    `json:"amount"`
	NewTotal  int    `json:"new_total"`
	Source    string `json:"source"` // e.g., "task_completion", "bonus"
	TaskID    string `json:"task_id,omitempty"`
}

// Payload implements Event interface.
func (e XPGainedEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_id": e.StudentID,
		"amount":     e.Amount,
		"new_total":  e.NewTotal,
		"source":     e.Source,
		"task_id":    e.TaskID,
	}
}

// NewXPGainedEvent creates a new XPGainedEvent.
func NewXPGainedEvent(studentID string, amount, newTotal int, source, taskID string) XPGainedEvent {
	return XPGainedEvent{
		BaseEvent: NewBaseEvent(EventXPGained, studentID),
		StudentID: studentID,
		Amount:    amount,
		NewTotal:  newTotal,
		Source:    source,
		TaskID:    taskID,
	}
}

// TaskCompletedEvent is emitted when a student completes a task.
type TaskCompletedEvent struct {
	BaseEvent
	StudentID string        `json:"student_id"`
	TaskID    string        `json:"task_id"`
	XPEarned  int           `json:"xp_earned"`
	TimeSpent time.Duration `json:"time_spent"`
	HelperID  string        `json:"helper_id,omitempty"` // Who helped, if anyone
}

// Payload implements Event interface.
func (e TaskCompletedEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_id": e.StudentID,
		"task_id":    e.TaskID,
		"xp_earned":  e.XPEarned,
		"time_spent": e.TimeSpent.String(),
		"helper_id":  e.HelperID,
	}
}

// NewTaskCompletedEvent creates a new TaskCompletedEvent.
func NewTaskCompletedEvent(studentID, taskID string, xpEarned int, timeSpent time.Duration) TaskCompletedEvent {
	return TaskCompletedEvent{
		BaseEvent: NewBaseEvent(EventTaskCompleted, studentID),
		StudentID: studentID,
		TaskID:    taskID,
		XPEarned:  xpEarned,
		TimeSpent: timeSpent,
	}
}

// WithHelper adds helper information to the event.
func (e TaskCompletedEvent) WithHelper(helperID string) TaskCompletedEvent {
	e.HelperID = helperID
	return e
}

// DailyStreakBrokenEvent is emitted when a student's daily streak is broken.
type DailyStreakBrokenEvent struct {
	BaseEvent
	StudentID      string `json:"student_id"`
	PreviousStreak int    `json:"previous_streak"`
	DaysMissed     int    `json:"days_missed"`
}

// Payload implements Event interface.
func (e DailyStreakBrokenEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_id":      e.StudentID,
		"previous_streak": e.PreviousStreak,
		"days_missed":     e.DaysMissed,
	}
}

// NewDailyStreakBrokenEvent creates a new DailyStreakBrokenEvent.
func NewDailyStreakBrokenEvent(studentID string, previousStreak, daysMissed int) DailyStreakBrokenEvent {
	return DailyStreakBrokenEvent{
		BaseEvent:      NewBaseEvent(EventDailyStreakBroken, studentID),
		StudentID:      studentID,
		PreviousStreak: previousStreak,
		DaysMissed:     daysMissed,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Leaderboard Events
// ═══════════════════════════════════════════════════════════════════════════

// RankChangedEvent is emitted when a student's rank changes.
type RankChangedEvent struct {
	BaseEvent
	StudentID   string `json:"student_id"`
	OldRank     int    `json:"old_rank"`
	NewRank     int    `json:"new_rank"`
	RankChange  int    `json:"rank_change"` // Positive = moved up, Negative = moved down
	Cohort      string `json:"cohort"`
	OvertakenBy string `json:"overtaken_by,omitempty"` // Who overtook this student
	Overtook    string `json:"overtook,omitempty"`     // Who this student overtook
}

// Payload implements Event interface.
func (e RankChangedEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_id":   e.StudentID,
		"old_rank":     e.OldRank,
		"new_rank":     e.NewRank,
		"rank_change":  e.RankChange,
		"cohort":       e.Cohort,
		"overtaken_by": e.OvertakenBy,
		"overtook":     e.Overtook,
	}
}

// NewRankChangedEvent creates a new RankChangedEvent.
func NewRankChangedEvent(studentID string, oldRank, newRank int, cohort string) RankChangedEvent {
	return RankChangedEvent{
		BaseEvent:  NewBaseEvent(EventRankChanged, studentID),
		StudentID:  studentID,
		OldRank:    oldRank,
		NewRank:    newRank,
		RankChange: oldRank - newRank, // Positive means moved up
		Cohort:     cohort,
	}
}

// MovedUp returns true if the student moved up in rank.
func (e RankChangedEvent) MovedUp() bool {
	return e.RankChange > 0
}

// MovedDown returns true if the student moved down in rank.
func (e RankChangedEvent) MovedDown() bool {
	return e.RankChange < 0
}

// EnteredTopNEvent is emitted when a student enters the top N.
type EnteredTopNEvent struct {
	BaseEvent
	StudentID string `json:"student_id"`
	TopN      int    `json:"top_n"` // e.g., 10, 50, 100
	NewRank   int    `json:"new_rank"`
	Cohort    string `json:"cohort"`
}

// Payload implements Event interface.
func (e EnteredTopNEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_id": e.StudentID,
		"top_n":      e.TopN,
		"new_rank":   e.NewRank,
		"cohort":     e.Cohort,
	}
}

// NewEnteredTopNEvent creates a new EnteredTopNEvent.
func NewEnteredTopNEvent(studentID string, topN, newRank int, cohort string) EnteredTopNEvent {
	return EnteredTopNEvent{
		BaseEvent: NewBaseEvent(EventEnteredTopN, studentID),
		StudentID: studentID,
		TopN:      topN,
		NewRank:   newRank,
		Cohort:    cohort,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Activity Events
// ═══════════════════════════════════════════════════════════════════════════

// StudentWentOnlineEvent is emitted when a student comes online.
type StudentWentOnlineEvent struct {
	BaseEvent
	StudentID string `json:"student_id"`
	SessionID string `json:"session_id"`
}

// Payload implements Event interface.
func (e StudentWentOnlineEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_id": e.StudentID,
		"session_id": e.SessionID,
	}
}

// NewStudentWentOnlineEvent creates a new StudentWentOnlineEvent.
func NewStudentWentOnlineEvent(studentID, sessionID string) StudentWentOnlineEvent {
	return StudentWentOnlineEvent{
		BaseEvent: NewBaseEvent(EventStudentWentOnline, studentID),
		StudentID: studentID,
		SessionID: sessionID,
	}
}

// StudentWentOfflineEvent is emitted when a student goes offline.
type StudentWentOfflineEvent struct {
	BaseEvent
	StudentID       string        `json:"student_id"`
	SessionID       string        `json:"session_id"`
	SessionDuration time.Duration `json:"session_duration"`
	TasksCompleted  int           `json:"tasks_completed"`
	XPGained        int           `json:"xp_gained"`
}

// Payload implements Event interface.
func (e StudentWentOfflineEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_id":       e.StudentID,
		"session_id":       e.SessionID,
		"session_duration": e.SessionDuration.String(),
		"tasks_completed":  e.TasksCompleted,
		"xp_gained":        e.XPGained,
	}
}

// NewStudentWentOfflineEvent creates a new StudentWentOfflineEvent.
func NewStudentWentOfflineEvent(studentID, sessionID string, duration time.Duration, tasks, xp int) StudentWentOfflineEvent {
	return StudentWentOfflineEvent{
		BaseEvent:       NewBaseEvent(EventStudentWentOffline, studentID),
		StudentID:       studentID,
		SessionID:       sessionID,
		SessionDuration: duration,
		TasksCompleted:  tasks,
		XPGained:        xp,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Social Events (Core to "From Competition to Collaboration" philosophy)
// ═══════════════════════════════════════════════════════════════════════════

// HelpRequestedEvent is emitted when a student requests help.
type HelpRequestedEvent struct {
	BaseEvent
	StudentID string `json:"student_id"`
	TaskID    string `json:"task_id"`
	Message   string `json:"message,omitempty"`
}

// Payload implements Event interface.
func (e HelpRequestedEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_id": e.StudentID,
		"task_id":    e.TaskID,
		"message":    e.Message,
	}
}

// NewHelpRequestedEvent creates a new HelpRequestedEvent.
func NewHelpRequestedEvent(studentID, taskID, message string) HelpRequestedEvent {
	return HelpRequestedEvent{
		BaseEvent: NewBaseEvent(EventHelpRequested, studentID),
		StudentID: studentID,
		TaskID:    taskID,
		Message:   message,
	}
}

// HelpProvidedEvent is emitted when a student provides help to another.
type HelpProvidedEvent struct {
	BaseEvent
	HelperID  string `json:"helper_id"`
	HelpeeID  string `json:"helpee_id"`
	TaskID    string `json:"task_id"`
	RequestID string `json:"request_id,omitempty"`
}

// Payload implements Event interface.
func (e HelpProvidedEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"helper_id":  e.HelperID,
		"helpee_id":  e.HelpeeID,
		"task_id":    e.TaskID,
		"request_id": e.RequestID,
	}
}

// NewHelpProvidedEvent creates a new HelpProvidedEvent.
func NewHelpProvidedEvent(helperID, helpeeID, taskID string) HelpProvidedEvent {
	return HelpProvidedEvent{
		BaseEvent: NewBaseEvent(EventHelpProvided, helperID),
		HelperID:  helperID,
		HelpeeID:  helpeeID,
		TaskID:    taskID,
	}
}

// ConnectionMadeEvent is emitted when two students connect.
type ConnectionMadeEvent struct {
	BaseEvent
	StudentAID string `json:"student_a_id"`
	StudentBID string `json:"student_b_id"`
	Context    string `json:"context"` // e.g., "help_request", "study_buddy", "mentor"
	TaskID     string `json:"task_id,omitempty"`
}

// Payload implements Event interface.
func (e ConnectionMadeEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_a_id": e.StudentAID,
		"student_b_id": e.StudentBID,
		"context":      e.Context,
		"task_id":      e.TaskID,
	}
}

// NewConnectionMadeEvent creates a new ConnectionMadeEvent.
func NewConnectionMadeEvent(studentAID, studentBID, context string) ConnectionMadeEvent {
	return ConnectionMadeEvent{
		BaseEvent:  NewBaseEvent(EventConnectionMade, studentAID),
		StudentAID: studentAID,
		StudentBID: studentBID,
		Context:    context,
	}
}

// EndorsementGivenEvent is emitted when a student endorses another.
type EndorsementGivenEvent struct {
	BaseEvent
	FromStudentID string `json:"from_student_id"`
	ToStudentID   string `json:"to_student_id"`
	Rating        int    `json:"rating"` // 1-5
	TaskID        string `json:"task_id,omitempty"`
	Comment       string `json:"comment,omitempty"`
}

// Payload implements Event interface.
func (e EndorsementGivenEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"from_student_id": e.FromStudentID,
		"to_student_id":   e.ToStudentID,
		"rating":          e.Rating,
		"task_id":         e.TaskID,
		"comment":         e.Comment,
	}
}

// NewEndorsementGivenEvent creates a new EndorsementGivenEvent.
func NewEndorsementGivenEvent(fromID, toID string, rating int) EndorsementGivenEvent {
	return EndorsementGivenEvent{
		BaseEvent:     NewBaseEvent(EventEndorsementGiven, fromID),
		FromStudentID: fromID,
		ToStudentID:   toID,
		Rating:        rating,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// System Events
// ═══════════════════════════════════════════════════════════════════════════

// StudentInactiveEvent is emitted when a student has been inactive for too long.
type StudentInactiveEvent struct {
	BaseEvent
	StudentID    string    `json:"student_id"`
	DaysInactive int       `json:"days_inactive"`
	LastSeenAt   time.Time `json:"last_seen_at"`
}

// Payload implements Event interface.
func (e StudentInactiveEvent) Payload() map[string]interface{} {
	return map[string]interface{}{
		"student_id":    e.StudentID,
		"days_inactive": e.DaysInactive,
		"last_seen_at":  e.LastSeenAt.Format(time.RFC3339),
	}
}

// NewStudentInactiveEvent creates a new StudentInactiveEvent.
func NewStudentInactiveEvent(studentID string, daysInactive int, lastSeenAt time.Time) StudentInactiveEvent {
	return StudentInactiveEvent{
		BaseEvent:    NewBaseEvent(EventStudentInactive, studentID),
		StudentID:    studentID,
		DaysInactive: daysInactive,
		LastSeenAt:   lastSeenAt,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Event Envelope (for serialization and transport)
// ═══════════════════════════════════════════════════════════════════════════

// EventEnvelope wraps an event for transport/storage.
type EventEnvelope struct {
	ID            string          `json:"id"`
	Type          EventType       `json:"type"`
	AggregateID   string          `json:"aggregate_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Version       int             `json:"version"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

// EventHandler is a function that handles an event.
type EventHandler func(event Event) error

// EventPublisher defines the interface for publishing events.
type EventPublisher interface {
	// Publish sends an event to subscribers.
	Publish(event Event) error
}

// EventSubscriber defines the interface for subscribing to events.
type EventSubscriber interface {
	// Subscribe registers a handler for an event type.
	Subscribe(eventType EventType, handler EventHandler) error

	// SubscribeAll registers a handler for all events.
	SubscribeAll(handler EventHandler) error
}

// EventBus combines publishing and subscribing.
type EventBus interface {
	EventPublisher
	EventSubscriber
}
