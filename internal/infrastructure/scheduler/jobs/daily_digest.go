// Package jobs contains implementations of scheduled jobs for Alem Community Hub.
package jobs

import (
	"alem-hub/internal/domain/leaderboard"
	"alem-hub/internal/domain/notification"
	"alem-hub/internal/domain/shared"
	"alem-hub/internal/domain/student"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// DAILY DIGEST JOB
// ══════════════════════════════════════════════════════════════════════════════

// DailyDigestJob sends personalized daily summaries to students.
//
// Philosophy: "From Competition to Collaboration"
// The daily digest is not just a stats report — it's a motivational tool that:
// - Celebrates progress (even small wins)
// - Shows community activity (you're not alone!)
// - Highlights opportunities to help others
// - Provides gentle nudges based on goals
//
// Each digest is personalized to make students feel seen and valued.
type DailyDigestJob struct {
	// Dependencies
	studentRepo     student.Repository
	progressRepo    student.ProgressRepository
	leaderboardRepo leaderboard.LeaderboardRepository
	socialRepo      SocialRepository
	notificationSvc NotificationService
	eventPublisher  shared.EventPublisher
	logger          *slog.Logger

	// Configuration
	config DailyDigestConfig

	// State
	lastRunStats atomic.Value // *DailyDigestStats
}

// SocialRepository interface for getting social data.
type SocialRepository interface {
	GetEndorsementsReceived(ctx context.Context, studentID string, since time.Time) (int, error)
	GetHelpProvidedCount(ctx context.Context, studentID string, since time.Time) (int, error)
}

// NotificationService interface for sending notifications.
type NotificationService interface {
	Send(ctx context.Context, notification *notification.Notification) notification.DeliveryResult
}

// DailyDigestConfig contains configuration for the daily digest job.
type DailyDigestConfig struct {
	// SendTime is the hour (0-23) in the timezone to send digests.
	SendTime int

	// Timezone for calculating send time.
	Timezone *time.Location

	// EnableDigest enables sending daily digests.
	EnableDigest bool

	// IncludeLeaderboard includes leaderboard position in digest.
	IncludeLeaderboard bool

	// IncludeSocialStats includes social interaction stats.
	IncludeSocialStats bool

	// IncludeMotivationalQuote includes a motivational quote.
	IncludeMotivationalQuote bool

	// IncludeStreakInfo includes streak information.
	IncludeStreakInfo bool

	// Concurrency is the number of digests to send in parallel.
	Concurrency int

	// Timeout is the maximum duration for the job.
	Timeout time.Duration

	// SkipInactiveAfterDays skips sending to students inactive for N days.
	SkipInactiveAfterDays int
}

// DefaultDailyDigestConfig returns sensible defaults.
func DefaultDailyDigestConfig() DailyDigestConfig {
	// Default to Almaty timezone (UTC+5)
	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		loc = time.FixedZone("UTC+5", 5*60*60)
	}

	return DailyDigestConfig{
		SendTime:                 21, // 9 PM Almaty time
		Timezone:                 loc,
		EnableDigest:             true,
		IncludeLeaderboard:       true,
		IncludeSocialStats:       true,
		IncludeMotivationalQuote: true,
		IncludeStreakInfo:        true,
		Concurrency:              10,
		Timeout:                  15 * time.Minute,
		SkipInactiveAfterDays:    14,
	}
}

// DailyDigestStats contains statistics from a digest run.
type DailyDigestStats struct {
	StartedAt      time.Time
	CompletedAt    time.Time
	Duration       time.Duration
	TotalStudents  int
	DigestsSent    int
	DigestsSkipped int
	DigestsFailed  int
	SkippedReasons map[string]int
	Errors         []error
}

// DigestContent contains the personalized content for a student's digest.
type DigestContent struct {
	// Basic info
	StudentName string
	Date        string

	// Progress
	TodayXP        int
	TotalXP        int
	TasksCompleted int
	Level          int

	// Streak
	CurrentStreak int
	BestStreak    int
	StreakStatus  string // "maintained", "broken", "new_record"

	// Leaderboard
	CurrentRank   int
	RankChange    int
	RankDirection string // "up", "down", "same"
	NeighborAbove string // Name of student above
	NeighborBelow string // Name of student below
	XPToNextRank  int

	// Social
	HelpProvided         int
	EndorsementsReceived int
	NewConnections       int

	// Community
	StudentsOnlineNow int
	CommunityXPToday  int
	TopMover          string // Who gained the most XP today

	// Motivation
	MotivationalQuote string
	PersonalizedTip   string
}

// NewDailyDigestJob creates a new daily digest job.
func NewDailyDigestJob(
	studentRepo student.Repository,
	progressRepo student.ProgressRepository,
	leaderboardRepo leaderboard.LeaderboardRepository,
	socialRepo SocialRepository,
	notificationSvc NotificationService,
	eventPublisher shared.EventPublisher,
	logger *slog.Logger,
	config DailyDigestConfig,
) *DailyDigestJob {
	if logger == nil {
		logger = slog.Default()
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 10
	}

	return &DailyDigestJob{
		studentRepo:     studentRepo,
		progressRepo:    progressRepo,
		leaderboardRepo: leaderboardRepo,
		socialRepo:      socialRepo,
		notificationSvc: notificationSvc,
		eventPublisher:  eventPublisher,
		logger:          logger,
		config:          config,
	}
}

// Name returns the job name.
func (j *DailyDigestJob) Name() string {
	return "daily_digest"
}

// Description returns a human-readable description.
func (j *DailyDigestJob) Description() string {
	return "Sends personalized daily progress summaries to students"
}

// Run executes the daily digest job.
func (j *DailyDigestJob) Run(ctx context.Context) error {
	startedAt := time.Now()
	stats := &DailyDigestStats{
		StartedAt:      startedAt,
		SkippedReasons: make(map[string]int),
		Errors:         make([]error, 0),
	}

	j.logger.Info("starting daily_digest job")

	if !j.config.EnableDigest {
		j.logger.Info("daily digest is disabled")
		return nil
	}

	// Apply timeout
	if j.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, j.config.Timeout)
		defer cancel()
	}

	// Get students who should receive digest
	students, err := j.getEligibleStudents(ctx, stats)
	if err != nil {
		return fmt.Errorf("failed to get eligible students: %w", err)
	}

	stats.TotalStudents = len(students)
	j.logger.Info("found eligible students for digest", "count", stats.TotalStudents)

	if stats.TotalStudents == 0 {
		stats.CompletedAt = time.Now()
		stats.Duration = stats.CompletedAt.Sub(startedAt)
		j.lastRunStats.Store(stats)
		return nil
	}

	// Get community-wide stats
	communityStats := j.getCommunityStats(ctx)

	// Send digests concurrently
	j.sendDigestsConcurrently(ctx, students, communityStats, stats)

	// Finalize stats
	stats.CompletedAt = time.Now()
	stats.Duration = stats.CompletedAt.Sub(startedAt)
	j.lastRunStats.Store(stats)

	j.logger.Info("daily_digest job completed",
		"duration", stats.Duration.String(),
		"total", stats.TotalStudents,
		"sent", stats.DigestsSent,
		"skipped", stats.DigestsSkipped,
		"failed", stats.DigestsFailed,
	)

	return nil
}

// getEligibleStudents returns students who should receive the digest.
func (j *DailyDigestJob) getEligibleStudents(ctx context.Context, stats *DailyDigestStats) ([]*student.Student, error) {
	// Get all active students with digest enabled
	opts := student.DefaultListOptions()
	allStudents, err := j.studentRepo.GetByStatus(ctx, student.StatusActive, opts)
	if err != nil {
		return nil, err
	}

	eligible := make([]*student.Student, 0, len(allStudents))
	now := time.Now()

	for _, s := range allStudents {
		// Check if student wants daily digest
		if !s.Preferences.DailyDigest {
			stats.SkippedReasons["digest_disabled"]++
			continue
		}

		// Check if in quiet hours
		if s.Preferences.IsQuietHour(now.In(j.config.Timezone)) {
			stats.SkippedReasons["quiet_hours"]++
			continue
		}

		// Skip long-inactive students
		if j.config.SkipInactiveAfterDays > 0 {
			daysSinceActive := int(now.Sub(s.LastSeenAt).Hours() / 24)
			if daysSinceActive > j.config.SkipInactiveAfterDays {
				stats.SkippedReasons["too_inactive"]++
				continue
			}
		}

		eligible = append(eligible, s)
	}

	return eligible, nil
}

// CommunityStats holds community-wide statistics.
type CommunityStats struct {
	TotalActiveStudents int
	OnlineNow           int
	TotalXPToday        int
	TopMoverName        string
	TopMoverXP          int
}

// getCommunityStats gathers community-wide statistics.
func (j *DailyDigestJob) getCommunityStats(ctx context.Context) *CommunityStats {
	stats := &CommunityStats{}

	// Get total active students
	count, err := j.studentRepo.Count(ctx)
	if err == nil {
		stats.TotalActiveStudents = count
	}

	// Get online count
	onlineStudents, err := j.studentRepo.FindOnline(ctx)
	if err == nil {
		stats.OnlineNow = len(onlineStudents)
	}

	// TODO: Calculate total XP gained today and top mover
	// This would require aggregating daily progress data

	return stats
}

// sendDigestsConcurrently sends digests using a worker pool.
func (j *DailyDigestJob) sendDigestsConcurrently(
	ctx context.Context,
	students []*student.Student,
	communityStats *CommunityStats,
	stats *DailyDigestStats,
) {
	var (
		wg        sync.WaitGroup
		semaphore = make(chan struct{}, j.config.Concurrency)
		mu        sync.Mutex
	)

	for _, s := range students {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wg.Add(1)
		semaphore <- struct{}{} // Acquire

		go func(st *student.Student) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release

			// Build and send digest
			err := j.sendDigestToStudent(ctx, st, communityStats)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				stats.DigestsFailed++
				stats.Errors = append(stats.Errors, err)
				j.logger.Error("failed to send digest",
					"student_id", st.ID,
					"error", err,
				)
			} else {
				stats.DigestsSent++
			}
		}(s)
	}

	wg.Wait()
}

// sendDigestToStudent builds and sends a digest to a single student.
func (j *DailyDigestJob) sendDigestToStudent(
	ctx context.Context,
	s *student.Student,
	communityStats *CommunityStats,
) error {
	// Build digest content
	content := j.buildDigestContent(ctx, s, communityStats)

	// Format the message
	message := j.formatDigestMessage(content)

	// Create notification
	n := &notification.Notification{
		RecipientID:    notification.RecipientID(s.ID),
		TelegramChatID: int64(s.TelegramID),
		Type:           notification.TypeDailyDigest,
		Priority:       notification.PriorityLow,
		Status:         notification.StatusPending,
		Message:        message,
		CreatedAt:      time.Now(),
	}

	// Send notification
	result := j.notificationSvc.Send(ctx, n)
	if !result.Success {
		return result.Error
	}

	return nil
}

// buildDigestContent builds personalized content for a student's digest.
func (j *DailyDigestJob) buildDigestContent(
	ctx context.Context,
	s *student.Student,
	communityStats *CommunityStats,
) *DigestContent {
	content := &DigestContent{
		StudentName: s.DisplayName,
		Date:        time.Now().In(j.config.Timezone).Format("02.01.2006"),
		TotalXP:     int(s.CurrentXP),
		Level:       int(s.Level()),
	}

	// Get today's progress
	today := time.Now().In(j.config.Timezone).Truncate(24 * time.Hour)
	dailyGrind, err := j.progressRepo.GetDailyGrind(ctx, s.ID, today)
	if err == nil && dailyGrind != nil {
		content.TodayXP = dailyGrind.XPGained
		content.TasksCompleted = dailyGrind.TasksCompleted
	}

	// Get streak info
	if j.config.IncludeStreakInfo {
		streak, err := j.progressRepo.GetStreak(ctx, s.ID)
		if err == nil && streak != nil {
			content.CurrentStreak = streak.CurrentStreak
			content.BestStreak = streak.BestStreak

			if streak.CurrentStreak > streak.BestStreak {
				content.StreakStatus = "new_record"
			} else if streak.CurrentStreak > 0 {
				content.StreakStatus = "maintained"
			} else {
				content.StreakStatus = "broken"
			}
		}
	}

	// Get leaderboard info
	if j.config.IncludeLeaderboard {
		entry, err := j.leaderboardRepo.GetStudentRank(ctx, s.ID, leaderboard.CohortAll)
		if err == nil && entry != nil {
			content.CurrentRank = int(entry.Rank)
			content.RankChange = int(entry.RankChange)

			if entry.RankChange > 0 {
				content.RankDirection = "up"
			} else if entry.RankChange < 0 {
				content.RankDirection = "down"
			} else {
				content.RankDirection = "same"
			}

			// Get neighbors
			neighbors := j.getNeighbors(ctx, s.ID, entry.Rank)
			content.NeighborAbove = neighbors.above
			content.NeighborBelow = neighbors.below
			content.XPToNextRank = neighbors.xpToNext
		}
	}

	// Get social stats
	if j.config.IncludeSocialStats && j.socialRepo != nil {
		yesterday := time.Now().AddDate(0, 0, -1)

		helpCount, _ := j.socialRepo.GetHelpProvidedCount(ctx, s.ID, yesterday)
		content.HelpProvided = helpCount

		endorsements, _ := j.socialRepo.GetEndorsementsReceived(ctx, s.ID, yesterday)
		content.EndorsementsReceived = endorsements
	}

	// Add community stats
	content.StudentsOnlineNow = communityStats.OnlineNow
	content.CommunityXPToday = communityStats.TotalXPToday
	content.TopMover = communityStats.TopMoverName

	// Add motivational content
	if j.config.IncludeMotivationalQuote {
		content.MotivationalQuote = j.getMotivationalQuote()
		content.PersonalizedTip = j.getPersonalizedTip(content)
	}

	return content
}

type neighborInfo struct {
	above    string
	below    string
	xpToNext int
}

func (j *DailyDigestJob) getNeighbors(ctx context.Context, studentID string, rank leaderboard.Rank) neighborInfo {
	info := neighborInfo{}

	neighbors, err := j.leaderboardRepo.GetNeighbors(ctx, studentID, leaderboard.CohortAll, 1)
	if err != nil || len(neighbors) == 0 {
		return info
	}

	for _, n := range neighbors {
		if n.StudentID == studentID {
			continue
		}
		if n.Rank < rank {
			info.above = n.DisplayName
			info.xpToNext = int(n.XP) - int(neighbors[0].XP) + 1
		} else if n.Rank > rank {
			info.below = n.DisplayName
		}
	}

	return info
}

// formatDigestMessage formats the digest content into a Telegram message.
func (j *DailyDigestJob) formatDigestMessage(content *DigestContent) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("📊 *Твой день, %s*\n", content.StudentName))
	sb.WriteString(fmt.Sprintf("_%s_\n\n", content.Date))

	// Progress section
	sb.WriteString("*📈 Прогресс*\n")
	if content.TodayXP > 0 {
		sb.WriteString(fmt.Sprintf("• Сегодня: +%d XP\n", content.TodayXP))
	} else {
		sb.WriteString("• Сегодня: пока без XP\n")
	}
	sb.WriteString(fmt.Sprintf("• Всего: %d XP (уровень %d)\n", content.TotalXP, content.Level))

	if content.TasksCompleted > 0 {
		sb.WriteString(fmt.Sprintf("• Задач решено: %d\n", content.TasksCompleted))
	}
	sb.WriteString("\n")

	// Streak section
	if content.CurrentStreak > 0 || content.BestStreak > 0 {
		sb.WriteString("*🔥 Серия*\n")
		switch content.StreakStatus {
		case "new_record":
			sb.WriteString(fmt.Sprintf("• Новый рекорд! %d дней подряд! 🎉\n", content.CurrentStreak))
		case "maintained":
			sb.WriteString(fmt.Sprintf("• %d дней подряд (рекорд: %d)\n", content.CurrentStreak, content.BestStreak))
		case "broken":
			sb.WriteString(fmt.Sprintf("• Серия прервалась :(\n"))
			sb.WriteString(fmt.Sprintf("• Твой рекорд: %d дней — побьём его?\n", content.BestStreak))
		}
		sb.WriteString("\n")
	}

	// Leaderboard section
	if content.CurrentRank > 0 {
		sb.WriteString("*🏆 Рейтинг*\n")

		rankEmoji := "➖"
		if content.RankDirection == "up" {
			rankEmoji = fmt.Sprintf("🔼+%d", content.RankChange)
		} else if content.RankDirection == "down" {
			rankEmoji = fmt.Sprintf("🔽%d", content.RankChange)
		}

		sb.WriteString(fmt.Sprintf("• Место: #%d %s\n", content.CurrentRank, rankEmoji))

		if content.NeighborAbove != "" && content.XPToNextRank > 0 {
			sb.WriteString(fmt.Sprintf("• До %s: %d XP\n", content.NeighborAbove, content.XPToNextRank))
		}
		sb.WriteString("\n")
	}

	// Social section
	if content.HelpProvided > 0 || content.EndorsementsReceived > 0 {
		sb.WriteString("*🤝 Сообщество*\n")
		if content.HelpProvided > 0 {
			sb.WriteString(fmt.Sprintf("• Помог %d студентам 👏\n", content.HelpProvided))
		}
		if content.EndorsementsReceived > 0 {
			sb.WriteString(fmt.Sprintf("• Получено благодарностей: %d ⭐\n", content.EndorsementsReceived))
		}
		sb.WriteString("\n")
	}

	// Community section
	if content.StudentsOnlineNow > 0 {
		sb.WriteString("*👥 Прямо сейчас*\n")
		sb.WriteString(fmt.Sprintf("• Студентов онлайн: %d\n", content.StudentsOnlineNow))
		if content.TopMover != "" {
			sb.WriteString(fmt.Sprintf("• Топ-мувер дня: %s 🚀\n", content.TopMover))
		}
		sb.WriteString("\n")
	}

	// Motivational section
	if content.MotivationalQuote != "" {
		sb.WriteString("─────────────────\n")
		sb.WriteString(fmt.Sprintf("_\"%s\"_\n", content.MotivationalQuote))
		sb.WriteString("\n")
	}

	// Personalized tip
	if content.PersonalizedTip != "" {
		sb.WriteString(fmt.Sprintf("💡 %s\n", content.PersonalizedTip))
	}

	// Footer
	sb.WriteString("\n_Удачного дня! 🍀_")

	return sb.String()
}

// getMotivationalQuote returns a random motivational quote.
func (j *DailyDigestJob) getMotivationalQuote() string {
	quotes := []string{
		"Код — это поэзия, которую понимают машины",
		"Каждый эксперт когда-то был новичком",
		"Лучший код — это тот, который не нужно писать",
		"Ошибки — это ступени к мастерству",
		"Вместе мы можем больше, чем поодиночке",
		"Сегодняшний баг — завтрашний опыт",
		"Маленькие шаги приводят к большим результатам",
		"Помогая другим, мы растём сами",
		"Учиться никогда не поздно",
		"Успех — это сумма маленьких усилий",
	}

	// Use day of year as seed for consistent daily quote
	dayOfYear := time.Now().YearDay()
	return quotes[dayOfYear%len(quotes)]
}

// getPersonalizedTip returns a personalized tip based on the student's data.
func (j *DailyDigestJob) getPersonalizedTip(content *DigestContent) string {
	// Prioritize tips based on student's situation
	if content.StreakStatus == "broken" {
		return "Самое время начать новую серию! Даже одна задача сегодня — уже победа."
	}

	if content.TodayXP == 0 {
		return "Ещё не поздно добавить XP сегодня. Начни с чего-то простого!"
	}

	if content.CurrentStreak >= 7 {
		return "Целая неделя подряд! Ты на правильном пути. Не останавливайся!"
	}

	if content.RankDirection == "down" && content.RankChange < -5 {
		return "Не расстраивайся из-за рейтинга — завтра новый день для рывка!"
	}

	if content.HelpProvided > 0 {
		return "Спасибо, что помогаешь другим. Это делает сообщество сильнее!"
	}

	// Default tips
	tips := []string{
		"Попробуй помочь кому-нибудь сегодня — это лучший способ закрепить знания.",
		"Не забывай делать перерывы. Отдохнувший мозг работает лучше!",
		"Застрял? Загляни в /help — кто-то точно может подсказать.",
	}

	dayOfYear := time.Now().YearDay()
	return tips[dayOfYear%len(tips)]
}

// LastRunStats returns statistics from the last digest run.
func (j *DailyDigestJob) LastRunStats() *DailyDigestStats {
	stats := j.lastRunStats.Load()
	if stats == nil {
		return nil
	}
	return stats.(*DailyDigestStats)
}
