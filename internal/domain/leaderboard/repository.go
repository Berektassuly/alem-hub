// Package leaderboard содержит доменную модель лидерборда Alem Community Hub.
package leaderboard

import (
	"context"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD REPOSITORY INTERFACE
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardRepository определяет контракт для работы с лидербордом.
// Реализация находится в infrastructure слое (PostgreSQL, Redis, etc.).
//
// Философия: репозиторий скрывает детали хранения и предоставляет
// высокоуровневые операции, соответствующие бизнес-логике.
type LeaderboardRepository interface {
	// ──────────────────────────────────────────────────────────────────────────
	// SNAPSHOT OPERATIONS
	// ──────────────────────────────────────────────────────────────────────────

	// SaveSnapshot сохраняет снапшот лидерборда.
	// Каждый снапшот — это момент времени, используемый для расчёта изменений.
	SaveSnapshot(ctx context.Context, snapshot *LeaderboardSnapshot) error

	// GetLatestSnapshot возвращает последний снапшот для когорты.
	// Если cohort == CohortAll, возвращает общий лидерборд.
	GetLatestSnapshot(ctx context.Context, cohort Cohort) (*LeaderboardSnapshot, error)

	// GetSnapshotByID возвращает снапшот по его ID.
	GetSnapshotByID(ctx context.Context, id string) (*LeaderboardSnapshot, error)

	// GetSnapshotAt возвращает снапшот, актуальный на указанное время.
	// Используется для исторического анализа.
	GetSnapshotAt(ctx context.Context, cohort Cohort, at time.Time) (*LeaderboardSnapshot, error)

	// GetPreviousSnapshot возвращает предыдущий снапшот перед указанным.
	// Используется для расчёта RankChange.
	GetPreviousSnapshot(ctx context.Context, snapshotID string) (*LeaderboardSnapshot, error)

	// ListSnapshots возвращает список метаданных снапшотов за период.
	ListSnapshots(ctx context.Context, cohort Cohort, from, to time.Time) ([]SnapshotMeta, error)

	// DeleteOldSnapshots удаляет снапшоты старше указанного времени.
	// Возвращает количество удалённых снапшотов.
	DeleteOldSnapshots(ctx context.Context, olderThan time.Time) (int, error)

	// ──────────────────────────────────────────────────────────────────────────
	// RANKING QUERIES (Read Model)
	// ──────────────────────────────────────────────────────────────────────────

	// GetStudentRank возвращает текущую позицию студента в лидерборде.
	// Возвращает nil, если студент не найден.
	GetStudentRank(ctx context.Context, studentID string, cohort Cohort) (*LeaderboardEntry, error)

	// GetTop возвращает топ-N студентов из кеша или последнего снапшота.
	GetTop(ctx context.Context, cohort Cohort, limit int) ([]*LeaderboardEntry, error)

	// GetPage возвращает страницу лидерборда.
	// page начинается с 1, pageSize — количество записей на странице.
	GetPage(ctx context.Context, cohort Cohort, page, pageSize int) ([]*LeaderboardEntry, error)

	// GetNeighbors возвращает соседей студента по рангу (±rangeSize).
	GetNeighbors(ctx context.Context, studentID string, cohort Cohort, rangeSize int) ([]*LeaderboardEntry, error)

	// GetTotalCount возвращает общее количество студентов в лидерборде.
	GetTotalCount(ctx context.Context, cohort Cohort) (int, error)

	// ──────────────────────────────────────────────────────────────────────────
	// RANK HISTORY
	// ──────────────────────────────────────────────────────────────────────────

	// GetRankHistory возвращает историю рангов студента за период.
	GetRankHistory(ctx context.Context, studentID string, from, to time.Time) ([]RankHistoryEntry, error)

	// GetBestRank возвращает лучшую позицию студента за всё время.
	GetBestRank(ctx context.Context, studentID string) (*RankHistoryEntry, error)

	// ──────────────────────────────────────────────────────────────────────────
	// COHORT OPERATIONS
	// ──────────────────────────────────────────────────────────────────────────

	// ListCohorts возвращает список всех когорт с активными студентами.
	ListCohorts(ctx context.Context) ([]Cohort, error)

	// GetCohortStats возвращает статистику по когорте.
	GetCohortStats(ctx context.Context, cohort Cohort) (*CohortStats, error)
}

// ══════════════════════════════════════════════════════════════════════════════
// SUPPORTING TYPES
// ══════════════════════════════════════════════════════════════════════════════

// RankHistoryEntry представляет запись истории рангов.
type RankHistoryEntry struct {
	// StudentID - идентификатор студента.
	StudentID string

	// Rank - ранг в этот момент времени.
	Rank Rank

	// XP - XP в этот момент времени.
	XP XP

	// SnapshotAt - время снапшота.
	SnapshotAt time.Time

	// RankChange - изменение с предыдущего снапшота.
	RankChange RankChange
}

// CohortStats содержит агрегированную статистику по когорте.
type CohortStats struct {
	// Cohort - когорта.
	Cohort Cohort

	// TotalStudents - общее количество студентов.
	TotalStudents int

	// ActiveStudents - количество активных студентов.
	ActiveStudents int

	// TotalXP - суммарный XP когорты.
	TotalXP int

	// AverageXP - средний XP.
	AverageXP XP

	// MedianXP - медианный XP.
	MedianXP XP

	// TopStudentXP - XP лучшего студента.
	TopStudentXP XP

	// OnlineCount - количество онлайн студентов.
	OnlineCount int

	// LastUpdated - время последнего обновления статистики.
	LastUpdated time.Time
}

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD CACHE INTERFACE
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardCache определяет контракт для кеширования лидерборда.
// Отделён от основного репозитория для гибкости (Redis, in-memory).
type LeaderboardCache interface {
	// GetCachedTop возвращает закешированный топ-N.
	// Возвращает nil, если кеш пуст или устарел.
	GetCachedTop(ctx context.Context, cohort Cohort, limit int) ([]*LeaderboardEntry, error)

	// SetCachedTop сохраняет топ-N в кеш с TTL.
	SetCachedTop(ctx context.Context, cohort Cohort, entries []*LeaderboardEntry, ttl time.Duration) error

	// GetCachedRank возвращает закешированный ранг студента.
	GetCachedRank(ctx context.Context, studentID string, cohort Cohort) (*LeaderboardEntry, error)

	// SetCachedRank сохраняет ранг студента в кеш.
	SetCachedRank(ctx context.Context, entry *LeaderboardEntry, ttl time.Duration) error

	// InvalidateCache сбрасывает кеш для когорты.
	InvalidateCache(ctx context.Context, cohort Cohort) error

	// InvalidateAll сбрасывает весь кеш лидерборда.
	InvalidateAll(ctx context.Context) error
}

// ══════════════════════════════════════════════════════════════════════════════
// RANK CHANGE NOTIFIER INTERFACE
// ══════════════════════════════════════════════════════════════════════════════

// RankChangeNotifier определяет контракт для уведомлений об изменениях ранга.
// Реализация связывает leaderboard с notification доменом.
type RankChangeNotifier interface {
	// NotifyRankUp уведомляет студента о повышении в рейтинге.
	NotifyRankUp(ctx context.Context, studentID string, oldRank, newRank Rank, change RankChange) error

	// NotifyRankDown уведомляет студента о понижении в рейтинге.
	NotifyRankDown(ctx context.Context, studentID string, oldRank, newRank Rank, change RankChange) error

	// NotifyEnteredTop уведомляет о входе в топ-N.
	NotifyEnteredTop(ctx context.Context, studentID string, topN int, rank Rank) error

	// NotifyLeftTop уведомляет о выходе из топ-N.
	NotifyLeftTop(ctx context.Context, studentID string, topN int, rank Rank) error

	// NotifyOvertaken уведомляет, что студента обогнал другой студент.
	NotifyOvertaken(ctx context.Context, studentID, overtakerID string, newRank Rank) error
}

// ══════════════════════════════════════════════════════════════════════════════
// QUERY OPTIONS
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardQueryOptions содержит опции для запросов к лидерборду.
type LeaderboardQueryOptions struct {
	// Cohort - фильтр по когорте (пусто = все).
	Cohort Cohort

	// OnlyOnline - показывать только онлайн студентов.
	OnlyOnline bool

	// OnlyAvailableForHelp - только готовые помогать.
	OnlyAvailableForHelp bool

	// MinXP - минимальный XP для фильтрации.
	MinXP XP

	// MaxXP - максимальный XP для фильтрации.
	MaxXP XP

	// Page - номер страницы (начиная с 1).
	Page int

	// PageSize - размер страницы.
	PageSize int
}

// DefaultQueryOptions возвращает опции по умолчанию.
func DefaultQueryOptions() LeaderboardQueryOptions {
	return LeaderboardQueryOptions{
		Cohort:               CohortAll,
		OnlyOnline:           false,
		OnlyAvailableForHelp: false,
		MinXP:                0,
		MaxXP:                0,
		Page:                 1,
		PageSize:             20,
	}
}

// WithCohort устанавливает фильтр по когорте.
func (o LeaderboardQueryOptions) WithCohort(cohort Cohort) LeaderboardQueryOptions {
	o.Cohort = cohort
	return o
}

// WithOnlyOnline включает фильтр только онлайн.
func (o LeaderboardQueryOptions) WithOnlyOnline() LeaderboardQueryOptions {
	o.OnlyOnline = true
	return o
}

// WithOnlyAvailableForHelp включает фильтр только готовых помочь.
func (o LeaderboardQueryOptions) WithOnlyAvailableForHelp() LeaderboardQueryOptions {
	o.OnlyAvailableForHelp = true
	return o
}

// WithPage устанавливает номер страницы.
func (o LeaderboardQueryOptions) WithPage(page int) LeaderboardQueryOptions {
	if page < 1 {
		page = 1
	}
	o.Page = page
	return o
}

// WithPageSize устанавливает размер страницы.
func (o LeaderboardQueryOptions) WithPageSize(size int) LeaderboardQueryOptions {
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	o.PageSize = size
	return o
}

// Offset возвращает смещение для SQL-запроса.
func (o LeaderboardQueryOptions) Offset() int {
	return (o.Page - 1) * o.PageSize
}

// Limit возвращает лимит для SQL-запроса.
func (o LeaderboardQueryOptions) Limit() int {
	return o.PageSize
}
