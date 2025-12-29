// Package eventhandler содержит обработчики доменных событий.
package eventhandler

import (
	"alem-hub/internal/domain/activity"
	"alem-hub/internal/domain/notification"
	"alem-hub/internal/domain/shared"
	"alem-hub/internal/domain/social"
	"alem-hub/internal/domain/student"
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════
// ON TASK COMPLETED HANDLER
// Обрабатывает событие выполнения задачи студентом.
//
// Ключевые функции:
// 1. Обновление индекса "кто решил" — для функции поиска помощников
// 2. Проверка достижений — milestone'ы по количеству задач
// 3. Обновление социального графа — если была получена помощь
// 4. Подтверждение выполнения — уведомление студенту
//
// Философия "От конкуренции к сотрудничеству":
// - Каждое решение задачи делает студента потенциальным помощником для других
// - Если помощь была получена, укрепляем социальную связь
// ═══════════════════════════════════════════════════════════════════════════

// OnTaskCompletedHandler обрабатывает событие выполнения задачи.
type OnTaskCompletedHandler struct {
	// Repositories (интерфейсы из domain layer)
	studentRepo  student.Repository
	activityRepo activity.Repository
	taskIndex    activity.TaskIndex
	socialRepo   social.Repository
	progressRepo student.ProgressRepository

	// Notification sender
	notificationSender notification.NotificationSender

	// Logger для структурированного логирования
	logger *slog.Logger

	// Configuration
	config TaskCompletedConfig
}

// TaskCompletedConfig содержит конфигурацию обработчика.
type TaskCompletedConfig struct {
	// SendConfirmation — отправлять ли подтверждение о выполнении задачи.
	SendConfirmation bool

	// TaskMilestones — milestones для достижений (количество задач).
	// Например: [10, 25, 50, 100] — уведомляем при достижении этих отметок.
	TaskMilestones []int

	// NotifyHelperOnSuccess — уведомлять ли помощника, когда его подопечный решил задачу.
	NotifyHelperOnSuccess bool

	// IndexRetentionDays — сколько дней хранить записи в индексе.
	IndexRetentionDays int

	// UpdateSocialGraphOnHelp — обновлять ли социальный граф при получении помощи.
	UpdateSocialGraphOnHelp bool
}

// DefaultTaskCompletedConfig возвращает конфигурацию по умолчанию.
func DefaultTaskCompletedConfig() TaskCompletedConfig {
	return TaskCompletedConfig{
		SendConfirmation:        true,
		TaskMilestones:          []int{10, 25, 50, 100, 200, 500},
		NotifyHelperOnSuccess:   true,
		IndexRetentionDays:      90,
		UpdateSocialGraphOnHelp: true,
	}
}

// NewOnTaskCompletedHandler создаёт новый обработчик события выполнения задачи.
func NewOnTaskCompletedHandler(
	studentRepo student.Repository,
	activityRepo activity.Repository,
	taskIndex activity.TaskIndex,
	socialRepo social.Repository,
	progressRepo student.ProgressRepository,
	notificationSender notification.NotificationSender,
	logger *slog.Logger,
	config TaskCompletedConfig,
) *OnTaskCompletedHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &OnTaskCompletedHandler{
		studentRepo:        studentRepo,
		activityRepo:       activityRepo,
		taskIndex:          taskIndex,
		socialRepo:         socialRepo,
		progressRepo:       progressRepo,
		notificationSender: notificationSender,
		logger:             logger.With("handler", "on_task_completed"),
		config:             config,
	}
}

// Handle обрабатывает событие выполнения задачи.
// Реализует интерфейс shared.EventHandler.
func (h *OnTaskCompletedHandler) Handle(event shared.Event) error {
	ctx := context.Background()

	// Type assertion для получения конкретного типа события
	taskEvent, ok := event.(shared.TaskCompletedEvent)
	if !ok {
		h.logger.Warn("received non-TaskCompletedEvent",
			"event_type", event.EventType(),
		)
		return nil
	}

	h.logger.Info("processing task completed event",
		"student_id", taskEvent.StudentID,
		"task_id", taskEvent.TaskID,
		"xp_earned", taskEvent.XPEarned,
		"helper_id", taskEvent.HelperID,
	)

	// 1. Обновляем индекс "кто решил" — ключевая функция для поиска помощников
	if err := h.updateTaskIndex(ctx, taskEvent); err != nil {
		h.logger.Error("failed to update task index",
			"task_id", taskEvent.TaskID,
			"student_id", taskEvent.StudentID,
			"error", err,
		)
		// Продолжаем выполнение — индекс не критичен
	}

	// 2. Получаем информацию о студенте для дальнейших операций
	studentEntity, err := h.studentRepo.GetByID(ctx, taskEvent.StudentID)
	if err != nil {
		h.logger.Error("failed to get student",
			"student_id", taskEvent.StudentID,
			"error", err,
		)
		return fmt.Errorf("get student: %w", err)
	}

	// 3. Обновляем активность студента
	if err := h.updateStudentActivity(ctx, taskEvent); err != nil {
		h.logger.Error("failed to update student activity",
			"student_id", taskEvent.StudentID,
			"error", err,
		)
	}

	// 4. Проверяем достижения по количеству задач
	if err := h.checkTaskMilestones(ctx, studentEntity); err != nil {
		h.logger.Error("failed to check task milestones",
			"student_id", taskEvent.StudentID,
			"error", err,
		)
	}

	// 5. Если была получена помощь — обновляем социальный граф
	if taskEvent.HelperID != "" && h.config.UpdateSocialGraphOnHelp {
		if err := h.processHelpReceived(ctx, taskEvent); err != nil {
			h.logger.Error("failed to process help received",
				"student_id", taskEvent.StudentID,
				"helper_id", taskEvent.HelperID,
				"error", err,
			)
		}
	}

	// 6. Отправляем подтверждение о выполнении (если включено)
	if h.config.SendConfirmation {
		if err := h.sendCompletionConfirmation(ctx, studentEntity, taskEvent); err != nil {
			h.logger.Warn("failed to send completion confirmation",
				"student_id", taskEvent.StudentID,
				"error", err,
			)
		}
	}

	h.logger.Info("task completed event processed successfully",
		"student_id", taskEvent.StudentID,
		"task_id", taskEvent.TaskID,
	)

	return nil
}

// updateTaskIndex обновляет индекс "кто решил задачу".
// Этот индекс — ключевой для функции поиска помощников.
func (h *OnTaskCompletedHandler) updateTaskIndex(
	ctx context.Context,
	event shared.TaskCompletedEvent,
) error {
	if h.taskIndex == nil {
		h.logger.Debug("task index not configured, skipping")
		return nil
	}

	taskID := activity.TaskID(event.TaskID)
	studentID := activity.StudentID(event.StudentID)
	completedAt := event.OccurredAt()

	// Добавляем студента в индекс решивших задачу
	if err := h.taskIndex.IndexTaskCompletion(ctx, taskID, studentID, completedAt); err != nil {
		return fmt.Errorf("index task completion: %w", err)
	}

	h.logger.Debug("task index updated",
		"task_id", event.TaskID,
		"student_id", event.StudentID,
	)

	return nil
}

// updateStudentActivity обновляет запись активности студента.
func (h *OnTaskCompletedHandler) updateStudentActivity(
	ctx context.Context,
	event shared.TaskCompletedEvent,
) error {
	if h.activityRepo == nil {
		return nil
	}

	// Создаём запись о выполнении задачи
	completion, err := activity.NewTaskCompletion(
		generateID(),
		activity.StudentID(event.StudentID),
		activity.TaskID(event.TaskID),
		event.OccurredAt(),
		event.XPEarned,
	)
	if err != nil {
		return fmt.Errorf("create task completion: %w", err)
	}

	// Устанавливаем время выполнения, если известно
	if event.TimeSpent > 0 {
		if err := completion.SetTimeSpent(event.TimeSpent); err != nil {
			h.logger.Warn("failed to set time spent",
				"error", err,
			)
		}
	}

	// Отмечаем, если была получена помощь
	if event.HelperID != "" {
		helperID := activity.StudentID(event.HelperID)
		if err := completion.MarkHelpReceived(helperID); err != nil {
			h.logger.Warn("failed to mark help received",
				"error", err,
			)
		}
	}

	// Сохраняем запись о выполнении
	if err := h.activityRepo.SaveTaskCompletion(ctx, completion); err != nil {
		return fmt.Errorf("save task completion: %w", err)
	}

	// Обновляем дневной прогресс
	if err := h.updateDailyProgress(ctx, event); err != nil {
		h.logger.Warn("failed to update daily progress",
			"error", err,
		)
	}

	return nil
}

// updateDailyProgress обновляет дневной прогресс студента (Daily Grind).
func (h *OnTaskCompletedHandler) updateDailyProgress(
	ctx context.Context,
	event shared.TaskCompletedEvent,
) error {
	if h.activityRepo == nil {
		return nil
	}

	studentID := activity.StudentID(event.StudentID)
	today := time.Now()

	// Получаем или создаём запись дневного прогресса
	progress, err := h.activityRepo.GetDailyProgress(ctx, studentID, today)
	if err != nil {
		// Если записи нет — создаём новую
		progress = activity.NewDailyProgress(studentID, today)
	}

	// Добавляем выполненную задачу
	progress.AddTaskCompletion(event.XPEarned, event.TimeSpent)

	// Сохраняем обновлённый прогресс
	if err := h.activityRepo.SaveDailyProgress(ctx, progress); err != nil {
		return fmt.Errorf("save daily progress: %w", err)
	}

	return nil
}

// checkTaskMilestones проверяет достижения по количеству задач.
func (h *OnTaskCompletedHandler) checkTaskMilestones(
	ctx context.Context,
	studentEntity *student.Student,
) error {
	if h.progressRepo == nil {
		return nil
	}

	// Получаем общее количество выполненных задач
	activityData, err := h.activityRepo.GetActivity(ctx, activity.StudentID(studentEntity.ID))
	if err != nil {
		return fmt.Errorf("get activity: %w", err)
	}

	totalTasks := activityData.TotalTasksCompleted

	// Проверяем каждый milestone
	for _, milestone := range h.config.TaskMilestones {
		// Milestone достигнут именно сейчас (предыдущее значение было меньше)
		if totalTasks == milestone {
			if err := h.sendMilestoneNotification(ctx, studentEntity, milestone); err != nil {
				h.logger.Warn("failed to send milestone notification",
					"milestone", milestone,
					"error", err,
				)
			}

			// Сохраняем достижение
			if err := h.recordMilestoneAchievement(ctx, studentEntity.ID, milestone); err != nil {
				h.logger.Warn("failed to record milestone achievement",
					"milestone", milestone,
					"error", err,
				)
			}
		}
	}

	return nil
}

// sendMilestoneNotification отправляет уведомление о достижении milestone.
func (h *OnTaskCompletedHandler) sendMilestoneNotification(
	ctx context.Context,
	studentEntity *student.Student,
	milestone int,
) error {
	emoji := notification.NotificationTypeAchievement.Emoji()

	var message string
	switch {
	case milestone >= 500:
		message = fmt.Sprintf("%s Легендарный результат! %d задач выполнено! Ты — настоящий мастер!",
			emoji, milestone)
	case milestone >= 100:
		message = fmt.Sprintf("%s Великолепно! %d задач! Твоя целеустремлённость впечатляет!",
			emoji, milestone)
	case milestone >= 50:
		message = fmt.Sprintf("%s Отличная работа! %d задач решено! Ты на правильном пути!",
			emoji, milestone)
	default:
		message = fmt.Sprintf("%s Поздравляем! %d задач выполнено! Так держать!",
			emoji, milestone)
	}

	notif, err := notification.NewNotification(
		notification.NotificationID(generateID()),
		notification.NotificationTypeAchievement,
		notification.RecipientID(studentEntity.ID),
		notification.TelegramChatID(studentEntity.TelegramID),
		message,
		notification.PriorityHigh,
	)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	notif.SetMetadata("milestone_type", "tasks_completed")
	notif.SetMetadata("milestone_value", fmt.Sprintf("%d", milestone))

	result := h.notificationSender.Send(ctx, notif)
	if !result.Success {
		return fmt.Errorf("send notification: %w", result.Error)
	}

	return nil
}

// recordMilestoneAchievement сохраняет достижение в прогрессе студента.
func (h *OnTaskCompletedHandler) recordMilestoneAchievement(
	ctx context.Context,
	studentID string,
	milestone int,
) error {
	if h.progressRepo == nil {
		return nil
	}

	achievementType := student.AchievementType(fmt.Sprintf("tasks_%d", milestone))
	achievement := student.Achievement{
		Type:        achievementType,
		Name:        fmt.Sprintf("%d задач", milestone),
		Description: fmt.Sprintf("Выполнено %d задач", milestone),
		UnlockedAt:  time.Now(),
	}

	return h.progressRepo.SaveAchievement(ctx, studentID, achievement)
}

// processHelpReceived обрабатывает получение помощи.
// Обновляет социальный граф и уведомляет помощника.
func (h *OnTaskCompletedHandler) processHelpReceived(
	ctx context.Context,
	event shared.TaskCompletedEvent,
) error {
	// 1. Обновляем статистику помощника
	if h.socialRepo != nil {
		if err := h.updateHelperStats(ctx, event.HelperID); err != nil {
			h.logger.Warn("failed to update helper stats",
				"helper_id", event.HelperID,
				"error", err,
			)
		}
	}

	// 2. Создаём или укрепляем связь между студентами
	if h.socialRepo != nil {
		if err := h.ensureConnection(ctx, event.StudentID, event.HelperID, event.TaskID); err != nil {
			h.logger.Warn("failed to ensure connection",
				"student_id", event.StudentID,
				"helper_id", event.HelperID,
				"error", err,
			)
		}
	}

	// 3. Уведомляем помощника об успехе подопечного
	if h.config.NotifyHelperOnSuccess {
		if err := h.notifyHelperOnSuccess(ctx, event); err != nil {
			h.logger.Warn("failed to notify helper",
				"helper_id", event.HelperID,
				"error", err,
			)
		}
	}

	return nil
}

// updateHelperStats обновляет статистику помощника.
func (h *OnTaskCompletedHandler) updateHelperStats(
	ctx context.Context,
	helperID string,
) error {
	profile, err := h.socialRepo.SocialProfiles().GetByStudentID(ctx, social.StudentID(helperID))
	if err != nil {
		return fmt.Errorf("get social profile: %w", err)
	}

	// Увеличиваем счётчик помощей
	profile.IncrementHelpCount()

	if err := h.socialRepo.SocialProfiles().Update(ctx, profile); err != nil {
		return fmt.Errorf("update social profile: %w", err)
	}

	return nil
}

// ensureConnection создаёт или укрепляет связь между студентами.
func (h *OnTaskCompletedHandler) ensureConnection(
	ctx context.Context,
	studentID, helperID, taskID string,
) error {
	connRepo := h.socialRepo.Connections()

	// Проверяем, существует ли уже связь
	exists, err := connRepo.ExistsBetweenStudents(
		ctx,
		social.StudentID(studentID),
		social.StudentID(helperID),
	)
	if err != nil {
		return fmt.Errorf("check connection exists: %w", err)
	}

	if exists {
		// Связь уже есть — обновляем статистику взаимодействий
		conn, err := connRepo.GetByStudents(
			ctx,
			social.StudentID(studentID),
			social.StudentID(helperID),
		)
		if err != nil {
			return fmt.Errorf("get connection: %w", err)
		}

		conn.RecordInteraction("task_help", taskID)
		if err := connRepo.Update(ctx, conn); err != nil {
			return fmt.Errorf("update connection: %w", err)
		}
	} else {
		// Создаём новую связь типа "helper"
		conn, err := social.NewConnection(
			generateID(),
			social.StudentID(helperID), // Инициатор — помощник
			social.StudentID(studentID),
			social.ConnectionTypeHelper,
		)
		if err != nil {
			return fmt.Errorf("create connection: %w", err)
		}

		// Связь типа helper сразу активна
		if err := conn.Accept(); err != nil {
			h.logger.Warn("failed to accept connection",
				"error", err,
			)
		}

		if err := connRepo.Create(ctx, conn); err != nil {
			return fmt.Errorf("save connection: %w", err)
		}
	}

	return nil
}

// notifyHelperOnSuccess уведомляет помощника об успехе подопечного.
func (h *OnTaskCompletedHandler) notifyHelperOnSuccess(
	ctx context.Context,
	event shared.TaskCompletedEvent,
) error {
	// Получаем информацию о помощнике
	helper, err := h.studentRepo.GetByID(ctx, event.HelperID)
	if err != nil {
		return fmt.Errorf("get helper: %w", err)
	}

	// Получаем информацию о студенте, которому помогли
	studentEntity, err := h.studentRepo.GetByID(ctx, event.StudentID)
	if err != nil {
		return fmt.Errorf("get student: %w", err)
	}

	// Проверяем, что помощник может получать уведомления
	if !helper.Status.CanReceiveNotifications() || !helper.Preferences.HelpRequests {
		return nil
	}

	emoji := notification.NotificationTypeEndorsementReceived.Emoji()
	message := fmt.Sprintf("%s %s решил задачу %s с твоей помощью! Спасибо, что помогаешь сообществу!",
		emoji, studentEntity.DisplayName, event.TaskID)

	notif, err := notification.NewNotification(
		notification.NotificationID(generateID()),
		notification.NotificationTypeEndorsementReceived,
		notification.RecipientID(helper.ID),
		notification.TelegramChatID(helper.TelegramID),
		message,
		notification.PriorityLow,
	)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	notif.SetMetadata("helped_student_id", event.StudentID)
	notif.SetMetadata("task_id", event.TaskID)

	result := h.notificationSender.Send(ctx, notif)
	if !result.Success {
		return fmt.Errorf("send notification: %w", result.Error)
	}

	return nil
}

// sendCompletionConfirmation отправляет подтверждение о выполнении задачи.
func (h *OnTaskCompletedHandler) sendCompletionConfirmation(
	ctx context.Context,
	studentEntity *student.Student,
	event shared.TaskCompletedEvent,
) error {
	// Не отправляем подтверждение в тихие часы
	if studentEntity.Preferences.IsQuietHour(time.Now()) {
		return nil
	}

	emoji := notification.NotificationTypeTaskCompleted.Emoji()
	message := fmt.Sprintf("%s Задача %s засчитана! +%d XP",
		emoji, event.TaskID, event.XPEarned)

	// Добавляем мотивирующий постскриптум для особых случаев
	if event.XPEarned >= 200 {
		message += " 🔥 Отличный результат!"
	}

	notif, err := notification.NewNotification(
		notification.NotificationID(generateID()),
		notification.NotificationTypeTaskCompleted,
		notification.RecipientID(studentEntity.ID),
		notification.TelegramChatID(studentEntity.TelegramID),
		message,
		notification.PriorityLow, // Низкий приоритет — информационное уведомление
	)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	notif.SetMetadata("task_id", event.TaskID)
	notif.SetMetadata("xp_earned", fmt.Sprintf("%d", event.XPEarned))

	result := h.notificationSender.Send(ctx, notif)
	if !result.Success {
		return fmt.Errorf("send notification: %w", result.Error)
	}

	return nil
}

// EventType возвращает тип события, который обрабатывает этот handler.
func (h *OnTaskCompletedHandler) EventType() shared.EventType {
	return shared.EventTaskCompleted
}
