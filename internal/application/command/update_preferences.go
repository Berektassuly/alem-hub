// Package command contains write operations (CQRS - Commands).
package command

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"context"
	"errors"
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// UPDATE PREFERENCES COMMAND
// Updates student's notification and privacy preferences.
// This gives students control over how they participate in the community.
// ══════════════════════════════════════════════════════════════════════════════

// UpdatePreferencesCommand contains the data to update preferences.
type UpdatePreferencesCommand struct {
	// StudentID is the ID of the student.
	StudentID string

	// Preferences contains the new preference values.
	// Only non-nil values will be updated.
	Preferences PreferenceUpdates

	// CorrelationID for tracing.
	CorrelationID string
}

// PreferenceUpdates contains optional preference updates.
// nil values mean "don't change".
type PreferenceUpdates struct {
	// RankChanges - notify about rank changes.
	RankChanges *bool

	// DailyDigest - send daily summary.
	DailyDigest *bool

	// HelpRequests - notify about help requests for completed tasks.
	HelpRequests *bool

	// InactivityReminders - send reminders when inactive.
	InactivityReminders *bool

	// QuietHoursStart - start of quiet hours (0-23).
	QuietHoursStart *int

	// QuietHoursEnd - end of quiet hours (0-23).
	QuietHoursEnd *int

	// IsOpenToHelp - whether the student is willing to help others.
	IsOpenToHelp *bool

	// IsProfilePublic - whether the profile is public.
	IsProfilePublic *bool

	// DisplayName - update display name.
	DisplayName *string
}

// Validate validates the command.
func (c UpdatePreferencesCommand) Validate() error {
	if c.StudentID == "" {
		return errors.New("update_preferences: student_id is required")
	}

	// Validate quiet hours if provided
	if c.Preferences.QuietHoursStart != nil {
		if *c.Preferences.QuietHoursStart < 0 || *c.Preferences.QuietHoursStart > 23 {
			return errors.New("update_preferences: quiet_hours_start must be 0-23")
		}
	}
	if c.Preferences.QuietHoursEnd != nil {
		if *c.Preferences.QuietHoursEnd < 0 || *c.Preferences.QuietHoursEnd > 23 {
			return errors.New("update_preferences: quiet_hours_end must be 0-23")
		}
	}

	// Validate display name if provided
	if c.Preferences.DisplayName != nil {
		name := *c.Preferences.DisplayName
		if len(name) < 1 || len(name) > 100 {
			return errors.New("update_preferences: display_name must be 1-100 characters")
		}
	}

	return nil
}

// UpdatePreferencesResult contains the result of updating preferences.
type UpdatePreferencesResult struct {
	// Success indicates if preferences were updated.
	Success bool

	// StudentID is the ID of the student.
	StudentID string

	// UpdatedPreferences contains the final preference values.
	UpdatedPreferences student.NotificationPreferences

	// ChangedFields lists which fields were changed.
	ChangedFields []string

	// UpdatedAt is when the preferences were updated.
	UpdatedAt time.Time
}

// ══════════════════════════════════════════════════════════════════════════════
// HANDLER
// ══════════════════════════════════════════════════════════════════════════════

// UpdatePreferencesHandler handles the UpdatePreferencesCommand.
type UpdatePreferencesHandler struct {
	studentRepo student.Repository
	cache       student.StudentCache // Optional cache for invalidation
}

// NewUpdatePreferencesHandler creates a new UpdatePreferencesHandler.
func NewUpdatePreferencesHandler(
	studentRepo student.Repository,
	cache student.StudentCache,
) *UpdatePreferencesHandler {
	return &UpdatePreferencesHandler{
		studentRepo: studentRepo,
		cache:       cache,
	}
}

// Handle executes the update preferences command.
func (h *UpdatePreferencesHandler) Handle(
	ctx context.Context,
	cmd UpdatePreferencesCommand,
) (*UpdatePreferencesResult, error) {
	// Validate command
	if err := cmd.Validate(); err != nil {
		return nil, fmt.Errorf("update_preferences: validation failed: %w", err)
	}

	// Get student
	stud, err := h.studentRepo.GetByID(ctx, cmd.StudentID)
	if err != nil {
		return nil, fmt.Errorf("update_preferences: student not found: %w", err)
	}

	// Track changes
	changedFields := make([]string, 0)
	prefs := stud.Preferences

	// Apply updates
	if cmd.Preferences.RankChanges != nil && *cmd.Preferences.RankChanges != prefs.RankChanges {
		prefs.RankChanges = *cmd.Preferences.RankChanges
		changedFields = append(changedFields, "rank_changes")
	}

	if cmd.Preferences.DailyDigest != nil && *cmd.Preferences.DailyDigest != prefs.DailyDigest {
		prefs.DailyDigest = *cmd.Preferences.DailyDigest
		changedFields = append(changedFields, "daily_digest")
	}

	if cmd.Preferences.HelpRequests != nil && *cmd.Preferences.HelpRequests != prefs.HelpRequests {
		prefs.HelpRequests = *cmd.Preferences.HelpRequests
		changedFields = append(changedFields, "help_requests")
	}

	if cmd.Preferences.InactivityReminders != nil && *cmd.Preferences.InactivityReminders != prefs.InactivityReminders {
		prefs.InactivityReminders = *cmd.Preferences.InactivityReminders
		changedFields = append(changedFields, "inactivity_reminders")
	}

	if cmd.Preferences.QuietHoursStart != nil && *cmd.Preferences.QuietHoursStart != prefs.QuietHoursStart {
		prefs.QuietHoursStart = *cmd.Preferences.QuietHoursStart
		changedFields = append(changedFields, "quiet_hours_start")
	}

	if cmd.Preferences.QuietHoursEnd != nil && *cmd.Preferences.QuietHoursEnd != prefs.QuietHoursEnd {
		prefs.QuietHoursEnd = *cmd.Preferences.QuietHoursEnd
		changedFields = append(changedFields, "quiet_hours_end")
	}

	// Update display name if provided
	if cmd.Preferences.DisplayName != nil && *cmd.Preferences.DisplayName != stud.DisplayName {
		stud.DisplayName = *cmd.Preferences.DisplayName
		changedFields = append(changedFields, "display_name")
	}

	// Apply preferences to student
	stud.UpdatePreferences(prefs)

	// Save changes only if something changed
	if len(changedFields) > 0 {
		if err := h.studentRepo.Update(ctx, stud); err != nil {
			return nil, fmt.Errorf("update_preferences: failed to save: %w", err)
		}

		// Invalidate cache if available
		if h.cache != nil {
			_ = h.cache.Invalidate(ctx, cmd.StudentID)
		}
	}

	return &UpdatePreferencesResult{
		Success:            true,
		StudentID:          cmd.StudentID,
		UpdatedPreferences: prefs,
		ChangedFields:      changedFields,
		UpdatedAt:          stud.UpdatedAt,
	}, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// PRESET PREFERENCES
// Helper commands for common preference presets.
// ══════════════════════════════════════════════════════════════════════════════

// EnableAllNotificationsCommand enables all notifications for a student.
type EnableAllNotificationsCommand struct {
	StudentID     string
	CorrelationID string
}

// DisableAllNotificationsCommand disables all notifications for a student.
type DisableAllNotificationsCommand struct {
	StudentID     string
	CorrelationID string
}

// SetQuietHoursCommand sets quiet hours for a student.
type SetQuietHoursCommand struct {
	StudentID     string
	StartHour     int // 0-23
	EndHour       int // 0-23
	CorrelationID string
}

// PresetPreferencesHandler handles preset preference commands.
type PresetPreferencesHandler struct {
	prefsHandler *UpdatePreferencesHandler
}

// NewPresetPreferencesHandler creates a new handler.
func NewPresetPreferencesHandler(prefsHandler *UpdatePreferencesHandler) *PresetPreferencesHandler {
	return &PresetPreferencesHandler{prefsHandler: prefsHandler}
}

// HandleEnableAll enables all notifications.
func (h *PresetPreferencesHandler) HandleEnableAll(
	ctx context.Context,
	cmd EnableAllNotificationsCommand,
) (*UpdatePreferencesResult, error) {
	t := true
	return h.prefsHandler.Handle(ctx, UpdatePreferencesCommand{
		StudentID:     cmd.StudentID,
		CorrelationID: cmd.CorrelationID,
		Preferences: PreferenceUpdates{
			RankChanges:         &t,
			DailyDigest:         &t,
			HelpRequests:        &t,
			InactivityReminders: &t,
		},
	})
}

// HandleDisableAll disables all notifications.
func (h *PresetPreferencesHandler) HandleDisableAll(
	ctx context.Context,
	cmd DisableAllNotificationsCommand,
) (*UpdatePreferencesResult, error) {
	f := false
	return h.prefsHandler.Handle(ctx, UpdatePreferencesCommand{
		StudentID:     cmd.StudentID,
		CorrelationID: cmd.CorrelationID,
		Preferences: PreferenceUpdates{
			RankChanges:         &f,
			DailyDigest:         &f,
			HelpRequests:        &f,
			InactivityReminders: &f,
		},
	})
}

// HandleSetQuietHours sets quiet hours.
func (h *PresetPreferencesHandler) HandleSetQuietHours(
	ctx context.Context,
	cmd SetQuietHoursCommand,
) (*UpdatePreferencesResult, error) {
	return h.prefsHandler.Handle(ctx, UpdatePreferencesCommand{
		StudentID:     cmd.StudentID,
		CorrelationID: cmd.CorrelationID,
		Preferences: PreferenceUpdates{
			QuietHoursStart: &cmd.StartHour,
			QuietHoursEnd:   &cmd.EndHour,
		},
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// TOGGLE HELP AVAILABILITY COMMAND
// Quick toggle for help availability.
// ══════════════════════════════════════════════════════════════════════════════

// ToggleHelpAvailabilityCommand toggles whether a student is available to help.
type ToggleHelpAvailabilityCommand struct {
	// StudentID is the ID of the student.
	StudentID string

	// IsAvailable is whether the student wants to be available to help.
	IsAvailable bool

	// CorrelationID for tracing.
	CorrelationID string
}

// ToggleHelpAvailabilityResult contains the result.
type ToggleHelpAvailabilityResult struct {
	// Success indicates if the toggle was successful.
	Success bool

	// StudentID is the ID of the student.
	StudentID string

	// IsAvailable is the new availability status.
	IsAvailable bool

	// UpdatedAt is when the change was made.
	UpdatedAt time.Time
}

// ToggleHelpAvailabilityHandler handles the command.
type ToggleHelpAvailabilityHandler struct {
	studentRepo student.Repository
	cache       student.StudentCache
}

// NewToggleHelpAvailabilityHandler creates a new handler.
func NewToggleHelpAvailabilityHandler(
	studentRepo student.Repository,
	cache student.StudentCache,
) *ToggleHelpAvailabilityHandler {
	return &ToggleHelpAvailabilityHandler{
		studentRepo: studentRepo,
		cache:       cache,
	}
}

// Handle executes the toggle command.
func (h *ToggleHelpAvailabilityHandler) Handle(
	ctx context.Context,
	cmd ToggleHelpAvailabilityCommand,
) (*ToggleHelpAvailabilityResult, error) {
	if cmd.StudentID == "" {
		return nil, errors.New("toggle_help: student_id is required")
	}

	// Get student
	stud, err := h.studentRepo.GetByID(ctx, cmd.StudentID)
	if err != nil {
		return nil, fmt.Errorf("toggle_help: student not found: %w", err)
	}

	// Update help requests preference
	prefs := stud.Preferences
	prefs.HelpRequests = cmd.IsAvailable
	stud.UpdatePreferences(prefs)

	// Save changes
	if err := h.studentRepo.Update(ctx, stud); err != nil {
		return nil, fmt.Errorf("toggle_help: failed to save: %w", err)
	}

	// Invalidate cache
	if h.cache != nil {
		_ = h.cache.Invalidate(ctx, cmd.StudentID)
	}

	return &ToggleHelpAvailabilityResult{
		Success:     true,
		StudentID:   cmd.StudentID,
		IsAvailable: cmd.IsAvailable,
		UpdatedAt:   stud.UpdatedAt,
	}, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// RESET PREFERENCES COMMAND
// Resets preferences to defaults.
// ══════════════════════════════════════════════════════════════════════════════

// ResetPreferencesCommand resets all preferences to defaults.
type ResetPreferencesCommand struct {
	// StudentID is the ID of the student.
	StudentID string

	// CorrelationID for tracing.
	CorrelationID string
}

// ResetPreferencesHandler handles the ResetPreferencesCommand.
type ResetPreferencesHandler struct {
	studentRepo student.Repository
	cache       student.StudentCache
}

// NewResetPreferencesHandler creates a new handler.
func NewResetPreferencesHandler(
	studentRepo student.Repository,
	cache student.StudentCache,
) *ResetPreferencesHandler {
	return &ResetPreferencesHandler{
		studentRepo: studentRepo,
		cache:       cache,
	}
}

// Handle executes the reset preferences command.
func (h *ResetPreferencesHandler) Handle(
	ctx context.Context,
	cmd ResetPreferencesCommand,
) (*UpdatePreferencesResult, error) {
	if cmd.StudentID == "" {
		return nil, errors.New("reset_preferences: student_id is required")
	}

	// Get student
	stud, err := h.studentRepo.GetByID(ctx, cmd.StudentID)
	if err != nil {
		return nil, fmt.Errorf("reset_preferences: student not found: %w", err)
	}

	// Reset to defaults
	defaultPrefs := student.DefaultNotificationPreferences()
	stud.UpdatePreferences(defaultPrefs)

	// Save changes
	if err := h.studentRepo.Update(ctx, stud); err != nil {
		return nil, fmt.Errorf("reset_preferences: failed to save: %w", err)
	}

	// Invalidate cache
	if h.cache != nil {
		_ = h.cache.Invalidate(ctx, cmd.StudentID)
	}

	return &UpdatePreferencesResult{
		Success:            true,
		StudentID:          cmd.StudentID,
		UpdatedPreferences: defaultPrefs,
		ChangedFields:      []string{"all_reset_to_defaults"},
		UpdatedAt:          stud.UpdatedAt,
	}, nil
}
