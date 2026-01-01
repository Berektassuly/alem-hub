// Package leaderboard содержит доменную модель лидерборда Alem Community Hub.
// Лидерборд - это не просто рейтинг, а инструмент для мотивации и объединения студентов.
// Философия: "От конкуренции к сотрудничеству" - мы показываем не только место,
// но и возможности для взаимопомощи.
package leaderboard

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// VALUE OBJECTS
// ══════════════════════════════════════════════════════════════════════════════

// Rank представляет позицию студента в лидерборде.
// Rank начинается с 1 (первое место).
type Rank int

// IsValid проверяет, что ранг положительный.
func (r Rank) IsValid() bool {
	return r > 0
}

// IsTop10 возвращает true, если студент в топ-10.
func (r Rank) IsTop10() bool {
	return r >= 1 && r <= 10
}

// IsTop50 возвращает true, если студент в топ-50.
func (r Rank) IsTop50() bool {
	return r >= 1 && r <= 50
}

// IsTop100 возвращает true, если студент в топ-100.
func (r Rank) IsTop100() bool {
	return r >= 1 && r <= 100
}

// String возвращает строковое представление ранга.
func (r Rank) String() string {
	return fmt.Sprintf("#%d", r)
}

// RankChange представляет изменение позиции в рейтинге.
// Положительное значение = подъём, отрицательное = падение.
type RankChange int

// Direction возвращает направление изменения.
func (rc RankChange) Direction() RankDirection {
	switch {
	case rc > 0:
		return RankDirectionUp
	case rc < 0:
		return RankDirectionDown
	default:
		return RankDirectionStable
	}
}

// Abs возвращает абсолютное значение изменения.
func (rc RankChange) Abs() int {
	if rc < 0 {
		return int(-rc)
	}
	return int(rc)
}

// IsSignificant возвращает true, если изменение значительное (более N позиций).
func (rc RankChange) IsSignificant(threshold int) bool {
	return rc.Abs() >= threshold
}

// String возвращает строковое представление изменения.
func (rc RankChange) String() string {
	switch {
	case rc > 0:
		return fmt.Sprintf("+%d", rc)
	case rc < 0:
		return fmt.Sprintf("%d", rc)
	default:
		return "±0"
	}
}

// RankDirection определяет направление изменения ранга.
type RankDirection string

const (
	// RankDirectionUp - студент поднялся в рейтинге.
	RankDirectionUp RankDirection = "up"
	// RankDirectionDown - студент опустился в рейтинге.
	RankDirectionDown RankDirection = "down"
	// RankDirectionStable - позиция не изменилась.
	RankDirectionStable RankDirection = "stable"
	// RankDirectionNew - новый участник в рейтинге.
	RankDirectionNew RankDirection = "new"
)

// Emoji возвращает эмодзи для отображения направления.
func (rd RankDirection) Emoji() string {
	switch rd {
	case RankDirectionUp:
		return "🔼"
	case RankDirectionDown:
		return "🔽"
	case RankDirectionNew:
		return "🆕"
	default:
		return "➖"
	}
}

// XP представляет очки опыта для лидерборда.
type XP int

// IsValid проверяет, что XP неотрицательный.
func (x XP) IsValid() bool {
	return x >= 0
}

// Cohort представляет поток студентов (например, "2024-spring").
// Лидерборд может быть отфильтрован по когорте.
type Cohort string

// CohortAll представляет общий лидерборд без фильтрации по когорте.
const CohortAll Cohort = ""

// IsValid проверяет корректность когорты.
func (c Cohort) IsValid() bool {
	if c == CohortAll {
		return true
	}
	s := string(c)
	return len(s) >= 4 && len(s) <= 30
}

// String возвращает строковое представление когорты.
func (c Cohort) String() string {
	if c == CohortAll {
		return "all"
	}
	return string(c)
}

// IsFiltered возвращает true, если это фильтр по конкретной когорте.
func (c Cohort) IsFiltered() bool {
	return c != CohortAll
}

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD ENTRY
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardEntry представляет одну запись в лидерборде.
// Содержит всю информацию для отображения студента в рейтинге.
type LeaderboardEntry struct {
	// Rank - текущая позиция в рейтинге.
	Rank Rank

	// StudentID - внутренний идентификатор студента.
	StudentID string

	// DisplayName - отображаемое имя студента.
	DisplayName string

	// XP - текущее количество очков опыта.
	XP XP

	// Level - уровень студента (вычисляется из XP).
	Level int

	// Cohort - когорта студента.
	Cohort Cohort

	// RankChange - изменение позиции с прошлого снапшота.
	RankChange RankChange

	// IsOnline - онлайн ли студент сейчас.
	IsOnline bool

	// IsAvailableForHelp - готов ли помогать (онлайн + настройки).
	IsAvailableForHelp bool

	// HelpRating - рейтинг как помощника (0.0 - 5.0).
	HelpRating float64

	// UpdatedAt - время последнего обновления XP.
	UpdatedAt time.Time
}

// NewLeaderboardEntry создаёт новую запись лидерборда с валидацией.
func NewLeaderboardEntry(
	rank Rank,
	studentID string,
	displayName string,
	xp XP,
	level int,
	cohort Cohort,
) (*LeaderboardEntry, error) {
	if !rank.IsValid() {
		return nil, ErrInvalidRank
	}
	if studentID == "" {
		return nil, ErrInvalidStudentID
	}
	if !xp.IsValid() {
		return nil, ErrInvalidXP
	}

	return &LeaderboardEntry{
		Rank:               rank,
		StudentID:          studentID,
		DisplayName:        displayName,
		XP:                 xp,
		Level:              level,
		Cohort:             cohort,
		RankChange:         0,
		IsOnline:           false,
		IsAvailableForHelp: false,
		HelpRating:         0.0,
		UpdatedAt:          time.Now().UTC(),
	}, nil
}

// Direction возвращает направление изменения ранга.
func (e *LeaderboardEntry) Direction() RankDirection {
	return e.RankChange.Direction()
}

// HasImproved возвращает true, если студент поднялся в рейтинге.
func (e *LeaderboardEntry) HasImproved() bool {
	return e.RankChange > 0
}

// HasDropped возвращает true, если студент опустился в рейтинге.
func (e *LeaderboardEntry) HasDropped() bool {
	return e.RankChange < 0
}

// IsStable возвращает true, если позиция не изменилась.
func (e *LeaderboardEntry) IsStable() bool {
	return e.RankChange == 0
}

// XPToNext возвращает количество XP до следующего места.
// Требует информацию о следующем месте (XP студента выше).
func (e *LeaderboardEntry) XPToNext(nextXP XP) XP {
	if nextXP <= e.XP {
		return 0
	}
	return nextXP - e.XP + 1
}

// XPGap возвращает разрыв в XP с указанным студентом.
func (e *LeaderboardEntry) XPGap(other *LeaderboardEntry) XP {
	if other == nil {
		return 0
	}
	diff := e.XP - other.XP
	if diff < 0 {
		return XP(-diff)
	}
	return XP(diff)
}

// Clone создаёт копию записи.
func (e *LeaderboardEntry) Clone() *LeaderboardEntry {
	if e == nil {
		return nil
	}
	clone := *e
	return &clone
}

// String возвращает строковое представление для логирования.
func (e *LeaderboardEntry) String() string {
	return fmt.Sprintf(
		"Entry{Rank: %d, DisplayName: %s, XP: %d, Change: %s}",
		e.Rank, e.DisplayName, e.XP, e.RankChange.String(),
	)
}

// ══════════════════════════════════════════════════════════════════════════════
// RANKING (Ranked List)
// ══════════════════════════════════════════════════════════════════════════════

// Ranking представляет полный отсортированный список студентов.
// Это вспомогательная структура для построения лидерборда.
type Ranking struct {
	entries []*LeaderboardEntry
	byID    map[string]*LeaderboardEntry
}

// NewRanking создаёт пустой Ranking.
func NewRanking() *Ranking {
	return &Ranking{
		entries: make([]*LeaderboardEntry, 0),
		byID:    make(map[string]*LeaderboardEntry),
	}
}

// Add добавляет запись в рейтинг (без автоматической сортировки).
func (r *Ranking) Add(entry *LeaderboardEntry) error {
	if entry == nil {
		return ErrNilEntry
	}
	if _, exists := r.byID[entry.StudentID]; exists {
		return ErrDuplicateStudent
	}

	r.entries = append(r.entries, entry)
	r.byID[entry.StudentID] = entry
	return nil
}

// SortByXP сортирует записи по XP (по убыванию) и присваивает ранги.
func (r *Ranking) SortByXP() {
	sort.Slice(r.entries, func(i, j int) bool {
		// По убыванию XP
		if r.entries[i].XP != r.entries[j].XP {
			return r.entries[i].XP > r.entries[j].XP
		}
		// При равном XP - по алфавиту DisplayName (стабильная сортировка)
		return r.entries[i].DisplayName < r.entries[j].DisplayName
	})

	// Присваиваем ранги с учётом "shared rank" (одинаковый XP = одинаковый ранг)
	currentRank := Rank(1)
	for i, entry := range r.entries {
		if i > 0 && entry.XP == r.entries[i-1].XP {
			// Одинаковый XP = тот же ранг
			entry.Rank = r.entries[i-1].Rank
		} else {
			entry.Rank = currentRank
		}
		currentRank = Rank(i + 2) // Следующий "реальный" ранг
	}
}

// GetByID возвращает запись по ID студента.
func (r *Ranking) GetByID(studentID string) *LeaderboardEntry {
	return r.byID[studentID]
}

// GetByRank возвращает запись по рангу.
// Если несколько студентов делят один ранг, возвращает первого.
func (r *Ranking) GetByRank(rank Rank) *LeaderboardEntry {
	for _, entry := range r.entries {
		if entry.Rank == rank {
			return entry
		}
	}
	return nil
}

// Top возвращает топ-N записей.
func (r *Ranking) Top(n int) []*LeaderboardEntry {
	if n <= 0 {
		return nil
	}
	if n > len(r.entries) {
		n = len(r.entries)
	}
	result := make([]*LeaderboardEntry, n)
	copy(result, r.entries[:n])
	return result
}

// Slice возвращает срез записей [from:to).
func (r *Ranking) Slice(from, to int) []*LeaderboardEntry {
	if from < 0 {
		from = 0
	}
	if to > len(r.entries) {
		to = len(r.entries)
	}
	if from >= to {
		return nil
	}
	result := make([]*LeaderboardEntry, to-from)
	copy(result, r.entries[from:to])
	return result
}

// Neighbors возвращает соседей студента по рангу (±range).
// Включает самого студента в центре.
func (r *Ranking) Neighbors(studentID string, rangeSize int) []*LeaderboardEntry {
	entry := r.GetByID(studentID)
	if entry == nil {
		return nil
	}

	// Находим индекс студента
	var idx int
	for i, e := range r.entries {
		if e.StudentID == studentID {
			idx = i
			break
		}
	}

	from := idx - rangeSize
	to := idx + rangeSize + 1

	if from < 0 {
		from = 0
	}
	if to > len(r.entries) {
		to = len(r.entries)
	}

	return r.Slice(from, to)
}

// FilterByCohort возвращает новый Ranking только с указанной когортой.
func (r *Ranking) FilterByCohort(cohort Cohort) *Ranking {
	filtered := NewRanking()

	for _, entry := range r.entries {
		if entry.Cohort == cohort {
			entryCopy := entry.Clone()
			_ = filtered.Add(entryCopy) // Ошибка невозможна - копии уникальны
		}
	}

	filtered.SortByXP()
	return filtered
}

// FilterOnline возвращает новый Ranking только с онлайн студентами.
func (r *Ranking) FilterOnline() *Ranking {
	filtered := NewRanking()

	for _, entry := range r.entries {
		if entry.IsOnline {
			entryCopy := entry.Clone()
			_ = filtered.Add(entryCopy)
		}
	}

	filtered.SortByXP()
	return filtered
}

// FilterAvailableForHelp возвращает студентов, готовых помочь.
func (r *Ranking) FilterAvailableForHelp() *Ranking {
	filtered := NewRanking()

	for _, entry := range r.entries {
		if entry.IsAvailableForHelp {
			entryCopy := entry.Clone()
			_ = filtered.Add(entryCopy)
		}
	}

	filtered.SortByXP()
	return filtered
}

// Count возвращает общее количество записей.
func (r *Ranking) Count() int {
	return len(r.entries)
}

// All возвращает все записи.
func (r *Ranking) All() []*LeaderboardEntry {
	result := make([]*LeaderboardEntry, len(r.entries))
	copy(result, r.entries)
	return result
}

// AverageXP возвращает средний XP по всем участникам.
func (r *Ranking) AverageXP() XP {
	if len(r.entries) == 0 {
		return 0
	}

	var total int
	for _, entry := range r.entries {
		total += int(entry.XP)
	}

	return XP(total / len(r.entries))
}

// MedianXP возвращает медианный XP.
func (r *Ranking) MedianXP() XP {
	if len(r.entries) == 0 {
		return 0
	}

	mid := len(r.entries) / 2
	if len(r.entries)%2 == 0 {
		return XP((int(r.entries[mid-1].XP) + int(r.entries[mid].XP)) / 2)
	}
	return r.entries[mid].XP
}

// ══════════════════════════════════════════════════════════════════════════════
// DOMAIN ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrInvalidRank - невалидный ранг (должен быть положительным).
	ErrInvalidRank = errors.New("invalid rank: must be positive")

	// ErrInvalidStudentID - невалидный ID студента.
	ErrInvalidStudentID = errors.New("invalid student id: cannot be empty")

	// ErrInvalidXP - невалидное значение XP.
	ErrInvalidXP = errors.New("invalid xp: must be non-negative")

	// ErrInvalidCohort - невалидная когорта.
	ErrInvalidCohort = errors.New("invalid cohort: must be 4-30 chars")

	// ErrNilEntry - попытка добавить nil запись.
	ErrNilEntry = errors.New("cannot add nil entry")

	// ErrDuplicateStudent - студент уже есть в рейтинге.
	ErrDuplicateStudent = errors.New("student already exists in ranking")

	// ErrSnapshotNotFound - снапшот не найден.
	ErrSnapshotNotFound = errors.New("leaderboard snapshot not found")

	// ErrEmptyLeaderboard - лидерборд пуст.
	ErrEmptyLeaderboard = errors.New("leaderboard is empty")
)
