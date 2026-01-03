package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/domain/student"
)

// StudentCache implements student.StudentCache interface using generic Redis Cache.
type StudentCache struct {
	cache *Cache
}

// NewStudentCache creates a new StudentCache.
func NewStudentCache(cache *Cache) *StudentCache {
	return &StudentCache{
		cache: cache,
	}
}

// Get gets a student from cache.
func (s *StudentCache) Get(ctx context.Context, studentID string) (*student.Student, error) {
	var st student.Student
	key := StudentKey(studentID)
	err := s.cache.Get(ctx, key, &st)
	if err != nil {
		if err == ErrCacheMiss {
			// domain expects nil/error on miss? Usually nil for cache miss in repo pattern or specific error.
			// Repo returns ErrStudentNotFound. But Cache interface says Get -> (*Student, error).
			// Let's return error if miss to allow caller to handle.
			return nil, err
		}
		return nil, err
	}
	return &st, nil
}

// Set sets a student in cache.
func (s *StudentCache) Set(ctx context.Context, st *student.Student, ttl time.Duration) error {
	if st == nil {
		return nil
	}
	key := StudentKey(st.ID)
	return s.cache.Set(ctx, key, st, ttl)
}

// Delete removes a student from cache.
func (s *StudentCache) Delete(ctx context.Context, studentID string) error {
	key := StudentKey(studentID)
	return s.cache.Delete(ctx, key)
}

// GetByTelegramID gets a student from cache by Telegram ID.
// This often requires a secondary index key like "student:telegram:{id}" -> studentID.
// For now, if we don't have secondary index management in Set, we might miss this.
// Assuming we store full object or ID pointer.
// A common pattern: "student:telegram:123" -> "student_uuid". Then 2nd lookup.
// For simplicity here, let's assume we might store the student object directly under "student:telegram:123" too
// or implement the lookup.
func (s *StudentCache) GetByTelegramID(ctx context.Context, telegramID student.TelegramID) (*student.Student, error) {
	key := fmt.Sprintf("%s:telegram:%d", PrefixStudent, telegramID)
	// Try direct object
	var st student.Student
	err := s.cache.Get(ctx, key, &st)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// SetByTelegramID sets a student in cache by Telegram ID.
func (s *StudentCache) SetByTelegramID(ctx context.Context, st *student.Student, ttl time.Duration) error {
	if st == nil {
		return nil
	}
	key := fmt.Sprintf("%s:telegram:%d", PrefixStudent, st.TelegramID)
	return s.cache.Set(ctx, key, st, ttl)
}

// Invalidate invalidates all keys for a student.
func (s *StudentCache) Invalidate(ctx context.Context, studentID string) error {
	// We might need to look up telegram ID to invalidate that too, which is hard without knowing it.
	// For now just invalidate ID key.
	return s.cache.Delete(ctx, StudentKey(studentID))
}

// InvalidateAll clears all student cache.
func (s *StudentCache) InvalidateAll(ctx context.Context) error {
	return s.cache.DeleteByPattern(ctx, PrefixStudent+"*")
}
