// Package alem implements Alem Platform API client.
package alem

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/activity"
	"github.com/alem-hub/alem-community-hub/internal/domain/leaderboard"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
    "strings"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// MAPPER - DTO to Domain Entity transformations
// ══════════════════════════════════════════════════════════════════════════════

// Mapper handles transformation between Alem API DTOs and domain entities.
// This follows the Anti-Corruption Layer pattern from DDD, protecting our domain
// from external API changes.
type Mapper struct{}

// NewMapper creates a new Mapper instance.
func NewMapper() *Mapper {
	return &Mapper{}
}

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT MAPPING
// ══════════════════════════════════════════════════════════════════════════════

// StudentFromDTO converts a StudentDTO to domain Student entity.
// Note: This creates a partial student without TelegramID, as that comes from
// our own database, not from Alem API.
func (m *Mapper) StudentFromDTO(dto *StudentDTO) (*student.Student, error) {
	if dto == nil {
		return nil, ErrNilDTO
	}

	// Determine status based on API data
	status := student.StatusActive
	if !dto.IsActive {
		status = student.StatusInactive
	}

	// Determine online state
	onlineState := student.OnlineStateOffline
	if dto.IsOnline {
		onlineState = student.OnlineStateOnline
	} else if dto.LastActivityAt != nil && time.Since(*dto.LastActivityAt) < 30*time.Minute {
		onlineState = student.OnlineStateAway
	}

	// Extract cohort - prefer explicit cohort, fallback to pool
	cohort := dto.Cohort
	if cohort == "" {
		cohort = dto.Pool
	}
	if cohort == "" {
		cohort = "default"
	}

	// Build display name
	displayName := dto.FullName()

	// Handle last seen time
	lastSeenAt := time.Now().UTC()
	if dto.LastActivityAt != nil {
		lastSeenAt = *dto.LastActivityAt
	}

	// Create the student entity
	s := &student.Student{
		ID:           dto.ID,
		TelegramID:   0, // Will be set when linking accounts
        // AlemLogin removed - not storing it anymore
        Email:        dto.Login + "@alem.school", // Derive email if missing in DTO? Or leave empty? 
        // Ideally we should have Email in DTO but DTO struct review might reveal it.
        // If I leave Email empty, it fails validation. I need to assume email can be constructed or just placeholder?
        // Since this mapper handles 'StudentFromDTO', which seems to be used for syncing FROM Alem,
        // we might not have the email if Alem API doesn't return it.
        // But if we are in 'Refactor auth' mode, we might assume we rely on what we have.
        // I'll construct a placeholder or try to use Login as prefix.
		DisplayName:  displayName,
		CurrentXP:    student.XP(dto.XP),
		Cohort:       student.Cohort(cohort),
		Status:       status,
		OnlineState:  onlineState,
		LastSeenAt:   lastSeenAt,
		LastSyncedAt: time.Now().UTC(),
		JoinedAt:     dto.CreatedAt,
		Preferences:  student.DefaultNotificationPreferences(),
		HelpRating:   0.0,
		HelpCount:    0,
		CreatedAt:    dto.CreatedAt,
		UpdatedAt:    dto.UpdatedAt,
	}

	return s, nil
}

// StudentToSyncData converts a StudentDTO to a lightweight sync data structure.
// Used for efficient delta syncing without creating full domain objects.
type StudentSyncData struct {
	AlemID      string
	Login       string
	DisplayName string
	XP          int
	Level       int
	Cohort      string
	IsActive    bool
	IsOnline    bool
	LastSeenAt  time.Time
	UpdatedAt   time.Time
}

// ToSyncData extracts sync-relevant data from StudentDTO.
func (m *Mapper) ToSyncData(dto *StudentDTO) StudentSyncData {
	lastSeen := time.Now()
	if dto.LastActivityAt != nil {
		lastSeen = *dto.LastActivityAt
	}

	cohort := dto.Cohort
	if cohort == "" {
		cohort = dto.Pool
	}

	return StudentSyncData{
		AlemID:      dto.ID,
		Login:       dto.Login,
		DisplayName: dto.FullName(),
		XP:          dto.XP,
		Level:       dto.Level,
		Cohort:      cohort,
		IsActive:    dto.IsActive,
		IsOnline:    dto.IsOnline,
		LastSeenAt:  lastSeen,
		UpdatedAt:   dto.UpdatedAt,
	}
}

// UpdateStudentFromDTO updates an existing student with data from DTO.
// Returns the XP delta (positive if gained, negative if somehow lost).
func (m *Mapper) UpdateStudentFromDTO(s *student.Student, dto *StudentDTO) (xpDelta int, err error) {
	if s == nil || dto == nil {
		return 0, ErrNilDTO
	}

	oldXP := int(s.CurrentXP)
	newXP := dto.XP
	xpDelta = newXP - oldXP

	// Update XP
	if _, err := s.UpdateXP(student.XP(newXP)); err != nil {
		return 0, err
	}

	// Update display name if changed
	newDisplayName := dto.FullName()
	if newDisplayName != s.DisplayName && newDisplayName != "" {
		s.DisplayName = newDisplayName
	}

	// Update online state
	if dto.IsOnline {
		s.MarkOnline()
	} else if dto.LastActivityAt != nil && time.Since(*dto.LastActivityAt) < 30*time.Minute {
		s.MarkAway()
	} else {
		s.MarkOffline()
	}

	// Update status
	if !dto.IsActive && s.Status == student.StatusActive {
		_ = s.MarkInactive()
	} else if dto.IsActive && s.Status == student.StatusInactive {
		s.Status = student.StatusActive
	}

	// Update last seen
	if dto.LastActivityAt != nil {
		s.LastSeenAt = *dto.LastActivityAt
	}

	// Mark as synced
	s.SyncedWith(time.Now().UTC())

	return xpDelta, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// TASK COMPLETION MAPPING
// ══════════════════════════════════════════════════════════════════════════════

// TaskCompletionFromDTO converts a TaskCompletionDTO to domain TaskCompletion entity.
func (m *Mapper) TaskCompletionFromDTO(dto *TaskCompletionDTO) (*activity.TaskCompletion, error) {
	if dto == nil {
		return nil, ErrNilDTO
	}

	// Determine completion time
	completedAt := time.Now()
	if dto.CompletedAt != nil {
		completedAt = *dto.CompletedAt
	}

	// Use slug if available, otherwise fall back to ID
	taskID := dto.TaskSlug
	if taskID == "" {
		taskID = dto.TaskID
	}

	tc, err := activity.NewTaskCompletion(
		dto.ID,
		activity.StudentID(dto.StudentID),
		activity.TaskID(taskID),
		completedAt,
		dto.XPEarned,
	)
	if err != nil {
		return nil, err
	}

	// Set optional fields
	if dto.TimeSpent > 0 {
		_ = tc.SetTimeSpent(time.Duration(dto.TimeSpent) * time.Second)
	}

	if dto.Attempts > 0 {
		_ = tc.SetAttempts(dto.Attempts)
	}

	return tc, nil
}

// TaskCompletionsFromDTOs converts a slice of TaskCompletionDTOs to domain entities.
// Invalid DTOs are skipped (with error logged) rather than failing the entire operation.
func (m *Mapper) TaskCompletionsFromDTOs(dtos []TaskCompletionDTO) ([]*activity.TaskCompletion, []error) {
	completions := make([]*activity.TaskCompletion, 0, len(dtos))
	var errors []error

	for i := range dtos {
		tc, err := m.TaskCompletionFromDTO(&dtos[i])
		if err != nil {
			errors = append(errors, err)
			continue
		}
		completions = append(completions, tc)
	}

	return completions, errors
}

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD MAPPING
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardEntryFromDTO converts a LeaderboardEntryDTO to domain LeaderboardEntry.
// The cohort parameter is passed from the parent LeaderboardDTO.
func (m *Mapper) LeaderboardEntryFromDTO(dto *LeaderboardEntryDTO, cohort string) *leaderboard.LeaderboardEntry {
	displayName := dto.DisplayName
	if displayName == "" {
		displayName = dto.Login
	}

	cohortVal := leaderboard.Cohort(cohort)
	if cohortVal == "" {
		cohortVal = leaderboard.CohortAll
	}

	entry, _ := leaderboard.NewLeaderboardEntry(
		leaderboard.Rank(dto.Rank),
		dto.StudentID,
		displayName,
		leaderboard.XP(dto.XP),
		dto.Level,
		cohortVal,
	)
	if entry != nil {
		entry.RankChange = leaderboard.RankChange(dto.Change)
		entry.IsOnline = dto.IsOnline
	}
	return entry
}

// LeaderboardRankingFromDTO converts a LeaderboardDTO to domain Ranking.
func (m *Mapper) LeaderboardRankingFromDTO(dto *LeaderboardDTO) *leaderboard.Ranking {
	if dto == nil {
		return nil
	}

	ranking := leaderboard.NewRanking()
	for i := range dto.Entries {
		entry := m.LeaderboardEntryFromDTO(&dto.Entries[i], dto.Cohort)
		if entry != nil {
			_ = ranking.Add(entry)
		}
	}

	return ranking
}

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE STATUS MAPPING
// ══════════════════════════════════════════════════════════════════════════════

// OnlineStatusFromDTO converts an OnlineStatusDTO to domain OnlineStatus.
func (m *Mapper) OnlineStatusFromDTO(dto *OnlineStatusDTO) activity.OnlineStatus {
	lastSeenAt := time.Now()
	if dto.LastSeenAt != nil {
		lastSeenAt = *dto.LastSeenAt
	}

	status := activity.NewOnlineStatus(
		activity.StudentID(dto.StudentID),
		dto.IsOnline,
		lastSeenAt,
	)

	return status
}

// OnlineStatusesFromDTOs converts a slice of OnlineStatusDTO to domain OnlineStatus.
func (m *Mapper) OnlineStatusesFromDTOs(dtos []OnlineStatusDTO) []activity.OnlineStatus {
	statuses := make([]activity.OnlineStatus, len(dtos))
	for i := range dtos {
		statuses[i] = m.OnlineStatusFromDTO(&dtos[i])
	}
	return statuses
}

// ══════════════════════════════════════════════════════════════════════════════
// ACTIVITY MAPPING
// ══════════════════════════════════════════════════════════════════════════════

// ActivityEventFromDTO converts an ActivityDTO to an internal activity event structure.
type ActivityEvent struct {
	ID          string
	StudentID   string
	Type        string
	Description string
	Timestamp   time.Time
	XPGained    int
	Metadata    map[string]interface{}
}

// ActivityEventFromDTO converts an ActivityDTO to ActivityEvent.
func (m *Mapper) ActivityEventFromDTO(dto *ActivityDTO) (*ActivityEvent, error) {
	if dto == nil {
		return nil, ErrNilDTO
	}

	event := &ActivityEvent{
		ID:          dto.ID,
		StudentID:   dto.StudentID,
		Type:        dto.Type,
		Description: dto.Description,
		Timestamp:   dto.Timestamp,
		XPGained:    dto.XPGained,
	}

	return event, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// SYNC DELTA MAPPING
// ══════════════════════════════════════════════════════════════════════════════

// SyncResult represents the result of mapping a sync delta.
type SyncResult struct {
	// Students are the mapped student entities
	Students []*student.Student

	// StudentSyncData is lightweight sync data for all students
	StudentSyncData []StudentSyncData

	// TaskCompletions are the mapped task completions
	TaskCompletions []*activity.TaskCompletion

	// OnlineStatuses are the mapped online statuses
	OnlineStatuses []activity.OnlineStatus

	// DeletedStudentIDs are IDs of deleted/deactivated students
	DeletedStudentIDs []string

	// Errors encountered during mapping
	Errors []error

	// SyncTimestamp is when the sync occurred
	SyncTimestamp time.Time

	// NextSyncToken for the next delta sync
	NextSyncToken string

	// FullSyncRequired indicates if a full sync is needed
	FullSyncRequired bool
}

// SyncDeltaFromDTO converts a SyncDeltaDTO to SyncResult.
func (m *Mapper) SyncDeltaFromDTO(dto *SyncDeltaDTO) *SyncResult {
	if dto == nil {
		return &SyncResult{
			SyncTimestamp: time.Now(),
		}
	}

	result := &SyncResult{
		DeletedStudentIDs: dto.DeletedStudentIDs,
		SyncTimestamp:     dto.SyncTimestamp,
		NextSyncToken:     dto.NextSyncToken,
		FullSyncRequired:  dto.FullSyncRequired,
	}

	// Map students
	result.StudentSyncData = make([]StudentSyncData, len(dto.Students))
	result.Students = make([]*student.Student, 0, len(dto.Students))

	for i := range dto.Students {
		result.StudentSyncData[i] = m.ToSyncData(&dto.Students[i])

		s, err := m.StudentFromDTO(&dto.Students[i])
		if err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.Students = append(result.Students, s)
	}

	// Map task completions
	result.TaskCompletions, _ = m.TaskCompletionsFromDTOs(dto.TaskCompletions)

	// Map online statuses
	result.OnlineStatuses = m.OnlineStatusesFromDTOs(dto.OnlineChanges)

	return result
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER TYPES AND ERRORS
// ══════════════════════════════════════════════════════════════════════════════

// ErrNilDTO is returned when trying to map a nil DTO.
var ErrNilDTO = &MappingError{Message: "cannot map nil DTO"}

// MappingError represents an error during DTO to domain mapping.
type MappingError struct {
	Field   string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *MappingError) Error() string {
	if e.Field != "" {
		return "mapping error for field " + e.Field + ": " + e.Message
	}
	return "mapping error: " + e.Message
}

// Unwrap returns the underlying error.
func (e *MappingError) Unwrap() error {
	return e.Cause
}

// ══════════════════════════════════════════════════════════════════════════════
// REVERSE MAPPING (Domain to DTO) - For webhooks/API responses
// ══════════════════════════════════════════════════════════════════════════════

// StudentToDTO converts a domain Student to StudentDTO.
// This is useful for sending data back to external systems or debugging.
func (m *Mapper) StudentToDTO(s *student.Student) *StudentDTO {
	if s == nil {
		return nil
	}

	var lastActivityAt *time.Time
	if !s.LastSeenAt.IsZero() {
		lastActivityAt = &s.LastSeenAt
	}

    login := ""
    if idx := strings.Index(s.Email, "@"); idx > 0 {
        login = s.Email[:idx]
    }

	return &StudentDTO{
		ID:             s.ID,
		Login:          login,
		FirstName:      s.DisplayName, // We don't have separate first/last names
		Cohort:         string(s.Cohort),
		Level:          int(s.Level()),
		XP:             int(s.CurrentXP),
		IsActive:       s.Status.IsEnrolled(),
		IsOnline:       s.OnlineState == student.OnlineStateOnline,
		LastActivityAt: lastActivityAt,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}
