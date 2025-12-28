// Package notification содержит доменную модель уведомлений Alem Community Hub.
// Уведомления — это главный инструмент вовлечения студентов и создания чувства общности.
// Философия: уведомления должны мотивировать, информировать и объединять, а не раздражать.
package notification

import (
	"errors"
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// VALUE OBJECTS
// ══════════════════════════════════════════════════════════════════════════════

// NotificationID представляет уникальный идентификатор уведомления.
type NotificationID string

// IsValid проверяет, что ID не пустой.
func (id NotificationID) IsValid() bool {
	return len(id) > 0
}

// String возвращает строковое представление ID.
func (id NotificationID) String() string {
	return string(id)
}

// RecipientID представляет идентификатор получателя уведомления.
type RecipientID string

// IsValid проверяет, что ID получателя не пустой.
func (id RecipientID) IsValid() bool {
	return len(id) > 0
}

// String возвращает строковое представление ID получателя.
func (id RecipientID) String() string {
	return string(id)
}

// TelegramChatID представляет ID чата в Telegram.
type TelegramChatID int64

// IsValid проверяет, что ID чата положительный.
func (id TelegramChatID) IsValid() bool {
	return id > 0
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION TYPE
// ══════════════════════════════════════════════════════════════════════════════

// NotificationType определяет тип уведомления.
type NotificationType string

const (
	// NotificationTypeRankUp - студент поднялся в рейтинге.
	// "🚀 Ты поднялся на 5 мест! Теперь ты #42"
	NotificationTypeRankUp NotificationType = "rank_up"

	// NotificationTypeRankDown - студента обогнали.
	// "⚡ @arman обогнал тебя! Ты теперь #43"
	NotificationTypeRankDown NotificationType = "rank_down"

	// NotificationTypeEnteredTop - студент вошёл в топ (10, 50, 100).
	// "🏆 Поздравляем! Ты вошёл в топ-50!"
	NotificationTypeEnteredTop NotificationType = "entered_top"

	// NotificationTypeLeftTop - студент выпал из топа.
	// "📉 Ты выпал из топ-50. Ещё немного усилий!"
	NotificationTypeLeftTop NotificationType = "left_top"

	// NotificationTypeHelpRequest - кто-то просит помощь по задаче, которую студент решил.
	// "🆘 @dana просит помощь по graph-01. Ты уже решил эту задачу!"
	NotificationTypeHelpRequest NotificationType = "help_request"

	// NotificationTypeHelpOffer - кто-то предлагает помощь.
	// "🤝 @arman готов помочь тебе с задачей sort-01"
	NotificationTypeHelpOffer NotificationType = "help_offer"

	// NotificationTypeDailyDigest - ежедневная сводка прогресса.
	// "📊 Твой день: +150 XP, 3 задачи, #42 в рейтинге"
	NotificationTypeDailyDigest NotificationType = "daily_digest"

	// NotificationTypeWeeklyDigest - еженедельный отчёт.
	// "📈 Твоя неделя: +1200 XP, поднялся на 15 мест!"
	NotificationTypeWeeklyDigest NotificationType = "weekly_digest"

	// NotificationTypeInactivityReminder - напоминание о неактивности.
	// "👋 Давно тебя не видели! Задачи ждут"
	NotificationTypeInactivityReminder NotificationType = "inactivity_reminder"

	// NotificationTypeStreakReminder - напоминание о серии активных дней.
	// "🔥 Не потеряй свою серию в 7 дней! Осталось 3 часа"
	NotificationTypeStreakReminder NotificationType = "streak_reminder"

	// NotificationTypeStreakBroken - серия активных дней прервана.
	// "💔 Твоя серия в 12 дней прервалась. Начни новую!"
	NotificationTypeStreakBroken NotificationType = "streak_broken"

	// NotificationTypeStreakMilestone - достигнут milestone серии.
	// "🎯 Серия 7 дней! Так держать!"
	NotificationTypeStreakMilestone NotificationType = "streak_milestone"

	// NotificationTypeAchievement - получено достижение.
	// "🏅 Новое достижение: Первые шаги!"
	NotificationTypeAchievement NotificationType = "achievement"

	// NotificationTypeLevelUp - повышение уровня.
	// "⬆️ Уровень повышен! Теперь ты Level 5"
	NotificationTypeLevelUp NotificationType = "level_up"

	// NotificationTypeNewNeighbor - появился новый сосед по рангу.
	// "👥 Новый сосед: @max теперь рядом с тобой в рейтинге"
	NotificationTypeNewNeighbor NotificationType = "new_neighbor"

	// NotificationTypeBuddyOnline - study buddy вошёл в систему.
	// "🟢 Твой study buddy @dana онлайн!"
	NotificationTypeBuddyOnline NotificationType = "buddy_online"

	// NotificationTypeEncouragement - мотивационное сообщение.
	// "💪 Ты почти догнал @arman! Всего 50 XP!"
	NotificationTypeEncouragement NotificationType = "encouragement"

	// NotificationTypeWelcome - приветственное сообщение для нового пользователя.
	// "👋 Добро пожаловать в Alem Community Hub!"
	NotificationTypeWelcome NotificationType = "welcome"

	// NotificationTypeSystemAlert - системное уведомление.
	// "⚙️ Плановые работы 25 декабря с 03:00 до 05:00"
	NotificationTypeSystemAlert NotificationType = "system_alert"

	// NotificationTypeTaskCompleted - задача выполнена (подтверждение).
	// "✅ Задача graph-01 засчитана! +100 XP"
	NotificationTypeTaskCompleted NotificationType = "task_completed"

	// NotificationTypeEndorsementReceived - получена благодарность за помощь.
	// "⭐ @dana поблагодарил тебя за помощь!"
	NotificationTypeEndorsementReceived NotificationType = "endorsement_received"
)

// IsValid проверяет, что тип уведомления корректен.
func (t NotificationType) IsValid() bool {
	switch t {
	case NotificationTypeRankUp,
		NotificationTypeRankDown,
		NotificationTypeEnteredTop,
		NotificationTypeLeftTop,
		NotificationTypeHelpRequest,
		NotificationTypeHelpOffer,
		NotificationTypeDailyDigest,
		NotificationTypeWeeklyDigest,
		NotificationTypeInactivityReminder,
		NotificationTypeStreakReminder,
		NotificationTypeStreakBroken,
		NotificationTypeStreakMilestone,
		NotificationTypeAchievement,
		NotificationTypeLevelUp,
		NotificationTypeNewNeighbor,
		NotificationTypeBuddyOnline,
		NotificationTypeEncouragement,
		NotificationTypeWelcome,
		NotificationTypeSystemAlert,
		NotificationTypeTaskCompleted,
		NotificationTypeEndorsementReceived:
		return true
	default:
		return false
	}
}

// Category возвращает категорию уведомления для группировки.
func (t NotificationType) Category() NotificationCategory {
	switch t {
	case NotificationTypeRankUp, NotificationTypeRankDown,
		NotificationTypeEnteredTop, NotificationTypeLeftTop:
		return CategoryRanking

	case NotificationTypeHelpRequest, NotificationTypeHelpOffer,
		NotificationTypeEndorsementReceived:
		return CategorySocial

	case NotificationTypeDailyDigest, NotificationTypeWeeklyDigest:
		return CategoryDigest

	case NotificationTypeStreakReminder, NotificationTypeStreakBroken,
		NotificationTypeStreakMilestone, NotificationTypeInactivityReminder:
		return CategoryMotivation

	case NotificationTypeAchievement, NotificationTypeLevelUp,
		NotificationTypeTaskCompleted:
		return CategoryProgress

	case NotificationTypeNewNeighbor, NotificationTypeBuddyOnline:
		return CategoryCommunity

	case NotificationTypeWelcome, NotificationTypeSystemAlert:
		return CategorySystem

	case NotificationTypeEncouragement:
		return CategoryMotivation

	default:
		return CategorySystem
	}
}

// DefaultPriority возвращает приоритет по умолчанию для данного типа.
func (t NotificationType) DefaultPriority() Priority {
	switch t {
	case NotificationTypeWelcome, NotificationTypeAchievement,
		NotificationTypeEnteredTop, NotificationTypeLevelUp:
		return PriorityHigh

	case NotificationTypeRankUp, NotificationTypeRankDown,
		NotificationTypeHelpRequest, NotificationTypeTaskCompleted,
		NotificationTypeEndorsementReceived:
		return PriorityNormal

	case NotificationTypeDailyDigest, NotificationTypeWeeklyDigest,
		NotificationTypeStreakReminder, NotificationTypeEncouragement,
		NotificationTypeBuddyOnline, NotificationTypeNewNeighbor:
		return PriorityLow

	case NotificationTypeInactivityReminder, NotificationTypeStreakBroken,
		NotificationTypeLeftTop:
		return PriorityNormal

	case NotificationTypeSystemAlert:
		return PriorityUrgent

	default:
		return PriorityNormal
	}
}

// Emoji возвращает эмодзи для данного типа уведомления.
func (t NotificationType) Emoji() string {
	switch t {
	case NotificationTypeRankUp:
		return "🚀"
	case NotificationTypeRankDown:
		return "⚡"
	case NotificationTypeEnteredTop:
		return "🏆"
	case NotificationTypeLeftTop:
		return "📉"
	case NotificationTypeHelpRequest:
		return "🆘"
	case NotificationTypeHelpOffer:
		return "🤝"
	case NotificationTypeDailyDigest:
		return "📊"
	case NotificationTypeWeeklyDigest:
		return "📈"
	case NotificationTypeInactivityReminder:
		return "👋"
	case NotificationTypeStreakReminder:
		return "🔥"
	case NotificationTypeStreakBroken:
		return "💔"
	case NotificationTypeStreakMilestone:
		return "🎯"
	case NotificationTypeAchievement:
		return "🏅"
	case NotificationTypeLevelUp:
		return "⬆️"
	case NotificationTypeNewNeighbor:
		return "👥"
	case NotificationTypeBuddyOnline:
		return "🟢"
	case NotificationTypeEncouragement:
		return "💪"
	case NotificationTypeWelcome:
		return "👋"
	case NotificationTypeSystemAlert:
		return "⚙️"
	case NotificationTypeTaskCompleted:
		return "✅"
	case NotificationTypeEndorsementReceived:
		return "⭐"
	default:
		return "📬"
	}
}

// String возвращает строковое представление типа.
func (t NotificationType) String() string {
	return string(t)
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION CATEGORY
// ══════════════════════════════════════════════════════════════════════════════

// NotificationCategory определяет категорию уведомления для группировки и фильтрации.
type NotificationCategory string

const (
	// CategoryRanking - уведомления о рейтинге.
	CategoryRanking NotificationCategory = "ranking"

	// CategorySocial - социальные уведомления (помощь, благодарности).
	CategorySocial NotificationCategory = "social"

	// CategoryDigest - дайджесты и сводки.
	CategoryDigest NotificationCategory = "digest"

	// CategoryMotivation - мотивационные уведомления.
	CategoryMotivation NotificationCategory = "motivation"

	// CategoryProgress - уведомления о прогрессе.
	CategoryProgress NotificationCategory = "progress"

	// CategoryCommunity - уведомления о сообществе.
	CategoryCommunity NotificationCategory = "community"

	// CategorySystem - системные уведомления.
	CategorySystem NotificationCategory = "system"
)

// IsValid проверяет корректность категории.
func (c NotificationCategory) IsValid() bool {
	switch c {
	case CategoryRanking, CategorySocial, CategoryDigest,
		CategoryMotivation, CategoryProgress, CategoryCommunity, CategorySystem:
		return true
	default:
		return false
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// PRIORITY
// ══════════════════════════════════════════════════════════════════════════════

// Priority определяет приоритет уведомления.
type Priority int

const (
	// PriorityLow - низкий приоритет (можно отложить, объединить с другими).
	PriorityLow Priority = 1

	// PriorityNormal - обычный приоритет.
	PriorityNormal Priority = 2

	// PriorityHigh - высокий приоритет (важное уведомление).
	PriorityHigh Priority = 3

	// PriorityUrgent - срочное уведомление (отправляется немедленно).
	PriorityUrgent Priority = 4
)

// IsValid проверяет корректность приоритета.
func (p Priority) IsValid() bool {
	return p >= PriorityLow && p <= PriorityUrgent
}

// String возвращает строковое представление приоритета.
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityUrgent:
		return "urgent"
	default:
		return "unknown"
	}
}

// ShouldSendImmediately возвращает true, если уведомление нужно отправить сразу.
func (p Priority) ShouldSendImmediately() bool {
	return p >= PriorityHigh
}

// CanBeBatched возвращает true, если уведомление можно объединить с другими.
func (p Priority) CanBeBatched() bool {
	return p == PriorityLow
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION STATUS
// ══════════════════════════════════════════════════════════════════════════════

// NotificationStatus определяет статус доставки уведомления.
type NotificationStatus string

const (
	// StatusPending - уведомление ожидает отправки.
	StatusPending NotificationStatus = "pending"

	// StatusQueued - уведомление в очереди на отправку.
	StatusQueued NotificationStatus = "queued"

	// StatusSending - уведомление отправляется.
	StatusSending NotificationStatus = "sending"

	// StatusDelivered - уведомление доставлено.
	StatusDelivered NotificationStatus = "delivered"

	// StatusFailed - доставка не удалась.
	StatusFailed NotificationStatus = "failed"

	// StatusCancelled - уведомление отменено.
	StatusCancelled NotificationStatus = "cancelled"

	// StatusExpired - уведомление устарело и не было отправлено.
	StatusExpired NotificationStatus = "expired"

	// StatusSkipped - уведомление пропущено (например, quiet hours).
	StatusSkipped NotificationStatus = "skipped"
)

// IsValid проверяет корректность статуса.
func (s NotificationStatus) IsValid() bool {
	switch s {
	case StatusPending, StatusQueued, StatusSending,
		StatusDelivered, StatusFailed, StatusCancelled,
		StatusExpired, StatusSkipped:
		return true
	default:
		return false
	}
}

// IsFinal возвращает true, если это конечный статус.
func (s NotificationStatus) IsFinal() bool {
	switch s {
	case StatusDelivered, StatusFailed, StatusCancelled, StatusExpired, StatusSkipped:
		return true
	default:
		return false
	}
}

// IsSuccess возвращает true, если уведомление успешно доставлено.
func (s NotificationStatus) IsSuccess() bool {
	return s == StatusDelivered
}

// CanRetry возвращает true, если можно повторить отправку.
func (s NotificationStatus) CanRetry() bool {
	return s == StatusFailed
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION ENTITY
// ══════════════════════════════════════════════════════════════════════════════

// Notification представляет уведомление, отправляемое студенту.
type Notification struct {
	// ID - уникальный идентификатор уведомления.
	ID NotificationID

	// Type - тип уведомления.
	Type NotificationType

	// RecipientID - ID получателя (студента).
	RecipientID RecipientID

	// TelegramChatID - ID чата в Telegram для отправки.
	TelegramChatID TelegramChatID

	// Priority - приоритет уведомления.
	Priority Priority

	// Status - текущий статус доставки.
	Status NotificationStatus

	// Title - заголовок уведомления (опционально).
	Title string

	// Message - текст уведомления.
	Message string

	// Data - дополнительные данные для форматирования.
	Data NotificationData

	// ScheduledAt - запланированное время отправки (nil = сразу).
	ScheduledAt *time.Time

	// SentAt - фактическое время отправки.
	SentAt *time.Time

	// DeliveredAt - время доставки.
	DeliveredAt *time.Time

	// ExpiresAt - время истечения (после которого не отправлять).
	ExpiresAt *time.Time

	// RetryCount - количество попыток отправки.
	RetryCount int

	// MaxRetries - максимальное количество попыток.
	MaxRetries int

	// LastError - последняя ошибка доставки.
	LastError string

	// Metadata - произвольные метаданные.
	Metadata map[string]string

	// CreatedAt - время создания.
	CreatedAt time.Time

	// UpdatedAt - время последнего обновления.
	UpdatedAt time.Time
}

// NotificationData содержит типизированные данные для разных типов уведомлений.
type NotificationData struct {
	// Rank-related
	OldRank        int    `json:"old_rank,omitempty"`
	NewRank        int    `json:"new_rank,omitempty"`
	RankChange     int    `json:"rank_change,omitempty"`
	TopNumber      int    `json:"top_number,omitempty"` // 10, 50, 100
	CompetitorName string `json:"competitor_name,omitempty"`
	CompetitorID   string `json:"competitor_id,omitempty"`

	// XP-related
	XPGained int `json:"xp_gained,omitempty"`
	TotalXP  int `json:"total_xp,omitempty"`
	XPToNext int `json:"xp_to_next,omitempty"`

	// Level-related
	OldLevel int `json:"old_level,omitempty"`
	NewLevel int `json:"new_level,omitempty"`

	// Task-related
	TaskID   string `json:"task_id,omitempty"`
	TaskName string `json:"task_name,omitempty"`

	// Streak-related
	StreakDays     int `json:"streak_days,omitempty"`
	BestStreak     int `json:"best_streak,omitempty"`
	HoursRemaining int `json:"hours_remaining,omitempty"`

	// Social-related
	HelperID      string  `json:"helper_id,omitempty"`
	HelperName    string  `json:"helper_name,omitempty"`
	RequesterID   string  `json:"requester_id,omitempty"`
	RequesterName string  `json:"requester_name,omitempty"`
	HelpRating    float64 `json:"help_rating,omitempty"`

	// Achievement-related
	AchievementID   string `json:"achievement_id,omitempty"`
	AchievementName string `json:"achievement_name,omitempty"`
	AchievementDesc string `json:"achievement_desc,omitempty"`

	// Digest-related
	TasksCompleted int        `json:"tasks_completed,omitempty"`
	DaysActive     int        `json:"days_active,omitempty"`
	RankProgress   int        `json:"rank_progress,omitempty"`
	PeriodStart    *time.Time `json:"period_start,omitempty"`
	PeriodEnd      *time.Time `json:"period_end,omitempty"`

	// Inactivity-related
	DaysInactive int `json:"days_inactive,omitempty"`

	// Buddy-related
	BuddyID   string `json:"buddy_id,omitempty"`
	BuddyName string `json:"buddy_name,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// FACTORY & VALIDATION
// ══════════════════════════════════════════════════════════════════════════════

// NewNotificationParams содержит параметры для создания уведомления.
type NewNotificationParams struct {
	ID             NotificationID
	Type           NotificationType
	RecipientID    RecipientID
	TelegramChatID TelegramChatID
	Message        string
	Title          string
	Data           NotificationData
	Priority       *Priority
	ScheduledAt    *time.Time
	ExpiresAt      *time.Time
	MaxRetries     int
}

// NewNotification создаёт новое уведомление с валидацией.
func NewNotification(params NewNotificationParams) (*Notification, error) {
	if !params.ID.IsValid() {
		return nil, ErrInvalidNotificationID
	}

	if !params.Type.IsValid() {
		return nil, ErrInvalidNotificationType
	}

	if !params.RecipientID.IsValid() {
		return nil, ErrInvalidRecipientID
	}

	if !params.TelegramChatID.IsValid() {
		return nil, ErrInvalidTelegramChatID
	}

	if params.Message == "" {
		return nil, ErrEmptyMessage
	}

	priority := params.Type.DefaultPriority()
	if params.Priority != nil && params.Priority.IsValid() {
		priority = *params.Priority
	}

	maxRetries := 3
	if params.MaxRetries > 0 {
		maxRetries = params.MaxRetries
	}

	now := time.Now().UTC()

	return &Notification{
		ID:             params.ID,
		Type:           params.Type,
		RecipientID:    params.RecipientID,
		TelegramChatID: params.TelegramChatID,
		Priority:       priority,
		Status:         StatusPending,
		Title:          params.Title,
		Message:        params.Message,
		Data:           params.Data,
		ScheduledAt:    params.ScheduledAt,
		ExpiresAt:      params.ExpiresAt,
		RetryCount:     0,
		MaxRetries:     maxRetries,
		Metadata:       make(map[string]string),
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// DOMAIN METHODS
// ══════════════════════════════════════════════════════════════════════════════

// Category возвращает категорию уведомления.
func (n *Notification) Category() NotificationCategory {
	return n.Type.Category()
}

// MarkQueued переводит уведомление в статус "в очереди".
func (n *Notification) MarkQueued() error {
	if n.Status != StatusPending {
		return ErrInvalidStatusTransition
	}
	n.Status = StatusQueued
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkSending переводит уведомление в статус "отправляется".
func (n *Notification) MarkSending() error {
	if n.Status != StatusQueued && n.Status != StatusPending {
		return ErrInvalidStatusTransition
	}
	n.Status = StatusSending
	now := time.Now().UTC()
	n.SentAt = &now
	n.UpdatedAt = now
	return nil
}

// MarkDelivered помечает уведомление как доставленное.
func (n *Notification) MarkDelivered() error {
	if n.Status != StatusSending {
		return ErrInvalidStatusTransition
	}
	n.Status = StatusDelivered
	now := time.Now().UTC()
	n.DeliveredAt = &now
	n.UpdatedAt = now
	return nil
}

// MarkFailed помечает уведомление как неудачное.
func (n *Notification) MarkFailed(err string) error {
	if n.Status != StatusSending {
		return ErrInvalidStatusTransition
	}
	n.Status = StatusFailed
	n.LastError = err
	n.RetryCount++
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkCancelled отменяет уведомление.
func (n *Notification) MarkCancelled() error {
	if n.Status.IsFinal() {
		return ErrInvalidStatusTransition
	}
	n.Status = StatusCancelled
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkExpired помечает уведомление как устаревшее.
func (n *Notification) MarkExpired() error {
	if n.Status.IsFinal() {
		return ErrInvalidStatusTransition
	}
	n.Status = StatusExpired
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkSkipped помечает уведомление как пропущенное.
func (n *Notification) MarkSkipped(reason string) error {
	if n.Status.IsFinal() {
		return ErrInvalidStatusTransition
	}
	n.Status = StatusSkipped
	n.LastError = reason
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// ResetForRetry подготавливает уведомление для повторной отправки.
func (n *Notification) ResetForRetry() error {
	if !n.CanRetry() {
		return ErrMaxRetriesExceeded
	}
	n.Status = StatusPending
	n.SentAt = nil
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// CanRetry возвращает true, если можно повторить отправку.
func (n *Notification) CanRetry() bool {
	return n.Status.CanRetry() && n.RetryCount < n.MaxRetries
}

// IsExpired проверяет, истекло ли время жизни уведомления.
func (n *Notification) IsExpired() bool {
	if n.ExpiresAt == nil {
		return false
	}
	return time.Now().UTC().After(*n.ExpiresAt)
}

// IsScheduled возвращает true, если уведомление запланировано на будущее.
func (n *Notification) IsScheduled() bool {
	if n.ScheduledAt == nil {
		return false
	}
	return n.ScheduledAt.After(time.Now().UTC())
}

// IsReadyToSend возвращает true, если уведомление готово к отправке.
func (n *Notification) IsReadyToSend() bool {
	if n.Status != StatusPending && n.Status != StatusQueued {
		return false
	}
	if n.IsExpired() {
		return false
	}
	if n.IsScheduled() {
		return false
	}
	return true
}

// SetMetadata устанавливает значение метаданных.
func (n *Notification) SetMetadata(key, value string) {
	if n.Metadata == nil {
		n.Metadata = make(map[string]string)
	}
	n.Metadata[key] = value
	n.UpdatedAt = time.Now().UTC()
}

// GetMetadata возвращает значение метаданных.
func (n *Notification) GetMetadata(key string) (string, bool) {
	if n.Metadata == nil {
		return "", false
	}
	value, ok := n.Metadata[key]
	return value, ok
}

// Clone создаёт глубокую копию уведомления.
func (n *Notification) Clone() *Notification {
	if n == nil {
		return nil
	}

	clone := *n

	// Копируем указатели
	if n.ScheduledAt != nil {
		t := *n.ScheduledAt
		clone.ScheduledAt = &t
	}
	if n.SentAt != nil {
		t := *n.SentAt
		clone.SentAt = &t
	}
	if n.DeliveredAt != nil {
		t := *n.DeliveredAt
		clone.DeliveredAt = &t
	}
	if n.ExpiresAt != nil {
		t := *n.ExpiresAt
		clone.ExpiresAt = &t
	}

	// Копируем map
	if n.Metadata != nil {
		clone.Metadata = make(map[string]string, len(n.Metadata))
		for k, v := range n.Metadata {
			clone.Metadata[k] = v
		}
	}

	return &clone
}

// String возвращает строковое представление для логирования.
func (n *Notification) String() string {
	return fmt.Sprintf(
		"Notification{ID: %s, Type: %s, Recipient: %s, Status: %s, Priority: %s}",
		n.ID, n.Type, n.RecipientID, n.Status, n.Priority,
	)
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION BATCH (for grouping low-priority notifications)
// ══════════════════════════════════════════════════════════════════════════════

// NotificationBatch представляет группу уведомлений для одного получателя.
type NotificationBatch struct {
	// RecipientID - получатель батча.
	RecipientID RecipientID

	// TelegramChatID - ID чата в Telegram.
	TelegramChatID TelegramChatID

	// Notifications - список уведомлений в батче.
	Notifications []*Notification

	// CreatedAt - время создания батча.
	CreatedAt time.Time
}

// NewNotificationBatch создаёт новый батч уведомлений.
func NewNotificationBatch(recipientID RecipientID, chatID TelegramChatID) *NotificationBatch {
	return &NotificationBatch{
		RecipientID:    recipientID,
		TelegramChatID: chatID,
		Notifications:  make([]*Notification, 0),
		CreatedAt:      time.Now().UTC(),
	}
}

// Add добавляет уведомление в батч.
func (b *NotificationBatch) Add(n *Notification) error {
	if n == nil {
		return ErrNilNotification
	}
	if n.RecipientID != b.RecipientID {
		return ErrRecipientMismatch
	}
	b.Notifications = append(b.Notifications, n)
	return nil
}

// Count возвращает количество уведомлений в батче.
func (b *NotificationBatch) Count() int {
	return len(b.Notifications)
}

// IsEmpty возвращает true, если батч пуст.
func (b *NotificationBatch) IsEmpty() bool {
	return len(b.Notifications) == 0
}

// HighestPriority возвращает наивысший приоритет в батче.
func (b *NotificationBatch) HighestPriority() Priority {
	if b.IsEmpty() {
		return PriorityLow
	}

	highest := PriorityLow
	for _, n := range b.Notifications {
		if n.Priority > highest {
			highest = n.Priority
		}
	}
	return highest
}

// ══════════════════════════════════════════════════════════════════════════════
// DOMAIN ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrInvalidNotificationID - невалидный ID уведомления.
	ErrInvalidNotificationID = errors.New("invalid notification id: cannot be empty")

	// ErrInvalidNotificationType - невалидный тип уведомления.
	ErrInvalidNotificationType = errors.New("invalid notification type")

	// ErrInvalidRecipientID - невалидный ID получателя.
	ErrInvalidRecipientID = errors.New("invalid recipient id: cannot be empty")

	// ErrInvalidTelegramChatID - невалидный ID чата Telegram.
	ErrInvalidTelegramChatID = errors.New("invalid telegram chat id: must be positive")

	// ErrEmptyMessage - пустое сообщение.
	ErrEmptyMessage = errors.New("notification message cannot be empty")

	// ErrInvalidPriority - невалидный приоритет.
	ErrInvalidPriority = errors.New("invalid priority")

	// ErrInvalidStatusTransition - недопустимый переход статуса.
	ErrInvalidStatusTransition = errors.New("invalid status transition")

	// ErrMaxRetriesExceeded - превышено количество попыток.
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")

	// ErrNotificationExpired - уведомление устарело.
	ErrNotificationExpired = errors.New("notification has expired")

	// ErrNilNotification - nil уведомление.
	ErrNilNotification = errors.New("notification cannot be nil")

	// ErrRecipientMismatch - несоответствие получателя.
	ErrRecipientMismatch = errors.New("notification recipient does not match batch")

	// ErrNotificationNotFound - уведомление не найдено.
	ErrNotificationNotFound = errors.New("notification not found")
)
