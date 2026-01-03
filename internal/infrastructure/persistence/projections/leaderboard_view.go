// Package projections implements read models for CQRS pattern.
// Projections are denormalized views optimized for fast reads.
// They are updated asynchronously when domain events occur.
//
// Philosophy: "От конкуренции к сотрудничеству" - the leaderboard is not just
// a ranking, but a tool for finding helpers and building community.
package projections

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/domain/leaderboard"
)

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD VIEW - Denormalized Read Model
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardView represents the denormalized leaderboard optimized for reads.
// It combines data from multiple sources (students, online status, help ratings)
// into a single, fast-access structure.
type LeaderboardView struct {
	mu sync.RWMutex

	// entries holds all leaderboard entries indexed by student ID.
	entries map[string]*LeaderboardViewEntry

	// sortedByXP holds entries sorted by XP (descending).
	sortedByXP []*LeaderboardViewEntry

	// sortedByCohort holds entries grouped by cohort, then sorted by XP.
	sortedByCohort map[leaderboard.Cohort][]*LeaderboardViewEntry

	// onlineStudents is a set of currently online student IDs.
	onlineStudents map[string]bool

	// helpersIndex maps task ID to students who completed it (for help finding).
	helpersIndex map[string][]string

	// metadata holds aggregate statistics.
	metadata LeaderboardMetadata

	// lastUpdated is the timestamp of the last update.
	lastUpdated time.Time

	// version is incremented on each update for cache invalidation.
	version int64
}

// LeaderboardViewEntry is a denormalized entry in the leaderboard view.
// It contains all information needed to display a student in the leaderboard
// without additional database queries.
type LeaderboardViewEntry struct {
	// Core identification
	StudentID   string `json:"student_id"`
	TelegramID  int64  `json:"telegram_id"`
	DisplayName string `json:"display_name"`

	// Ranking data
	Rank       leaderboard.Rank       `json:"rank"`
	XP         leaderboard.XP         `json:"xp"`
	Level      int                    `json:"level"`
	RankChange leaderboard.RankChange `json:"rank_change"`

	// Cohort information
	Cohort     leaderboard.Cohort `json:"cohort"`
	CohortRank leaderboard.Rank   `json:"cohort_rank"` // Rank within cohort

	// Online status (denormalized from Redis)
	IsOnline       bool      `json:"is_online"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	OnlineDuration string    `json:"online_duration"` // Human-readable

	// Help capability (для философии "от конкуренции к сотрудничеству")
	IsAvailableForHelp bool    `json:"is_available_for_help"`
	HelpRating         float64 `json:"help_rating"`
	HelpCount          int     `json:"help_count"`
	HelpScore          int     `json:"help_score"` // Computed score for ranking helpers

	// Streak info (для мотивации)
	CurrentStreak int `json:"current_streak"`
	BestStreak    int `json:"best_streak"`

	// Daily progress (Daily Grind)
	TodayXPGained      leaderboard.XP `json:"today_xp_gained"`
	TodayTasksComplete int            `json:"today_tasks_complete"`
	TodayRankChange    int            `json:"today_rank_change"`

	// Timestamps
	JoinedAt  time.Time `json:"joined_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LeaderboardMetadata holds aggregate statistics about the leaderboard.
type LeaderboardMetadata struct {
	TotalStudents         int            `json:"total_students"`
	ActiveStudents        int            `json:"active_students"`
	OnlineStudents        int            `json:"online_students"`
	AvailableHelpersCount int            `json:"available_helpers_count"`
	TotalXP               int            `json:"total_xp"`
	AverageXP             leaderboard.XP `json:"average_xp"`
	MedianXP              leaderboard.XP `json:"median_xp"`
	TopStudentXP          leaderboard.XP `json:"top_student_xp"`
	CohortCount           int            `json:"cohort_count"`
	LastSnapshotAt        time.Time      `json:"last_snapshot_at"`
	Version               int64          `json:"version"`
}

// NewLeaderboardView creates a new empty leaderboard view.
func NewLeaderboardView() *LeaderboardView {
	return &LeaderboardView{
		entries:        make(map[string]*LeaderboardViewEntry),
		sortedByXP:     make([]*LeaderboardViewEntry, 0),
		sortedByCohort: make(map[leaderboard.Cohort][]*LeaderboardViewEntry),
		onlineStudents: make(map[string]bool),
		helpersIndex:   make(map[string][]string),
		lastUpdated:    time.Now().UTC(),
		version:        1,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// REBUILD OPERATIONS (Called when snapshot changes)
// ══════════════════════════════════════════════════════════════════════════════

// RebuildFromSnapshot rebuilds the entire view from a leaderboard snapshot.
// This is called after periodic synchronization with the database.
func (lv *LeaderboardView) RebuildFromSnapshot(snapshot *leaderboard.LeaderboardSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("projections: cannot rebuild from nil snapshot")
	}

	lv.mu.Lock()
	defer lv.mu.Unlock()

	// Clear existing data
	lv.entries = make(map[string]*LeaderboardViewEntry)
	lv.sortedByXP = make([]*LeaderboardViewEntry, 0, len(snapshot.Entries))
	lv.sortedByCohort = make(map[leaderboard.Cohort][]*LeaderboardViewEntry)

	// Build entries from snapshot
	for _, entry := range snapshot.Entries {
		viewEntry := lv.convertToViewEntry(entry)
		lv.entries[entry.StudentID] = viewEntry
		lv.sortedByXP = append(lv.sortedByXP, viewEntry)

		// Group by cohort
		cohortList := lv.sortedByCohort[entry.Cohort]
		lv.sortedByCohort[entry.Cohort] = append(cohortList, viewEntry)
	}

	// Sort by XP (already should be sorted, but ensure consistency)
	lv.sortByXP()

	// Calculate cohort ranks
	lv.calculateCohortRanks()

	// Update metadata
	lv.updateMetadata(snapshot)

	lv.lastUpdated = time.Now().UTC()
	lv.version++

	return nil
}

// convertToViewEntry converts a domain LeaderboardEntry to a ViewEntry.
func (lv *LeaderboardView) convertToViewEntry(entry *leaderboard.LeaderboardEntry) *LeaderboardViewEntry {
	return &LeaderboardViewEntry{
		StudentID:          entry.StudentID,
		DisplayName:        entry.DisplayName,
		Rank:               entry.Rank,
		XP:                 entry.XP,
		Level:              entry.Level,
		RankChange:         entry.RankChange,
		Cohort:             entry.Cohort,
		IsOnline:           entry.IsOnline,
		LastSeenAt:         entry.UpdatedAt,
		IsAvailableForHelp: entry.IsAvailableForHelp,
		HelpRating:         entry.HelpRating,
		UpdatedAt:          entry.UpdatedAt,
	}
}

// sortByXP sorts entries by XP (descending) and assigns ranks.
func (lv *LeaderboardView) sortByXP() {
	sort.Slice(lv.sortedByXP, func(i, j int) bool {
		if lv.sortedByXP[i].XP != lv.sortedByXP[j].XP {
			return lv.sortedByXP[i].XP > lv.sortedByXP[j].XP
		}
		return lv.sortedByXP[i].DisplayName < lv.sortedByXP[j].DisplayName
	})

	// Assign ranks with shared rank support
	currentRank := leaderboard.Rank(1)
	for i, entry := range lv.sortedByXP {
		if i > 0 && entry.XP == lv.sortedByXP[i-1].XP {
			entry.Rank = lv.sortedByXP[i-1].Rank
		} else {
			entry.Rank = currentRank
		}
		currentRank = leaderboard.Rank(i + 2)
	}
}

// calculateCohortRanks calculates ranks within each cohort.
func (lv *LeaderboardView) calculateCohortRanks() {
	for cohort, entries := range lv.sortedByCohort {
		// Sort cohort entries by XP
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].XP != entries[j].XP {
				return entries[i].XP > entries[j].XP
			}
			return entries[i].DisplayName < entries[j].DisplayName
		})

		// Assign cohort ranks
		currentRank := leaderboard.Rank(1)
		for i, entry := range entries {
			if i > 0 && entry.XP == entries[i-1].XP {
				entry.CohortRank = entries[i-1].CohortRank
			} else {
				entry.CohortRank = currentRank
			}
			currentRank = leaderboard.Rank(i + 2)
		}

		lv.sortedByCohort[cohort] = entries
	}
}

// updateMetadata recalculates aggregate metadata.
func (lv *LeaderboardView) updateMetadata(snapshot *leaderboard.LeaderboardSnapshot) {
	lv.metadata = LeaderboardMetadata{
		TotalStudents:  len(lv.entries),
		TotalXP:        snapshot.TotalXP,
		AverageXP:      snapshot.AverageXP,
		CohortCount:    len(lv.sortedByCohort),
		LastSnapshotAt: snapshot.SnapshotAt,
		Version:        lv.version,
	}

	// Count online and available helpers
	online := 0
	helpers := 0
	for _, entry := range lv.entries {
		if entry.IsOnline {
			online++
		}
		if entry.IsAvailableForHelp {
			helpers++
		}
	}
	lv.metadata.OnlineStudents = online
	lv.metadata.AvailableHelpersCount = helpers

	// Get top XP
	if len(lv.sortedByXP) > 0 {
		lv.metadata.TopStudentXP = lv.sortedByXP[0].XP
	}

	// Calculate median
	if len(lv.sortedByXP) > 0 {
		mid := len(lv.sortedByXP) / 2
		if len(lv.sortedByXP)%2 == 0 {
			lv.metadata.MedianXP = leaderboard.XP(
				(int(lv.sortedByXP[mid-1].XP) + int(lv.sortedByXP[mid].XP)) / 2,
			)
		} else {
			lv.metadata.MedianXP = lv.sortedByXP[mid].XP
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// INCREMENTAL UPDATE OPERATIONS (Called on domain events)
// ══════════════════════════════════════════════════════════════════════════════

// UpdateEntry updates a single entry in the view.
// Called when a student's data changes (XP update, status change, etc.)
func (lv *LeaderboardView) UpdateEntry(entry *LeaderboardViewEntry) error {
	if entry == nil {
		return fmt.Errorf("projections: cannot update nil entry")
	}

	lv.mu.Lock()
	defer lv.mu.Unlock()

	// Update or insert entry
	oldEntry, exists := lv.entries[entry.StudentID]
	lv.entries[entry.StudentID] = entry

	// If XP changed, we need to resort
	if !exists || oldEntry.XP != entry.XP {
		lv.rebuildSortedList()
	}

	entry.UpdatedAt = time.Now().UTC()
	lv.lastUpdated = time.Now().UTC()
	lv.version++

	return nil
}

// UpdateOnlineStatus updates the online status for a student.
// Called when online status changes (from Redis tracker).
func (lv *LeaderboardView) UpdateOnlineStatus(studentID string, isOnline bool, lastSeen time.Time) {
	lv.mu.Lock()
	defer lv.mu.Unlock()

	if entry, exists := lv.entries[studentID]; exists {
		entry.IsOnline = isOnline
		entry.LastSeenAt = lastSeen
		entry.OnlineDuration = formatDuration(time.Since(lastSeen))
	}

	if isOnline {
		lv.onlineStudents[studentID] = true
	} else {
		delete(lv.onlineStudents, studentID)
	}

	lv.metadata.OnlineStudents = len(lv.onlineStudents)
}

// UpdateDailyProgress updates the daily progress for a student.
// Called when daily grind data changes.
func (lv *LeaderboardView) UpdateDailyProgress(studentID string, xpGained leaderboard.XP, tasksComplete int, rankChange int) {
	lv.mu.Lock()
	defer lv.mu.Unlock()

	if entry, exists := lv.entries[studentID]; exists {
		entry.TodayXPGained = xpGained
		entry.TodayTasksComplete = tasksComplete
		entry.TodayRankChange = rankChange
	}
}

// UpdateHelpCapability updates help-related fields for a student.
func (lv *LeaderboardView) UpdateHelpCapability(studentID string, isAvailable bool, rating float64, helpCount int) {
	lv.mu.Lock()
	defer lv.mu.Unlock()

	if entry, exists := lv.entries[studentID]; exists {
		entry.IsAvailableForHelp = isAvailable
		entry.HelpRating = rating
		entry.HelpCount = helpCount
		entry.HelpScore = calculateHelpScore(rating, helpCount)
	}

	// Recalculate available helpers count
	count := 0
	for _, e := range lv.entries {
		if e.IsAvailableForHelp {
			count++
		}
	}
	lv.metadata.AvailableHelpersCount = count
}

// UpdateStreak updates streak information for a student.
func (lv *LeaderboardView) UpdateStreak(studentID string, currentStreak, bestStreak int) {
	lv.mu.Lock()
	defer lv.mu.Unlock()

	if entry, exists := lv.entries[studentID]; exists {
		entry.CurrentStreak = currentStreak
		entry.BestStreak = bestStreak
	}
}

// AddTaskCompletion registers that a student completed a task.
// Used for the "find helper" feature.
func (lv *LeaderboardView) AddTaskCompletion(studentID, taskID string) {
	lv.mu.Lock()
	defer lv.mu.Unlock()

	helpers := lv.helpersIndex[taskID]

	// Check if already in the list
	for _, id := range helpers {
		if id == studentID {
			return
		}
	}

	lv.helpersIndex[taskID] = append(helpers, studentID)
}

// rebuildSortedList rebuilds the sorted list from entries map.
func (lv *LeaderboardView) rebuildSortedList() {
	lv.sortedByXP = make([]*LeaderboardViewEntry, 0, len(lv.entries))
	lv.sortedByCohort = make(map[leaderboard.Cohort][]*LeaderboardViewEntry)

	for _, entry := range lv.entries {
		lv.sortedByXP = append(lv.sortedByXP, entry)
		cohortList := lv.sortedByCohort[entry.Cohort]
		lv.sortedByCohort[entry.Cohort] = append(cohortList, entry)
	}

	lv.sortByXP()
	lv.calculateCohortRanks()
}

// ══════════════════════════════════════════════════════════════════════════════
// QUERY OPERATIONS (Fast reads from denormalized data)
// ══════════════════════════════════════════════════════════════════════════════

// GetTop returns the top N students.
func (lv *LeaderboardView) GetTop(ctx context.Context, cohort leaderboard.Cohort, limit int) ([]*LeaderboardViewEntry, error) {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	var source []*LeaderboardViewEntry
	if cohort == leaderboard.CohortAll || cohort == "" {
		source = lv.sortedByXP
	} else {
		source = lv.sortedByCohort[cohort]
	}

	if limit <= 0 || limit > len(source) {
		limit = len(source)
	}

	result := make([]*LeaderboardViewEntry, limit)
	for i := 0; i < limit; i++ {
		result[i] = source[i].clone()
	}

	return result, nil
}

// GetPage returns a page of the leaderboard.
func (lv *LeaderboardView) GetPage(ctx context.Context, cohort leaderboard.Cohort, page, pageSize int) (*LeaderboardPage, error) {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	var source []*LeaderboardViewEntry
	if cohort == leaderboard.CohortAll || cohort == "" {
		source = lv.sortedByXP
	} else {
		source = lv.sortedByCohort[cohort]
	}

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	if offset >= len(source) {
		return &LeaderboardPage{
			Entries:     make([]*LeaderboardViewEntry, 0),
			Page:        page,
			PageSize:    pageSize,
			TotalCount:  len(source),
			TotalPages:  (len(source) + pageSize - 1) / pageSize,
			HasNext:     false,
			HasPrevious: page > 1,
		}, nil
	}

	end := offset + pageSize
	if end > len(source) {
		end = len(source)
	}

	entries := make([]*LeaderboardViewEntry, end-offset)
	for i := offset; i < end; i++ {
		entries[i-offset] = source[i].clone()
	}

	totalPages := (len(source) + pageSize - 1) / pageSize

	return &LeaderboardPage{
		Entries:     entries,
		Page:        page,
		PageSize:    pageSize,
		TotalCount:  len(source),
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}, nil
}

// GetByStudentID returns an entry by student ID.
func (lv *LeaderboardView) GetByStudentID(ctx context.Context, studentID string) (*LeaderboardViewEntry, error) {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	if entry, exists := lv.entries[studentID]; exists {
		return entry.clone(), nil
	}

	return nil, fmt.Errorf("projections: student %s not found in leaderboard", studentID)
}

// GetNeighbors returns students around a given student's rank.
func (lv *LeaderboardView) GetNeighbors(ctx context.Context, studentID string, cohort leaderboard.Cohort, rangeSize int) ([]*LeaderboardViewEntry, error) {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	var source []*LeaderboardViewEntry
	if cohort == leaderboard.CohortAll || cohort == "" {
		source = lv.sortedByXP
	} else {
		source = lv.sortedByCohort[cohort]
	}

	// Find student index
	idx := -1
	for i, entry := range source {
		if entry.StudentID == studentID {
			idx = i
			break
		}
	}

	if idx == -1 {
		return nil, fmt.Errorf("projections: student %s not found in leaderboard", studentID)
	}

	// Calculate range
	from := idx - rangeSize
	to := idx + rangeSize + 1

	if from < 0 {
		from = 0
	}
	if to > len(source) {
		to = len(source)
	}

	result := make([]*LeaderboardViewEntry, to-from)
	for i := from; i < to; i++ {
		result[i-from] = source[i].clone()
	}

	return result, nil
}

// GetOnline returns all online students sorted by XP.
func (lv *LeaderboardView) GetOnline(ctx context.Context, cohort leaderboard.Cohort, limit int) ([]*LeaderboardViewEntry, error) {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	var source []*LeaderboardViewEntry
	if cohort == leaderboard.CohortAll || cohort == "" {
		source = lv.sortedByXP
	} else {
		source = lv.sortedByCohort[cohort]
	}

	result := make([]*LeaderboardViewEntry, 0)
	for _, entry := range source {
		if entry.IsOnline {
			result = append(result, entry.clone())
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// GetAvailableHelpers returns students available for help, sorted by help score.
func (lv *LeaderboardView) GetAvailableHelpers(ctx context.Context, cohort leaderboard.Cohort, limit int) ([]*LeaderboardViewEntry, error) {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	var source []*LeaderboardViewEntry
	if cohort == leaderboard.CohortAll || cohort == "" {
		source = lv.sortedByXP
	} else {
		source = lv.sortedByCohort[cohort]
	}

	// Filter available helpers
	helpers := make([]*LeaderboardViewEntry, 0)
	for _, entry := range source {
		if entry.IsAvailableForHelp {
			helpers = append(helpers, entry.clone())
		}
	}

	// Sort by help score (descending), then by online status
	sort.Slice(helpers, func(i, j int) bool {
		// Online helpers first
		if helpers[i].IsOnline != helpers[j].IsOnline {
			return helpers[i].IsOnline
		}
		// Then by help score
		if helpers[i].HelpScore != helpers[j].HelpScore {
			return helpers[i].HelpScore > helpers[j].HelpScore
		}
		// Then by XP
		return helpers[i].XP > helpers[j].XP
	})

	if limit > 0 && limit < len(helpers) {
		helpers = helpers[:limit]
	}

	return helpers, nil
}

// FindHelpersForTask returns students who completed a specific task.
// This is the core of "от конкуренции к сотрудничеству" philosophy.
func (lv *LeaderboardView) FindHelpersForTask(ctx context.Context, taskID string, limit int) ([]*LeaderboardViewEntry, error) {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	studentIDs, exists := lv.helpersIndex[taskID]
	if !exists || len(studentIDs) == 0 {
		return make([]*LeaderboardViewEntry, 0), nil
	}

	helpers := make([]*LeaderboardViewEntry, 0, len(studentIDs))
	for _, id := range studentIDs {
		if entry, ok := lv.entries[id]; ok {
			helpers = append(helpers, entry.clone())
		}
	}

	// Sort by: online status, help score, XP
	sort.Slice(helpers, func(i, j int) bool {
		// Online first
		if helpers[i].IsOnline != helpers[j].IsOnline {
			return helpers[i].IsOnline
		}
		// Available for help
		if helpers[i].IsAvailableForHelp != helpers[j].IsAvailableForHelp {
			return helpers[i].IsAvailableForHelp
		}
		// Then by help rating
		if helpers[i].HelpRating != helpers[j].HelpRating {
			return helpers[i].HelpRating > helpers[j].HelpRating
		}
		return helpers[i].XP > helpers[j].XP
	})

	if limit > 0 && limit < len(helpers) {
		helpers = helpers[:limit]
	}

	return helpers, nil
}

// GetMetadata returns current metadata about the leaderboard.
func (lv *LeaderboardView) GetMetadata(ctx context.Context) LeaderboardMetadata {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	return lv.metadata
}

// GetCohorts returns a list of all cohorts with their student counts.
func (lv *LeaderboardView) GetCohorts(ctx context.Context) []CohortInfo {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	result := make([]CohortInfo, 0, len(lv.sortedByCohort))
	for cohort, entries := range lv.sortedByCohort {
		online := 0
		for _, e := range entries {
			if e.IsOnline {
				online++
			}
		}
		topRank := leaderboard.Rank(0)
		if len(entries) > 0 {
			topRank = entries[0].Rank
		}
		result = append(result, CohortInfo{
			Cohort:         cohort,
			StudentCount:   len(entries),
			OnlineCount:    online,
			TopStudentRank: topRank,
		})
	}

	// Sort by cohort name
	sort.Slice(result, func(i, j int) bool {
		return string(result[i].Cohort) < string(result[j].Cohort)
	})

	return result
}

// GetVersion returns the current version number.
func (lv *LeaderboardView) GetVersion() int64 {
	lv.mu.RLock()
	defer lv.mu.RUnlock()
	return lv.version
}

// GetLastUpdated returns when the view was last updated.
func (lv *LeaderboardView) GetLastUpdated() time.Time {
	lv.mu.RLock()
	defer lv.mu.RUnlock()
	return lv.lastUpdated
}

// ══════════════════════════════════════════════════════════════════════════════
// SUPPORTING TYPES
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardPage represents a paginated result.
type LeaderboardPage struct {
	Entries     []*LeaderboardViewEntry `json:"entries"`
	Page        int                     `json:"page"`
	PageSize    int                     `json:"page_size"`
	TotalCount  int                     `json:"total_count"`
	TotalPages  int                     `json:"total_pages"`
	HasNext     bool                    `json:"has_next"`
	HasPrevious bool                    `json:"has_previous"`
}

// CohortInfo contains summary info about a cohort.
type CohortInfo struct {
	Cohort         leaderboard.Cohort `json:"cohort"`
	StudentCount   int                `json:"student_count"`
	OnlineCount    int                `json:"online_count"`
	TopStudentRank leaderboard.Rank   `json:"top_student_rank"`
}

// clone creates a copy of the entry to prevent data races.
func (e *LeaderboardViewEntry) clone() *LeaderboardViewEntry {
	if e == nil {
		return nil
	}
	entryCopy := *e
	return &entryCopy
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// calculateHelpScore computes a help score based on rating and count.
func calculateHelpScore(rating float64, helpCount int) int {
	if helpCount == 0 {
		return 0
	}

	// Base score from rating (0-50)
	ratingScore := rating * 10

	// Bonus for help count (0-50, logarithmic scale)
	countBonus := float64(helpCount)
	if countBonus > 50 {
		countBonus = 50
	}

	return int(ratingScore + countBonus)
}

// formatDuration formats a duration to human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "только что"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		return fmt.Sprintf("%d мин назад", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%d ч назад", hours)
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%d дн назад", days)
}

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD VIEW REPOSITORY INTERFACE
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardViewRepository defines the interface for leaderboard view storage.
// The view can be stored in memory, Redis, or a database.
type LeaderboardViewRepository interface {
	// Save persists the entire view (for checkpointing).
	Save(ctx context.Context, view *LeaderboardView) error

	// Load loads the view from persistent storage.
	Load(ctx context.Context) (*LeaderboardView, error)

	// GetEntry retrieves a single entry.
	GetEntry(ctx context.Context, studentID string) (*LeaderboardViewEntry, error)

	// SaveEntry persists a single entry.
	SaveEntry(ctx context.Context, entry *LeaderboardViewEntry) error
}
