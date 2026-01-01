// Package query contains read operations (CQRS - Queries).
package query

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/leaderboard"
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"context"
	"errors"
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// GET NEIGHBORS QUERY
// Получает соседей студента по рангу (±N позиций).
// Это ключевая мотивационная фича: показывает, кого можно догнать
// и кто дышит в спину. Философия: "ты не один в этой гонке".
// ══════════════════════════════════════════════════════════════════════════════

// GetNeighborsQuery содержит параметры запроса соседей.
type GetNeighborsQuery struct {
	// StudentID - внутренний ID студента.
	StudentID string

	// TelegramID - альтернативный способ идентификации.
	TelegramID int64

	// RangeSize - сколько соседей показывать с каждой стороны (по умолчанию 5).
	RangeSize int

	// Cohort - когорта для расчёта (пустая = общий рейтинг).
	Cohort string

	// IncludeOnlineStatus - включить информацию об онлайн-статусе.
	IncludeOnlineStatus bool

	// IncludeXPGap - включить расчёт разрыва в XP.
	IncludeXPGap bool
}

// Validate проверяет корректность параметров.
func (q *GetNeighborsQuery) Validate() error {
	if q.StudentID == "" && q.TelegramID == 0 {
		return errors.New("either student_id or telegram_id must be provided")
	}
	if q.RangeSize < 0 {
		return errors.New("range_size cannot be negative")
	}
	if q.RangeSize == 0 {
		q.RangeSize = 5
	}
	if q.RangeSize > 25 {
		q.RangeSize = 25
	}
	return nil
}

// NeighborDTO - DTO для соседа в рейтинге.
type NeighborDTO struct {
	// ─────────────────────────────────────────────────────────────────────────
	// Идентификация
	// ─────────────────────────────────────────────────────────────────────────

	// StudentID - внутренний ID.
	StudentID string `json:"student_id"`

	// DisplayName - отображаемое имя.
	DisplayName string `json:"display_name"`

	// ─────────────────────────────────────────────────────────────────────────
	// Позиция
	// ─────────────────────────────────────────────────────────────────────────

	// Rank - позиция в рейтинге.
	Rank int `json:"rank"`

	// XP - текущий XP.
	XP int `json:"xp"`

	// Level - уровень.
	Level int `json:"level"`

	// RankChange - изменение позиции.
	RankChange int `json:"rank_change"`

	// RankDirection - направление: "up", "down", "stable".
	RankDirection string `json:"rank_direction"`

	// ─────────────────────────────────────────────────────────────────────────
	// Относительно текущего студента
	// ─────────────────────────────────────────────────────────────────────────

	// Position - позиция относительно текущего студента.
	// Отрицательное = выше (ближе к #1), положительное = ниже.
	Position int `json:"position"`

	// XPGap - разница в XP с текущим студентом.
	// Положительное = сосед впереди, отрицательное = сосед позади.
	XPGap int `json:"xp_gap"`

	// IsCurrentStudent - это текущий студент (центр списка).
	IsCurrentStudent bool `json:"is_current_student"`

	// ─────────────────────────────────────────────────────────────────────────
	// Статус
	// ─────────────────────────────────────────────────────────────────────────

	// IsOnline - онлайн ли сейчас.
	IsOnline bool `json:"is_online"`

	// LastSeenAt - время последней активности.
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`

	// IsAvailableForHelp - готов ли помогать.
	IsAvailableForHelp bool `json:"is_available_for_help"`

	// HelpRating - рейтинг как помощника.
	HelpRating float64 `json:"help_rating,omitempty"`
}

// GetNeighborsResult содержит результат запроса соседей.
type GetNeighborsResult struct {
	// CurrentStudent - текущий студент (центр).
	CurrentStudent NeighborDTO `json:"current_student"`

	// Neighbors - все соседи включая текущего студента.
	Neighbors []NeighborDTO `json:"neighbors"`

	// AboveCount - сколько соседей выше текущего.
	AboveCount int `json:"above_count"`

	// BelowCount - сколько соседей ниже текущего.
	BelowCount int `json:"below_count"`

	// ─────────────────────────────────────────────────────────────────────────
	// Мотивационная информация
	// ─────────────────────────────────────────────────────────────────────────

	// ClosestAbove - ближайший сосед выше (кого догонять).
	ClosestAbove *NeighborDTO `json:"closest_above,omitempty"`

	// ClosestBelow - ближайший сосед ниже (кто догоняет).
	ClosestBelow *NeighborDTO `json:"closest_below,omitempty"`

	// XPToOvertakeNext - сколько XP нужно, чтобы обогнать следующего.
	XPToOvertakeNext int `json:"xp_to_overtake_next"`

	// XPAheadOfChaser - на сколько XP мы опережаем преследователя.
	XPAheadOfChaser int `json:"xp_ahead_of_chaser"`

	// ─────────────────────────────────────────────────────────────────────────
	// Статистика
	// ─────────────────────────────────────────────────────────────────────────

	// OnlineCount - сколько соседей онлайн.
	OnlineCount int `json:"online_count"`

	// Cohort - когорта.
	Cohort string `json:"cohort"`

	// TotalInCohort - всего студентов в когорте/рейтинге.
	TotalInCohort int `json:"total_in_cohort"`

	// GeneratedAt - время генерации.
	GeneratedAt time.Time `json:"generated_at"`

	// MotivationalMessage - мотивационное сообщение.
	MotivationalMessage string `json:"motivational_message,omitempty"`
}

// GetNeighborsHandler обрабатывает запросы на получение соседей.
type GetNeighborsHandler struct {
	studentRepo     student.Repository
	leaderboardRepo leaderboard.LeaderboardRepository
	onlineTracker   student.OnlineTracker
}

// NewGetNeighborsHandler создаёт новый обработчик.
func NewGetNeighborsHandler(
	studentRepo student.Repository,
	leaderboardRepo leaderboard.LeaderboardRepository,
	onlineTracker student.OnlineTracker,
) *GetNeighborsHandler {
	return &GetNeighborsHandler{
		studentRepo:     studentRepo,
		leaderboardRepo: leaderboardRepo,
		onlineTracker:   onlineTracker,
	}
}

// Handle выполняет запрос на получение соседей.
func (h *GetNeighborsHandler) Handle(ctx context.Context, query GetNeighborsQuery) (*GetNeighborsResult, error) {
	// Валидация
	if err := query.Validate(); err != nil {
		return nil, shared.WrapError("query", "GetNeighbors", shared.ErrValidation, err.Error(), err)
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
		return nil, shared.WrapError("query", "GetNeighbors", shared.ErrNotFound, "student not found", err)
	}

	cohort := leaderboard.Cohort(query.Cohort)

	// Получаем соседей из репозитория
	neighbors, err := h.leaderboardRepo.GetNeighbors(ctx, stud.ID, cohort, query.RangeSize)
	if err != nil {
		return nil, shared.WrapError("query", "GetNeighbors", shared.ErrNotFound, "neighbors not found", err)
	}

	if len(neighbors) == 0 {
		return nil, shared.WrapError("query", "GetNeighbors", shared.ErrNotFound, "no neighbors found", nil)
	}

	// Обогащаем онлайн-статусом
	if query.IncludeOnlineStatus {
		neighbors = h.enrichWithOnlineStatus(ctx, neighbors)
	}

	// Получаем общее количество
	totalCount, err := h.leaderboardRepo.GetTotalCount(ctx, cohort)
	if err != nil {
		totalCount = 0
	}

	// Находим текущего студента в списке
	var currentEntry *leaderboard.LeaderboardEntry
	currentIdx := -1
	for i, n := range neighbors {
		if n.StudentID == stud.ID {
			currentEntry = n
			currentIdx = i
			break
		}
	}

	if currentEntry == nil {
		return nil, shared.WrapError("query", "GetNeighbors", shared.ErrNotFound, "current student not in neighbors", nil)
	}

	// Формируем результат
	return h.buildResult(neighbors, currentEntry, currentIdx, totalCount, cohort)
}

// enrichWithOnlineStatus обогащает записи онлайн-статусом.
func (h *GetNeighborsHandler) enrichWithOnlineStatus(
	ctx context.Context,
	entries []*leaderboard.LeaderboardEntry,
) []*leaderboard.LeaderboardEntry {
	if h.onlineTracker == nil {
		return entries
	}

	studentIDs := make([]string, len(entries))
	for i, e := range entries {
		studentIDs[i] = e.StudentID
	}

	onlineStates, err := h.onlineTracker.GetOnlineStates(ctx, studentIDs)
	if err != nil {
		return entries
	}

	for _, entry := range entries {
		if state, ok := onlineStates[entry.StudentID]; ok {
			entry.IsOnline = state.IsAvailable()
		}
	}

	return entries
}

// buildResult формирует итоговый результат.
func (h *GetNeighborsHandler) buildResult(
	neighbors []*leaderboard.LeaderboardEntry,
	currentEntry *leaderboard.LeaderboardEntry,
	currentIdx int,
	totalCount int,
	cohort leaderboard.Cohort,
) (*GetNeighborsResult, error) {
	dtos := make([]NeighborDTO, len(neighbors))
	var currentDTO NeighborDTO
	var closestAbove, closestBelow *NeighborDTO
	onlineCount := 0
	aboveCount := 0
	belowCount := 0

	for i, n := range neighbors {
		position := i - currentIdx // Отрицательное = выше, положительное = ниже
		xpGap := int(n.XP) - int(currentEntry.XP)

		dto := NeighborDTO{
			StudentID:          n.StudentID,
			DisplayName:        n.DisplayName,
			Rank:               int(n.Rank),
			XP:                 int(n.XP),
			Level:              n.Level,
			RankChange:         int(n.RankChange),
			RankDirection:      string(n.Direction()),
			Position:           position,
			XPGap:              xpGap,
			IsCurrentStudent:   n.StudentID == currentEntry.StudentID,
			IsOnline:           n.IsOnline,
			IsAvailableForHelp: n.IsAvailableForHelp,
			HelpRating:         n.HelpRating,
		}

		if !n.UpdatedAt.IsZero() {
			dto.LastSeenAt = &n.UpdatedAt
		}

		dtos[i] = dto

		if n.IsOnline {
			onlineCount++
		}

		if n.StudentID == currentEntry.StudentID {
			currentDTO = dto
		} else if position < 0 {
			aboveCount++
			// Ближайший сверху (position = -1)
			if position == -1 {
				closestAbove = &dtos[i]
			}
		} else if position > 0 {
			belowCount++
			// Ближайший снизу (position = 1)
			if position == 1 {
				closestBelow = &dtos[i]
			}
		}
	}

	// Расчёт XP до обгона
	xpToOvertake := 0
	if closestAbove != nil {
		xpToOvertake = closestAbove.XP - currentDTO.XP + 1
	}

	xpAhead := 0
	if closestBelow != nil {
		xpAhead = currentDTO.XP - closestBelow.XP
	}

	// Мотивационное сообщение
	message := h.generateMotivationalMessage(currentDTO, closestAbove, closestBelow, xpToOvertake)

	return &GetNeighborsResult{
		CurrentStudent:      currentDTO,
		Neighbors:           dtos,
		AboveCount:          aboveCount,
		BelowCount:          belowCount,
		ClosestAbove:        closestAbove,
		ClosestBelow:        closestBelow,
		XPToOvertakeNext:    xpToOvertake,
		XPAheadOfChaser:     xpAhead,
		OnlineCount:         onlineCount,
		Cohort:              string(cohort),
		TotalInCohort:       totalCount,
		GeneratedAt:         time.Now().UTC(),
		MotivationalMessage: message,
	}, nil
}

// generateMotivationalMessage генерирует мотивационное сообщение.
func (h *GetNeighborsHandler) generateMotivationalMessage(
	current NeighborDTO,
	above *NeighborDTO,
	below *NeighborDTO,
	xpToOvertake int,
) string {
	// Первое место
	if above == nil && current.Rank == 1 {
		return "🥇 Ты на первом месте! Удерживай позицию!"
	}

	// Очень близко к обгону
	if above != nil && xpToOvertake <= 10 {
		return fmt.Sprintf("🔥 Всего %d XP до обгона @%s!", xpToOvertake, above.DisplayName)
	}

	// Близко к обгону
	if above != nil && xpToOvertake <= 50 {
		return fmt.Sprintf("💪 До @%s осталось %d XP - одна задача!", above.DisplayName, xpToOvertake)
	}

	// Кто-то близко к нам
	if below != nil {
		gap := current.XP - below.XP
		if gap <= 20 {
			return fmt.Sprintf("⚠️ @%s дышит в спину! Всего %d XP разницы!", below.DisplayName, gap)
		}
	}

	// Оба соседа онлайн
	if above != nil && below != nil && above.IsOnline && below.IsOnline {
		return "🏃 Твои соседи онлайн! Не отставай!"
	}

	// Мы онлайн, соседи нет
	if (above == nil || !above.IsOnline) && (below == nil || !below.IsOnline) {
		return "🌟 Ты активнее своих соседей! Самое время продвинуться!"
	}

	// Поднялись в рейтинге
	if current.RankChange > 0 {
		return fmt.Sprintf("📈 +%d позиций! Отличный прогресс!", current.RankChange)
	}

	return ""
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// FormatNeighborPosition форматирует позицию соседа для отображения.
func FormatNeighborPosition(position int, isOnline bool) string {
	statusIcon := "⚪"
	if isOnline {
		statusIcon = "🟢"
	}

	if position < 0 {
		return fmt.Sprintf("%s ⬆️ %d", statusIcon, -position)
	}
	if position > 0 {
		return fmt.Sprintf("%s ⬇️ %d", statusIcon, position)
	}
	return fmt.Sprintf("%s 👤 (ты)", statusIcon)
}

// FormatXPGap форматирует разницу в XP.
func FormatXPGap(gap int) string {
	if gap > 0 {
		return fmt.Sprintf("+%d XP впереди", gap)
	}
	if gap < 0 {
		return fmt.Sprintf("%d XP позади", gap)
	}
	return "Одинаковый XP"
}
