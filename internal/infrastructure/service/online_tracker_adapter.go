package service

import (
	"context"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/domain/activity"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/persistence/redis"
)

// StudentOnlineTrackerAdapter adapts redis.OnlineTracker to student.OnlineTracker interface.
type StudentOnlineTrackerAdapter struct {
	tracker *redis.OnlineTracker
}

func NewStudentOnlineTrackerAdapter(tracker *redis.OnlineTracker) *StudentOnlineTrackerAdapter {
	return &StudentOnlineTrackerAdapter{tracker: tracker}
}

func (a *StudentOnlineTrackerAdapter) MarkOnline(ctx context.Context, studentID string) error {
	if a.tracker == nil {
		return nil
	}
	info := redis.OnlineInfo{
		StudentID:  studentID,
		State:      redis.StateOnline,
		LastSeenAt: time.Now(),
	}
	return a.tracker.SetOnline(ctx, info)
}

func (a *StudentOnlineTrackerAdapter) MarkOffline(ctx context.Context, studentID string) error {
	if a.tracker == nil {
		return nil
	}
	return a.tracker.SetOffline(ctx, studentID)
}

func (a *StudentOnlineTrackerAdapter) IsOnline(ctx context.Context, studentID string) (bool, error) {
	if a.tracker == nil {
		return false, nil
	}
	return a.tracker.IsOnline(ctx, studentID)
}

func (a *StudentOnlineTrackerAdapter) GetOnlineStudents(ctx context.Context) ([]string, error) {
	if a.tracker == nil {
		return []string{}, nil
	}
	entries, err := a.tracker.GetAllOnline(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(entries))
	for i, entry := range entries {
		result[i] = entry.StudentID
	}
	return result, nil
}

func (a *StudentOnlineTrackerAdapter) GetOnlineCount(ctx context.Context) (int, error) {
	if a.tracker == nil {
		return 0, nil
	}
	count, err := a.tracker.GetOnlineCount(ctx)
	return int(count), err
}

func (a *StudentOnlineTrackerAdapter) GetLastSeen(ctx context.Context, studentID string) (time.Time, error) {
	if a.tracker == nil {
		return time.Time{}, nil
	}
	info, err := a.tracker.GetInfo(ctx, studentID)
	if err != nil {
		return time.Time{}, err
	}
	if info == nil {
		return time.Time{}, nil
	}
	return info.LastSeenAt, nil
}

func (a *StudentOnlineTrackerAdapter) SetLastActivity(ctx context.Context, studentID string, at time.Time) error {
	if a.tracker == nil {
		return nil
	}
	return a.tracker.Heartbeat(ctx, studentID)
}

func (a *StudentOnlineTrackerAdapter) GetOnlineStates(ctx context.Context, studentIDs []string) (map[string]student.OnlineState, error) {
	if a.tracker == nil {
		return map[string]student.OnlineState{}, nil
	}
	states, err := a.tracker.GetStates(ctx, studentIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]student.OnlineState)
	for id, state := range states {
		result[id] = redisStateToStudentState(state)
	}
	return result, nil
}

func redisStateToStudentState(state redis.OnlineState) student.OnlineState {
	switch state {
	case redis.StateOnline:
		return student.OnlineStateOnline
	case redis.StateAway:
		return student.OnlineStateAway
	default:
		return student.OnlineStateOffline
	}
}

// ActivityOnlineTrackerAdapter adapts redis.OnlineTracker to activity.OnlineTracker interface.
type ActivityOnlineTrackerAdapter struct {
	tracker *redis.OnlineTracker
}

func NewActivityOnlineTrackerAdapter(tracker *redis.OnlineTracker) *ActivityOnlineTrackerAdapter {
	return &ActivityOnlineTrackerAdapter{tracker: tracker}
}

func (a *ActivityOnlineTrackerAdapter) MarkOnline(ctx context.Context, studentID activity.StudentID, ttl time.Duration) error {
	if a.tracker == nil {
		return nil
	}
	info := redis.OnlineInfo{
		StudentID:  string(studentID),
		State:      redis.StateOnline,
		LastSeenAt: time.Now(),
	}
	return a.tracker.SetOnline(ctx, info)
}

func (a *ActivityOnlineTrackerAdapter) MarkOffline(ctx context.Context, studentID activity.StudentID) error {
	if a.tracker == nil {
		return nil
	}
	return a.tracker.SetOffline(ctx, string(studentID))
}

func (a *ActivityOnlineTrackerAdapter) RefreshOnline(ctx context.Context, studentID activity.StudentID, ttl time.Duration) error {
	if a.tracker == nil {
		return nil
	}
	return a.tracker.Heartbeat(ctx, string(studentID))
}

func (a *ActivityOnlineTrackerAdapter) IsOnline(ctx context.Context, studentID activity.StudentID) (bool, error) {
	if a.tracker == nil {
		return false, nil
	}
	return a.tracker.IsOnline(ctx, string(studentID))
}

func (a *ActivityOnlineTrackerAdapter) GetAllOnline(ctx context.Context) ([]activity.StudentID, error) {
	if a.tracker == nil {
		return []activity.StudentID{}, nil
	}
	entries, err := a.tracker.GetAllOnline(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]activity.StudentID, len(entries))
	for i, entry := range entries {
		result[i] = activity.StudentID(entry.StudentID)
	}
	return result, nil
}

func (a *ActivityOnlineTrackerAdapter) GetOnlineCount(ctx context.Context) (int, error) {
	if a.tracker == nil {
		return 0, nil
	}
	count, err := a.tracker.GetOnlineCount(ctx)
	return int(count), err
}

// TaskIndexStub provides a stub implementation of activity.TaskIndex.
type TaskIndexStub struct{}

func NewTaskIndexStub() *TaskIndexStub {
	return &TaskIndexStub{}
}

func (t *TaskIndexStub) IndexTaskCompletion(ctx context.Context, taskID activity.TaskID, studentID activity.StudentID, completedAt time.Time) error {
	return nil
}

func (t *TaskIndexStub) GetSolvers(ctx context.Context, taskID activity.TaskID, limit int) ([]activity.StudentID, error) {
	return []activity.StudentID{}, nil
}

func (t *TaskIndexStub) GetSolversOnline(ctx context.Context, taskID activity.TaskID, onlineTracker activity.OnlineTracker) ([]activity.StudentID, error) {
	return []activity.StudentID{}, nil
}

func (t *TaskIndexStub) GetTasksSolvedBy(ctx context.Context, studentID activity.StudentID) ([]activity.TaskID, error) {
	return []activity.TaskID{}, nil
}

func (t *TaskIndexStub) GetRecentSolvers(ctx context.Context, within time.Duration, limit int) ([]activity.StudentID, error) {
	return []activity.StudentID{}, nil
}
