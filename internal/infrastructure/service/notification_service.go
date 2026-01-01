package service

import (
	"context"
	"log/slog"

	"github.com/alem-hub/alem-community-hub/internal/domain/notification"
	"github.com/google/uuid"
)

// IDGeneratorImpl implements IDGenerator.
type IDGeneratorImpl struct{}

func NewIDGenerator() *IDGeneratorImpl {
	return &IDGeneratorImpl{}
}

func (g *IDGeneratorImpl) GenerateID() string {
	return uuid.New().String()
}

// NotificationServiceStub implements NotificationService.
type NotificationServiceStub struct {
	logger *slog.Logger
}

func NewNotificationServiceStub(logger *slog.Logger) *NotificationServiceStub {
	return &NotificationServiceStub{
		logger: logger,
	}
}

func (s *NotificationServiceStub) CreateNotification(ctx context.Context, rule *notification.TriggerRule, triggerCtx *notification.TriggerContext) (*notification.Notification, error) {
	s.logger.Info("stub: creating notification", "rule", rule.Name)
	return &notification.Notification{}, nil
}

func (s *NotificationServiceStub) ScheduleNotification(ctx context.Context, notif *notification.Notification) error {
	s.logger.Info("stub: scheduling notification", "id", notif.ID, "recipient", notif.RecipientID)
	return nil
}

func (s *NotificationServiceStub) CancelNotification(ctx context.Context, id notification.NotificationID) error {
	s.logger.Info("stub: cancelling notification", "id", id)
	return nil
}

func (s *NotificationServiceStub) ProcessPendingNotifications(ctx context.Context, batchSize int) (processed int, err error) {
	return 0, nil
}

func (s *NotificationServiceStub) ProcessExpiredNotifications(ctx context.Context) (expired int, err error) {
	return 0, nil
}

func (s *NotificationServiceStub) RetryFailedNotifications(ctx context.Context, batchSize int) (retried int, err error) {
	return 0, nil
}

func (s *NotificationServiceStub) EvaluateTriggers(ctx context.Context, triggerCtx *notification.TriggerContext) ([]*notification.TriggerRule, error) {
	return []*notification.TriggerRule{}, nil
}
