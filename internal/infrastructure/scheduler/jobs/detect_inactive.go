// Package jobs contains implementations of scheduled jobs for Alem Community Hub.
package jobs

import (
	"alem-hub/internal/domain/notification"
	"alem-hub/internal/domain/shared"
	"alem-hub/internal/domain/social"
	"alem-hub/internal/domain/student"
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// DETECT INACTIVE JOB
// ══════════════════════════════════════════════════════════════════════════════

// DetectInactiveJob finds students who have been inactive for too long
// and triggers re-engagement actions.
//
// Philosophy: "From Competition to Collaboration"
// - We don't just notify inactive students, we help them get back on track
// - We reach out to their study buddies to check on them
// - We provide personalized encouragement based on their progress
//
// This job embodies the core mission: reducing the 70% dropout rate by
// creating a support network that catches students before they fall behind.
type DetectInactiveJob struct {
	// Dependencies
	studentRepo      student.Repository
	socialRepo       social.SocialRepository
	notificationSvc  notification.NotificationService
	notificationRepo notification.NotificationRepository
	eventPublisher   shared.EventPublisher
	logger           *slog.Logger

	// Configuration
	config DetectInactiveConfig

	// State
	lastRunStats atomic.Value // *DetectInactiveStats
}

// DetectInactiveConfig contains configuration for the detect inactive job.
type DetectInactiveConfig struct {
	// InactivityThresholds defines inactivity levels and their actions.
	// Key: days of inactivity, Value: action type
	InactivityThresholds map[int]InactivityAction

	// EnableNotifications enables sending notifications to inactive students.
	EnableNotifications bool

	// NotifyStudyBuddies enables notifying study buddies about inactive friends.
	NotifyStudyBuddies bool

	// StudyBuddyNotificationDelay is the days after which to notify study buddies.
	StudyBuddyNotificationDelay int

	// MaxNotificationsPerStudent limits how often we notify the same student.
	MaxNotificationsPerStudent int

	// NotificationCooldownDays is the minimum days between notifications.
	NotificationCooldownDays int

	// MarkInactiveAfterDays marks students as inactive after N days.
	MarkInactiveAfterDays int

	// Timeout is the maximum duration for the job.
	Timeout time.Duration
}

// InactivityAction defines what action to take for inactive students.
type InactivityAction string

const (
	// ActionGentleReminder sends a friendly reminder.
	ActionGentleReminder InactivityAction = "gentle_reminder"

	// ActionEncouragement sends an encouraging message with progress summary.
	ActionEncouragement InactivityAction = "encouragement"

	// ActionNotifyStudyBuddy notifies the student's study buddy.
	ActionNotifyStudyBuddy InactivityAction = "notify_study_buddy"

	// ActionUrgentOutreach sends an urgent message and notifies multiple connections.
	ActionUrgentOutreach InactivityAction = "urgent_outreach"

	// ActionMarkInactive marks the student as inactive in the system.
	ActionMarkInactive InactivityAction = "mark_inactive"
)

// DefaultDetectInactiveConfig returns sensible defaults.
func DefaultDetectInactiveConfig() DetectInactiveConfig {
	return DetectInactiveConfig{
		InactivityThresholds: map[int]InactivityAction{
			3:  ActionGentleReminder,   // 3 days: gentle reminder
			5:  ActionEncouragement,    // 5 days: encouragement with progress
			7:  ActionNotifyStudyBuddy, // 7 days: notify study buddy
			14: ActionUrgentOutreach,   // 14 days: urgent outreach
			21: ActionMarkInactive,     // 21 days: mark as inactive
		},
		EnableNotifications:         true,
		NotifyStudyBuddies:          true,
		StudyBuddyNotificationDelay: 7,
		MaxNotificationsPerStudent:  5,
		NotificationCooldownDays:    2,
		MarkInactiveAfterDays:       21,
		Timeout:                     5 * time.Minute,
	}
}

// DetectInactiveStats contains statistics from a detection run.
type DetectInactiveStats struct {
	StartedAt              time.Time
	CompletedAt            time.Time
	Duration               time.Duration
	TotalStudentsChecked   int
	InactiveStudentsFound  int
	NotificationsSent      int
	StudyBuddiesNotified   int
	StudentsMarkedInactive int
	StudentsReactivated    int // Students who came back
	SkippedDueToCooldown   int
	ActionsByType          map[InactivityAction]int
	Errors                 []error
}

// InactiveStudentInfo contains information about an inactive student.
type InactiveStudentInfo struct {
	Student           *student.Student
	DaysInactive      int
	LastActivity      time.Time
	Progress          *student.DailyGrind
	StudyBuddies      []string
	RecommendedAction InactivityAction
	AlreadyNotified   bool
	CooldownRemaining time.Duration
}

// NewDetectInactiveJob creates a new detect inactive job.
func NewDetectInactiveJob(
	studentRepo student.Repository,
	socialRepo social.SocialRepository,
	notificationSvc notification.NotificationService,
	notificationRepo notification.NotificationRepository,
	eventPublisher shared.EventPublisher,
	logger *slog.Logger,
	config DetectInactiveConfig,
) *DetectInactiveJob {
	if logger == nil {
		logger = slog.Default()
	}

	return &DetectInactiveJob{
		studentRepo:      studentRepo,
		socialRepo:       socialRepo,
		notificationSvc:  notificationSvc,
		notificationRepo: notificationRepo,
		eventPublisher:   eventPublisher,
		logger:           logger,
		config:           config,
	}
}

// Name returns the job name.
func (j *DetectInactiveJob) Name() string {
	return "detect_inactive"
}

// Description returns a human-readable description.
func (j *DetectInactiveJob) Description() string {
	return "Detects inactive students and triggers re-engagement actions"
}

// Run executes the detection job.
func (j *DetectInactiveJob) Run(ctx context.Context) error {
	startedAt := time.Now()
	stats := &DetectInactiveStats{
		StartedAt:     startedAt,
		ActionsByType: make(map[InactivityAction]int),
		Errors:        make([]error, 0),
	}

	j.logger.Info("starting detect_inactive job")

	// Apply timeout
	if j.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, j.config.Timeout)
		defer cancel()
	}

	// Find all inactive students at various thresholds
	inactiveStudents, err := j.findInactiveStudents(ctx, stats)
	if err != nil {
		return fmt.Errorf("failed to find inactive students: %w", err)
	}

	j.logger.Info("found inactive students",
		"total_checked", stats.TotalStudentsChecked,
		"inactive_found", len(inactiveStudents),
	)

	// Process each inactive student
	for _, info := range inactiveStudents {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := j.processInactiveStudent(ctx, info, stats); err != nil {
			stats.Errors = append(stats.Errors, err)
			j.logger.Error("failed to process inactive student",
				"student_id", info.Student.ID,
				"error", err,
			)
		}
	}

	// Finalize stats
	stats.CompletedAt = time.Now()
	stats.Duration = stats.CompletedAt.Sub(startedAt)
	j.lastRunStats.Store(stats)

	j.logger.Info("detect_inactive job completed",
		"duration", stats.Duration.String(),
		"inactive_found", stats.InactiveStudentsFound,
		"notifications_sent", stats.NotificationsSent,
		"buddies_notified", stats.StudyBuddiesNotified,
		"marked_inactive", stats.StudentsMarkedInactive,
	)

	return nil
}

// findInactiveStudents finds students who haven't been active recently.
func (j *DetectInactiveJob) findInactiveStudents(ctx context.Context, stats *DetectInactiveStats) ([]*InactiveStudentInfo, error) {
	// Get all enrolled students
	opts := student.DefaultListOptions().WithInactive()
	allStudents, err := j.studentRepo.GetAll(ctx, opts)
	if err != nil {
		return nil, err
	}

	stats.TotalStudentsChecked = len(allStudents)
	inactiveStudents := make([]*InactiveStudentInfo, 0)
	now := time.Now()

	for _, s := range allStudents {
		// Skip students who aren't enrolled
		if !s.Status.IsEnrolled() {
			continue
		}

		// Calculate days since last activity
		daysSinceLastSeen := int(now.Sub(s.LastSeenAt).Hours() / 24)

		// Skip recently active students
		if daysSinceLastSeen < 3 { // Minimum threshold
			continue
		}

		// Determine the appropriate action based on inactivity level
		action := j.determineAction(daysSinceLastSeen)
		if action == "" {
			continue
		}

		// Check notification cooldown
		alreadyNotified, cooldownRemaining := j.checkNotificationCooldown(ctx, s.ID)

		info := &InactiveStudentInfo{
			Student:           s,
			DaysInactive:      daysSinceLastSeen,
			LastActivity:      s.LastSeenAt,
			RecommendedAction: action,
			AlreadyNotified:   alreadyNotified,
			CooldownRemaining: cooldownRemaining,
		}

		// Get study buddies for potential outreach
		if action == ActionNotifyStudyBuddy || action == ActionUrgentOutreach {
			buddies, _ := j.getStudyBuddies(ctx, s.ID)
			info.StudyBuddies = buddies
		}

		inactiveStudents = append(inactiveStudents, info)
		stats.InactiveStudentsFound++
	}

	return inactiveStudents, nil
}

// determineAction determines what action to take based on days inactive.
func (j *DetectInactiveJob) determineAction(daysInactive int) InactivityAction {
	var selectedAction InactivityAction
	var selectedThreshold int

	for threshold, action := range j.config.InactivityThresholds {
		if daysInactive >= threshold && threshold > selectedThreshold {
			selectedThreshold = threshold
			selectedAction = action
		}
	}

	return selectedAction
}

// processInactiveStudent processes a single inactive student.
func (j *DetectInactiveJob) processInactiveStudent(
	ctx context.Context,
	info *InactiveStudentInfo,
	stats *DetectInactiveStats,
) error {
	stats.ActionsByType[info.RecommendedAction]++

	switch info.RecommendedAction {
	case ActionGentleReminder:
		return j.sendGentleReminder(ctx, info, stats)

	case ActionEncouragement:
		return j.sendEncouragement(ctx, info, stats)

	case ActionNotifyStudyBuddy:
		return j.notifyStudyBuddies(ctx, info, stats)

	case ActionUrgentOutreach:
		return j.urgentOutreach(ctx, info, stats)

	case ActionMarkInactive:
		return j.markStudentInactive(ctx, info, stats)

	default:
		return nil
	}
}

// sendGentleReminder sends a gentle reminder to the inactive student.
func (j *DetectInactiveJob) sendGentleReminder(
	ctx context.Context,
	info *InactiveStudentInfo,
	stats *DetectInactiveStats,
) error {
	if !j.config.EnableNotifications {
		return nil
	}

	// Check cooldown
	if info.AlreadyNotified && info.CooldownRemaining > 0 {
		stats.SkippedDueToCooldown++
		return nil
	}

	// Check if student allows inactivity reminders
	if !info.Student.Preferences.InactivityReminders {
		return nil
	}

	// Create and send notification
	n := j.createInactivityNotification(info, notification.TypeInactivityReminder)
	n.Message = fmt.Sprintf(
		"Привет, %s! 👋\n\n"+
			"Мы заметили, что тебя не было %d дней. "+
			"Надеемся, у тебя всё хорошо!\n\n"+
			"Помни: маленькие шаги каждый день приводят к большим результатам. "+
			"Может, сегодня хороший день, чтобы решить одну задачку? 💪",
		info.Student.DisplayName,
		info.DaysInactive,
	)

	if err := j.sendNotification(ctx, n); err != nil {
		return err
	}

	stats.NotificationsSent++

	// Emit event
	event := shared.NewStudentInactiveEvent(
		info.Student.ID,
		info.DaysInactive,
		info.LastActivity,
	)
	_ = j.eventPublisher.Publish(event)

	return nil
}

// sendEncouragement sends an encouraging message with progress summary.
func (j *DetectInactiveJob) sendEncouragement(
	ctx context.Context,
	info *InactiveStudentInfo,
	stats *DetectInactiveStats,
) error {
	if !j.config.EnableNotifications {
		return nil
	}

	if info.AlreadyNotified && info.CooldownRemaining > 0 {
		stats.SkippedDueToCooldown++
		return nil
	}

	if !info.Student.Preferences.InactivityReminders {
		return nil
	}

	// Create encouraging notification with progress info
	n := j.createInactivityNotification(info, notification.TypeInactivityReminder)
	n.Message = fmt.Sprintf(
		"Привет, %s! 🌟\n\n"+
			"Прошло уже %d дней с твоего последнего визита. "+
			"У тебя уже есть %d XP — это отличный прогресс!\n\n"+
			"Знаешь что? Многие студенты сейчас онлайн и готовы помочь. "+
			"Может, это хороший момент вернуться и продолжить путь? 🚀\n\n"+
			"Мы верим в тебя!",
		info.Student.DisplayName,
		info.DaysInactive,
		info.Student.CurrentXP,
	)

	if err := j.sendNotification(ctx, n); err != nil {
		return err
	}

	stats.NotificationsSent++
	return nil
}

// notifyStudyBuddies notifies the student's study buddies about their absence.
func (j *DetectInactiveJob) notifyStudyBuddies(
	ctx context.Context,
	info *InactiveStudentInfo,
	stats *DetectInactiveStats,
) error {
	// First, send notification to the student themselves
	if j.config.EnableNotifications {
		if err := j.sendEncouragement(ctx, info, stats); err != nil {
			j.logger.Warn("failed to send encouragement", "error", err)
		}
	}

	// Then notify study buddies
	if !j.config.NotifyStudyBuddies || len(info.StudyBuddies) == 0 {
		return nil
	}

	for _, buddyID := range info.StudyBuddies {
		buddy, err := j.studentRepo.GetByID(ctx, buddyID)
		if err != nil {
			continue
		}

		// Create notification for study buddy
		n := &notification.Notification{
			RecipientID:    notification.RecipientID(buddy.ID),
			TelegramChatID: int64(buddy.TelegramID),
			Type:           notification.TypeInactivityReminder,
			Priority:       notification.PriorityNormal,
			Status:         notification.StatusPending,
			Message: fmt.Sprintf(
				"Привет, %s! 💙\n\n"+
					"Твой друг %s не заходил уже %d дней. "+
					"Может, напишешь ему/ей? Иногда одно сообщение поддержки "+
					"может всё изменить.\n\n"+
					"Вместе мы сильнее! 🤝",
				buddy.DisplayName,
				info.Student.DisplayName,
				info.DaysInactive,
			),
			CreatedAt: time.Now(),
		}

		if err := j.sendNotification(ctx, n); err != nil {
			j.logger.Warn("failed to notify study buddy",
				"buddy_id", buddyID,
				"error", err,
			)
			continue
		}

		stats.StudyBuddiesNotified++
	}

	return nil
}

// urgentOutreach performs urgent outreach for very inactive students.
func (j *DetectInactiveJob) urgentOutreach(
	ctx context.Context,
	info *InactiveStudentInfo,
	stats *DetectInactiveStats,
) error {
	// Notify the student with urgency
	if j.config.EnableNotifications && !info.AlreadyNotified {
		n := j.createInactivityNotification(info, notification.TypeInactivityReminder)
		n.Priority = notification.PriorityHigh
		n.Message = fmt.Sprintf(
			"Привет, %s! ❤️\n\n"+
				"Мы очень скучаем! Тебя не было уже %d дней.\n\n"+
				"Мы понимаем, что иногда жизнь подкидывает сюрпризы. "+
				"Если тебе нужна помощь или поддержка — мы рядом.\n\n"+
				"Помни: никогда не поздно вернуться. "+
				"Каждый день — это новый шанс! 🌅",
			info.Student.DisplayName,
			info.DaysInactive,
		)

		if err := j.sendNotification(ctx, n); err == nil {
			stats.NotificationsSent++
		}
	}

	// Notify all study buddies
	for _, buddyID := range info.StudyBuddies {
		buddy, err := j.studentRepo.GetByID(ctx, buddyID)
		if err != nil {
			continue
		}

		n := &notification.Notification{
			RecipientID:    notification.RecipientID(buddy.ID),
			TelegramChatID: int64(buddy.TelegramID),
			Type:           notification.TypeInactivityReminder,
			Priority:       notification.PriorityHigh,
			Status:         notification.StatusPending,
			Message: fmt.Sprintf(
				"⚠️ %s, обрати внимание!\n\n"+
					"Твой друг %s не появлялся уже %d дней. "+
					"Это довольно долго.\n\n"+
					"Если у вас есть связь вне платформы — "+
					"может, стоит написать и узнать, всё ли в порядке?\n\n"+
					"Твоя поддержка может многое значить! 💪",
				buddy.DisplayName,
				info.Student.DisplayName,
				info.DaysInactive,
			),
			CreatedAt: time.Now(),
		}

		if err := j.sendNotification(ctx, n); err == nil {
			stats.StudyBuddiesNotified++
		}
	}

	return nil
}

// markStudentInactive marks a student as inactive in the system.
func (j *DetectInactiveJob) markStudentInactive(
	ctx context.Context,
	info *InactiveStudentInfo,
	stats *DetectInactiveStats,
) error {
	// Only mark as inactive if not already
	if info.Student.Status == student.StatusInactive {
		return nil
	}

	if err := info.Student.MarkInactive(); err != nil {
		return err
	}

	if err := j.studentRepo.Update(ctx, info.Student); err != nil {
		return fmt.Errorf("failed to update student status: %w", err)
	}

	stats.StudentsMarkedInactive++

	j.logger.Info("student marked as inactive",
		"student_id", info.Student.ID,
		"days_inactive", info.DaysInactive,
	)

	// Send final notification
	if j.config.EnableNotifications {
		n := j.createInactivityNotification(info, notification.TypeInactivityReminder)
		n.Message = fmt.Sprintf(
			"Привет, %s 🙁\n\n"+
				"К сожалению, мы вынуждены отметить тебя как неактивного "+
				"после %d дней отсутствия.\n\n"+
				"Но двери всегда открыты! 🚪\n"+
				"Если захочешь вернуться — просто напиши /start, "+
				"и мы снова будем рады тебя видеть.\n\n"+
				"Удачи тебе во всём! 🍀",
			info.Student.DisplayName,
			info.DaysInactive,
		)
		_ = j.sendNotification(ctx, n)
	}

	return nil
}

// Helper methods

func (j *DetectInactiveJob) createInactivityNotification(
	info *InactiveStudentInfo,
	notifType notification.NotificationType,
) *notification.Notification {
	return &notification.Notification{
		RecipientID:    notification.RecipientID(info.Student.ID),
		TelegramChatID: int64(info.Student.TelegramID),
		Type:           notifType,
		Priority:       notification.PriorityNormal,
		Status:         notification.StatusPending,
		CreatedAt:      time.Now(),
		Data: notification.NotificationData{
			"student_id":    info.Student.ID,
			"days_inactive": info.DaysInactive,
			"action":        string(info.RecommendedAction),
		},
	}
}

func (j *DetectInactiveJob) sendNotification(ctx context.Context, n *notification.Notification) error {
	// TODO: Implement actual notification sending through notification service
	// For now, we just log it
	j.logger.Info("sending notification",
		"recipient", n.RecipientID,
		"type", n.Type,
	)
	return nil
}

func (j *DetectInactiveJob) checkNotificationCooldown(ctx context.Context, studentID string) (bool, time.Duration) {
	// Check when we last notified this student about inactivity
	cooldownDays := j.config.NotificationCooldownDays
	if cooldownDays <= 0 {
		return false, 0
	}

	since := time.Now().AddDate(0, 0, -cooldownDays)
	count, err := j.notificationRepo.CountByRecipient(ctx, notification.RecipientID(studentID), since)
	if err != nil {
		return false, 0
	}

	if count >= j.config.MaxNotificationsPerStudent {
		// In cooldown
		return true, time.Duration(cooldownDays) * 24 * time.Hour
	}

	return false, 0
}

func (j *DetectInactiveJob) getStudyBuddies(ctx context.Context, studentID string) ([]string, error) {
	connections, err := j.socialRepo.GetConnections(ctx, studentID)
	if err != nil {
		return nil, err
	}

	buddyIDs := make([]string, 0, len(connections))
	for _, conn := range connections {
		// Get the other party in the connection
		if conn.StudentAID == studentID {
			buddyIDs = append(buddyIDs, conn.StudentBID)
		} else {
			buddyIDs = append(buddyIDs, conn.StudentAID)
		}
	}

	return buddyIDs, nil
}

// LastRunStats returns statistics from the last detection run.
func (j *DetectInactiveJob) LastRunStats() *DetectInactiveStats {
	stats := j.lastRunStats.Load()
	if stats == nil {
		return nil
	}
	return stats.(*DetectInactiveStats)
}
