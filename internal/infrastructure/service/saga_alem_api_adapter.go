package service

import (
	"context"
	"errors"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/application/saga"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/external/alem"
)

// SagaAlemAPIAdapter adapts the alem.Client to the saga.AlemAPIClient interface.
type SagaAlemAPIAdapter struct {
	client *alem.Client
}

func NewSagaAlemAPIAdapter(client *alem.Client) *SagaAlemAPIAdapter {
	return &SagaAlemAPIAdapter{client: client}
}

func (a *SagaAlemAPIAdapter) GetStudentByLogin(ctx context.Context, login string) (*saga.AlemStudentData, error) {
	dto, err := a.client.GetStudentByLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	if dto == nil {
		return nil, nil
	}

	joinedAt := dto.CreatedAt
	if joinedAt.IsZero() {
		joinedAt = time.Now()
	}

	return &saga.AlemStudentData{
		Login:       dto.Login,
		DisplayName: dto.FullName(),
		XP:          dto.XP,
		Level:       dto.Level,
		Cohort:      dto.Cohort,
		JoinedAt:    joinedAt,
	}, nil
}

func (a *SagaAlemAPIAdapter) ValidateLogin(ctx context.Context, login string) (bool, error) {
	dto, err := a.client.GetStudentByLogin(ctx, login)
	if err != nil {
		return false, err
	}
	return dto != nil, nil
}

func (a *SagaAlemAPIAdapter) Authenticate(ctx context.Context, email, password string) (*saga.AlemStudentData, error) {
	// This is a stub implementation since we don't have actual authentication endpoints
	// In a real implementation, this would call the Alem API's authentication endpoint
	return nil, errors.New("authentication not implemented: use the platform's authentication flow")
}
