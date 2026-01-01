package service

import (
	"context"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/application/command"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/external/alem"
)

// AlemAPIAdapter adapts the alem.Client to the command.AlemAPIClient interface.
type AlemAPIAdapter struct {
	client *alem.Client
}

func NewAlemAPIAdapter(client *alem.Client) *AlemAPIAdapter {
	return &AlemAPIAdapter{client: client}
}

func (a *AlemAPIAdapter) GetStudentByLogin(ctx context.Context, login string) (*command.AlemStudentData, error) {
	dto, err := a.client.GetStudentByLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	if dto == nil {
		return nil, nil
	}

	lastActivity := time.Now()
	if dto.LastActivityAt != nil {
		lastActivity = *dto.LastActivityAt
	}

	var completedTasks []string
	if dto.Stats != nil {
		// We don't have the actual task IDs in StudentDTO, so we'd need another call
		// For now, return empty slice
		completedTasks = []string{}
	}

	return &command.AlemStudentData{
		Login:           dto.Login,
		DisplayName:     dto.FullName(),
		XP:              dto.XP,
		Level:           dto.Level,
		Cohort:          dto.Cohort,
		CompletedTasks:  completedTasks,
		LastActivityAt:  lastActivity,
		IsOnline:        dto.IsOnline,
		ProfileImageURL: dto.AvatarURL,
	}, nil
}

func (a *AlemAPIAdapter) GetAllStudents(ctx context.Context) ([]command.AlemStudentData, error) {
	// Stub: return empty slice for now
	return []command.AlemStudentData{}, nil
}

func (a *AlemAPIAdapter) GetStudentTasks(ctx context.Context, login string) ([]string, error) {
	// Stub: return empty slice for now
	return []string{}, nil
}
