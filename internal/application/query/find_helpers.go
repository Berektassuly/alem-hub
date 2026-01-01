// Package query contains read operations (CQRS - Queries).
package query

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/activity"
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/social"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"context"
	"errors"
	"sort"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// FIND HELPERS QUERY
// Находит студентов, которые могут помочь с конкретной задачей.
// Это КЛЮЧЕВОЙ запрос проекта, реализующий философию:
// "От конкуренции к сотрудничеству".
//
// Лидерборд становится не просто рейтингом, а "телефонной книгой помощников".
// ══════════════════════════════════════════════════════════════════════════════

// FindHelpersQuery содержит параметры поиска помощников.
type FindHelpersQuery struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Обязательные параметры
	// ─────────────────────────────────────────────────────────────────────────

	// RequesterID - ID студента, который ищет помощь.
	RequesterID string

	// RequesterTelegramID - альтернативная идентификация.
	RequesterTelegramID int64

	// TaskID - ID задачи, по которой нужна помощь.
	TaskID string

	// ─────────────────────────────────────────────────────────────────────────
	// Опциональные параметры
	// ─────────────────────────────────────────────────────────────────────────

	// Limit - максимальное количество результатов (по умолчанию 5).
	Limit int

	// PreferOnline - предпочитать онлайн студентов.
	PreferOnline bool

	// PreferKnownHelpers - предпочитать тех, кто уже помогал ранее.
	PreferKnownHelpers bool

	// MinHelpRating - минимальный рейтинг помощника (0.0 - 5.0).
	MinHelpRating float64

	// MaxResponseTime - максимальное время с последней активности.
	MaxResponseTime time.Duration

	// Cohort - фильтр по когорте (пустая = все).
	Cohort string
}

// Validate проверяет корректность параметров.
func (q *FindHelpersQuery) Validate() error {
	if q.RequesterID == "" && q.RequesterTelegramID == 0 {
		return errors.New("requester identification is required")
	}
	if q.TaskID == "" {
		return errors.New("task_id is required")
	}
	if q.Limit <= 0 {
		q.Limit = 5
	}
	if q.Limit > 20 {
		q.Limit = 20
	}
	if q.MinHelpRating < 0 || q.MinHelpRating > 5 {
		return errors.New("min_help_rating must be between 0 and 5")
	}
	if q.MaxResponseTime == 0 {
		q.MaxResponseTime = 24 * time.Hour // По умолчанию - активность за последние 24 часа
	}
	return nil
}

// HelperDTO - DTO для потенциального помощника.
type HelperDTO struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Идентификация
	// ─────────────────────────────────────────────────────────────────────────

	// StudentID - внутренний ID.
	StudentID string `json:"student_id"`

	// DisplayName - отображаемое имя.
	DisplayName string `json:"display_name"`

	// TelegramUsername - Telegram username для связи.
	TelegramUsername string `json:"telegram_username,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Информация о задаче
	// ─────────────────────────────────────────────────────────────────────────

	// CompletedTaskAt - когда решил эту задачу.
	CompletedTaskAt time.Time `json:"completed_task_at"`

	// TimeSinceCompletion - сколько прошло с решения.
	TimeSinceCompletion string `json:"time_since_completion"`

	// ─────────────────────────────────────────────────────────────────────────
	// Онлайн-статус
	// ─────────────────────────────────────────────────────────────────────────

	// IsOnline - онлайн ли сейчас.
	IsOnline bool `json:"is_online"`

	// OnlineStatus - статус: "online", "away", "offline".
	OnlineStatus string `json:"online_status"`

	// LastSeenAt - время последней активности.
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`

	// LastSeenFormatted - форматированное время.
	LastSeenFormatted string `json:"last_seen_formatted,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Рейтинг помощника
	// ─────────────────────────────────────────────────────────────────────────

	// HelpRating - средний рейтинг как помощника (0.0 - 5.0).
	HelpRating float64 `json:"help_rating"`

	// HelpRatingFormatted - форматированный рейтинг с звёздами.
	HelpRatingFormatted string `json:"help_rating_formatted"`

	// TotalHelpCount - сколько раз помогал другим.
	TotalHelpCount int `json:"total_help_count"`

	// ─────────────────────────────────────────────────────────────────────────
	// История взаимодействий
	// ─────────────────────────────────────────────────────────────────────────

	// HasPriorContact - помогал ли этому студенту раньше.
	HasPriorContact bool `json:"has_prior_contact"`

	// PriorHelpCount - сколько раз помогал именно этому студенту.
	PriorHelpCount int `json:"prior_help_count,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Уровень и XP (контекст)
	// ─────────────────────────────────────────────────────────────────────────

	// Level - уровень студента.
	Level int `json:"level"`

	// XP - текущий XP.
	XP int `json:"xp"`

	// Rank - позиция в рейтинге.
	Rank int `json:"rank,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Скоринг
	// ─────────────────────────────────────────────────────────────────────────

	// Score - внутренний скор для ранжирования (чем выше, тем лучше).
	Score float64 `json:"score"`

	// ScoreBreakdown - разбивка скора по факторам.
	ScoreBreakdown map[string]float64 `json:"score_breakdown,omitempty"`

	// RecommendationReason - почему рекомендуем этого помощника.
	RecommendationReason string `json:"recommendation_reason,omitempty"`
}

// FindHelpersResult содержит результат поиска помощников.
type FindHelpersResult struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Результаты
	// ─────────────────────────────────────────────────────────────────────────

	// Helpers - список потенциальных помощников, отсортированный по релевантности.
	Helpers []HelperDTO `json:"helpers"`

	// TotalFound - сколько всего найдено (до применения лимита).
	TotalFound int `json:"total_found"`

	// ─────────────────────────────────────────────────────────────────────────
	// Контекст задачи
	// ─────────────────────────────────────────────────────────────────────────

	// TaskID - ID задачи.
	TaskID string `json:"task_id"`

	// TotalSolvers - сколько студентов решило эту задачу.
	TotalSolvers int `json:"total_solvers"`

	// OnlineSolvers - сколько из них сейчас онлайн.
	OnlineSolvers int `json:"online_solvers"`

	// ─────────────────────────────────────────────────────────────────────────
	// Метаданные
	// ─────────────────────────────────────────────────────────────────────────

	// GeneratedAt - время генерации результата.
	GeneratedAt time.Time `json:"generated_at"`

	// SearchCriteria - использованные критерии поиска.
	SearchCriteria FindHelpersSearchCriteria `json:"search_criteria"`

	// Message - сообщение пользователю.
	Message string `json:"message,omitempty"`
}

// FindHelpersSearchCriteria - использованные критерии поиска.
type FindHelpersSearchCriteria struct {
	TaskID          string        `json:"task_id"`
	PreferOnline    bool          `json:"prefer_online"`
	MinHelpRating   float64       `json:"min_help_rating"`
	MaxResponseTime time.Duration `json:"max_response_time"`
}

// FindHelpersHandler обрабатывает запросы на поиск помощников.
type FindHelpersHandler struct {
	studentRepo   student.Repository
	activityRepo  activity.Repository
	onlineTracker activity.OnlineTracker
	taskIndex     activity.TaskIndex
	socialRepo    social.Repository
}

// NewFindHelpersHandler создаёт новый обработчик.
func NewFindHelpersHandler(
	studentRepo student.Repository,
	activityRepo activity.Repository,
	onlineTracker activity.OnlineTracker,
	taskIndex activity.TaskIndex,
	socialRepo social.Repository,
) *FindHelpersHandler {
	return &FindHelpersHandler{
		studentRepo:   studentRepo,
		activityRepo:  activityRepo,
		onlineTracker: onlineTracker,
		taskIndex:     taskIndex,
		socialRepo:    socialRepo,
	}
}

// Handle выполняет поиск помощников.
func (h *FindHelpersHandler) Handle(ctx context.Context, query FindHelpersQuery) (*FindHelpersResult, error) {
	// Валидация
	if err := query.Validate(); err != nil {
		return nil, shared.WrapError("query", "FindHelpers", shared.ErrValidation, err.Error(), err)
	}

	// Получаем ID запрашивающего студента
	requesterID, err := h.getRequesterID(ctx, query)
	if err != nil {
		return nil, err
	}

	// Получаем студентов, решивших задачу
	taskID := activity.TaskID(query.TaskID)
	solverIDs, err := h.taskIndex.GetSolvers(ctx, taskID, 100) // Берём больше, потом фильтруем
	if err != nil {
		return nil, shared.WrapError("query", "FindHelpers", shared.ErrNotFound, "failed to get solvers", err)
	}

	if len(solverIDs) == 0 {
		return &FindHelpersResult{
			Helpers:       []HelperDTO{},
			TotalFound:    0,
			TaskID:        query.TaskID,
			TotalSolvers:  0,
			OnlineSolvers: 0,
			GeneratedAt:   time.Now().UTC(),
			Message:       "Пока никто не решил эту задачу. Ты можешь стать первым! 💪",
		}, nil
	}

	// Фильтруем и обогащаем данные
	helpers, err := h.buildHelpersList(ctx, solverIDs, requesterID, query)
	if err != nil {
		return nil, err
	}

	// Сортируем по скору
	sort.Slice(helpers, func(i, j int) bool {
		return helpers[i].Score > helpers[j].Score
	})

	// Подсчёт статистики до обрезки
	totalFound := len(helpers)
	onlineSolvers := 0
	for _, h := range helpers {
		if h.IsOnline {
			onlineSolvers++
		}
	}

	// Применяем лимит
	if len(helpers) > query.Limit {
		helpers = helpers[:query.Limit]
	}

	// Генерируем сообщение
	message := h.generateMessage(helpers, onlineSolvers, totalFound)

	return &FindHelpersResult{
		Helpers:       helpers,
		TotalFound:    totalFound,
		TaskID:        query.TaskID,
		TotalSolvers:  len(solverIDs),
		OnlineSolvers: onlineSolvers,
		GeneratedAt:   time.Now().UTC(),
		SearchCriteria: FindHelpersSearchCriteria{
			TaskID:          query.TaskID,
			PreferOnline:    query.PreferOnline,
			MinHelpRating:   query.MinHelpRating,
			MaxResponseTime: query.MaxResponseTime,
		},
		Message: message,
	}, nil
}

// getRequesterID получает ID запрашивающего студента.
func (h *FindHelpersHandler) getRequesterID(ctx context.Context, query FindHelpersQuery) (string, error) {
	if query.RequesterID != "" {
		return query.RequesterID, nil
	}

	stud, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(query.RequesterTelegramID))
	if err != nil {
		return "", shared.WrapError("query", "FindHelpers", shared.ErrNotFound, "requester not found", err)
	}
	return stud.ID, nil
}

// buildHelpersList строит список помощников с полной информацией.
func (h *FindHelpersHandler) buildHelpersList(
	ctx context.Context,
	solverIDs []activity.StudentID,
	requesterID string,
	query FindHelpersQuery,
) ([]HelperDTO, error) {
	helpers := make([]HelperDTO, 0, len(solverIDs))

	for _, solverID := range solverIDs {
		// Пропускаем самого себя
		if string(solverID) == requesterID {
			continue
		}

		helper, err := h.buildHelperDTO(ctx, string(solverID), requesterID, query)
		if err != nil {
			continue // Пропускаем при ошибке
		}

		// Фильтруем по минимальному рейтингу
		if query.MinHelpRating > 0 && helper.HelpRating < query.MinHelpRating {
			continue
		}

		// Фильтруем по времени последней активности
		if helper.LastSeenAt != nil && query.MaxResponseTime > 0 {
			if time.Since(*helper.LastSeenAt) > query.MaxResponseTime {
				continue
			}
		}

		helpers = append(helpers, *helper)
	}

	return helpers, nil
}

// buildHelperDTO строит DTO для одного помощника.
func (h *FindHelpersHandler) buildHelperDTO(
	ctx context.Context,
	helperID string,
	requesterID string,
	query FindHelpersQuery,
) (*HelperDTO, error) {
	// Получаем данные студента
	stud, err := h.studentRepo.GetByID(ctx, helperID)
	if err != nil {
		return nil, err
	}

	// Получаем информацию о решении задачи
	taskID := activity.TaskID(query.TaskID)
	completion, err := h.getTaskCompletion(ctx, helperID, taskID)

	// Проверяем онлайн-статус
	isOnline := false
	onlineStatus := "offline"
	if h.onlineTracker != nil {
		isOnline, _ = h.onlineTracker.IsOnline(ctx, activity.StudentID(helperID))
		if isOnline {
			onlineStatus = "online"
		} else if stud.LastSeenAt.After(time.Now().Add(-30 * time.Minute)) {
			onlineStatus = "away"
		}
	}

	// Проверяем прошлые взаимодействия
	hasPriorContact, priorHelpCount := h.checkPriorContact(ctx, helperID, requesterID)

	// Вычисляем скор
	score, breakdown := h.calculateScore(stud, isOnline, hasPriorContact, completion)

	dto := &HelperDTO{
		StudentID:           stud.ID,
		DisplayName:         stud.DisplayName,
		IsOnline:            isOnline,
		OnlineStatus:        onlineStatus,
		HelpRating:          stud.HelpRating,
		HelpRatingFormatted: formatHelpRating(stud.HelpRating),
		TotalHelpCount:      stud.HelpCount,
		HasPriorContact:     hasPriorContact,
		PriorHelpCount:      priorHelpCount,
		Level:               int(stud.Level()),
		XP:                  int(stud.CurrentXP),
		Score:               score,
		ScoreBreakdown:      breakdown,
	}

	// Last seen
	if !stud.LastSeenAt.IsZero() {
		dto.LastSeenAt = &stud.LastSeenAt
		dto.LastSeenFormatted = formatLastSeen(stud.LastSeenAt)
	}

	// Completion time
	if completion != nil {
		dto.CompletedTaskAt = completion.CompletedAt
		dto.TimeSinceCompletion = formatTimeSince(completion.CompletedAt)
	}

	// Reason
	dto.RecommendationReason = h.generateRecommendationReason(dto)

	return dto, nil
}

// getTaskCompletion получает информацию о завершении задачи.
func (h *FindHelpersHandler) getTaskCompletion(ctx context.Context, studentID string, taskID activity.TaskID) (*activity.TaskCompletion, error) {
	completions, err := h.activityRepo.GetTaskCompletionsByStudent(ctx, activity.StudentID(studentID), 100)
	if err != nil {
		return nil, err
	}

	for _, c := range completions {
		if c.TaskID == taskID {
			return c, nil
		}
	}

	return nil, nil
}

// checkPriorContact проверяет, есть ли история взаимодействий.
func (h *FindHelpersHandler) checkPriorContact(ctx context.Context, helperID, requesterID string) (bool, int) {
	if h.socialRepo == nil {
		return false, 0
	}

	connRepo := h.socialRepo.Connections()
	conn, err := connRepo.GetByStudents(ctx, social.StudentID(helperID), social.StudentID(requesterID))
	if err != nil || conn == nil {
		return false, 0
	}

	return true, conn.Stats.InteractionCount
}

// calculateScore вычисляет скор помощника для ранжирования.
func (h *FindHelpersHandler) calculateScore(
	stud *student.Student,
	isOnline bool,
	hasPriorContact bool,
	completion *activity.TaskCompletion,
) (float64, map[string]float64) {
	breakdown := make(map[string]float64)
	score := 0.0

	// Онлайн-статус (максимум 30 баллов)
	if isOnline {
		breakdown["online"] = 30.0
		score += 30.0
	} else if !stud.LastSeenAt.IsZero() {
		elapsed := time.Since(stud.LastSeenAt)
		if elapsed < 5*time.Minute {
			breakdown["online"] = 25.0
			score += 25.0
		} else if elapsed < 30*time.Minute {
			breakdown["online"] = 15.0
			score += 15.0
		} else if elapsed < time.Hour {
			breakdown["online"] = 5.0
			score += 5.0
		}
	}

	// Рейтинг помощника (максимум 25 баллов)
	ratingScore := stud.HelpRating * 5.0 // 0-25
	breakdown["rating"] = ratingScore
	score += ratingScore

	// История помощи (максимум 20 баллов)
	if stud.HelpCount > 0 {
		helpScore := float64(stud.HelpCount)
		if helpScore > 20 {
			helpScore = 20
		}
		breakdown["help_history"] = helpScore
		score += helpScore
	}

	// Предыдущий контакт (бонус 15 баллов)
	if hasPriorContact {
		breakdown["prior_contact"] = 15.0
		score += 15.0
	}

	// Свежесть решения задачи (максимум 10 баллов)
	if completion != nil {
		elapsed := time.Since(completion.CompletedAt)
		if elapsed < 24*time.Hour {
			breakdown["recency"] = 10.0
			score += 10.0
		} else if elapsed < 7*24*time.Hour {
			breakdown["recency"] = 5.0
			score += 5.0
		}
	}

	return score, breakdown
}

// generateRecommendationReason генерирует причину рекомендации.
func (h *FindHelpersHandler) generateRecommendationReason(dto *HelperDTO) string {
	if dto.IsOnline && dto.HelpRating >= 4.5 {
		return "🌟 Онлайн + отличный рейтинг"
	}
	if dto.IsOnline {
		return "🟢 Сейчас онлайн"
	}
	if dto.HasPriorContact {
		return "🤝 Помогал тебе раньше"
	}
	if dto.HelpRating >= 4.5 {
		return "⭐ Высокий рейтинг помощника"
	}
	if dto.TotalHelpCount >= 10 {
		return "💪 Опытный помощник"
	}
	if dto.OnlineStatus == "away" {
		return "🟡 Недавно был активен"
	}
	return "✅ Решил эту задачу"
}

// generateMessage генерирует сообщение для пользователя.
func (h *FindHelpersHandler) generateMessage(helpers []HelperDTO, onlineSolvers, totalFound int) string {
	if len(helpers) == 0 {
		return "🔍 Не удалось найти доступных помощников. Попробуй позже!"
	}

	if onlineSolvers > 0 {
		return "🟢 Найдены помощники онлайн! Напиши им прямо сейчас."
	}

	if totalFound > len(helpers) {
		return "📋 Показаны лучшие кандидаты. Всего найдено: " + string(rune(totalFound))
	}

	return "👋 Напиши любому из них — комьюнити всегда поможет!"
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// formatHelpRating форматирует рейтинг со звёздами.
func formatHelpRating(rating float64) string {
	if rating == 0 {
		return "Новичок"
	}

	fullStars := int(rating)
	hasHalf := rating-float64(fullStars) >= 0.5

	result := ""
	for i := 0; i < fullStars; i++ {
		result += "⭐"
	}
	if hasHalf {
		result += "✨"
	}

	return result
}

// formatLastSeen форматирует время последней активности.
func formatLastSeen(t time.Time) string {
	elapsed := time.Since(t)

	switch {
	case elapsed < time.Minute:
		return "только что"
	case elapsed < time.Hour:
		return formatMinutes(int(elapsed.Minutes())) + " назад"
	case elapsed < 24*time.Hour:
		return formatHours(int(elapsed.Hours())) + " назад"
	default:
		return formatDays(int(elapsed.Hours()/24)) + " назад"
	}
}

// formatTimeSince форматирует время с момента события.
func formatTimeSince(t time.Time) string {
	elapsed := time.Since(t)

	switch {
	case elapsed < time.Hour:
		return formatMinutes(int(elapsed.Minutes())) + " назад"
	case elapsed < 24*time.Hour:
		return formatHours(int(elapsed.Hours())) + " назад"
	case elapsed < 7*24*time.Hour:
		return formatDays(int(elapsed.Hours()/24)) + " назад"
	default:
		return t.Format("02.01.2006")
	}
}

func formatMinutes(m int) string {
	if m == 1 {
		return "1 минуту"
	}
	if m >= 2 && m <= 4 {
		return string(rune('0'+m)) + " минуты"
	}
	return string(rune('0'+m/10)) + string(rune('0'+m%10)) + " минут"
}

func formatHours(h int) string {
	if h == 1 {
		return "1 час"
	}
	if h >= 2 && h <= 4 {
		return string(rune('0'+h)) + " часа"
	}
	return string(rune('0'+h/10)) + string(rune('0'+h%10)) + " часов"
}

func formatDays(d int) string {
	if d == 1 {
		return "1 день"
	}
	if d >= 2 && d <= 4 {
		return string(rune('0'+d)) + " дня"
	}
	return string(rune('0'+d/10)) + string(rune('0'+d%10)) + " дней"
}
