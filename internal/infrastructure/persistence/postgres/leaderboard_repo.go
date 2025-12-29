// Package postgres implements PostgreSQL persistence layer for Alem Community Hub.
package postgres

import (
	"alem-hub/internal/domain/leaderboard"
	"alem-hub/internal/domain/student"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD REPOSITORY IMPLEMENTATION
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardRepository implements leaderboard.LeaderboardRepository for PostgreSQL.
type LeaderboardRepository struct {
	conn *Connection
}

// NewLeaderboardRepository creates a new LeaderboardRepository.
func NewLeaderboardRepository(conn *Connection) *LeaderboardRepository {
	return &LeaderboardRepository{conn: conn}
}

// ─────────────────────────────────────────────────────────────────────────────
// SNAPSHOT OPERATIONS
// ─────────────────────────────────────────────────────────────────────────────

// SaveSnapshot saves a leaderboard snapshot.
func (r *LeaderboardRepository) SaveSnapshot(ctx context.Context, snapshot *leaderboard.LeaderboardSnapshot) error {
	// Start transaction
	return r.conn.WithTx(ctx, DefaultTxOptions(), func(tx pgx.Tx) error {
		// Insert snapshot metadata
		_, err := tx.Exec(ctx, `
			INSERT INTO leaderboard_snapshots (id, cohort, snapshot_at, total_students, total_xp, average_xp)
			VALUES ($1, $2, $3, $4, $5, $6)
		`,
			snapshot.ID,
			string(snapshot.Cohort),
			snapshot.SnapshotAt,
			snapshot.TotalStudents,
			snapshot.TotalXP,
			int(snapshot.AverageXP),
		)
		if err != nil {
			return fmt.Errorf("failed to insert snapshot: %w", err)
		}

		// Batch insert entries
		if len(snapshot.Entries) > 0 {
			batch := &pgx.Batch{}
			for _, entry := range snapshot.Entries {
				batch.Queue(`
					INSERT INTO leaderboard_entries 
					(snapshot_id, student_id, rank, xp, level, rank_change, is_online, is_available_for_help)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				`,
					snapshot.ID,
					entry.StudentID,
					int(entry.Rank),
					int(entry.XP),
					entry.Level,
					int(entry.RankChange),
					entry.IsOnline,
					entry.IsAvailableForHelp,
				)
			}

			br := tx.SendBatch(ctx, batch)
			defer br.Close()

			for range snapshot.Entries {
				if _, err := br.Exec(); err != nil {
					return fmt.Errorf("failed to insert entry: %w", err)
				}
			}
		}

		return nil
	})
}

// GetLatestSnapshot returns the latest snapshot for a cohort.
func (r *LeaderboardRepository) GetLatestSnapshot(ctx context.Context, cohort leaderboard.Cohort) (*leaderboard.LeaderboardSnapshot, error) {
	// Get snapshot metadata
	var snapshot leaderboard.LeaderboardSnapshot
	var cohortStr string

	err := r.conn.QueryRow(ctx, `
		SELECT id, cohort, snapshot_at, total_students, total_xp, average_xp, created_at
		FROM leaderboard_snapshots
		WHERE cohort = $1
		ORDER BY snapshot_at DESC
		LIMIT 1
	`, string(cohort)).Scan(
		&snapshot.ID,
		&cohortStr,
		&snapshot.SnapshotAt,
		&snapshot.TotalStudents,
		&snapshot.TotalXP,
		&snapshot.AverageXP,
		&snapshot.SnapshotAt, // created_at
	)

	if IsNoRows(err) {
		return nil, leaderboard.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest snapshot: %w", err)
	}

	snapshot.Cohort = leaderboard.Cohort(cohortStr)

	// Get entries
	entries, err := r.getSnapshotEntries(ctx, snapshot.ID)
	if err != nil {
		return nil, err
	}
	snapshot.Entries = entries
	snapshot.RebuildIndex()

	return &snapshot, nil
}

// GetSnapshotByID returns a snapshot by ID.
func (r *LeaderboardRepository) GetSnapshotByID(ctx context.Context, id string) (*leaderboard.LeaderboardSnapshot, error) {
	var snapshot leaderboard.LeaderboardSnapshot
	var cohortStr string

	err := r.conn.QueryRow(ctx, `
		SELECT id, cohort, snapshot_at, total_students, total_xp, average_xp
		FROM leaderboard_snapshots
		WHERE id = $1
	`, id).Scan(
		&snapshot.ID,
		&cohortStr,
		&snapshot.SnapshotAt,
		&snapshot.TotalStudents,
		&snapshot.TotalXP,
		&snapshot.AverageXP,
	)

	if IsNoRows(err) {
		return nil, leaderboard.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot by id: %w", err)
	}

	snapshot.Cohort = leaderboard.Cohort(cohortStr)

	entries, err := r.getSnapshotEntries(ctx, snapshot.ID)
	if err != nil {
		return nil, err
	}
	snapshot.Entries = entries
	snapshot.RebuildIndex()

	return &snapshot, nil
}

// GetSnapshotAt returns a snapshot at a specific time.
func (r *LeaderboardRepository) GetSnapshotAt(ctx context.Context, cohort leaderboard.Cohort, at time.Time) (*leaderboard.LeaderboardSnapshot, error) {
	var snapshot leaderboard.LeaderboardSnapshot
	var cohortStr string

	err := r.conn.QueryRow(ctx, `
		SELECT id, cohort, snapshot_at, total_students, total_xp, average_xp
		FROM leaderboard_snapshots
		WHERE cohort = $1 AND snapshot_at <= $2
		ORDER BY snapshot_at DESC
		LIMIT 1
	`, string(cohort), at).Scan(
		&snapshot.ID,
		&cohortStr,
		&snapshot.SnapshotAt,
		&snapshot.TotalStudents,
		&snapshot.TotalXP,
		&snapshot.AverageXP,
	)

	if IsNoRows(err) {
		return nil, leaderboard.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot at time: %w", err)
	}

	snapshot.Cohort = leaderboard.Cohort(cohortStr)

	entries, err := r.getSnapshotEntries(ctx, snapshot.ID)
	if err != nil {
		return nil, err
	}
	snapshot.Entries = entries
	snapshot.RebuildIndex()

	return &snapshot, nil
}

// GetPreviousSnapshot returns the snapshot before a given snapshot.
func (r *LeaderboardRepository) GetPreviousSnapshot(ctx context.Context, snapshotID string) (*leaderboard.LeaderboardSnapshot, error) {
	// First, get the current snapshot to find its timestamp and cohort
	var snapshotAt time.Time
	var cohortStr string

	err := r.conn.QueryRow(ctx, `
		SELECT snapshot_at, cohort FROM leaderboard_snapshots WHERE id = $1
	`, snapshotID).Scan(&snapshotAt, &cohortStr)

	if IsNoRows(err) {
		return nil, leaderboard.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot time: %w", err)
	}

	// Get previous snapshot
	var snapshot leaderboard.LeaderboardSnapshot
	err = r.conn.QueryRow(ctx, `
		SELECT id, cohort, snapshot_at, total_students, total_xp, average_xp
		FROM leaderboard_snapshots
		WHERE cohort = $1 AND snapshot_at < $2
		ORDER BY snapshot_at DESC
		LIMIT 1
	`, cohortStr, snapshotAt).Scan(
		&snapshot.ID,
		&cohortStr,
		&snapshot.SnapshotAt,
		&snapshot.TotalStudents,
		&snapshot.TotalXP,
		&snapshot.AverageXP,
	)

	if IsNoRows(err) {
		return nil, leaderboard.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get previous snapshot: %w", err)
	}

	snapshot.Cohort = leaderboard.Cohort(cohortStr)

	entries, err := r.getSnapshotEntries(ctx, snapshot.ID)
	if err != nil {
		return nil, err
	}
	snapshot.Entries = entries
	snapshot.RebuildIndex()

	return &snapshot, nil
}

// ListSnapshots returns snapshot metadata for a time period.
func (r *LeaderboardRepository) ListSnapshots(ctx context.Context, cohort leaderboard.Cohort, from, to time.Time) ([]leaderboard.SnapshotMeta, error) {
	rows, err := r.conn.Query(ctx, `
		SELECT id, cohort, snapshot_at, total_students, total_xp, average_xp
		FROM leaderboard_snapshots
		WHERE cohort = $1 AND snapshot_at >= $2 AND snapshot_at <= $3
		ORDER BY snapshot_at DESC
	`, string(cohort), from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}
	defer rows.Close()

	var metas []leaderboard.SnapshotMeta
	for rows.Next() {
		var meta leaderboard.SnapshotMeta
		var cohortStr string
		var avgXP int

		err := rows.Scan(
			&meta.ID,
			&cohortStr,
			&meta.SnapshotAt,
			&meta.TotalStudents,
			&meta.TotalXP,
			&avgXP,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan snapshot meta: %w", err)
		}

		meta.Cohort = leaderboard.Cohort(cohortStr)
		meta.AverageXP = leaderboard.XP(avgXP)
		metas = append(metas, meta)
	}

	return metas, rows.Err()
}

// DeleteOldSnapshots deletes snapshots older than a specific time.
func (r *LeaderboardRepository) DeleteOldSnapshots(ctx context.Context, olderThan time.Time) (int, error) {
	result, err := r.conn.Exec(ctx, `
		DELETE FROM leaderboard_snapshots WHERE snapshot_at < $1
	`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old snapshots: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// RANKING QUERIES
// ─────────────────────────────────────────────────────────────────────────────

// GetStudentRank returns the current rank for a student.
func (r *LeaderboardRepository) GetStudentRank(ctx context.Context, studentID string, cohort leaderboard.Cohort) (*leaderboard.LeaderboardEntry, error) {
	// Get from latest snapshot
	query := `
		SELECT le.rank, le.xp, le.level, le.rank_change, le.is_online, le.is_available_for_help,
			   s.id, s.alem_login, s.display_name, s.cohort, s.help_rating
		FROM leaderboard_entries le
		JOIN leaderboard_snapshots ls ON le.snapshot_id = ls.id
		JOIN students s ON le.student_id = s.id
		WHERE le.student_id = $1 AND ls.cohort = $2
		ORDER BY ls.snapshot_at DESC
		LIMIT 1
	`

	var entry leaderboard.LeaderboardEntry
	var rank, xp, level, rankChange int
	var cohortStr string

	err := r.conn.QueryRow(ctx, query, studentID, string(cohort)).Scan(
		&rank,
		&xp,
		&level,
		&rankChange,
		&entry.IsOnline,
		&entry.IsAvailableForHelp,
		&entry.StudentID,
		&entry.AlemLogin,
		&entry.DisplayName,
		&cohortStr,
		&entry.HelpRating,
	)

	if IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get student rank: %w", err)
	}

	entry.Rank = leaderboard.Rank(rank)
	entry.XP = leaderboard.XP(xp)
	entry.Level = level
	entry.RankChange = leaderboard.RankChange(rankChange)
	entry.Cohort = leaderboard.Cohort(cohortStr)

	return &entry, nil
}

// GetTop returns the top N students.
func (r *LeaderboardRepository) GetTop(ctx context.Context, cohort leaderboard.Cohort, limit int) ([]*leaderboard.LeaderboardEntry, error) {
	// Get from latest snapshot
	query := `
		SELECT le.rank, le.xp, le.level, le.rank_change, le.is_online, le.is_available_for_help,
			   s.id, s.alem_login, s.display_name, s.cohort, s.help_rating
		FROM leaderboard_entries le
		JOIN leaderboard_snapshots ls ON le.snapshot_id = ls.id
		JOIN students s ON le.student_id = s.id
		WHERE ls.id = (
			SELECT id FROM leaderboard_snapshots WHERE cohort = $1 ORDER BY snapshot_at DESC LIMIT 1
		)
		ORDER BY le.rank ASC
		LIMIT $2
	`

	rows, err := r.conn.Query(ctx, query, string(cohort), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top: %w", err)
	}
	defer rows.Close()

	return r.scanLeaderboardEntries(rows)
}

// GetPage returns a page of the leaderboard.
func (r *LeaderboardRepository) GetPage(ctx context.Context, cohort leaderboard.Cohort, page, pageSize int) ([]*leaderboard.LeaderboardEntry, error) {
	offset := (page - 1) * pageSize

	query := `
		SELECT le.rank, le.xp, le.level, le.rank_change, le.is_online, le.is_available_for_help,
			   s.id, s.alem_login, s.display_name, s.cohort, s.help_rating
		FROM leaderboard_entries le
		JOIN leaderboard_snapshots ls ON le.snapshot_id = ls.id
		JOIN students s ON le.student_id = s.id
		WHERE ls.id = (
			SELECT id FROM leaderboard_snapshots WHERE cohort = $1 ORDER BY snapshot_at DESC LIMIT 1
		)
		ORDER BY le.rank ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.conn.Query(ctx, query, string(cohort), pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get page: %w", err)
	}
	defer rows.Close()

	return r.scanLeaderboardEntries(rows)
}

// GetNeighbors returns neighbors around a student (±rangeSize).
func (r *LeaderboardRepository) GetNeighbors(ctx context.Context, studentID string, cohort leaderboard.Cohort, rangeSize int) ([]*leaderboard.LeaderboardEntry, error) {
	// First get the student's rank
	entry, err := r.GetStudentRank(ctx, studentID, cohort)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	// Calculate offset (position - rangeSize - 1 because ranks are 1-indexed)
	offset := int(entry.Rank) - rangeSize - 1
	if offset < 0 {
		offset = 0
	}
	limit := rangeSize*2 + 1

	query := `
		SELECT le.rank, le.xp, le.level, le.rank_change, le.is_online, le.is_available_for_help,
			   s.id, s.alem_login, s.display_name, s.cohort, s.help_rating
		FROM leaderboard_entries le
		JOIN leaderboard_snapshots ls ON le.snapshot_id = ls.id
		JOIN students s ON le.student_id = s.id
		WHERE ls.id = (
			SELECT id FROM leaderboard_snapshots WHERE cohort = $1 ORDER BY snapshot_at DESC LIMIT 1
		)
		ORDER BY le.rank ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.conn.Query(ctx, query, string(cohort), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get neighbors: %w", err)
	}
	defer rows.Close()

	return r.scanLeaderboardEntries(rows)
}

// GetTotalCount returns the total number of students in the leaderboard.
func (r *LeaderboardRepository) GetTotalCount(ctx context.Context, cohort leaderboard.Cohort) (int, error) {
	var count int
	err := r.conn.QueryRow(ctx, `
		SELECT total_students 
		FROM leaderboard_snapshots 
		WHERE cohort = $1 
		ORDER BY snapshot_at DESC 
		LIMIT 1
	`, string(cohort)).Scan(&count)

	if IsNoRows(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get total count: %w", err)
	}

	return count, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// RANK HISTORY
// ─────────────────────────────────────────────────────────────────────────────

// GetRankHistory returns rank history for a student.
func (r *LeaderboardRepository) GetRankHistory(ctx context.Context, studentID string, from, to time.Time) ([]leaderboard.RankHistoryEntry, error) {
	query := `
		SELECT rh.rank, rh.xp, rh.snapshot_at, rh.rank_change
		FROM rank_history rh
		WHERE rh.student_id = $1 AND rh.snapshot_at >= $2 AND rh.snapshot_at <= $3
		ORDER BY rh.snapshot_at ASC
	`

	rows, err := r.conn.Query(ctx, query, studentID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to get rank history: %w", err)
	}
	defer rows.Close()

	var entries []leaderboard.RankHistoryEntry
	for rows.Next() {
		var entry leaderboard.RankHistoryEntry
		var rank, xp, rankChange int

		err := rows.Scan(&rank, &xp, &entry.SnapshotAt, &rankChange)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rank history entry: %w", err)
		}

		entry.StudentID = studentID
		entry.Rank = leaderboard.Rank(rank)
		entry.XP = leaderboard.XP(xp)
		entry.RankChange = leaderboard.RankChange(rankChange)
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// GetBestRank returns the best rank achieved by a student.
func (r *LeaderboardRepository) GetBestRank(ctx context.Context, studentID string) (*leaderboard.RankHistoryEntry, error) {
	query := `
		SELECT rank, xp, snapshot_at, rank_change
		FROM rank_history
		WHERE student_id = $1
		ORDER BY rank ASC
		LIMIT 1
	`

	var entry leaderboard.RankHistoryEntry
	var rank, xp, rankChange int

	err := r.conn.QueryRow(ctx, query, studentID).Scan(&rank, &xp, &entry.SnapshotAt, &rankChange)
	if IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get best rank: %w", err)
	}

	entry.StudentID = studentID
	entry.Rank = leaderboard.Rank(rank)
	entry.XP = leaderboard.XP(xp)
	entry.RankChange = leaderboard.RankChange(rankChange)

	return &entry, nil
}

// SaveRankHistory saves rank history entry.
func (r *LeaderboardRepository) SaveRankHistory(ctx context.Context, entry leaderboard.RankHistoryEntry, snapshotID string) error {
	query := `
		INSERT INTO rank_history (student_id, rank, xp, snapshot_id, snapshot_at, rank_change)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := r.conn.Exec(ctx, query,
		entry.StudentID,
		int(entry.Rank),
		int(entry.XP),
		snapshotID,
		entry.SnapshotAt,
		int(entry.RankChange),
	)
	if err != nil {
		return fmt.Errorf("failed to save rank history: %w", err)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// COHORT OPERATIONS
// ─────────────────────────────────────────────────────────────────────────────

// ListCohorts returns all cohorts with active students.
func (r *LeaderboardRepository) ListCohorts(ctx context.Context) ([]leaderboard.Cohort, error) {
	query := `
		SELECT DISTINCT cohort FROM students WHERE status = 'active' ORDER BY cohort
	`

	rows, err := r.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list cohorts: %w", err)
	}
	defer rows.Close()

	var cohorts []leaderboard.Cohort
	for rows.Next() {
		var cohort string
		if err := rows.Scan(&cohort); err != nil {
			return nil, fmt.Errorf("failed to scan cohort: %w", err)
		}
		cohorts = append(cohorts, leaderboard.Cohort(cohort))
	}

	return cohorts, rows.Err()
}

// GetCohortStats returns statistics for a cohort.
func (r *LeaderboardRepository) GetCohortStats(ctx context.Context, cohort leaderboard.Cohort) (*leaderboard.CohortStats, error) {
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'active') as active,
			COALESCE(SUM(current_xp), 0) as total_xp,
			COALESCE(AVG(current_xp), 0) as avg_xp,
			COALESCE(MAX(current_xp), 0) as max_xp,
			COUNT(*) FILTER (WHERE online_state = 'online') as online
		FROM students
		WHERE cohort = $1
	`

	var stats leaderboard.CohortStats
	var avgXP, maxXP float64

	err := r.conn.QueryRow(ctx, query, string(cohort)).Scan(
		&stats.TotalStudents,
		&stats.ActiveStudents,
		&stats.TotalXP,
		&avgXP,
		&maxXP,
		&stats.OnlineCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get cohort stats: %w", err)
	}

	stats.Cohort = cohort
	stats.AverageXP = leaderboard.XP(avgXP)
	stats.TopStudentXP = leaderboard.XP(maxXP)
	stats.LastUpdated = time.Now().UTC()

	// Get median XP
	medianQuery := `
		SELECT COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY current_xp), 0)
		FROM students
		WHERE cohort = $1 AND status = 'active'
	`
	var medianXP float64
	_ = r.conn.QueryRow(ctx, medianQuery, string(cohort)).Scan(&medianXP)
	stats.MedianXP = leaderboard.XP(medianXP)

	return &stats, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LIVE RANKING (Real-time from students table)
// ─────────────────────────────────────────────────────────────────────────────

// BuildLiveRanking builds a ranking directly from the students table.
// This is used when snapshots are not available or for real-time queries.
func (r *LeaderboardRepository) BuildLiveRanking(ctx context.Context, cohort leaderboard.Cohort) (*leaderboard.Ranking, error) {
	query := `
		SELECT s.id, s.alem_login, s.display_name, s.current_xp, s.cohort,
			   s.online_state, s.help_rating,
			   (s.online_state = 'online' OR s.online_state = 'away') AND 
			   (s.preferences->>'help_requests')::boolean AS available_for_help
		FROM students s
		WHERE s.status = 'active'
	`

	args := []interface{}{}
	if cohort != leaderboard.CohortAll {
		query += " AND s.cohort = $1"
		args = append(args, string(cohort))
	}

	query += " ORDER BY s.current_xp DESC"

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to build live ranking: %w", err)
	}
	defer rows.Close()

	ranking := leaderboard.NewRanking()

	for rows.Next() {
		var entry leaderboard.LeaderboardEntry
		var xp int
		var cohortStr, onlineState string
		var availableForHelp bool

		err := rows.Scan(
			&entry.StudentID,
			&entry.AlemLogin,
			&entry.DisplayName,
			&xp,
			&cohortStr,
			&onlineState,
			&entry.HelpRating,
			&availableForHelp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan student for ranking: %w", err)
		}

		entry.XP = leaderboard.XP(xp)
		entry.Level = xp / 1000 // Simple level calculation
		entry.Cohort = leaderboard.Cohort(cohortStr)
		entry.IsOnline = onlineState == "online"
		entry.IsAvailableForHelp = availableForHelp

		_ = ranking.Add(&entry)
	}

	ranking.SortByXP()

	return ranking, rows.Err()
}

// CreateSnapshotFromLive creates a snapshot from live data.
func (r *LeaderboardRepository) CreateSnapshotFromLive(ctx context.Context, cohort leaderboard.Cohort) (*leaderboard.LeaderboardSnapshot, error) {
	ranking, err := r.BuildLiveRanking(ctx, cohort)
	if err != nil {
		return nil, err
	}

	snapshotID := uuid.New().String()
	snapshot := leaderboard.NewLeaderboardSnapshot(snapshotID, cohort, ranking)

	if err := r.SaveSnapshot(ctx, snapshot); err != nil {
		return nil, err
	}

	return snapshot, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ══════════════════════════════════════════════════════════════════════════════

// getSnapshotEntries retrieves all entries for a snapshot.
func (r *LeaderboardRepository) getSnapshotEntries(ctx context.Context, snapshotID string) ([]*leaderboard.LeaderboardEntry, error) {
	query := `
		SELECT le.rank, le.xp, le.level, le.rank_change, le.is_online, le.is_available_for_help,
			   s.id, s.alem_login, s.display_name, s.cohort, s.help_rating
		FROM leaderboard_entries le
		JOIN students s ON le.student_id = s.id
		WHERE le.snapshot_id = $1
		ORDER BY le.rank ASC
	`

	rows, err := r.conn.Query(ctx, query, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot entries: %w", err)
	}
	defer rows.Close()

	return r.scanLeaderboardEntries(rows)
}

// scanLeaderboardEntries scans leaderboard entries from rows.
func (r *LeaderboardRepository) scanLeaderboardEntries(rows pgx.Rows) ([]*leaderboard.LeaderboardEntry, error) {
	var entries []*leaderboard.LeaderboardEntry

	for rows.Next() {
		var entry leaderboard.LeaderboardEntry
		var rank, xp, level, rankChange int
		var cohortStr string

		err := rows.Scan(
			&rank,
			&xp,
			&level,
			&rankChange,
			&entry.IsOnline,
			&entry.IsAvailableForHelp,
			&entry.StudentID,
			&entry.AlemLogin,
			&entry.DisplayName,
			&cohortStr,
			&entry.HelpRating,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan leaderboard entry: %w", err)
		}

		entry.Rank = leaderboard.Rank(rank)
		entry.XP = leaderboard.XP(xp)
		entry.Level = level
		entry.RankChange = leaderboard.RankChange(rankChange)
		entry.Cohort = leaderboard.Cohort(cohortStr)

		entries = append(entries, &entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return entries, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// SOCIAL QUERIES (Who can help?)
// Philosophy: "From Competition to Collaboration"
// ══════════════════════════════════════════════════════════════════════════════

// FindHelpersForTask finds students who completed a specific task and can help.
func (r *LeaderboardRepository) FindHelpersForTask(ctx context.Context, taskID string, limit int) ([]*leaderboard.LeaderboardEntry, error) {
	query := `
		SELECT DISTINCT ON (s.id)
			0 as rank, -- Rank not relevant here
			s.current_xp as xp,
			s.current_xp / 1000 as level,
			0 as rank_change,
			s.online_state = 'online' as is_online,
			(s.online_state IN ('online', 'away')) AND 
			(s.preferences->>'help_requests')::boolean as is_available_for_help,
			s.id, s.alem_login, s.display_name, s.cohort, s.help_rating
		FROM students s
		JOIN task_completions tc ON s.id = tc.student_id
		WHERE tc.task_id = $1 
			AND s.status = 'active'
			AND (s.preferences->>'help_requests')::boolean = true
		ORDER BY s.id, 
			CASE WHEN s.online_state = 'online' THEN 0 ELSE 1 END,
			s.help_rating DESC,
			tc.completed_at DESC
		LIMIT $2
	`

	rows, err := r.conn.Query(ctx, query, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find helpers for task: %w", err)
	}
	defer rows.Close()

	return r.scanLeaderboardEntries(rows)
}

// FindOnlineHelpers finds online students who are available to help.
func (r *LeaderboardRepository) FindOnlineHelpers(ctx context.Context, cohort leaderboard.Cohort, limit int) ([]*leaderboard.LeaderboardEntry, error) {
	query := `
		SELECT 
			0 as rank,
			s.current_xp as xp,
			s.current_xp / 1000 as level,
			0 as rank_change,
			true as is_online,
			true as is_available_for_help,
			s.id, s.alem_login, s.display_name, s.cohort, s.help_rating
		FROM students s
		WHERE s.status = 'active'
			AND s.online_state IN ('online', 'away')
			AND (s.preferences->>'help_requests')::boolean = true
	`

	args := []interface{}{}
	argIndex := 1

	if cohort != leaderboard.CohortAll {
		query += fmt.Sprintf(" AND s.cohort = $%d", argIndex)
		args = append(args, string(cohort))
		argIndex++
	}

	query += fmt.Sprintf(" ORDER BY s.help_rating DESC, s.current_xp DESC LIMIT $%d", argIndex)
	args = append(args, limit)

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to find online helpers: %w", err)
	}
	defer rows.Close()

	return r.scanLeaderboardEntries(rows)
}

// ══════════════════════════════════════════════════════════════════════════════
// TASK COMPLETIONS
// ══════════════════════════════════════════════════════════════════════════════

// SaveTaskCompletion saves a task completion record.
func (r *LeaderboardRepository) SaveTaskCompletion(ctx context.Context, studentID, taskID, taskName string, xpEarned int) error {
	query := `
		INSERT INTO task_completions (student_id, task_id, task_name, xp_earned, completed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT(student_id, task_id) DO UPDATE SET
			xp_earned = EXCLUDED.xp_earned,
			completed_at = EXCLUDED.completed_at
	`

	_, err := r.conn.Exec(ctx, query, studentID, taskID, taskName, xpEarned, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to save task completion: %w", err)
	}

	return nil
}

// GetStudentsWhoCompletedTask returns students who completed a specific task.
func (r *LeaderboardRepository) GetStudentsWhoCompletedTask(ctx context.Context, taskID string) ([]string, error) {
	query := `
		SELECT student_id FROM task_completions WHERE task_id = $1 ORDER BY completed_at DESC
	`

	rows, err := r.conn.Query(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get students who completed task: %w", err)
	}
	defer rows.Close()

	var studentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan student id: %w", err)
		}
		studentIDs = append(studentIDs, id)
	}

	return studentIDs, rows.Err()
}

// Ensure interfaces are implemented
var (
	_ student.Repository         = (*StudentRepository)(nil)
	_ student.ProgressRepository = (*ProgressRepository)(nil)
)
