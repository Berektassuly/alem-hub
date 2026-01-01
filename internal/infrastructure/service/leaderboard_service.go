package service

import (
	"context"

	"github.com/alem-hub/alem-community-hub/internal/domain/leaderboard"
)

// LeaderboardService provides high-level leaderboard operations.
type LeaderboardService struct {
	repo  leaderboard.LeaderboardRepository
	cache leaderboard.LeaderboardCache
}

// NewLeaderboardService creates a new LeaderboardService.
func NewLeaderboardService(repo leaderboard.LeaderboardRepository, cache leaderboard.LeaderboardCache) *LeaderboardService {
	return &LeaderboardService{
		repo:  repo,
		cache: cache,
	}
}

// GetStudentRank returns the global rank of a student.
func (s *LeaderboardService) GetStudentRank(ctx context.Context, studentID string) (int, error) {
	// We use the global cohort for sync purposes
	cohort := leaderboard.CohortAll

	// 1. Try cache if available
	if s.cache != nil {
		entry, err := s.cache.GetCachedRank(ctx, studentID, cohort)
		if err == nil && entry != nil {
			return int(entry.Rank), nil
		}
	}

	// 2. Try repository
	entry, err := s.repo.GetStudentRank(ctx, studentID, cohort)
	if err != nil {
		return 0, err
	}
	if entry == nil {
		return 0, nil // Rank unknown/unranked
	}

	return int(entry.Rank), nil
}

// InvalidateCache invalidates the leaderboard cache.
func (s *LeaderboardService) InvalidateCache(ctx context.Context) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.InvalidateAll(ctx)
}
