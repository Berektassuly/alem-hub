package service

import (
	"context"

	"github.com/alem-hub/alem-community-hub/internal/domain/activity"
	"github.com/alem-hub/alem-community-hub/internal/domain/social"
)

// HelperNotifierStub implements HelperNotifier commands.
type HelperNotifierStub struct{}

func NewHelperNotifierStub() *HelperNotifierStub {
	return &HelperNotifierStub{}
}

func (s *HelperNotifierStub) NotifyHelpRequest(ctx context.Context, helperID string, request *social.HelpRequest) error {
	// Stub implementation: do nothing or log
	return nil
}

// HelperMatchingServiceStub implements HelperMatchingService commands.
type HelperMatchingServiceStub struct{}

func NewHelperMatchingServiceStub() *HelperMatchingServiceStub {
	return &HelperMatchingServiceStub{}
}

func (s *HelperMatchingServiceStub) FindHelpers(ctx context.Context, requesterID string, taskID string, limit int) ([]activity.HelperSuggestion, error) {
	// Stub implementation: return empty list
	return []activity.HelperSuggestion{}, nil
}
