// Package jobs contains implementations of scheduled jobs for Alem Community Hub.
package jobs

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/leaderboard"
	"github.com/alem-hub/alem-community-hub/internal/domain/shared"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ══════════════════════════════════════════════════════════════════════════════
// REBUILD LEADERBOARD JOB
// ══════════════════════════════════════════════════════════════════════════════

// RebuildLeaderboardJob rebuilds the leaderboard and detects rank changes.
// This job is essential for the "From Competition to Collaboration" philosophy:
// - Accurate rankings help students see their real progress
// - Rank change notifications create engagement and motivation
// - The leaderboard becomes a "phone book" for finding help
type RebuildLeaderboardJob struct {
	// Dependencies
	studentRepo      student.Repository
	leaderboardRepo  leaderboard.LeaderboardRepository
	leaderboardCache leaderboard.LeaderboardCache
	onlineTracker    student.OnlineTracker
	eventPublisher   shared.EventPublisher
	notifier         leaderboard.RankChangeNotifier
	logger           *slog.Logger

	// Configuration
	config RebuildLeaderboardConfig

	// State
	lastRebuildStats atomic.Value // *RebuildStats
}

// RebuildLeaderboardConfig contains configuration for the rebuild job.
type RebuildLeaderboardConfig struct {
	// NotifyRankChanges enables notifications for rank changes.
	NotifyRankChanges bool

	// MinRankChangeForNotification is the minimum rank change to trigger a notification.
	MinRankChangeForNotification int

	// NotifyTopNEntry enables notifications when entering top N.
	NotifyTopNEntry bool

	// TopNThresholds are the top-N levels to notify about (e.g., 10, 50, 100).
	TopNThresholds []int

	// SnapshotRetentionDays is how long to keep old snapshots.
	SnapshotRetentionDays int

	// RebuildCohorts specifies which cohorts to rebuild (empty = all + general).
	RebuildCohorts []string

	// CacheTTL is the TTL for cached leaderboard data.
	CacheTTL time.Duration

	// Timeout is the maximum duration for the rebuild operation.
	Timeout time.Duration
}

// DefaultRebuildLeaderboardConfig returns sensible defaults.
func DefaultRebuildLeaderboardConfig() RebuildLeaderboardConfig {
	return RebuildLeaderboardConfig{
		NotifyRankChanges:            true,
		MinRankChangeForNotification: 3,
		NotifyTopNEntry:              true,
		TopNThresholds:               []int{10, 50, 100},
		SnapshotRetentionDays:        7,
		RebuildCohorts:               nil, // nil = all
		CacheTTL:                     10 * time.Minute,
		Timeout:                      5 * time.Minute,
	}
}

// RebuildStats contains statistics from a rebuild run.
type RebuildStats struct {
	StartedAt         time.Time
	CompletedAt       time.Time
	Duration          time.Duration
	TotalStudents     int
	CohortsProcessed  int
	SnapshotsCreated  int
	RankChangesFound  int
	NotificationsSent int
	TopNEntries       int
	TopNExits         int
	Errors            []error
}

// NewRebuildLeaderboardJob creates a new rebuild leaderboard job.
func NewRebuildLeaderboardJob(
	studentRepo student.Repository,
	leaderboardRepo leaderboard.LeaderboardRepository,
	leaderboardCache leaderboard.LeaderboardCache,
	onlineTracker student.OnlineTracker,
	eventPublisher shared.EventPublisher,
	notifier leaderboard.RankChangeNotifier,
	logger *slog.Logger,
	config RebuildLeaderboardConfig,
) *RebuildLeaderboardJob {
	if logger == nil {
		logger = slog.Default()
	}

	return &RebuildLeaderboardJob{
		studentRepo:      studentRepo,
		leaderboardRepo:  leaderboardRepo,
		leaderboardCache: leaderboardCache,
		onlineTracker:    onlineTracker,
		eventPublisher:   eventPublisher,
		notifier:         notifier,
		logger:           logger,
		config:           config,
	}
}

// Name returns the job name.
func (j *RebuildLeaderboardJob) Name() string {
	return "rebuild_leaderboard"
}

// Description returns a human-readable description.
func (j *RebuildLeaderboardJob) Description() string {
	return "Rebuilds leaderboard rankings and detects rank changes for notifications"
}

// Run executes the rebuild job.
func (j *RebuildLeaderboardJob) Run(ctx context.Context) error {
	startedAt := time.Now()
	stats := &RebuildStats{
		StartedAt: startedAt,
		Errors:    make([]error, 0),
	}

	j.logger.Info("starting rebuild_leaderboard job")

	// Apply timeout
	if j.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, j.config.Timeout)
		defer cancel()
	}

	// Get all active students
	students, err := j.getAllActiveStudents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get students: %w", err)
	}

	stats.TotalStudents = len(students)
	j.logger.Info("found students for leaderboard", "count", stats.TotalStudents)

	if stats.TotalStudents == 0 {
		stats.CompletedAt = time.Now()
		stats.Duration = stats.CompletedAt.Sub(startedAt)
		j.lastRebuildStats.Store(stats)
		return nil
	}

	// Get online states for all students
	studentIDs := make([]string, len(students))
	for i, s := range students {
		studentIDs[i] = s.ID
	}
	onlineStates, _ := j.onlineTracker.GetOnlineStates(ctx, studentIDs)

	// Rebuild general leaderboard (all cohorts)
	if err := j.rebuildLeaderboard(ctx, leaderboard.CohortAll, students, onlineStates, stats); err != nil {
		stats.Errors = append(stats.Errors, err)
		j.logger.Error("failed to rebuild general leaderboard", "error", err)
	}

	// Rebuild per-cohort leaderboards
	cohorts := j.getCohorts(students)
	for _, cohort := range cohorts {
		cohortStudents := j.filterByCohort(students, cohort)
		if len(cohortStudents) == 0 {
			continue
		}

		if err := j.rebuildLeaderboard(ctx, leaderboard.Cohort(cohort), cohortStudents, onlineStates, stats); err != nil {
			stats.Errors = append(stats.Errors, err)
			j.logger.Error("failed to rebuild cohort leaderboard",
				"cohort", cohort,
				"error", err,
			)
		}
		stats.CohortsProcessed++
	}

	// Cleanup old snapshots
	if j.config.SnapshotRetentionDays > 0 {
		threshold := time.Now().AddDate(0, 0, -j.config.SnapshotRetentionDays)
		deleted, err := j.leaderboardRepo.DeleteOldSnapshots(ctx, threshold)
		if err != nil {
			j.logger.Warn("failed to delete old snapshots", "error", err)
		} else if deleted > 0 {
			j.logger.Info("deleted old snapshots", "count", deleted)
		}
	}

	// Finalize stats
	stats.CompletedAt = time.Now()
	stats.Duration = stats.CompletedAt.Sub(startedAt)
	j.lastRebuildStats.Store(stats)

	j.logger.Info("rebuild_leaderboard job completed",
		"duration", stats.Duration.String(),
		"total_students", stats.TotalStudents,
		"snapshots_created", stats.SnapshotsCreated,
		"rank_changes", stats.RankChangesFound,
		"notifications", stats.NotificationsSent,
	)

	if len(stats.Errors) > 0 {
		return fmt.Errorf("rebuild completed with %d errors", len(stats.Errors))
	}

	return nil
}

// rebuildLeaderboard rebuilds the leaderboard for a specific cohort.
func (j *RebuildLeaderboardJob) rebuildLeaderboard(
	ctx context.Context,
	cohort leaderboard.Cohort,
	students []*student.Student,
	onlineStates map[string]student.OnlineState,
	stats *RebuildStats,
) error {
	// Get previous snapshot for comparison
	prevSnapshot, _ := j.leaderboardRepo.GetLatestSnapshot(ctx, cohort)

	// Build new ranking
	ranking := leaderboard.NewRanking()
	for _, s := range students {
		entry, err := leaderboard.NewLeaderboardEntry(
			leaderboard.Rank(1), // Will be recalculated
			s.ID,
			s.DisplayName,
			leaderboard.XP(s.CurrentXP),
			int(s.Level()),
			leaderboard.Cohort(s.Cohort),
		)
		if err != nil {
			continue
		}

		// Set online state
		if state, ok := onlineStates[s.ID]; ok {
			entry.IsOnline = state == student.OnlineStateOnline
			entry.IsAvailableForHelp = state.IsAvailable() && s.CanHelp()
		}
		entry.HelpRating = s.HelpRating
		entry.UpdatedAt = s.UpdatedAt

		if err := ranking.Add(entry); err != nil {
			j.logger.Warn("failed to add entry to ranking",
				"student_id", s.ID,
				"error", err,
			)
		}
	}

	// Sort and assign ranks
	ranking.SortByXP()

	// Create new snapshot
	snapshotID := uuid.New().String()
	newSnapshot := leaderboard.NewLeaderboardSnapshot(snapshotID, cohort, ranking)

	// Calculate diff and update rank changes
	if prevSnapshot != nil {
		diff := leaderboard.CalculateDiff(prevSnapshot, newSnapshot)

		// Process rank changes
		for studentID, change := range diff.RankChanges {
			stats.RankChangesFound++

			// Update entry with rank change
			entry := newSnapshot.GetByID(studentID)
			if entry != nil {
				entry.RankChange = change
			}

			// Send notifications for significant changes
			if j.config.NotifyRankChanges && change.IsSignificant(j.config.MinRankChangeForNotification) {
				j.notifyRankChange(ctx, studentID, change, entry, stats)
			}
		}

		// Process top N changes
		for _, topChange := range diff.TopChanges {
			if topChange.IsEntered() {
				stats.TopNEntries++
				if j.config.NotifyTopNEntry {
					j.notifyTopNEntry(ctx, topChange, stats)
				}
			}
			if topChange.IsLeft() {
				stats.TopNExits++
			}
		}
	}

	// Save snapshot
	if err := j.leaderboardRepo.SaveSnapshot(ctx, newSnapshot); err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}
	stats.SnapshotsCreated++

	// Update cache
	if j.leaderboardCache != nil {
		// Cache top entries
		topEntries := newSnapshot.Top(100)
		if err := j.leaderboardCache.SetCachedTop(ctx, cohort, topEntries, j.config.CacheTTL); err != nil {
			j.logger.Warn("failed to cache top entries", "error", err)
		}

		// Cache individual ranks
		for _, entry := range topEntries {
			if err := j.leaderboardCache.SetCachedRank(ctx, entry, j.config.CacheTTL); err != nil {
				j.logger.Warn("failed to cache rank",
					"student_id", entry.StudentID,
					"error", err,
				)
			}
		}
	}

	j.logger.Debug("leaderboard rebuilt",
		"cohort", cohort.String(),
		"students", newSnapshot.TotalStudents,
		"average_xp", newSnapshot.AverageXP,
	)

	return nil
}

// notifyRankChange sends notifications for rank changes.
func (j *RebuildLeaderboardJob) notifyRankChange(
	ctx context.Context,
	studentID string,
	change leaderboard.RankChange,
	entry *leaderboard.LeaderboardEntry,
	stats *RebuildStats,
) {
	if j.notifier == nil || entry == nil {
		return
	}

	var err error
	oldRank := leaderboard.Rank(int(entry.Rank) - int(change))
	newRank := entry.Rank

	cohort := string(entry.Cohort)
	if change > 0 {
		// Student moved up
		err = j.notifier.NotifyRankUp(ctx, studentID, oldRank, newRank, change)

		// Emit event
		event := shared.NewRankChangedEvent(studentID, int(oldRank), int(newRank), cohort)
		_ = j.eventPublisher.Publish(event)
	} else {
		// Student moved down
		err = j.notifier.NotifyRankDown(ctx, studentID, oldRank, newRank, change)

		// Emit event
		event := shared.NewRankChangedEvent(studentID, int(oldRank), int(newRank), cohort)
		_ = j.eventPublisher.Publish(event)
	}

	if err != nil {
		j.logger.Warn("failed to send rank change notification",
			"student_id", studentID,
			"error", err,
		)
	} else {
		stats.NotificationsSent++
	}
}

// notifyTopNEntry sends notifications for entering top N.
func (j *RebuildLeaderboardJob) notifyTopNEntry(
	ctx context.Context,
	change leaderboard.TopChange,
	stats *RebuildStats,
) {
	if j.notifier == nil {
		return
	}

	err := j.notifier.NotifyEnteredTop(ctx, change.StudentID, change.EnteredTop, change.NewRank)
	if err != nil {
		j.logger.Warn("failed to send top N entry notification",
			"student_id", change.StudentID,
			"top_n", change.EnteredTop,
			"error", err,
		)
	} else {
		stats.NotificationsSent++

		// Emit event (use "all" as default cohort for top N events)
		event := shared.NewEnteredTopNEvent(change.StudentID, change.EnteredTop, int(change.NewRank), "all")
		_ = j.eventPublisher.Publish(event)
	}
}

// getAllActiveStudents retrieves all students for the leaderboard.
func (j *RebuildLeaderboardJob) getAllActiveStudents(ctx context.Context) ([]*student.Student, error) {
	opts := student.DefaultListOptions().WithLimit(10000) // High limit for all students
	return j.studentRepo.GetByStatus(ctx, student.StatusActive, opts)
}

// getCohorts extracts unique cohorts from students.
func (j *RebuildLeaderboardJob) getCohorts(students []*student.Student) []string {
	cohortSet := make(map[string]struct{})
	for _, s := range students {
		if s.Cohort != "" {
			cohortSet[string(s.Cohort)] = struct{}{}
		}
	}

	cohorts := make([]string, 0, len(cohortSet))
	for cohort := range cohortSet {
		cohorts = append(cohorts, cohort)
	}
	return cohorts
}

// filterByCohort filters students by cohort.
func (j *RebuildLeaderboardJob) filterByCohort(students []*student.Student, cohort string) []*student.Student {
	filtered := make([]*student.Student, 0)
	for _, s := range students {
		if string(s.Cohort) == cohort {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// LastRebuildStats returns statistics from the last rebuild.
func (j *RebuildLeaderboardJob) LastRebuildStats() *RebuildStats {
	stats := j.lastRebuildStats.Load()
	if stats == nil {
		return nil
	}
	return stats.(*RebuildStats)
}
