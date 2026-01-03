// Package query contains read operations (CQRS - Queries).
package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/domain/leaderboard"
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
)

// ══════════════════════════════════════════════════════════════════════════════
// GET DAILY PROGRESS QUERY
// Получает статистику "Daily Grind" - дневной прогресс студента.
// Это ключевая мотивационная фича: показывает прогресс за сегодня,
// серии активности, и даёт ощущение движения вперёд.
//
// Философия: "Маленькие шаги каждый день = большой путь".
// ══════════════════════════════════════════════════════════════════════════════

// GetDailyProgressQuery содержит параметры запроса дневного прогресса.
type GetDailyProgressQuery struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Идентификация
	// ─────────────────────────────────────────────────────────────────────────

	// StudentID - внутренний ID студента.
	StudentID string

	// TelegramID - альтернативная идентификация.
	TelegramID int64

	// ─────────────────────────────────────────────────────────────────────────
	// Период
	// ─────────────────────────────────────────────────────────────────────────

	// Date - дата для которой получить прогресс (пустая = сегодня).
	Date time.Time

	// IncludeHistory - включить историю за последние N дней.
	IncludeHistory bool

	// HistoryDays - за сколько дней показывать историю (по умолчанию 7).
	HistoryDays int

	// ─────────────────────────────────────────────────────────────────────────
	// Дополнительные данные
	// ─────────────────────────────────────────────────────────────────────────

	// IncludeRankProgress - включить изменение позиции за день.
	IncludeRankProgress bool

	// IncludeStreak - включить информацию о серии.
	IncludeStreak bool

	// IncludeAchievements - включить достижения за период.
	IncludeAchievements bool

	// IncludeComparison - включить сравнение со вчера.
	IncludeComparison bool
}

// Validate проверяет корректность параметров.
func (q *GetDailyProgressQuery) Validate() error {
	if q.StudentID == "" && q.TelegramID == 0 {
		return errors.New("either student_id or telegram_id must be provided")
	}
	if q.Date.IsZero() {
		q.Date = time.Now().UTC()
	}
	if q.HistoryDays <= 0 {
		q.HistoryDays = 7
	}
	if q.HistoryDays > 30 {
		q.HistoryDays = 30
	}
	return nil
}

// DailyProgressDTO - DTO для дневного прогресса.
type DailyProgressDTO struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Основная информация
	// ─────────────────────────────────────────────────────────────────────────

	// Date - дата.
	Date time.Time `json:"date"`

	// DateFormatted - форматированная дата.
	DateFormatted string `json:"date_formatted"`

	// IsToday - это сегодняшний день.
	IsToday bool `json:"is_today"`

	// ─────────────────────────────────────────────────────────────────────────
	// XP и задачи
	// ─────────────────────────────────────────────────────────────────────────

	// XPStart - XP на начало дня.
	XPStart int `json:"xp_start"`

	// XPCurrent - текущий XP.
	XPCurrent int `json:"xp_current"`

	// XPGained - заработано XP за день.
	XPGained int `json:"xp_gained"`

	// XPGainedFormatted - форматированный XP.
	XPGainedFormatted string `json:"xp_gained_formatted"`

	// TasksCompleted - задач выполнено.
	TasksCompleted int `json:"tasks_completed"`

	// ─────────────────────────────────────────────────────────────────────────
	// Время активности
	// ─────────────────────────────────────────────────────────────────────────

	// SessionsCount - количество сессий.
	SessionsCount int `json:"sessions_count"`

	// TotalActiveMinutes - общее время активности в минутах.
	TotalActiveMinutes int `json:"total_active_minutes"`

	// ActiveTimeFormatted - форматированное время активности.
	ActiveTimeFormatted string `json:"active_time_formatted"`

	// FirstActivityAt - первая активность за день.
	FirstActivityAt *time.Time `json:"first_activity_at,omitempty"`

	// LastActivityAt - последняя активность за день.
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Позиция в рейтинге
	// ─────────────────────────────────────────────────────────────────────────

	// RankAtStart - позиция на начало дня.
	RankAtStart int `json:"rank_at_start,omitempty"`

	// RankCurrent - текущая позиция.
	RankCurrent int `json:"rank_current,omitempty"`

	// RankChange - изменение позиции за день (+ = поднялся).
	RankChange int `json:"rank_change,omitempty"`

	// RankChangeFormatted - форматированное изменение.
	RankChangeFormatted string `json:"rank_change_formatted,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Серия (streak)
	// ─────────────────────────────────────────────────────────────────────────

	// StreakDay - какой это день серии (0 = серия сброшена).
	StreakDay int `json:"streak_day"`

	// StreakDayFormatted - форматированный день серии.
	StreakDayFormatted string `json:"streak_day_formatted,omitempty"`

	// StreakStatus - статус серии: "active", "at_risk", "broken".
	StreakStatus string `json:"streak_status,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Оценка дня
	// ─────────────────────────────────────────────────────────────────────────

	// DayRating - оценка дня: "excellent", "good", "normal", "low", "inactive".
	DayRating string `json:"day_rating"`

	// DayRatingEmoji - эмодзи для оценки.
	DayRatingEmoji string `json:"day_rating_emoji"`

	// ProgressPercent - прогресс к дневной цели (0-100+).
	ProgressPercent int `json:"progress_percent"`

	// IsActive - была ли активность.
	IsActive bool `json:"is_active"`
}

// StreakInfoDTO - информация о серии.
type StreakInfoDTO struct {
	// CurrentStreak - текущая серия дней.
	CurrentStreak int `json:"current_streak"`

	// BestStreak - лучшая серия за всё время.
	BestStreak int `json:"best_streak"`

	// LastActiveDate - дата последней активности.
	LastActiveDate time.Time `json:"last_active_date"`

	// StreakStartDate - начало текущей серии.
	StreakStartDate time.Time `json:"streak_start_date,omitempty"`

	// IsAtRisk - серия под угрозой (сегодня ещё не было активности).
	IsAtRisk bool `json:"is_at_risk"`

	// HoursToSaveStreak - часов до потери серии.
	HoursToSaveStreak int `json:"hours_to_save_streak,omitempty"`

	// StreakMessage - мотивационное сообщение о серии.
	StreakMessage string `json:"streak_message,omitempty"`
}

// ComparisonDTO - сравнение с предыдущим днём.
type ComparisonDTO struct {
	// XPDifference - разница в XP.
	XPDifference int `json:"xp_difference"`

	// TasksDifference - разница в задачах.
	TasksDifference int `json:"tasks_difference"`

	// ActiveTimeDifference - разница во времени активности (минуты).
	ActiveTimeDifference int `json:"active_time_difference"`

	// Trend - тренд: "better", "same", "worse".
	Trend string `json:"trend"`

	// TrendEmoji - эмодзи тренда.
	TrendEmoji string `json:"trend_emoji"`

	// Message - сообщение о сравнении.
	Message string `json:"message"`
}

// GetDailyProgressResult содержит результат запроса.
type GetDailyProgressResult struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Информация о студенте
	// ─────────────────────────────────────────────────────────────────────────

	// StudentID - ID студента.
	StudentID string `json:"student_id"`

	// DisplayName - отображаемое имя.
	DisplayName string `json:"display_name"`

	// ─────────────────────────────────────────────────────────────────────────
	// Прогресс
	// ─────────────────────────────────────────────────────────────────────────

	// Today - прогресс за сегодня (или запрошенный день).
	Today DailyProgressDTO `json:"today"`

	// History - история за последние дни (если запрошена).
	History []DailyProgressDTO `json:"history,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Серия
	// ─────────────────────────────────────────────────────────────────────────

	// Streak - информация о серии (если запрошена).
	Streak *StreakInfoDTO `json:"streak,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Сравнение
	// ─────────────────────────────────────────────────────────────────────────

	// Comparison - сравнение со вчера (если запрошено).
	Comparison *ComparisonDTO `json:"comparison,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Общая статистика
	// ─────────────────────────────────────────────────────────────────────────

	// TotalXP - общий XP студента.
	TotalXP int `json:"total_xp"`

	// Level - текущий уровень.
	Level int `json:"level"`

	// WeeklyXP - XP за последнюю неделю.
	WeeklyXP int `json:"weekly_xp,omitempty"`

	// WeeklyTasks - задач за неделю.
	WeeklyTasks int `json:"weekly_tasks,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Метаданные
	// ─────────────────────────────────────────────────────────────────────────

	// GeneratedAt - время генерации.
	GeneratedAt time.Time `json:"generated_at"`

	// MotivationalMessage - мотивационное сообщение.
	MotivationalMessage string `json:"motivational_message,omitempty"`
}

// GetDailyProgressHandler обрабатывает запросы дневного прогресса.
type GetDailyProgressHandler struct {
	studentRepo     student.Repository
	progressRepo    student.ProgressRepository
	leaderboardRepo leaderboard.LeaderboardRepository
}

// NewGetDailyProgressHandler создаёт новый обработчик.
func NewGetDailyProgressHandler(
	studentRepo student.Repository,
	progressRepo student.ProgressRepository,
	leaderboardRepo leaderboard.LeaderboardRepository,
) *GetDailyProgressHandler {
	return &GetDailyProgressHandler{
		studentRepo:     studentRepo,
		progressRepo:    progressRepo,
		leaderboardRepo: leaderboardRepo,
	}
}

// Handle выполняет запрос.
func (h *GetDailyProgressHandler) Handle(ctx context.Context, query GetDailyProgressQuery) (*GetDailyProgressResult, error) {
	// Валидация
	if err := query.Validate(); err != nil {
		return nil, shared.WrapError("query", "GetDailyProgress", shared.ErrValidation, err.Error(), err)
	}

	// Получаем студента
	var stud *student.Student
	var err error

	if query.StudentID != "" {
		stud, err = h.studentRepo.GetByID(ctx, query.StudentID)
	} else {
		stud, err = h.studentRepo.GetByTelegramID(ctx, student.TelegramID(query.TelegramID))
	}

	if err != nil {
		return nil, shared.WrapError("query", "GetDailyProgress", shared.ErrNotFound, "student not found", err)
	}

	// Получаем дневной прогресс
	todayGrind, err := h.progressRepo.GetTodayDailyGrind(ctx, stud.ID)
	if err != nil {
		// Создаём пустой, если не найден
		todayGrind = student.NewDailyGrind(stud.ID, stud.CurrentXP, 0)
	}

	// Получаем позицию в рейтинге
	var currentRank int
	if query.IncludeRankProgress && h.leaderboardRepo != nil {
		entry, err := h.leaderboardRepo.GetStudentRank(ctx, stud.ID, leaderboard.CohortAll)
		if err == nil && entry != nil {
			currentRank = int(entry.Rank)
			todayGrind.UpdateRank(currentRank)
		}
	}

	// Строим DTO для сегодня
	todayDTO := h.buildDailyProgressDTO(todayGrind, true)

	result := &GetDailyProgressResult{
		StudentID:   stud.ID,
		DisplayName: stud.DisplayName,
		Today:       todayDTO,
		TotalXP:     int(stud.CurrentXP),
		Level:       int(stud.Level()),
		GeneratedAt: time.Now().UTC(),
	}

	// Получаем историю
	if query.IncludeHistory {
		result.History = h.getHistory(ctx, stud.ID, query.HistoryDays)
		result.WeeklyXP, result.WeeklyTasks = h.calculateWeeklyStats(result.History)
	}

	// Получаем серию
	if query.IncludeStreak {
		result.Streak = h.getStreakInfo(ctx, stud.ID, todayDTO.IsActive)
	}

	// Сравнение со вчера
	if query.IncludeComparison && len(result.History) > 0 {
		result.Comparison = h.buildComparison(todayDTO, result.History)
	}

	// Мотивационное сообщение
	result.MotivationalMessage = h.generateMotivationalMessage(result)

	return result, nil
}

// buildDailyProgressDTO строит DTO дневного прогресса.
func (h *GetDailyProgressHandler) buildDailyProgressDTO(grind *student.DailyGrind, isToday bool) DailyProgressDTO {
	dto := DailyProgressDTO{
		Date:                grind.Date,
		DateFormatted:       formatDateRu(grind.Date),
		IsToday:             isToday,
		XPStart:             int(grind.XPStart),
		XPCurrent:           int(grind.XPCurrent),
		XPGained:            int(grind.XPGained),
		XPGainedFormatted:   formatXPGained(int(grind.XPGained)),
		TasksCompleted:      grind.TasksCompleted,
		SessionsCount:       grind.SessionsCount,
		TotalActiveMinutes:  grind.TotalSessionMinutes,
		ActiveTimeFormatted: formatActiveTime(grind.TotalSessionMinutes),
		RankAtStart:         grind.RankAtStart,
		RankCurrent:         grind.RankCurrent,
		RankChange:          grind.RankChange,
		RankChangeFormatted: formatRankChangeDaily(grind.RankChange),
		StreakDay:           grind.StreakDay,
		IsActive:            grind.IsActive(),
	}

	// First/Last activity
	if !grind.FirstActivityAt.IsZero() {
		dto.FirstActivityAt = &grind.FirstActivityAt
	}
	if !grind.LastActivityAt.IsZero() {
		dto.LastActivityAt = &grind.LastActivityAt
	}

	// Оценка дня
	dto.DayRating, dto.DayRatingEmoji = rateDayProgress(int(grind.XPGained), grind.TasksCompleted)

	// Прогресс к цели (цель: 100 XP в день)
	dto.ProgressPercent = int(grind.XPGained) * 100 / 100
	if dto.ProgressPercent > 200 {
		dto.ProgressPercent = 200
	}

	return dto
}

// getHistory получает историю за N дней.
func (h *GetDailyProgressHandler) getHistory(ctx context.Context, studentID string, days int) []DailyProgressDTO {
	history, err := h.progressRepo.GetDailyGrindHistory(ctx, studentID, days)
	if err != nil || len(history) == 0 {
		return nil
	}

	result := make([]DailyProgressDTO, len(history))
	for i, grind := range history {
		result[i] = h.buildDailyProgressDTO(grind, false)
	}

	return result
}

// calculateWeeklyStats вычисляет недельную статистику.
func (h *GetDailyProgressHandler) calculateWeeklyStats(history []DailyProgressDTO) (int, int) {
	weeklyXP := 0
	weeklyTasks := 0

	for _, day := range history {
		weeklyXP += day.XPGained
		weeklyTasks += day.TasksCompleted
	}

	return weeklyXP, weeklyTasks
}

// getStreakInfo получает информацию о серии.
func (h *GetDailyProgressHandler) getStreakInfo(ctx context.Context, studentID string, isActiveToday bool) *StreakInfoDTO {
	streak, err := h.progressRepo.GetStreak(ctx, studentID)
	if err != nil || streak == nil {
		return &StreakInfoDTO{
			CurrentStreak: 0,
			BestStreak:    0,
			StreakMessage: "Начни серию сегодня! 🔥",
		}
	}

	info := &StreakInfoDTO{
		CurrentStreak:   streak.CurrentStreak,
		BestStreak:      streak.BestStreak,
		LastActiveDate:  streak.LastActiveDate,
		StreakStartDate: streak.StreakStartDate,
	}

	// Проверяем риск потери серии
	if !isActiveToday && streak.CurrentStreak > 0 {
		info.IsAtRisk = true
		endOfDay := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 23, 59, 59, 0, time.UTC)
		info.HoursToSaveStreak = int(time.Until(endOfDay).Hours())
	}

	// Генерируем сообщение
	info.StreakMessage = generateStreakMessage(streak.CurrentStreak, info.IsAtRisk, isActiveToday)

	return info
}

// buildComparison строит сравнение со вчера.
func (h *GetDailyProgressHandler) buildComparison(today DailyProgressDTO, history []DailyProgressDTO) *ComparisonDTO {
	if len(history) == 0 {
		return nil
	}

	// Ищем вчера
	yesterday := time.Now().AddDate(0, 0, -1)
	var yesterdayData *DailyProgressDTO

	for _, day := range history {
		if day.Date.Year() == yesterday.Year() &&
			day.Date.YearDay() == yesterday.YearDay() {
			yesterdayData = &day
			break
		}
	}

	if yesterdayData == nil {
		return &ComparisonDTO{
			Trend:      "unknown",
			TrendEmoji: "❓",
			Message:    "Нет данных за вчера",
		}
	}

	comp := &ComparisonDTO{
		XPDifference:         today.XPGained - yesterdayData.XPGained,
		TasksDifference:      today.TasksCompleted - yesterdayData.TasksCompleted,
		ActiveTimeDifference: today.TotalActiveMinutes - yesterdayData.TotalActiveMinutes,
	}

	// Определяем тренд
	if comp.XPDifference > 0 {
		comp.Trend = "better"
		comp.TrendEmoji = "📈"
		comp.Message = fmt.Sprintf("На %d XP больше чем вчера!", comp.XPDifference)
	} else if comp.XPDifference < 0 {
		comp.Trend = "worse"
		comp.TrendEmoji = "📉"
		comp.Message = "Сегодня чуть меньше активности, но ещё есть время!"
	} else {
		comp.Trend = "same"
		comp.TrendEmoji = "➡️"
		comp.Message = "Стабильный результат!"
	}

	return comp
}

// generateMotivationalMessage генерирует мотивационное сообщение.
func (h *GetDailyProgressHandler) generateMotivationalMessage(result *GetDailyProgressResult) string {
	// Отличный день
	if result.Today.XPGained >= 200 {
		return "🏆 Невероятный день! Ты настоящий герой!"
	}

	// Хороший день
	if result.Today.XPGained >= 100 {
		return "🔥 Отличный прогресс! Так держать!"
	}

	// Есть активность
	if result.Today.IsActive {
		return "💪 Хорошее начало! Продолжай в том же духе!"
	}

	// Серия под угрозой
	if result.Streak != nil && result.Streak.IsAtRisk && result.Streak.CurrentStreak > 0 {
		return fmt.Sprintf("⚠️ Серия %d дней под угрозой! Сделай хотя бы одну задачу!", result.Streak.CurrentStreak)
	}

	// Нет активности сегодня
	return "🌟 День только начался! Время для первой задачи!"
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// formatDateRu форматирует дату на русском.
func formatDateRu(t time.Time) string {
	months := []string{
		"января", "февраля", "марта", "апреля", "мая", "июня",
		"июля", "августа", "сентября", "октября", "ноября", "декабря",
	}

	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return "Сегодня"
	}
	if t.Year() == now.Year() && t.YearDay() == now.YearDay()-1 {
		return "Вчера"
	}

	return fmt.Sprintf("%d %s", t.Day(), months[t.Month()-1])
}

// formatXPGained форматирует заработанный XP.
func formatXPGained(xp int) string {
	if xp == 0 {
		return "0 XP"
	}
	if xp > 0 {
		return fmt.Sprintf("+%d XP", xp)
	}
	return fmt.Sprintf("%d XP", xp)
}

// formatActiveTime форматирует время активности.
func formatActiveTime(minutes int) string {
	if minutes == 0 {
		return "—"
	}
	if minutes < 60 {
		return fmt.Sprintf("%d мин", minutes)
	}
	hours := minutes / 60
	mins := minutes % 60
	if mins == 0 {
		return fmt.Sprintf("%d ч", hours)
	}
	return fmt.Sprintf("%d ч %d мин", hours, mins)
}

// formatRankChangeDaily форматирует изменение ранга за день.
func formatRankChangeDaily(change int) string {
	if change > 0 {
		return fmt.Sprintf("↑%d", change)
	}
	if change < 0 {
		return fmt.Sprintf("↓%d", -change)
	}
	return "—"
}

// rateDayProgress оценивает дневной прогресс.
func rateDayProgress(xp, tasks int) (string, string) {
	switch {
	case xp >= 200:
		return "excellent", "🌟"
	case xp >= 100:
		return "good", "✅"
	case xp >= 50 || tasks >= 1:
		return "normal", "👍"
	case xp > 0:
		return "low", "💡"
	default:
		return "inactive", "💤"
	}
}

// generateStreakMessage генерирует сообщение о серии.
func generateStreakMessage(streak int, isAtRisk, isActiveToday bool) string {
	if streak == 0 {
		return "Начни серию сегодня! 🔥"
	}

	if isAtRisk {
		return fmt.Sprintf("🔥 Серия %d дней! Не потеряй её сегодня!", streak)
	}

	if isActiveToday {
		switch {
		case streak >= 30:
			return fmt.Sprintf("🏆 %d дней подряд! Легенда!", streak)
		case streak >= 7:
			return fmt.Sprintf("🔥 %d дней! Отличная серия!", streak)
		default:
			return fmt.Sprintf("🔥 %d дней подряд! Продолжай!", streak)
		}
	}

	return fmt.Sprintf("Серия: %d дней", streak)
}
