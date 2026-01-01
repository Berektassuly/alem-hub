// Package command contains write operations (CQRS - Commands).
package command

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/activity"
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/social"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"context"
	"errors"
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// REQUEST HELP COMMAND
// Creates a help request for a specific task and finds potential helpers.
// This is the core of "From Competition to Collaboration" philosophy.
// The leaderboard transforms into a "phone book" of helpers.
// ══════════════════════════════════════════════════════════════════════════════

// RequestHelpCommand contains the data to request help.
type RequestHelpCommand struct {
	// RequesterID is the ID of the student requesting help.
	RequesterID string

	// TaskID is the ID of the task they need help with.
	TaskID string

	// Message is an optional message describing the problem.
	Message string

	// Priority is the priority level of the request.
	Priority social.HelpRequestPriority

	// DeadlineAt is when the student needs to complete the task (optional).
	DeadlineAt *time.Time

	// PreferredHelperIDs is a list of preferred helpers (optional).
	PreferredHelperIDs []string

	// MaxHelpers is the maximum number of helpers to match (default: 5).
	MaxHelpers int

	// NotifyHelpers controls whether to notify matched helpers.
	NotifyHelpers bool

	// CorrelationID for tracing.
	CorrelationID string
}

// Validate validates the command.
func (c RequestHelpCommand) Validate() error {
	if c.RequesterID == "" {
		return errors.New("request_help: requester_id is required")
	}
	if c.TaskID == "" {
		return errors.New("request_help: task_id is required")
	}
	if c.MaxHelpers <= 0 {
		c.MaxHelpers = 5
	}
	if c.Priority == "" {
		c.Priority = social.HelpRequestPriorityNormal
	}
	if !c.Priority.IsValid() {
		return fmt.Errorf("request_help: invalid priority: %s", c.Priority)
	}
	return nil
}

// RequestHelpResult contains the result of requesting help.
type RequestHelpResult struct {
	// RequestID is the ID of the created help request.
	RequestID string

	// Status is the status of the request.
	Status social.HelpRequestStatus

	// MatchedHelpers contains the list of matched helpers.
	MatchedHelpers []MatchedHelperInfo

	// TotalHelpersFound is the total number of potential helpers found.
	TotalHelpersFound int

	// NotifiedCount is the number of helpers notified.
	NotifiedCount int

	// ExpiresAt is when the request will expire.
	ExpiresAt time.Time

	// Events contains domain events generated.
	Events []shared.Event

	// CreatedAt is when the request was created.
	CreatedAt time.Time
}

// MatchedHelperInfo contains information about a matched helper.
type MatchedHelperInfo struct {
	// StudentID is the helper's ID.
	StudentID string

	// DisplayName is the helper's display name.
	DisplayName string



	// TelegramID is the helper's Telegram ID.
	TelegramID int64

	// IsOnline indicates if the helper is currently online.
	IsOnline bool

	// LastSeenAt is when the helper was last seen.
	LastSeenAt time.Time

	// HelpRating is the helper's average rating.
	HelpRating float64

	// TimesHelped is how many times this helper has helped others.
	TimesHelped int

	// HasHelpedBefore indicates if they helped this requester before.
	HasHelpedBefore bool

	// MatchScore is the matching score (0-100).
	MatchScore int

	// CompletedTaskAt is when the helper completed the task.
	CompletedTaskAt *time.Time
}

// ══════════════════════════════════════════════════════════════════════════════
// DEPENDENCIES
// ══════════════════════════════════════════════════════════════════════════════

// HelperNotifier defines the interface for notifying helpers.
type HelperNotifier interface {
	// NotifyHelpRequest sends a notification about a help request.
	NotifyHelpRequest(ctx context.Context, helperID string, request *social.HelpRequest) error
}

// HelperMatchingService defines the interface for matching helpers.
type HelperMatchingService interface {
	// FindHelpers finds potential helpers for a task.
	FindHelpers(ctx context.Context, requesterID string, taskID string, limit int) ([]activity.HelperSuggestion, error)
}

// ══════════════════════════════════════════════════════════════════════════════
// HANDLER
// ══════════════════════════════════════════════════════════════════════════════

// RequestHelpHandler handles the RequestHelpCommand.
type RequestHelpHandler struct {
	studentRepo     student.Repository
	socialRepo      social.Repository
	activityRepo    activity.Repository
	onlineTracker   activity.OnlineTracker
	helperNotifier  HelperNotifier
	matchingService HelperMatchingService
	eventPublisher  shared.EventPublisher

	// Configuration
	requestExpiration time.Duration
	maxOpenRequests   int
}

// RequestHelpHandlerConfig contains configuration for the handler.
type RequestHelpHandlerConfig struct {
	RequestExpiration time.Duration // How long before a request expires
	MaxOpenRequests   int           // Max open requests per student
}

// DefaultRequestHelpHandlerConfig returns default configuration.
func DefaultRequestHelpHandlerConfig() RequestHelpHandlerConfig {
	return RequestHelpHandlerConfig{
		RequestExpiration: 24 * time.Hour,
		MaxOpenRequests:   3,
	}
}

// NewRequestHelpHandler creates a new RequestHelpHandler.
func NewRequestHelpHandler(
	studentRepo student.Repository,
	socialRepo social.Repository,
	activityRepo activity.Repository,
	onlineTracker activity.OnlineTracker,
	helperNotifier HelperNotifier,
	matchingService HelperMatchingService,
	eventPublisher shared.EventPublisher,
	config RequestHelpHandlerConfig,
) *RequestHelpHandler {
	if config.RequestExpiration == 0 {
		config = DefaultRequestHelpHandlerConfig()
	}

	return &RequestHelpHandler{
		studentRepo:       studentRepo,
		socialRepo:        socialRepo,
		activityRepo:      activityRepo,
		onlineTracker:     onlineTracker,
		helperNotifier:    helperNotifier,
		matchingService:   matchingService,
		eventPublisher:    eventPublisher,
		requestExpiration: config.RequestExpiration,
		maxOpenRequests:   config.MaxOpenRequests,
	}
}

// Handle executes the request help command.
func (h *RequestHelpHandler) Handle(ctx context.Context, cmd RequestHelpCommand) (*RequestHelpResult, error) {
	// Validate command
	if err := cmd.Validate(); err != nil {
		return nil, fmt.Errorf("request_help: validation failed: %w", err)
	}

	// Verify requester exists
	requester, err := h.studentRepo.GetByID(ctx, cmd.RequesterID)
	if err != nil {
		return nil, fmt.Errorf("request_help: failed to get requester: %w", err)
	}

	// Check if student is enrolled
	if !requester.Status.IsEnrolled() {
		return nil, errors.New("request_help: student is not enrolled")
	}

	// Check for open requests limit
	openRequests, err := h.socialRepo.HelpRequests().GetOpenByRequesterID(
		ctx,
		social.StudentID(cmd.RequesterID),
	)
	if err == nil && len(openRequests) >= h.maxOpenRequests {
		return nil, fmt.Errorf("request_help: max open requests limit reached (%d)", h.maxOpenRequests)
	}

	// Initialize result
	now := time.Now().UTC()
	result := &RequestHelpResult{
		CreatedAt: now,
		ExpiresAt: now.Add(h.requestExpiration),
		Events:    make([]shared.Event, 0),
	}

	// Create help request
	request, err := h.createHelpRequest(ctx, cmd, requester, now, result)
	if err != nil {
		return nil, fmt.Errorf("request_help: failed to create request: %w", err)
	}

	result.RequestID = request.ID
	result.Status = request.Status

	// Find potential helpers
	helpers, err := h.findAndMatchHelpers(ctx, cmd, request)
	if err != nil {
		// Log but don't fail - request is created but no helpers matched yet
	}

	result.MatchedHelpers = helpers
	result.TotalHelpersFound = len(helpers)

	// Update request with matched helpers
	if len(helpers) > 0 {
		request.Status = social.HelpRequestStatusMatched
		for _, helper := range helpers {
			request.MatchedHelpers = append(request.MatchedHelpers, social.MatchedHelper{
				StudentID:  social.StudentID(helper.StudentID),
				MatchScore: helper.MatchScore,
				IsOnline:   helper.IsOnline,
				LastSeenAt: helper.LastSeenAt,
			})
		}
		_ = h.socialRepo.HelpRequests().Update(ctx, request)
	}

	// Notify helpers if requested
	if cmd.NotifyHelpers && len(helpers) > 0 {
		result.NotifiedCount = h.notifyHelpers(ctx, helpers, request)
	}

	// Emit event
	event := shared.NewHelpRequestedEvent(cmd.RequesterID, cmd.TaskID, cmd.Message)
	if cmd.CorrelationID != "" {
		event.BaseEvent = event.BaseEvent.WithCorrelationID(cmd.CorrelationID)
	}
	result.Events = append(result.Events, event)

	// Publish events
	for _, e := range result.Events {
		_ = h.eventPublisher.Publish(e)
	}

	return result, nil
}

// createHelpRequest creates a new help request entity.
func (h *RequestHelpHandler) createHelpRequest(
	ctx context.Context,
	cmd RequestHelpCommand,
	requester *student.Student,
	now time.Time,
	result *RequestHelpResult,
) (*social.HelpRequest, error) {
	requestID := generateHelpRequestID(cmd.RequesterID, cmd.TaskID)

	params := social.NewHelpRequestParams{
		ID:          requestID,
		RequesterID: social.StudentID(cmd.RequesterID),
		TaskID:      social.TaskID(cmd.TaskID),
		TaskName:    cmd.TaskID, // Using task ID as name
		Description: cmd.Message,
		Priority:    cmd.Priority,
		DeadlineAt:  cmd.DeadlineAt,
	}

	request, err := social.NewHelpRequest(params)
	if err != nil {
		return nil, err
	}

	// Set expiration
	request.ExpiresAt = result.ExpiresAt

	// Save to repository
	if err := h.socialRepo.HelpRequests().Create(ctx, request); err != nil {
		return nil, fmt.Errorf("failed to save help request: %w", err)
	}

	return request, nil
}

// findAndMatchHelpers finds and ranks potential helpers.
func (h *RequestHelpHandler) findAndMatchHelpers(
	ctx context.Context,
	cmd RequestHelpCommand,
	request *social.HelpRequest,
) ([]MatchedHelperInfo, error) {
	maxHelpers := cmd.MaxHelpers
	if maxHelpers <= 0 {
		maxHelpers = 5
	}

	// Use matching service if available
	if h.matchingService != nil {
		suggestions, err := h.matchingService.FindHelpers(ctx, cmd.RequesterID, cmd.TaskID, maxHelpers*2)
		if err == nil {
			return h.convertSuggestionsToHelpers(ctx, cmd.RequesterID, suggestions, maxHelpers)
		}
	}

	// Fallback: Manual matching
	return h.manualHelperMatching(ctx, cmd, maxHelpers)
}

// convertSuggestionsToHelpers converts activity suggestions to helper info.
func (h *RequestHelpHandler) convertSuggestionsToHelpers(
	ctx context.Context,
	requesterID string,
	suggestions []activity.HelperSuggestion,
	limit int,
) ([]MatchedHelperInfo, error) {
	helpers := make([]MatchedHelperInfo, 0, limit)

	for _, suggestion := range suggestions {
		if len(helpers) >= limit {
			break
		}

		// Skip self
		if string(suggestion.StudentID) == requesterID {
			continue
		}

		// Get student details
		stud, err := h.studentRepo.GetByID(ctx, string(suggestion.StudentID))
		if err != nil {
			continue
		}

		// Skip students who don't want to help
		if !stud.CanHelp() {
			continue
		}

		helper := MatchedHelperInfo{
			StudentID:       string(suggestion.StudentID),
			DisplayName:     stud.DisplayName,
			TelegramID:      int64(stud.TelegramID),
			IsOnline:        suggestion.IsOnline,
			LastSeenAt:      suggestion.LastSeenAt,
			HelpRating:      suggestion.HelperRating,
			TimesHelped:     suggestion.TimesHelpedOther,
			HasHelpedBefore: suggestion.HasPriorContact,
			MatchScore:      calculateMatchScore(suggestion),
		}

		if !suggestion.CompletedTaskAt.IsZero() {
			helper.CompletedTaskAt = &suggestion.CompletedTaskAt
		}

		helpers = append(helpers, helper)
	}

	return helpers, nil
}

// manualHelperMatching performs manual helper matching.
func (h *RequestHelpHandler) manualHelperMatching(
	ctx context.Context,
	cmd RequestHelpCommand,
	limit int,
) ([]MatchedHelperInfo, error) {
	helpers := make([]MatchedHelperInfo, 0, limit)

	// First, check preferred helpers
	for _, preferredID := range cmd.PreferredHelperIDs {
		if len(helpers) >= limit {
			break
		}

		stud, err := h.studentRepo.GetByID(ctx, preferredID)
		if err != nil || !stud.CanHelp() {
			continue
		}

		// Check if they completed the task
		hasCompleted, _ := h.activityRepo.HasStudentCompletedTask(
			ctx,
			activity.StudentID(preferredID),
			activity.TaskID(cmd.TaskID),
		)
		if !hasCompleted {
			continue
		}

		isOnline, _ := h.onlineTracker.IsOnline(ctx, activity.StudentID(preferredID))

		helpers = append(helpers, MatchedHelperInfo{
			StudentID:   preferredID,
			DisplayName: stud.DisplayName,
			TelegramID:  int64(stud.TelegramID),
			IsOnline:    isOnline,
			LastSeenAt:  stud.LastSeenAt,
			HelpRating:  stud.HelpRating,
			TimesHelped: stud.HelpCount,
			MatchScore:  90, // High score for preferred helpers
		})
	}

	// Then find others who completed the task
	if len(helpers) < limit {
		completedBy, err := h.activityRepo.GetStudentsWhoCompletedTask(
			ctx,
			activity.TaskID(cmd.TaskID),
			limit*3, // Get more to filter
		)
		if err == nil {
			for _, studentID := range completedBy {
				if len(helpers) >= limit {
					break
				}

				// Skip requester and already added helpers
				if string(studentID) == cmd.RequesterID {
					continue
				}
				alreadyAdded := false
				for _, h := range helpers {
					if h.StudentID == string(studentID) {
						alreadyAdded = true
						break
					}
				}
				if alreadyAdded {
					continue
				}

				stud, err := h.studentRepo.GetByID(ctx, string(studentID))
				if err != nil || !stud.CanHelp() {
					continue
				}

				isOnline, _ := h.onlineTracker.IsOnline(ctx, studentID)

				score := 50
				if isOnline {
					score += 20
				}
				if stud.HelpRating >= 4.0 {
					score += 15
				}
				if stud.HelpCount > 5 {
					score += 10
				}

				helpers = append(helpers, MatchedHelperInfo{
					StudentID:   string(studentID),
					DisplayName: stud.DisplayName,
					TelegramID:  int64(stud.TelegramID),
					IsOnline:    isOnline,
					LastSeenAt:  stud.LastSeenAt,
					HelpRating:  stud.HelpRating,
					TimesHelped: stud.HelpCount,
					MatchScore:  score,
				})
			}
		}
	}

	return helpers, nil
}

// notifyHelpers notifies matched helpers about the request.
func (h *RequestHelpHandler) notifyHelpers(
	ctx context.Context,
	helpers []MatchedHelperInfo,
	request *social.HelpRequest,
) int {
	notified := 0

	for _, helper := range helpers {
		// Prioritize online helpers
		if !helper.IsOnline && notified >= 2 {
			continue
		}

		if err := h.helperNotifier.NotifyHelpRequest(ctx, helper.StudentID, request); err != nil {
			continue
		}

		notified++
	}

	return notified
}

// calculateMatchScore calculates a match score for a helper.
func calculateMatchScore(suggestion activity.HelperSuggestion) int {
	score := 50 // Base score

	// Online bonus
	if suggestion.IsOnline {
		score += 25
	} else if time.Since(suggestion.LastSeenAt) < 30*time.Minute {
		score += 15
	} else if time.Since(suggestion.LastSeenAt) < time.Hour {
		score += 10
	}

	// Rating bonus
	if suggestion.HelperRating >= 4.5 {
		score += 15
	} else if suggestion.HelperRating >= 4.0 {
		score += 10
	} else if suggestion.HelperRating >= 3.5 {
		score += 5
	}

	// Experience bonus
	if suggestion.TimesHelpedOther >= 20 {
		score += 10
	} else if suggestion.TimesHelpedOther >= 10 {
		score += 7
	} else if suggestion.TimesHelpedOther >= 5 {
		score += 5
	}

	// Prior contact bonus
	if suggestion.HasPriorContact {
		score += 5
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

func generateHelpRequestID(requesterID, taskID string) string {
	return fmt.Sprintf("help_%s_%s_%d", requesterID, taskID, time.Now().UnixNano())
}

// ══════════════════════════════════════════════════════════════════════════════
// RESOLVE HELP REQUEST COMMAND
// Marks a help request as resolved.
// ══════════════════════════════════════════════════════════════════════════════

// ResolveHelpRequestCommand marks a help request as resolved.
type ResolveHelpRequestCommand struct {
	// RequestID is the ID of the help request.
	RequestID string

	// RequesterID is the ID of the requester (for validation).
	RequesterID string

	// HelperID is the ID of the helper who resolved it.
	HelperID string

	// Resolution describes how the problem was resolved.
	Resolution string

	// CorrelationID for tracing.
	CorrelationID string
}

// Validate validates the command.
func (c ResolveHelpRequestCommand) Validate() error {
	if c.RequestID == "" {
		return errors.New("resolve_help: request_id is required")
	}
	if c.RequesterID == "" {
		return errors.New("resolve_help: requester_id is required")
	}
	return nil
}

// ResolveHelpRequestResult contains the result of resolving a request.
type ResolveHelpRequestResult struct {
	// Success indicates if the request was resolved.
	Success bool

	// RequestID is the ID of the resolved request.
	RequestID string

	// HelperID is the ID of the helper.
	HelperID string

	// Duration is how long the request was open.
	Duration time.Duration

	// Events contains domain events generated.
	Events []shared.Event
}

// ResolveHelpRequestHandler handles the ResolveHelpRequestCommand.
type ResolveHelpRequestHandler struct {
	socialRepo     social.Repository
	studentRepo    student.Repository
	eventPublisher shared.EventPublisher
}

// NewResolveHelpRequestHandler creates a new handler.
func NewResolveHelpRequestHandler(
	socialRepo social.Repository,
	studentRepo student.Repository,
	eventPublisher shared.EventPublisher,
) *ResolveHelpRequestHandler {
	return &ResolveHelpRequestHandler{
		socialRepo:     socialRepo,
		studentRepo:    studentRepo,
		eventPublisher: eventPublisher,
	}
}

// Handle executes the resolve help request command.
func (h *ResolveHelpRequestHandler) Handle(
	ctx context.Context,
	cmd ResolveHelpRequestCommand,
) (*ResolveHelpRequestResult, error) {
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// Get the request
	request, err := h.socialRepo.HelpRequests().GetByID(ctx, cmd.RequestID)
	if err != nil {
		return nil, fmt.Errorf("resolve_help: request not found: %w", err)
	}

	// Verify requester
	if string(request.RequesterID) != cmd.RequesterID {
		return nil, errors.New("resolve_help: requester mismatch")
	}

	// Resolve the request
	var helperID *social.StudentID
	if cmd.HelperID != "" {
		h := social.StudentID(cmd.HelperID)
		helperID = &h
	}

	resolution := social.HelpResolution{
		Method:   social.HelpResolutionWithHelper,
		HelperID: helperID,
		Notes:    cmd.Resolution,
	}

	if err := request.Resolve(resolution); err != nil {
		return nil, fmt.Errorf("resolve_help: failed to resolve: %w", err)
	}

	// Save changes
	if err := h.socialRepo.HelpRequests().Update(ctx, request); err != nil {
		return nil, fmt.Errorf("resolve_help: failed to save: %w", err)
	}

	// Update helper's help count if provided
	if cmd.HelperID != "" {
		helper, err := h.studentRepo.GetByID(ctx, cmd.HelperID)
		if err == nil {
			helper.HelpCount++
			_ = h.studentRepo.Update(ctx, helper)
		}
	}

	result := &ResolveHelpRequestResult{
		Success:   true,
		RequestID: cmd.RequestID,
		HelperID:  cmd.HelperID,
		Duration:  time.Since(request.CreatedAt),
		Events:    make([]shared.Event, 0),
	}

	// Emit event
	if cmd.HelperID != "" {
		event := shared.NewHelpProvidedEvent(
			cmd.HelperID,
			cmd.RequesterID,
			string(request.TaskID),
		)
		event.RequestID = cmd.RequestID
		if cmd.CorrelationID != "" {
			event.BaseEvent = event.BaseEvent.WithCorrelationID(cmd.CorrelationID)
		}
		result.Events = append(result.Events, event)
		_ = h.eventPublisher.Publish(event)
	}

	return result, nil
}
