// Package leaderboard содержит доменную модель лидерборда Alem Community Hub.
package leaderboard

import (
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD SNAPSHOT
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardSnapshot представляет состояние лидерборда в определённый момент времени.
// Снапшоты используются для:
// 1. Отслеживания изменений позиций (RankChange)
// 2. Аналитики и истории
// 3. Быстрого чтения (CQRS Read Model)
type LeaderboardSnapshot struct {
	// ID - уникальный идентификатор снапшота.
	ID string

	// Cohort - когорта, для которой создан снапшот (пустая = общий).
	Cohort Cohort

	// SnapshotAt - время создания снапшота.
	SnapshotAt time.Time

	// TotalStudents - общее количество студентов в снапшоте.
	TotalStudents int

	// TotalXP - суммарный XP всех студентов.
	TotalXP int

	// AverageXP - средний XP.
	AverageXP XP

	// Entries - список записей лидерборда (отсортирован по рангу).
	Entries []*LeaderboardEntry

	// byID - индекс для быстрого поиска по ID.
	byID map[string]*LeaderboardEntry
}

// NewLeaderboardSnapshot создаёт новый снапшот из Ranking.
func NewLeaderboardSnapshot(id string, cohort Cohort, ranking *Ranking) *LeaderboardSnapshot {
	if ranking == nil {
		return &LeaderboardSnapshot{
			ID:         id,
			Cohort:     cohort,
			SnapshotAt: time.Now().UTC(),
			Entries:    make([]*LeaderboardEntry, 0),
			byID:       make(map[string]*LeaderboardEntry),
		}
	}

	entries := ranking.All()
	byID := make(map[string]*LeaderboardEntry, len(entries))

	var totalXP int
	for _, entry := range entries {
		byID[entry.StudentID] = entry
		totalXP += int(entry.XP)
	}

	var avgXP XP
	if len(entries) > 0 {
		avgXP = XP(totalXP / len(entries))
	}

	return &LeaderboardSnapshot{
		ID:            id,
		Cohort:        cohort,
		SnapshotAt:    time.Now().UTC(),
		TotalStudents: len(entries),
		TotalXP:       totalXP,
		AverageXP:     avgXP,
		Entries:       entries,
		byID:          byID,
	}
}

// NewEmptySnapshot создаёт пустой снапшот.
func NewEmptySnapshot(id string, cohort Cohort) *LeaderboardSnapshot {
	return &LeaderboardSnapshot{
		ID:         id,
		Cohort:     cohort,
		SnapshotAt: time.Now().UTC(),
		Entries:    make([]*LeaderboardEntry, 0),
		byID:       make(map[string]*LeaderboardEntry),
	}
}

// GetByID возвращает запись по ID студента.
func (s *LeaderboardSnapshot) GetByID(studentID string) *LeaderboardEntry {
	if s.byID == nil {
		return nil
	}
	return s.byID[studentID]
}

// GetByRank возвращает запись по рангу.
func (s *LeaderboardSnapshot) GetByRank(rank Rank) *LeaderboardEntry {
	for _, entry := range s.Entries {
		if entry.Rank == rank {
			return entry
		}
	}
	return nil
}

// GetRank возвращает ранг студента по его ID.
// Возвращает 0, если студент не найден.
func (s *LeaderboardSnapshot) GetRank(studentID string) Rank {
	entry := s.GetByID(studentID)
	if entry == nil {
		return 0
	}
	return entry.Rank
}

// Top возвращает топ-N записей.
func (s *LeaderboardSnapshot) Top(n int) []*LeaderboardEntry {
	if n <= 0 {
		return nil
	}
	if n > len(s.Entries) {
		n = len(s.Entries)
	}
	result := make([]*LeaderboardEntry, n)
	copy(result, s.Entries[:n])
	return result
}

// Page возвращает страницу лидерборда.
// page начинается с 1, pageSize - количество записей на странице.
func (s *LeaderboardSnapshot) Page(page, pageSize int) []*LeaderboardEntry {
	if page < 1 || pageSize <= 0 {
		return nil
	}

	from := (page - 1) * pageSize
	to := from + pageSize

	if from >= len(s.Entries) {
		return nil
	}
	if to > len(s.Entries) {
		to = len(s.Entries)
	}

	result := make([]*LeaderboardEntry, to-from)
	copy(result, s.Entries[from:to])
	return result
}

// TotalPages возвращает общее количество страниц.
func (s *LeaderboardSnapshot) TotalPages(pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	pages := len(s.Entries) / pageSize
	if len(s.Entries)%pageSize != 0 {
		pages++
	}
	return pages
}

// Neighbors возвращает соседей студента по рангу (±rangeSize).
func (s *LeaderboardSnapshot) Neighbors(studentID string, rangeSize int) []*LeaderboardEntry {
	entry := s.GetByID(studentID)
	if entry == nil {
		return nil
	}

	// Находим индекс студента
	var idx int
	for i, e := range s.Entries {
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
	if to > len(s.Entries) {
		to = len(s.Entries)
	}

	result := make([]*LeaderboardEntry, to-from)
	copy(result, s.Entries[from:to])
	return result
}

// IsEmpty возвращает true, если снапшот пуст.
func (s *LeaderboardSnapshot) IsEmpty() bool {
	return len(s.Entries) == 0
}

// Count возвращает количество записей.
func (s *LeaderboardSnapshot) Count() int {
	return len(s.Entries)
}

// Contains проверяет, есть ли студент в снапшоте.
func (s *LeaderboardSnapshot) Contains(studentID string) bool {
	return s.GetByID(studentID) != nil
}

// TopStudentIDs возвращает список ID студентов в топ-N.
func (s *LeaderboardSnapshot) TopStudentIDs(n int) []string {
	top := s.Top(n)
	ids := make([]string, len(top))
	for i, entry := range top {
		ids[i] = entry.StudentID
	}
	return ids
}

// OnlineCount возвращает количество онлайн студентов.
func (s *LeaderboardSnapshot) OnlineCount() int {
	count := 0
	for _, entry := range s.Entries {
		if entry.IsOnline {
			count++
		}
	}
	return count
}

// AvailableForHelpCount возвращает количество студентов, готовых помочь.
func (s *LeaderboardSnapshot) AvailableForHelpCount() int {
	count := 0
	for _, entry := range s.Entries {
		if entry.IsAvailableForHelp {
			count++
		}
	}
	return count
}

// ══════════════════════════════════════════════════════════════════════════════
// SNAPSHOT DIFF (Changes between snapshots)
// ══════════════════════════════════════════════════════════════════════════════

// SnapshotDiff представляет различия между двумя снапшотами.
// Используется для определения изменений рангов и генерации уведомлений.
type SnapshotDiff struct {
	// OldSnapshot - предыдущий снапшот.
	OldSnapshot *LeaderboardSnapshot

	// NewSnapshot - новый снапшот.
	NewSnapshot *LeaderboardSnapshot

	// RankChanges - карта изменений рангов (studentID -> RankChange).
	RankChanges map[string]RankChange

	// NewEntries - новые студенты (не было в старом снапшоте).
	NewEntries []*LeaderboardEntry

	// RemovedEntries - удалённые студенты (были в старом, нет в новом).
	RemovedEntries []*LeaderboardEntry

	// TopChanges - студенты, которые вошли/вышли из топ-N.
	TopChanges []TopChange
}

// TopChange представляет изменение в топе (вход/выход).
type TopChange struct {
	StudentID string

	OldRank    Rank
	NewRank    Rank
	EnteredTop int // Вошёл в топ-N (0 если не вошёл)
	LeftTop    int // Вышел из топ-N (0 если не вышел)
}

// IsEntered возвращает true, если студент вошёл в топ.
func (tc *TopChange) IsEntered() bool {
	return tc.EnteredTop > 0
}

// IsLeft возвращает true, если студент вышел из топа.
func (tc *TopChange) IsLeft() bool {
	return tc.LeftTop > 0
}

// CalculateDiff вычисляет разницу между двумя снапшотами.
// oldSnapshot может быть nil (первый снапшот).
func CalculateDiff(oldSnapshot, newSnapshot *LeaderboardSnapshot) *SnapshotDiff {
	diff := &SnapshotDiff{
		OldSnapshot:    oldSnapshot,
		NewSnapshot:    newSnapshot,
		RankChanges:    make(map[string]RankChange),
		NewEntries:     make([]*LeaderboardEntry, 0),
		RemovedEntries: make([]*LeaderboardEntry, 0),
		TopChanges:     make([]TopChange, 0),
	}

	if newSnapshot == nil {
		return diff
	}

	// Если старого снапшота нет, все записи новые
	if oldSnapshot == nil || oldSnapshot.IsEmpty() {
		for _, entry := range newSnapshot.Entries {
			entry.RankChange = 0 // Новый участник, нет изменений
			diff.NewEntries = append(diff.NewEntries, entry)
		}
		return diff
	}

	// Обрабатываем каждую запись нового снапшота
	for _, newEntry := range newSnapshot.Entries {
		oldEntry := oldSnapshot.GetByID(newEntry.StudentID)

		if oldEntry == nil {
			// Новый студент
			newEntry.RankChange = 0
			diff.NewEntries = append(diff.NewEntries, newEntry)
		} else {
			// Существующий студент - вычисляем изменение ранга
			// Положительное значение = поднялся (был 10, стал 5 = +5)
			rankChange := RankChange(int(oldEntry.Rank) - int(newEntry.Rank))
			newEntry.RankChange = rankChange
			diff.RankChanges[newEntry.StudentID] = rankChange

			// Проверяем изменения в топах
			topChange := checkTopChange(oldEntry, newEntry)
			if topChange != nil {
				diff.TopChanges = append(diff.TopChanges, *topChange)
			}
		}
	}

	// Находим удалённых студентов
	for _, oldEntry := range oldSnapshot.Entries {
		if !newSnapshot.Contains(oldEntry.StudentID) {
			diff.RemovedEntries = append(diff.RemovedEntries, oldEntry)
		}
	}

	return diff
}

// checkTopChange проверяет изменения в топах (10, 50, 100).
func checkTopChange(oldEntry, newEntry *LeaderboardEntry) *TopChange {
	topLevels := []int{100, 50, 10}

	for _, topN := range topLevels {
		wasInTop := int(oldEntry.Rank) <= topN
		nowInTop := int(newEntry.Rank) <= topN

		if !wasInTop && nowInTop {
			// Вошёл в топ
			return &TopChange{
				StudentID:  newEntry.StudentID,
				OldRank:    oldEntry.Rank,
				NewRank:    newEntry.Rank,
				EnteredTop: topN,
			}
		}
		if wasInTop && !nowInTop {
			// Вышел из топа
			return &TopChange{
				StudentID: newEntry.StudentID,
				OldRank:   oldEntry.Rank,
				NewRank:   newEntry.Rank,
				LeftTop:   topN,
			}
		}
	}

	return nil
}

// GetRankChange возвращает изменение ранга для студента.
func (d *SnapshotDiff) GetRankChange(studentID string) RankChange {
	return d.RankChanges[studentID]
}

// HasChanges возвращает true, если есть какие-либо изменения.
func (d *SnapshotDiff) HasChanges() bool {
	return len(d.RankChanges) > 0 || len(d.NewEntries) > 0 || len(d.RemovedEntries) > 0
}

// SignificantChanges возвращает студентов с изменением ранга >= threshold.
func (d *SnapshotDiff) SignificantChanges(threshold int) []string {
	result := make([]string, 0)
	for studentID, change := range d.RankChanges {
		if change.IsSignificant(threshold) {
			result = append(result, studentID)
		}
	}
	return result
}

// Improved возвращает студентов, которые поднялись в рейтинге.
func (d *SnapshotDiff) Improved() []string {
	result := make([]string, 0)
	for studentID, change := range d.RankChanges {
		if change > 0 {
			result = append(result, studentID)
		}
	}
	return result
}

// Dropped возвращает студентов, которые опустились в рейтинге.
func (d *SnapshotDiff) Dropped() []string {
	result := make([]string, 0)
	for studentID, change := range d.RankChanges {
		if change < 0 {
			result = append(result, studentID)
		}
	}
	return result
}

// ══════════════════════════════════════════════════════════════════════════════
// SNAPSHOT METADATA (for storage)
// ══════════════════════════════════════════════════════════════════════════════

// SnapshotMeta содержит метаданные снапшота без самих записей.
// Используется для списка снапшотов и быстрого поиска.
type SnapshotMeta struct {
	ID            string
	Cohort        Cohort
	SnapshotAt    time.Time
	TotalStudents int
	TotalXP       int
	AverageXP     XP
}

// ToMeta преобразует снапшот в метаданные.
func (s *LeaderboardSnapshot) ToMeta() SnapshotMeta {
	return SnapshotMeta{
		ID:            s.ID,
		Cohort:        s.Cohort,
		SnapshotAt:    s.SnapshotAt,
		TotalStudents: s.TotalStudents,
		TotalXP:       s.TotalXP,
		AverageXP:     s.AverageXP,
	}
}

// String возвращает строковое представление для логирования.
func (s *LeaderboardSnapshot) String() string {
	return fmt.Sprintf(
		"Snapshot{ID: %s, Cohort: %s, Students: %d, AvgXP: %d, At: %s}",
		s.ID, s.Cohort.String(), s.TotalStudents, s.AverageXP,
		s.SnapshotAt.Format(time.RFC3339),
	)
}

// RebuildIndex перестраивает внутренний индекс byID.
// Используется после десериализации из БД.
func (s *LeaderboardSnapshot) RebuildIndex() {
	s.byID = make(map[string]*LeaderboardEntry, len(s.Entries))
	for _, entry := range s.Entries {
		s.byID[entry.StudentID] = entry
	}
}
