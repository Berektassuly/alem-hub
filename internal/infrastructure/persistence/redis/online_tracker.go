// Package redis implements Redis caching, pub/sub, and online tracking functionality.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE TRACKER ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrStudentIDEmpty is returned when student ID is empty.
	ErrStudentIDEmpty = errors.New("online_tracker: student ID cannot be empty")

	// ErrInvalidOnlineState is returned when online state is invalid.
	ErrInvalidOnlineState = errors.New("online_tracker: invalid online state")
)

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE STATE CONSTANTS
// ══════════════════════════════════════════════════════════════════════════════

// OnlineState represents a student's online presence state.
type OnlineState string

const (
	// StateOnline indicates the student is currently active (last seen < 5 min ago).
	StateOnline OnlineState = "online"

	// StateAway indicates the student is away (last seen 5-30 min ago).
	StateAway OnlineState = "away"

	// StateOffline indicates the student is offline (last seen > 30 min ago or never).
	StateOffline OnlineState = "offline"
)

// IsValid checks if the online state is valid.
func (s OnlineState) IsValid() bool {
	switch s {
	case StateOnline, StateAway, StateOffline:
		return true
	default:
		return false
	}
}

// IsAvailable returns true if the student can be contacted.
func (s OnlineState) IsAvailable() bool {
	return s == StateOnline || s == StateAway
}

// Emoji returns an emoji representation of the state.
func (s OnlineState) Emoji() string {
	switch s {
	case StateOnline:
		return "🟢"
	case StateAway:
		return "🟡"
	default:
		return "⚪"
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE INFO STRUCTURE
// ══════════════════════════════════════════════════════════════════════════════

// OnlineInfo contains detailed information about a student's online status.
type OnlineInfo struct {
	// StudentID is the unique identifier of the student.
	StudentID string `json:"student_id"`



	// DisplayName is the student's display name.
	DisplayName string `json:"display_name,omitempty"`

	// State is the current online state.
	State OnlineState `json:"state"`

	// LastSeenAt is the timestamp of last activity.
	LastSeenAt time.Time `json:"last_seen_at"`

	// CurrentTask is the task the student is currently working on (if known).
	CurrentTask string `json:"current_task,omitempty"`

	// IsAvailableForHelp indicates if the student is willing to help others.
	IsAvailableForHelp bool `json:"is_available_for_help"`

	// SessionStartedAt is when the current session started (if online).
	SessionStartedAt time.Time `json:"session_started_at,omitempty"`
}

// TimeSinceLastSeen returns the duration since last activity.
func (o *OnlineInfo) TimeSinceLastSeen() time.Duration {
	return time.Since(o.LastSeenAt)
}

// IsOnline returns true if the student is in online state.
func (o *OnlineInfo) IsOnline() bool {
	return o.State == StateOnline
}

// IsAway returns true if the student is in away state.
func (o *OnlineInfo) IsAway() bool {
	return o.State == StateAway
}

// CalculateState determines the state based on last seen time.
func (o *OnlineInfo) CalculateState() OnlineState {
	elapsed := time.Since(o.LastSeenAt)
	switch {
	case elapsed < TTLOnlineStatus:
		return StateOnline
	case elapsed < TTLAwayStatus:
		return StateAway
	default:
		return StateOffline
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE EVENT STRUCTURE (for Pub/Sub)
// ══════════════════════════════════════════════════════════════════════════════

// OnlineEventType defines the type of online status change event.
type OnlineEventType string

const (
	// EventWentOnline is emitted when a student comes online.
	EventWentOnline OnlineEventType = "went_online"

	// EventWentAway is emitted when a student goes away.
	EventWentAway OnlineEventType = "went_away"

	// EventWentOffline is emitted when a student goes offline.
	EventWentOffline OnlineEventType = "went_offline"

	// EventHeartbeat is emitted periodically while online.
	EventHeartbeat OnlineEventType = "heartbeat"
)

// OnlineEvent represents a change in online status for Pub/Sub.
type OnlineEvent struct {
	// Type is the type of event.
	Type OnlineEventType `json:"type"`

	// StudentID is the student's unique identifier.
	StudentID string `json:"student_id"`



	// PreviousState is the state before the change (if applicable).
	PreviousState OnlineState `json:"previous_state,omitempty"`

	// NewState is the new state.
	NewState OnlineState `json:"new_state"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
}

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE TRACKER
// ══════════════════════════════════════════════════════════════════════════════

// OnlineTracker manages real-time online status of students using Redis.
// It uses TTL-based keys for automatic expiration and Pub/Sub for real-time updates.
//
// Architecture:
//   - Each online student has a key "online:{student_id}" with TTL
//   - A sorted set "online:all" tracks all online students by last_seen timestamp
//   - Pub/Sub channel "pubsub:online_status" broadcasts status changes
type OnlineTracker struct {
	cache *Cache
}

// Key names for online tracking.
const (
	// keyOnlineAll is the sorted set containing all online students.
	keyOnlineAll = "online:all"

	// keyOnlineByTask is the prefix for sets of students working on specific tasks.
	keyOnlineByTask = "online:task:"

	// channelOnlineStatus is the Pub/Sub channel for online status changes.
	channelOnlineStatus = "pubsub:online_status"
)

// NewOnlineTracker creates a new OnlineTracker instance.
func NewOnlineTracker(cache *Cache) *OnlineTracker {
	return &OnlineTracker{cache: cache}
}

// ══════════════════════════════════════════════════════════════════════════════
// CORE OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// SetOnline marks a student as online and updates their info.
// This should be called whenever a student performs an action (heartbeat).
func (t *OnlineTracker) SetOnline(ctx context.Context, info OnlineInfo) error {
	if info.StudentID == "" {
		return ErrStudentIDEmpty
	}

	now := time.Now().UTC()
	info.LastSeenAt = now
	info.State = StateOnline

	// Get previous state to determine if we need to emit an event
	previousState, _ := t.GetState(ctx, info.StudentID)

	// Use pipeline for atomic operations
	pipe := t.cache.Client().Pipeline()

	// 1. Store detailed info with TTL
	key := OnlineKey(info.StudentID)
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal online info: %w", err)
	}
	pipe.Set(ctx, key, data, TTLAwayStatus)

	// 2. Add to sorted set with timestamp as score
	pipe.ZAdd(ctx, keyOnlineAll, redis.Z{
		Score:  float64(now.Unix()),
		Member: info.StudentID,
	})

	// 3. If working on a task, add to task-specific set
	if info.CurrentTask != "" {
		taskKey := keyOnlineByTask + info.CurrentTask
		pipe.SAdd(ctx, taskKey, info.StudentID)
		pipe.Expire(ctx, taskKey, TTLAwayStatus)
	}

	// Execute pipeline
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to set online status: %w", err)
	}

	// Emit event if state changed
	if previousState != StateOnline {
		event := OnlineEvent{
			Type:          EventWentOnline,
			StudentID:     info.StudentID,
			PreviousState: previousState,
			NewState:      StateOnline,
			Timestamp:     now,
		}
		t.publishEvent(ctx, event)
	}

	return nil
}

// Heartbeat updates the last seen time without changing other info.
// This is a lightweight operation for periodic keep-alive.
func (t *OnlineTracker) Heartbeat(ctx context.Context, studentID string) error {
	if studentID == "" {
		return ErrStudentIDEmpty
	}

	now := time.Now().UTC()

	// Get existing info
	info, err := t.GetInfo(ctx, studentID)
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			// No existing info, create minimal entry
			info = &OnlineInfo{
				StudentID:  studentID,
				State:      StateOnline,
				LastSeenAt: now,
			}
		} else {
			return err
		}
	}

	info.LastSeenAt = now
	info.State = StateOnline

	// Update storage
	pipe := t.cache.Client().Pipeline()

	key := OnlineKey(studentID)
	data, _ := json.Marshal(info)
	pipe.Set(ctx, key, data, TTLAwayStatus)

	pipe.ZAdd(ctx, keyOnlineAll, redis.Z{
		Score:  float64(now.Unix()),
		Member: studentID,
	})

	_, err = pipe.Exec(ctx)
	return err
}

// SetOffline explicitly marks a student as offline.
// This should be called when a student logs out or disconnects.
func (t *OnlineTracker) SetOffline(ctx context.Context, studentID string) error {
	if studentID == "" {
		return ErrStudentIDEmpty
	}

	// Get current info for the event
	var previousState OnlineState
	info, _ := t.GetInfo(ctx, studentID)
	if info != nil {
		previousState = info.State
	}

	// Remove from all tracking structures
	pipe := t.cache.Client().Pipeline()

	key := OnlineKey(studentID)
	pipe.Del(ctx, key)
	pipe.ZRem(ctx, keyOnlineAll, studentID)

	// Remove from task sets (we don't know which tasks, so we skip this)
	// Task sets will expire naturally via TTL

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to set offline: %w", err)
	}

	// Emit event
	if previousState != StateOffline {
		event := OnlineEvent{
			Type:          EventWentOffline,
			StudentID:     studentID,
			PreviousState: previousState,
			NewState:      StateOffline,
			Timestamp:     time.Now().UTC(),
		}
		t.publishEvent(ctx, event)
	}

	return nil
}

// SetAway marks a student as away (idle).
func (t *OnlineTracker) SetAway(ctx context.Context, studentID string) error {
	if studentID == "" {
		return ErrStudentIDEmpty
	}

	info, err := t.GetInfo(ctx, studentID)
	if err != nil {
		return err
	}

	previousState := info.State
	info.State = StateAway

	key := OnlineKey(studentID)
	data, _ := json.Marshal(info)
	if err := t.cache.Client().Set(ctx, key, data, TTLAwayStatus-TTLOnlineStatus).Err(); err != nil {
		return err
	}

	// Emit event
	if previousState != StateAway {
		event := OnlineEvent{
			Type:          EventWentAway,
			StudentID:     studentID,
			PreviousState: previousState,
			NewState:      StateAway,
			Timestamp:     time.Now().UTC(),
		}
		t.publishEvent(ctx, event)
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// QUERY OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// GetInfo retrieves detailed online info for a student.
func (t *OnlineTracker) GetInfo(ctx context.Context, studentID string) (*OnlineInfo, error) {
	if studentID == "" {
		return nil, ErrStudentIDEmpty
	}

	key := OnlineKey(studentID)
	data, err := t.cache.Client().Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}

	var info OnlineInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal online info: %w", err)
	}

	// Recalculate state based on last seen time
	info.State = info.CalculateState()

	return &info, nil
}

// GetState returns just the online state for a student.
func (t *OnlineTracker) GetState(ctx context.Context, studentID string) (OnlineState, error) {
	info, err := t.GetInfo(ctx, studentID)
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			return StateOffline, nil
		}
		return StateOffline, err
	}
	return info.CalculateState(), nil
}

// IsOnline checks if a student is currently online.
func (t *OnlineTracker) IsOnline(ctx context.Context, studentID string) (bool, error) {
	state, err := t.GetState(ctx, studentID)
	if err != nil {
		return false, err
	}
	return state == StateOnline, nil
}

// IsAvailable checks if a student is available (online or away).
func (t *OnlineTracker) IsAvailable(ctx context.Context, studentID string) (bool, error) {
	state, err := t.GetState(ctx, studentID)
	if err != nil {
		return false, err
	}
	return state.IsAvailable(), nil
}

// GetAllOnline returns all currently online students.
// This uses the sorted set for efficient retrieval.
func (t *OnlineTracker) GetAllOnline(ctx context.Context) ([]OnlineInfo, error) {
	cutoff := time.Now().Add(-TTLOnlineStatus).Unix()

	// Get students whose last_seen is within TTLOnlineStatus
	studentIDs, err := t.cache.Client().ZRangeByScore(ctx, keyOnlineAll, &redis.ZRangeBy{
		Min: strconv.FormatInt(cutoff, 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get online students: %w", err)
	}

	if len(studentIDs) == 0 {
		return []OnlineInfo{}, nil
	}

	// Fetch detailed info for each student
	return t.getInfoBatch(ctx, studentIDs)
}

// GetAllAvailable returns all students who are online or away.
func (t *OnlineTracker) GetAllAvailable(ctx context.Context) ([]OnlineInfo, error) {
	cutoff := time.Now().Add(-TTLAwayStatus).Unix()

	studentIDs, err := t.cache.Client().ZRangeByScore(ctx, keyOnlineAll, &redis.ZRangeBy{
		Min: strconv.FormatInt(cutoff, 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get available students: %w", err)
	}

	if len(studentIDs) == 0 {
		return []OnlineInfo{}, nil
	}

	return t.getInfoBatch(ctx, studentIDs)
}

// GetOnlineCount returns the count of currently online students.
func (t *OnlineTracker) GetOnlineCount(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-TTLOnlineStatus).Unix()

	return t.cache.Client().ZCount(ctx, keyOnlineAll,
		strconv.FormatInt(cutoff, 10), "+inf").Result()
}

// GetAvailableCount returns the count of available students (online + away).
func (t *OnlineTracker) GetAvailableCount(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-TTLAwayStatus).Unix()

	return t.cache.Client().ZCount(ctx, keyOnlineAll,
		strconv.FormatInt(cutoff, 10), "+inf").Result()
}

// GetStudentsOnTask returns students currently working on a specific task.
func (t *OnlineTracker) GetStudentsOnTask(ctx context.Context, taskID string) ([]OnlineInfo, error) {
	if taskID == "" {
		return nil, errors.New("task ID cannot be empty")
	}

	taskKey := keyOnlineByTask + taskID
	studentIDs, err := t.cache.Client().SMembers(ctx, taskKey).Result()
	if err != nil {
		return nil, err
	}

	if len(studentIDs) == 0 {
		return []OnlineInfo{}, nil
	}

	// Filter to only online students
	result := make([]OnlineInfo, 0, len(studentIDs))
	for _, id := range studentIDs {
		info, err := t.GetInfo(ctx, id)
		if err == nil && info.State == StateOnline {
			result = append(result, *info)
		}
	}

	return result, nil
}

// GetRecentlyOnline returns students who were online within the specified duration.
func (t *OnlineTracker) GetRecentlyOnline(ctx context.Context, within time.Duration) ([]OnlineInfo, error) {
	cutoff := time.Now().Add(-within).Unix()

	studentIDs, err := t.cache.Client().ZRangeByScore(ctx, keyOnlineAll, &redis.ZRangeBy{
		Min: strconv.FormatInt(cutoff, 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, err
	}

	if len(studentIDs) == 0 {
		return []OnlineInfo{}, nil
	}

	return t.getInfoBatch(ctx, studentIDs)
}

// ══════════════════════════════════════════════════════════════════════════════
// BATCH OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// GetStates returns online states for multiple students.
func (t *OnlineTracker) GetStates(ctx context.Context, studentIDs []string) (map[string]OnlineState, error) {
	if len(studentIDs) == 0 {
		return make(map[string]OnlineState), nil
	}

	result := make(map[string]OnlineState, len(studentIDs))

	// Use pipeline for efficient batch retrieval
	pipe := t.cache.Client().Pipeline()
	cmds := make(map[string]*redis.StringCmd, len(studentIDs))

	for _, id := range studentIDs {
		key := OnlineKey(id)
		cmds[id] = pipe.Get(ctx, key)
	}

	_, _ = pipe.Exec(ctx) // Errors are handled per-command

	now := time.Now()
	for id, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			result[id] = StateOffline
			continue
		}

		var info OnlineInfo
		if err := json.Unmarshal(data, &info); err != nil {
			result[id] = StateOffline
			continue
		}

		elapsed := now.Sub(info.LastSeenAt)
		switch {
		case elapsed < TTLOnlineStatus:
			result[id] = StateOnline
		case elapsed < TTLAwayStatus:
			result[id] = StateAway
		default:
			result[id] = StateOffline
		}
	}

	return result, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// PUB/SUB OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// Subscribe creates a subscription to online status changes.
// Remember to call Close() on the returned PubSub when done.
func (t *OnlineTracker) Subscribe(ctx context.Context) *redis.PubSub {
	return t.cache.Client().Subscribe(ctx, channelOnlineStatus)
}

// SubscribeWithHandler subscribes to online events and calls handler for each event.
// This is a blocking operation and should be run in a goroutine.
// The handler receives deserialized OnlineEvent objects.
func (t *OnlineTracker) SubscribeWithHandler(ctx context.Context, handler func(OnlineEvent)) error {
	pubsub := t.Subscribe(ctx)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return errors.New("subscription closed")
			}

			var event OnlineEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				continue // Skip malformed messages
			}

			handler(event)
		}
	}
}

// publishEvent publishes an online status change event.
func (t *OnlineTracker) publishEvent(ctx context.Context, event OnlineEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	// Fire and forget - don't block on publish errors
	_ = t.cache.Client().Publish(ctx, channelOnlineStatus, data).Err()
}

// ══════════════════════════════════════════════════════════════════════════════
// MAINTENANCE OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// CleanupStale removes stale entries from the online tracking set.
// This should be run periodically (e.g., every hour) as a background job.
func (t *OnlineTracker) CleanupStale(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-TTLAwayStatus).Unix()

	// Remove entries older than TTLAwayStatus
	removed, err := t.cache.Client().ZRemRangeByScore(ctx, keyOnlineAll,
		"-inf", strconv.FormatInt(cutoff, 10)).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup stale entries: %w", err)
	}

	return removed, nil
}

// RefreshAll recalculates states for all tracked students.
// This can be used after a server restart to ensure consistency.
func (t *OnlineTracker) RefreshAll(ctx context.Context) error {
	// Get all student IDs from the sorted set
	studentIDs, err := t.cache.Client().ZRange(ctx, keyOnlineAll, 0, -1).Result()
	if err != nil {
		return err
	}

	now := time.Now()
	cutoff := now.Add(-TTLAwayStatus)

	for _, id := range studentIDs {
		info, err := t.GetInfo(ctx, id)
		if err != nil {
			continue
		}

		// Remove if too old
		if info.LastSeenAt.Before(cutoff) {
			_ = t.SetOffline(ctx, id)
			continue
		}

		// Update state
		newState := info.CalculateState()
		if newState != info.State {
			info.State = newState
			key := OnlineKey(id)
			data, _ := json.Marshal(info)
			_ = t.cache.Client().Set(ctx, key, data, TTLAwayStatus-now.Sub(info.LastSeenAt)).Err()
		}
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ══════════════════════════════════════════════════════════════════════════════

// getInfoBatch retrieves OnlineInfo for multiple students efficiently.
func (t *OnlineTracker) getInfoBatch(ctx context.Context, studentIDs []string) ([]OnlineInfo, error) {
	if len(studentIDs) == 0 {
		return []OnlineInfo{}, nil
	}

	// Build keys
	keys := make([]string, len(studentIDs))
	for i, id := range studentIDs {
		keys[i] = OnlineKey(id)
	}

	// Use MGet for batch retrieval
	values, err := t.cache.Client().MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	result := make([]OnlineInfo, 0, len(studentIDs))
	for _, val := range values {
		if val == nil {
			continue
		}

		var info OnlineInfo
		if err := json.Unmarshal([]byte(val.(string)), &info); err != nil {
			continue
		}

		// Recalculate state
		info.State = info.CalculateState()
		result = append(result, info)
	}

	return result, nil
}
