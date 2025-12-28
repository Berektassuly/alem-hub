package social

import (
	"errors"
	"sort"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// MATCHING PHILOSOPHY
//
// Философия подбора: "От конкуренции к сотрудничеству"
//
// При подборе помощников/менторов/напарников мы приоритизируем:
// 1. Социальные связи (уже помогали друг другу)
// 2. Совместимость (близкий уровень, схожие интересы)
// 3. Доступность (онлайн, готов помогать)
// 4. Репутация (рейтинг помощника)
//
// НЕ приоритизируем:
// - Чистый рейтинг XP (не хотим создавать иерархию)
// - Топовых игроков (они и так заняты)
// ══════════════════════════════════════════════════════════════════════════════

// ══════════════════════════════════════════════════════════════════════════════
// DOMAIN ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrNoMatchFound - подходящий кандидат не найден.
	ErrNoMatchFound = errors.New("no suitable match found")

	// ErrInsufficientCandidates - недостаточно кандидатов для подбора.
	ErrInsufficientCandidates = errors.New("insufficient candidates for matching")

	// ErrMatchAlreadyExists - связь уже существует.
	ErrMatchAlreadyExists = errors.New("match already exists")

	// ErrInvalidMatchCriteria - невалидные критерии подбора.
	ErrInvalidMatchCriteria = errors.New("invalid match criteria")

	// ErrStudentNotEligible - студент не подходит для подбора.
	ErrStudentNotEligible = errors.New("student not eligible for matching")
)

// ══════════════════════════════════════════════════════════════════════════════
// VALUE OBJECTS FOR MATCHING
// ══════════════════════════════════════════════════════════════════════════════

// MatchScore представляет оценку совместимости (0-100).
type MatchScore int

// IsValid проверяет корректность оценки.
func (m MatchScore) IsValid() bool {
	return m >= 0 && m <= 100
}

// Quality возвращает качественную оценку совместимости.
func (m MatchScore) Quality() MatchQuality {
	switch {
	case m >= 80:
		return MatchQualityExcellent
	case m >= 60:
		return MatchQualityGood
	case m >= 40:
		return MatchQualityFair
	case m >= 20:
		return MatchQualityPoor
	default:
		return MatchQualityNone
	}
}

// MatchQuality определяет качество подбора.
type MatchQuality string

const (
	// MatchQualityExcellent - отличная совместимость (80-100).
	MatchQualityExcellent MatchQuality = "excellent"

	// MatchQualityGood - хорошая совместимость (60-79).
	MatchQualityGood MatchQuality = "good"

	// MatchQualityFair - удовлетворительная совместимость (40-59).
	MatchQualityFair MatchQuality = "fair"

	// MatchQualityPoor - низкая совместимость (20-39).
	MatchQualityPoor MatchQuality = "poor"

	// MatchQualityNone - нет совместимости (0-19).
	MatchQualityNone MatchQuality = "none"
)

// MatchReason представляет причину совместимости.
type MatchReason struct {
	// Factor - название фактора.
	Factor string

	// Weight - вес фактора в итоговой оценке.
	Weight int

	// Score - оценка по данному фактору (0-100).
	Score int

	// Description - описание для пользователя.
	Description string
}

// ══════════════════════════════════════════════════════════════════════════════
// MENTOR MATCH
// Подбор ментора для студента.
// ══════════════════════════════════════════════════════════════════════════════

// MentorMatch представляет результат подбора ментора.
type MentorMatch struct {
	// ID - уникальный идентификатор подбора (UUID).
	ID string

	// MenteeID - кому ищем ментора.
	MenteeID StudentID

	// MentorID - предложенный ментор.
	MentorID StudentID

	// Score - оценка совместимости (0-100).
	Score MatchScore

	// Reasons - причины, почему ментор подходит.
	Reasons []MatchReason

	// Status - статус подбора.
	Status MentorMatchStatus

	// CreatedAt - когда создан подбор.
	CreatedAt time.Time

	// ExpiresAt - когда истекает предложение.
	ExpiresAt time.Time

	// AcceptedAt - когда принято (nil если не принято).
	AcceptedAt *time.Time

	// DeclinedAt - когда отклонено (nil если не отклонено).
	DeclinedAt *time.Time

	// DeclineReason - причина отклонения.
	DeclineReason string
}

// MentorMatchStatus определяет статус подбора ментора.
type MentorMatchStatus string

const (
	// MentorMatchStatusPending - ожидает ответа ментора.
	MentorMatchStatusPending MentorMatchStatus = "pending"

	// MentorMatchStatusAccepted - ментор принял.
	MentorMatchStatusAccepted MentorMatchStatus = "accepted"

	// MentorMatchStatusDeclined - ментор отклонил.
	MentorMatchStatusDeclined MentorMatchStatus = "declined"

	// MentorMatchStatusExpired - истекло время ответа.
	MentorMatchStatusExpired MentorMatchStatus = "expired"

	// MentorMatchStatusCancelled - отменено системой или студентом.
	MentorMatchStatusCancelled MentorMatchStatus = "cancelled"
)

// IsValid проверяет корректность статуса.
func (s MentorMatchStatus) IsValid() bool {
	switch s {
	case MentorMatchStatusPending, MentorMatchStatusAccepted, MentorMatchStatusDeclined,
		MentorMatchStatusExpired, MentorMatchStatusCancelled:
		return true
	default:
		return false
	}
}

// IsPending возвращает true, если подбор ожидает ответа.
func (s MentorMatchStatus) IsPending() bool {
	return s == MentorMatchStatusPending
}

// IsFinal возвращает true, если статус финальный.
func (s MentorMatchStatus) IsFinal() bool {
	return s == MentorMatchStatusAccepted || s == MentorMatchStatusDeclined ||
		s == MentorMatchStatusExpired || s == MentorMatchStatusCancelled
}

// NewMentorMatchParams параметры для создания подбора ментора.
type NewMentorMatchParams struct {
	ID       string
	MenteeID StudentID
	MentorID StudentID
	Score    MatchScore
	Reasons  []MatchReason
}

// NewMentorMatch создаёт новый подбор ментора.
func NewMentorMatch(params NewMentorMatchParams) (*MentorMatch, error) {
	if params.ID == "" {
		return nil, errors.New("mentor match id is required")
	}

	if !params.MenteeID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	if !params.MentorID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	if params.MenteeID == params.MentorID {
		return nil, errors.New("mentee and mentor cannot be the same")
	}

	if !params.Score.IsValid() {
		return nil, ErrInvalidMatchCriteria
	}

	now := time.Now().UTC()

	return &MentorMatch{
		ID:        params.ID,
		MenteeID:  params.MenteeID,
		MentorID:  params.MentorID,
		Score:     params.Score,
		Reasons:   params.Reasons,
		Status:    MentorMatchStatusPending,
		CreatedAt: now,
		ExpiresAt: now.Add(48 * time.Hour), // 48 часов на ответ
	}, nil
}

// Accept принимает предложение менторства.
func (m *MentorMatch) Accept() error {
	if m.Status.IsFinal() {
		return errors.New("match already finalized")
	}

	if time.Now().After(m.ExpiresAt) {
		m.Status = MentorMatchStatusExpired
		return errors.New("match expired")
	}

	now := time.Now().UTC()
	m.Status = MentorMatchStatusAccepted
	m.AcceptedAt = &now
	return nil
}

// Decline отклоняет предложение менторства.
func (m *MentorMatch) Decline(reason string) error {
	if m.Status.IsFinal() {
		return errors.New("match already finalized")
	}

	now := time.Now().UTC()
	m.Status = MentorMatchStatusDeclined
	m.DeclinedAt = &now
	m.DeclineReason = reason
	return nil
}

// Cancel отменяет подбор.
func (m *MentorMatch) Cancel() error {
	if m.Status.IsFinal() {
		return errors.New("match already finalized")
	}

	m.Status = MentorMatchStatusCancelled
	return nil
}

// MarkExpired помечает подбор как истёкший.
func (m *MentorMatch) MarkExpired() error {
	if m.Status.IsFinal() {
		return errors.New("match already finalized")
	}

	m.Status = MentorMatchStatusExpired
	return nil
}

// IsExpired проверяет, истёк ли срок.
func (m *MentorMatch) IsExpired() bool {
	return time.Now().After(m.ExpiresAt) && m.Status == MentorMatchStatusPending
}

// Quality возвращает качество подбора.
func (m *MentorMatch) Quality() MatchQuality {
	return m.Score.Quality()
}

// GetTopReasons возвращает топ N причин совместимости.
func (m *MentorMatch) GetTopReasons(n int) []MatchReason {
	if len(m.Reasons) == 0 {
		return nil
	}

	// Сортируем по вкладу (weight * score)
	sorted := make([]MatchReason, len(m.Reasons))
	copy(sorted, m.Reasons)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Weight*sorted[i].Score > sorted[j].Weight*sorted[j].Score
	})

	if n >= len(sorted) {
		return sorted
	}
	return sorted[:n]
}

// ══════════════════════════════════════════════════════════════════════════════
// STUDY BUDDY
// Напарник по учёбе — двусторонняя связь для совместной работы.
// ══════════════════════════════════════════════════════════════════════════════

// StudyBuddy представляет результат подбора напарника.
type StudyBuddy struct {
	// ID - уникальный идентификатор подбора (UUID).
	ID string

	// InitiatorID - кто ищет напарника.
	InitiatorID StudentID

	// BuddyID - предложенный напарник.
	BuddyID StudentID

	// Score - оценка совместимости (0-100).
	Score MatchScore

	// Reasons - причины, почему напарник подходит.
	Reasons []MatchReason

	// CommonTasks - общие задачи, над которыми можно работать.
	CommonTasks []TaskID

	// CompatibilityFactors - факторы совместимости.
	CompatibilityFactors CompatibilityFactors

	// Status - статус подбора.
	Status StudyBuddyStatus

	// CreatedAt - когда создан подбор.
	CreatedAt time.Time

	// ExpiresAt - когда истекает предложение.
	ExpiresAt time.Time

	// MutuallyAcceptedAt - когда оба приняли (nil если не приняли).
	MutuallyAcceptedAt *time.Time

	// InitiatorAccepted - инициатор принял.
	InitiatorAccepted bool

	// BuddyAccepted - напарник принял.
	BuddyAccepted bool
}

// CompatibilityFactors факторы совместимости напарников.
type CompatibilityFactors struct {
	// LevelDifference - разница в уровнях (0 = одинаковый уровень).
	LevelDifference int

	// XPDifference - разница в XP.
	XPDifference int

	// CommonTasksCount - количество общих решённых задач.
	CommonTasksCount int

	// OnlineOverlap - пересечение по времени онлайна (часы в неделю).
	OnlineOverlap int

	// SameCohort - из одной когорты.
	SameCohort bool

	// HavePreviousConnection - уже взаимодействовали ранее.
	HavePreviousConnection bool

	// SimilarProgress - схожий прогресс в программе.
	SimilarProgress bool
}

// StudyBuddyStatus определяет статус подбора напарника.
type StudyBuddyStatus string

const (
	// StudyBuddyStatusPending - ожидает ответа обоих.
	StudyBuddyStatusPending StudyBuddyStatus = "pending"

	// StudyBuddyStatusInitiatorAccepted - инициатор принял, ждём напарника.
	StudyBuddyStatusInitiatorAccepted StudyBuddyStatus = "initiator_accepted"

	// StudyBuddyStatusBuddyAccepted - напарник принял, ждём инициатора.
	StudyBuddyStatusBuddyAccepted StudyBuddyStatus = "buddy_accepted"

	// StudyBuddyStatusMutuallyAccepted - оба приняли.
	StudyBuddyStatusMutuallyAccepted StudyBuddyStatus = "mutually_accepted"

	// StudyBuddyStatusDeclined - кто-то отклонил.
	StudyBuddyStatusDeclined StudyBuddyStatus = "declined"

	// StudyBuddyStatusExpired - истекло время ответа.
	StudyBuddyStatusExpired StudyBuddyStatus = "expired"

	// StudyBuddyStatusCancelled - отменено.
	StudyBuddyStatusCancelled StudyBuddyStatus = "cancelled"
)

// IsValid проверяет корректность статуса.
func (s StudyBuddyStatus) IsValid() bool {
	switch s {
	case StudyBuddyStatusPending, StudyBuddyStatusInitiatorAccepted, StudyBuddyStatusBuddyAccepted,
		StudyBuddyStatusMutuallyAccepted, StudyBuddyStatusDeclined, StudyBuddyStatusExpired,
		StudyBuddyStatusCancelled:
		return true
	default:
		return false
	}
}

// IsPending возвращает true, если подбор ожидает ответа.
func (s StudyBuddyStatus) IsPending() bool {
	return s == StudyBuddyStatusPending || s == StudyBuddyStatusInitiatorAccepted ||
		s == StudyBuddyStatusBuddyAccepted
}

// IsFinal возвращает true, если статус финальный.
func (s StudyBuddyStatus) IsFinal() bool {
	return s == StudyBuddyStatusMutuallyAccepted || s == StudyBuddyStatusDeclined ||
		s == StudyBuddyStatusExpired || s == StudyBuddyStatusCancelled
}

// IsSuccess возвращает true, если подбор успешен.
func (s StudyBuddyStatus) IsSuccess() bool {
	return s == StudyBuddyStatusMutuallyAccepted
}

// NewStudyBuddyParams параметры для создания подбора напарника.
type NewStudyBuddyParams struct {
	ID                   string
	InitiatorID          StudentID
	BuddyID              StudentID
	Score                MatchScore
	Reasons              []MatchReason
	CommonTasks          []TaskID
	CompatibilityFactors CompatibilityFactors
}

// NewStudyBuddy создаёт новый подбор напарника.
func NewStudyBuddy(params NewStudyBuddyParams) (*StudyBuddy, error) {
	if params.ID == "" {
		return nil, errors.New("study buddy id is required")
	}

	if !params.InitiatorID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	if !params.BuddyID.IsValid() {
		return nil, ErrInvalidStudentID
	}

	if params.InitiatorID == params.BuddyID {
		return nil, errors.New("initiator and buddy cannot be the same")
	}

	if !params.Score.IsValid() {
		return nil, ErrInvalidMatchCriteria
	}

	now := time.Now().UTC()

	return &StudyBuddy{
		ID:                   params.ID,
		InitiatorID:          params.InitiatorID,
		BuddyID:              params.BuddyID,
		Score:                params.Score,
		Reasons:              params.Reasons,
		CommonTasks:          params.CommonTasks,
		CompatibilityFactors: params.CompatibilityFactors,
		Status:               StudyBuddyStatusPending,
		CreatedAt:            now,
		ExpiresAt:            now.Add(72 * time.Hour), // 72 часа на ответ
		InitiatorAccepted:    false,
		BuddyAccepted:        false,
	}, nil
}

// AcceptByInitiator регистрирует согласие инициатора.
func (s *StudyBuddy) AcceptByInitiator() error {
	if s.Status.IsFinal() {
		return errors.New("buddy match already finalized")
	}

	if time.Now().After(s.ExpiresAt) {
		s.Status = StudyBuddyStatusExpired
		return errors.New("buddy match expired")
	}

	s.InitiatorAccepted = true
	s.updateStatus()
	return nil
}

// AcceptByBuddy регистрирует согласие напарника.
func (s *StudyBuddy) AcceptByBuddy() error {
	if s.Status.IsFinal() {
		return errors.New("buddy match already finalized")
	}

	if time.Now().After(s.ExpiresAt) {
		s.Status = StudyBuddyStatusExpired
		return errors.New("buddy match expired")
	}

	s.BuddyAccepted = true
	s.updateStatus()
	return nil
}

// Accept регистрирует согласие студента (определяет по ID).
func (s *StudyBuddy) Accept(studentID StudentID) error {
	if studentID == s.InitiatorID {
		return s.AcceptByInitiator()
	}
	if studentID == s.BuddyID {
		return s.AcceptByBuddy()
	}
	return errors.New("student is not part of this buddy match")
}

// updateStatus обновляет статус на основе принятий.
func (s *StudyBuddy) updateStatus() {
	if s.InitiatorAccepted && s.BuddyAccepted {
		now := time.Now().UTC()
		s.Status = StudyBuddyStatusMutuallyAccepted
		s.MutuallyAcceptedAt = &now
	} else if s.InitiatorAccepted {
		s.Status = StudyBuddyStatusInitiatorAccepted
	} else if s.BuddyAccepted {
		s.Status = StudyBuddyStatusBuddyAccepted
	}
}

// Decline отклоняет предложение.
func (s *StudyBuddy) Decline(studentID StudentID) error {
	if s.Status.IsFinal() {
		return errors.New("buddy match already finalized")
	}

	if studentID != s.InitiatorID && studentID != s.BuddyID {
		return errors.New("student is not part of this buddy match")
	}

	s.Status = StudyBuddyStatusDeclined
	return nil
}

// Cancel отменяет подбор.
func (s *StudyBuddy) Cancel() error {
	if s.Status.IsFinal() {
		return errors.New("buddy match already finalized")
	}

	s.Status = StudyBuddyStatusCancelled
	return nil
}

// MarkExpired помечает подбор как истёкший.
func (s *StudyBuddy) MarkExpired() error {
	if s.Status.IsFinal() {
		return errors.New("buddy match already finalized")
	}

	s.Status = StudyBuddyStatusExpired
	return nil
}

// IsExpired проверяет, истёк ли срок.
func (s *StudyBuddy) IsExpired() bool {
	return time.Now().After(s.ExpiresAt) && s.Status.IsPending()
}

// InvolvesStudent проверяет, участвует ли студент.
func (s *StudyBuddy) InvolvesStudent(studentID StudentID) bool {
	return s.InitiatorID == studentID || s.BuddyID == studentID
}

// GetOtherStudent возвращает ID другого участника.
func (s *StudyBuddy) GetOtherStudent(studentID StudentID) StudentID {
	if s.InitiatorID == studentID {
		return s.BuddyID
	}
	return s.InitiatorID
}

// Quality возвращает качество подбора.
func (s *StudyBuddy) Quality() MatchQuality {
	return s.Score.Quality()
}

// ══════════════════════════════════════════════════════════════════════════════
// MATCHING CRITERIA
// Критерии для подбора кандидатов.
// ══════════════════════════════════════════════════════════════════════════════

// MentorCriteria критерии для подбора ментора.
type MentorCriteria struct {
	// MenteeID - для кого ищем ментора.
	MenteeID StudentID

	// PreferredTasks - задачи, по которым нужна помощь.
	PreferredTasks []TaskID

	// MinMentorLevel - минимальный уровень ментора.
	MinMentorLevel int

	// MaxMentorLevel - максимальный уровень ментора (0 = без ограничений).
	MaxMentorLevel int

	// PreferOnline - предпочитать онлайн менторов.
	PreferOnline bool

	// PreferSameCohort - предпочитать менторов из той же когорты.
	PreferSameCohort bool

	// PreferPreviousHelpers - предпочитать тех, кто уже помогал.
	PreferPreviousHelpers bool

	// MinRating - минимальный рейтинг ментора.
	MinRating Rating

	// MaxCandidates - максимальное количество кандидатов для оценки.
	MaxCandidates int

	// ExcludeStudentIDs - исключить студентов (уже отклонённые и т.д.).
	ExcludeStudentIDs []StudentID
}

// DefaultMentorCriteria возвращает критерии по умолчанию.
func DefaultMentorCriteria(menteeID StudentID) MentorCriteria {
	return MentorCriteria{
		MenteeID:              menteeID,
		MinMentorLevel:        0,
		MaxMentorLevel:        0,
		PreferOnline:          true,
		PreferSameCohort:      true,
		PreferPreviousHelpers: true,
		MinRating:             3.0,
		MaxCandidates:         50,
		ExcludeStudentIDs:     make([]StudentID, 0),
	}
}

// Validate проверяет корректность критериев.
func (c MentorCriteria) Validate() error {
	if !c.MenteeID.IsValid() {
		return ErrInvalidStudentID
	}

	if c.MaxMentorLevel > 0 && c.MinMentorLevel > c.MaxMentorLevel {
		return ErrInvalidMatchCriteria
	}

	if !c.MinRating.IsValid() {
		return ErrInvalidMatchCriteria
	}

	return nil
}

// StudyBuddyCriteria критерии для подбора напарника.
type StudyBuddyCriteria struct {
	// StudentID - для кого ищем напарника.
	StudentID StudentID

	// CurrentTasks - текущие задачи студента.
	CurrentTasks []TaskID

	// MaxLevelDifference - максимальная разница в уровнях.
	MaxLevelDifference int

	// MaxXPDifference - максимальная разница в XP.
	MaxXPDifference int

	// PreferOnline - предпочитать онлайн студентов.
	PreferOnline bool

	// PreferSameCohort - предпочитать студентов из той же когорты.
	PreferSameCohort bool

	// PreferSimilarProgress - предпочитать студентов со схожим прогрессом.
	PreferSimilarProgress bool

	// MinOnlineOverlap - минимальное пересечение по времени (часы в неделю).
	MinOnlineOverlap int

	// MaxCandidates - максимальное количество кандидатов для оценки.
	MaxCandidates int

	// ExcludeStudentIDs - исключить студентов.
	ExcludeStudentIDs []StudentID
}

// DefaultStudyBuddyCriteria возвращает критерии по умолчанию.
func DefaultStudyBuddyCriteria(studentID StudentID) StudyBuddyCriteria {
	return StudyBuddyCriteria{
		StudentID:             studentID,
		MaxLevelDifference:    3,
		MaxXPDifference:       2000,
		PreferOnline:          true,
		PreferSameCohort:      true,
		PreferSimilarProgress: true,
		MinOnlineOverlap:      5,
		MaxCandidates:         50,
		ExcludeStudentIDs:     make([]StudentID, 0),
	}
}

// Validate проверяет корректность критериев.
func (c StudyBuddyCriteria) Validate() error {
	if !c.StudentID.IsValid() {
		return ErrInvalidStudentID
	}

	if c.MaxLevelDifference < 0 {
		return ErrInvalidMatchCriteria
	}

	if c.MaxXPDifference < 0 {
		return ErrInvalidMatchCriteria
	}

	return nil
}

// HelperCriteria критерии для подбора помощника по задаче.
type HelperCriteria struct {
	// RequesterID - кто ищет помощь.
	RequesterID StudentID

	// TaskID - по какой задаче нужна помощь.
	TaskID TaskID

	// TaskName - название задачи.
	TaskName string

	// PreferOnline - предпочитать онлайн помощников.
	PreferOnline bool

	// PreferHighRating - предпочитать помощников с высоким рейтингом.
	PreferHighRating bool

	// PreferPreviousHelpers - предпочитать тех, кто уже помогал.
	PreferPreviousHelpers bool

	// PreferSameCohort - предпочитать помощников из той же когорты.
	PreferSameCohort bool

	// MinRating - минимальный рейтинг помощника.
	MinRating Rating

	// MaxCandidates - максимальное количество кандидатов.
	MaxCandidates int

	// ExcludeStudentIDs - исключить студентов.
	ExcludeStudentIDs []StudentID
}

// DefaultHelperCriteria возвращает критерии по умолчанию.
func DefaultHelperCriteria(requesterID StudentID, taskID TaskID) HelperCriteria {
	return HelperCriteria{
		RequesterID:           requesterID,
		TaskID:                taskID,
		PreferOnline:          true,
		PreferHighRating:      true,
		PreferPreviousHelpers: true,
		PreferSameCohort:      false, // Помощь важнее когорты
		MinRating:             2.0,   // Низкий порог для большего выбора
		MaxCandidates:         30,
		ExcludeStudentIDs:     make([]StudentID, 0),
	}
}

// Validate проверяет корректность критериев.
func (c HelperCriteria) Validate() error {
	if !c.RequesterID.IsValid() {
		return ErrInvalidStudentID
	}

	if !c.TaskID.IsValid() {
		return ErrInvalidTaskID
	}

	if !c.MinRating.IsValid() {
		return ErrInvalidMatchCriteria
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// CANDIDATE
// Кандидат для подбора.
// ══════════════════════════════════════════════════════════════════════════════

// Candidate представляет кандидата для подбора.
type Candidate struct {
	// StudentID - ID студента.
	StudentID StudentID

	// DisplayName - имя для отображения.
	DisplayName string

	// Level - уровень студента.
	Level int

	// XP - текущий XP.
	XP int

	// Cohort - когорта студента.
	Cohort string

	// HelpRating - рейтинг как помощника.
	HelpRating Rating

	// HelpCount - количество оказанных помощей.
	HelpCount int

	// IsOnline - онлайн ли сейчас.
	IsOnline bool

	// LastSeenAt - когда был онлайн.
	LastSeenAt time.Time

	// SolvedTasks - решённые задачи.
	SolvedTasks []TaskID

	// HasPreviousConnection - было ли предыдущее взаимодействие.
	HasPreviousConnection bool

	// PreviousConnectionType - тип предыдущей связи.
	PreviousConnectionType ConnectionType

	// IsOpenToHelp - готов помогать.
	IsOpenToHelp bool

	// IsMentor - является ментором.
	IsMentor bool
}

// HasSolvedTask проверяет, решил ли кандидат задачу.
func (c Candidate) HasSolvedTask(taskID TaskID) bool {
	for _, t := range c.SolvedTasks {
		if t == taskID {
			return true
		}
	}
	return false
}

// CommonTasks возвращает общие решённые задачи.
func (c Candidate) CommonTasks(tasks []TaskID) []TaskID {
	common := make([]TaskID, 0)
	for _, t := range tasks {
		if c.HasSolvedTask(t) {
			common = append(common, t)
		}
	}
	return common
}

// MinutesSinceLastSeen возвращает минуты с последнего визита.
func (c Candidate) MinutesSinceLastSeen() int {
	return int(time.Since(c.LastSeenAt).Minutes())
}

// ══════════════════════════════════════════════════════════════════════════════
// MATCHING RESULT
// Результат подбора.
// ══════════════════════════════════════════════════════════════════════════════

// MatchResult представляет результат подбора кандидата.
type MatchResult struct {
	// Candidate - кандидат.
	Candidate Candidate

	// Score - оценка совместимости (0-100).
	Score MatchScore

	// Reasons - причины совместимости.
	Reasons []MatchReason

	// RankPosition - позиция в общем рейтинге кандидатов.
	RankPosition int
}

// MatchResultList список результатов подбора с методами для работы.
type MatchResultList []MatchResult

// Len возвращает длину списка.
func (m MatchResultList) Len() int {
	return len(m)
}

// Less сравнивает по оценке совместимости.
func (m MatchResultList) Less(i, j int) bool {
	return m[i].Score > m[j].Score // Сортировка по убыванию
}

// Swap меняет элементы местами.
func (m MatchResultList) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// Sort сортирует по оценке совместимости.
func (m MatchResultList) Sort() {
	sort.Sort(m)
	// Обновляем позиции после сортировки
	for i := range m {
		m[i].RankPosition = i + 1
	}
}

// TopN возвращает топ N результатов.
func (m MatchResultList) TopN(n int) MatchResultList {
	if n >= len(m) {
		return m
	}
	return m[:n]
}

// FilterByMinScore фильтрует по минимальной оценке.
func (m MatchResultList) FilterByMinScore(minScore MatchScore) MatchResultList {
	filtered := make(MatchResultList, 0)
	for _, result := range m {
		if result.Score >= minScore {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// FilterByQuality фильтрует по качеству.
func (m MatchResultList) FilterByQuality(minQuality MatchQuality) MatchResultList {
	minScore := MatchScore(0)
	switch minQuality {
	case MatchQualityExcellent:
		minScore = 80
	case MatchQualityGood:
		minScore = 60
	case MatchQualityFair:
		minScore = 40
	case MatchQualityPoor:
		minScore = 20
	}
	return m.FilterByMinScore(minScore)
}

// ══════════════════════════════════════════════════════════════════════════════
// WEIGHT CONFIGURATION
// Конфигурация весов для алгоритмов подбора.
// ══════════════════════════════════════════════════════════════════════════════

// MatchWeights веса факторов для расчёта оценки совместимости.
type MatchWeights struct {
	// OnlineBonus - бонус за онлайн статус.
	OnlineBonus int

	// RecentActivityBonus - бонус за недавнюю активность.
	RecentActivityBonus int

	// HighRatingBonus - бонус за высокий рейтинг.
	HighRatingBonus int

	// PreviousConnectionBonus - бонус за предыдущую связь.
	PreviousConnectionBonus int

	// SameCohortBonus - бонус за ту же когорту.
	SameCohortBonus int

	// TaskSolvedBase - базовые очки за решённую задачу.
	TaskSolvedBase int

	// HelpCountMultiplier - множитель за количество помощей.
	HelpCountMultiplier int

	// LevelDifferenceMax - максимальная разница в уровнях для штрафа.
	LevelDifferenceMax int

	// LevelDifferencePenalty - штраф за разницу в уровнях.
	LevelDifferencePenalty int
}

// DefaultMatchWeights возвращает веса по умолчанию.
func DefaultMatchWeights() MatchWeights {
	return MatchWeights{
		OnlineBonus:             15,
		RecentActivityBonus:     10,
		HighRatingBonus:         20,
		PreviousConnectionBonus: 15,
		SameCohortBonus:         10,
		TaskSolvedBase:          30,
		HelpCountMultiplier:     2,
		LevelDifferenceMax:      5,
		LevelDifferencePenalty:  5,
	}
}

// MentorMatchWeights возвращает веса для подбора ментора.
func MentorMatchWeights() MatchWeights {
	weights := DefaultMatchWeights()
	weights.HighRatingBonus = 25       // Рейтинг важнее для ментора
	weights.TaskSolvedBase = 20        // Решённая задача менее важна
	weights.LevelDifferencePenalty = 0 // Разница в уровнях не наказывается
	return weights
}

// StudyBuddyMatchWeights возвращает веса для подбора напарника.
func StudyBuddyMatchWeights() MatchWeights {
	weights := DefaultMatchWeights()
	weights.SameCohortBonus = 20         // Когорта важнее для напарника
	weights.LevelDifferencePenalty = 8   // Большой штраф за разницу в уровнях
	weights.PreviousConnectionBonus = 25 // Предыдущая связь очень важна
	return weights
}
