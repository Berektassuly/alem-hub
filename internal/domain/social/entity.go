// Package social содержит доменную модель для социальных функций сообщества.
// Философия: "От конкуренции к сотрудничеству" — лидерборд становится
// инструментом поиска помощи, а не источником стресса.
package social

import (
	"errors"
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// VALUE OBJECTS
// ══════════════════════════════════════════════════════════════════════════════

// StudentID представляет идентификатор студента (UUID в строковом формате).
type StudentID string

// IsValid проверяет, что StudentID непустой.
func (s StudentID) IsValid() bool {
	return len(s) > 0
}

// String возвращает строковое представление.
func (s StudentID) String() string {
	return string(s)
}

// TaskID представляет идентификатор задачи на платформе Alem.
type TaskID string

// IsValid проверяет корректность TaskID.
func (t TaskID) IsValid() bool {
	s := string(t)
	return len(s) >= 1 && len(s) <= 100
}

// String возвращает строковое представление.
func (t TaskID) String() string {
	return string(t)
}

// Rating представляет оценку (0.0 - 5.0).
type Rating float64

// IsValid проверяет, что рейтинг в допустимом диапазоне.
func (r Rating) IsValid() bool {
	return r >= 0.0 && r <= 5.0
}

// Stars возвращает целое число звёзд для отображения.
func (r Rating) Stars() int {
	return int(r + 0.5) // Округление
}

// ══════════════════════════════════════════════════════════════════════════════
// ENUMS
// ══════════════════════════════════════════════════════════════════════════════

// ConnectionType определяет тип связи между студентами.
type ConnectionType string

const (
	// ConnectionTypeStudyBuddy - напарник по учёбе (двусторонняя связь).
	ConnectionTypeStudyBuddy ConnectionType = "study_buddy"

	// ConnectionTypeMentor - менторская связь (mentor → mentee).
	ConnectionTypeMentor ConnectionType = "mentor"

	// ConnectionTypeHelper - разовая помощь по задаче.
	ConnectionTypeHelper ConnectionType = "helper"

	// ConnectionTypeCoworker - работали вместе над проектом.
	ConnectionTypeCoworker ConnectionType = "coworker"
)

// IsValid проверяет корректность типа связи.
func (c ConnectionType) IsValid() bool {
	switch c {
	case ConnectionTypeStudyBuddy, ConnectionTypeMentor, ConnectionTypeHelper, ConnectionTypeCoworker:
		return true
	default:
		return false
	}
}

// IsBidirectional возвращает true, если связь двусторонняя.
func (c ConnectionType) IsBidirectional() bool {
	return c == ConnectionTypeStudyBuddy || c == ConnectionTypeCoworker
}

// ConnectionStatus определяет статус связи.
type ConnectionStatus string

const (
	// ConnectionStatusPending - ожидает подтверждения.
	ConnectionStatusPending ConnectionStatus = "pending"

	// ConnectionStatusActive - активная связь.
	ConnectionStatusActive ConnectionStatus = "active"

	// ConnectionStatusDeclined - отклонена.
	ConnectionStatusDeclined ConnectionStatus = "declined"

	// ConnectionStatusEnded - завершена (по инициативе одной из сторон).
	ConnectionStatusEnded ConnectionStatus = "ended"
)

// IsValid проверяет корректность статуса.
func (c ConnectionStatus) IsValid() bool {
	switch c {
	case ConnectionStatusPending, ConnectionStatusActive, ConnectionStatusDeclined, ConnectionStatusEnded:
		return true
	default:
		return false
	}
}

// IsActive возвращает true, если связь активна.
func (c ConnectionStatus) IsActive() bool {
	return c == ConnectionStatusActive
}

// HelpRequestStatus определяет статус запроса помощи.
type HelpRequestStatus string

const (
	// HelpRequestStatusOpen - открыт, ищем помощника.
	HelpRequestStatusOpen HelpRequestStatus = "open"

	// HelpRequestStatusMatched - найден помощник, ожидаем связи.
	HelpRequestStatusMatched HelpRequestStatus = "matched"

	// HelpRequestStatusInProgress - помощь оказывается.
	HelpRequestStatusInProgress HelpRequestStatus = "in_progress"

	// HelpRequestStatusResolved - проблема решена.
	HelpRequestStatusResolved HelpRequestStatus = "resolved"

	// HelpRequestStatusCancelled - отменён пользователем.
	HelpRequestStatusCancelled HelpRequestStatus = "cancelled"

	// HelpRequestStatusExpired - истёк срок (24 часа без ответа).
	HelpRequestStatusExpired HelpRequestStatus = "expired"
)

// IsValid проверяет корректность статуса.
func (h HelpRequestStatus) IsValid() bool {
	switch h {
	case HelpRequestStatusOpen, HelpRequestStatusMatched, HelpRequestStatusInProgress,
		HelpRequestStatusResolved, HelpRequestStatusCancelled, HelpRequestStatusExpired:
		return true
	default:
		return false
	}
}

// IsOpen возвращает true, если запрос ещё открыт.
func (h HelpRequestStatus) IsOpen() bool {
	return h == HelpRequestStatusOpen || h == HelpRequestStatusMatched
}

// IsClosed возвращает true, если запрос закрыт.
func (h HelpRequestStatus) IsClosed() bool {
	return h == HelpRequestStatusResolved || h == HelpRequestStatusCancelled || h == HelpRequestStatusExpired
}

// HelpRequestPriority определяет приоритет запроса.
type HelpRequestPriority string

const (
	// HelpRequestPriorityLow - низкий приоритет (общий вопрос).
	HelpRequestPriorityLow HelpRequestPriority = "low"

	// HelpRequestPriorityNormal - обычный приоритет.
	HelpRequestPriorityNormal HelpRequestPriority = "normal"

	// HelpRequestPriorityHigh - высокий приоритет (дедлайн скоро).
	HelpRequestPriorityHigh HelpRequestPriority = "high"

	// HelpRequestPriorityUrgent - срочный (дедлайн сегодня/завтра).
	HelpRequestPriorityUrgent HelpRequestPriority = "urgent"
)

// IsValid проверяет корректность приоритета.
func (p HelpRequestPriority) IsValid() bool {
	switch p {
	case HelpRequestPriorityLow, HelpRequestPriorityNormal, HelpRequestPriorityHigh, HelpRequestPriorityUrgent:
		return true
	default:
		return false
	}
}

// Weight возвращает числовой вес приоритета для сортировки.
func (p HelpRequestPriority) Weight() int {
	switch p {
	case HelpRequestPriorityUrgent:
		return 4
	case HelpRequestPriorityHigh:
		return 3
	case HelpRequestPriorityNormal:
		return 2
	case HelpRequestPriorityLow:
		return 1
	default:
		return 0
	}
}

// EndorsementType определяет тип благодарности.
type EndorsementType string

const (
	// EndorsementTypeClear - понятно объяснил.
	EndorsementTypeClear EndorsementType = "clear"

	// EndorsementTypePatient - терпеливый.
	EndorsementTypePatient EndorsementType = "patient"

	// EndorsementTypeDeep - глубокие знания.
	EndorsementTypeDeep EndorsementType = "deep"

	// EndorsementTypeFast - быстро помог.
	EndorsementTypeFast EndorsementType = "fast"

	// EndorsementTypeFriendly - дружелюбный.
	EndorsementTypeFriendly EndorsementType = "friendly"

	// EndorsementTypeInspiring - вдохновляющий.
	EndorsementTypeInspiring EndorsementType = "inspiring"
)

// IsValid проверяет корректность типа благодарности.
func (e EndorsementType) IsValid() bool {
	switch e {
	case EndorsementTypeClear, EndorsementTypePatient, EndorsementTypeDeep,
		EndorsementTypeFast, EndorsementTypeFriendly, EndorsementTypeInspiring:
		return true
	default:
		return false
	}
}

// Emoji возвращает эмодзи для типа.
func (e EndorsementType) Emoji() string {
	switch e {
	case EndorsementTypeClear:
		return "💡"
	case EndorsementTypePatient:
		return "🧘"
	case EndorsementTypeDeep:
		return "🎓"
	case EndorsementTypeFast:
		return "⚡"
	case EndorsementTypeFriendly:
		return "😊"
	case EndorsementTypeInspiring:
		return "✨"
	default:
		return "👍"
	}
}

// Label возвращает человекочитаемую метку.
func (e EndorsementType) Label() string {
	switch e {
	case EndorsementTypeClear:
		return "Понятно объясняет"
	case EndorsementTypePatient:
		return "Терпеливый"
	case EndorsementTypeDeep:
		return "Глубокие знания"
	case EndorsementTypeFast:
		return "Быстрый ответ"
	case EndorsementTypeFriendly:
		return "Дружелюбный"
	case EndorsementTypeInspiring:
		return "Вдохновляющий"
	default:
		return "Помог"
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// DOMAIN ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrConnectionNotFound - связь не найдена.
	ErrConnectionNotFound = errors.New("connection not found")

	// ErrConnectionAlreadyExists - связь уже существует.
	ErrConnectionAlreadyExists = errors.New("connection already exists")

	// ErrConnectionSameStudent - нельзя создать связь с самим собой.
	ErrConnectionSameStudent = errors.New("cannot create connection with self")

	// ErrConnectionInvalidType - невалидный тип связи.
	ErrConnectionInvalidType = errors.New("invalid connection type")

	// ErrConnectionInvalidStatus - невалидный статус связи.
	ErrConnectionInvalidStatus = errors.New("invalid connection status")

	// ErrConnectionNotPending - связь не в статусе ожидания.
	ErrConnectionNotPending = errors.New("connection is not pending")

	// ErrConnectionAlreadyEnded - связь уже завершена.
	ErrConnectionAlreadyEnded = errors.New("connection already ended")

	// ErrHelpRequestNotFound - запрос помощи не найден.
	ErrHelpRequestNotFound = errors.New("help request not found")

	// ErrHelpRequestAlreadyClosed - запрос уже закрыт.
	ErrHelpRequestAlreadyClosed = errors.New("help request already closed")

	// ErrHelpRequestInvalidPriority - невалидный приоритет.
	ErrHelpRequestInvalidPriority = errors.New("invalid help request priority")

	// ErrHelpRequestSelfHelp - нельзя помогать самому себе.
	ErrHelpRequestSelfHelp = errors.New("cannot help yourself")

	// ErrHelpRequestAlreadyMatched - помощник уже назначен.
	ErrHelpRequestAlreadyMatched = errors.New("help request already has a helper")

	// ErrEndorsementNotFound - благодарность не найдена.
	ErrEndorsementNotFound = errors.New("endorsement not found")

	// ErrEndorsementAlreadyExists - благодарность уже существует.
	ErrEndorsementAlreadyExists = errors.New("endorsement already exists for this help request")

	// ErrEndorsementInvalidRating - невалидный рейтинг.
	ErrEndorsementInvalidRating = errors.New("invalid endorsement rating: must be between 0 and 5")

	// ErrEndorsementSelfEndorse - нельзя благодарить самого себя.
	ErrEndorsementSelfEndorse = errors.New("cannot endorse yourself")

	// ErrInvalidStudentID - невалидный ID студента.
	ErrInvalidStudentID = errors.New("invalid student id")

	// ErrInvalidTaskID - невалидный ID задачи.
	ErrInvalidTaskID = errors.New("invalid task id")
)

// ══════════════════════════════════════════════════════════════════════════════
// ENTITY: CONNECTION
// Представляет связь между двумя студентами.
// ══════════════════════════════════════════════════════════════════════════════

// Connection - связь между двумя студентами.
// Воплощает идею сообщества: не просто соревнование, а взаимопомощь.
type Connection struct {
	// ID - уникальный идентификатор связи (UUID).
	ID string

	// InitiatorID - кто инициировал связь.
	InitiatorID StudentID

	// ReceiverID - кто получил запрос на связь.
	ReceiverID StudentID

	// Type - тип связи (study_buddy, mentor, helper, coworker).
	Type ConnectionType

	// Status - текущий статус связи.
	Status ConnectionStatus

	// Context - контекст создания связи (например, TaskID или причина).
	Context ConnectionContext

	// Stats - статистика по связи.
	Stats ConnectionStats

	// CreatedAt - когда связь была создана.
	CreatedAt time.Time

	// UpdatedAt - когда связь была обновлена.
	UpdatedAt time.Time

	// AcceptedAt - когда связь была принята (nil если ещё pending).
	AcceptedAt *time.Time

	// EndedAt - когда связь была завершена (nil если активна).
	EndedAt *time.Time

	// EndReason - причина завершения связи.
	EndReason string
}

// ConnectionContext содержит контекст создания связи.
type ConnectionContext struct {
	// TaskID - ID задачи, если связь возникла из-за помощи по задаче.
	TaskID TaskID

	// HelpRequestID - ID запроса помощи, если применимо.
	HelpRequestID string

	// Note - произвольная заметка инициатора.
	Note string
}

// ConnectionStats содержит статистику взаимодействия.
type ConnectionStats struct {
	// InteractionCount - количество взаимодействий.
	InteractionCount int

	// TotalHelpTime - общее время помощи (в минутах).
	TotalHelpTime int

	// LastInteractionAt - время последнего взаимодействия.
	LastInteractionAt time.Time

	// TasksSolvedTogether - задачи, решённые вместе.
	TasksSolvedTogether int

	// MutualRating - взаимная оценка (среднее).
	MutualRating Rating
}

// NewConnectionParams параметры для создания новой связи.
type NewConnectionParams struct {
	ID          string
	InitiatorID StudentID
	ReceiverID  StudentID
	Type        ConnectionType
	Context     ConnectionContext
}

// NewConnection создаёт новую связь между студентами.
func NewConnection(params NewConnectionParams) (*Connection, error) {
	if params.ID == "" {
		return nil, errors.New("connection id is required")
	}

	if !params.InitiatorID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	if !params.ReceiverID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	if params.InitiatorID == params.ReceiverID {
		return nil, ErrConnectionSameStudent
	}

	if !params.Type.IsValid() {
		return nil, ErrConnectionInvalidType
	}

	now := time.Now().UTC()

	return &Connection{
		ID:          params.ID,
		InitiatorID: params.InitiatorID,
		ReceiverID:  params.ReceiverID,
		Type:        params.Type,
		Status:      ConnectionStatusPending,
		Context:     params.Context,
		Stats: ConnectionStats{
			InteractionCount:    0,
			TotalHelpTime:       0,
			LastInteractionAt:   now,
			TasksSolvedTogether: 0,
			MutualRating:        0,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Accept принимает запрос на связь.
func (c *Connection) Accept() error {
	if c.Status != ConnectionStatusPending {
		return ErrConnectionNotPending
	}

	now := time.Now().UTC()
	c.Status = ConnectionStatusActive
	c.AcceptedAt = &now
	c.UpdatedAt = now
	return nil
}

// Decline отклоняет запрос на связь.
func (c *Connection) Decline() error {
	if c.Status != ConnectionStatusPending {
		return ErrConnectionNotPending
	}

	c.Status = ConnectionStatusDeclined
	c.UpdatedAt = time.Now().UTC()
	return nil
}

// End завершает связь.
func (c *Connection) End(reason string) error {
	if c.Status == ConnectionStatusEnded {
		return ErrConnectionAlreadyEnded
	}

	now := time.Now().UTC()
	c.Status = ConnectionStatusEnded
	c.EndedAt = &now
	c.EndReason = reason
	c.UpdatedAt = now
	return nil
}

// RecordInteraction записывает взаимодействие.
func (c *Connection) RecordInteraction(helpTimeMinutes int) {
	c.Stats.InteractionCount++
	c.Stats.TotalHelpTime += helpTimeMinutes
	c.Stats.LastInteractionAt = time.Now().UTC()
	c.UpdatedAt = c.Stats.LastInteractionAt
}

// RecordTaskSolved записывает совместно решённую задачу.
func (c *Connection) RecordTaskSolved() {
	c.Stats.TasksSolvedTogether++
	c.Stats.LastInteractionAt = time.Now().UTC()
	c.UpdatedAt = c.Stats.LastInteractionAt
}

// UpdateRating обновляет взаимную оценку.
func (c *Connection) UpdateRating(rating Rating) error {
	if !rating.IsValid() {
		return ErrEndorsementInvalidRating
	}

	// Вычисляем среднее между старым и новым рейтингом
	if c.Stats.MutualRating == 0 {
		c.Stats.MutualRating = rating
	} else {
		c.Stats.MutualRating = Rating((float64(c.Stats.MutualRating) + float64(rating)) / 2)
	}
	c.UpdatedAt = time.Now().UTC()
	return nil
}

// IsActive проверяет, активна ли связь.
func (c *Connection) IsActive() bool {
	return c.Status.IsActive()
}

// IsPending проверяет, ожидает ли связь подтверждения.
func (c *Connection) IsPending() bool {
	return c.Status == ConnectionStatusPending
}

// InvolveStudent проверяет, участвует ли студент в связи.
func (c *Connection) InvolveStudent(studentID StudentID) bool {
	return c.InitiatorID == studentID || c.ReceiverID == studentID
}

// GetOtherStudent возвращает ID другого участника связи.
func (c *Connection) GetOtherStudent(studentID StudentID) StudentID {
	if c.InitiatorID == studentID {
		return c.ReceiverID
	}
	return c.InitiatorID
}

// DurationDays возвращает продолжительность связи в днях.
func (c *Connection) DurationDays() int {
	if c.AcceptedAt == nil {
		return 0
	}

	endTime := time.Now().UTC()
	if c.EndedAt != nil {
		endTime = *c.EndedAt
	}

	return int(endTime.Sub(*c.AcceptedAt).Hours() / 24)
}

// String возвращает строковое представление для логирования.
func (c *Connection) String() string {
	return fmt.Sprintf(
		"Connection{ID: %s, %s -> %s, Type: %s, Status: %s}",
		c.ID, c.InitiatorID, c.ReceiverID, c.Type, c.Status,
	)
}

// Clone создаёт глубокую копию связи.
func (c *Connection) Clone() *Connection {
	if c == nil {
		return nil
	}

	clone := *c
	if c.AcceptedAt != nil {
		acceptedAt := *c.AcceptedAt
		clone.AcceptedAt = &acceptedAt
	}
	if c.EndedAt != nil {
		endedAt := *c.EndedAt
		clone.EndedAt = &endedAt
	}
	return &clone
}

// ══════════════════════════════════════════════════════════════════════════════
// ENTITY: HELP REQUEST
// Запрос помощи по задаче — ключевой элемент превращения лидерборда
// в инструмент сотрудничества.
// ══════════════════════════════════════════════════════════════════════════════

// HelpRequest - запрос помощи от студента.
// Это сердце социального функционала: студент застрял → ищем того, кто решил.
type HelpRequest struct {
	// ID - уникальный идентификатор запроса (UUID).
	ID string

	// RequesterID - кто просит помощь.
	RequesterID StudentID

	// TaskID - по какой задаче нужна помощь.
	TaskID TaskID

	// TaskName - название задачи (для отображения).
	TaskName string

	// Description - описание проблемы (опционально).
	Description string

	// Priority - приоритет запроса.
	Priority HelpRequestPriority

	// Status - текущий статус запроса.
	Status HelpRequestStatus

	// HelperID - кто взялся помочь (nil если ещё не назначен).
	HelperID *StudentID

	// MatchedHelpers - список потенциальных помощников (для выбора).
	MatchedHelpers []MatchedHelper

	// DeadlineAt - когда нужна помощь (дедлайн задачи).
	DeadlineAt *time.Time

	// ExpiresAt - когда запрос истекает (24 часа по умолчанию).
	ExpiresAt time.Time

	// CreatedAt - когда создан запрос.
	CreatedAt time.Time

	// UpdatedAt - когда обновлён запрос.
	UpdatedAt time.Time

	// ResolvedAt - когда проблема решена.
	ResolvedAt *time.Time

	// Resolution - как была решена проблема.
	Resolution *HelpResolution
}

// MatchedHelper представляет потенциального помощника.
type MatchedHelper struct {
	// StudentID - ID помощника.
	StudentID StudentID

	// DisplayName - имя для отображения.
	DisplayName string

	// HelpRating - рейтинг как помощника.
	HelpRating Rating

	// SolvedAt - когда решил эту задачу.
	SolvedAt time.Time

	// IsOnline - онлайн ли сейчас.
	IsOnline bool

	// LastSeenAt - когда был онлайн.
	LastSeenAt time.Time

	// MatchScore - оценка совместимости (0-100).
	MatchScore int

	// MatchReasons - причины, почему подходит.
	MatchReasons []string
}

// HelpResolution описывает, как была решена проблема.
type HelpResolution struct {
	// Method - способ решения.
	Method HelpResolutionMethod

	// HelperID - кто помог (если применимо).
	HelperID *StudentID

	// DurationMinutes - сколько времени заняла помощь.
	DurationMinutes int

	// Notes - заметки о решении.
	Notes string
}

// HelpResolutionMethod определяет способ решения.
type HelpResolutionMethod string

const (
	// HelpResolutionWithHelper - решено с помощью другого студента.
	HelpResolutionWithHelper HelpResolutionMethod = "with_helper"

	// HelpResolutionSelf - решено самостоятельно.
	HelpResolutionSelf HelpResolutionMethod = "self"

	// HelpResolutionSkipped - задача пропущена.
	HelpResolutionSkipped HelpResolutionMethod = "skipped"

	// HelpResolutionOther - другой способ.
	HelpResolutionOther HelpResolutionMethod = "other"
)

// NewHelpRequestParams параметры для создания запроса помощи.
type NewHelpRequestParams struct {
	ID          string
	RequesterID StudentID
	TaskID      TaskID
	TaskName    string
	Description string
	Priority    HelpRequestPriority
	DeadlineAt  *time.Time
}

// NewHelpRequest создаёт новый запрос помощи.
func NewHelpRequest(params NewHelpRequestParams) (*HelpRequest, error) {
	if params.ID == "" {
		return nil, errors.New("help request id is required")
	}

	if !params.RequesterID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	if !params.TaskID.IsValid() {
		return nil, ErrInvalidTaskID
	}

	if params.TaskName == "" {
		return nil, errors.New("task name is required")
	}

	priority := params.Priority
	if priority == "" {
		priority = HelpRequestPriorityNormal
	}
	if !priority.IsValid() {
		return nil, ErrHelpRequestInvalidPriority
	}

	now := time.Now().UTC()

	// Запрос истекает через 24 часа по умолчанию
	expiresAt := now.Add(24 * time.Hour)

	return &HelpRequest{
		ID:             params.ID,
		RequesterID:    params.RequesterID,
		TaskID:         params.TaskID,
		TaskName:       params.TaskName,
		Description:    params.Description,
		Priority:       priority,
		Status:         HelpRequestStatusOpen,
		MatchedHelpers: make([]MatchedHelper, 0),
		DeadlineAt:     params.DeadlineAt,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// AddMatchedHelper добавляет потенциального помощника.
func (h *HelpRequest) AddMatchedHelper(helper MatchedHelper) error {
	if helper.StudentID == h.RequesterID {
		return ErrHelpRequestSelfHelp
	}

	// Проверяем, что помощник ещё не добавлен
	for _, existing := range h.MatchedHelpers {
		if existing.StudentID == helper.StudentID {
			return nil // Уже есть, ничего не делаем
		}
	}

	h.MatchedHelpers = append(h.MatchedHelpers, helper)
	h.Status = HelpRequestStatusMatched
	h.UpdatedAt = time.Now().UTC()
	return nil
}

// AssignHelper назначает помощника.
func (h *HelpRequest) AssignHelper(helperID StudentID) error {
	if h.Status.IsClosed() {
		return ErrHelpRequestAlreadyClosed
	}

	if helperID == h.RequesterID {
		return ErrHelpRequestSelfHelp
	}

	if h.HelperID != nil {
		return ErrHelpRequestAlreadyMatched
	}

	h.HelperID = &helperID
	h.Status = HelpRequestStatusInProgress
	h.UpdatedAt = time.Now().UTC()
	return nil
}

// Resolve помечает запрос как решённый.
func (h *HelpRequest) Resolve(resolution HelpResolution) error {
	if h.Status.IsClosed() {
		return ErrHelpRequestAlreadyClosed
	}

	now := time.Now().UTC()
	h.Status = HelpRequestStatusResolved
	h.ResolvedAt = &now
	h.Resolution = &resolution
	h.UpdatedAt = now
	return nil
}

// Cancel отменяет запрос.
func (h *HelpRequest) Cancel() error {
	if h.Status.IsClosed() {
		return ErrHelpRequestAlreadyClosed
	}

	h.Status = HelpRequestStatusCancelled
	h.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkExpired помечает запрос как истёкший.
func (h *HelpRequest) MarkExpired() error {
	if h.Status.IsClosed() {
		return ErrHelpRequestAlreadyClosed
	}

	h.Status = HelpRequestStatusExpired
	h.UpdatedAt = time.Now().UTC()
	return nil
}

// IsExpired проверяет, истёк ли запрос.
func (h *HelpRequest) IsExpired() bool {
	return time.Now().After(h.ExpiresAt)
}

// IsOpen проверяет, открыт ли запрос.
func (h *HelpRequest) IsOpen() bool {
	return h.Status.IsOpen()
}

// HoursUntilDeadline возвращает часы до дедлайна.
func (h *HelpRequest) HoursUntilDeadline() int {
	if h.DeadlineAt == nil {
		return -1 // Нет дедлайна
	}

	hours := int(time.Until(*h.DeadlineAt).Hours())
	if hours < 0 {
		return 0
	}
	return hours
}

// GetTopHelpers возвращает топ N подходящих помощников.
func (h *HelpRequest) GetTopHelpers(n int) []MatchedHelper {
	if len(h.MatchedHelpers) == 0 {
		return nil
	}

	// Возвращаем первые n (уже отсортированы по MatchScore)
	if n >= len(h.MatchedHelpers) {
		return h.MatchedHelpers
	}
	return h.MatchedHelpers[:n]
}

// String возвращает строковое представление.
func (h *HelpRequest) String() string {
	return fmt.Sprintf(
		"HelpRequest{ID: %s, Task: %s, Priority: %s, Status: %s}",
		h.ID, h.TaskID, h.Priority, h.Status,
	)
}

// Clone создаёт глубокую копию.
func (h *HelpRequest) Clone() *HelpRequest {
	if h == nil {
		return nil
	}

	clone := *h

	if h.HelperID != nil {
		helperID := *h.HelperID
		clone.HelperID = &helperID
	}

	if h.DeadlineAt != nil {
		deadlineAt := *h.DeadlineAt
		clone.DeadlineAt = &deadlineAt
	}

	if h.ResolvedAt != nil {
		resolvedAt := *h.ResolvedAt
		clone.ResolvedAt = &resolvedAt
	}

	if h.Resolution != nil {
		resolution := *h.Resolution
		clone.Resolution = &resolution
	}

	clone.MatchedHelpers = make([]MatchedHelper, len(h.MatchedHelpers))
	copy(clone.MatchedHelpers, h.MatchedHelpers)

	return &clone
}

// ══════════════════════════════════════════════════════════════════════════════
// ENTITY: ENDORSEMENT
// Благодарность за помощь — система социального капитала.
// ══════════════════════════════════════════════════════════════════════════════

// Endorsement - благодарность одного студента другому за помощь.
// Формирует репутацию помощника и стимулирует помогать.
type Endorsement struct {
	// ID - уникальный идентификатор (UUID).
	ID string

	// GiverID - кто даёт благодарность.
	GiverID StudentID

	// ReceiverID - кто получает благодарность.
	ReceiverID StudentID

	// HelpRequestID - за какой запрос помощи.
	HelpRequestID string

	// TaskID - по какой задаче была помощь.
	TaskID TaskID

	// Type - тип благодарности (clear, patient, etc).
	Type EndorsementType

	// Rating - числовая оценка (1-5).
	Rating Rating

	// Comment - текстовый комментарий (опционально).
	Comment string

	// IsPublic - показывать ли публично.
	IsPublic bool

	// CreatedAt - когда создана благодарность.
	CreatedAt time.Time
}

// NewEndorsementParams параметры для создания благодарности.
type NewEndorsementParams struct {
	ID            string
	GiverID       StudentID
	ReceiverID    StudentID
	HelpRequestID string
	TaskID        TaskID
	Type          EndorsementType
	Rating        Rating
	Comment       string
	IsPublic      bool
}

// NewEndorsement создаёт новую благодарность.
func NewEndorsement(params NewEndorsementParams) (*Endorsement, error) {
	if params.ID == "" {
		return nil, errors.New("endorsement id is required")
	}

	if !params.GiverID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	if !params.ReceiverID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	if params.GiverID == params.ReceiverID {
		return nil, ErrEndorsementSelfEndorse
	}

	if !params.Rating.IsValid() {
		return nil, ErrEndorsementInvalidRating
	}

	if params.Rating == 0 {
		return nil, errors.New("rating must be at least 1")
	}

	endorsementType := params.Type
	if endorsementType == "" {
		endorsementType = EndorsementTypeClear
	}
	if !endorsementType.IsValid() {
		return nil, errors.New("invalid endorsement type")
	}

	return &Endorsement{
		ID:            params.ID,
		GiverID:       params.GiverID,
		ReceiverID:    params.ReceiverID,
		HelpRequestID: params.HelpRequestID,
		TaskID:        params.TaskID,
		Type:          endorsementType,
		Rating:        params.Rating,
		Comment:       params.Comment,
		IsPublic:      params.IsPublic,
		CreatedAt:     time.Now().UTC(),
	}, nil
}

// IsPositive возвращает true, если оценка положительная (4+).
func (e *Endorsement) IsPositive() bool {
	return e.Rating >= 4
}

// String возвращает строковое представление.
func (e *Endorsement) String() string {
	return fmt.Sprintf(
		"Endorsement{ID: %s, %s -> %s, Type: %s, Rating: %.1f}",
		e.ID, e.GiverID, e.ReceiverID, e.Type, e.Rating,
	)
}

// Clone создаёт копию благодарности.
func (e *Endorsement) Clone() *Endorsement {
	if e == nil {
		return nil
	}
	clone := *e
	return &clone
}

// ══════════════════════════════════════════════════════════════════════════════
// AGGREGATE: SOCIAL PROFILE
// Агрегат социального профиля студента.
// ══════════════════════════════════════════════════════════════════════════════

// SocialProfile - социальный профиль студента (агрегат).
// Объединяет всю социальную информацию о студенте.
type SocialProfile struct {
	// StudentID - ID студента.
	StudentID StudentID

	// DisplayName - имя для отображения.
	DisplayName string

	// TotalConnections - общее количество связей.
	TotalConnections int

	// ActiveConnections - активные связи.
	ActiveConnections int

	// TotalHelpGiven - сколько раз помог.
	TotalHelpGiven int

	// TotalHelpReceived - сколько раз получил помощь.
	TotalHelpReceived int

	// AverageRating - средний рейтинг как помощника.
	AverageRating Rating

	// TotalEndorsements - общее количество благодарностей.
	TotalEndorsements int

	// TopEndorsementTypes - топ типов благодарностей.
	TopEndorsementTypes []EndorsementTypeStat

	// IsOpenToHelp - готов помогать.
	IsOpenToHelp bool

	// IsMentor - является ментором.
	IsMentor bool

	// SpecializedTasks - задачи, в которых силён.
	SpecializedTasks []TaskID

	// LastHelpAt - когда последний раз помогал.
	LastHelpAt *time.Time

	// UpdatedAt - когда обновлён профиль.
	UpdatedAt time.Time
}

// EndorsementTypeStat статистика по типу благодарности.
type EndorsementTypeStat struct {
	Type  EndorsementType
	Count int
}

// HelpScore возвращает "индекс полезности" студента (0-100).
// Используется для ранжирования помощников.
func (p *SocialProfile) HelpScore() int {
	if p.TotalHelpGiven == 0 {
		return 0
	}

	// Формула учитывает: количество помощей, рейтинг, разнообразие
	baseScore := float64(p.TotalHelpGiven) * 5
	ratingBonus := float64(p.AverageRating) * 10
	endorsementBonus := float64(p.TotalEndorsements) * 2

	score := baseScore + ratingBonus + endorsementBonus

	// Нормализуем до 100
	if score > 100 {
		score = 100
	}

	return int(score)
}

// String возвращает строковое представление.
func (p *SocialProfile) String() string {
	return fmt.Sprintf(
		"SocialProfile{Student: %s, HelpGiven: %d, Rating: %.1f, Score: %d}",
		p.StudentID, p.TotalHelpGiven, p.AverageRating, p.HelpScore(),
	)
}
