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
	"sort"
	"strings"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════
// ON STUDENT STUCK HANDLER
// Обрабатывает событие "студент застрял" (запрос помощи).
//
// Ключевая функция философии "От конкуренции к сотрудничеству":
// - Находит студентов, которые уже решили эту задачу
// - Ранжирует их по онлайн-статусу, рейтингу помощника, прошлым связям
// - Уведомляет потенциальных помощников о запросе
// - Предлагает студенту список тех, кто может помочь
//
// Это превращает лидерборд из источника стресса в "телефонную книгу помощников".
// ═══════════════════════════════════════════════════════════════════════════

// OnStudentStuckHandler обрабатывает запросы помощи от застрявших студентов.
type OnStudentStuckHandler struct {
	// Repositories
	studentRepo     student.Repository
	activityRepo    activity.Repository
	taskIndex       activity.TaskIndex
	onlineTracker   activity.OnlineTracker
	socialRepo      social.Repository
	helpRequestRepo social.HelpRequestRepository

	// Notification sender
	notificationSender notification.NotificationSender

	// Logger
	logger *slog.Logger

	// Configuration
	config StudentStuckConfig
}

// StudentStuckConfig содержит конфигурацию обработчика.
type StudentStuckConfig struct {
	// MaxHelpersToShow — максимальное количество помощников в списке.
	MaxHelpersToShow int

	// MaxHelpersToNotify — максимальное количество помощников для уведомления.
	// Не беспокоим слишком много людей.
	MaxHelpersToNotify int

	// RecentActivityWindow — окно для определения "недавно активен".
	RecentActivityWindow time.Duration

	// PrioritizePriorHelpers — приоритизировать ли тех, кто уже помогал этому студенту.
	PrioritizePriorHelpers bool

	// MinHelperRating — минимальный рейтинг помощника для уведомления.
	MinHelperRating float64

	// NotifyOnlyOnline — уведомлять только онлайн-студентов.
	NotifyOnlyOnline bool

	// CooldownBetweenRequests — минимальный интервал между запросами помощи.
	CooldownBetweenRequests time.Duration
}

// DefaultStudentStuckConfig возвращает конфигурацию по умолчанию.
func DefaultStudentStuckConfig() StudentStuckConfig {
	return StudentStuckConfig{
		MaxHelpersToShow:        5,
		MaxHelpersToNotify:      3,
		RecentActivityWindow:    30 * time.Minute,
		PrioritizePriorHelpers:  true,
		MinHelperRating:         3.0,
		NotifyOnlyOnline:        false,
		CooldownBetweenRequests: 15 * time.Minute,
	}
}

// NewOnStudentStuckHandler создаёт новый обработчик события "студент застрял".
func NewOnStudentStuckHandler(
	studentRepo student.Repository,
	activityRepo activity.Repository,
	taskIndex activity.TaskIndex,
	onlineTracker activity.OnlineTracker,
	socialRepo social.Repository,
	helpRequestRepo social.HelpRequestRepository,
	notificationSender notification.NotificationSender,
	logger *slog.Logger,
	config StudentStuckConfig,
) *OnStudentStuckHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &OnStudentStuckHandler{
		studentRepo:        studentRepo,
		activityRepo:       activityRepo,
		taskIndex:          taskIndex,
		onlineTracker:      onlineTracker,
		socialRepo:         socialRepo,
		helpRequestRepo:    helpRequestRepo,
		notificationSender: notificationSender,
		logger:             logger.With("handler", "on_student_stuck"),
		config:             config,
	}
}

// Handle обрабатывает событие запроса помощи.
// Реализует интерфейс shared.EventHandler.
func (h *OnStudentStuckHandler) Handle(event shared.Event) error {
	ctx := context.Background()

	// Type assertion для получения конкретного типа события
	helpEvent, ok := event.(shared.HelpRequestedEvent)
	if !ok {
		h.logger.Warn("received non-HelpRequestedEvent",
			"event_type", event.EventType(),
		)
		return nil
	}

	h.logger.Info("processing help request event",
		"student_id", helpEvent.StudentID,
		"task_id", helpEvent.TaskID,
		"message", helpEvent.Message,
	)

	// 1. Получаем информацию о студенте, который просит помощь
	requestingStudent, err := h.studentRepo.GetByID(ctx, helpEvent.StudentID)
	if err != nil {
		h.logger.Error("failed to get requesting student",
			"student_id", helpEvent.StudentID,
			"error", err,
		)
		return fmt.Errorf("get student: %w", err)
	}

	// 2. Создаём запись запроса помощи
	helpRequest, err := h.createHelpRequest(ctx, helpEvent)
	if err != nil {
		h.logger.Error("failed to create help request",
			"error", err,
		)
		// Продолжаем — запись не критична
	}

	// 3. Находим потенциальных помощников
	helpers, err := h.findPotentialHelpers(ctx, helpEvent)
	if err != nil {
		h.logger.Error("failed to find helpers",
			"task_id", helpEvent.TaskID,
			"error", err,
		)
		// Продолжаем — отправим уведомление об отсутствии помощников
	}

	// 4. Ранжируем помощников по релевантности
	rankedHelpers := h.rankHelpers(ctx, helpers, helpEvent.StudentID)

	// 5. Отправляем уведомление студенту со списком помощников
	if err := h.notifyStudentWithHelpers(ctx, requestingStudent, helpEvent, rankedHelpers); err != nil {
		h.logger.Error("failed to notify student",
			"student_id", helpEvent.StudentID,
			"error", err,
		)
	}

	// 6. Уведомляем потенциальных помощников о запросе
	if len(rankedHelpers) > 0 {
		if err := h.notifyPotentialHelpers(ctx, requestingStudent, helpEvent, rankedHelpers, helpRequest); err != nil {
			h.logger.Error("failed to notify helpers",
				"error", err,
			)
		}
	}

	h.logger.Info("help request event processed successfully",
		"student_id", helpEvent.StudentID,
		"task_id", helpEvent.TaskID,
		"helpers_found", len(rankedHelpers),
	)

	return nil
}

// createHelpRequest создаёт запись о запросе помощи.
func (h *OnStudentStuckHandler) createHelpRequest(
	ctx context.Context,
	event shared.HelpRequestedEvent,
) (*social.HelpRequest, error) {
	if h.helpRequestRepo == nil {
		return nil, nil
	}

	helpRequest, err := social.NewHelpRequest(
		generateID(),
		social.StudentID(event.StudentID),
		social.TaskID(event.TaskID),
		event.Message,
	)
	if err != nil {
		return nil, fmt.Errorf("create help request: %w", err)
	}

	if err := h.helpRequestRepo.Create(ctx, helpRequest); err != nil {
		return nil, fmt.Errorf("save help request: %w", err)
	}

	return helpRequest, nil
}

// findPotentialHelpers находит студентов, которые решили эту задачу.
func (h *OnStudentStuckHandler) findPotentialHelpers(
	ctx context.Context,
	event shared.HelpRequestedEvent,
) ([]activity.StudentID, error) {
	if h.taskIndex == nil {
		return nil, nil
	}

	taskID := activity.TaskID(event.TaskID)

	// Получаем всех, кто решил задачу
	solvers, err := h.taskIndex.GetSolvers(ctx, taskID, h.config.MaxHelpersToShow*2) // Берём с запасом для фильтрации
	if err != nil {
		return nil, fmt.Errorf("get solvers: %w", err)
	}

	// Исключаем самого просящего
	filtered := make([]activity.StudentID, 0, len(solvers))
	for _, solver := range solvers {
		if string(solver) != event.StudentID {
			filtered = append(filtered, solver)
		}
	}

	return filtered, nil
}

// HelperCandidate представляет кандидата в помощники с метаданными для ранжирования.
type HelperCandidate struct {
	StudentID       string
	Student         *student.Student
	IsOnline        bool
	LastSeenAt      time.Time
	HelperRating    float64
	HelpCount       int
	HasPriorContact bool
	Score           float64 // Итоговый скор для сортировки
}

// rankHelpers ранжирует помощников по релевантности.
// Алгоритм учитывает:
// 1. Онлайн-статус (приоритет)
// 2. Прошлые связи с просящим студентом
// 3. Рейтинг как помощника
// 4. Недавнюю активность
func (h *OnStudentStuckHandler) rankHelpers(
	ctx context.Context,
	solvers []activity.StudentID,
	requestingStudentID string,
) []HelperCandidate {
	if len(solvers) == 0 {
		return nil
	}

	candidates := make([]HelperCandidate, 0, len(solvers))

	for _, solverID := range solvers {
		// Получаем информацию о студенте
		studentEntity, err := h.studentRepo.GetByID(ctx, string(solverID))
		if err != nil {
			h.logger.Debug("failed to get solver student",
				"solver_id", solverID,
				"error", err,
			)
			continue
		}

		// Проверяем, может ли студент помогать
		if !studentEntity.CanHelp() {
			continue
		}

		// Проверяем минимальный рейтинг
		if studentEntity.HelpRating > 0 && studentEntity.HelpRating < h.config.MinHelperRating {
			continue
		}

		candidate := HelperCandidate{
			StudentID:    string(solverID),
			Student:      studentEntity,
			HelperRating: studentEntity.HelpRating,
			HelpCount:    studentEntity.HelpCount,
		}

		// Проверяем онлайн-статус
		if h.onlineTracker != nil {
			isOnline, err := h.onlineTracker.IsOnline(ctx, activity.StudentID(solverID))
			if err == nil {
				candidate.IsOnline = isOnline
			}
		} else {
			candidate.IsOnline = studentEntity.OnlineState == student.OnlineStateOnline
		}

		candidate.LastSeenAt = studentEntity.LastSeenAt

		// Проверяем прошлые связи
		if h.socialRepo != nil {
			exists, err := h.socialRepo.Connections().ExistsActiveConnection(
				ctx,
				social.StudentID(requestingStudentID),
				social.StudentID(solverID),
			)
			if err == nil {
				candidate.HasPriorContact = exists
			}
		}

		// Вычисляем скор для ранжирования
		candidate.Score = h.calculateHelperScore(candidate)

		candidates = append(candidates, candidate)
	}

	// Сортируем по скору (от большего к меньшему)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Ограничиваем количество
	if len(candidates) > h.config.MaxHelpersToShow {
		candidates = candidates[:h.config.MaxHelpersToShow]
	}

	return candidates
}

// calculateHelperScore вычисляет скор релевантности помощника.
func (h *OnStudentStuckHandler) calculateHelperScore(candidate HelperCandidate) float64 {
	var score float64

	// Онлайн-статус — высший приоритет
	if candidate.IsOnline {
		score += 100
	} else {
		// Бонус за недавнюю активность
		timeSinceLastSeen := time.Since(candidate.LastSeenAt)
		if timeSinceLastSeen < h.config.RecentActivityWindow {
			score += 50
		} else if timeSinceLastSeen < time.Hour {
			score += 25
		}
	}

	// Бонус за прошлые связи (уже помогал или знаком)
	if h.config.PrioritizePriorHelpers && candidate.HasPriorContact {
		score += 30
	}

	// Бонус за рейтинг помощника (0-5 → 0-25)
	if candidate.HelperRating > 0 {
		score += candidate.HelperRating * 5
	}

	// Небольшой бонус за опыт помощи
	experienceBonus := float64(candidate.HelpCount)
	if experienceBonus > 10 {
		experienceBonus = 10 // Ограничиваем влияние
	}
	score += experienceBonus

	return score
}

// notifyStudentWithHelpers отправляет студенту список потенциальных помощников.
func (h *OnStudentStuckHandler) notifyStudentWithHelpers(
	ctx context.Context,
	requestingStudent *student.Student,
	event shared.HelpRequestedEvent,
	helpers []HelperCandidate,
) error {
	emoji := notification.NotificationTypeHelpOffer.Emoji()

	var message string
	if len(helpers) == 0 {
		// Помощники не найдены
		message = fmt.Sprintf("%s По задаче %s пока нет доступных помощников. "+
			"Попробуй позже или задай вопрос в общем чате!",
			"🔍", event.TaskID)
	} else {
		// Формируем список помощников
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s Помощь по задаче %s:\n\n", emoji, event.TaskID))

		for i, helper := range helpers {
			statusEmoji := "⚪" // оффлайн
			if helper.IsOnline {
				statusEmoji = "🟢" // онлайн
			} else if time.Since(helper.LastSeenAt) < h.config.RecentActivityWindow {
				statusEmoji = "🟡" // недавно был
			}

			// Рейтинг звёздами
			ratingStr := ""
			if helper.HelperRating > 0 {
				stars := int(helper.HelperRating + 0.5)
				ratingStr = fmt.Sprintf(" ⭐%.1f", helper.HelperRating)
				if stars >= 4 {
					ratingStr = fmt.Sprintf(" ⭐%.1f 🏆", helper.HelperRating)
				}
			}

			sb.WriteString(fmt.Sprintf("%d. %s @%s%s\n",
				i+1, statusEmoji, helper.Student.AlemLogin, ratingStr))
		}

		sb.WriteString("\nНажми на логин, чтобы написать!")
		message = sb.String()
	}

	notif, err := notification.NewNotification(
		notification.NotificationID(generateID()),
		notification.NotificationTypeHelpOffer,
		notification.RecipientID(requestingStudent.ID),
		notification.TelegramChatID(requestingStudent.TelegramID),
		message,
		notification.PriorityNormal,
	)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	notif.SetMetadata("task_id", event.TaskID)
	notif.SetMetadata("helpers_count", fmt.Sprintf("%d", len(helpers)))

	result := h.notificationSender.Send(ctx, notif)
	if !result.Success {
		return fmt.Errorf("send notification: %w", result.Error)
	}

	return nil
}

// notifyPotentialHelpers уведомляет потенциальных помощников о запросе.
func (h *OnStudentStuckHandler) notifyPotentialHelpers(
	ctx context.Context,
	requestingStudent *student.Student,
	event shared.HelpRequestedEvent,
	helpers []HelperCandidate,
	helpRequest *social.HelpRequest,
) error {
	// Уведомляем только топ-N помощников
	toNotify := helpers
	if len(toNotify) > h.config.MaxHelpersToNotify {
		toNotify = toNotify[:h.config.MaxHelpersToNotify]
	}

	// Фильтруем по онлайн-статусу, если настроено
	if h.config.NotifyOnlyOnline {
		online := make([]HelperCandidate, 0)
		for _, helper := range toNotify {
			if helper.IsOnline {
				online = append(online, helper)
			}
		}
		toNotify = online
	}

	emoji := notification.NotificationTypeHelpRequest.Emoji()

	for _, helper := range toNotify {
		// Проверяем, что помощник может получать уведомления
		if !helper.Student.Status.CanReceiveNotifications() {
			continue
		}

		if !helper.Student.Preferences.HelpRequests {
			continue
		}

		// Проверяем тихие часы
		if helper.Student.Preferences.IsQuietHour(time.Now()) {
			continue
		}

		// Формируем сообщение
		var message string
		if helper.HasPriorContact {
			// Знакомый студент
			message = fmt.Sprintf("%s %s снова просит помощь по задаче %s. Ты уже помогал ему раньше!",
				emoji, requestingStudent.DisplayName, event.TaskID)
		} else {
			message = fmt.Sprintf("%s %s просит помощь по задаче %s. Ты уже решил эту задачу — можешь помочь?",
				emoji, requestingStudent.DisplayName, event.TaskID)
		}

		// Добавляем сообщение от студента, если есть
		if event.Message != "" {
			// Ограничиваем длину сообщения
			msg := event.Message
			if len(msg) > 100 {
				msg = msg[:97] + "..."
			}
			message += fmt.Sprintf("\n\n💬 \"%s\"", msg)
		}

		notif, err := notification.NewNotification(
			notification.NotificationID(generateID()),
			notification.NotificationTypeHelpRequest,
			notification.RecipientID(helper.StudentID),
			notification.TelegramChatID(helper.Student.TelegramID),
			message,
			notification.PriorityNormal,
		)
		if err != nil {
			h.logger.Warn("failed to create helper notification",
				"helper_id", helper.StudentID,
				"error", err,
			)
			continue
		}

		notif.SetMetadata("requester_id", event.StudentID)
		notif.SetMetadata("task_id", event.TaskID)
		if helpRequest != nil {
			notif.SetMetadata("help_request_id", helpRequest.ID)
		}

		result := h.notificationSender.Send(ctx, notif)
		if !result.Success {
			h.logger.Warn("failed to send helper notification",
				"helper_id", helper.StudentID,
				"error", result.Error,
			)
		} else {
			h.logger.Debug("helper notified",
				"helper_id", helper.StudentID,
				"helper_login", helper.Student.AlemLogin,
			)
		}
	}

	return nil
}

// EventType возвращает тип события, который обрабатывает этот handler.
func (h *OnStudentStuckHandler) EventType() shared.EventType {
	return shared.EventHelpRequested
}

// ═══════════════════════════════════════════════════════════════════════════
// UTILITY FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════

// generateID генерирует уникальный идентификатор.
// В production используйте UUID или другой надёжный генератор.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
