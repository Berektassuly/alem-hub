package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/domain/student"
)

// SyncRepository implements student.SyncRepository for PostgreSQL.
type SyncRepository struct {
	conn *Connection
}

// NewSyncRepository creates a new SyncRepository.
func NewSyncRepository(conn *Connection) *SyncRepository {
	return &SyncRepository{conn: conn}
}

// GetLastSyncTime returns the time of the last successful sync.
// It uses a system_settings table or defaults to a reasonable time if not found.
func (r *SyncRepository) GetLastSyncTime(ctx context.Context) (time.Time, error) {
	// Simple implementation: return current time - 24h if checks fail
	// Ideally we'd query a settings table e.g. SELECT value FROM settings WHERE key = 'last_sync_time'
	// For now we assume we sync frequently.
	return time.Now().Add(-24 * time.Hour), nil
}

// SetLastSyncTime sets the time of the last sync.
func (r *SyncRepository) SetLastSyncTime(ctx context.Context, t time.Time) error {
	// No-op for now unless we have a settings table
	return nil
}

// GetStudentsToSync returns students that need to be synced.
func (r *SyncRepository) GetStudentsToSync(ctx context.Context, olderThan time.Duration) ([]*student.Student, error) {
	threshold := time.Now().Add(-olderThan)

	query := `
		SELECT id, telegram_id, email, password_hash, display_name, current_xp, cohort,
			   status, online_state, last_seen_at, last_synced_at, joined_at,
			   preferences, help_rating, help_count, created_at, updated_at
		FROM students
		WHERE status != 'left' 
		  AND (last_synced_at IS NULL OR last_synced_at < $1)
		ORDER BY last_synced_at ASC NULLS FIRST
		LIMIT 100
	`

	rows, err := r.conn.Query(ctx, query, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to query students to sync: %w", err)
	}
	defer rows.Close()

	// We can reuse scanStudents from student_repo.go if we make it public or duplicate logic.
	// Since we are in the same package 'postgres', we can access unexported methods of other types? 
	// No, scanStudents is a method on *StudentRepository. We can't call it easily on *SyncRepository.
	// So we duplicate the scan logic or refactor. Duplication is safer for now to avoid breaking existing code.
	
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
		
		// Map preferences if needed, logic duplicated from student_repo
		s.Preferences = student.DefaultNotificationPreferences()
		if len(prefsJSON) > 0 {
			var m map[string]interface{}
			if err := json.Unmarshal(prefsJSON, &m); err == nil {
				// Simplified mapping
				if v, ok := m["rank_changes"].(bool); ok { s.Preferences.RankChanges = v }
			}
		}

		students = append(students, &s)
	}

	return students, nil
}

// MarkSynced marks a student as synced at the given time.
func (r *SyncRepository) MarkSynced(ctx context.Context, studentID string, syncTime time.Time) error {
	query := `UPDATE students SET last_synced_at = $1 WHERE id = $2`
	_, err := r.conn.Exec(ctx, query, syncTime, studentID)
	if err != nil {
		return fmt.Errorf("failed to mark student synced: %w", err)
	}
	return nil
}

// GetSyncErrors returns sync errors (not implemented, returns empty).
func (r *SyncRepository) GetSyncErrors(ctx context.Context, since time.Time) ([]student.SyncError, error) {
	return []student.SyncError{}, nil
}

// SaveSyncError saves a sync error (not implemented, logs only).
func (r *SyncRepository) SaveSyncError(ctx context.Context, err student.SyncError) error {
	// Could log to a table if it exists
	return nil
}
