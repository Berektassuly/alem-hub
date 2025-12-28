// Package student содержит доменную модель студента Alem School.
// Это ядро бизнес-логики - здесь нет внешних зависимостей.
package student

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// VALUE OBJECTS
// ══════════════════════════════════════════════════════════════════════════════

// TelegramID представляет уникальный идентификатор пользователя Telegram.
type TelegramID int64

// IsValid проверяет, что TelegramID положительный.
func (t TelegramID) IsValid() bool {
	return t > 0
}

// AlemLogin представляет логин студента на платформе Alem.
type AlemLogin string

// IsValid проверяет корректность логина Alem.
func (a AlemLogin) IsValid() bool {
	s := string(a)
	return len(s) >= 2 && len(s) <= 50 && !strings.ContainsAny(s, " \t\n\r")
}

// String возвращает строковое представление логина.
func (a AlemLogin) String() string {
	return string(a)
}

// XP представляет очки опыта студента.
type XP int

// IsValid проверяет, что XP неотрицательный.
func (x XP) IsValid() bool {
	return x >= 0
}

// Add складывает XP.
func (x XP) Add(delta XP) XP {
	return x + delta
}

// Diff вычисляет разницу между двумя значениями XP.
func (x XP) Diff(other XP) XP {
	return x - other
}

// Level представляет уровень студента, вычисляемый из XP.
type Level int

// CalculateLevel вычисляет уровень на основе XP.
// Формула: каждые 1000 XP = 1 уровень (можно настроить).
func CalculateLevel(xp XP) Level {
	if xp < 0 {
		return 0
	}
	return Level(xp / 1000)
}

// Cohort представляет поток студентов (например, "2024-spring").
type Cohort string

// IsValid проверяет корректность когорты.
func (c Cohort) IsValid() bool {
	s := string(c)
	return len(s) >= 4 && len(s) <= 30
}

// String возвращает строковое представление когорты.
func (c Cohort) String() string {
	return string(c)
}

// ══════════════════════════════════════════════════════════════════════════════
// ENUMS
// ══════════════════════════════════════════════════════════════════════════════

// Status определяет текущий статус студента в программе.
type Status string

const (
	// StatusActive - студент активно учится.
	StatusActive Status = "active"
	// StatusInactive - студент неактивен (не заходил более N дней).
	StatusInactive Status = "inactive"
	// StatusGraduated - студент успешно закончил программу.
	StatusGraduated Status = "graduated"
	// StatusLeft - студент покинул программу.
	StatusLeft Status = "left"
	// StatusSuspended - студент временно отстранён.
	StatusSuspended Status = "suspended"
)

// IsValid проверяет, что статус корректен.
func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusInactive, StatusGraduated, StatusLeft, StatusSuspended:
		return true
	default:
		return false
	}
}

// CanReceiveNotifications возвращает true, если студенту можно слать уведомления.
func (s Status) CanReceiveNotifications() bool {
	return s == StatusActive || s == StatusInactive
}

// IsEnrolled возвращает true, если студент всё ещё в программе.
func (s Status) IsEnrolled() bool {
	return s == StatusActive || s == StatusInactive
}

// OnlineState определяет текущее состояние онлайн-присутствия студента.
type OnlineState string

const (
	// OnlineStateOnline - студент сейчас онлайн (активен в последние 5 минут).
	OnlineStateOnline OnlineState = "online"
	// OnlineStateAway - студент отошёл (неактивен 5-30 минут).
	OnlineStateAway OnlineState = "away"
	// OnlineStateOffline - студент оффлайн.
	OnlineStateOffline OnlineState = "offline"
)

// IsAvailable возвращает true, если к студенту можно обратиться за помощью.
func (o OnlineState) IsAvailable() bool {
	return o == OnlineStateOnline || o == OnlineStateAway
}

// ══════════════════════════════════════════════════════════════════════════════
// MAIN ENTITY: STUDENT
// ══════════════════════════════════════════════════════════════════════════════

// Student - центральная сущность системы, представляющая студента Alem School.
type Student struct {
	// ID - внутренний уникальный идентификатор (UUID в строковом формате).
	ID string

	// TelegramID - идентификатор пользователя в Telegram.
	TelegramID TelegramID

	// AlemLogin - логин на платформе Alem.
	AlemLogin AlemLogin

	// DisplayName - отображаемое имя (может отличаться от логина).
	DisplayName string

	// CurrentXP - текущее количество очков опыта.
	CurrentXP XP

	// Cohort - поток, к которому принадлежит студент.
	Cohort Cohort

	// Status - текущий статус в программе.
	Status Status

	// OnlineState - текущее состояние онлайн-присутствия.
	OnlineState OnlineState

	// LastSeenAt - время последней активности.
	LastSeenAt time.Time

	// LastSyncedAt - время последней синхронизации с Alem API.
	LastSyncedAt time.Time

	// JoinedAt - время регистрации в боте.
	JoinedAt time.Time

	// Preferences - настройки уведомлений студента.
	Preferences NotificationPreferences

	// HelpRating - средний рейтинг как помощника (0.0 - 5.0).
	HelpRating float64

	// HelpCount - количество оказанных помощей.
	HelpCount int

	// CreatedAt - время создания записи.
	CreatedAt time.Time

	// UpdatedAt - время последнего обновления.
	UpdatedAt time.Time
}

// NotificationPreferences содержит настройки уведомлений студента.
type NotificationPreferences struct {
	// RankChanges - уведомлять об изменении позиции в рейтинге.
	RankChanges bool

	// DailyDigest - отправлять ежедневную сводку.
	DailyDigest bool

	// HelpRequests - уведомлять о запросах помощи по решённым задачам.
	HelpRequests bool

	// InactivityReminders - напоминать о неактивности.
	InactivityReminders bool

	// QuietHoursStart - начало тихого времени (часы, 0-23).
	QuietHoursStart int

	// QuietHoursEnd - конец тихого времени (часы, 0-23).
	QuietHoursEnd int
}

// DefaultNotificationPreferences возвращает настройки по умолчанию.
func DefaultNotificationPreferences() NotificationPreferences {
	return NotificationPreferences{
		RankChanges:         true,
		DailyDigest:         true,
		HelpRequests:        true,
		InactivityReminders: true,
		QuietHoursStart:     23, // 23:00 - 08:00 тихие часы
		QuietHoursEnd:       8,
	}
}

// IsQuietHour проверяет, попадает ли указанное время в тихие часы.
func (p NotificationPreferences) IsQuietHour(t time.Time) bool {
	hour := t.Hour()
	if p.QuietHoursStart < p.QuietHoursEnd {
		// Простой случай: например, 1:00 - 6:00
		return hour >= p.QuietHoursStart && hour < p.QuietHoursEnd
	}
	// Через полночь: например, 23:00 - 8:00
	return hour >= p.QuietHoursStart || hour < p.QuietHoursEnd
}

// ══════════════════════════════════════════════════════════════════════════════
// DOMAIN ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrInvalidTelegramID - невалидный Telegram ID.
	ErrInvalidTelegramID = errors.New("invalid telegram id: must be positive")

	// ErrInvalidAlemLogin - невалидный логин Alem.
	ErrInvalidAlemLogin = errors.New("invalid alem login: must be 2-50 chars without whitespace")

	// ErrInvalidXP - невалидное значение XP.
	ErrInvalidXP = errors.New("invalid xp: must be non-negative")

	// ErrInvalidCohort - невалидная когорта.
	ErrInvalidCohort = errors.New("invalid cohort: must be 4-30 chars")

	// ErrInvalidStatus - невалидный статус.
	ErrInvalidStatus = errors.New("invalid student status")

	// ErrInvalidDisplayName - невалидное отображаемое имя.
	ErrInvalidDisplayName = errors.New("invalid display name: must be 1-100 chars")

	// ErrInvalidHelpRating - невалидный рейтинг помощника.
	ErrInvalidHelpRating = errors.New("invalid help rating: must be between 0.0 and 5.0")

	// ErrStudentNotFound - студент не найден.
	ErrStudentNotFound = errors.New("student not found")

	// ErrStudentAlreadyExists - студент уже существует.
	ErrStudentAlreadyExists = errors.New("student already exists")

	// ErrStudentNotEnrolled - студент больше не в программе.
	ErrStudentNotEnrolled = errors.New("student is not enrolled in the program")
)

// ══════════════════════════════════════════════════════════════════════════════
// FACTORY & VALIDATION
// ══════════════════════════════════════════════════════════════════════════════

// NewStudentParams содержит параметры для создания нового студента.
type NewStudentParams struct {
	ID          string
	TelegramID  TelegramID
	AlemLogin   AlemLogin
	DisplayName string
	Cohort      Cohort
	InitialXP   XP
}

// NewStudent создаёт нового студента с валидацией всех полей.
func NewStudent(params NewStudentParams) (*Student, error) {
	if params.ID == "" {
		return nil, errors.New("student id is required")
	}

	if !params.TelegramID.IsValid() {
		return nil, ErrInvalidTelegramID
	}

	if !params.AlemLogin.IsValid() {
		return nil, ErrInvalidAlemLogin
	}

	displayName := strings.TrimSpace(params.DisplayName)
	if len(displayName) == 0 || len(displayName) > 100 {
		return nil, ErrInvalidDisplayName
	}

	if !params.Cohort.IsValid() {
		return nil, ErrInvalidCohort
	}

	if !params.InitialXP.IsValid() {
		return nil, ErrInvalidXP
	}

	now := time.Now().UTC()

	return &Student{
		ID:           params.ID,
		TelegramID:   params.TelegramID,
		AlemLogin:    params.AlemLogin,
		DisplayName:  displayName,
		CurrentXP:    params.InitialXP,
		Cohort:       params.Cohort,
		Status:       StatusActive,
		OnlineState:  OnlineStateOffline,
		LastSeenAt:   now,
		LastSyncedAt: now,
		JoinedAt:     now,
		Preferences:  DefaultNotificationPreferences(),
		HelpRating:   0.0,
		HelpCount:    0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// DOMAIN METHODS (Business Logic)
// ══════════════════════════════════════════════════════════════════════════════

// Level возвращает текущий уровень студента.
func (s *Student) Level() Level {
	return CalculateLevel(s.CurrentXP)
}

// UpdateXP обновляет XP студента и возвращает дельту изменения.
// Возвращает ошибку, если новый XP невалиден.
func (s *Student) UpdateXP(newXP XP) (delta XP, err error) {
	if !newXP.IsValid() {
		return 0, ErrInvalidXP
	}

	delta = newXP.Diff(s.CurrentXP)
	s.CurrentXP = newXP
	s.UpdatedAt = time.Now().UTC()

	return delta, nil
}

// MarkOnline обновляет состояние студента на "онлайн".
func (s *Student) MarkOnline() {
	s.OnlineState = OnlineStateOnline
	s.LastSeenAt = time.Now().UTC()
	s.UpdatedAt = s.LastSeenAt

	// Если был неактивен, возвращаем в активное состояние
	if s.Status == StatusInactive {
		s.Status = StatusActive
	}
}

// MarkAway обновляет состояние студента на "отошёл".
func (s *Student) MarkAway() {
	s.OnlineState = OnlineStateAway
	s.UpdatedAt = time.Now().UTC()
}

// MarkOffline обновляет состояние студента на "оффлайн".
func (s *Student) MarkOffline() {
	s.OnlineState = OnlineStateOffline
	s.UpdatedAt = time.Now().UTC()
}

// UpdateOnlineState обновляет состояние на основе времени последней активности.
func (s *Student) UpdateOnlineState() {
	elapsed := time.Since(s.LastSeenAt)

	switch {
	case elapsed < 5*time.Minute:
		s.OnlineState = OnlineStateOnline
	case elapsed < 30*time.Minute:
		s.OnlineState = OnlineStateAway
	default:
		s.OnlineState = OnlineStateOffline
	}
}

// MarkInactive помечает студента как неактивного.
// Вызывается, когда студент не заходил более N дней.
func (s *Student) MarkInactive() error {
	if !s.Status.IsEnrolled() {
		return ErrStudentNotEnrolled
	}

	s.Status = StatusInactive
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkGraduated помечает студента как выпускника.
func (s *Student) MarkGraduated() error {
	if !s.Status.IsEnrolled() {
		return ErrStudentNotEnrolled
	}

	s.Status = StatusGraduated
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// Leave помечает студента как покинувшего программу.
func (s *Student) Leave() error {
	if !s.Status.IsEnrolled() {
		return ErrStudentNotEnrolled
	}

	s.Status = StatusLeft
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// Suspend временно отстраняет студента.
func (s *Student) Suspend() error {
	if !s.Status.IsEnrolled() {
		return ErrStudentNotEnrolled
	}

	s.Status = StatusSuspended
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// Reinstate восстанавливает отстранённого студента.
func (s *Student) Reinstate() error {
	if s.Status != StatusSuspended {
		return errors.New("can only reinstate suspended students")
	}

	s.Status = StatusActive
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// UpdatePreferences обновляет настройки уведомлений.
func (s *Student) UpdatePreferences(prefs NotificationPreferences) {
	s.Preferences = prefs
	s.UpdatedAt = time.Now().UTC()
}

// AddHelpRating добавляет оценку за помощь и пересчитывает средний рейтинг.
func (s *Student) AddHelpRating(rating float64) error {
	if rating < 0.0 || rating > 5.0 {
		return ErrInvalidHelpRating
	}

	// Пересчёт среднего рейтинга
	totalRating := s.HelpRating * float64(s.HelpCount)
	s.HelpCount++
	s.HelpRating = (totalRating + rating) / float64(s.HelpCount)
	s.UpdatedAt = time.Now().UTC()

	return nil
}

// CanHelp проверяет, может ли студент помогать другим.
func (s *Student) CanHelp() bool {
	return s.Status.IsEnrolled() && s.Preferences.HelpRequests
}

// CanReceiveNotification проверяет, можно ли отправить уведомление сейчас.
func (s *Student) CanReceiveNotification(notificationType string, at time.Time) bool {
	if !s.Status.CanReceiveNotifications() {
		return false
	}

	// Проверяем тихие часы
	if s.Preferences.IsQuietHour(at) {
		return false
	}

	// Проверяем настройки для конкретного типа уведомлений
	switch notificationType {
	case "rank_change":
		return s.Preferences.RankChanges
	case "daily_digest":
		return s.Preferences.DailyDigest
	case "help_request":
		return s.Preferences.HelpRequests
	case "inactivity_reminder":
		return s.Preferences.InactivityReminders
	default:
		return true
	}
}

// SyncedWith обновляет время последней синхронизации.
func (s *Student) SyncedWith(syncTime time.Time) {
	s.LastSyncedAt = syncTime
	s.UpdatedAt = time.Now().UTC()
}

// DaysSinceLastSeen возвращает количество дней с последнего визита.
func (s *Student) DaysSinceLastSeen() int {
	return int(time.Since(s.LastSeenAt).Hours() / 24)
}

// IsNewbie возвращает true, если студент в программе менее 7 дней.
func (s *Student) IsNewbie() bool {
	return time.Since(s.JoinedAt) < 7*24*time.Hour
}

// IsVeteran возвращает true, если студент в программе более 30 дней.
func (s *Student) IsVeteran() bool {
	return time.Since(s.JoinedAt) > 30*24*time.Hour
}

// String возвращает строковое представление студента для логирования.
func (s *Student) String() string {
	return fmt.Sprintf(
		"Student{ID: %s, Login: %s, XP: %d, Level: %d, Status: %s}",
		s.ID, s.AlemLogin, s.CurrentXP, s.Level(), s.Status,
	)
}

// Clone создаёт глубокую копию студента.
func (s *Student) Clone() *Student {
	if s == nil {
		return nil
	}

	clone := *s
	return &clone
}
