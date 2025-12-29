// Package projections implements read models for CQRS pattern.
package projections

import (
	"alem-hub/internal/domain/leaderboard"
	"alem-hub/internal/domain/social"
	"alem-hub/internal/domain/student"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT CARD VIEW - Denormalized Read Model for Student Profile
// ══════════════════════════════════════════════════════════════════════════════

// StudentCardView represents the complete denormalized student profile.
// It aggregates data from student, activity, social, and leaderboard domains
// into a single view for fast retrieval when displaying the /me command or profile.
//
// Philosophy: This view supports "от конкуренции к сотрудничеству" by highlighting
// a student's contributions to the community, not just their XP ranking.
type StudentCardView struct {
	mu sync.RWMutex

	// cards holds all student cards indexed by student ID.
	cards map[string]*StudentCard

	// byTelegramID indexes cards by Telegram ID for fast lookup.
	byTelegramID map[int64]*StudentCard

	// byAlemLogin indexes cards by Alem login.
	byAlemLogin map[string]*StudentCard

	// lastUpdated is the timestamp of the last update.
	lastUpdated time.Time

	// version is incremented on each update.
	version int64
}

// StudentCard is a comprehensive denormalized view of a student.
// It contains everything needed to display a full student profile.
type StudentCard struct {
	// ═══════════════════════════════════════════════════════════════════════════
	// CORE IDENTITY
	// ═══════════════════════════════════════════════════════════════════════════

	StudentID   string             `json:"student_id"`
	TelegramID  student.TelegramID `json:"telegram_id"`
	AlemLogin   student.AlemLogin  `json:"alem_login"`
	DisplayName string             `json:"display_name"`
	Cohort      student.Cohort     `json:"cohort"`
	Status      student.Status     `json:"status"`
	JoinedAt    time.Time          `json:"joined_at"`

	// ═══════════════════════════════════════════════════════════════════════════
	// PROGRESS & RANKING
	// ═══════════════════════════════════════════════════════════════════════════

	// Current progress
	CurrentXP    student.XP    `json:"current_xp"`
	CurrentLevel student.Level `json:"current_level"`

	// Ranking in global leaderboard
	GlobalRank       leaderboard.Rank       `json:"global_rank"`
	GlobalRankChange leaderboard.RankChange `json:"global_rank_change"`
	GlobalPercentile float64                `json:"global_percentile"` // Top X%

	// Ranking within cohort
	CohortRank       leaderboard.Rank       `json:"cohort_rank"`
	CohortRankChange leaderboard.RankChange `json:"cohort_rank_change"`
	CohortSize       int                    `json:"cohort_size"`

	// XP statistics
	TodayXP      student.XP `json:"today_xp"`
	WeekXP       student.XP `json:"week_xp"`
	MonthXP      student.XP `json:"month_xp"`
	XPToNextRank student.XP `json:"xp_to_next_rank"` // XP needed to overtake the next person

	// ═══════════════════════════════════════════════════════════════════════════
	// STREAK & DAILY GRIND
	// ═══════════════════════════════════════════════════════════════════════════

	CurrentStreak   int       `json:"current_streak"`
	BestStreak      int       `json:"best_streak"`
	StreakStartDate time.Time `json:"streak_start_date"`
	LastActiveDate  time.Time `json:"last_active_date"`
	DaysInactive    int       `json:"days_inactive"`
	IsStreakAtRisk  bool      `json:"is_streak_at_risk"` // No activity today yet

	// Today's Daily Grind summary
	TodayTasksCompleted int    `json:"today_tasks_completed"`
	TodaySessionMinutes int    `json:"today_session_minutes"`
	TodayRankChange     int    `json:"today_rank_change"`
	DailyGrindSummary   string `json:"daily_grind_summary"` // Human-readable summary

	// ═══════════════════════════════════════════════════════════════════════════
	// ONLINE STATUS
	// ═══════════════════════════════════════════════════════════════════════════

	OnlineState     student.OnlineState `json:"online_state"`
	IsOnline        bool                `json:"is_online"`
	LastSeenAt      time.Time           `json:"last_seen_at"`
	LastSeenDisplay string              `json:"last_seen_display"` // "5 мин назад", "online"

	// ═══════════════════════════════════════════════════════════════════════════
	// SOCIAL & HELP (Core of "от конкуренции к сотрудничеству")
	// ═══════════════════════════════════════════════════════════════════════════

	// Help statistics
	HelpRating        float64 `json:"help_rating"`         // Average rating (0-5)
	HelpCount         int     `json:"help_count"`          // Times helped others
	HelpReceivedCount int     `json:"help_received_count"` // Times received help
	HelpScore         int     `json:"help_score"`          // Computed helper score

	// Availability
	IsAvailableForHelp     bool `json:"is_available_for_help"`
	CanReceiveHelpRequests bool `json:"can_receive_help_requests"`

	// Top endorsements received
	TopEndorsements   []EndorsementSummary `json:"top_endorsements"`
	TotalEndorsements int                  `json:"total_endorsements"`

	// Social connections
	ConnectionsCount     int `json:"connections_count"`
	StudyBuddiesCount    int `json:"study_buddies_count"`
	MentoringCount       int `json:"mentoring_count"` // People mentored
	BeingMentoredByCount int `json:"being_mentored_by_count"`

	// ═══════════════════════════════════════════════════════════════════════════
	// ACHIEVEMENTS
	// ═══════════════════════════════════════════════════════════════════════════

	Achievements       []AchievementSummary `json:"achievements"`
	AchievementsCount  int                  `json:"achievements_count"`
	RecentAchievements []AchievementSummary `json:"recent_achievements"` // Last 3

	// ═══════════════════════════════════════════════════════════════════════════
	// TASKS & SKILLS
	// ═══════════════════════════════════════════════════════════════════════════

	TotalTasksCompleted int      `json:"total_tasks_completed"`
	RecentTasks         []string `json:"recent_tasks"`       // Last 5 completed task IDs
	SpecializedTopics   []string `json:"specialized_topics"` // Topics student is good at

	// ═══════════════════════════════════════════════════════════════════════════
	// PREFERENCES
	// ═══════════════════════════════════════════════════════════════════════════

	Preferences StudentPreferences `json:"preferences"`

	// ═══════════════════════════════════════════════════════════════════════════
	// METADATA
	// ═══════════════════════════════════════════════════════════════════════════

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int64     `json:"version"`
}

// StudentPreferences represents notification and display preferences.
type StudentPreferences struct {
	RankNotifications        bool `json:"rank_notifications"`
	DailyDigest              bool `json:"daily_digest"`
	HelpRequestNotifications bool `json:"help_request_notifications"`
	InactivityReminders      bool `json:"inactivity_reminders"`
	QuietHoursStart          int  `json:"quiet_hours_start"`
	QuietHoursEnd            int  `json:"quiet_hours_end"`
}

// EndorsementSummary is a compact view of an endorsement type.
type EndorsementSummary struct {
	Type  social.EndorsementType `json:"type"`
	Count int                    `json:"count"`
	Emoji string                 `json:"emoji"`
	Label string                 `json:"label"`
}

// AchievementSummary is a compact view of an achievement.
type AchievementSummary struct {
	Type       student.AchievementType `json:"type"`
	Name       string                  `json:"name"`
	Emoji      string                  `json:"emoji"`
	UnlockedAt time.Time               `json:"unlocked_at"`
	IsRecent   bool                    `json:"is_recent"` // Unlocked in last 7 days
}

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT CARD VIEW CONSTRUCTOR
// ══════════════════════════════════════════════════════════════════════════════

// NewStudentCardView creates a new empty student card view.
func NewStudentCardView() *StudentCardView {
	return &StudentCardView{
		cards:        make(map[string]*StudentCard),
		byTelegramID: make(map[int64]*StudentCard),
		byAlemLogin:  make(map[string]*StudentCard),
		lastUpdated:  time.Now().UTC(),
		version:      1,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// BUILD / REBUILD OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// BuildCardParams contains all the data needed to build a student card.
type BuildCardParams struct {
	Student          *student.Student
	Progress         *student.Progress
	DailyGrind       *student.DailyGrind
	Streak           *student.Streak
	Achievements     []student.Achievement
	SocialProfile    *social.SocialProfile
	LeaderboardEntry *leaderboard.LeaderboardEntry
	CohortSize       int
	TotalStudents    int
}

// BuildCard constructs a complete StudentCard from domain entities.
func (sv *StudentCardView) BuildCard(params BuildCardParams) (*StudentCard, error) {
	if params.Student == nil {
		return nil, fmt.Errorf("projections: student is required to build card")
	}

	s := params.Student
	now := time.Now().UTC()

	card := &StudentCard{
		// Core identity
		StudentID:   s.ID,
		TelegramID:  s.TelegramID,
		AlemLogin:   s.AlemLogin,
		DisplayName: s.DisplayName,
		Cohort:      s.Cohort,
		Status:      s.Status,
		JoinedAt:    s.JoinedAt,

		// Progress
		CurrentXP:    s.CurrentXP,
		CurrentLevel: s.Level(),

		// Online status
		OnlineState:     s.OnlineState,
		IsOnline:        s.OnlineState == student.OnlineStateOnline,
		LastSeenAt:      s.LastSeenAt,
		LastSeenDisplay: formatLastSeen(s.LastSeenAt),

		// Help
		HelpRating:         s.HelpRating,
		HelpCount:          s.HelpCount,
		HelpScore:          calculateHelpScore(s.HelpRating, s.HelpCount),
		IsAvailableForHelp: s.CanHelp(),

		// Preferences
		Preferences: StudentPreferences{
			RankNotifications:        s.Preferences.RankChanges,
			DailyDigest:              s.Preferences.DailyDigest,
			HelpRequestNotifications: s.Preferences.HelpRequests,
			InactivityReminders:      s.Preferences.InactivityReminders,
			QuietHoursStart:          s.Preferences.QuietHoursStart,
			QuietHoursEnd:            s.Preferences.QuietHoursEnd,
		},

		// Metadata
		CreatedAt: s.CreatedAt,
		UpdatedAt: now,
		Version:   1,
	}

	// Add progress data
	if params.Progress != nil {
		card.TotalTasksCompleted = params.Progress.TotalTasksCompleted
		card.LastActiveDate = params.Progress.LastActivityAt
	}

	// Add streak data
	if params.Streak != nil {
		card.CurrentStreak = params.Streak.CurrentStreak
		card.BestStreak = params.Streak.BestStreak
		card.StreakStartDate = params.Streak.StreakStartDate
		card.LastActiveDate = params.Streak.LastActiveDate
		card.IsStreakAtRisk = isStreakAtRisk(params.Streak)
	}

	// Calculate days inactive
	if !card.LastActiveDate.IsZero() {
		card.DaysInactive = int(now.Sub(card.LastActiveDate).Hours() / 24)
	}

	// Add daily grind data
	if params.DailyGrind != nil {
		card.TodayXP = student.XP(params.DailyGrind.XPGained)
		card.TodayTasksCompleted = params.DailyGrind.TasksCompleted
		card.TodaySessionMinutes = params.DailyGrind.TotalSessionMinutes
		card.TodayRankChange = params.DailyGrind.RankChange
		card.DailyGrindSummary = buildDailyGrindSummary(params.DailyGrind)
	}

	// Add achievements
	if params.Achievements != nil {
		card.AchievementsCount = len(params.Achievements)
		card.Achievements = convertAchievements(params.Achievements)
		card.RecentAchievements = filterRecentAchievements(card.Achievements, 3)
	}

	// Add social profile data
	if params.SocialProfile != nil {
		sp := params.SocialProfile
		card.ConnectionsCount = sp.TotalConnections
		card.HelpReceivedCount = sp.TotalHelpReceived
		card.TotalEndorsements = sp.TotalEndorsements
		card.CanReceiveHelpRequests = sp.IsOpenToHelp
		card.TopEndorsements = convertEndorsements(sp.TopEndorsementTypes)

		// Count specific connection types (if available)
		// These would need to come from the social profile
	}

	// Add leaderboard data
	if params.LeaderboardEntry != nil {
		le := params.LeaderboardEntry
		card.GlobalRank = le.Rank
		card.GlobalRankChange = le.RankChange
	}

	// Calculate percentile if we have total students
	if params.TotalStudents > 0 && card.GlobalRank > 0 {
		card.GlobalPercentile = 100.0 - (float64(card.GlobalRank-1) / float64(params.TotalStudents) * 100.0)
	}

	// Set cohort size
	card.CohortSize = params.CohortSize

	return card, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// UPDATE OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// UpsertCard inserts or updates a student card.
func (sv *StudentCardView) UpsertCard(card *StudentCard) error {
	if card == nil {
		return fmt.Errorf("projections: cannot upsert nil card")
	}

	sv.mu.Lock()
	defer sv.mu.Unlock()

	card.UpdatedAt = time.Now().UTC()
	card.Version++

	// Update all indexes
	sv.cards[card.StudentID] = card
	sv.byTelegramID[int64(card.TelegramID)] = card
	sv.byAlemLogin[string(card.AlemLogin)] = card

	sv.lastUpdated = time.Now().UTC()
	sv.version++

	return nil
}

// UpdateOnlineStatus updates the online status for a student.
func (sv *StudentCardView) UpdateOnlineStatus(studentID string, state student.OnlineState, lastSeen time.Time) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if card, exists := sv.cards[studentID]; exists {
		card.OnlineState = state
		card.IsOnline = state == student.OnlineStateOnline
		card.LastSeenAt = lastSeen
		card.LastSeenDisplay = formatLastSeen(lastSeen)
		card.UpdatedAt = time.Now().UTC()
	}
}

// UpdateXP updates the XP-related fields for a student.
func (sv *StudentCardView) UpdateXP(studentID string, currentXP student.XP, todayXP student.XP) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if card, exists := sv.cards[studentID]; exists {
		card.CurrentXP = currentXP
		card.CurrentLevel = student.CalculateLevel(currentXP)
		card.TodayXP = todayXP
		card.UpdatedAt = time.Now().UTC()
	}
}

// UpdateRank updates the ranking for a student.
func (sv *StudentCardView) UpdateRank(studentID string, globalRank leaderboard.Rank, rankChange leaderboard.RankChange, cohortRank leaderboard.Rank) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if card, exists := sv.cards[studentID]; exists {
		card.GlobalRank = globalRank
		card.GlobalRankChange = rankChange
		card.CohortRank = cohortRank
		card.UpdatedAt = time.Now().UTC()
	}
}

// UpdateStreak updates streak information for a student.
func (sv *StudentCardView) UpdateStreak(studentID string, currentStreak, bestStreak int, lastActiveDate time.Time) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if card, exists := sv.cards[studentID]; exists {
		card.CurrentStreak = currentStreak
		card.BestStreak = bestStreak
		card.LastActiveDate = lastActiveDate
		card.IsStreakAtRisk = isStreakAtRiskFromData(lastActiveDate)
		card.DaysInactive = int(time.Since(lastActiveDate).Hours() / 24)
		card.UpdatedAt = time.Now().UTC()
	}
}

// UpdateDailyGrind updates daily grind data for a student.
func (sv *StudentCardView) UpdateDailyGrind(studentID string, xpGained student.XP, tasksCompleted, sessionMinutes, rankChange int) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if card, exists := sv.cards[studentID]; exists {
		card.TodayXP = xpGained
		card.TodayTasksCompleted = tasksCompleted
		card.TodaySessionMinutes = sessionMinutes
		card.TodayRankChange = rankChange
		card.DailyGrindSummary = buildDailyGrindSummaryFromData(xpGained, tasksCompleted, rankChange)
		card.UpdatedAt = time.Now().UTC()
	}
}

// UpdateHelpStats updates help-related statistics for a student.
func (sv *StudentCardView) UpdateHelpStats(studentID string, rating float64, helpCount, helpReceivedCount int) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if card, exists := sv.cards[studentID]; exists {
		card.HelpRating = rating
		card.HelpCount = helpCount
		card.HelpReceivedCount = helpReceivedCount
		card.HelpScore = calculateHelpScore(rating, helpCount)
		card.UpdatedAt = time.Now().UTC()
	}
}

// AddAchievement adds a new achievement to a student's card.
func (sv *StudentCardView) AddAchievement(studentID string, achievement student.Achievement) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if card, exists := sv.cards[studentID]; exists {
		summary := convertAchievement(achievement)
		card.Achievements = append(card.Achievements, summary)
		card.AchievementsCount = len(card.Achievements)
		card.RecentAchievements = filterRecentAchievements(card.Achievements, 3)
		card.UpdatedAt = time.Now().UTC()
	}
}

// UpdateConnections updates social connection counts.
func (sv *StudentCardView) UpdateConnections(studentID string, total, studyBuddies, mentoring, beingMentored int) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if card, exists := sv.cards[studentID]; exists {
		card.ConnectionsCount = total
		card.StudyBuddiesCount = studyBuddies
		card.MentoringCount = mentoring
		card.BeingMentoredByCount = beingMentored
		card.UpdatedAt = time.Now().UTC()
	}
}

// DeleteCard removes a student card from the view.
func (sv *StudentCardView) DeleteCard(studentID string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if card, exists := sv.cards[studentID]; exists {
		delete(sv.byTelegramID, int64(card.TelegramID))
		delete(sv.byAlemLogin, string(card.AlemLogin))
		delete(sv.cards, studentID)
		sv.version++
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// QUERY OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// GetByStudentID returns a student card by student ID.
func (sv *StudentCardView) GetByStudentID(ctx context.Context, studentID string) (*StudentCard, error) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	if card, exists := sv.cards[studentID]; exists {
		return card.clone(), nil
	}

	return nil, fmt.Errorf("projections: student card not found for ID %s", studentID)
}

// GetByTelegramID returns a student card by Telegram ID.
func (sv *StudentCardView) GetByTelegramID(ctx context.Context, telegramID int64) (*StudentCard, error) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	if card, exists := sv.byTelegramID[telegramID]; exists {
		return card.clone(), nil
	}

	return nil, fmt.Errorf("projections: student card not found for Telegram ID %d", telegramID)
}

// GetByAlemLogin returns a student card by Alem login.
func (sv *StudentCardView) GetByAlemLogin(ctx context.Context, login string) (*StudentCard, error) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	if card, exists := sv.byAlemLogin[login]; exists {
		return card.clone(), nil
	}

	return nil, fmt.Errorf("projections: student card not found for login %s", login)
}

// GetAll returns all student cards with pagination.
func (sv *StudentCardView) GetAll(ctx context.Context, offset, limit int) ([]*StudentCard, error) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	// Convert map to slice
	all := make([]*StudentCard, 0, len(sv.cards))
	for _, card := range sv.cards {
		all = append(all, card)
	}

	// Sort by XP descending
	sort.Slice(all, func(i, j int) bool {
		return all[i].CurrentXP > all[j].CurrentXP
	})

	// Apply pagination
	if offset >= len(all) {
		return make([]*StudentCard, 0), nil
	}

	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	result := make([]*StudentCard, end-offset)
	for i := offset; i < end; i++ {
		result[i-offset] = all[i].clone()
	}

	return result, nil
}

// GetByCohort returns all student cards for a specific cohort.
func (sv *StudentCardView) GetByCohort(ctx context.Context, cohort student.Cohort) ([]*StudentCard, error) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	result := make([]*StudentCard, 0)
	for _, card := range sv.cards {
		if card.Cohort == cohort {
			result = append(result, card.clone())
		}
	}

	// Sort by cohort rank
	sort.Slice(result, func(i, j int) bool {
		return result[i].CohortRank < result[j].CohortRank
	})

	return result, nil
}

// GetTopHelpers returns students with highest help scores.
func (sv *StudentCardView) GetTopHelpers(ctx context.Context, limit int) ([]*StudentCard, error) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	helpers := make([]*StudentCard, 0)
	for _, card := range sv.cards {
		if card.HelpCount > 0 {
			helpers = append(helpers, card.clone())
		}
	}

	// Sort by help score descending
	sort.Slice(helpers, func(i, j int) bool {
		return helpers[i].HelpScore > helpers[j].HelpScore
	})

	if limit > 0 && limit < len(helpers) {
		helpers = helpers[:limit]
	}

	return helpers, nil
}

// GetInactiveStudents returns students who haven't been active for specified days.
func (sv *StudentCardView) GetInactiveStudents(ctx context.Context, daysInactive int) ([]*StudentCard, error) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	result := make([]*StudentCard, 0)
	for _, card := range sv.cards {
		if card.DaysInactive >= daysInactive {
			result = append(result, card.clone())
		}
	}

	// Sort by days inactive descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].DaysInactive > result[j].DaysInactive
	})

	return result, nil
}

// GetStreaksAtRisk returns students whose streaks are at risk (no activity today).
func (sv *StudentCardView) GetStreaksAtRisk(ctx context.Context) ([]*StudentCard, error) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	result := make([]*StudentCard, 0)
	for _, card := range sv.cards {
		if card.IsStreakAtRisk && card.CurrentStreak > 0 {
			result = append(result, card.clone())
		}
	}

	// Sort by streak length descending (longer streaks = higher priority)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CurrentStreak > result[j].CurrentStreak
	})

	return result, nil
}

// Count returns the total number of student cards.
func (sv *StudentCardView) Count() int {
	sv.mu.RLock()
	defer sv.mu.RUnlock()
	return len(sv.cards)
}

// Exists checks if a student card exists.
func (sv *StudentCardView) Exists(studentID string) bool {
	sv.mu.RLock()
	defer sv.mu.RUnlock()
	_, exists := sv.cards[studentID]
	return exists
}

// GetVersion returns the current version.
func (sv *StudentCardView) GetVersion() int64 {
	sv.mu.RLock()
	defer sv.mu.RUnlock()
	return sv.version
}

// GetLastUpdated returns when the view was last updated.
func (sv *StudentCardView) GetLastUpdated() time.Time {
	sv.mu.RLock()
	defer sv.mu.RUnlock()
	return sv.lastUpdated
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// clone creates a deep copy of a StudentCard.
func (c *StudentCard) clone() *StudentCard {
	if c == nil {
		return nil
	}

	cardCopy := *c

	// Deep copy slices
	if c.TopEndorsements != nil {
		cardCopy.TopEndorsements = make([]EndorsementSummary, len(c.TopEndorsements))
		copy(cardCopy.TopEndorsements, c.TopEndorsements)
	}

	if c.Achievements != nil {
		cardCopy.Achievements = make([]AchievementSummary, len(c.Achievements))
		copy(cardCopy.Achievements, c.Achievements)
	}

	if c.RecentAchievements != nil {
		cardCopy.RecentAchievements = make([]AchievementSummary, len(c.RecentAchievements))
		copy(cardCopy.RecentAchievements, c.RecentAchievements)
	}

	if c.RecentTasks != nil {
		cardCopy.RecentTasks = make([]string, len(c.RecentTasks))
		copy(cardCopy.RecentTasks, c.RecentTasks)
	}

	if c.SpecializedTopics != nil {
		cardCopy.SpecializedTopics = make([]string, len(c.SpecializedTopics))
		copy(cardCopy.SpecializedTopics, c.SpecializedTopics)
	}

	return &cardCopy
}

// formatLastSeen formats the last seen time to human-readable string.
func formatLastSeen(t time.Time) string {
	if t.IsZero() {
		return "никогда"
	}

	d := time.Since(t)

	if d < time.Minute {
		return "online"
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
	if days == 1 {
		return "вчера"
	}
	return fmt.Sprintf("%d дн назад", days)
}

// isStreakAtRisk checks if a streak is at risk (no activity today).
func isStreakAtRisk(streak *student.Streak) bool {
	if streak == nil || streak.CurrentStreak == 0 {
		return false
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	lastActive := streak.LastActiveDate.Truncate(24 * time.Hour)

	return !lastActive.Equal(today)
}

// isStreakAtRiskFromData checks if streak is at risk from last active date.
func isStreakAtRiskFromData(lastActiveDate time.Time) bool {
	if lastActiveDate.IsZero() {
		return false
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	lastActive := lastActiveDate.Truncate(24 * time.Hour)

	return !lastActive.Equal(today)
}

// buildDailyGrindSummary creates a human-readable summary of daily grind.
func buildDailyGrindSummary(dg *student.DailyGrind) string {
	if dg == nil || !dg.IsActive() {
		return "Сегодня пока без активности"
	}

	return buildDailyGrindSummaryFromData(
		student.XP(dg.XPGained),
		dg.TasksCompleted,
		dg.RankChange,
	)
}

// buildDailyGrindSummaryFromData creates summary from raw data.
func buildDailyGrindSummaryFromData(xpGained student.XP, tasksCompleted, rankChange int) string {
	if xpGained == 0 && tasksCompleted == 0 {
		return "Сегодня пока без активности"
	}

	rankEmoji := ""
	if rankChange > 0 {
		rankEmoji = " 📈"
	} else if rankChange < 0 {
		rankEmoji = " 📉"
	}

	return fmt.Sprintf("+%d XP, %d задач%s", xpGained, tasksCompleted, rankEmoji)
}

// convertAchievements converts domain achievements to summaries.
func convertAchievements(achievements []student.Achievement) []AchievementSummary {
	result := make([]AchievementSummary, len(achievements))
	for i, a := range achievements {
		result[i] = convertAchievement(a)
	}

	// Sort by unlock time descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].UnlockedAt.After(result[j].UnlockedAt)
	})

	return result
}

// convertAchievement converts a single achievement to summary.
func convertAchievement(a student.Achievement) AchievementSummary {
	def, found := student.GetAchievementDefinition(a.Type)

	summary := AchievementSummary{
		Type:       a.Type,
		UnlockedAt: a.UnlockedAt,
		IsRecent:   time.Since(a.UnlockedAt) < 7*24*time.Hour,
	}

	if found {
		summary.Name = def.Name
		summary.Emoji = def.Emoji
	} else {
		summary.Name = string(a.Type)
		summary.Emoji = "🏆"
	}

	return summary
}

// filterRecentAchievements returns the N most recent achievements.
func filterRecentAchievements(achievements []AchievementSummary, n int) []AchievementSummary {
	if len(achievements) <= n {
		result := make([]AchievementSummary, len(achievements))
		copy(result, achievements)
		return result
	}

	result := make([]AchievementSummary, n)
	copy(result, achievements[:n])
	return result
}

// convertEndorsements converts endorsement stats to summaries.
func convertEndorsements(stats []social.EndorsementTypeStat) []EndorsementSummary {
	result := make([]EndorsementSummary, len(stats))
	for i, s := range stats {
		result[i] = EndorsementSummary{
			Type:  s.Type,
			Count: s.Count,
			Emoji: s.Type.Emoji(),
			Label: s.Type.Label(),
		}
	}
	return result
}

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT CARD VIEW REPOSITORY INTERFACE
// ══════════════════════════════════════════════════════════════════════════════

// StudentCardViewRepository defines the interface for student card view storage.
type StudentCardViewRepository interface {
	// Save persists the entire view (for checkpointing).
	Save(ctx context.Context, view *StudentCardView) error

	// Load loads the view from persistent storage.
	Load(ctx context.Context) (*StudentCardView, error)

	// GetCard retrieves a single card.
	GetCard(ctx context.Context, studentID string) (*StudentCard, error)

	// SaveCard persists a single card.
	SaveCard(ctx context.Context, card *StudentCard) error

	// DeleteCard removes a card from storage.
	DeleteCard(ctx context.Context, studentID string) error
}
