// Package notification содержит доменную модель уведомлений Alem Community Hub.
package notification

import (
	"context"
	"errors"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// CHANNEL TYPE
// ══════════════════════════════════════════════════════════════════════════════

// ChannelType определяет тип канала доставки уведомлений.
type ChannelType string

const (
	// ChannelTypeTelegram - доставка через Telegram Bot API.
	ChannelTypeTelegram ChannelType = "telegram"

	// ChannelTypeEmail - доставка по email (на будущее).
	ChannelTypeEmail ChannelType = "email"

	// ChannelTypePush - push-уведомления (на будущее).
	ChannelTypePush ChannelType = "push"

	// ChannelTypeWebhook - доставка через webhook (на будущее).
	ChannelTypeWebhook ChannelType = "webhook"

	// ChannelTypeInApp - уведомления внутри приложения (на будущее).
	ChannelTypeInApp ChannelType = "in_app"
)

// IsValid проверяет корректность типа канала.
func (ct ChannelType) IsValid() bool {
	switch ct {
	case ChannelTypeTelegram, ChannelTypeEmail, ChannelTypePush,
		ChannelTypeWebhook, ChannelTypeInApp:
		return true
	default:
		return false
	}
}

// String возвращает строковое представление типа канала.
func (ct ChannelType) String() string {
	return string(ct)
}

// SupportsRichContent возвращает true, если канал поддерживает форматирование.
func (ct ChannelType) SupportsRichContent() bool {
	switch ct {
	case ChannelTypeTelegram, ChannelTypeEmail:
		return true
	default:
		return false
	}
}

// SupportsInlineButtons возвращает true, если канал поддерживает кнопки.
func (ct ChannelType) SupportsInlineButtons() bool {
	return ct == ChannelTypeTelegram
}

// ══════════════════════════════════════════════════════════════════════════════
// DELIVERY RESULT
// ══════════════════════════════════════════════════════════════════════════════

// DeliveryResult представляет результат доставки уведомления.
type DeliveryResult struct {
	// Success - успешно ли доставлено.
	Success bool

	// MessageID - ID отправленного сообщения (для Telegram).
	MessageID string

	// Channel - канал, через который было отправлено.
	Channel ChannelType

	// DeliveredAt - время доставки.
	DeliveredAt time.Time

	// Error - ошибка доставки (если Success = false).
	Error error

	// ErrorCode - код ошибки (если есть).
	ErrorCode string

	// Retryable - можно ли повторить отправку.
	Retryable bool

	// RetryAfter - через сколько можно повторить (для rate limiting).
	RetryAfter time.Duration

	// Metadata - дополнительные данные от канала.
	Metadata map[string]string
}

// NewSuccessResult создаёт результат успешной доставки.
func NewSuccessResult(channel ChannelType, messageID string) DeliveryResult {
	return DeliveryResult{
		Success:     true,
		MessageID:   messageID,
		Channel:     channel,
		DeliveredAt: time.Now().UTC(),
		Metadata:    make(map[string]string),
	}
}

// NewFailureResult создаёт результат неудачной доставки.
func NewFailureResult(channel ChannelType, err error, retryable bool) DeliveryResult {
	return DeliveryResult{
		Success:     false,
		Channel:     channel,
		DeliveredAt: time.Now().UTC(),
		Error:       err,
		Retryable:   retryable,
		Metadata:    make(map[string]string),
	}
}

// NewRateLimitedResult создаёт результат с rate limiting.
func NewRateLimitedResult(channel ChannelType, retryAfter time.Duration) DeliveryResult {
	return DeliveryResult{
		Success:     false,
		Channel:     channel,
		DeliveredAt: time.Now().UTC(),
		Error:       ErrRateLimited,
		ErrorCode:   "RATE_LIMITED",
		Retryable:   true,
		RetryAfter:  retryAfter,
		Metadata:    make(map[string]string),
	}
}

// SetMetadata устанавливает метаданные результата.
func (dr *DeliveryResult) SetMetadata(key, value string) {
	if dr.Metadata == nil {
		dr.Metadata = make(map[string]string)
	}
	dr.Metadata[key] = value
}

// ══════════════════════════════════════════════════════════════════════════════
// DELIVERY OPTIONS
// ══════════════════════════════════════════════════════════════════════════════

// DeliveryOptions содержит опции для отправки уведомления.
type DeliveryOptions struct {
	// ParseMode - режим парсинга сообщения (HTML, Markdown, MarkdownV2).
	ParseMode string

	// DisableNotification - отправить беззвучно.
	DisableNotification bool

	// DisableWebPagePreview - не показывать превью ссылок.
	DisableWebPagePreview bool

	// ReplyToMessageID - ID сообщения для ответа.
	ReplyToMessageID string

	// InlineKeyboard - inline-клавиатура с кнопками.
	InlineKeyboard [][]InlineButton

	// Timeout - таймаут отправки.
	Timeout time.Duration
}

// DefaultDeliveryOptions возвращает опции по умолчанию.
func DefaultDeliveryOptions() DeliveryOptions {
	return DeliveryOptions{
		ParseMode:             "HTML",
		DisableNotification:   false,
		DisableWebPagePreview: true,
		Timeout:               30 * time.Second,
	}
}

// WithSilent создаёт копию опций с беззвучной отправкой.
func (opts DeliveryOptions) WithSilent() DeliveryOptions {
	opts.DisableNotification = true
	return opts
}

// WithParseMode создаёт копию опций с указанным режимом парсинга.
func (opts DeliveryOptions) WithParseMode(mode string) DeliveryOptions {
	opts.ParseMode = mode
	return opts
}

// WithInlineKeyboard создаёт копию опций с клавиатурой.
func (opts DeliveryOptions) WithInlineKeyboard(keyboard [][]InlineButton) DeliveryOptions {
	opts.InlineKeyboard = keyboard
	return opts
}

// WithTimeout создаёт копию опций с указанным таймаутом.
func (opts DeliveryOptions) WithTimeout(timeout time.Duration) DeliveryOptions {
	opts.Timeout = timeout
	return opts
}

// ══════════════════════════════════════════════════════════════════════════════
// INLINE BUTTON
// ══════════════════════════════════════════════════════════════════════════════

// InlineButton представляет кнопку для inline-клавиатуры.
type InlineButton struct {
	// Text - текст на кнопке.
	Text string

	// CallbackData - данные для callback (до 64 байт).
	CallbackData string

	// URL - ссылка (если кнопка-ссылка).
	URL string

	// SwitchInlineQuery - запрос для inline-режима.
	SwitchInlineQuery string
}

// NewCallbackButton создаёт кнопку с callback.
func NewCallbackButton(text, callbackData string) InlineButton {
	return InlineButton{
		Text:         text,
		CallbackData: callbackData,
	}
}

// NewURLButton создаёт кнопку-ссылку.
func NewURLButton(text, url string) InlineButton {
	return InlineButton{
		Text: text,
		URL:  url,
	}
}

// IsValid проверяет корректность кнопки.
func (b InlineButton) IsValid() bool {
	if b.Text == "" {
		return false
	}
	// Должен быть хотя бы один тип действия
	return b.CallbackData != "" || b.URL != "" || b.SwitchInlineQuery != ""
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION CHANNEL INTERFACE
// ══════════════════════════════════════════════════════════════════════════════

// NotificationChannel определяет интерфейс канала доставки уведомлений.
// Это абстракция над конкретными системами доставки (Telegram, Email и т.д.).
type NotificationChannel interface {
	// Type возвращает тип канала.
	Type() ChannelType

	// Send отправляет уведомление.
	// ctx используется для отмены и таймаутов.
	// notification содержит данные уведомления.
	// opts содержит опции доставки.
	Send(ctx context.Context, notification *Notification, opts DeliveryOptions) DeliveryResult

	// SendBatch отправляет группу уведомлений одному получателю.
	// Используется для объединения низкоприоритетных уведомлений.
	SendBatch(ctx context.Context, batch *NotificationBatch, opts DeliveryOptions) DeliveryResult

	// IsAvailable проверяет доступность канала.
	IsAvailable(ctx context.Context) bool

	// SupportsRecipient проверяет, поддерживается ли получатель.
	// Например, Telegram канал требует TelegramChatID.
	SupportsRecipient(notification *Notification) bool
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION SENDER (Aggregate service interface)
// ══════════════════════════════════════════════════════════════════════════════

// NotificationSender определяет интерфейс для отправки уведомлений.
// Это высокоуровневый интерфейс, который выбирает подходящий канал.
type NotificationSender interface {
	// Send отправляет одно уведомление через подходящий канал.
	Send(ctx context.Context, notification *Notification) DeliveryResult

	// SendBatch отправляет батч уведомлений.
	SendBatch(ctx context.Context, batch *NotificationBatch) DeliveryResult

	// RegisterChannel регистрирует канал доставки.
	RegisterChannel(channel NotificationChannel)

	// GetChannel возвращает канал по типу.
	GetChannel(channelType ChannelType) (NotificationChannel, bool)

	// GetAvailableChannels возвращает список доступных каналов.
	GetAvailableChannels(ctx context.Context) []ChannelType
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION REPOSITORY
// ══════════════════════════════════════════════════════════════════════════════

// NotificationRepository определяет интерфейс для хранения уведомлений.
type NotificationRepository interface {
	// Save сохраняет уведомление.
	Save(ctx context.Context, notification *Notification) error

	// GetByID возвращает уведомление по ID.
	GetByID(ctx context.Context, id NotificationID) (*Notification, error)

	// GetPending возвращает уведомления, ожидающие отправки.
	GetPending(ctx context.Context, limit int) ([]*Notification, error)

	// GetByRecipient возвращает уведомления получателя.
	GetByRecipient(ctx context.Context, recipientID RecipientID, limit int) ([]*Notification, error)

	// GetByStatus возвращает уведомления с указанным статусом.
	GetByStatus(ctx context.Context, status NotificationStatus, limit int) ([]*Notification, error)

	// GetFailedForRetry возвращает неудачные уведомления для повторной отправки.
	GetFailedForRetry(ctx context.Context, maxRetries int, limit int) ([]*Notification, error)

	// GetExpired возвращает устаревшие уведомления.
	GetExpired(ctx context.Context, limit int) ([]*Notification, error)

	// UpdateStatus обновляет статус уведомления.
	UpdateStatus(ctx context.Context, id NotificationID, status NotificationStatus) error

	// Delete удаляет уведомление.
	Delete(ctx context.Context, id NotificationID) error

	// DeleteOlderThan удаляет уведомления старше указанной даты.
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)

	// CountByRecipient возвращает количество уведомлений получателя за период.
	CountByRecipient(ctx context.Context, recipientID RecipientID, since time.Time) (int, error)

	// CountByType возвращает количество уведомлений определённого типа за период.
	CountByType(ctx context.Context, notificationType NotificationType, since time.Time) (int, error)
}

// ══════════════════════════════════════════════════════════════════════════════
// TRIGGER RULE REPOSITORY
// ══════════════════════════════════════════════════════════════════════════════

// TriggerRuleRepository определяет интерфейс для хранения правил триггеров.
type TriggerRuleRepository interface {
	// Save сохраняет правило.
	Save(ctx context.Context, rule *TriggerRule) error

	// GetByID возвращает правило по ID.
	GetByID(ctx context.Context, id TriggerRuleID) (*TriggerRule, error)

	// GetAll возвращает все правила.
	GetAll(ctx context.Context) ([]*TriggerRule, error)

	// GetEnabled возвращает активные правила.
	GetEnabled(ctx context.Context) ([]*TriggerRule, error)

	// GetByNotificationType возвращает правила для типа уведомления.
	GetByNotificationType(ctx context.Context, notificationType NotificationType) ([]*TriggerRule, error)

	// GetByConditionType возвращает правила с определённым типом условия.
	GetByConditionType(ctx context.Context, conditionType ConditionType) ([]*TriggerRule, error)

	// GetByTag возвращает правила с указанным тегом.
	GetByTag(ctx context.Context, tag string) ([]*TriggerRule, error)

	// Update обновляет правило.
	Update(ctx context.Context, rule *TriggerRule) error

	// Delete удаляет правило.
	Delete(ctx context.Context, id TriggerRuleID) error

	// Enable активирует правило.
	Enable(ctx context.Context, id TriggerRuleID) error

	// Disable деактивирует правило.
	Disable(ctx context.Context, id TriggerRuleID) error
}

// ══════════════════════════════════════════════════════════════════════════════
// TRIGGER HISTORY REPOSITORY
// ══════════════════════════════════════════════════════════════════════════════

// TriggerHistoryEntry представляет запись истории срабатывания триггера.
type TriggerHistoryEntry struct {
	// ID - уникальный идентификатор записи.
	ID string

	// RuleID - ID правила, которое сработало.
	RuleID TriggerRuleID

	// RecipientID - ID получателя.
	RecipientID RecipientID

	// NotificationID - ID созданного уведомления.
	NotificationID NotificationID

	// TriggeredAt - время срабатывания.
	TriggeredAt time.Time

	// Context - контекст срабатывания (сериализованный).
	Context map[string]interface{}
}

// TriggerHistoryRepository определяет интерфейс для истории срабатываний.
type TriggerHistoryRepository interface {
	// Save сохраняет запись истории.
	Save(ctx context.Context, entry *TriggerHistoryEntry) error

	// GetLastTriggered возвращает время последнего срабатывания правила для получателя.
	GetLastTriggered(ctx context.Context, ruleID TriggerRuleID, recipientID RecipientID) (*time.Time, error)

	// CountTriggers возвращает количество срабатываний за период.
	CountTriggers(ctx context.Context, ruleID TriggerRuleID, recipientID RecipientID, since time.Time) (int, error)

	// GetHistory возвращает историю срабатываний для получателя.
	GetHistory(ctx context.Context, recipientID RecipientID, limit int) ([]*TriggerHistoryEntry, error)

	// DeleteOlderThan удаляет старые записи.
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
}

// ══════════════════════════════════════════════════════════════════════════════
// MESSAGE FORMATTER
// ══════════════════════════════════════════════════════════════════════════════

// MessageFormatter определяет интерфейс для форматирования сообщений.
type MessageFormatter interface {
	// Format форматирует сообщение из шаблона и данных.
	Format(template string, data NotificationData) (string, error)

	// FormatTitle форматирует заголовок.
	FormatTitle(template string, data NotificationData) (string, error)

	// SupportedFormat возвращает поддерживаемый формат (HTML, Markdown).
	SupportedFormat() string
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION SERVICE (Domain Service Interface)
// ══════════════════════════════════════════════════════════════════════════════

// NotificationService определяет интерфейс доменного сервиса уведомлений.
type NotificationService interface {
	// CreateNotification создаёт уведомление на основе правила и контекста.
	CreateNotification(ctx context.Context, rule *TriggerRule, triggerCtx *TriggerContext) (*Notification, error)

	// ScheduleNotification планирует отправку уведомления.
	ScheduleNotification(ctx context.Context, notification *Notification) error

	// CancelNotification отменяет запланированное уведомление.
	CancelNotification(ctx context.Context, id NotificationID) error

	// ProcessPendingNotifications обрабатывает очередь уведомлений.
	ProcessPendingNotifications(ctx context.Context, batchSize int) (processed int, err error)

	// ProcessExpiredNotifications обрабатывает устаревшие уведомления.
	ProcessExpiredNotifications(ctx context.Context) (expired int, err error)

	// RetryFailedNotifications повторяет неудачные уведомления.
	RetryFailedNotifications(ctx context.Context, batchSize int) (retried int, err error)

	// EvaluateTriggers вычисляет все активные триггеры для события.
	EvaluateTriggers(ctx context.Context, triggerCtx *TriggerContext) ([]*TriggerRule, error)
}

// ══════════════════════════════════════════════════════════════════════════════
// CHANNEL ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrChannelUnavailable - канал недоступен.
	ErrChannelUnavailable = errors.New("notification channel is unavailable")

	// ErrChannelNotFound - канал не найден.
	ErrChannelNotFound = errors.New("notification channel not found")

	// ErrUnsupportedRecipient - получатель не поддерживается каналом.
	ErrUnsupportedRecipient = errors.New("recipient not supported by channel")

	// ErrDeliveryFailed - доставка не удалась.
	ErrDeliveryFailed = errors.New("notification delivery failed")

	// ErrRateLimited - превышен лимит запросов к каналу.
	ErrRateLimited = errors.New("rate limited by channel")

	// ErrInvalidMessage - невалидное сообщение.
	ErrInvalidMessage = errors.New("invalid notification message")

	// ErrRecipientBlocked - получатель заблокировал бота.
	ErrRecipientBlocked = errors.New("recipient has blocked the bot")

	// ErrChatNotFound - чат не найден.
	ErrChatNotFound = errors.New("chat not found")

	// ErrMessageTooLong - сообщение слишком длинное.
	ErrMessageTooLong = errors.New("message is too long")

	// ErrTemplateError - ошибка в шаблоне сообщения.
	ErrTemplateError = errors.New("message template error")

	// ErrTimeout - таймаут при отправке.
	ErrTimeout = errors.New("delivery timeout")
)
