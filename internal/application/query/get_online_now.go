// Package query contains read operations (CQRS - Queries).
package query

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/domain/activity"
	"github.com/alem-hub/alem-community-hub/internal/domain/leaderboard"
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
)

// ══════════════════════════════════════════════════════════════════════════════
// GET ONLINE NOW QUERY
// Получает список студентов, которые сейчас онлайн или были активны недавно.
// Это социальная фича: "Кто сейчас работает? Присоединяйся!"
// Помогает бороться с изоляцией и создаёт ощущение "мы вместе".
// ══════════════════════════════════════════════════════════════════════════════

// OnlineStatus определяет статус присутствия.
type OnlineStatus string

const (
	// OnlineStatusOnline - активен прямо сейчас (< 5 минут).
	OnlineStatusOnline OnlineStatus = "online"

	// OnlineStatusAway - недавно был активен (5-30 минут).
	OnlineStatusAway OnlineStatus = "away"

	// OnlineStatusRecent - был активен недавно (30 мин - 1 час).
	OnlineStatusRecent OnlineStatus = "recent"
)

// GetOnlineNowQuery содержит параметры запроса онлайн-студентов.
type GetOnlineNowQuery struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Фильтрация
	// ─────────────────────────────────────────────────────────────────────────

	// Cohort - фильтр по когорте (пустая = все).
	Cohort string

	// IncludeAway - включать студентов со статусом "away".
	IncludeAway bool

	// IncludeRecent - включать недавно активных (до 1 часа).
	IncludeRecent bool

	// OnlyAvailableForHelp - только готовые помогать.
	OnlyAvailableForHelp bool

	// ─────────────────────────────────────────────────────────────────────────
	// Пагинация
	// ─────────────────────────────────────────────────────────────────────────

	// Limit - максимальное количество (по умолчанию 50).
	Limit int

	// Offset - смещение для пагинации.
	Offset int

	// ─────────────────────────────────────────────────────────────────────────
	// Сортировка
	// ─────────────────────────────────────────────────────────────────────────

	// SortBy - поле сортировки: "last_seen", "xp", "help_rating".
	SortBy string

	// SortDesc - сортировка по убыванию.
	SortDesc bool

	// ─────────────────────────────────────────────────────────────────────────
	// Дополнительные данные
	// ─────────────────────────────────────────────────────────────────────────

	// IncludeActivity - включить информацию о текущей активности.
	IncludeActivity bool

	// IncludeRank - включить позицию в рейтинге.
	IncludeRank bool
}

// Validate проверяет корректность параметров.
func (q *GetOnlineNowQuery) Validate() error {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		return errors.New("offset cannot be negative")
	}
	if q.SortBy == "" {
		q.SortBy = "last_seen"
	}
	return nil
}

// OnlineStudentDTO - DTO для онлайн-студента.
type OnlineStudentDTO struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Идентификация
	// ─────────────────────────────────────────────────────────────────────────

	// StudentID - внутренний ID.
	StudentID string `json:"student_id"`

	// DisplayName - отображаемое имя.
	DisplayName string `json:"display_name"`

	// ─────────────────────────────────────────────────────────────────────────
	// Онлайн-статус
	// ─────────────────────────────────────────────────────────────────────────

	// Status - статус: "online", "away", "recent".
	Status OnlineStatus `json:"status"`

	// StatusEmoji - эмодзи для статуса.
	StatusEmoji string `json:"status_emoji"`

	// LastSeenAt - время последней активности.
	LastSeenAt time.Time `json:"last_seen_at"`

	// LastSeenFormatted - форматированное время.
	LastSeenFormatted string `json:"last_seen_formatted"`

	// ActiveDuration - сколько времени активен (текущая сессия).
	ActiveDuration string `json:"active_duration,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Рейтинг и уровень
	// ─────────────────────────────────────────────────────────────────────────

	// XP - текущий XP.
	XP int `json:"xp"`

	// Level - уровень.
	Level int `json:"level"`

	// Rank - позиция в рейтинге (опционально).
	Rank int `json:"rank,omitempty"`

	// RankChange - изменение позиции.
	RankChange int `json:"rank_change,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Социальная информация
	// ─────────────────────────────────────────────────────────────────────────

	// IsAvailableForHelp - готов ли помогать.
	IsAvailableForHelp bool `json:"is_available_for_help"`

	// HelpRating - рейтинг помощника.
	HelpRating float64 `json:"help_rating,omitempty"`

	// HelpCount - количество оказанных помощей.
	HelpCount int `json:"help_count,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Текущая активность (опционально)
	// ─────────────────────────────────────────────────────────────────────────

	// CurrentTask - текущая задача (если известно).
	CurrentTask string `json:"current_task,omitempty"`

	// TodayXPGained - XP заработано сегодня.
	TodayXPGained int `json:"today_xp_gained,omitempty"`

	// TodayTasksCompleted - задач выполнено сегодня.
	TodayTasksCompleted int `json:"today_tasks_completed,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Когорта
	// ─────────────────────────────────────────────────────────────────────────

	// Cohort - когорта студента.
	Cohort string `json:"cohort"`
}

// GetOnlineNowResult содержит результат запроса.
type GetOnlineNowResult struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Результаты
	// ─────────────────────────────────────────────────────────────────────────

	// Students - список онлайн-студентов.
	Students []OnlineStudentDTO `json:"students"`

	// ─────────────────────────────────────────────────────────────────────────
	// Статистика
	// ─────────────────────────────────────────────────────────────────────────

	// TotalOnline - всего студентов со статусом "online".
	TotalOnline int `json:"total_online"`

	// TotalAway - всего студентов со статусом "away".
	TotalAway int `json:"total_away"`

	// TotalRecent - всего студентов со статусом "recent".
	TotalRecent int `json:"total_recent"`

	// TotalCount - общее количество в результате (до пагинации).
	TotalCount int `json:"total_count"`

	// ─────────────────────────────────────────────────────────────────────────
	// Аналитика активности
	// ─────────────────────────────────────────────────────────────────────────

	// PeakOnlineToday - пиковое количество онлайн сегодня.
	PeakOnlineToday int `json:"peak_online_today,omitempty"`

	// AverageOnlineToday - среднее количество онлайн сегодня.
	AverageOnlineToday int `json:"average_online_today,omitempty"`

	// CommunityActivity - уровень активности: "high", "medium", "low".
	CommunityActivity string `json:"community_activity"`

	// ─────────────────────────────────────────────────────────────────────────
	// Метаданные
	// ─────────────────────────────────────────────────────────────────────────

	// Cohort - фильтр когорты.
	Cohort string `json:"cohort"`

	// GeneratedAt - время генерации.
	GeneratedAt time.Time `json:"generated_at"`

	// HasMore - есть ли ещё записи.
	HasMore bool `json:"has_more"`

	// Message - сообщение для пользователя.
	Message string `json:"message,omitempty"`
}

// GetOnlineNowHandler обрабатывает запросы онлайн-студентов.
type GetOnlineNowHandler struct {
	studentRepo     student.Repository
	onlineTracker   student.OnlineTracker
	activityRepo    activity.Repository
	leaderboardRepo leaderboard.LeaderboardRepository
}

// NewGetOnlineNowHandler создаёт новый обработчик.
func NewGetOnlineNowHandler(
	studentRepo student.Repository,
	onlineTracker student.OnlineTracker,
	activityRepo activity.Repository,
	leaderboardRepo leaderboard.LeaderboardRepository,
) *GetOnlineNowHandler {
	return &GetOnlineNowHandler{
		studentRepo:     studentRepo,
		onlineTracker:   onlineTracker,
		activityRepo:    activityRepo,
		leaderboardRepo: leaderboardRepo,
	}
}

// Handle выполняет запрос.
func (h *GetOnlineNowHandler) Handle(ctx context.Context, query GetOnlineNowQuery) (*GetOnlineNowResult, error) {
	// Валидация
	if err := query.Validate(); err != nil {
		return nil, shared.WrapError("query", "GetOnlineNow", shared.ErrValidation, err.Error(), err)
	}

	// Получаем ID онлайн-студентов
	onlineIDs, err := h.onlineTracker.GetOnlineStudents(ctx)
	if err != nil {
		return nil, shared.WrapError("query", "GetOnlineNow", shared.ErrNotFound, "failed to get online students", err)
	}

	// Получаем данные студентов
	students, err := h.studentRepo.GetByIDs(ctx, onlineIDs)
	if err != nil {
		return nil, shared.WrapError("query", "GetOnlineNow", shared.ErrNotFound, "failed to get students", err)
	}

	// Строим DTO
	dtos := make([]OnlineStudentDTO, 0, len(students))
	statsOnline, statsAway, statsRecent := 0, 0, 0

	for _, stud := range students {
		// Фильтр по когорте
		if query.Cohort != "" && string(stud.Cohort) != query.Cohort {
			continue
		}

		// Определяем статус
		status := h.determineStatus(stud.LastSeenAt)

		// Фильтр по статусу
		if status == OnlineStatusAway && !query.IncludeAway {
			continue
		}
		if status == OnlineStatusRecent && !query.IncludeRecent {
			continue
		}

		// Фильтр по готовности помогать
		if query.OnlyAvailableForHelp && !stud.CanHelp() {
			continue
		}

		dto := h.buildDTO(ctx, stud, status, query)
		dtos = append(dtos, dto)

		// Статистика
		switch status {
		case OnlineStatusOnline:
			statsOnline++
		case OnlineStatusAway:
			statsAway++
		case OnlineStatusRecent:
			statsRecent++
		}
	}

	// Сортировка
	h.sortStudents(dtos, query.SortBy, query.SortDesc)

	// Пагинация
	totalCount := len(dtos)
	hasMore := false

	if query.Offset < len(dtos) {
		end := query.Offset + query.Limit
		if end > len(dtos) {
			end = len(dtos)
		} else {
			hasMore = true
		}
		dtos = dtos[query.Offset:end]
	} else {
		dtos = []OnlineStudentDTO{}
	}

	// Определяем уровень активности
	communityActivity := h.determineCommunityActivity(statsOnline)

	// Генерируем сообщение
	message := h.generateMessage(statsOnline, statsAway, communityActivity)

	return &GetOnlineNowResult{
		Students:          dtos,
		TotalOnline:       statsOnline,
		TotalAway:         statsAway,
		TotalRecent:       statsRecent,
		TotalCount:        totalCount,
		CommunityActivity: communityActivity,
		Cohort:            query.Cohort,
		GeneratedAt:       time.Now().UTC(),
		HasMore:           hasMore,
		Message:           message,
	}, nil
}

// determineStatus определяет онлайн-статус по времени последней активности.
func (h *GetOnlineNowHandler) determineStatus(lastSeen time.Time) OnlineStatus {
	elapsed := time.Since(lastSeen)

	switch {
	case elapsed < 5*time.Minute:
		return OnlineStatusOnline
	case elapsed < 30*time.Minute:
		return OnlineStatusAway
	default:
		return OnlineStatusRecent
	}
}

// buildDTO строит DTO для студента.
func (h *GetOnlineNowHandler) buildDTO(
	ctx context.Context,
	stud *student.Student,
	status OnlineStatus,
	query GetOnlineNowQuery,
) OnlineStudentDTO {
	dto := OnlineStudentDTO{
		StudentID:          stud.ID,
		DisplayName:        stud.DisplayName,
		Status:             status,
		StatusEmoji:        getStatusEmoji(status),
		LastSeenAt:         stud.LastSeenAt,
		LastSeenFormatted:  formatOnlineTime(stud.LastSeenAt),
		XP:                 int(stud.CurrentXP),
		Level:              int(stud.Level()),
		IsAvailableForHelp: stud.CanHelp(),
		HelpRating:         stud.HelpRating,
		HelpCount:          stud.HelpCount,
		Cohort:             string(stud.Cohort),
	}

	// Добавляем ранг если запрошено
	if query.IncludeRank && h.leaderboardRepo != nil {
		entry, err := h.leaderboardRepo.GetStudentRank(ctx, stud.ID, leaderboard.Cohort(query.Cohort))
		if err == nil && entry != nil {
			dto.Rank = int(entry.Rank)
			dto.RankChange = int(entry.RankChange)
		}
	}

	// Добавляем активность если запрошено
	if query.IncludeActivity && h.activityRepo != nil {
		h.enrichWithActivity(ctx, &dto, stud.ID)
	}

	return dto
}

// enrichWithActivity добавляет информацию об активности.
func (h *GetOnlineNowHandler) enrichWithActivity(ctx context.Context, dto *OnlineStudentDTO, studentID string) {
	progress, err := h.activityRepo.GetDailyProgress(ctx, activity.StudentID(studentID), time.Now().UTC())
	if err == nil && progress != nil {
		dto.TodayXPGained = progress.XPEarned
		dto.TodayTasksCompleted = progress.TasksCompleted
	}
}

// sortStudents сортирует студентов.
func (h *GetOnlineNowHandler) sortStudents(students []OnlineStudentDTO, sortBy string, desc bool) {
	sort.Slice(students, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "xp":
			less = students[i].XP < students[j].XP
		case "help_rating":
			less = students[i].HelpRating < students[j].HelpRating
		case "rank":
			less = students[i].Rank < students[j].Rank
		default: // "last_seen"
			less = students[i].LastSeenAt.Before(students[j].LastSeenAt)
		}

		if desc {
			return !less
		}
		return less
	})
}

// determineCommunityActivity определяет уровень активности сообщества.
func (h *GetOnlineNowHandler) determineCommunityActivity(onlineCount int) string {
	switch {
	case onlineCount >= 20:
		return "high"
	case onlineCount >= 5:
		return "medium"
	default:
		return "low"
	}
}

// generateMessage генерирует сообщение для пользователя.
func (h *GetOnlineNowHandler) generateMessage(online, away int, activity string) string {
	total := online + away

	switch activity {
	case "high":
		return "🔥 Сообщество активно! Самое время учиться вместе."
	case "medium":
		if online > 0 {
			return "🟢 Есть студенты онлайн. Не стесняйся обратиться за помощью!"
		}
		return "🟡 Несколько человек были активны недавно."
	default:
		if total == 0 {
			return "😴 Сейчас тихо. Ты можешь быть первым!"
		}
		return "💡 Немного людей онлайн, но можешь написать кому-нибудь!"
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// getStatusEmoji возвращает эмодзи для статуса.
func getStatusEmoji(status OnlineStatus) string {
	switch status {
	case OnlineStatusOnline:
		return "🟢"
	case OnlineStatusAway:
		return "🟡"
	case OnlineStatusRecent:
		return "⚪"
	default:
		return "⚪"
	}
}

// formatOnlineTime форматирует время онлайн.
func formatOnlineTime(t time.Time) string {
	elapsed := time.Since(t)

	switch {
	case elapsed < time.Minute:
		return "только что"
	case elapsed < 5*time.Minute:
		return "пару минут назад"
	case elapsed < 30*time.Minute:
		mins := int(elapsed.Minutes())
		return formatMinutesRu(mins) + " назад"
	case elapsed < time.Hour:
		return "полчаса назад"
	case elapsed < 2*time.Hour:
		return "час назад"
	default:
		hours := int(elapsed.Hours())
		return formatHoursRu(hours) + " назад"
	}
}

// formatMinutesRu форматирует минуты на русском.
func formatMinutesRu(m int) string {
	if m%10 == 1 && m%100 != 11 {
		return string(rune('0'+m/10)) + string(rune('0'+m%10)) + " минуту"
	}
	if m%10 >= 2 && m%10 <= 4 && (m%100 < 10 || m%100 >= 20) {
		return string(rune('0'+m/10)) + string(rune('0'+m%10)) + " минуты"
	}
	return string(rune('0'+m/10)) + string(rune('0'+m%10)) + " минут"
}

// formatHoursRu форматирует часы на русском.
func formatHoursRu(h int) string {
	if h%10 == 1 && h%100 != 11 {
		return string(rune('0'+h/10)) + string(rune('0'+h%10)) + " час"
	}
	if h%10 >= 2 && h%10 <= 4 && (h%100 < 10 || h%100 >= 20) {
		return string(rune('0'+h/10)) + string(rune('0'+h%10)) + " часа"
	}
	return string(rune('0'+h/10)) + string(rune('0'+h%10)) + " часов"
}
