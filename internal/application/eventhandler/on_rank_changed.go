// Package eventhandler содержит обработчики доменных событий.
// Эти обработчики реализуют event-driven архитектуру и связывают
// различные части системы через асинхронные события.
//
// Философия: обработчики событий — это "реактивная" часть системы.
// Они реагируют на изменения и запускают побочные эффекты,
// такие как отправка уведомлений или обновление кешей.
package eventhandler

import (
	"alem-hub/internal/domain/leaderboard"
	"alem-hub/internal/domain/notification"
	"alem-hub/internal/domain/shared"
	"alem-hub/internal/domain/student"
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════
// ON RANK CHANGED HANDLER
// Обрабатывает событие изменения ранга студента в лидерборде.
//
// Философия "От конкуренции к сотрудничеству":
// - Уведомления о повышении мотивируют, а не хвастают
// - Уведомления о понижении подаются мягко, предлагая помощь
// - Вход в топ — это признание усилий сообщества
// ═══════════════════════════════════════════════════════════════════════════

// OnRankChangedHandler обрабатывает событие изменения ранга студента.
// Отправляет уведомления и обновляет связанные сущности.
type OnRankChangedHandler struct {
	// Dependencies (интерфейсы из domain layer)
	studentRepo        student.Repository
	notificationSender notification.NotificationSender
	leaderboardCache   leaderboard.LeaderboardCache

	// Logger для структурированного логирования
	logger *slog.Logger

	// Configuration
	config RankChangedConfig
}

// RankChangedConfig содержит конфигурацию обработчика.
type RankChangedConfig struct {
	// MinRankChangeForNotification — минимальное изменение ранга для уведомления.
	// Не беспокоим студента при изменении на 1-2 позиции.
	MinRankChangeForNotification int

	// TopNMilestones — пороги для уведомлений о входе в топ.
	// Например: [10, 50, 100] — уведомляем при входе в топ-10, топ-50, топ-100.
	TopNMilestones []int

	// NotifyOnOvertake — уведомлять ли, когда кто-то обогнал студента.
	NotifyOnOvertake bool

	// CooldownPeriod — минимальный интервал между уведомлениями одному студенту.
	CooldownPeriod time.Duration

	// QuietHoursEnabled — учитывать ли тихие часы студента.
	QuietHoursEnabled bool
}

// DefaultRankChangedConfig возвращает конфигурацию по умолчанию.
func DefaultRankChangedConfig() RankChangedConfig {
	return RankChangedConfig{
		MinRankChangeForNotification: 3,
		TopNMilestones:               []int{10, 50, 100},
		NotifyOnOvertake:             true,
		CooldownPeriod:               30 * time.Minute,
		QuietHoursEnabled:            true,
	}
}

// NewOnRankChangedHandler создаёт новый обработчик события изменения ранга.
func NewOnRankChangedHandler(
	studentRepo student.Repository,
	notificationSender notification.NotificationSender,
	leaderboardCache leaderboard.LeaderboardCache,
	logger *slog.Logger,
	config RankChangedConfig,
) *OnRankChangedHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &OnRankChangedHandler{
		studentRepo:        studentRepo,
		notificationSender: notificationSender,
		leaderboardCache:   leaderboardCache,
		logger:             logger.With("handler", "on_rank_changed"),
		config:             config,
	}
}

// Handle обрабатывает событие изменения ранга.
// Реализует интерфейс shared.EventHandler.
func (h *OnRankChangedHandler) Handle(event shared.Event) error {
	ctx := context.Background()

	// Type assertion для получения конкретного типа события
	rankEvent, ok := event.(shared.RankChangedEvent)
	if !ok {
		h.logger.Warn("received non-RankChangedEvent",
			"event_type", event.EventType(),
		)
		return nil
	}

	h.logger.Info("processing rank changed event",
		"student_id", rankEvent.StudentID,
		"old_rank", rankEvent.OldRank,
		"new_rank", rankEvent.NewRank,
		"rank_change", rankEvent.RankChange,
		"cohort", rankEvent.Cohort,
	)

	// 1. Получаем информацию о студенте
	studentEntity, err := h.studentRepo.GetByID(ctx, rankEvent.StudentID)
	if err != nil {
		h.logger.Error("failed to get student",
			"student_id", rankEvent.StudentID,
			"error", err,
		)
		return fmt.Errorf("get student: %w", err)
	}

	// 2. Проверяем, можно ли отправить уведомление
	if !h.shouldNotify(studentEntity, rankEvent) {
		h.logger.Debug("skipping notification",
			"reason", "notification conditions not met",
			"student_id", rankEvent.StudentID,
		)
		return nil
	}

	// 3. Формируем и отправляем уведомление
	if err := h.sendNotification(ctx, studentEntity, rankEvent); err != nil {
		h.logger.Error("failed to send notification",
			"student_id", rankEvent.StudentID,
			"error", err,
		)
		// Не возвращаем ошибку — уведомление не критично
	}

	// 4. Инвалидируем кеш лидерборда для обновления данных
	if h.leaderboardCache != nil {
		cohort := leaderboard.Cohort(rankEvent.Cohort)
		if err := h.leaderboardCache.InvalidateCache(ctx, cohort); err != nil {
			h.logger.Warn("failed to invalidate leaderboard cache",
				"cohort", rankEvent.Cohort,
				"error", err,
			)
		}
	}

	// 5. Проверяем вход/выход из топ-N
	if err := h.checkTopNMilestones(ctx, studentEntity, rankEvent); err != nil {
		h.logger.Error("failed to check top-N milestones",
			"student_id", rankEvent.StudentID,
			"error", err,
		)
	}

	h.logger.Info("rank changed event processed successfully",
		"student_id", rankEvent.StudentID,
	)

	return nil
}

// shouldNotify определяет, нужно ли отправлять уведомление.
func (h *OnRankChangedHandler) shouldNotify(
	studentEntity *student.Student,
	event shared.RankChangedEvent,
) bool {
	// Проверяем, что студент может получать уведомления
	if !studentEntity.Status.CanReceiveNotifications() {
		return false
	}

	// Проверяем настройки студента
	if !studentEntity.Preferences.RankChanges {
		return false
	}

	// Проверяем тихие часы
	if h.config.QuietHoursEnabled {
		if studentEntity.Preferences.IsQuietHour(time.Now()) {
			return false
		}
	}

	// Проверяем минимальное изменение ранга
	absChange := event.RankChange
	if absChange < 0 {
		absChange = -absChange
	}
	if absChange < h.config.MinRankChangeForNotification {
		return false
	}

	return true
}

// sendNotification формирует и отправляет уведомление о изменении ранга.
func (h *OnRankChangedHandler) sendNotification(
	ctx context.Context,
	studentEntity *student.Student,
	event shared.RankChangedEvent,
) error {
	var notificationType notification.NotificationType
	var message string
	var priority notification.Priority

	if event.MovedUp() {
		// Студент поднялся в рейтинге 🚀
		notificationType = notification.NotificationTypeRankUp
		priority = notification.PriorityNormal

		message = h.formatRankUpMessage(event)
	} else if event.MovedDown() {
		// Студента обогнали ⚡
		notificationType = notification.NotificationTypeRankDown
		priority = notification.PriorityLow // Понижаем приоритет, чтобы не расстраивать

		message = h.formatRankDownMessage(event)
	} else {
		// Ранг не изменился — не отправляем
		return nil
	}

	// Создаём уведомление
	notif, err := notification.NewNotification(
		notification.NotificationID(generateID()),
		notificationType,
		notification.RecipientID(studentEntity.ID),
		notification.TelegramChatID(studentEntity.TelegramID),
		message,
		priority,
	)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	// Добавляем метаданные для аналитики
	notif.SetMetadata("old_rank", fmt.Sprintf("%d", event.OldRank))
	notif.SetMetadata("new_rank", fmt.Sprintf("%d", event.NewRank))
	notif.SetMetadata("rank_change", fmt.Sprintf("%d", event.RankChange))
	notif.SetMetadata("cohort", event.Cohort)

	// Отправляем уведомление
	result := h.notificationSender.Send(ctx, notif)
	if !result.Success {
		return fmt.Errorf("send notification: %w", result.Error)
	}

	h.logger.Debug("notification sent",
		"notification_id", notif.ID,
		"type", notificationType,
		"student_id", studentEntity.ID,
	)

	return nil
}

// formatRankUpMessage формирует сообщение о повышении в рейтинге.
// Философия: мотивировать, признавать усилия, но не хвастаться.
func (h *OnRankChangedHandler) formatRankUpMessage(event shared.RankChangedEvent) string {
	emoji := notification.NotificationTypeRankUp.Emoji()
	change := event.RankChange

	switch {
	case change >= 20:
		return fmt.Sprintf("%s Невероятный рывок! Ты поднялся на %d мест и теперь #%d! Твоя работа вдохновляет!",
			emoji, change, event.NewRank)

	case change >= 10:
		return fmt.Sprintf("%s Отличный прогресс! +%d мест! Теперь ты #%d в рейтинге.",
			emoji, change, event.NewRank)

	case change >= 5:
		return fmt.Sprintf("%s Ты поднялся на %d мест! Позиция #%d — так держать!",
			emoji, change, event.NewRank)

	default:
		return fmt.Sprintf("%s +%d в рейтинге! Ты теперь #%d.",
			emoji, change, event.NewRank)
	}
}

// formatRankDownMessage формирует сообщение о понижении в рейтинге.
// Философия: мягко подать, предложить поддержку, не демотивировать.
func (h *OnRankChangedHandler) formatRankDownMessage(event shared.RankChangedEvent) string {
	emoji := notification.NotificationTypeRankDown.Emoji()
	change := -event.RankChange // RankChange отрицательный при понижении

	// Мягкие формулировки, фокус на возможности улучшить
	switch {
	case event.Overtook != "" && h.config.NotifyOnOvertake:
		return fmt.Sprintf("%s Кто-то наступает на пятки! Позиция #%d. Время поднажать?",
			emoji, event.NewRank)

	case change >= 10:
		return fmt.Sprintf("%s Позиция изменилась: теперь #%d. Нужна помощь? Загляни в /help",
			emoji, event.NewRank)

	default:
		return fmt.Sprintf("%s Небольшое изменение в рейтинге: #%d. Каждая задача — шаг вперёд!",
			emoji, event.NewRank)
	}
}

// checkTopNMilestones проверяет пересечение порогов топ-N.
func (h *OnRankChangedHandler) checkTopNMilestones(
	ctx context.Context,
	studentEntity *student.Student,
	event shared.RankChangedEvent,
) error {
	for _, milestone := range h.config.TopNMilestones {
		// Вошёл в топ-N
		if event.OldRank > milestone && event.NewRank <= milestone {
			if err := h.sendTopEnteredNotification(ctx, studentEntity, milestone, event.NewRank); err != nil {
				h.logger.Error("failed to send top entered notification",
					"milestone", milestone,
					"error", err,
				)
			}
		}

		// Выпал из топ-N
		if event.OldRank <= milestone && event.NewRank > milestone {
			if err := h.sendTopLeftNotification(ctx, studentEntity, milestone, event.NewRank); err != nil {
				h.logger.Error("failed to send top left notification",
					"milestone", milestone,
					"error", err,
				)
			}
		}
	}

	return nil
}

// sendTopEnteredNotification отправляет уведомление о входе в топ-N.
func (h *OnRankChangedHandler) sendTopEnteredNotification(
	ctx context.Context,
	studentEntity *student.Student,
	topN int,
	newRank int,
) error {
	emoji := notification.NotificationTypeEnteredTop.Emoji()
	message := fmt.Sprintf("%s Поздравляем! Ты вошёл в топ-%d! Позиция #%d. Твои усилия оценены сообществом!",
		emoji, topN, newRank)

	notif, err := notification.NewNotification(
		notification.NotificationID(generateID()),
		notification.NotificationTypeEnteredTop,
		notification.RecipientID(studentEntity.ID),
		notification.TelegramChatID(studentEntity.TelegramID),
		message,
		notification.PriorityHigh, // Высокий приоритет — это важное достижение
	)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	notif.SetMetadata("top_n", fmt.Sprintf("%d", topN))
	notif.SetMetadata("new_rank", fmt.Sprintf("%d", newRank))

	result := h.notificationSender.Send(ctx, notif)
	if !result.Success {
		return fmt.Errorf("send notification: %w", result.Error)
	}

	return nil
}

// sendTopLeftNotification отправляет уведомление о выходе из топ-N.
func (h *OnRankChangedHandler) sendTopLeftNotification(
	ctx context.Context,
	studentEntity *student.Student,
	topN int,
	newRank int,
) error {
	emoji := notification.NotificationTypeLeftTop.Emoji()
	// Мягкая формулировка с акцентом на возможность вернуться
	message := fmt.Sprintf("%s Ты покинул топ-%d (сейчас #%d). Совсем немного усилий — и ты снова там! Попробуй /help для поддержки.",
		emoji, topN, newRank)

	notif, err := notification.NewNotification(
		notification.NotificationID(generateID()),
		notification.NotificationTypeLeftTop,
		notification.RecipientID(studentEntity.ID),
		notification.TelegramChatID(studentEntity.TelegramID),
		message,
		notification.PriorityLow, // Низкий приоритет — не расстраивать
	)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	notif.SetMetadata("top_n", fmt.Sprintf("%d", topN))
	notif.SetMetadata("new_rank", fmt.Sprintf("%d", newRank))

	result := h.notificationSender.Send(ctx, notif)
	if !result.Success {
		return fmt.Errorf("send notification: %w", result.Error)
	}

	return nil
}

// EventType возвращает тип события, который обрабатывает этот handler.
func (h *OnRankChangedHandler) EventType() shared.EventType {
	return shared.EventRankChanged
}
