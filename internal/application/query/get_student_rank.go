// Package query contains read operations (CQRS - Queries).
package query

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/leaderboard"
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"context"
	"errors"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// GET STUDENT RANK QUERY
// Получает текущую позицию студента в лидерборде с расширенной статистикой.
// Это ключевой запрос для команды /me - показывает "где я нахожусь".
// ══════════════════════════════════════════════════════════════════════════════

// GetStudentRankQuery содержит параметры запроса позиции студента.
type GetStudentRankQuery struct {
	// StudentID - внутренний ID студента.
	StudentID string

	// TelegramID - альтернативный способ идентификации (опционально).
	TelegramID int64

	// Cohort - когорта для расчёта позиции (пустая = общий рейтинг).
	Cohort string

	// IncludeHistory - включить историю изменений ранга.
	IncludeHistory bool

	// HistoryDays - за сколько дней показывать историю (по умолчанию 7).
	HistoryDays int
}

// Validate проверяет корректность параметров запроса.
func (q *GetStudentRankQuery) Validate() error {
	if q.StudentID == "" && q.TelegramID == 0 {
		return errors.New("either student_id or telegram_id must be provided")
	}
	if q.HistoryDays < 0 {
		return errors.New("history_days cannot be negative")
	}
	if q.HistoryDays == 0 {
		q.HistoryDays = 7
	}
	if q.HistoryDays > 30 {
		q.HistoryDays = 30
	}
	return nil
}

// StudentRankDTO - DTO с позицией студента в рейтинге.
type StudentRankDTO struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Идентификация
	// ─────────────────────────────────────────────────────────────────────────

	// StudentID - внутренний ID студента.
	StudentID string `json:"student_id"`

	// AlemLogin - логин на платформе Alem.
	AlemLogin string `json:"alem_login"`

	// DisplayName - отображаемое имя.
	DisplayName string `json:"display_name"`

	// ─────────────────────────────────────────────────────────────────────────
	// Текущая позиция
	// ─────────────────────────────────────────────────────────────────────────

	// Rank - текущая позиция в рейтинге.
	Rank int `json:"rank"`

	// TotalStudents - общее количество студентов в рейтинге.
	TotalStudents int `json:"total_students"`

	// Percentile - процентиль (например, "топ 15%").
	Percentile float64 `json:"percentile"`

	// ─────────────────────────────────────────────────────────────────────────
	// XP и уровень
	// ─────────────────────────────────────────────────────────────────────────

	// XP - текущее количество очков опыта.
	XP int `json:"xp"`

	// Level - уровень студента.
	Level int `json:"level"`

	// XPToNextLevel - XP до следующего уровня.
	XPToNextLevel int `json:"xp_to_next_level"`

	// LevelProgress - прогресс до следующего уровня (0.0 - 1.0).
	LevelProgress float64 `json:"level_progress"`

	// ─────────────────────────────────────────────────────────────────────────
	// Изменения позиции
	// ─────────────────────────────────────────────────────────────────────────

	// RankChange - изменение позиции с прошлого обновления.
	RankChange int `json:"rank_change"`

	// RankDirection - направление изменения: "up", "down", "stable".
	RankDirection string `json:"rank_direction"`

	// BestRank - лучшая позиция за всё время.
	BestRank int `json:"best_rank"`

	// BestRankDate - когда была достигнута лучшая позиция.
	BestRankDate *time.Time `json:"best_rank_date,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Близость к соседям
	// ─────────────────────────────────────────────────────────────────────────

	// XPToNextRank - сколько XP до следующего места.
	XPToNextRank int `json:"xp_to_next_rank"`

	// NextRankStudent - логин студента на следующем месте.
	NextRankStudent string `json:"next_rank_student,omitempty"`

	// XPAheadOfPrevious - на сколько XP опережаем предыдущего.
	XPAheadOfPrevious int `json:"xp_ahead_of_previous"`

	// PreviousRankStudent - логин студента на предыдущем месте.
	PreviousRankStudent string `json:"previous_rank_student,omitempty"`

	// ─────────────────────────────────────────────────────────────────────────
	// Статус
	// ─────────────────────────────────────────────────────────────────────────

	// IsOnline - онлайн ли студент сейчас.
	IsOnline bool `json:"is_online"`

	// LastSeenAt - время последней активности.
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`

	// Cohort - когорта студента.
	Cohort string `json:"cohort"`

	// ─────────────────────────────────────────────────────────────────────────
	// Социальные данные
	// ─────────────────────────────────────────────────────────────────────────

	// IsAvailableForHelp - готов ли студент помогать другим.
	IsAvailableForHelp bool `json:"is_available_for_help"`

	// HelpRating - рейтинг как помощника (0-5).
	HelpRating float64 `json:"help_rating"`

	// ─────────────────────────────────────────────────────────────────────────
	// История (опционально)
	// ─────────────────────────────────────────────────────────────────────────

	// RankHistory - история изменений позиции.
	RankHistory []RankHistoryPointDTO `json:"rank_history,omitempty"`
}

// RankHistoryPointDTO - точка в истории рангов.
type RankHistoryPointDTO struct {
	// Date - дата.
	Date time.Time `json:"date"`

	// Rank - позиция на эту дату.
	Rank int `json:"rank"`

	// XP - XP на эту дату.
	XP int `json:"xp"`

	// Change - изменение с предыдущего дня.
	Change int `json:"change"`
}

// GetStudentRankResult содержит результат запроса позиции студента.
type GetStudentRankResult struct {
	// Student - информация о студенте и его позиции.
	Student StudentRankDTO `json:"student"`

	// Cohort - по какой когорте считали (пустая = общий).
	Cohort string `json:"cohort"`

	// GeneratedAt - время генерации результата.
	GeneratedAt time.Time `json:"generated_at"`

	// Message - мотивационное сообщение (например, "До топ-50 осталось 120 XP!").
	Message string `json:"message,omitempty"`
}

// GetStudentRankHandler обрабатывает запросы на получение позиции студента.
type GetStudentRankHandler struct {
	studentRepo      student.Repository
	leaderboardRepo  leaderboard.LeaderboardRepository
	leaderboardCache leaderboard.LeaderboardCache
	onlineTracker    student.OnlineTracker
}

// NewGetStudentRankHandler создаёт новый обработчик.
func NewGetStudentRankHandler(
	studentRepo student.Repository,
	leaderboardRepo leaderboard.LeaderboardRepository,
	leaderboardCache leaderboard.LeaderboardCache,
	onlineTracker student.OnlineTracker,
) *GetStudentRankHandler {
	return &GetStudentRankHandler{
		studentRepo:      studentRepo,
		leaderboardRepo:  leaderboardRepo,
		leaderboardCache: leaderboardCache,
		onlineTracker:    onlineTracker,
	}
}

// Handle выполняет запрос на получение позиции студента.
func (h *GetStudentRankHandler) Handle(ctx context.Context, query GetStudentRankQuery) (*GetStudentRankResult, error) {
	// Валидация
	if err := query.Validate(); err != nil {
		return nil, shared.WrapError("query", "GetStudentRank", shared.ErrValidation, err.Error(), err)
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
		return nil, shared.WrapError("query", "GetStudentRank", shared.ErrNotFound, "student not found", err)
	}

	cohort := leaderboard.Cohort(query.Cohort)

	// Получаем позицию в лидерборде
	entry, err := h.leaderboardRepo.GetStudentRank(ctx, stud.ID, cohort)
	if err != nil {
		return nil, shared.WrapError("query", "GetStudentRank", shared.ErrNotFound, "rank not found", err)
	}

	// Получаем общее количество студентов
	totalCount, err := h.leaderboardRepo.GetTotalCount(ctx, cohort)
	if err != nil {
		totalCount = 0
	}

	// Получаем соседей для расчёта XP gap
	neighbors, err := h.leaderboardRepo.GetNeighbors(ctx, stud.ID, cohort, 1)
	if err != nil {
		neighbors = nil
	}

	// Получаем лучший ранг
	bestRankEntry, _ := h.leaderboardRepo.GetBestRank(ctx, stud.ID)

	// Получаем онлайн-статус
	isOnline := false
	if h.onlineTracker != nil {
		isOnline, _ = h.onlineTracker.IsOnline(ctx, stud.ID)
	}

	// Получаем историю (если запрошена)
	var rankHistory []RankHistoryPointDTO
	if query.IncludeHistory {
		rankHistory = h.getRankHistory(ctx, stud.ID, query.HistoryDays)
	}

	// Формируем DTO
	dto := h.buildDTO(stud, entry, totalCount, neighbors, bestRankEntry, isOnline, rankHistory)

	// Генерируем мотивационное сообщение
	message := h.generateMotivationalMessage(dto)

	return &GetStudentRankResult{
		Student:     dto,
		Cohort:      string(cohort),
		GeneratedAt: time.Now().UTC(),
		Message:     message,
	}, nil
}

// buildDTO формирует DTO из доменных объектов.
func (h *GetStudentRankHandler) buildDTO(
	stud *student.Student,
	entry *leaderboard.LeaderboardEntry,
	totalCount int,
	neighbors []*leaderboard.LeaderboardEntry,
	bestRank *leaderboard.RankHistoryEntry,
	isOnline bool,
	history []RankHistoryPointDTO,
) StudentRankDTO {
	dto := StudentRankDTO{
		StudentID:     stud.ID,
		AlemLogin:     string(stud.AlemLogin),
		DisplayName:   stud.DisplayName,
		Rank:          int(entry.Rank),
		TotalStudents: totalCount,
		XP:            int(entry.XP),
		Level:         entry.Level,
		RankChange:    int(entry.RankChange),
		RankDirection: string(entry.Direction()),
		IsOnline:      isOnline,
		Cohort:        string(entry.Cohort),
		RankHistory:   history,
	}

	// Процентиль
	if totalCount > 0 {
		dto.Percentile = 100.0 - (float64(entry.Rank-1) / float64(totalCount) * 100.0)
	}

	// XP до следующего уровня (каждые 1000 XP = 1 уровень)
	currentLevelXP := entry.Level * 1000
	nextLevelXP := (entry.Level + 1) * 1000
	dto.XPToNextLevel = nextLevelXP - int(entry.XP)
	if dto.XPToNextLevel < 0 {
		dto.XPToNextLevel = 0
	}

	// Прогресс уровня
	if nextLevelXP > currentLevelXP {
		dto.LevelProgress = float64(int(entry.XP)-currentLevelXP) / float64(nextLevelXP-currentLevelXP)
	}

	// Last seen
	if !stud.LastSeenAt.IsZero() {
		dto.LastSeenAt = &stud.LastSeenAt
	}

	// Лучший ранг
	if bestRank != nil {
		dto.BestRank = int(bestRank.Rank)
		if !bestRank.SnapshotAt.IsZero() {
			dto.BestRankDate = &bestRank.SnapshotAt
		}
	}

	// Соседи
	if neighbors != nil {
		for _, n := range neighbors {
			if n.StudentID == stud.ID {
				continue
			}
			// Сосед выше (ранг меньше = выше)
			if n.Rank < entry.Rank {
				dto.XPToNextRank = int(n.XP) - int(entry.XP) + 1
				dto.NextRankStudent = n.AlemLogin
			}
			// Сосед ниже
			if n.Rank > entry.Rank {
				dto.XPAheadOfPrevious = int(entry.XP) - int(n.XP)
				dto.PreviousRankStudent = n.AlemLogin
			}
		}
	}

	return dto
}

// getRankHistory получает историю рангов.
func (h *GetStudentRankHandler) getRankHistory(ctx context.Context, studentID string, days int) []RankHistoryPointDTO {
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -days)

	history, err := h.leaderboardRepo.GetRankHistory(ctx, studentID, from, to)
	if err != nil || len(history) == 0 {
		return nil
	}

	result := make([]RankHistoryPointDTO, len(history))
	for i, h := range history {
		result[i] = RankHistoryPointDTO{
			Date:   h.SnapshotAt,
			Rank:   int(h.Rank),
			XP:     int(h.XP),
			Change: int(h.RankChange),
		}
	}

	return result
}

// generateMotivationalMessage генерирует мотивационное сообщение.
func (h *GetStudentRankHandler) generateMotivationalMessage(dto StudentRankDTO) string {
	// Топ-10
	if dto.Rank <= 10 {
		return "🏆 Ты в топ-10! Продолжай в том же духе!"
	}

	// Близко к топ-10
	if dto.Rank <= 15 && dto.XPToNextRank > 0 {
		return "🔥 До топ-10 осталось совсем немного!"
	}

	// Близко к топ-50
	if dto.Rank > 50 && dto.Rank <= 60 {
		return "⭐ Ещё чуть-чуть и войдёшь в топ-50!"
	}

	// Топ-50
	if dto.Rank <= 50 {
		return "⭐ Ты в топ-50! Отличный результат!"
	}

	// Рост
	if dto.RankChange > 0 {
		return "📈 Отличный прогресс! Ты поднялся в рейтинге!"
	}

	// Падение
	if dto.RankChange < 0 {
		return "💪 Не сдавайся! Время наверстать упущенное!"
	}

	// По умолчанию
	if dto.XPToNextRank > 0 && dto.XPToNextRank <= 50 {
		return "🎯 До следующего места осталось меньше 50 XP!"
	}

	return ""
}

// ══════════════════════════════════════════════════════════════════════════════
// PERCENTILE HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// FormatPercentile форматирует процентиль для отображения.
func FormatPercentile(percentile float64) string {
	if percentile >= 99 {
		return "топ 1%"
	}
	if percentile >= 95 {
		return "топ 5%"
	}
	if percentile >= 90 {
		return "топ 10%"
	}
	if percentile >= 75 {
		return "топ 25%"
	}
	if percentile >= 50 {
		return "верхняя половина"
	}
	return "есть куда расти"
}

// FormatLevelProgress форматирует прогресс уровня.
func FormatLevelProgress(progress float64) string {
	bars := int(progress * 10)
	empty := 10 - bars

	result := "["
	for i := 0; i < bars; i++ {
		result += "█"
	}
	for i := 0; i < empty; i++ {
		result += "░"
	}
	result += "]"

	return result
}
