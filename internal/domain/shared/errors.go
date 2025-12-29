// Package shared contains common domain types, errors, events, and value objects
// that are used across all domain packages. This package has zero external dependencies.
package shared

import (
	"errors"
	"fmt"
)

// Base domain errors that can be used for error checking with errors.Is().
var (
	// Entity errors
	ErrNotFound      = errors.New("entity not found")
	ErrAlreadyExists = errors.New("entity already exists")
	ErrInvalidEntity = errors.New("invalid entity")

	// Validation errors
	ErrValidation      = errors.New("validation error")
	ErrInvalidID       = errors.New("invalid ID")
	ErrInvalidInput    = errors.New("invalid input")
	ErrEmptyValue      = errors.New("value cannot be empty")
	ErrNegativeValue   = errors.New("value cannot be negative")
	ErrValueOutOfRange = errors.New("value out of range")
	ErrFutureTimestamp = errors.New("timestamp cannot be in the future")
	ErrInvalidFormat   = errors.New("invalid format")

	// State errors
	ErrInvalidState     = errors.New("invalid state")
	ErrStateTransition  = errors.New("invalid state transition")
	ErrAlreadyProcessed = errors.New("already processed")
	ErrExpired          = errors.New("expired")

	// Authorization errors
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")

	// Concurrency errors
	ErrConcurrentModification = errors.New("concurrent modification detected")
	ErrOptimisticLock         = errors.New("optimistic lock failure")

	// External service errors
	ErrExternalService    = errors.New("external service error")
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrTimeout            = errors.New("operation timeout")
	ErrRateLimited        = errors.New("rate limited")
)

// DomainError represents a domain-specific error with context.
type DomainError struct {
	Domain  string // e.g., "student", "leaderboard", "activity"
	Op      string // Operation that failed, e.g., "Create", "Update"
	Kind    error  // Base error type for errors.Is() checking
	Message string // Human-readable message
	Err     error  // Underlying error (optional)
}

// Error implements the error interface.
func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s.%s: %s: %v", e.Domain, e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("%s.%s: %s", e.Domain, e.Op, e.Message)
}

// Unwrap returns the underlying error for errors.Unwrap().
func (e *DomainError) Unwrap() error {
	if e.Err != nil {
		return e.Err
	}
	return e.Kind
}

// Is implements errors.Is() matching.
func (e *DomainError) Is(target error) bool {
	if e.Kind != nil && errors.Is(e.Kind, target) {
		return true
	}
	if e.Err != nil && errors.Is(e.Err, target) {
		return true
	}
	return false
}

// NewDomainError creates a new domain error.
func NewDomainError(domain, op string, kind error, message string) *DomainError {
	return &DomainError{
		Domain:  domain,
		Op:      op,
		Kind:    kind,
		Message: message,
	}
}

// WrapError wraps an existing error with domain context.
func WrapError(domain, op string, kind error, message string, err error) *DomainError {
	return &DomainError{
		Domain:  domain,
		Op:      op,
		Kind:    kind,
		Message: message,
		Err:     err,
	}
}

// Student domain errors
var (
	ErrStudentNotFound      = NewDomainError("student", "Find", ErrNotFound, "student not found")
	ErrStudentAlreadyExists = NewDomainError("student", "Create", ErrAlreadyExists, "student already exists")
	ErrInvalidTelegramID    = NewDomainError("student", "Validate", ErrInvalidID, "invalid Telegram ID")
	ErrInvalidAlemID        = NewDomainError("student", "Validate", ErrInvalidID, "invalid Alem ID")
	ErrStudentNotActive     = NewDomainError("student", "CheckStatus", ErrInvalidState, "student is not active")
	ErrStudentAlreadyLinked = NewDomainError("student", "Link", ErrAlreadyExists, "Telegram already linked to Alem account")
	ErrInvalidStudentStatus = NewDomainError("student", "UpdateStatus", ErrStateTransition, "invalid student status transition")
)

// Leaderboard domain errors
var (
	ErrLeaderboardNotFound = NewDomainError("leaderboard", "Find", ErrNotFound, "leaderboard not found")
	ErrInvalidCohort       = NewDomainError("leaderboard", "Validate", ErrInvalidInput, "invalid cohort")
	ErrInvalidRank         = NewDomainError("leaderboard", "Validate", ErrValueOutOfRange, "invalid rank")
	ErrSnapshotNotFound    = NewDomainError("leaderboard", "FindSnapshot", ErrNotFound, "snapshot not found")
	ErrLeaderboardStale    = NewDomainError("leaderboard", "Refresh", ErrExpired, "leaderboard data is stale")
)

// Activity domain errors
var (
	ErrActivityNotFound     = NewDomainError("activity", "Find", ErrNotFound, "activity not found")
	ErrSessionNotFound      = NewDomainError("activity", "FindSession", ErrNotFound, "session not found")
	ErrSessionAlreadyActive = NewDomainError("activity", "StartSession", ErrAlreadyExists, "session already active")
	ErrSessionAlreadyEnded  = NewDomainError("activity", "EndSession", ErrInvalidState, "session already ended")
	ErrNoActiveSession      = NewDomainError("activity", "EndSession", ErrNotFound, "no active session")
	ErrInvalidTaskID        = NewDomainError("activity", "Validate", ErrInvalidID, "invalid task ID")
	ErrTaskAlreadyCompleted = NewDomainError("activity", "CompleteTask", ErrAlreadyExists, "task already completed")
)

// Social domain errors
var (
	ErrConnectionNotFound   = NewDomainError("social", "FindConnection", ErrNotFound, "connection not found")
	ErrConnectionExists     = NewDomainError("social", "CreateConnection", ErrAlreadyExists, "connection already exists")
	ErrSelfConnection       = NewDomainError("social", "CreateConnection", ErrInvalidInput, "cannot connect to self")
	ErrHelpRequestNotFound  = NewDomainError("social", "FindHelpRequest", ErrNotFound, "help request not found")
	ErrHelpRequestExpired   = NewDomainError("social", "RespondHelp", ErrExpired, "help request expired")
	ErrEndorsementNotFound  = NewDomainError("social", "FindEndorsement", ErrNotFound, "endorsement not found")
	ErrSelfEndorsement      = NewDomainError("social", "CreateEndorsement", ErrInvalidInput, "cannot endorse self")
	ErrDuplicateEndorsement = NewDomainError("social", "CreateEndorsement", ErrAlreadyExists, "already endorsed for this help")
	ErrInvalidRating        = NewDomainError("social", "Validate", ErrValueOutOfRange, "rating must be between 1 and 5")
)

// Notification domain errors
var (
	ErrNotificationNotFound = NewDomainError("notification", "Find", ErrNotFound, "notification not found")
	ErrNotificationFailed   = NewDomainError("notification", "Send", ErrExternalService, "failed to send notification")
	ErrInvalidChannel       = NewDomainError("notification", "Validate", ErrInvalidInput, "invalid notification channel")
	ErrNotificationDisabled = NewDomainError("notification", "Check", ErrForbidden, "notifications disabled by user")
	ErrTooManyNotifications = NewDomainError("notification", "Send", ErrRateLimited, "too many notifications")
)

// External service errors
var (
	ErrAlemAPIUnavailable     = NewDomainError("alem", "Request", ErrServiceUnavailable, "Alem API is unavailable")
	ErrAlemAPIRateLimited     = NewDomainError("alem", "Request", ErrRateLimited, "Alem API rate limit exceeded")
	ErrAlemAPITimeout         = NewDomainError("alem", "Request", ErrTimeout, "Alem API request timeout")
	ErrAlemAPIInvalidResponse = NewDomainError("alem", "Parse", ErrInvalidFormat, "invalid response from Alem API")
	ErrTelegramAPIFailed      = NewDomainError("telegram", "Send", ErrExternalService, "Telegram API request failed")
)

// IsNotFound checks if the error is a "not found" error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExists checks if the error is an "already exists" error.
func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

// IsValidation checks if the error is a validation error.
func IsValidation(err error) bool {
	return errors.Is(err, ErrValidation) ||
		errors.Is(err, ErrInvalidID) ||
		errors.Is(err, ErrInvalidInput) ||
		errors.Is(err, ErrEmptyValue) ||
		errors.Is(err, ErrNegativeValue) ||
		errors.Is(err, ErrValueOutOfRange)
}

// IsExternalService checks if the error is from an external service.
func IsExternalService(err error) bool {
	return errors.Is(err, ErrExternalService) ||
		errors.Is(err, ErrServiceUnavailable) ||
		errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrRateLimited)
}

// IsRetryable checks if the operation can be retried.
func IsRetryable(err error) bool {
	return errors.Is(err, ErrServiceUnavailable) ||
		errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrConcurrentModification)
}
