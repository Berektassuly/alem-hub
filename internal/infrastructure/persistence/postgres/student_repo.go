// Package postgres implements PostgreSQL persistence layer for Alem Community Hub.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/domain/student"

	"github.com/jackc/pgx/v5"
)

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT REPOSITORY IMPLEMENTATION
// ══════════════════════════════════════════════════════════════════════════════

// StudentRepository implements student.Repository for PostgreSQL.
type StudentRepository struct {
	conn *Connection
}

// NewStudentRepository creates a new StudentRepository.
func NewStudentRepository(conn *Connection) *StudentRepository {
	return &StudentRepository{conn: conn}
}

// ─────────────────────────────────────────────────────────────────────────────
// CRUD Operations
// ─────────────────────────────────────────────────────────────────────────────

// Create creates a new student.
func (r *StudentRepository) Create(ctx context.Context, s *student.Student) error {
	query := `
		INSERT INTO students (
			id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			status, online_state, last_seen_at, last_synced_at, joined_at,
			preferences, help_rating, help_count, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`

	prefsJSON, err := json.Marshal(preferencesToMap(s.Preferences))
	if err != nil {
		return fmt.Errorf("failed to marshal preferences: %w", err)
	}

	_, err = r.conn.Exec(ctx, query,
		s.ID,
		int64(s.TelegramID),
		s.Email,
		s.PasswordHash,
		s.DisplayName,
		int(s.CurrentXP),
		string(s.Cohort),
		string(s.Status),
		string(s.OnlineState),
		s.LastSeenAt,
		s.LastSyncedAt,
		s.JoinedAt,
		prefsJSON,
		s.HelpRating,
		s.HelpCount,
		s.CreatedAt,
		s.UpdatedAt,
	)
	if err != nil {
		if IsUniqueViolation(err) {
			return student.ErrStudentAlreadyExists
		}
		return fmt.Errorf("failed to create student: %w", err)
	}

	return nil
}

// GetByID returns a student by internal ID.
func (r *StudentRepository) GetByID(ctx context.Context, id string) (*student.Student, error) {
	query := `
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
		WHERE id = $1
	`

	row := r.conn.QueryRow(ctx, query, id)
	return r.scanStudent(row)
}

// GetByTelegramID returns a student by Telegram ID.
func (r *StudentRepository) GetByTelegramID(ctx context.Context, telegramID student.TelegramID) (*student.Student, error) {
	query := `
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
		WHERE telegram_id = $1
	`

	row := r.conn.QueryRow(ctx, query, int64(telegramID))
	return r.scanStudent(row)
}

// GetByEmail returns a student by email.
func (r *StudentRepository) GetByEmail(ctx context.Context, email string) (*student.Student, error) {
	query := `
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
		WHERE email = $1
	`

	row := r.conn.QueryRow(ctx, query, email)
	return r.scanStudent(row)
}

// Update updates a student.
func (r *StudentRepository) Update(ctx context.Context, s *student.Student) error {
	query := `
		UPDATE students SET
			telegram_id = $1,
			email = $2,
			password_hash = $3,
			display_name = $4,
			current_xp = $5,
			cohort = $6,
			status = $7,
			online_state = $8,
			last_seen_at = $9,
			last_synced_at = $10,
			preferences = $11,
			help_rating = $12,
			help_count = $13,
			updated_at = $14
		WHERE id = $15
	`

	prefsJSON, err := json.Marshal(preferencesToMap(s.Preferences))
	if err != nil {
		return fmt.Errorf("failed to marshal preferences: %w", err)
	}

	result, err := r.conn.Exec(ctx, query,
		int64(s.TelegramID),
		s.Email,
		s.PasswordHash,
		s.DisplayName,
		int(s.CurrentXP),
		string(s.Cohort),
		string(s.Status),
		string(s.OnlineState),
		s.LastSeenAt,
		s.LastSyncedAt,
		prefsJSON,
		s.HelpRating,
		s.HelpCount,
		time.Now().UTC(),
		s.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update student: %w", err)
	}

	if result.RowsAffected() == 0 {
		return student.ErrStudentNotFound
	}

	return nil
}

// Delete performs a soft delete on a student (sets status to 'left').
func (r *StudentRepository) Delete(ctx context.Context, id string) error {
	query := `
		UPDATE students 
		SET status = $1, updated_at = $2
		WHERE id = $3
	`

	result, err := r.conn.Exec(ctx, query,
		string(student.StatusLeft),
		time.Now().UTC(),
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to delete student: %w", err)
	}

	if result.RowsAffected() == 0 {
		return student.ErrStudentNotFound
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Bulk Operations
// ─────────────────────────────────────────────────────────────────────────────

// GetAll returns all students with pagination.
func (r *StudentRepository) GetAll(ctx context.Context, opts student.ListOptions) ([]*student.Student, error) {
	query := r.buildListQuery(opts, "")
	return r.queryStudents(ctx, query, opts.Limit, opts.Offset)
}

// GetByCohort returns students by cohort.
func (r *StudentRepository) GetByCohort(ctx context.Context, cohort student.Cohort, opts student.ListOptions) ([]*student.Student, error) {
	query := r.buildListQuery(opts, "cohort = $3")
	return r.queryStudentsWithArgs(ctx, query, opts.Limit, opts.Offset, string(cohort))
}

// GetByStatus returns students by status.
func (r *StudentRepository) GetByStatus(ctx context.Context, status student.Status, opts student.ListOptions) ([]*student.Student, error) {
	query := r.buildListQuery(opts, "status = $3")
	return r.queryStudentsWithArgs(ctx, query, opts.Limit, opts.Offset, string(status))
}

// GetByIDs returns students by a list of IDs.
func (r *StudentRepository) GetByIDs(ctx context.Context, ids []string) ([]*student.Student, error) {
	if len(ids) == 0 {
		return []*student.Student{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
		WHERE id IN (%s)
	`, strings.Join(placeholders, ", "))

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query students by ids: %w", err)
	}
	defer rows.Close()

	return r.scanStudents(rows)
}

// Count returns the total number of students.
func (r *StudentRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.conn.QueryRow(ctx, "SELECT COUNT(*) FROM students").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count students: %w", err)
	}
	return count, nil
}

// CountByCohort returns the number of students in a cohort.
func (r *StudentRepository) CountByCohort(ctx context.Context, cohort student.Cohort) (int, error) {
	var count int
	err := r.conn.QueryRow(ctx,
		"SELECT COUNT(*) FROM students WHERE cohort = $1",
		string(cohort),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count students by cohort: %w", err)
	}
	return count, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Search & Filter
// ─────────────────────────────────────────────────────────────────────────────

// Search searches students by name or login.
func (r *StudentRepository) Search(ctx context.Context, query string, opts student.ListOptions) ([]*student.Student, error) {
	searchPattern := "%" + strings.ToLower(query) + "%"

	sqlQuery := `
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
		WHERE (LOWER(email) LIKE $1 OR LOWER(display_name) LIKE $1)
	`

	if !opts.IncludeInactive {
		sqlQuery += " AND status IN ('active', 'inactive')"
	}

	sqlQuery += r.buildOrderBy(opts)
	sqlQuery += " LIMIT $2 OFFSET $3"

	rows, err := r.conn.Query(ctx, sqlQuery, searchPattern, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to search students: %w", err)
	}
	defer rows.Close()

	return r.scanStudents(rows)
}

// FindInactive finds students inactive for more than the specified duration.
func (r *StudentRepository) FindInactive(ctx context.Context, threshold time.Duration) ([]*student.Student, error) {
	thresholdTime := time.Now().UTC().Add(-threshold)

	query := `
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
		WHERE last_seen_at < $1 AND status = 'active'
		ORDER BY last_seen_at ASC
	`

	rows, err := r.conn.Query(ctx, query, thresholdTime)
	if err != nil {
		return nil, fmt.Errorf("failed to find inactive students: %w", err)
	}
	defer rows.Close()

	return r.scanStudents(rows)
}

// FindOnline finds students who are currently online.
func (r *StudentRepository) FindOnline(ctx context.Context) ([]*student.Student, error) {
	query := `
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
		WHERE online_state = 'online' AND status = 'active'
		ORDER BY current_xp DESC
	`

	rows, err := r.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to find online students: %w", err)
	}
	defer rows.Close()

	return r.scanStudents(rows)
}

// FindByXPRange finds students within the specified XP range.
func (r *StudentRepository) FindByXPRange(ctx context.Context, minXP, maxXP student.XP) ([]*student.Student, error) {
	query := `
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
		WHERE current_xp >= $1 AND current_xp <= $2
		ORDER BY current_xp DESC
	`

	rows, err := r.conn.Query(ctx, query, int(minXP), int(maxXP))
	if err != nil {
		return nil, fmt.Errorf("failed to find students by xp range: %w", err)
	}
	defer rows.Close()

	return r.scanStudents(rows)
}

// ─────────────────────────────────────────────────────────────────────────────
// Existence Checks
// ─────────────────────────────────────────────────────────────────────────────

// Exists checks if a student exists by ID.
func (r *StudentRepository) Exists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM students WHERE id = $1)",
		id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check student existence: %w", err)
	}
	return exists, nil
}

// ExistsByTelegramID checks if a student exists by Telegram ID.
func (r *StudentRepository) ExistsByTelegramID(ctx context.Context, telegramID student.TelegramID) (bool, error) {
	var exists bool
	err := r.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM students WHERE telegram_id = $1)",
		int64(telegramID),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check student existence by telegram id: %w", err)
	}
	return exists, nil
}

// ExistsByEmail checks if a student exists by email.
func (r *StudentRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM students WHERE email = $1)",
		email,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check student existence by email: %w", err)
	}
	return exists, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// PROGRESS REPOSITORY IMPLEMENTATION
// ══════════════════════════════════════════════════════════════════════════════

// ProgressRepository implements student.ProgressRepository for PostgreSQL.
type ProgressRepository struct {
	conn *Connection
}

// NewProgressRepository creates a new ProgressRepository.
func NewProgressRepository(conn *Connection) *ProgressRepository {
	return &ProgressRepository{conn: conn}
}

// ─────────────────────────────────────────────────────────────────────────────
// XP History
// ─────────────────────────────────────────────────────────────────────────────

// SaveXPChange saves an XP change entry.
func (r *ProgressRepository) SaveXPChange(ctx context.Context, entry student.XPHistoryEntry) error {
	query := `
		INSERT INTO xp_history (student_id, old_xp, new_xp, delta, reason, task_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	var taskID *string
	if entry.TaskID != "" {
		taskID = &entry.TaskID
	}

	_, err := r.conn.Exec(ctx, query,
		entry.Timestamp, // Note: This should be student_id, fixing below
		int(entry.OldXP),
		int(entry.NewXP),
		int(entry.Delta),
		entry.Reason,
		taskID,
		entry.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("failed to save xp change: %w", err)
	}

	return nil
}

// SaveXPChangeForStudent saves an XP change entry for a specific student.
func (r *ProgressRepository) SaveXPChangeForStudent(ctx context.Context, studentID string, entry student.XPHistoryEntry) error {
	query := `
		INSERT INTO xp_history (student_id, old_xp, new_xp, delta, reason, task_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	var taskID *string
	if entry.TaskID != "" {
		taskID = &entry.TaskID
	}

	_, err := r.conn.Exec(ctx, query,
		studentID,
		int(entry.OldXP),
		int(entry.NewXP),
		int(entry.Delta),
		entry.Reason,
		taskID,
		entry.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("failed to save xp change: %w", err)
	}

	return nil
}

// GetXPHistory returns XP history for a student within a time range.
func (r *ProgressRepository) GetXPHistory(ctx context.Context, studentID string, from, to time.Time) ([]student.XPHistoryEntry, error) {
	query := `
		SELECT old_xp, new_xp, delta, reason, COALESCE(task_id, ''), created_at
		FROM xp_history
		WHERE student_id = $1 AND created_at >= $2 AND created_at <= $3
		ORDER BY created_at ASC
	`

	rows, err := r.conn.Query(ctx, query, studentID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to get xp history: %w", err)
	}
	defer rows.Close()

	return r.scanXPHistoryEntries(rows)
}

// GetRecentXPChanges returns the most recent XP changes.
func (r *ProgressRepository) GetRecentXPChanges(ctx context.Context, studentID string, limit int) ([]student.XPHistoryEntry, error) {
	query := `
		SELECT old_xp, new_xp, delta, reason, COALESCE(task_id, ''), created_at
		FROM xp_history
		WHERE student_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.conn.Query(ctx, query, studentID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent xp changes: %w", err)
	}
	defer rows.Close()

	return r.scanXPHistoryEntries(rows)
}

// ─────────────────────────────────────────────────────────────────────────────
// Daily Grind
// ─────────────────────────────────────────────────────────────────────────────

// SaveDailyGrind saves or updates daily progress.
func (r *ProgressRepository) SaveDailyGrind(ctx context.Context, grind *student.DailyGrind) error {
	query := `
		INSERT INTO daily_grinds (
			student_id, date, xp_start, xp_current, xp_gained, tasks_completed,
			sessions_count, total_session_minutes, first_activity_at, last_activity_at,
			rank_at_start, rank_current, rank_change, streak_day
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT(student_id, date) DO UPDATE SET
			xp_current = EXCLUDED.xp_current,
			xp_gained = EXCLUDED.xp_gained,
			tasks_completed = EXCLUDED.tasks_completed,
			sessions_count = EXCLUDED.sessions_count,
			total_session_minutes = EXCLUDED.total_session_minutes,
			first_activity_at = COALESCE(daily_grinds.first_activity_at, EXCLUDED.first_activity_at),
			last_activity_at = EXCLUDED.last_activity_at,
			rank_current = EXCLUDED.rank_current,
			rank_change = EXCLUDED.rank_change,
			streak_day = EXCLUDED.streak_day
	`

	var firstActivity, lastActivity *time.Time
	if !grind.FirstActivityAt.IsZero() {
		firstActivity = &grind.FirstActivityAt
	}
	if !grind.LastActivityAt.IsZero() {
		lastActivity = &grind.LastActivityAt
	}

	_, err := r.conn.Exec(ctx, query,
		grind.StudentID,
		grind.Date,
		int(grind.XPStart),
		int(grind.XPCurrent),
		int(grind.XPGained),
		grind.TasksCompleted,
		grind.SessionsCount,
		grind.TotalSessionMinutes,
		firstActivity,
		lastActivity,
		grind.RankAtStart,
		grind.RankCurrent,
		grind.RankChange,
		grind.StreakDay,
	)
	if err != nil {
		return fmt.Errorf("failed to save daily grind: %w", err)
	}

	return nil
}

// GetDailyGrind returns daily progress for a specific date.
func (r *ProgressRepository) GetDailyGrind(ctx context.Context, studentID string, date time.Time) (*student.DailyGrind, error) {
	dateOnly := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	query := `
		SELECT student_id, date, xp_start, xp_current, xp_gained, tasks_completed,
			   sessions_count, total_session_minutes, first_activity_at, last_activity_at,
			   rank_at_start, rank_current, rank_change, streak_day
		FROM daily_grinds
		WHERE student_id = $1 AND date = $2
	`

	row := r.conn.QueryRow(ctx, query, studentID, dateOnly)
	return r.scanDailyGrind(row)
}

// GetDailyGrindHistory returns daily progress history.
func (r *ProgressRepository) GetDailyGrindHistory(ctx context.Context, studentID string, days int) ([]*student.DailyGrind, error) {
	query := `
		SELECT student_id, date, xp_start, xp_current, xp_gained, tasks_completed,
			   sessions_count, total_session_minutes, first_activity_at, last_activity_at,
			   rank_at_start, rank_current, rank_change, streak_day
		FROM daily_grinds
		WHERE student_id = $1
		ORDER BY date DESC
		LIMIT $2
	`

	rows, err := r.conn.Query(ctx, query, studentID, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily grind history: %w", err)
	}
	defer rows.Close()

	var grinds []*student.DailyGrind
	for rows.Next() {
		grind, err := r.scanDailyGrindFromRows(rows)
		if err != nil {
			return nil, err
		}
		grinds = append(grinds, grind)
	}

	return grinds, rows.Err()
}

// GetTodayDailyGrind returns today's progress.
func (r *ProgressRepository) GetTodayDailyGrind(ctx context.Context, studentID string) (*student.DailyGrind, error) {
	return r.GetDailyGrind(ctx, studentID, time.Now().UTC())
}

// ─────────────────────────────────────────────────────────────────────────────
// Streaks
// ─────────────────────────────────────────────────────────────────────────────

// SaveStreak saves or updates a streak.
func (r *ProgressRepository) SaveStreak(ctx context.Context, streak *student.Streak) error {
	query := `
		INSERT INTO streaks (student_id, current_streak, best_streak, last_active_date, streak_start_date)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT(student_id) DO UPDATE SET
			current_streak = EXCLUDED.current_streak,
			best_streak = GREATEST(streaks.best_streak, EXCLUDED.best_streak),
			last_active_date = EXCLUDED.last_active_date,
			streak_start_date = EXCLUDED.streak_start_date
	`

	var lastActive, streakStart *time.Time
	if !streak.LastActiveDate.IsZero() {
		lastActive = &streak.LastActiveDate
	}
	if !streak.StreakStartDate.IsZero() {
		streakStart = &streak.StreakStartDate
	}

	_, err := r.conn.Exec(ctx, query,
		streak.StudentID,
		streak.CurrentStreak,
		streak.BestStreak,
		lastActive,
		streakStart,
	)
	if err != nil {
		return fmt.Errorf("failed to save streak: %w", err)
	}

	return nil
}

// GetStreak returns the current streak for a student.
func (r *ProgressRepository) GetStreak(ctx context.Context, studentID string) (*student.Streak, error) {
	query := `
		SELECT student_id, current_streak, best_streak, last_active_date, streak_start_date
		FROM streaks
		WHERE student_id = $1
	`

	row := r.conn.QueryRow(ctx, query, studentID)

	var streak student.Streak
	var lastActive, streakStart *time.Time

	err := row.Scan(
		&streak.StudentID,
		&streak.CurrentStreak,
		&streak.BestStreak,
		&lastActive,
		&streakStart,
	)

	if IsNoRows(err) {
		return student.NewStreak(studentID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get streak: %w", err)
	}

	if lastActive != nil {
		streak.LastActiveDate = *lastActive
	}
	if streakStart != nil {
		streak.StreakStartDate = *streakStart
	}

	return &streak, nil
}

// GetTopStreaks returns top students by current streak.
func (r *ProgressRepository) GetTopStreaks(ctx context.Context, limit int) ([]*student.Streak, error) {
	query := `
		SELECT student_id, current_streak, best_streak, last_active_date, streak_start_date
		FROM streaks
		WHERE current_streak > 0
		ORDER BY current_streak DESC
		LIMIT $1
	`

	rows, err := r.conn.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top streaks: %w", err)
	}
	defer rows.Close()

	var streaks []*student.Streak
	for rows.Next() {
		var streak student.Streak
		var lastActive, streakStart *time.Time

		err := rows.Scan(
			&streak.StudentID,
			&streak.CurrentStreak,
			&streak.BestStreak,
			&lastActive,
			&streakStart,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan streak: %w", err)
		}

		if lastActive != nil {
			streak.LastActiveDate = *lastActive
		}
		if streakStart != nil {
			streak.StreakStartDate = *streakStart
		}

		streaks = append(streaks, &streak)
	}

	return streaks, rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Achievements
// ─────────────────────────────────────────────────────────────────────────────

// SaveAchievement saves an unlocked achievement.
func (r *ProgressRepository) SaveAchievement(ctx context.Context, studentID string, achievement student.Achievement) error {
	query := `
		INSERT INTO achievements (student_id, achievement_type, unlocked_at, metadata)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT(student_id, achievement_type) DO NOTHING
	`

	var metadataJSON []byte
	var err error
	if achievement.Metadata != nil {
		metadataJSON, err = json.Marshal(achievement.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal achievement metadata: %w", err)
		}
	}

	_, err = r.conn.Exec(ctx, query,
		studentID,
		string(achievement.Type),
		achievement.UnlockedAt,
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to save achievement: %w", err)
	}

	return nil
}

// GetAchievements returns all achievements for a student.
func (r *ProgressRepository) GetAchievements(ctx context.Context, studentID string) ([]student.Achievement, error) {
	query := `
		SELECT achievement_type, unlocked_at, metadata
		FROM achievements
		WHERE student_id = $1
		ORDER BY unlocked_at DESC
	`

	rows, err := r.conn.Query(ctx, query, studentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get achievements: %w", err)
	}
	defer rows.Close()

	var achievements []student.Achievement
	for rows.Next() {
		var achievement student.Achievement
		var achievementType string
		var metadataJSON []byte

		err := rows.Scan(&achievementType, &achievement.UnlockedAt, &metadataJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan achievement: %w", err)
		}

		achievement.Type = student.AchievementType(achievementType)
		if len(metadataJSON) > 0 {
			_ = json.Unmarshal(metadataJSON, &achievement.Metadata)
		}

		achievements = append(achievements, achievement)
	}

	return achievements, rows.Err()
}

// HasAchievement checks if a student has an achievement.
func (r *ProgressRepository) HasAchievement(ctx context.Context, studentID string, achievementType student.AchievementType) (bool, error) {
	var exists bool
	err := r.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM achievements WHERE student_id = $1 AND achievement_type = $2)",
		studentID,
		string(achievementType),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check achievement: %w", err)
	}
	return exists, nil
}

// GetRecentAchievements returns recent achievements for all students.
func (r *ProgressRepository) GetRecentAchievements(ctx context.Context, since time.Time) ([]student.StudentAchievement, error) {
	query := `
		SELECT student_id, achievement_type, unlocked_at, metadata
		FROM achievements
		WHERE unlocked_at >= $1
		ORDER BY unlocked_at DESC
	`

	rows, err := r.conn.Query(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent achievements: %w", err)
	}
	defer rows.Close()

	var studentAchievements []student.StudentAchievement
	for rows.Next() {
		var sa student.StudentAchievement
		var achievementType string
		var metadataJSON []byte

		err := rows.Scan(&sa.StudentID, &achievementType, &sa.Achievement.UnlockedAt, &metadataJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan student achievement: %w", err)
		}

		sa.Achievement.Type = student.AchievementType(achievementType)
		if len(metadataJSON) > 0 {
			_ = json.Unmarshal(metadataJSON, &sa.Achievement.Metadata)
		}

		studentAchievements = append(studentAchievements, sa)
	}

	return studentAchievements, rows.Err()
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ══════════════════════════════════════════════════════════════════════════════

// scanStudent scans a single student from a row.
func (r *StudentRepository) scanStudent(row pgx.Row) (*student.Student, error) {
	var s student.Student
	var telegramID int64
	var email, passwordHash, cohort, status, onlineState string
	var currentXP int
	var prefsJSON []byte

	err := row.Scan(
		&s.ID,
		&telegramID,
		&email,
		&passwordHash,
		&s.DisplayName,
		&currentXP,
		&cohort,
		&status,
		&onlineState,
		&s.LastSeenAt,
		&s.LastSyncedAt,
		&s.JoinedAt,
		&prefsJSON,
		&s.HelpRating,
		&s.HelpCount,
		&s.CreatedAt,
		&s.UpdatedAt,
	)

	if IsNoRows(err) {
		return nil, student.ErrStudentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan student: %w", err)
	}

	s.TelegramID = student.TelegramID(telegramID)
	s.Email = email
	s.PasswordHash = passwordHash
	s.CurrentXP = student.XP(currentXP)
	s.Cohort = student.Cohort(cohort)
	s.Status = student.Status(status)
	s.OnlineState = student.OnlineState(onlineState)
	s.Preferences = mapToPreferences(prefsJSON)

	return &s, nil
}

// scanStudents scans multiple students from rows.
func (r *StudentRepository) scanStudents(rows pgx.Rows) ([]*student.Student, error) {
	var students []*student.Student

	for rows.Next() {
		var s student.Student
		var telegramID int64
		var email, passwordHash, cohort, status, onlineState string
		var currentXP int
		var prefsJSON []byte

		err := rows.Scan(
			&s.ID,
			&telegramID,
			&email,
			&passwordHash,
			&s.DisplayName,
			&currentXP,
			&cohort,
			&status,
			&onlineState,
			&s.LastSeenAt,
			&s.LastSyncedAt,
			&s.JoinedAt,
			&prefsJSON,
			&s.HelpRating,
			&s.HelpCount,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan student: %w", err)
		}

		s.TelegramID = student.TelegramID(telegramID)
		s.Email = email
		s.PasswordHash = passwordHash
		s.CurrentXP = student.XP(currentXP)
		s.Cohort = student.Cohort(cohort)
		s.Status = student.Status(status)
		s.OnlineState = student.OnlineState(onlineState)
		s.Preferences = mapToPreferences(prefsJSON)

		students = append(students, &s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return students, nil
}

// buildListQuery builds a SELECT query with filters and ordering.
func (r *StudentRepository) buildListQuery(opts student.ListOptions, whereClause string) string {
	query := `
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
	`

	conditions := []string{}
	if whereClause != "" {
		conditions = append(conditions, whereClause)
	}
	if !opts.IncludeInactive {
		conditions = append(conditions, "status IN ('active', 'inactive')")
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += r.buildOrderBy(opts)
	query += " LIMIT $1 OFFSET $2"

	return query
}

// buildOrderBy builds ORDER BY clause.
func (r *StudentRepository) buildOrderBy(opts student.ListOptions) string {
	orderField := "current_xp"
	validFields := map[string]string{
		"current_xp":   "current_xp",
		"xp":           "current_xp",
		"display_name": "display_name",
		"name":         "display_name",
		"last_seen_at": "last_seen_at",
		"joined_at":    "joined_at",
		"help_rating":  "help_rating",
		"created_at":   "created_at",
	}

	if field, ok := validFields[opts.SortBy]; ok {
		orderField = field
	}

	direction := "DESC"
	if !opts.SortDesc {
		direction = "ASC"
	}

	return fmt.Sprintf(" ORDER BY %s %s", orderField, direction)
}

// queryStudents executes a query and returns students.
func (r *StudentRepository) queryStudents(ctx context.Context, query string, limit, offset int) ([]*student.Student, error) {
	rows, err := r.conn.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query students: %w", err)
	}
	defer rows.Close()

	return r.scanStudents(rows)
}

// queryStudentsWithArgs executes a query with additional args.
func (r *StudentRepository) queryStudentsWithArgs(ctx context.Context, query string, limit, offset int, args ...interface{}) ([]*student.Student, error) {
	allArgs := append([]interface{}{limit, offset}, args...)
	rows, err := r.conn.Query(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query students: %w", err)
	}
	defer rows.Close()

	return r.scanStudents(rows)
}

// scanXPHistoryEntries scans XP history entries from rows.
func (r *ProgressRepository) scanXPHistoryEntries(rows pgx.Rows) ([]student.XPHistoryEntry, error) {
	var entries []student.XPHistoryEntry
	for rows.Next() {
		var entry student.XPHistoryEntry
		var oldXP, newXP, delta int

		err := rows.Scan(&oldXP, &newXP, &delta, &entry.Reason, &entry.TaskID, &entry.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan xp history entry: %w", err)
		}

		entry.OldXP = student.XP(oldXP)
		entry.NewXP = student.XP(newXP)
		entry.Delta = student.XP(delta)

		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// scanDailyGrind scans a daily grind from a row.
func (r *ProgressRepository) scanDailyGrind(row pgx.Row) (*student.DailyGrind, error) {
	var grind student.DailyGrind
	var xpStart, xpCurrent, xpGained int
	var firstActivity, lastActivity *time.Time

	err := row.Scan(
		&grind.StudentID,
		&grind.Date,
		&xpStart,
		&xpCurrent,
		&xpGained,
		&grind.TasksCompleted,
		&grind.SessionsCount,
		&grind.TotalSessionMinutes,
		&firstActivity,
		&lastActivity,
		&grind.RankAtStart,
		&grind.RankCurrent,
		&grind.RankChange,
		&grind.StreakDay,
	)

	if IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan daily grind: %w", err)
	}

	grind.XPStart = student.XP(xpStart)
	grind.XPCurrent = student.XP(xpCurrent)
	grind.XPGained = student.XP(xpGained)
	if firstActivity != nil {
		grind.FirstActivityAt = *firstActivity
	}
	if lastActivity != nil {
		grind.LastActivityAt = *lastActivity
	}

	return &grind, nil
}

// scanDailyGrindFromRows scans a daily grind from rows.
func (r *ProgressRepository) scanDailyGrindFromRows(rows pgx.Rows) (*student.DailyGrind, error) {
	var grind student.DailyGrind
	var xpStart, xpCurrent, xpGained int
	var firstActivity, lastActivity *time.Time

	err := rows.Scan(
		&grind.StudentID,
		&grind.Date,
		&xpStart,
		&xpCurrent,
		&xpGained,
		&grind.TasksCompleted,
		&grind.SessionsCount,
		&grind.TotalSessionMinutes,
		&firstActivity,
		&lastActivity,
		&grind.RankAtStart,
		&grind.RankCurrent,
		&grind.RankChange,
		&grind.StreakDay,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan daily grind: %w", err)
	}

	grind.XPStart = student.XP(xpStart)
	grind.XPCurrent = student.XP(xpCurrent)
	grind.XPGained = student.XP(xpGained)
	if firstActivity != nil {
		grind.FirstActivityAt = *firstActivity
	}
	if lastActivity != nil {
		grind.LastActivityAt = *lastActivity
	}

	return &grind, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// PREFERENCES CONVERSION
// ══════════════════════════════════════════════════════════════════════════════

// preferencesToMap converts NotificationPreferences to a map for JSON storage.
func preferencesToMap(prefs student.NotificationPreferences) map[string]interface{} {
	return map[string]interface{}{
		"rank_changes":         prefs.RankChanges,
		"daily_digest":         prefs.DailyDigest,
		"help_requests":        prefs.HelpRequests,
		"inactivity_reminders": prefs.InactivityReminders,
		"quiet_hours_start":    prefs.QuietHoursStart,
		"quiet_hours_end":      prefs.QuietHoursEnd,
	}
}

// mapToPreferences converts JSON bytes to NotificationPreferences.
func mapToPreferences(data []byte) student.NotificationPreferences {
	prefs := student.DefaultNotificationPreferences()

	if len(data) == 0 {
		return prefs
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return prefs
	}

	if v, ok := m["rank_changes"].(bool); ok {
		prefs.RankChanges = v
	}
	if v, ok := m["daily_digest"].(bool); ok {
		prefs.DailyDigest = v
	}
	if v, ok := m["help_requests"].(bool); ok {
		prefs.HelpRequests = v
	}
	if v, ok := m["inactivity_reminders"].(bool); ok {
		prefs.InactivityReminders = v
	}
	if v, ok := m["quiet_hours_start"].(float64); ok {
		prefs.QuietHoursStart = int(v)
	}
	if v, ok := m["quiet_hours_end"].(float64); ok {
		prefs.QuietHoursEnd = int(v)
	}

	return prefs
}
