// Package query contains read operations following CQRS pattern.
// Queries never modify state - they only read and return data.
// Each query is a self-contained use case with its own request/response types.
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
// GET LEADERBOARD QUERY
// Получает топ-N студентов лидерборда с возможностью фильтрации.
// Поддерживает пагинацию и фильтрацию по когорте/онлайн-статусу.
// ══════════════════════════════════════════════════════════════════════════════

// GetLeaderboardQuery содержит параметры запроса лидерборда.
type GetLeaderboardQuery struct {
	// Cohort - фильтр по когорте (пустая строка = все когорты).
	Cohort string

	// Limit - количество записей (по умолчанию 20, максимум 100).
	Limit int

	// Offset - смещение для пагинации.
	Offset int

	// OnlyOnline - показывать только онлайн студентов.
	OnlyOnline bool

	// OnlyAvailableForHelp - показывать только готовых помогать.
	OnlyAvailableForHelp bool

	// IncludeRankChange - включать информацию об изменении позиции.
	IncludeRankChange bool
}

// Validate проверяет корректность параметров запроса.
func (q *GetLeaderboardQuery) Validate() error {
	if q.Limit < 0 {
		return errors.New("limit cannot be negative")
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.Offset < 0 {
		return errors.New("offset cannot be negative")
	}
	return nil
}

// LeaderboardEntryDTO - DTO для записи лидерборда (Data Transfer Object).
type LeaderboardEntryDTO struct {
	// Rank - позиция в рейтинге (начиная с 1).
	Rank int `json:"rank"`

	// StudentID - внутренний ID студента.
	StudentID string `json:"student_id"`

	// DisplayName - отображаемое имя.
	DisplayName string `json:"display_name"`

	// XP - текущее количество очков опыта.
	XP int `json:"xp"`

	// Level - уровень студента.
	Level int `json:"level"`

	// Cohort - когорта студента.
	Cohort string `json:"cohort"`

	// RankChange - изменение позиции (+ вверх, - вниз, 0 стабильно).
	RankChange int `json:"rank_change"`

	// RankDirection - направление изменения: "up", "down", "stable", "new".
	RankDirection string `json:"rank_direction"`

	// IsOnline - онлайн ли студент сейчас.
	IsOnline bool `json:"is_online"`

	// IsAvailableForHelp - готов ли помогать.
	IsAvailableForHelp bool `json:"is_available_for_help"`

	// HelpRating - рейтинг как помощника (0.0 - 5.0).
	HelpRating float64 `json:"help_rating,omitempty"`

	// LastSeenAt - время последней активности.
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
}

// GetLeaderboardResult содержит результат запроса лидерборда.
type GetLeaderboardResult struct {
	// Entries - записи лидерборда.
	Entries []LeaderboardEntryDTO `json:"entries"`

	// TotalCount - общее количество студентов в лидерборде.
	TotalCount int `json:"total_count"`

	// Cohort - когорта, по которой фильтровали (пустая = все).
	Cohort string `json:"cohort"`

	// OnlineCount - количество онлайн студентов.
	OnlineCount int `json:"online_count"`

	// AverageXP - средний XP в лидерборде.
	AverageXP int `json:"average_xp"`

	// MedianXP - медианный XP.
	MedianXP int `json:"median_xp"`

	// GeneratedAt - время генерации результата.
	GeneratedAt time.Time `json:"generated_at"`

	// HasMore - есть ли ещё записи после текущей страницы.
	HasMore bool `json:"has_more"`

	// Page - текущая страница (1-based).
	Page int `json:"page"`

	// PageSize - размер страницы.
	PageSize int `json:"page_size"`
}

// GetLeaderboardHandler обрабатывает запросы на получение лидерборда.
type GetLeaderboardHandler struct {
	leaderboardRepo  leaderboard.LeaderboardRepository
	leaderboardCache leaderboard.LeaderboardCache
	onlineTracker    student.OnlineTracker
}

// NewGetLeaderboardHandler создаёт новый обработчик запроса лидерборда.
func NewGetLeaderboardHandler(
	leaderboardRepo leaderboard.LeaderboardRepository,
	leaderboardCache leaderboard.LeaderboardCache,
	onlineTracker student.OnlineTracker,
) *GetLeaderboardHandler {
	return &GetLeaderboardHandler{
		leaderboardRepo:  leaderboardRepo,
		leaderboardCache: leaderboardCache,
		onlineTracker:    onlineTracker,
	}
}

// Handle выполняет запрос на получение лидерборда.
func (h *GetLeaderboardHandler) Handle(ctx context.Context, query GetLeaderboardQuery) (*GetLeaderboardResult, error) {
	// Валидация входных данных
	if err := query.Validate(); err != nil {
		return nil, shared.WrapError("query", "GetLeaderboard", shared.ErrValidation, err.Error(), err)
	}

	cohort := leaderboard.Cohort(query.Cohort)

	// Попытка получить из кеша
	cachedEntries, err := h.tryGetFromCache(ctx, cohort, query.Limit+query.Offset)
	if err == nil && len(cachedEntries) > 0 {
		return h.buildResult(ctx, cachedEntries, query, cohort)
	}

	// Получаем из репозитория
	entries, err := h.leaderboardRepo.GetTop(ctx, cohort, query.Limit+query.Offset)
	if err != nil {
		return nil, shared.WrapError("query", "GetLeaderboard", shared.ErrNotFound, "failed to get leaderboard", err)
	}

	// Обогащаем онлайн-статусом
	entries, err = h.enrichWithOnlineStatus(ctx, entries)
	if err != nil {
		// Логируем, но не прерываем - онлайн-статус не критичен
		// В production здесь был бы логгер
	}

	// Применяем фильтры
	entries = h.applyFilters(entries, query)

	// Применяем пагинацию
	paginatedEntries := h.paginate(entries, query.Offset, query.Limit)

	return h.buildResult(ctx, paginatedEntries, query, cohort)
}

// tryGetFromCache пытается получить данные из кеша.
func (h *GetLeaderboardHandler) tryGetFromCache(
	ctx context.Context,
	cohort leaderboard.Cohort,
	limit int,
) ([]*leaderboard.LeaderboardEntry, error) {
	if h.leaderboardCache == nil {
		return nil, errors.New("cache not available")
	}

	return h.leaderboardCache.GetCachedTop(ctx, cohort, limit)
}

// enrichWithOnlineStatus обогащает записи онлайн-статусом.
func (h *GetLeaderboardHandler) enrichWithOnlineStatus(
	ctx context.Context,
	entries []*leaderboard.LeaderboardEntry,
) ([]*leaderboard.LeaderboardEntry, error) {
	if h.onlineTracker == nil || len(entries) == 0 {
		return entries, nil
	}

	// Собираем ID студентов
	studentIDs := make([]string, len(entries))
	for i, e := range entries {
		studentIDs[i] = e.StudentID
	}

	// Получаем онлайн-статусы
	onlineStates, err := h.onlineTracker.GetOnlineStates(ctx, studentIDs)
	if err != nil {
		return entries, err
	}

	// Обогащаем записи
	for _, entry := range entries {
		if state, ok := onlineStates[entry.StudentID]; ok {
			entry.IsOnline = state.IsAvailable()
		}
	}

	return entries, nil
}

// applyFilters применяет фильтры к записям.
func (h *GetLeaderboardHandler) applyFilters(
	entries []*leaderboard.LeaderboardEntry,
	query GetLeaderboardQuery,
) []*leaderboard.LeaderboardEntry {
	if !query.OnlyOnline && !query.OnlyAvailableForHelp {
		return entries
	}

	filtered := make([]*leaderboard.LeaderboardEntry, 0, len(entries))
	for _, e := range entries {
		if query.OnlyOnline && !e.IsOnline {
			continue
		}
		if query.OnlyAvailableForHelp && !e.IsAvailableForHelp {
			continue
		}
		filtered = append(filtered, e)
	}

	return filtered
}

// paginate применяет пагинацию к записям.
func (h *GetLeaderboardHandler) paginate(
	entries []*leaderboard.LeaderboardEntry,
	offset, limit int,
) []*leaderboard.LeaderboardEntry {
	if offset >= len(entries) {
		return []*leaderboard.LeaderboardEntry{}
	}

	end := offset + limit
	if end > len(entries) {
		end = len(entries)
	}

	return entries[offset:end]
}

// buildResult формирует итоговый результат.
func (h *GetLeaderboardHandler) buildResult(
	ctx context.Context,
	entries []*leaderboard.LeaderboardEntry,
	query GetLeaderboardQuery,
	cohort leaderboard.Cohort,
) (*GetLeaderboardResult, error) {
	// Получаем статистику
	totalCount, err := h.leaderboardRepo.GetTotalCount(ctx, cohort)
	if err != nil {
		totalCount = len(entries)
	}

	// Считаем онлайн
	onlineCount := 0
	for _, e := range entries {
		if e.IsOnline {
			onlineCount++
		}
	}

	// Конвертируем в DTO
	dtos := make([]LeaderboardEntryDTO, len(entries))
	var totalXP int
	for i, e := range entries {
		dtos[i] = h.toDTO(e)
		totalXP += int(e.XP)
	}

	avgXP := 0
	if len(entries) > 0 {
		avgXP = totalXP / len(entries)
	}

	// Вычисляем медиану
	medianXP := 0
	if len(entries) > 0 {
		mid := len(entries) / 2
		if len(entries)%2 == 0 && mid > 0 {
			medianXP = (int(entries[mid-1].XP) + int(entries[mid].XP)) / 2
		} else {
			medianXP = int(entries[mid].XP)
		}
	}

	page := 1
	if query.Limit > 0 {
		page = (query.Offset / query.Limit) + 1
	}

	hasMore := query.Offset+len(entries) < totalCount

	return &GetLeaderboardResult{
		Entries:     dtos,
		TotalCount:  totalCount,
		Cohort:      string(cohort),
		OnlineCount: onlineCount,
		AverageXP:   avgXP,
		MedianXP:    medianXP,
		GeneratedAt: time.Now().UTC(),
		HasMore:     hasMore,
		Page:        page,
		PageSize:    query.Limit,
	}, nil
}

// toDTO конвертирует доменную сущность в DTO.
func (h *GetLeaderboardHandler) toDTO(e *leaderboard.LeaderboardEntry) LeaderboardEntryDTO {
	dto := LeaderboardEntryDTO{
		Rank:               int(e.Rank),
		StudentID:          e.StudentID,
		DisplayName:        e.DisplayName,
		XP:                 int(e.XP),
		Level:              e.Level,
		Cohort:             string(e.Cohort),
		RankChange:         int(e.RankChange),
		RankDirection:      string(e.Direction()),
		IsOnline:           e.IsOnline,
		IsAvailableForHelp: e.IsAvailableForHelp,
		HelpRating:         e.HelpRating,
	}

	if !e.UpdatedAt.IsZero() {
		dto.LastSeenAt = &e.UpdatedAt
	}

	return dto
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// FormatRankEmoji возвращает эмодзи для позиции в рейтинге.
func FormatRankEmoji(rank int) string {
	switch rank {
	case 1:
		return "🥇"
	case 2:
		return "🥈"
	case 3:
		return "🥉"
	default:
		if rank <= 10 {
			return "🏆"
		}
		if rank <= 50 {
			return "⭐"
		}
		return fmt.Sprintf("#%d", rank)
	}
}

// FormatRankChange форматирует изменение ранга для отображения.
func FormatRankChange(change int) string {
	switch {
	case change > 0:
		return fmt.Sprintf("🔼+%d", change)
	case change < 0:
		return fmt.Sprintf("🔽%d", change)
	default:
		return "➖"
	}
}

// FormatOnlineStatus форматирует онлайн-статус.
func FormatOnlineStatus(isOnline bool, lastSeen *time.Time) string {
	if isOnline {
		return "🟢 online"
	}

	if lastSeen == nil || lastSeen.IsZero() {
		return "⚪ offline"
	}

	elapsed := time.Since(*lastSeen)
	switch {
	case elapsed < 5*time.Minute:
		return "🟡 just now"
	case elapsed < 30*time.Minute:
		return fmt.Sprintf("🟡 %d min ago", int(elapsed.Minutes()))
	case elapsed < time.Hour:
		return "⚪ < 1h ago"
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("⚪ %dh ago", int(elapsed.Hours()))
	default:
		return fmt.Sprintf("⚪ %dd ago", int(elapsed.Hours()/24))
	}
}
