package postgres

import (
	"context"
	"time"
	"errors"

	"github.com/alem-hub/alem-community-hub/internal/domain/activity"
)

// ActivityRepository implements activity.Repository using PostgreSQL.
type ActivityRepository struct {
	conn *Connection
}

// NewActivityRepository creates a new ActivityRepository.
func NewActivityRepository(conn *Connection) *ActivityRepository {
	return &ActivityRepository{
		conn: conn,
	}
}

func (r *ActivityRepository) SaveSession(ctx context.Context, session *activity.Session) error {
	return errors.New("not implemented")
}

func (r *ActivityRepository) GetActiveSession(ctx context.Context, studentID activity.StudentID) (*activity.Session, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) GetSessionsByStudent(ctx context.Context, studentID activity.StudentID, from, to time.Time) ([]*activity.Session, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) EndExpiredSessions(ctx context.Context, inactiveThreshold time.Duration) (int, error) {
	return 0, errors.New("not implemented")
}

func (r *ActivityRepository) SaveTaskCompletion(ctx context.Context, completion *activity.TaskCompletion) error {
	return errors.New("not implemented")
}

func (r *ActivityRepository) GetTaskCompletion(ctx context.Context, id string) (*activity.TaskCompletion, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) GetTaskCompletionsByStudent(ctx context.Context, studentID activity.StudentID, limit int) ([]*activity.TaskCompletion, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) GetStudentsWhoCompletedTask(ctx context.Context, taskID activity.TaskID, limit int) ([]activity.StudentID, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) HasStudentCompletedTask(ctx context.Context, studentID activity.StudentID, taskID activity.TaskID) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *ActivityRepository) SaveActivity(ctx context.Context, act *activity.Activity) error {
	return errors.New("not implemented")
}

func (r *ActivityRepository) GetActivity(ctx context.Context, studentID activity.StudentID) (*activity.Activity, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) GetActivitiesByStudents(ctx context.Context, studentIDs []activity.StudentID) ([]*activity.Activity, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) GetOnlineStudents(ctx context.Context) ([]activity.OnlineStatus, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) GetRecentlyActiveStudents(ctx context.Context, within time.Duration, limit int) ([]activity.OnlineStatus, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) UpdateOnlineStatus(ctx context.Context, status activity.OnlineStatus) error {
	return errors.New("not implemented")
}

func (r *ActivityRepository) GetDailyProgress(ctx context.Context, studentID activity.StudentID, date time.Time) (*activity.DailyProgress, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) SaveDailyProgress(ctx context.Context, progress *activity.DailyProgress) error {
	return errors.New("not implemented")
}

func (r *ActivityRepository) GetDailyProgressRange(ctx context.Context, studentID activity.StudentID, from, to time.Time) ([]*activity.DailyProgress, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) GetInactiveStudents(ctx context.Context, inactiveDuration time.Duration) ([]activity.StudentID, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) GetTopHelpers(ctx context.Context, limit int) ([]activity.StudentID, error) {
	return nil, errors.New("not implemented")
}

func (r *ActivityRepository) GetStudentStreak(ctx context.Context, studentID activity.StudentID) (current int, longest int, err error) {
	return 0, 0, errors.New("not implemented")
}
