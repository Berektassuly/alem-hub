package student

import (
	"context"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// REPOSITORY INTERFACES
// Эти интерфейсы определяют контракт для работы с хранилищем данных.
// Реализации находятся в infrastructure/persistence.
// ══════════════════════════════════════════════════════════════════════════════

// Repository определяет основные операции CRUD для студентов.
type Repository interface {
	// ─────────────────────────────────────────────────────────────────────────
	// CRUD Operations
	// ─────────────────────────────────────────────────────────────────────────

	// Create создаёт нового студента.
	// Возвращает ErrStudentAlreadyExists, если студент уже существует.
	Create(ctx context.Context, student *Student) error

	// GetByID возвращает студента по внутреннему ID.
	// Возвращает ErrStudentNotFound, если студент не найден.
	GetByID(ctx context.Context, id string) (*Student, error)

	// GetByTelegramID возвращает студента по Telegram ID.
	// Возвращает ErrStudentNotFound, если студент не найден.
	GetByTelegramID(ctx context.Context, telegramID TelegramID) (*Student, error)

	// GetByAlemLogin возвращает студента по логину Alem.
	// Возвращает ErrStudentNotFound, если студент не найден.
	GetByAlemLogin(ctx context.Context, login AlemLogin) (*Student, error)

	// Update обновляет данные студента.
	// Возвращает ErrStudentNotFound, если студент не найден.
	Update(ctx context.Context, student *Student) error

	// Delete удаляет студента (soft delete).
	// Возвращает ErrStudentNotFound, если студент не найден.
	Delete(ctx context.Context, id string) error

	// ─────────────────────────────────────────────────────────────────────────
	// Bulk Operations
	// ─────────────────────────────────────────────────────────────────────────

	// GetAll возвращает всех студентов с пагинацией.
	GetAll(ctx context.Context, opts ListOptions) ([]*Student, error)

	// GetByCohort возвращает студентов указанной когорты.
	GetByCohort(ctx context.Context, cohort Cohort, opts ListOptions) ([]*Student, error)

	// GetByStatus возвращает студентов с указанным статусом.
	GetByStatus(ctx context.Context, status Status, opts ListOptions) ([]*Student, error)

	// GetByIDs возвращает студентов по списку ID.
	GetByIDs(ctx context.Context, ids []string) ([]*Student, error)

	// Count возвращает общее количество студентов.
	Count(ctx context.Context) (int, error)

	// CountByCohort возвращает количество студентов в когорте.
	CountByCohort(ctx context.Context, cohort Cohort) (int, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Search & Filter
	// ─────────────────────────────────────────────────────────────────────────

	// Search выполняет поиск студентов по имени или логину.
	Search(ctx context.Context, query string, opts ListOptions) ([]*Student, error)

	// FindInactive находит студентов, неактивных более указанного времени.
	FindInactive(ctx context.Context, threshold time.Duration) ([]*Student, error)

	// FindOnline находит студентов, которые сейчас онлайн.
	FindOnline(ctx context.Context) ([]*Student, error)

	// FindByXPRange находит студентов в указанном диапазоне XP.
	FindByXPRange(ctx context.Context, minXP, maxXP XP) ([]*Student, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Existence Checks
	// ─────────────────────────────────────────────────────────────────────────

	// Exists проверяет существование студента по ID.
	Exists(ctx context.Context, id string) (bool, error)

	// ExistsByTelegramID проверяет существование по Telegram ID.
	ExistsByTelegramID(ctx context.Context, telegramID TelegramID) (bool, error)

	// ExistsByAlemLogin проверяет существование по логину Alem.
	ExistsByAlemLogin(ctx context.Context, login AlemLogin) (bool, error)
}

// ListOptions содержит параметры для пагинации и сортировки.
type ListOptions struct {
	// Offset - смещение (для пагинации).
	Offset int

	// Limit - максимальное количество записей.
	Limit int

	// SortBy - поле для сортировки.
	SortBy string

	// SortDesc - сортировка по убыванию.
	SortDesc bool

	// IncludeInactive - включать неактивных студентов.
	IncludeInactive bool
}

// DefaultListOptions возвращает параметры по умолчанию.
func DefaultListOptions() ListOptions {
	return ListOptions{
		Offset:          0,
		Limit:           50,
		SortBy:          "current_xp",
		SortDesc:        true,
		IncludeInactive: false,
	}
}

// WithOffset устанавливает смещение.
func (o ListOptions) WithOffset(offset int) ListOptions {
	o.Offset = offset
	return o
}

// WithLimit устанавливает лимит.
func (o ListOptions) WithLimit(limit int) ListOptions {
	o.Limit = limit
	return o
}

// WithSort устанавливает сортировку.
func (o ListOptions) WithSort(field string, desc bool) ListOptions {
	o.SortBy = field
	o.SortDesc = desc
	return o
}

// WithInactive включает неактивных студентов.
func (o ListOptions) WithInactive() ListOptions {
	o.IncludeInactive = true
	return o
}

// ══════════════════════════════════════════════════════════════════════════════
// PROGRESS REPOSITORY
// ══════════════════════════════════════════════════════════════════════════════

// ProgressRepository определяет операции для работы с прогрессом студентов.
type ProgressRepository interface {
	// ─────────────────────────────────────────────────────────────────────────
	// XP History
	// ─────────────────────────────────────────────────────────────────────────

	// SaveXPChange сохраняет изменение XP.
	SaveXPChange(ctx context.Context, entry XPHistoryEntry) error

	// GetXPHistory возвращает историю XP студента.
	GetXPHistory(ctx context.Context, studentID string, from, to time.Time) ([]XPHistoryEntry, error)

	// GetRecentXPChanges возвращает последние N изменений XP.
	GetRecentXPChanges(ctx context.Context, studentID string, limit int) ([]XPHistoryEntry, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Daily Grind
	// ─────────────────────────────────────────────────────────────────────────

	// SaveDailyGrind сохраняет или обновляет дневной прогресс.
	SaveDailyGrind(ctx context.Context, grind *DailyGrind) error

	// GetDailyGrind возвращает дневной прогресс за указанную дату.
	GetDailyGrind(ctx context.Context, studentID string, date time.Time) (*DailyGrind, error)

	// GetDailyGrindHistory возвращает историю дневного прогресса.
	GetDailyGrindHistory(ctx context.Context, studentID string, days int) ([]*DailyGrind, error)

	// GetTodayDailyGrind возвращает прогресс за сегодня.
	GetTodayDailyGrind(ctx context.Context, studentID string) (*DailyGrind, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Streaks
	// ─────────────────────────────────────────────────────────────────────────

	// SaveStreak сохраняет или обновляет серию.
	SaveStreak(ctx context.Context, streak *Streak) error

	// GetStreak возвращает текущую серию студента.
	GetStreak(ctx context.Context, studentID string) (*Streak, error)

	// GetTopStreaks возвращает топ студентов по текущей серии.
	GetTopStreaks(ctx context.Context, limit int) ([]*Streak, error)

	// ─────────────────────────────────────────────────────────────────────────
	// Achievements
	// ─────────────────────────────────────────────────────────────────────────

	// SaveAchievement сохраняет разблокированное достижение.
	SaveAchievement(ctx context.Context, studentID string, achievement Achievement) error

	// GetAchievements возвращает все достижения студента.
	GetAchievements(ctx context.Context, studentID string) ([]Achievement, error)

	// HasAchievement проверяет, есть ли у студента достижение.
	HasAchievement(ctx context.Context, studentID string, achievementType AchievementType) (bool, error)

	// GetRecentAchievements возвращает недавние достижения всех студентов.
	GetRecentAchievements(ctx context.Context, since time.Time) ([]StudentAchievement, error)
}

// StudentAchievement связывает студента с достижением (для списков).
type StudentAchievement struct {
	StudentID   string
	Achievement Achievement
}

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE TRACKER
// Отслеживает онлайн-статус студентов (обычно реализуется через Redis).
// ══════════════════════════════════════════════════════════════════════════════

// OnlineTracker определяет операции для отслеживания онлайн-статуса.
type OnlineTracker interface {
	// MarkOnline отмечает студента как онлайн.
	MarkOnline(ctx context.Context, studentID string) error

	// MarkOffline отмечает студента как оффлайн.
	MarkOffline(ctx context.Context, studentID string) error

	// IsOnline проверяет, онлайн ли студент.
	IsOnline(ctx context.Context, studentID string) (bool, error)

	// GetOnlineStudents возвращает список ID онлайн-студентов.
	GetOnlineStudents(ctx context.Context) ([]string, error)

	// GetOnlineCount возвращает количество онлайн-студентов.
	GetOnlineCount(ctx context.Context) (int, error)

	// GetLastSeen возвращает время последней активности.
	GetLastSeen(ctx context.Context, studentID string) (time.Time, error)

	// SetLastActivity обновляет время последней активности.
	SetLastActivity(ctx context.Context, studentID string, at time.Time) error

	// GetOnlineStates возвращает онлайн-состояния для списка студентов.
	GetOnlineStates(ctx context.Context, studentIDs []string) (map[string]OnlineState, error)
}

// ══════════════════════════════════════════════════════════════════════════════
// SYNC REPOSITORY
// Для работы с синхронизацией данных с Alem API.
// ══════════════════════════════════════════════════════════════════════════════

// SyncRepository определяет операции для синхронизации с внешними источниками.
type SyncRepository interface {
	// GetLastSyncTime возвращает время последней успешной синхронизации.
	GetLastSyncTime(ctx context.Context) (time.Time, error)

	// SetLastSyncTime устанавливает время последней синхронизации.
	SetLastSyncTime(ctx context.Context, t time.Time) error

	// GetStudentsToSync возвращает студентов, требующих синхронизации.
	GetStudentsToSync(ctx context.Context, olderThan time.Duration) ([]*Student, error)

	// MarkSynced отмечает студента как синхронизированного.
	MarkSynced(ctx context.Context, studentID string, syncTime time.Time) error

	// GetSyncErrors возвращает ошибки синхронизации.
	GetSyncErrors(ctx context.Context, since time.Time) ([]SyncError, error)

	// SaveSyncError сохраняет ошибку синхронизации.
	SaveSyncError(ctx context.Context, err SyncError) error
}

// SyncError представляет ошибку синхронизации.
type SyncError struct {
	// StudentID - ID студента (если применимо).
	StudentID string

	// ErrorType - тип ошибки.
	ErrorType string

	// Message - сообщение об ошибке.
	Message string

	// OccurredAt - когда произошла ошибка.
	OccurredAt time.Time

	// Retries - количество повторных попыток.
	Retries int
}

// ══════════════════════════════════════════════════════════════════════════════
// CACHE INTERFACE
// Для кеширования часто запрашиваемых данных.
// ══════════════════════════════════════════════════════════════════════════════

// Cache определяет операции кеширования данных студентов.
type Cache interface {
	// Get получает студента из кеша.
	Get(ctx context.Context, studentID string) (*Student, error)

	// Set сохраняет студента в кеш.
	Set(ctx context.Context, student *Student, ttl time.Duration) error

	// Delete удаляет студента из кеша.
	Delete(ctx context.Context, studentID string) error

	// GetByTelegramID получает студента из кеша по Telegram ID.
	GetByTelegramID(ctx context.Context, telegramID TelegramID) (*Student, error)

	// SetByTelegramID сохраняет студента в кеш с ключом Telegram ID.
	SetByTelegramID(ctx context.Context, student *Student, ttl time.Duration) error

	// Invalidate инвалидирует все записи студента в кеше.
	Invalidate(ctx context.Context, studentID string) error

	// InvalidateAll очищает весь кеш студентов.
	InvalidateAll(ctx context.Context) error
}

// ══════════════════════════════════════════════════════════════════════════════
// UNIT OF WORK (для транзакций)
// ══════════════════════════════════════════════════════════════════════════════

// UnitOfWork представляет единицу работы с транзакционной семантикой.
type UnitOfWork interface {
	// Students возвращает репозиторий студентов в рамках транзакции.
	Students() Repository

	// Progress возвращает репозиторий прогресса в рамках транзакции.
	Progress() ProgressRepository

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
