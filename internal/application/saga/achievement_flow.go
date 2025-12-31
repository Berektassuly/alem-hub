// Package saga contains complex business processes that orchestrate
// multiple domain operations in a coordinated manner.
package saga

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/leaderboard"
	"github.com/alem-hub/alem-community-hub/internal/domain/notification"
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"context"
	"errors"
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// ACHIEVEMENT FLOW SAGA
// Complex business process: Achievement unlocking and notification
// Flow: Check Conditions → Validate Not Already Unlocked → Grant Achievement →
//
//	Award XP Bonus → Send Notification → Update Statistics → Publish Event
//
// Philosophy: Achievements celebrate progress and encourage collaboration.
// They are designed to motivate students and recognize both individual
// progress and community contributions.
// ══════════════════════════════════════════════════════════════════════════════

// AchievementCheckInput contains data needed to check for new achievements.
type AchievementCheckInput struct {
	// StudentID - the student to check achievements for.
	StudentID string

	// TriggerEvent - what triggered this check (e.g., "xp_gained", "task_completed").
	TriggerEvent string

	// Context - additional context for achievement checking.
	Context AchievementContext
}

// AchievementContext contains contextual data for achievement evaluation.
type AchievementContext struct {
	// NewXP - current XP after the triggering event.
	NewXP int

	// NewLevel - current level after the triggering event.
	NewLevel int

	// CurrentRank - current position in leaderboard.
	CurrentRank int

	// CurrentStreak - current daily streak.
	CurrentStreak int

	// TasksCompleted - total tasks completed.
	TasksCompleted int

	// HelpCount - number of times helped other students.
	HelpCount int

	// DaysInactive - days since last activity (for comeback achievement).
	DaysInactive int

	// TaskID - ID of the completed task (if applicable).
	TaskID string

	// Timestamp - when the triggering event occurred.
	Timestamp time.Time
}

// Validate checks if the input is valid.
func (i AchievementCheckInput) Validate() error {
	if i.StudentID == "" {
		return errors.New("achievement_flow: student ID is required")
	}
	return nil
}

// AchievementFlowResult contains the result of achievement processing.
type AchievementFlowResult struct {
	// StudentID - the student who received achievements.
	StudentID string

	// NewAchievements - list of newly unlocked achievements.
	NewAchievements []student.Achievement

	// TotalXPBonus - total XP awarded from all achievements.
	TotalXPBonus int

	// NotificationsSent - number of notifications sent.
	NotificationsSent int

	// ProcessedAt - when the flow completed.
	ProcessedAt time.Time
}

// HasNewAchievements returns true if any achievements were unlocked.
func (r *AchievementFlowResult) HasNewAchievements() bool {
	return len(r.NewAchievements) > 0
}

// AchievementFlowStep represents a step in the achievement flow.
type AchievementFlowStep string

const (
	StepLoadStudent         AchievementFlowStep = "load_student"
	StepLoadExistingAchievs AchievementFlowStep = "load_existing_achievements"
	StepCheckAchievements   AchievementFlowStep = "check_achievements"
	StepGrantAchievements   AchievementFlowStep = "grant_achievements"
	StepAwardXPBonus        AchievementFlowStep = "award_xp_bonus"
	StepSendNotifications   AchievementFlowStep = "send_notifications"
	StepUpdateStats         AchievementFlowStep = "update_statistics"
	StepPublishAchievEvents AchievementFlowStep = "publish_events"
	StepAchievementComplete AchievementFlowStep = "complete"
)

// AchievementFlowState tracks the current state of the achievement flow saga.
type AchievementFlowState struct {
	CurrentStep          AchievementFlowStep
	Input                AchievementCheckInput
	Student              *student.Student
	Streak               *student.Streak
	ExistingAchievements []student.Achievement
	NewAchievements      []student.Achievement
	TotalXPBonus         int
	NotificationsSent    int
	StartedAt            time.Time
	CompletedAt          *time.Time
	Error                error
	FailedStep           AchievementFlowStep
}

// ══════════════════════════════════════════════════════════════════════════════
// ACHIEVEMENT FLOW SAGA IMPLEMENTATION
// ══════════════════════════════════════════════════════════════════════════════

// AchievementFlowSaga orchestrates the complete achievement checking and granting process.
// It handles checking conditions, granting achievements, awarding bonuses, and notifications.
type AchievementFlowSaga struct {
	// Dependencies
	studentRepo        student.Repository
	progressRepo       student.ProgressRepository
	leaderboardRepo    leaderboard.LeaderboardRepository
	notificationSvc    notification.NotificationService
	eventBus           shared.EventPublisher
	achievementChecker *student.AchievementChecker
	idGenerator        IDGenerator

	// Configuration
	enableXPBonuses       bool
	enableNotifications   bool
	maxAchievementsPerRun int
}

// AchievementFlowConfig contains configuration for the achievement flow saga.
type AchievementFlowConfig struct {
	EnableXPBonuses       bool
	EnableNotifications   bool
	MaxAchievementsPerRun int
}

// DefaultAchievementFlowConfig returns default configuration.
func DefaultAchievementFlowConfig() AchievementFlowConfig {
	return AchievementFlowConfig{
		EnableXPBonuses:       true,
		EnableNotifications:   true,
		MaxAchievementsPerRun: 5, // Prevent spam if something goes wrong
	}
}

// NewAchievementFlowSaga creates a new achievement flow saga with all dependencies.
func NewAchievementFlowSaga(
	studentRepo student.Repository,
	progressRepo student.ProgressRepository,
	leaderboardRepo leaderboard.LeaderboardRepository,
	notificationSvc notification.NotificationService,
	eventBus shared.EventPublisher,
	idGenerator IDGenerator,
	config AchievementFlowConfig,
) *AchievementFlowSaga {
	return &AchievementFlowSaga{
		studentRepo:           studentRepo,
		progressRepo:          progressRepo,
		leaderboardRepo:       leaderboardRepo,
		notificationSvc:       notificationSvc,
		eventBus:              eventBus,
		achievementChecker:    student.NewAchievementChecker(),
		idGenerator:           idGenerator,
		enableXPBonuses:       config.EnableXPBonuses,
		enableNotifications:   config.EnableNotifications,
		maxAchievementsPerRun: config.MaxAchievementsPerRun,
	}
}

// Execute runs the complete achievement checking and granting process.
func (s *AchievementFlowSaga) Execute(ctx context.Context, input AchievementCheckInput) (*AchievementFlowResult, error) {
	state := &AchievementFlowState{
		CurrentStep: StepLoadStudent,
		Input:       input,
		StartedAt:   time.Now().UTC(),
	}

	// Validate input
	if err := input.Validate(); err != nil {
		state.FailedStep = StepLoadStudent
		state.Error = err
		return nil, s.wrapError(state, err)
	}

	// Step 1: Load student
	if err := s.stepLoadStudent(ctx, state); err != nil {
		return nil, s.wrapError(state, err)
	}

	// Step 2: Load existing achievements
	state.CurrentStep = StepLoadExistingAchievs
	if err := s.stepLoadExistingAchievements(ctx, state); err != nil {
		return nil, s.wrapError(state, err)
	}

	// Step 3: Check for new achievements
	state.CurrentStep = StepCheckAchievements
	if err := s.stepCheckAchievements(ctx, state); err != nil {
		return nil, s.wrapError(state, err)
	}

	// If no new achievements, return early
	if len(state.NewAchievements) == 0 {
		now := time.Now().UTC()
		state.CompletedAt = &now
		return &AchievementFlowResult{
			StudentID:         state.Input.StudentID,
			NewAchievements:   []student.Achievement{},
			TotalXPBonus:      0,
			NotificationsSent: 0,
			ProcessedAt:       now,
		}, nil
	}

	// Step 4: Grant achievements (persist to DB)
	state.CurrentStep = StepGrantAchievements
	if err := s.stepGrantAchievements(ctx, state); err != nil {
		return nil, s.wrapError(state, err)
	}

	// Step 5: Award XP bonuses
	state.CurrentStep = StepAwardXPBonus
	if err := s.stepAwardXPBonus(ctx, state); err != nil {
		// Non-critical - log but continue
		// XP can be awarded manually later
	}

	// Step 6: Send notifications
	state.CurrentStep = StepSendNotifications
	if err := s.stepSendNotifications(ctx, state); err != nil {
		// Non-critical - log but continue
	}

	// Step 7: Update statistics
	state.CurrentStep = StepUpdateStats
	if err := s.stepUpdateStatistics(ctx, state); err != nil {
		// Non-critical - log but continue
	}

	// Step 8: Publish domain events
	state.CurrentStep = StepPublishAchievEvents
	if err := s.stepPublishEvents(ctx, state); err != nil {
		// Non-critical - events can be replayed
	}

	// Complete
	state.CurrentStep = StepAchievementComplete
	now := time.Now().UTC()
	state.CompletedAt = &now

	return &AchievementFlowResult{
		StudentID:         state.Input.StudentID,
		NewAchievements:   state.NewAchievements,
		TotalXPBonus:      state.TotalXPBonus,
		NotificationsSent: state.NotificationsSent,
		ProcessedAt:       now,
	}, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// SAGA STEPS
// ══════════════════════════════════════════════════════════════════════════════

// stepLoadStudent loads the student entity from the repository.
func (s *AchievementFlowSaga) stepLoadStudent(ctx context.Context, state *AchievementFlowState) error {
	st, err := s.studentRepo.GetByID(ctx, state.Input.StudentID)
	if err != nil {
		state.FailedStep = StepLoadStudent
		state.Error = fmt.Errorf("failed to load student: %w", err)
		return state.Error
	}

	state.Student = st

	// Also load streak for achievement checking
	streak, err := s.progressRepo.GetStreak(ctx, state.Input.StudentID)
	if err != nil {
		// Create default streak if not found
		streak = student.NewStreak(state.Input.StudentID)
	}
	state.Streak = streak

	return nil
}

// stepLoadExistingAchievements loads the student's current achievements.
func (s *AchievementFlowSaga) stepLoadExistingAchievements(ctx context.Context, state *AchievementFlowState) error {
	achievements, err := s.progressRepo.GetAchievements(ctx, state.Input.StudentID)
	if err != nil {
		state.FailedStep = StepLoadExistingAchievs
		state.Error = fmt.Errorf("failed to load existing achievements: %w", err)
		return state.Error
	}

	state.ExistingAchievements = achievements
	return nil
}

// stepCheckAchievements evaluates all achievement conditions.
func (s *AchievementFlowSaga) stepCheckAchievements(ctx context.Context, state *AchievementFlowState) error {
	var newAchievements []student.Achievement

	// Get current rank from context or fetch it
	currentRank := state.Input.Context.CurrentRank
	if currentRank == 0 && s.leaderboardRepo != nil {
		entry, err := s.leaderboardRepo.GetStudentRank(
			ctx,
			state.Student.ID,
			leaderboard.Cohort(state.Student.Cohort),
		)
		if err == nil && entry != nil {
			currentRank = int(entry.Rank)
		}
	}

	// Use the domain achievement checker for standard achievements
	standardAchievements := s.achievementChecker.CheckNewAchievements(
		state.Student,
		state.Streak,
		currentRank,
		state.ExistingAchievements,
	)
	newAchievements = append(newAchievements, standardAchievements...)

	// Check for "First Task" achievement
	if state.Input.TriggerEvent == "task_completed" {
		firstTask := s.achievementChecker.CheckFirstTask(
			state.Input.Context.TasksCompleted,
			state.ExistingAchievements,
		)
		if firstTask != nil {
			newAchievements = append(newAchievements, *firstTask)
		}
	}

	// Check for "Comeback Kid" achievement
	if state.Input.Context.DaysInactive >= 7 {
		comeback := s.achievementChecker.CheckComebackKid(
			time.Now().Add(-time.Duration(state.Input.Context.DaysInactive)*24*time.Hour),
			state.ExistingAchievements,
		)
		if comeback != nil {
			newAchievements = append(newAchievements, *comeback)
		}
	}

	// Limit achievements per run to prevent spam
	if len(newAchievements) > s.maxAchievementsPerRun {
		newAchievements = newAchievements[:s.maxAchievementsPerRun]
	}

	state.NewAchievements = newAchievements
	return nil
}

// stepGrantAchievements persists the new achievements to the database.
func (s *AchievementFlowSaga) stepGrantAchievements(ctx context.Context, state *AchievementFlowState) error {
	for _, achievement := range state.NewAchievements {
		if err := s.progressRepo.SaveAchievement(ctx, state.Input.StudentID, achievement); err != nil {
			state.FailedStep = StepGrantAchievements
			state.Error = fmt.Errorf("failed to save achievement %s: %w", achievement.Type, err)
			return state.Error
		}
	}

	return nil
}

// stepAwardXPBonus awards XP bonuses for each achievement.
func (s *AchievementFlowSaga) stepAwardXPBonus(ctx context.Context, state *AchievementFlowState) error {
	if !s.enableXPBonuses {
		return nil
	}

	totalBonus := 0

	for _, achievement := range state.NewAchievements {
		def, found := student.GetAchievementDefinition(achievement.Type)
		if !found {
			continue
		}

		if def.XPBonus > 0 {
			totalBonus += int(def.XPBonus)

			// Record XP change in history
			entry := student.XPHistoryEntry{
				Timestamp: time.Now().UTC(),
				OldXP:     state.Student.CurrentXP,
				NewXP:     state.Student.CurrentXP + student.XP(def.XPBonus),
				Delta:     def.XPBonus,
				Reason:    fmt.Sprintf("achievement_%s", achievement.Type),
			}

			if err := s.progressRepo.SaveXPChange(ctx, entry); err != nil {
				// Log but continue - XP history is not critical
				continue
			}
		}
	}

	// Update student's XP
	if totalBonus > 0 {
		newXP := state.Student.CurrentXP + student.XP(totalBonus)
		if _, err := state.Student.UpdateXP(newXP); err != nil {
			return fmt.Errorf("failed to update student XP: %w", err)
		}

		if err := s.studentRepo.Update(ctx, state.Student); err != nil {
			return fmt.Errorf("failed to persist student XP update: %w", err)
		}
	}

	state.TotalXPBonus = totalBonus
	return nil
}

// stepSendNotifications sends achievement notifications to the student.
func (s *AchievementFlowSaga) stepSendNotifications(ctx context.Context, state *AchievementFlowState) error {
	if !s.enableNotifications || s.notificationSvc == nil {
		return nil
	}

	notificationsSent := 0

	for _, achievement := range state.NewAchievements {
		// Build notification message
		message := s.buildAchievementMessage(achievement)

		achievPriority := notification.PriorityHigh
		achievNotification, err := notification.NewNotification(notification.NewNotificationParams{
			ID:             notification.NotificationID(s.idGenerator.GenerateID()),
			Type:           notification.NotificationTypeAchievement,
			RecipientID:    notification.RecipientID(state.Student.ID),
			TelegramChatID: notification.TelegramChatID(state.Student.TelegramID),
			Title:          "🏅 Новое достижение!",
			Message:        message,
			Priority:       &achievPriority,
		})
		if err != nil {
			// Log but continue with other achievements
			continue
		}

		// Add achievement metadata
		achievNotification.SetMetadata("achievement_type", string(achievement.Type))

		if err := s.notificationSvc.ScheduleNotification(ctx, achievNotification); err != nil {
			// Log but continue
			continue
		}

		notificationsSent++
	}

	state.NotificationsSent = notificationsSent
	return nil
}

// stepUpdateStatistics updates any relevant statistics.
func (s *AchievementFlowSaga) stepUpdateStatistics(ctx context.Context, state *AchievementFlowState) error {
	// Update daily grind with achievement info
	dailyGrind, err := s.progressRepo.GetTodayDailyGrind(ctx, state.Input.StudentID)
	if err != nil {
		// Create new daily grind if not exists
		rank := 0
		if s.leaderboardRepo != nil {
			entry, _ := s.leaderboardRepo.GetStudentRank(
				ctx,
				state.Student.ID,
				leaderboard.Cohort(state.Student.Cohort),
			)
			if entry != nil {
				rank = int(entry.Rank)
			}
		}
		dailyGrind = student.NewDailyGrind(state.Input.StudentID, state.Student.CurrentXP, rank)
	}

	// Record XP gained from achievements
	if state.TotalXPBonus > 0 {
		dailyGrind.RecordXPGain(state.Student.CurrentXP)
	}

	if err := s.progressRepo.SaveDailyGrind(ctx, dailyGrind); err != nil {
		// Non-critical, log but continue
		return nil
	}

	return nil
}

// stepPublishEvents publishes domain events for each achievement.
func (s *AchievementFlowSaga) stepPublishEvents(ctx context.Context, state *AchievementFlowState) error {
	if s.eventBus == nil {
		return nil
	}

	for _, achievement := range state.NewAchievements {
		studentEvent := student.NewAchievementUnlockedEvent(state.Student, achievement)
		wrappedEvent := wrapAchievementEvent(state.Student.ID, studentEvent)

		if err := s.eventBus.Publish(wrappedEvent); err != nil {
			// Log but continue with other events
			continue
		}
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ══════════════════════════════════════════════════════════════════════════════

// buildAchievementMessage creates a formatted message for an achievement notification.
func (s *AchievementFlowSaga) buildAchievementMessage(achievement student.Achievement) string {
	def, found := student.GetAchievementDefinition(achievement.Type)
	if !found {
		return fmt.Sprintf("Ты получил достижение: %s!", achievement.Type)
	}

	message := fmt.Sprintf(
		"%s <b>%s</b>\n\n"+
			"<i>%s</i>",
		def.Emoji,
		def.Name,
		def.Description,
	)

	if def.XPBonus > 0 {
		message += fmt.Sprintf("\n\n🎁 <b>Бонус:</b> +%d XP", def.XPBonus)
	}

	// Add motivational suffix based on achievement type
	suffix := s.getAchievementSuffix(achievement.Type)
	if suffix != "" {
		message += "\n\n" + suffix
	}

	return message
}

// getAchievementSuffix returns a motivational suffix for specific achievement types.
func (s *AchievementFlowSaga) getAchievementSuffix(achievementType student.AchievementType) string {
	switch achievementType {
	case student.AchievementFirstTask:
		return "🚀 Отличное начало! Первый шаг сделан."
	case student.AchievementStreak7:
		return "🔥 Неделя огня! Продолжай в том же духе."
	case student.AchievementStreak30:
		return "💪 Железная воля! Ты — пример для подражания."
	case student.AchievementTop10:
		return "👑 Ты в элите! Помоги тем, кто ещё в пути."
	case student.AchievementTop50:
		return "⭐ Звезда восходит! До топ-10 рукой подать."
	case student.AchievementHelper5:
		return "🤝 Спасибо за помощь сообществу!"
	case student.AchievementHelper20:
		return "🎓 Ты — настоящий наставник!"
	case student.AchievementComebackKid:
		return "🎉 С возвращением! Рады видеть тебя снова."
	case student.AchievementNightOwl:
		return "🦉 Полуночное кодинг-сессия? Уважаем!"
	case student.AchievementEarlyBird:
		return "🐦 Ранняя пташка! Кто рано встаёт, тому XP даёт."
	default:
		return ""
	}
}

// wrapError wraps an error with saga context.
func (s *AchievementFlowSaga) wrapError(state *AchievementFlowState, err error) error {
	return &AchievementFlowError{
		Step:      state.FailedStep,
		StudentID: state.Input.StudentID,
		Cause:     err,
		Message:   fmt.Sprintf("achievement flow failed at step '%s': %v", state.FailedStep, err),
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// CONVENIENCE METHODS FOR COMMON TRIGGERS
// ══════════════════════════════════════════════════════════════════════════════

// CheckAfterXPGain checks achievements after XP gain event.
func (s *AchievementFlowSaga) CheckAfterXPGain(
	ctx context.Context,
	studentID string,
	newXP, newLevel int,
) (*AchievementFlowResult, error) {
	return s.Execute(ctx, AchievementCheckInput{
		StudentID:    studentID,
		TriggerEvent: "xp_gained",
		Context: AchievementContext{
			NewXP:     newXP,
			NewLevel:  newLevel,
			Timestamp: time.Now().UTC(),
		},
	})
}

// CheckAfterTaskCompletion checks achievements after task completion.
func (s *AchievementFlowSaga) CheckAfterTaskCompletion(
	ctx context.Context,
	studentID string,
	taskID string,
	totalTasks int,
) (*AchievementFlowResult, error) {
	return s.Execute(ctx, AchievementCheckInput{
		StudentID:    studentID,
		TriggerEvent: "task_completed",
		Context: AchievementContext{
			TaskID:         taskID,
			TasksCompleted: totalTasks,
			Timestamp:      time.Now().UTC(),
		},
	})
}

// CheckAfterHelpProvided checks achievements after helping another student.
func (s *AchievementFlowSaga) CheckAfterHelpProvided(
	ctx context.Context,
	studentID string,
	totalHelpCount int,
) (*AchievementFlowResult, error) {
	return s.Execute(ctx, AchievementCheckInput{
		StudentID:    studentID,
		TriggerEvent: "help_provided",
		Context: AchievementContext{
			HelpCount: totalHelpCount,
			Timestamp: time.Now().UTC(),
		},
	})
}

// CheckAfterRankChange checks achievements after leaderboard rank change.
func (s *AchievementFlowSaga) CheckAfterRankChange(
	ctx context.Context,
	studentID string,
	newRank int,
) (*AchievementFlowResult, error) {
	return s.Execute(ctx, AchievementCheckInput{
		StudentID:    studentID,
		TriggerEvent: "rank_changed",
		Context: AchievementContext{
			CurrentRank: newRank,
			Timestamp:   time.Now().UTC(),
		},
	})
}

// CheckAfterStreak checks achievements after streak update.
func (s *AchievementFlowSaga) CheckAfterStreak(
	ctx context.Context,
	studentID string,
	currentStreak int,
) (*AchievementFlowResult, error) {
	return s.Execute(ctx, AchievementCheckInput{
		StudentID:    studentID,
		TriggerEvent: "streak_updated",
		Context: AchievementContext{
			CurrentStreak: currentStreak,
			Timestamp:     time.Now().UTC(),
		},
	})
}

// CheckAfterReturn checks achievements when student returns after inactivity.
func (s *AchievementFlowSaga) CheckAfterReturn(
	ctx context.Context,
	studentID string,
	daysInactive int,
) (*AchievementFlowResult, error) {
	return s.Execute(ctx, AchievementCheckInput{
		StudentID:    studentID,
		TriggerEvent: "student_returned",
		Context: AchievementContext{
			DaysInactive: daysInactive,
			Timestamp:    time.Now().UTC(),
		},
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// ERRORS
// ══════════════════════════════════════════════════════════════════════════════

// AchievementFlowError represents an error during the achievement flow.
type AchievementFlowError struct {
	Step      AchievementFlowStep
	StudentID string
	Cause     error
	Message   string
}

// Error implements the error interface.
func (e *AchievementFlowError) Error() string {
	return e.Message
}

// Unwrap returns the underlying error.
func (e *AchievementFlowError) Unwrap() error {
	return e.Cause
}

// Achievement flow specific errors.
var (
	// ErrStudentNotFoundForAchievement - student not found when checking achievements.
	ErrStudentNotFoundForAchievement = errors.New("achievement_flow: student not found")

	// ErrAchievementAlreadyGranted - achievement was already granted to this student.
	ErrAchievementAlreadyGranted = errors.New("achievement_flow: achievement already granted")

	// ErrInvalidAchievementType - unknown achievement type.
	ErrInvalidAchievementType = errors.New("achievement_flow: invalid achievement type")
)

// ══════════════════════════════════════════════════════════════════════════════
// ACHIEVEMENT FLOW SAGA BUILDER (Fluent API)
// ══════════════════════════════════════════════════════════════════════════════

// AchievementFlowSagaBuilder provides a fluent API for building AchievementFlowSaga.
type AchievementFlowSagaBuilder struct {
	studentRepo     student.Repository
	progressRepo    student.ProgressRepository
	leaderboardRepo leaderboard.LeaderboardRepository
	notificationSvc notification.NotificationService
	eventBus        shared.EventPublisher
	idGenerator     IDGenerator
	config          AchievementFlowConfig
}

// NewAchievementFlowSagaBuilder creates a new builder.
func NewAchievementFlowSagaBuilder() *AchievementFlowSagaBuilder {
	return &AchievementFlowSagaBuilder{
		config: DefaultAchievementFlowConfig(),
	}
}

// WithStudentRepo sets the student repository.
func (b *AchievementFlowSagaBuilder) WithStudentRepo(repo student.Repository) *AchievementFlowSagaBuilder {
	b.studentRepo = repo
	return b
}

// WithProgressRepo sets the progress repository.
func (b *AchievementFlowSagaBuilder) WithProgressRepo(repo student.ProgressRepository) *AchievementFlowSagaBuilder {
	b.progressRepo = repo
	return b
}

// WithLeaderboardRepo sets the leaderboard repository.
func (b *AchievementFlowSagaBuilder) WithLeaderboardRepo(repo leaderboard.LeaderboardRepository) *AchievementFlowSagaBuilder {
	b.leaderboardRepo = repo
	return b
}

// WithNotificationService sets the notification service.
func (b *AchievementFlowSagaBuilder) WithNotificationService(svc notification.NotificationService) *AchievementFlowSagaBuilder {
	b.notificationSvc = svc
	return b
}

// WithEventBus sets the event bus.
func (b *AchievementFlowSagaBuilder) WithEventBus(bus shared.EventPublisher) *AchievementFlowSagaBuilder {
	b.eventBus = bus
	return b
}

// WithIDGenerator sets the ID generator.
func (b *AchievementFlowSagaBuilder) WithIDGenerator(gen IDGenerator) *AchievementFlowSagaBuilder {
	b.idGenerator = gen
	return b
}

// WithConfig sets the configuration.
func (b *AchievementFlowSagaBuilder) WithConfig(config AchievementFlowConfig) *AchievementFlowSagaBuilder {
	b.config = config
	return b
}

// Build creates the AchievementFlowSaga instance.
func (b *AchievementFlowSagaBuilder) Build() (*AchievementFlowSaga, error) {
	if b.studentRepo == nil {
		return nil, errors.New("student repository is required")
	}
	if b.progressRepo == nil {
		return nil, errors.New("progress repository is required")
	}
	if b.idGenerator == nil {
		return nil, errors.New("id generator is required")
	}

	return NewAchievementFlowSaga(
		b.studentRepo,
		b.progressRepo,
		b.leaderboardRepo,
		b.notificationSvc,
		b.eventBus,
		b.idGenerator,
		b.config,
	), nil
}

// ══════════════════════════════════════════════════════════════════════════════
// EVENT ADAPTERS
// Adapts student domain events to shared.Event interface for the event bus.
// ══════════════════════════════════════════════════════════════════════════════

// achievementEventAdapter adapts a student.AchievementUnlockedEvent to shared.Event.
type achievementEventAdapter struct {
	event     student.AchievementUnlockedEvent
	studentID string
}

// EventType implements shared.Event interface.
func (a *achievementEventAdapter) EventType() shared.EventType {
	return shared.EventLevelUp // Use level up event type as closest match for achievements
}

// OccurredAt implements shared.Event interface.
func (a *achievementEventAdapter) OccurredAt() time.Time {
	return a.event.OccurredAt()
}

// AggregateID implements shared.Event interface.
func (a *achievementEventAdapter) AggregateID() string {
	return a.studentID
}

// Payload implements shared.Event interface.
func (a *achievementEventAdapter) Payload() map[string]interface{} {
	return map[string]interface{}{
		"achievement_type": string(a.event.Achievement.Type),
		"xp_bonus":         int(a.event.XPBonus),
	}
}

// wrapAchievementEvent wraps a student event to satisfy shared.Event interface.
func wrapAchievementEvent(studentID string, event student.AchievementUnlockedEvent) shared.Event {
	return &achievementEventAdapter{event: event, studentID: studentID}
}
