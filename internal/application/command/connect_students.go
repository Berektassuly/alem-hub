// Package command contains write operations (CQRS - Commands).
package command

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/social"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"context"
	"errors"
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// CONNECT STUDENTS COMMAND
// Creates a connection (relationship) between two students.
// Connections are the foundation of the social graph that powers
// the "From Competition to Collaboration" philosophy.
// ══════════════════════════════════════════════════════════════════════════════

// ConnectStudentsCommand contains the data to create a connection.
type ConnectStudentsCommand struct {
	// InitiatorID is the ID of the student initiating the connection.
	InitiatorID string

	// TargetID is the ID of the student being connected to.
	TargetID string

	// Type is the type of connection.
	Type social.ConnectionType

	// Context describes how the connection was made.
	Context string

	// TaskID is the task related to this connection (for helper connections).
	TaskID string

	// Message is an optional message from the initiator.
	Message string

	// SkipConfirmation bypasses the pending state (for implicit connections).
	SkipConfirmation bool

	// CorrelationID for tracing.
	CorrelationID string
}

// Validate validates the command.
func (c ConnectStudentsCommand) Validate() error {
	if c.InitiatorID == "" {
		return errors.New("connect_students: initiator_id is required")
	}
	if c.TargetID == "" {
		return errors.New("connect_students: target_id is required")
	}
	if c.InitiatorID == c.TargetID {
		return errors.New("connect_students: cannot connect to self")
	}
	if c.Type == "" {
		return errors.New("connect_students: type is required")
	}
	if !c.Type.IsValid() {
		return fmt.Errorf("connect_students: invalid connection type: %s", c.Type)
	}
	return nil
}

// ConnectStudentsResult contains the result of creating a connection.
type ConnectStudentsResult struct {
	// ConnectionID is the ID of the created connection.
	ConnectionID string

	// Status is the status of the connection.
	Status social.ConnectionStatus

	// IsNewConnection indicates if this is a new connection or existing.
	IsNewConnection bool

	// WasUpgraded indicates if an existing connection was upgraded.
	WasUpgraded bool

	// PreviousType is the previous type (if upgraded).
	PreviousType social.ConnectionType

	// Events contains domain events generated.
	Events []shared.Event

	// CreatedAt is when the connection was created.
	CreatedAt time.Time
}

// ══════════════════════════════════════════════════════════════════════════════
// HANDLER
// ══════════════════════════════════════════════════════════════════════════════

// ConnectStudentsHandler handles the ConnectStudentsCommand.
type ConnectStudentsHandler struct {
	studentRepo    student.Repository
	socialRepo     social.Repository
	eventPublisher shared.EventPublisher
}

// NewConnectStudentsHandler creates a new ConnectStudentsHandler.
func NewConnectStudentsHandler(
	studentRepo student.Repository,
	socialRepo social.Repository,
	eventPublisher shared.EventPublisher,
) *ConnectStudentsHandler {
	return &ConnectStudentsHandler{
		studentRepo:    studentRepo,
		socialRepo:     socialRepo,
		eventPublisher: eventPublisher,
	}
}

// Handle executes the connect students command.
func (h *ConnectStudentsHandler) Handle(ctx context.Context, cmd ConnectStudentsCommand) (*ConnectStudentsResult, error) {
	// Validate command
	if err := cmd.Validate(); err != nil {
		return nil, fmt.Errorf("connect_students: validation failed: %w", err)
	}

	// Verify both students exist and are enrolled
	initiator, err := h.studentRepo.GetByID(ctx, cmd.InitiatorID)
	if err != nil {
		return nil, fmt.Errorf("connect_students: initiator not found: %w", err)
	}
	if !initiator.Status.IsEnrolled() {
		return nil, errors.New("connect_students: initiator is not enrolled")
	}

	target, err := h.studentRepo.GetByID(ctx, cmd.TargetID)
	if err != nil {
		return nil, fmt.Errorf("connect_students: target not found: %w", err)
	}
	if !target.Status.IsEnrolled() {
		return nil, errors.New("connect_students: target is not enrolled")
	}

	// Initialize result
	now := time.Now().UTC()
	result := &ConnectStudentsResult{
		CreatedAt: now,
		Events:    make([]shared.Event, 0),
	}

	// Check for existing connection
	existingConn, err := h.socialRepo.Connections().GetByStudents(
		ctx,
		social.StudentID(cmd.InitiatorID),
		social.StudentID(cmd.TargetID),
	)

	if err == nil && existingConn != nil {
		// Handle existing connection
		return h.handleExistingConnection(ctx, cmd, existingConn, result)
	}

	// Create new connection
	return h.createNewConnection(ctx, cmd, result)
}

// handleExistingConnection handles the case when a connection already exists.
func (h *ConnectStudentsHandler) handleExistingConnection(
	ctx context.Context,
	cmd ConnectStudentsCommand,
	existing *social.Connection,
	result *ConnectStudentsResult,
) (*ConnectStudentsResult, error) {
	result.ConnectionID = existing.ID
	result.Status = existing.Status
	result.IsNewConnection = false

	// Check if we should upgrade the connection type
	if h.shouldUpgrade(existing.Type, cmd.Type) {
		result.PreviousType = existing.Type
		result.WasUpgraded = true

		// Upgrade the connection by updating its type
		existing.Type = cmd.Type
		existing.UpdatedAt = time.Now().UTC()

		if err := h.socialRepo.Connections().Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("connect_students: failed to upgrade connection: %w", err)
		}

		result.Status = existing.Status
	}

	// If connection is pending and initiator is target of pending request, accept it
	if existing.Status == social.ConnectionStatusPending &&
		string(existing.ReceiverID) == cmd.InitiatorID {
		if err := existing.Accept(); err == nil {
			_ = h.socialRepo.Connections().Update(ctx, existing)
			result.Status = social.ConnectionStatusActive

			// Emit event for accepted connection
			event := shared.NewConnectionMadeEvent(
				cmd.InitiatorID,
				cmd.TargetID,
				string(cmd.Type),
			)
			if cmd.TaskID != "" {
				event.TaskID = cmd.TaskID
			}
			if cmd.CorrelationID != "" {
				event.BaseEvent = event.BaseEvent.WithCorrelationID(cmd.CorrelationID)
			}
			result.Events = append(result.Events, event)
			_ = h.eventPublisher.Publish(event)
		}
	}

	return result, nil
}

// createNewConnection creates a new connection between students.
func (h *ConnectStudentsHandler) createNewConnection(
	ctx context.Context,
	cmd ConnectStudentsCommand,
	result *ConnectStudentsResult,
) (*ConnectStudentsResult, error) {
	connectionID := generateConnectionID(cmd.InitiatorID, cmd.TargetID)

	// Build connection context from command data
	connContext := social.ConnectionContext{
		Note: cmd.Message,
	}
	if cmd.TaskID != "" {
		connContext.TaskID = social.TaskID(cmd.TaskID)
	}

	params := social.NewConnectionParams{
		ID:          connectionID,
		InitiatorID: social.StudentID(cmd.InitiatorID),
		ReceiverID:  social.StudentID(cmd.TargetID),
		Type:        cmd.Type,
		Context:     connContext,
	}

	connection, err := social.NewConnection(params)
	if err != nil {
		return nil, fmt.Errorf("connect_students: failed to create connection: %w", err)
	}

	// Skip confirmation for certain connection types or if explicitly requested
	if cmd.SkipConfirmation || cmd.Type == social.ConnectionTypeHelper {
		if err := connection.Accept(); err != nil {
			// Log but continue
		}
	}

	// Save connection
	if err := h.socialRepo.Connections().Create(ctx, connection); err != nil {
		return nil, fmt.Errorf("connect_students: failed to save connection: %w", err)
	}

	result.ConnectionID = connection.ID
	result.Status = connection.Status
	result.IsNewConnection = true

	// Emit event if connection is active
	if connection.Status == social.ConnectionStatusActive {
		event := shared.NewConnectionMadeEvent(
			cmd.InitiatorID,
			cmd.TargetID,
			string(cmd.Type),
		)
		if cmd.TaskID != "" {
			event.TaskID = cmd.TaskID
		}
		if cmd.CorrelationID != "" {
			event.BaseEvent = event.BaseEvent.WithCorrelationID(cmd.CorrelationID)
		}
		result.Events = append(result.Events, event)
		_ = h.eventPublisher.Publish(event)
	}

	return result, nil
}

// shouldUpgrade determines if a connection type should be upgraded.
func (h *ConnectStudentsHandler) shouldUpgrade(current, requested social.ConnectionType) bool {
	// Define upgrade paths
	// Helper -> StudyBuddy is an upgrade
	// Helper -> Mentor is an upgrade
	// StudyBuddy -> Mentor is an upgrade

	upgradeOrder := map[social.ConnectionType]int{
		social.ConnectionTypeHelper:     1,
		social.ConnectionTypeCoworker:   2,
		social.ConnectionTypeStudyBuddy: 3,
		social.ConnectionTypeMentor:     4,
	}

	return upgradeOrder[requested] > upgradeOrder[current]
}

func generateConnectionID(initiatorID, targetID string) string {
	return fmt.Sprintf("conn_%s_%s_%d", initiatorID, targetID, time.Now().UnixNano())
}

// ══════════════════════════════════════════════════════════════════════════════
// ACCEPT CONNECTION COMMAND
// ══════════════════════════════════════════════════════════════════════════════

// AcceptConnectionCommand accepts a pending connection request.
type AcceptConnectionCommand struct {
	// ConnectionID is the ID of the connection to accept.
	ConnectionID string

	// AccepterID is the ID of the student accepting (must be the target).
	AccepterID string

	// CorrelationID for tracing.
	CorrelationID string
}

// AcceptConnectionResult contains the result of accepting a connection.
type AcceptConnectionResult struct {
	// Success indicates if the connection was accepted.
	Success bool

	// ConnectionID is the ID of the connection.
	ConnectionID string

	// Events contains domain events generated.
	Events []shared.Event
}

// AcceptConnectionHandler handles the AcceptConnectionCommand.
type AcceptConnectionHandler struct {
	socialRepo     social.Repository
	eventPublisher shared.EventPublisher
}

// NewAcceptConnectionHandler creates a new handler.
func NewAcceptConnectionHandler(
	socialRepo social.Repository,
	eventPublisher shared.EventPublisher,
) *AcceptConnectionHandler {
	return &AcceptConnectionHandler{
		socialRepo:     socialRepo,
		eventPublisher: eventPublisher,
	}
}

// Handle executes the accept connection command.
func (h *AcceptConnectionHandler) Handle(
	ctx context.Context,
	cmd AcceptConnectionCommand,
) (*AcceptConnectionResult, error) {
	if cmd.ConnectionID == "" || cmd.AccepterID == "" {
		return nil, errors.New("accept_connection: connection_id and accepter_id are required")
	}

	// Get connection
	conn, err := h.socialRepo.Connections().GetByID(ctx, cmd.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("accept_connection: connection not found: %w", err)
	}

	// Verify accepter is the target
	if string(conn.ReceiverID) != cmd.AccepterID {
		return nil, errors.New("accept_connection: only the target can accept")
	}

	// Accept connection
	if err := conn.Accept(); err != nil {
		return nil, fmt.Errorf("accept_connection: failed to accept: %w", err)
	}

	// Save changes
	if err := h.socialRepo.Connections().Update(ctx, conn); err != nil {
		return nil, fmt.Errorf("accept_connection: failed to save: %w", err)
	}

	result := &AcceptConnectionResult{
		Success:      true,
		ConnectionID: cmd.ConnectionID,
		Events:       make([]shared.Event, 0),
	}

	// Emit event
	event := shared.NewConnectionMadeEvent(
		string(conn.InitiatorID),
		string(conn.ReceiverID),
		string(conn.Type),
	)
	if conn.Context.TaskID != "" {
		event.TaskID = string(conn.Context.TaskID)
	}
	if cmd.CorrelationID != "" {
		event.BaseEvent = event.BaseEvent.WithCorrelationID(cmd.CorrelationID)
	}
	result.Events = append(result.Events, event)
	_ = h.eventPublisher.Publish(event)

	return result, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// GIVE ENDORSEMENT COMMAND
// Awards an endorsement (thank you) from one student to another.
// ══════════════════════════════════════════════════════════════════════════════

// GiveEndorsementCommand creates an endorsement from one student to another.
type GiveEndorsementCommand struct {
	// GiverID is the ID of the student giving the endorsement.
	GiverID string

	// ReceiverID is the ID of the student receiving the endorsement.
	ReceiverID string

	// HelpRequestID is the ID of the help request this endorsement is for.
	HelpRequestID string

	// TaskID is the ID of the task related to the endorsement.
	TaskID string

	// Type is the type of endorsement.
	Type social.EndorsementType

	// Rating is the numeric rating (1-5).
	Rating float64

	// Comment is an optional comment.
	Comment string

	// IsPublic indicates if the endorsement should be public.
	IsPublic bool

	// CorrelationID for tracing.
	CorrelationID string
}

// Validate validates the command.
func (c GiveEndorsementCommand) Validate() error {
	if c.GiverID == "" {
		return errors.New("give_endorsement: giver_id is required")
	}
	if c.ReceiverID == "" {
		return errors.New("give_endorsement: receiver_id is required")
	}
	if c.GiverID == c.ReceiverID {
		return errors.New("give_endorsement: cannot endorse self")
	}
	if c.Rating < 1 || c.Rating > 5 {
		return errors.New("give_endorsement: rating must be between 1 and 5")
	}
	return nil
}

// GiveEndorsementResult contains the result of giving an endorsement.
type GiveEndorsementResult struct {
	// EndorsementID is the ID of the created endorsement.
	EndorsementID string

	// ReceiverNewRating is the receiver's new average rating.
	ReceiverNewRating float64

	// ReceiverTotalEndorsements is the receiver's total endorsement count.
	ReceiverTotalEndorsements int

	// Events contains domain events generated.
	Events []shared.Event

	// CreatedAt is when the endorsement was created.
	CreatedAt time.Time
}

// GiveEndorsementHandler handles the GiveEndorsementCommand.
type GiveEndorsementHandler struct {
	studentRepo    student.Repository
	socialRepo     social.Repository
	eventPublisher shared.EventPublisher
}

// NewGiveEndorsementHandler creates a new handler.
func NewGiveEndorsementHandler(
	studentRepo student.Repository,
	socialRepo social.Repository,
	eventPublisher shared.EventPublisher,
) *GiveEndorsementHandler {
	return &GiveEndorsementHandler{
		studentRepo:    studentRepo,
		socialRepo:     socialRepo,
		eventPublisher: eventPublisher,
	}
}

// Handle executes the give endorsement command.
func (h *GiveEndorsementHandler) Handle(
	ctx context.Context,
	cmd GiveEndorsementCommand,
) (*GiveEndorsementResult, error) {
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// Verify both students exist
	_, err := h.studentRepo.GetByID(ctx, cmd.GiverID)
	if err != nil {
		return nil, fmt.Errorf("give_endorsement: giver not found: %w", err)
	}

	receiver, err := h.studentRepo.GetByID(ctx, cmd.ReceiverID)
	if err != nil {
		return nil, fmt.Errorf("give_endorsement: receiver not found: %w", err)
	}

	// Create endorsement
	endorsementID := generateEndorsementID(cmd.GiverID, cmd.ReceiverID)

	endorsementType := cmd.Type
	if endorsementType == "" {
		endorsementType = social.EndorsementTypeClear
	}

	params := social.NewEndorsementParams{
		ID:            endorsementID,
		GiverID:       social.StudentID(cmd.GiverID),
		ReceiverID:    social.StudentID(cmd.ReceiverID),
		HelpRequestID: cmd.HelpRequestID,
		TaskID:        social.TaskID(cmd.TaskID),
		Type:          endorsementType,
		Rating:        social.Rating(cmd.Rating),
		Comment:       cmd.Comment,
		IsPublic:      cmd.IsPublic,
	}

	endorsement, err := social.NewEndorsement(params)
	if err != nil {
		return nil, fmt.Errorf("give_endorsement: failed to create: %w", err)
	}

	// Save endorsement
	if err := h.socialRepo.Endorsements().Create(ctx, endorsement); err != nil {
		return nil, fmt.Errorf("give_endorsement: failed to save: %w", err)
	}

	// Update receiver's rating
	if err := receiver.AddHelpRating(cmd.Rating); err != nil {
		// Log but don't fail
	}
	_ = h.studentRepo.Update(ctx, receiver)

	result := &GiveEndorsementResult{
		EndorsementID:             endorsementID,
		ReceiverNewRating:         receiver.HelpRating,
		ReceiverTotalEndorsements: receiver.HelpCount,
		CreatedAt:                 endorsement.CreatedAt,
		Events:                    make([]shared.Event, 0),
	}

	// Emit event
	event := shared.NewEndorsementGivenEvent(cmd.GiverID, cmd.ReceiverID, int(cmd.Rating))
	if cmd.TaskID != "" {
		event.TaskID = cmd.TaskID
	}
	event.Comment = cmd.Comment
	if cmd.CorrelationID != "" {
		event.BaseEvent = event.BaseEvent.WithCorrelationID(cmd.CorrelationID)
	}
	result.Events = append(result.Events, event)
	_ = h.eventPublisher.Publish(event)

	return result, nil
}

func generateEndorsementID(giverID, receiverID string) string {
	return fmt.Sprintf("endorsement_%s_%s_%d", giverID, receiverID, time.Now().UnixNano())
}
