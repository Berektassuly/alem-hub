// Package notification содержит доменную модель уведомлений Alem Community Hub.
package notification

import (
	"errors"
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// TRIGGER RULE
// Правила определяют, когда и при каких условиях отправлять уведомления.
// Это позволяет гибко настраивать логику уведомлений без изменения кода.
// ══════════════════════════════════════════════════════════════════════════════

// TriggerRuleID представляет уникальный идентификатор правила.
type TriggerRuleID string

// IsValid проверяет, что ID правила не пустой.
func (id TriggerRuleID) IsValid() bool {
	return len(id) > 0
}

// String возвращает строковое представление ID.
func (id TriggerRuleID) String() string {
	return string(id)
}

// ══════════════════════════════════════════════════════════════════════════════
// CONDITION TYPE
// ══════════════════════════════════════════════════════════════════════════════

// ConditionType определяет тип условия срабатывания.
type ConditionType string

const (
	// ConditionTypeRankChange - изменение позиции в рейтинге.
	ConditionTypeRankChange ConditionType = "rank_change"

	// ConditionTypeXPGained - получение XP.
	ConditionTypeXPGained ConditionType = "xp_gained"

	// ConditionTypeXPThreshold - достижение порога XP.
	ConditionTypeXPThreshold ConditionType = "xp_threshold"

	// ConditionTypeLevelUp - повышение уровня.
	ConditionTypeLevelUp ConditionType = "level_up"

	// ConditionTypeTopEntered - вход в топ (10, 50, 100).
	ConditionTypeTopEntered ConditionType = "top_entered"

	// ConditionTypeTopLeft - выход из топа.
	ConditionTypeTopLeft ConditionType = "top_left"

	// ConditionTypeTaskCompleted - выполнение задачи.
	ConditionTypeTaskCompleted ConditionType = "task_completed"

	// ConditionTypeStreakDays - достижение серии дней.
	ConditionTypeStreakDays ConditionType = "streak_days"

	// ConditionTypeStreakBroken - прерывание серии.
	ConditionTypeStreakBroken ConditionType = "streak_broken"

	// ConditionTypeStreakAtRisk - серия под угрозой (осталось мало времени).
	ConditionTypeStreakAtRisk ConditionType = "streak_at_risk"

	// ConditionTypeInactiveDays - неактивность N дней.
	ConditionTypeInactiveDays ConditionType = "inactive_days"

	// ConditionTypeStudentOnline - студент вошёл в онлайн.
	ConditionTypeStudentOnline ConditionType = "student_online"

	// ConditionTypeStudentOffline - студент вышел из онлайна.
	ConditionTypeStudentOffline ConditionType = "student_offline"

	// ConditionTypeHelpRequested - запрос помощи.
	ConditionTypeHelpRequested ConditionType = "help_requested"

	// ConditionTypeEndorsementReceived - получена благодарность.
	ConditionTypeEndorsementReceived ConditionType = "endorsement_received"

	// ConditionTypeAchievementUnlocked - разблокировано достижение.
	ConditionTypeAchievementUnlocked ConditionType = "achievement_unlocked"

	// ConditionTypeScheduled - запланированное время (для дайджестов).
	ConditionTypeScheduled ConditionType = "scheduled"

	// ConditionTypeCompetitorClose - конкурент близко по XP.
	ConditionTypeCompetitorClose ConditionType = "competitor_close"

	// ConditionTypeNewRegistration - новая регистрация.
	ConditionTypeNewRegistration ConditionType = "new_registration"
)

// IsValid проверяет корректность типа условия.
func (ct ConditionType) IsValid() bool {
	switch ct {
	case ConditionTypeRankChange,
		ConditionTypeXPGained,
		ConditionTypeXPThreshold,
		ConditionTypeLevelUp,
		ConditionTypeTopEntered,
		ConditionTypeTopLeft,
		ConditionTypeTaskCompleted,
		ConditionTypeStreakDays,
		ConditionTypeStreakBroken,
		ConditionTypeStreakAtRisk,
		ConditionTypeInactiveDays,
		ConditionTypeStudentOnline,
		ConditionTypeStudentOffline,
		ConditionTypeHelpRequested,
		ConditionTypeEndorsementReceived,
		ConditionTypeAchievementUnlocked,
		ConditionTypeScheduled,
		ConditionTypeCompetitorClose,
		ConditionTypeNewRegistration:
		return true
	default:
		return false
	}
}

// SuggestedNotificationType возвращает предлагаемый тип уведомления для условия.
func (ct ConditionType) SuggestedNotificationType() NotificationType {
	switch ct {
	case ConditionTypeRankChange:
		return NotificationTypeRankUp // или RankDown, зависит от направления
	case ConditionTypeXPGained, ConditionTypeXPThreshold:
		return NotificationTypeTaskCompleted
	case ConditionTypeLevelUp:
		return NotificationTypeLevelUp
	case ConditionTypeTopEntered:
		return NotificationTypeEnteredTop
	case ConditionTypeTopLeft:
		return NotificationTypeLeftTop
	case ConditionTypeTaskCompleted:
		return NotificationTypeTaskCompleted
	case ConditionTypeStreakDays:
		return NotificationTypeStreakMilestone
	case ConditionTypeStreakBroken:
		return NotificationTypeStreakBroken
	case ConditionTypeStreakAtRisk:
		return NotificationTypeStreakReminder
	case ConditionTypeInactiveDays:
		return NotificationTypeInactivityReminder
	case ConditionTypeStudentOnline:
		return NotificationTypeBuddyOnline
	case ConditionTypeHelpRequested:
		return NotificationTypeHelpRequest
	case ConditionTypeEndorsementReceived:
		return NotificationTypeEndorsementReceived
	case ConditionTypeAchievementUnlocked:
		return NotificationTypeAchievement
	case ConditionTypeScheduled:
		return NotificationTypeDailyDigest
	case ConditionTypeCompetitorClose:
		return NotificationTypeEncouragement
	case ConditionTypeNewRegistration:
		return NotificationTypeWelcome
	default:
		return NotificationTypeSystemAlert
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// COMPARISON OPERATOR
// ══════════════════════════════════════════════════════════════════════════════

// ComparisonOperator определяет оператор сравнения для условий.
type ComparisonOperator string

const (
	// OpEqual - равно.
	OpEqual ComparisonOperator = "eq"

	// OpNotEqual - не равно.
	OpNotEqual ComparisonOperator = "neq"

	// OpGreaterThan - больше.
	OpGreaterThan ComparisonOperator = "gt"

	// OpGreaterOrEqual - больше или равно.
	OpGreaterOrEqual ComparisonOperator = "gte"

	// OpLessThan - меньше.
	OpLessThan ComparisonOperator = "lt"

	// OpLessOrEqual - меньше или равно.
	OpLessOrEqual ComparisonOperator = "lte"

	// OpBetween - между (включительно).
	OpBetween ComparisonOperator = "between"

	// OpIn - входит в список.
	OpIn ComparisonOperator = "in"

	// OpNotIn - не входит в список.
	OpNotIn ComparisonOperator = "not_in"
)

// IsValid проверяет корректность оператора.
func (op ComparisonOperator) IsValid() bool {
	switch op {
	case OpEqual, OpNotEqual, OpGreaterThan, OpGreaterOrEqual,
		OpLessThan, OpLessOrEqual, OpBetween, OpIn, OpNotIn:
		return true
	default:
		return false
	}
}

// Evaluate вычисляет результат сравнения для числовых значений.
func (op ComparisonOperator) Evaluate(actual, expected int) bool {
	switch op {
	case OpEqual:
		return actual == expected
	case OpNotEqual:
		return actual != expected
	case OpGreaterThan:
		return actual > expected
	case OpGreaterOrEqual:
		return actual >= expected
	case OpLessThan:
		return actual < expected
	case OpLessOrEqual:
		return actual <= expected
	default:
		return false
	}
}

// EvaluateRange проверяет попадание в диапазон.
func (op ComparisonOperator) EvaluateRange(actual, min, max int) bool {
	if op == OpBetween {
		return actual >= min && actual <= max
	}
	return false
}

// EvaluateList проверяет вхождение в список.
func (op ComparisonOperator) EvaluateList(actual int, list []int) bool {
	for _, v := range list {
		if actual == v {
			return op == OpIn
		}
	}
	return op == OpNotIn
}

// ══════════════════════════════════════════════════════════════════════════════
// CONDITION
// ══════════════════════════════════════════════════════════════════════════════

// Condition представляет одно условие срабатывания триггера.
type Condition struct {
	// Type - тип условия.
	Type ConditionType

	// Operator - оператор сравнения.
	Operator ComparisonOperator

	// Value - значение для сравнения (основное).
	Value int

	// MinValue - минимальное значение (для OpBetween).
	MinValue int

	// MaxValue - максимальное значение (для OpBetween).
	MaxValue int

	// ListValues - список значений (для OpIn, OpNotIn).
	ListValues []int

	// StringValue - строковое значение (для некоторых условий).
	StringValue string

	// Negate - инвертировать результат.
	Negate bool
}

// NewCondition создаёт новое условие с валидацией.
func NewCondition(condType ConditionType, operator ComparisonOperator, value int) (*Condition, error) {
	if !condType.IsValid() {
		return nil, ErrInvalidConditionType
	}
	if !operator.IsValid() {
		return nil, ErrInvalidOperator
	}

	return &Condition{
		Type:     condType,
		Operator: operator,
		Value:    value,
	}, nil
}

// NewRangeCondition создаёт условие с диапазоном.
func NewRangeCondition(condType ConditionType, min, max int) (*Condition, error) {
	if !condType.IsValid() {
		return nil, ErrInvalidConditionType
	}
	if min > max {
		return nil, ErrInvalidRange
	}

	return &Condition{
		Type:     condType,
		Operator: OpBetween,
		MinValue: min,
		MaxValue: max,
	}, nil
}

// NewListCondition создаёт условие со списком значений.
func NewListCondition(condType ConditionType, operator ComparisonOperator, values []int) (*Condition, error) {
	if !condType.IsValid() {
		return nil, ErrInvalidConditionType
	}
	if operator != OpIn && operator != OpNotIn {
		return nil, ErrInvalidOperator
	}
	if len(values) == 0 {
		return nil, ErrEmptyValueList
	}

	return &Condition{
		Type:       condType,
		Operator:   operator,
		ListValues: values,
	}, nil
}

// Evaluate вычисляет условие для заданного значения.
func (c *Condition) Evaluate(actual int) bool {
	var result bool

	switch c.Operator {
	case OpBetween:
		result = c.Operator.EvaluateRange(actual, c.MinValue, c.MaxValue)
	case OpIn, OpNotIn:
		result = c.Operator.EvaluateList(actual, c.ListValues)
	default:
		result = c.Operator.Evaluate(actual, c.Value)
	}

	if c.Negate {
		return !result
	}
	return result
}

// Clone создаёт копию условия.
func (c *Condition) Clone() *Condition {
	if c == nil {
		return nil
	}

	clone := *c
	if c.ListValues != nil {
		clone.ListValues = make([]int, len(c.ListValues))
		copy(clone.ListValues, c.ListValues)
	}
	return &clone
}

// ══════════════════════════════════════════════════════════════════════════════
// TIME CONSTRAINT
// ══════════════════════════════════════════════════════════════════════════════

// TimeConstraint определяет временные ограничения для триггера.
type TimeConstraint struct {
	// DaysOfWeek - дни недели (0 = воскресенье, 6 = суббота). Пустой = все дни.
	DaysOfWeek []int

	// HoursStart - начало временного окна (0-23).
	HoursStart int

	// HoursEnd - конец временного окна (0-23).
	HoursEnd int

	// Timezone - часовой пояс (например, "Asia/Almaty").
	Timezone string

	// ExcludeQuietHours - исключать тихие часы пользователя.
	ExcludeQuietHours bool
}

// NewTimeConstraint создаёт временное ограничение.
func NewTimeConstraint(hoursStart, hoursEnd int, timezone string) (*TimeConstraint, error) {
	if hoursStart < 0 || hoursStart > 23 {
		return nil, ErrInvalidHours
	}
	if hoursEnd < 0 || hoursEnd > 23 {
		return nil, ErrInvalidHours
	}

	return &TimeConstraint{
		DaysOfWeek:        nil, // все дни
		HoursStart:        hoursStart,
		HoursEnd:          hoursEnd,
		Timezone:          timezone,
		ExcludeQuietHours: true,
	}, nil
}

// IsAllowed проверяет, разрешено ли отправлять уведомление в указанное время.
func (tc *TimeConstraint) IsAllowed(t time.Time) bool {
	// Конвертируем в нужный часовой пояс
	if tc.Timezone != "" {
		loc, err := time.LoadLocation(tc.Timezone)
		if err == nil {
			t = t.In(loc)
		}
	}

	// Проверяем день недели
	if len(tc.DaysOfWeek) > 0 {
		dayAllowed := false
		weekday := int(t.Weekday())
		for _, d := range tc.DaysOfWeek {
			if d == weekday {
				dayAllowed = true
				break
			}
		}
		if !dayAllowed {
			return false
		}
	}

	// Проверяем час
	hour := t.Hour()
	if tc.HoursStart <= tc.HoursEnd {
		// Простой случай: 9:00 - 21:00
		return hour >= tc.HoursStart && hour < tc.HoursEnd
	}
	// Через полночь: 21:00 - 9:00
	return hour >= tc.HoursStart || hour < tc.HoursEnd
}

// Clone создаёт копию ограничения.
func (tc *TimeConstraint) Clone() *TimeConstraint {
	if tc == nil {
		return nil
	}

	clone := *tc
	if tc.DaysOfWeek != nil {
		clone.DaysOfWeek = make([]int, len(tc.DaysOfWeek))
		copy(clone.DaysOfWeek, tc.DaysOfWeek)
	}
	return &clone
}

// ══════════════════════════════════════════════════════════════════════════════
// RATE LIMIT
// ══════════════════════════════════════════════════════════════════════════════

// RateLimit определяет ограничение частоты отправки.
type RateLimit struct {
	// MaxCount - максимальное количество уведомлений.
	MaxCount int

	// Period - период, за который считается лимит.
	Period time.Duration

	// PerRecipient - лимит на получателя (true) или глобальный (false).
	PerRecipient bool

	// BurstAllowed - разрешён ли "всплеск" (превышение лимита в особых случаях).
	BurstAllowed bool
}

// NewRateLimit создаёт новое ограничение частоты.
func NewRateLimit(maxCount int, period time.Duration) (*RateLimit, error) {
	if maxCount <= 0 {
		return nil, ErrInvalidRateLimit
	}
	if period <= 0 {
		return nil, ErrInvalidRateLimitPeriod
	}

	return &RateLimit{
		MaxCount:     maxCount,
		Period:       period,
		PerRecipient: true,
		BurstAllowed: false,
	}, nil
}

// IsExceeded проверяет, превышен ли лимит.
func (rl *RateLimit) IsExceeded(currentCount int) bool {
	return currentCount >= rl.MaxCount
}

// Clone создаёт копию лимита.
func (rl *RateLimit) Clone() *RateLimit {
	if rl == nil {
		return nil
	}
	clone := *rl
	return &clone
}

// ══════════════════════════════════════════════════════════════════════════════
// TRIGGER RULE ENTITY
// ══════════════════════════════════════════════════════════════════════════════

// TriggerRule определяет правило срабатывания уведомления.
type TriggerRule struct {
	// ID - уникальный идентификатор правила.
	ID TriggerRuleID

	// Name - человекочитаемое название правила.
	Name string

	// Description - описание правила.
	Description string

	// NotificationType - тип уведомления, которое создаётся при срабатывании.
	NotificationType NotificationType

	// Conditions - список условий (все должны выполняться - AND).
	Conditions []*Condition

	// TimeConstraint - временные ограничения.
	TimeConstraint *TimeConstraint

	// RateLimit - ограничение частоты.
	RateLimit *RateLimit

	// Priority - приоритет уведомления.
	Priority Priority

	// MessageTemplate - шаблон сообщения.
	MessageTemplate string

	// TitleTemplate - шаблон заголовка.
	TitleTemplate string

	// IsEnabled - правило активно.
	IsEnabled bool

	// RequiresUserConsent - требуется согласие пользователя (настройка).
	RequiresUserConsent bool

	// ConsentSettingKey - ключ настройки пользователя для проверки согласия.
	ConsentSettingKey string

	// CooldownPeriod - минимальный интервал между срабатываниями для одного пользователя.
	CooldownPeriod time.Duration

	// ExpiresAfter - через какое время уведомление устаревает.
	ExpiresAfter time.Duration

	// Tags - теги для группировки и фильтрации правил.
	Tags []string

	// Metadata - дополнительные данные.
	Metadata map[string]string

	// CreatedAt - время создания.
	CreatedAt time.Time

	// UpdatedAt - время обновления.
	UpdatedAt time.Time
}

// NewTriggerRuleParams содержит параметры для создания правила.
type NewTriggerRuleParams struct {
	ID               TriggerRuleID
	Name             string
	NotificationType NotificationType
	MessageTemplate  string
	Priority         *Priority
}

// NewTriggerRule создаёт новое правило с валидацией.
func NewTriggerRule(params NewTriggerRuleParams) (*TriggerRule, error) {
	if !params.ID.IsValid() {
		return nil, ErrInvalidTriggerRuleID
	}
	if params.Name == "" {
		return nil, ErrEmptyRuleName
	}
	if !params.NotificationType.IsValid() {
		return nil, ErrInvalidNotificationType
	}
	if params.MessageTemplate == "" {
		return nil, ErrEmptyMessageTemplate
	}

	priority := params.NotificationType.DefaultPriority()
	if params.Priority != nil && params.Priority.IsValid() {
		priority = *params.Priority
	}

	now := time.Now().UTC()

	return &TriggerRule{
		ID:                  params.ID,
		Name:                params.Name,
		NotificationType:    params.NotificationType,
		Conditions:          make([]*Condition, 0),
		Priority:            priority,
		MessageTemplate:     params.MessageTemplate,
		IsEnabled:           true,
		RequiresUserConsent: false,
		CooldownPeriod:      0,
		ExpiresAfter:        24 * time.Hour, // по умолчанию 24 часа
		Tags:                make([]string, 0),
		Metadata:            make(map[string]string),
		CreatedAt:           now,
		UpdatedAt:           now,
	}, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// DOMAIN METHODS
// ══════════════════════════════════════════════════════════════════════════════

// AddCondition добавляет условие к правилу.
func (tr *TriggerRule) AddCondition(condition *Condition) error {
	if condition == nil {
		return ErrNilCondition
	}
	tr.Conditions = append(tr.Conditions, condition)
	tr.UpdatedAt = time.Now().UTC()
	return nil
}

// RemoveCondition удаляет условие по индексу.
func (tr *TriggerRule) RemoveCondition(index int) error {
	if index < 0 || index >= len(tr.Conditions) {
		return ErrConditionIndexOutOfRange
	}
	tr.Conditions = append(tr.Conditions[:index], tr.Conditions[index+1:]...)
	tr.UpdatedAt = time.Now().UTC()
	return nil
}

// SetTimeConstraint устанавливает временные ограничения.
func (tr *TriggerRule) SetTimeConstraint(tc *TimeConstraint) {
	tr.TimeConstraint = tc
	tr.UpdatedAt = time.Now().UTC()
}

// SetRateLimit устанавливает ограничение частоты.
func (tr *TriggerRule) SetRateLimit(rl *RateLimit) {
	tr.RateLimit = rl
	tr.UpdatedAt = time.Now().UTC()
}

// Enable активирует правило.
func (tr *TriggerRule) Enable() {
	tr.IsEnabled = true
	tr.UpdatedAt = time.Now().UTC()
}

// Disable деактивирует правило.
func (tr *TriggerRule) Disable() {
	tr.IsEnabled = false
	tr.UpdatedAt = time.Now().UTC()
}

// SetCooldown устанавливает период охлаждения.
func (tr *TriggerRule) SetCooldown(duration time.Duration) {
	tr.CooldownPeriod = duration
	tr.UpdatedAt = time.Now().UTC()
}

// SetExpiration устанавливает время устаревания уведомлений.
func (tr *TriggerRule) SetExpiration(duration time.Duration) {
	tr.ExpiresAfter = duration
	tr.UpdatedAt = time.Now().UTC()
}

// RequireConsent устанавливает требование согласия пользователя.
func (tr *TriggerRule) RequireConsent(settingKey string) {
	tr.RequiresUserConsent = true
	tr.ConsentSettingKey = settingKey
	tr.UpdatedAt = time.Now().UTC()
}

// AddTag добавляет тег.
func (tr *TriggerRule) AddTag(tag string) {
	for _, t := range tr.Tags {
		if t == tag {
			return // уже есть
		}
	}
	tr.Tags = append(tr.Tags, tag)
	tr.UpdatedAt = time.Now().UTC()
}

// HasTag проверяет наличие тега.
func (tr *TriggerRule) HasTag(tag string) bool {
	for _, t := range tr.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// SetMetadata устанавливает метаданные.
func (tr *TriggerRule) SetMetadata(key, value string) {
	if tr.Metadata == nil {
		tr.Metadata = make(map[string]string)
	}
	tr.Metadata[key] = value
	tr.UpdatedAt = time.Now().UTC()
}

// ══════════════════════════════════════════════════════════════════════════════
// EVALUATION
// ══════════════════════════════════════════════════════════════════════════════

// TriggerContext содержит контекст для вычисления триггера.
type TriggerContext struct {
	// StudentID - ID студента.
	StudentID string

	// TelegramChatID - ID чата Telegram.
	TelegramChatID TelegramChatID

	// Timestamp - время события.
	Timestamp time.Time

	// Values - значения для проверки условий (ключ = тип условия).
	Values map[ConditionType]int

	// StringValues - строковые значения.
	StringValues map[ConditionType]string

	// UserPreferences - настройки пользователя для проверки согласия.
	UserPreferences map[string]bool

	// LastTriggeredAt - время последнего срабатывания этого правила для студента.
	LastTriggeredAt *time.Time

	// TriggerCount - количество срабатываний за период RateLimit.
	TriggerCount int

	// Data - дополнительные данные для шаблона уведомления.
	Data NotificationData
}

// NewTriggerContext создаёт новый контекст для вычисления.
func NewTriggerContext(studentID string, chatID TelegramChatID) *TriggerContext {
	return &TriggerContext{
		StudentID:       studentID,
		TelegramChatID:  chatID,
		Timestamp:       time.Now().UTC(),
		Values:          make(map[ConditionType]int),
		StringValues:    make(map[ConditionType]string),
		UserPreferences: make(map[string]bool),
	}
}

// SetValue устанавливает числовое значение для условия.
func (ctx *TriggerContext) SetValue(condType ConditionType, value int) {
	ctx.Values[condType] = value
}

// SetStringValue устанавливает строковое значение для условия.
func (ctx *TriggerContext) SetStringValue(condType ConditionType, value string) {
	ctx.StringValues[condType] = value
}

// SetUserPreference устанавливает настройку пользователя.
func (ctx *TriggerContext) SetUserPreference(key string, enabled bool) {
	ctx.UserPreferences[key] = enabled
}

// EvaluationResult содержит результат вычисления триггера.
type EvaluationResult struct {
	// ShouldTrigger - правило должно сработать.
	ShouldTrigger bool

	// Reason - причина (если не сработало).
	Reason string

	// FailedConditions - индексы условий, которые не выполнились.
	FailedConditions []int
}

// Evaluate вычисляет, должен ли триггер сработать.
func (tr *TriggerRule) Evaluate(ctx *TriggerContext) EvaluationResult {
	// Проверяем, активно ли правило
	if !tr.IsEnabled {
		return EvaluationResult{
			ShouldTrigger: false,
			Reason:        "rule is disabled",
		}
	}

	// Проверяем согласие пользователя
	if tr.RequiresUserConsent {
		if consent, ok := ctx.UserPreferences[tr.ConsentSettingKey]; !ok || !consent {
			return EvaluationResult{
				ShouldTrigger: false,
				Reason:        "user consent not given",
			}
		}
	}

	// Проверяем временные ограничения
	if tr.TimeConstraint != nil && !tr.TimeConstraint.IsAllowed(ctx.Timestamp) {
		return EvaluationResult{
			ShouldTrigger: false,
			Reason:        "outside allowed time window",
		}
	}

	// Проверяем cooldown
	if tr.CooldownPeriod > 0 && ctx.LastTriggeredAt != nil {
		if time.Since(*ctx.LastTriggeredAt) < tr.CooldownPeriod {
			return EvaluationResult{
				ShouldTrigger: false,
				Reason:        "cooldown period not elapsed",
			}
		}
	}

	// Проверяем rate limit
	if tr.RateLimit != nil && tr.RateLimit.IsExceeded(ctx.TriggerCount) {
		return EvaluationResult{
			ShouldTrigger: false,
			Reason:        "rate limit exceeded",
		}
	}

	// Проверяем все условия
	failedConditions := make([]int, 0)
	for i, condition := range tr.Conditions {
		value, ok := ctx.Values[condition.Type]
		if !ok {
			failedConditions = append(failedConditions, i)
			continue
		}
		if !condition.Evaluate(value) {
			failedConditions = append(failedConditions, i)
		}
	}

	if len(failedConditions) > 0 {
		return EvaluationResult{
			ShouldTrigger:    false,
			Reason:           "conditions not met",
			FailedConditions: failedConditions,
		}
	}

	return EvaluationResult{
		ShouldTrigger: true,
	}
}

// Clone создаёт глубокую копию правила.
func (tr *TriggerRule) Clone() *TriggerRule {
	if tr == nil {
		return nil
	}

	clone := *tr

	// Копируем условия
	if tr.Conditions != nil {
		clone.Conditions = make([]*Condition, len(tr.Conditions))
		for i, c := range tr.Conditions {
			clone.Conditions[i] = c.Clone()
		}
	}

	// Копируем ограничения
	clone.TimeConstraint = tr.TimeConstraint.Clone()
	clone.RateLimit = tr.RateLimit.Clone()

	// Копируем теги
	if tr.Tags != nil {
		clone.Tags = make([]string, len(tr.Tags))
		copy(clone.Tags, tr.Tags)
	}

	// Копируем метаданные
	if tr.Metadata != nil {
		clone.Metadata = make(map[string]string, len(tr.Metadata))
		for k, v := range tr.Metadata {
			clone.Metadata[k] = v
		}
	}

	return &clone
}

// String возвращает строковое представление для логирования.
func (tr *TriggerRule) String() string {
	return fmt.Sprintf(
		"TriggerRule{ID: %s, Name: %s, Type: %s, Enabled: %v, Conditions: %d}",
		tr.ID, tr.Name, tr.NotificationType, tr.IsEnabled, len(tr.Conditions),
	)
}

// ══════════════════════════════════════════════════════════════════════════════
// PREDEFINED RULES FACTORY
// ══════════════════════════════════════════════════════════════════════════════

// NewRankUpRule создаёт правило для уведомления о повышении в рейтинге.
func NewRankUpRule(id TriggerRuleID, minPositions int) (*TriggerRule, error) {
	rule, err := NewTriggerRule(NewTriggerRuleParams{
		ID:               id,
		Name:             "Rank Up Notification",
		NotificationType: NotificationTypeRankUp,
		MessageTemplate:  "🚀 Ты поднялся на {{.RankChange}} мест! Теперь ты #{{.NewRank}}",
	})
	if err != nil {
		return nil, err
	}

	condition, _ := NewCondition(ConditionTypeRankChange, OpGreaterOrEqual, minPositions)
	_ = rule.AddCondition(condition)
	rule.RequireConsent("rank_changes")

	return rule, nil
}

// NewInactivityRule создаёт правило для напоминания о неактивности.
func NewInactivityRule(id TriggerRuleID, days int) (*TriggerRule, error) {
	rule, err := NewTriggerRule(NewTriggerRuleParams{
		ID:               id,
		Name:             "Inactivity Reminder",
		NotificationType: NotificationTypeInactivityReminder,
		MessageTemplate:  "👋 Давно тебя не видели! Уже {{.DaysInactive}} дней без задач. Возвращайся!",
	})
	if err != nil {
		return nil, err
	}

	condition, _ := NewCondition(ConditionTypeInactiveDays, OpGreaterOrEqual, days)
	_ = rule.AddCondition(condition)

	rule.RequireConsent("inactivity_reminders")
	rule.SetCooldown(24 * time.Hour) // не чаще раза в день

	return rule, nil
}

// NewStreakReminderRule создаёт правило для напоминания о серии.
func NewStreakReminderRule(id TriggerRuleID, hoursRemaining int) (*TriggerRule, error) {
	rule, err := NewTriggerRule(NewTriggerRuleParams{
		ID:               id,
		Name:             "Streak At Risk Reminder",
		NotificationType: NotificationTypeStreakReminder,
		MessageTemplate:  "🔥 Не потеряй свою серию в {{.StreakDays}} дней! Осталось {{.HoursRemaining}} часов",
	})
	if err != nil {
		return nil, err
	}

	condition, _ := NewCondition(ConditionTypeStreakAtRisk, OpLessOrEqual, hoursRemaining)
	_ = rule.AddCondition(condition)

	rule.SetCooldown(6 * time.Hour) // не чаще раза в 6 часов

	return rule, nil
}

// NewDailyDigestRule создаёт правило для ежедневной сводки.
func NewDailyDigestRule(id TriggerRuleID, hour int, timezone string) (*TriggerRule, error) {
	rule, err := NewTriggerRule(NewTriggerRuleParams{
		ID:               id,
		Name:             "Daily Digest",
		NotificationType: NotificationTypeDailyDigest,
		MessageTemplate:  "📊 Твой день: +{{.XPGained}} XP, {{.TasksCompleted}} задач, #{{.NewRank}} в рейтинге",
	})
	if err != nil {
		return nil, err
	}

	tc, _ := NewTimeConstraint(hour, hour+1, timezone)
	rule.SetTimeConstraint(tc)
	rule.RequireConsent("daily_digest")
	rule.SetCooldown(23 * time.Hour)

	return rule, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// TRIGGER ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrInvalidTriggerRuleID - невалидный ID правила.
	ErrInvalidTriggerRuleID = errors.New("invalid trigger rule id: cannot be empty")

	// ErrEmptyRuleName - пустое название правила.
	ErrEmptyRuleName = errors.New("rule name cannot be empty")

	// ErrEmptyMessageTemplate - пустой шаблон сообщения.
	ErrEmptyMessageTemplate = errors.New("message template cannot be empty")

	// ErrInvalidConditionType - невалидный тип условия.
	ErrInvalidConditionType = errors.New("invalid condition type")

	// ErrInvalidOperator - невалидный оператор сравнения.
	ErrInvalidOperator = errors.New("invalid comparison operator")

	// ErrInvalidRange - невалидный диапазон (min > max).
	ErrInvalidRange = errors.New("invalid range: min must be <= max")

	// ErrEmptyValueList - пустой список значений.
	ErrEmptyValueList = errors.New("value list cannot be empty")

	// ErrInvalidHours - невалидные часы (не 0-23).
	ErrInvalidHours = errors.New("invalid hours: must be 0-23")

	// ErrInvalidRateLimit - невалидный лимит частоты.
	ErrInvalidRateLimit = errors.New("invalid rate limit: max count must be positive")

	// ErrInvalidRateLimitPeriod - невалидный период лимита.
	ErrInvalidRateLimitPeriod = errors.New("invalid rate limit period: must be positive")

	// ErrNilCondition - nil условие.
	ErrNilCondition = errors.New("condition cannot be nil")

	// ErrConditionIndexOutOfRange - индекс условия вне диапазона.
	ErrConditionIndexOutOfRange = errors.New("condition index out of range")

	// ErrTriggerRuleNotFound - правило не найдено.
	ErrTriggerRuleNotFound = errors.New("trigger rule not found")
)
