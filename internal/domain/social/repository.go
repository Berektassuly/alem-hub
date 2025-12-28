package social

import (
	"context"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// REPOSITORY INTERFACES
// Эти интерфейсы определяют контракт для работы с хранилищем данных.
// Реализации находятся в infrastructure/persistence.
//
// Принципы:
// - Dependency Inversion: Domain определяет интерфейсы, Infrastructure реализует
// - Разделение по агрегатам: каждый агрегат имеет свой репозиторий
// - CQRS-ready: методы разделены на команды (изменение) и запросы (чтение)
// ══════════════════════════════════════════════════════════════════════════════

// ══════════════════════════════════════════════════════════════════════════════
// CONNECTION REPOSITORY
// Работа со связями между студентами.
// ══════════════════════════════════════════════════════════════════════════════

// ConnectionRepository определяет операции для работы со связями.
type ConnectionRepository interface {
	// ─────────────────────────────────────────────────────────────────────────
	// CRUD Operations
	// ─────────────────────────────────────────────────────────────────────────

	// Create создаёт новую связь.
	// Возвращает ErrConnectionAlreadyExists, если связь уже существует.
	Create(ctx context.Context, conn *Connection) error

	// GetByID возвращает связь по ID.
	// Возвращает ErrConnectionNotFound, если связь не найдена.
	GetByID(ctx context.Context, id string) (*Connection, error)

	// Update обновляет данные связи.
	// Возвращает ErrConnectionNotFound, если связь не найдена.
	Update(ctx context.Context, conn *Connection) error

	// Delete удаляет связь (soft delete).
	// Возвращает ErrConnectionNotFound, если связь не найдена.
	Delete(ctx context.Context, id string) error

	// ─────────────────────────────────────────────────────────────────────────
	// Query Operations
	// ─────────────────────────────────────────────────────────────────────────

	// GetByStudents возвращает связь между двумя студентами.
	// Проверяет в обоих направлениях.
	GetByStudents(ctx context.Context, student1, student2 StudentID) (*Connection, error)

	// GetByStudentID возвращает все связи студента.
	GetByStudentID(ctx context.Context, studentID StudentID, opts ConnectionListOptions) ([]*Connection, error)

	// GetActiveByStudentID возвращает активные связи студента.
	GetActiveByStudentID(ctx context.Context, studentID StudentID) ([]*Connection, error)

	// GetPendingByStudentID возвращает ожидающие связи студента.
	// Включает как входящие, так и исходящие запросы.
	GetPendingByStudentID(ctx context.Context, studentID StudentID) ([]*Connection, error)

	// GetIncomingPending возвращает входящие запросы на связь.
	GetIncomingPending(ctx context.Context, studentID StudentID) ([]*Connection, error)

	// GetOutgoingPending возвращает исходящие запросы на связь.
	GetOutgoingPending(ctx context.Context, studentID StudentID) ([]*Connection, error)

	// GetByType возвращает связи определённого типа для студента.
	GetByType(ctx context.Context, studentID StudentID, connType ConnectionType) ([]*Connection, error)

	// GetByStatus возвращает связи с определённым статусом.
	GetByStatus(ctx context.Context, status ConnectionStatus, opts ConnectionListOptions) ([]*Connection, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Existence Checks
	// ─────────────────────────────────────────────────────────────────────────

	// Exists проверяет существование связи по ID.
	Exists(ctx context.Context, id string) (bool, error)

	// ExistsBetweenStudents проверяет существование связи между студентами.
	ExistsBetweenStudents(ctx context.Context, student1, student2 StudentID) (bool, error)

	// ExistsActiveConnection проверяет существование активной связи.
	ExistsActiveConnection(ctx context.Context, student1, student2 StudentID) (bool, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Aggregations
	// ─────────────────────────────────────────────────────────────────────────

	// CountByStudentID возвращает количество связей студента.
	CountByStudentID(ctx context.Context, studentID StudentID) (int, error)

	// CountActiveByStudentID возвращает количество активных связей.
	CountActiveByStudentID(ctx context.Context, studentID StudentID) (int, error)

	// CountByType возвращает количество связей по типу.
	CountByType(ctx context.Context, studentID StudentID, connType ConnectionType) (int, error)

	// GetConnectionStats возвращает статистику связей студента.
	GetConnectionStats(ctx context.Context, studentID StudentID) (*ConnectionStatsAggregate, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Bulk Operations
	// ─────────────────────────────────────────────────────────────────────────

	// GetByIDs возвращает связи по списку ID.
	GetByIDs(ctx context.Context, ids []string) ([]*Connection, error)

	// FindStale находит связи, неактивные более указанного времени.
	FindStale(ctx context.Context, threshold time.Duration) ([]*Connection, error)
}

// ConnectionListOptions параметры для списка связей.
type ConnectionListOptions struct {
	// Offset - смещение (для пагинации).
	Offset int

	// Limit - максимальное количество записей.
	Limit int

	// IncludeEnded - включать завершённые связи.
	IncludeEnded bool

	// Types - фильтр по типам связей (пустой = все типы).
	Types []ConnectionType

	// SortBy - поле для сортировки.
	SortBy string

	// SortDesc - сортировка по убыванию.
	SortDesc bool
}

// DefaultConnectionListOptions возвращает параметры по умолчанию.
func DefaultConnectionListOptions() ConnectionListOptions {
	return ConnectionListOptions{
		Offset:       0,
		Limit:        50,
		IncludeEnded: false,
		Types:        nil,
		SortBy:       "created_at",
		SortDesc:     true,
	}
}

// ConnectionStatsAggregate агрегированная статистика связей.
type ConnectionStatsAggregate struct {
	// TotalConnections - общее количество связей.
	TotalConnections int

	// ActiveConnections - активные связи.
	ActiveConnections int

	// PendingConnections - ожидающие связи.
	PendingConnections int

	// ConnectionsByType - количество по типам.
	ConnectionsByType map[ConnectionType]int

	// TotalInteractions - общее количество взаимодействий.
	TotalInteractions int

	// TotalHelpTime - общее время помощи (минуты).
	TotalHelpTime int

	// AverageDurationDays - средняя продолжительность связей (дни).
	AverageDurationDays int
}

// ══════════════════════════════════════════════════════════════════════════════
// HELP REQUEST REPOSITORY
// Работа с запросами помощи.
// ══════════════════════════════════════════════════════════════════════════════

// HelpRequestRepository определяет операции для работы с запросами помощи.
type HelpRequestRepository interface {
	// ─────────────────────────────────────────────────────────────────────────
	// CRUD Operations
	// ─────────────────────────────────────────────────────────────────────────

	// Create создаёт новый запрос помощи.
	Create(ctx context.Context, req *HelpRequest) error

	// GetByID возвращает запрос по ID.
	// Возвращает ErrHelpRequestNotFound, если запрос не найден.
	GetByID(ctx context.Context, id string) (*HelpRequest, error)

	// Update обновляет запрос помощи.
	// Возвращает ErrHelpRequestNotFound, если запрос не найден.
	Update(ctx context.Context, req *HelpRequest) error

	// Delete удаляет запрос (soft delete).
	// Возвращает ErrHelpRequestNotFound, если запрос не найден.
	Delete(ctx context.Context, id string) error

	// ─────────────────────────────────────────────────────────────────────────
	// Query Operations
	// ─────────────────────────────────────────────────────────────────────────

	// GetByRequesterID возвращает все запросы студента.
	GetByRequesterID(ctx context.Context, requesterID StudentID, opts HelpRequestListOptions) ([]*HelpRequest, error)

	// GetOpenByRequesterID возвращает открытые запросы студента.
	GetOpenByRequesterID(ctx context.Context, requesterID StudentID) ([]*HelpRequest, error)

	// GetByTaskID возвращает запросы по конкретной задаче.
	GetByTaskID(ctx context.Context, taskID TaskID, opts HelpRequestListOptions) ([]*HelpRequest, error)

	// GetOpenByTaskID возвращает открытые запросы по задаче.
	GetOpenByTaskID(ctx context.Context, taskID TaskID) ([]*HelpRequest, error)

	// GetByHelperID возвращает запросы, где студент помогает.
	GetByHelperID(ctx context.Context, helperID StudentID, opts HelpRequestListOptions) ([]*HelpRequest, error)

	// GetByStatus возвращает запросы с определённым статусом.
	GetByStatus(ctx context.Context, status HelpRequestStatus, opts HelpRequestListOptions) ([]*HelpRequest, error)

	// GetByPriority возвращает запросы с определённым приоритетом.
	GetByPriority(ctx context.Context, priority HelpRequestPriority, opts HelpRequestListOptions) ([]*HelpRequest, error)

	// GetExpired возвращает истёкшие запросы.
	GetExpired(ctx context.Context) ([]*HelpRequest, error)

	// GetRecentOpen возвращает недавние открытые запросы.
	GetRecentOpen(ctx context.Context, limit int) ([]*HelpRequest, error)

	// GetUrgent возвращает срочные запросы (дедлайн скоро).
	GetUrgent(ctx context.Context, withinHours int) ([]*HelpRequest, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Search
	// ─────────────────────────────────────────────────────────────────────────

	// Search ищет запросы по критериям.
	Search(ctx context.Context, criteria HelpRequestSearchCriteria) ([]*HelpRequest, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Existence Checks
	// ─────────────────────────────────────────────────────────────────────────

	// Exists проверяет существование запроса по ID.
	Exists(ctx context.Context, id string) (bool, error)

	// HasOpenRequestForTask проверяет, есть ли открытый запрос по задаче.
	HasOpenRequestForTask(ctx context.Context, requesterID StudentID, taskID TaskID) (bool, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Aggregations
	// ─────────────────────────────────────────────────────────────────────────

	// CountByRequesterID возвращает количество запросов студента.
	CountByRequesterID(ctx context.Context, requesterID StudentID) (int, error)

	// CountOpenByRequesterID возвращает количество открытых запросов.
	CountOpenByRequesterID(ctx context.Context, requesterID StudentID) (int, error)

	// CountByTaskID возвращает количество запросов по задаче.
	CountByTaskID(ctx context.Context, taskID TaskID) (int, error)

	// GetHelpRequestStats возвращает статистику запросов.
	GetHelpRequestStats(ctx context.Context, studentID StudentID) (*HelpRequestStatsAggregate, error)

	// GetPopularTasks возвращает задачи с наибольшим количеством запросов.
	GetPopularTasks(ctx context.Context, limit int, since time.Time) ([]TaskHelpStats, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Bulk Operations
	// ─────────────────────────────────────────────────────────────────────────

	// GetByIDs возвращает запросы по списку ID.
	GetByIDs(ctx context.Context, ids []string) ([]*HelpRequest, error)

	// MarkExpiredRequests помечает истёкшие запросы.
	MarkExpiredRequests(ctx context.Context) (int, error)
}

// HelpRequestListOptions параметры для списка запросов помощи.
type HelpRequestListOptions struct {
	// Offset - смещение (для пагинации).
	Offset int

	// Limit - максимальное количество записей.
	Limit int

	// IncludeClosed - включать закрытые запросы.
	IncludeClosed bool

	// Statuses - фильтр по статусам (пустой = все статусы).
	Statuses []HelpRequestStatus

	// Priorities - фильтр по приоритетам (пустой = все приоритеты).
	Priorities []HelpRequestPriority

	// SortBy - поле для сортировки.
	SortBy string

	// SortDesc - сортировка по убыванию.
	SortDesc bool
}

// DefaultHelpRequestListOptions возвращает параметры по умолчанию.
func DefaultHelpRequestListOptions() HelpRequestListOptions {
	return HelpRequestListOptions{
		Offset:        0,
		Limit:         50,
		IncludeClosed: false,
		Statuses:      nil,
		Priorities:    nil,
		SortBy:        "created_at",
		SortDesc:      true,
	}
}

// HelpRequestSearchCriteria критерии поиска запросов.
type HelpRequestSearchCriteria struct {
	// TaskIDs - фильтр по задачам.
	TaskIDs []TaskID

	// RequesterIDs - фильтр по авторам.
	RequesterIDs []StudentID

	// HelperIDs - фильтр по помощникам.
	HelperIDs []StudentID

	// Statuses - фильтр по статусам.
	Statuses []HelpRequestStatus

	// Priorities - фильтр по приоритетам.
	Priorities []HelpRequestPriority

	// CreatedAfter - созданы после.
	CreatedAfter *time.Time

	// CreatedBefore - созданы до.
	CreatedBefore *time.Time

	// HasDeadline - имеют дедлайн.
	HasDeadline *bool

	// DeadlineBefore - дедлайн до.
	DeadlineBefore *time.Time

	// Offset - смещение.
	Offset int

	// Limit - максимальное количество.
	Limit int
}

// HelpRequestStatsAggregate агрегированная статистика запросов.
type HelpRequestStatsAggregate struct {
	// TotalRequests - общее количество запросов.
	TotalRequests int

	// OpenRequests - открытые запросы.
	OpenRequests int

	// ResolvedRequests - решённые запросы.
	ResolvedRequests int

	// AverageResolutionTimeMinutes - среднее время решения.
	AverageResolutionTimeMinutes int

	// RequestsByPriority - запросы по приоритетам.
	RequestsByPriority map[HelpRequestPriority]int

	// RequestsByStatus - запросы по статусам.
	RequestsByStatus map[HelpRequestStatus]int

	// TopTasks - топ задач с запросами.
	TopTasks []TaskID
}

// TaskHelpStats статистика помощи по задаче.
type TaskHelpStats struct {
	// TaskID - ID задачи.
	TaskID TaskID

	// TaskName - название задачи.
	TaskName string

	// RequestCount - количество запросов.
	RequestCount int

	// ResolvedCount - количество решённых.
	ResolvedCount int

	// AverageResolutionMinutes - среднее время решения.
	AverageResolutionMinutes int
}

// ══════════════════════════════════════════════════════════════════════════════
// ENDORSEMENT REPOSITORY
// Работа с благодарностями.
// ══════════════════════════════════════════════════════════════════════════════

// EndorsementRepository определяет операции для работы с благодарностями.
type EndorsementRepository interface {
	// ─────────────────────────────────────────────────────────────────────────
	// CRUD Operations
	// ─────────────────────────────────────────────────────────────────────────

	// Create создаёт новую благодарность.
	// Возвращает ErrEndorsementAlreadyExists, если благодарность уже есть.
	Create(ctx context.Context, endorsement *Endorsement) error

	// GetByID возвращает благодарность по ID.
	// Возвращает ErrEndorsementNotFound, если не найдена.
	GetByID(ctx context.Context, id string) (*Endorsement, error)

	// Delete удаляет благодарность.
	// Возвращает ErrEndorsementNotFound, если не найдена.
	Delete(ctx context.Context, id string) error

	// ─────────────────────────────────────────────────────────────────────────
	// Query Operations
	// ─────────────────────────────────────────────────────────────────────────

	// GetByGiverID возвращает благодарности, данные студентом.
	GetByGiverID(ctx context.Context, giverID StudentID, opts EndorsementListOptions) ([]*Endorsement, error)

	// GetByReceiverID возвращает благодарности, полученные студентом.
	GetByReceiverID(ctx context.Context, receiverID StudentID, opts EndorsementListOptions) ([]*Endorsement, error)

	// GetByHelpRequestID возвращает благодарность за запрос помощи.
	GetByHelpRequestID(ctx context.Context, helpRequestID string) (*Endorsement, error)

	// GetByTaskID возвращает благодарности по задаче.
	GetByTaskID(ctx context.Context, taskID TaskID, opts EndorsementListOptions) ([]*Endorsement, error)

	// GetByType возвращает благодарности определённого типа.
	GetByType(ctx context.Context, receiverID StudentID, endorsementType EndorsementType) ([]*Endorsement, error)

	// GetPublic возвращает публичные благодарности.
	GetPublic(ctx context.Context, receiverID StudentID, opts EndorsementListOptions) ([]*Endorsement, error)

	// GetRecent возвращает недавние благодарности.
	GetRecent(ctx context.Context, limit int, since time.Time) ([]*Endorsement, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Existence Checks
	// ─────────────────────────────────────────────────────────────────────────

	// Exists проверяет существование благодарности по ID.
	Exists(ctx context.Context, id string) (bool, error)

	// ExistsForHelpRequest проверяет, есть ли благодарность за запрос.
	ExistsForHelpRequest(ctx context.Context, helpRequestID string) (bool, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Aggregations
	// ─────────────────────────────────────────────────────────────────────────

	// CountByReceiverID возвращает количество благодарностей.
	CountByReceiverID(ctx context.Context, receiverID StudentID) (int, error)

	// GetAverageRating возвращает средний рейтинг студента.
	GetAverageRating(ctx context.Context, receiverID StudentID) (Rating, error)

	// GetEndorsementStats возвращает статистику благодарностей.
	GetEndorsementStats(ctx context.Context, studentID StudentID) (*EndorsementStatsAggregate, error)

	// GetTypeStats возвращает статистику по типам благодарностей.
	GetTypeStats(ctx context.Context, receiverID StudentID) ([]EndorsementTypeStat, error)

	// GetTopHelpers возвращает топ помощников по рейтингу.
	GetTopHelpers(ctx context.Context, limit int, since time.Time) ([]HelperRankingEntry, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Bulk Operations
	// ─────────────────────────────────────────────────────────────────────────

	// GetByIDs возвращает благодарности по списку ID.
	GetByIDs(ctx context.Context, ids []string) ([]*Endorsement, error)
}

// EndorsementListOptions параметры для списка благодарностей.
type EndorsementListOptions struct {
	// Offset - смещение (для пагинации).
	Offset int

	// Limit - максимальное количество записей.
	Limit int

	// Types - фильтр по типам (пустой = все типы).
	Types []EndorsementType

	// MinRating - минимальный рейтинг.
	MinRating Rating

	// PublicOnly - только публичные.
	PublicOnly bool

	// SortBy - поле для сортировки.
	SortBy string

	// SortDesc - сортировка по убыванию.
	SortDesc bool
}

// DefaultEndorsementListOptions возвращает параметры по умолчанию.
func DefaultEndorsementListOptions() EndorsementListOptions {
	return EndorsementListOptions{
		Offset:     0,
		Limit:      50,
		Types:      nil,
		MinRating:  0,
		PublicOnly: false,
		SortBy:     "created_at",
		SortDesc:   true,
	}
}

// EndorsementStatsAggregate агрегированная статистика благодарностей.
type EndorsementStatsAggregate struct {
	// TotalReceived - всего получено.
	TotalReceived int

	// TotalGiven - всего дано.
	TotalGiven int

	// AverageRating - средний рейтинг.
	AverageRating Rating

	// ByType - статистика по типам.
	ByType []EndorsementTypeStat

	// PositiveCount - количество положительных (4+).
	PositiveCount int

	// RecentCount - количество за последнюю неделю.
	RecentCount int
}

// HelperRankingEntry запись в рейтинге помощников.
type HelperRankingEntry struct {
	// StudentID - ID студента.
	StudentID StudentID

	// DisplayName - имя для отображения.
	DisplayName string

	// AverageRating - средний рейтинг.
	AverageRating Rating

	// EndorsementCount - количество благодарностей.
	EndorsementCount int

	// HelpCount - количество помощей.
	HelpCount int

	// Rank - позиция в рейтинге.
	Rank int
}

// ══════════════════════════════════════════════════════════════════════════════
// MATCHING REPOSITORY
// Работа с подбором менторов и напарников.
// ══════════════════════════════════════════════════════════════════════════════

// MatchingRepository определяет операции для работы с подбором.
type MatchingRepository interface {
	// ─────────────────────────────────────────────────────────────────────────
	// Mentor Match
	// ─────────────────────────────────────────────────────────────────────────

	// CreateMentorMatch создаёт подбор ментора.
	CreateMentorMatch(ctx context.Context, match *MentorMatch) error

	// GetMentorMatchByID возвращает подбор ментора по ID.
	GetMentorMatchByID(ctx context.Context, id string) (*MentorMatch, error)

	// UpdateMentorMatch обновляет подбор ментора.
	UpdateMentorMatch(ctx context.Context, match *MentorMatch) error

	// GetPendingMentorMatches возвращает ожидающие подборы для ментора.
	GetPendingMentorMatches(ctx context.Context, mentorID StudentID) ([]*MentorMatch, error)

	// GetMentorMatchesByMentee возвращает подборы для студента.
	GetMentorMatchesByMentee(ctx context.Context, menteeID StudentID) ([]*MentorMatch, error)

	// GetExpiredMentorMatches возвращает истёкшие подборы.
	GetExpiredMentorMatches(ctx context.Context) ([]*MentorMatch, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Study Buddy
	// ─────────────────────────────────────────────────────────────────────────

	// CreateStudyBuddy создаёт подбор напарника.
	CreateStudyBuddy(ctx context.Context, buddy *StudyBuddy) error

	// GetStudyBuddyByID возвращает подбор напарника по ID.
	GetStudyBuddyByID(ctx context.Context, id string) (*StudyBuddy, error)

	// UpdateStudyBuddy обновляет подбор напарника.
	UpdateStudyBuddy(ctx context.Context, buddy *StudyBuddy) error

	// GetPendingStudyBuddies возвращает ожидающие подборы для студента.
	GetPendingStudyBuddies(ctx context.Context, studentID StudentID) ([]*StudyBuddy, error)

	// GetStudyBuddiesByStudent возвращает все подборы студента.
	GetStudyBuddiesByStudent(ctx context.Context, studentID StudentID) ([]*StudyBuddy, error)

	// GetExpiredStudyBuddies возвращает истёкшие подборы.
	GetExpiredStudyBuddies(ctx context.Context) ([]*StudyBuddy, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Candidate Search
	// ─────────────────────────────────────────────────────────────────────────

	// FindMentorCandidates ищет кандидатов в менторы.
	FindMentorCandidates(ctx context.Context, criteria MentorCriteria) ([]Candidate, error)

	// FindStudyBuddyCandidates ищет кандидатов в напарники.
	FindStudyBuddyCandidates(ctx context.Context, criteria StudyBuddyCriteria) ([]Candidate, error)

	// FindHelperCandidates ищет кандидатов для помощи по задаче.
	FindHelperCandidates(ctx context.Context, criteria HelperCriteria) ([]Candidate, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Bulk Operations
	// ─────────────────────────────────────────────────────────────────────────

	// MarkExpiredMentorMatches помечает истёкшие подборы менторов.
	MarkExpiredMentorMatches(ctx context.Context) (int, error)

	// MarkExpiredStudyBuddies помечает истёкшие подборы напарников.
	MarkExpiredStudyBuddies(ctx context.Context) (int, error)
}

// ══════════════════════════════════════════════════════════════════════════════
// SOCIAL PROFILE REPOSITORY
// Работа с социальным профилем студента.
// ══════════════════════════════════════════════════════════════════════════════

// SocialProfileRepository определяет операции для работы с социальным профилем.
type SocialProfileRepository interface {
	// ─────────────────────────────────────────────────────────────────────────
	// CRUD Operations
	// ─────────────────────────────────────────────────────────────────────────

	// GetByStudentID возвращает социальный профиль студента.
	GetByStudentID(ctx context.Context, studentID StudentID) (*SocialProfile, error)

	// Update обновляет социальный профиль.
	Update(ctx context.Context, profile *SocialProfile) error

	// ─────────────────────────────────────────────────────────────────────────
	// Query Operations
	// ─────────────────────────────────────────────────────────────────────────

	// GetMentors возвращает список менторов.
	GetMentors(ctx context.Context, opts SocialProfileListOptions) ([]*SocialProfile, error)

	// GetOpenToHelp возвращает студентов, готовых помогать.
	GetOpenToHelp(ctx context.Context, opts SocialProfileListOptions) ([]*SocialProfile, error)

	// GetBySpecializedTask возвращает студентов, специализирующихся на задаче.
	GetBySpecializedTask(ctx context.Context, taskID TaskID) ([]*SocialProfile, error)

	// GetTopHelpers возвращает топ помощников.
	GetTopHelpers(ctx context.Context, limit int) ([]*SocialProfile, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Aggregations
	// ─────────────────────────────────────────────────────────────────────────

	// GetGlobalStats возвращает глобальную статистику сообщества.
	GetGlobalStats(ctx context.Context) (*CommunityStats, error)
}

// SocialProfileListOptions параметры для списка профилей.
type SocialProfileListOptions struct {
	// Offset - смещение.
	Offset int

	// Limit - максимальное количество.
	Limit int

	// MinRating - минимальный рейтинг.
	MinRating Rating

	// MinHelpCount - минимальное количество помощей.
	MinHelpCount int

	// SortBy - поле для сортировки.
	SortBy string

	// SortDesc - сортировка по убыванию.
	SortDesc bool
}

// DefaultSocialProfileListOptions возвращает параметры по умолчанию.
func DefaultSocialProfileListOptions() SocialProfileListOptions {
	return SocialProfileListOptions{
		Offset:       0,
		Limit:        50,
		MinRating:    0,
		MinHelpCount: 0,
		SortBy:       "average_rating",
		SortDesc:     true,
	}
}

// CommunityStats глобальная статистика сообщества.
type CommunityStats struct {
	// TotalConnections - всего связей.
	TotalConnections int

	// ActiveConnections - активных связей.
	ActiveConnections int

	// TotalHelpRequests - всего запросов помощи.
	TotalHelpRequests int

	// ResolvedHelpRequests - решённых запросов.
	ResolvedHelpRequests int

	// TotalEndorsements - всего благодарностей.
	TotalEndorsements int

	// AverageRating - средний рейтинг сообщества.
	AverageRating Rating

	// ActiveMentors - активных менторов.
	ActiveMentors int

	// StudyBuddyPairs - пар напарников.
	StudyBuddyPairs int

	// MostHelpfulStudents - самые полезные студенты.
	MostHelpfulStudents []StudentID

	// MostRequestedTasks - самые запрашиваемые задачи.
	MostRequestedTasks []TaskID

	// UpdatedAt - когда обновлена статистика.
	UpdatedAt time.Time
}

// ══════════════════════════════════════════════════════════════════════════════
// COMPOSITE REPOSITORY
// Объединённый репозиторий для удобства DI.
// ══════════════════════════════════════════════════════════════════════════════

// Repository объединяет все социальные репозитории.
type Repository interface {
	// Connections возвращает репозиторий связей.
	Connections() ConnectionRepository

	// HelpRequests возвращает репозиторий запросов помощи.
	HelpRequests() HelpRequestRepository

	// Endorsements возвращает репозиторий благодарностей.
	Endorsements() EndorsementRepository

	// Matching возвращает репозиторий подбора.
	Matching() MatchingRepository

	// SocialProfiles возвращает репозиторий социальных профилей.
	SocialProfiles() SocialProfileRepository
}

// ══════════════════════════════════════════════════════════════════════════════
// UNIT OF WORK
// Для транзакционных операций.
// ══════════════════════════════════════════════════════════════════════════════

// UnitOfWork представляет единицу работы с транзакционной семантикой.
type UnitOfWork interface {
	// Repository возвращает социальный репозиторий в рамках транзакции.
	Repository() Repository

	// Commit фиксирует транзакцию.
	Commit(ctx context.Context) error

	// Rollback откатывает транзакцию.
	Rollback(ctx context.Context) error
}

// UnitOfWorkFactory создаёт единицы работы.
type UnitOfWorkFactory interface {
	// Begin начинает новую транзакцию.
	Begin(ctx context.Context) (UnitOfWork, error)
}

// ══════════════════════════════════════════════════════════════════════════════
// CACHE INTERFACE
// Кеширование социальных данных.
// ══════════════════════════════════════════════════════════════════════════════

// Cache определяет операции кеширования социальных данных.
type Cache interface {
	// ─────────────────────────────────────────────────────────────────────────
	// Social Profile
	// ─────────────────────────────────────────────────────────────────────────

	// GetSocialProfile получает социальный профиль из кеша.
	GetSocialProfile(ctx context.Context, studentID StudentID) (*SocialProfile, error)

	// SetSocialProfile сохраняет социальный профиль в кеш.
	SetSocialProfile(ctx context.Context, profile *SocialProfile, ttl time.Duration) error

	// InvalidateSocialProfile инвалидирует социальный профиль.
	InvalidateSocialProfile(ctx context.Context, studentID StudentID) error

	// ─────────────────────────────────────────────────────────────────────────
	// Helper Ranking
	// ─────────────────────────────────────────────────────────────────────────

	// GetTopHelpers получает топ помощников из кеша.
	GetTopHelpers(ctx context.Context, limit int) ([]HelperRankingEntry, error)

	// SetTopHelpers сохраняет топ помощников в кеш.
	SetTopHelpers(ctx context.Context, entries []HelperRankingEntry, ttl time.Duration) error

	// InvalidateTopHelpers инвалидирует кеш топ помощников.
	InvalidateTopHelpers(ctx context.Context) error

	// ─────────────────────────────────────────────────────────────────────────
	// Community Stats
	// ─────────────────────────────────────────────────────────────────────────

	// GetCommunityStats получает статистику сообщества из кеша.
	GetCommunityStats(ctx context.Context) (*CommunityStats, error)

	// SetCommunityStats сохраняет статистику в кеш.
	SetCommunityStats(ctx context.Context, stats *CommunityStats, ttl time.Duration) error

	// InvalidateCommunityStats инвалидирует статистику.
	InvalidateCommunityStats(ctx context.Context) error

	// ─────────────────────────────────────────────────────────────────────────
	// General
	// ─────────────────────────────────────────────────────────────────────────

	// InvalidateAll очищает весь кеш.
	InvalidateAll(ctx context.Context) error
}
