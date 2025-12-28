package student

import (
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// DOMAIN EVENTS
// События, которые происходят в домене студентов и на которые могут
// реагировать другие части системы (уведомления, аналитика и т.д.).
// ══════════════════════════════════════════════════════════════════════════════

// Event представляет базовый интерфейс доменного события.
type Event interface {
	// EventName возвращает имя события.
	EventName() string

	// OccurredAt возвращает время события.
	OccurredAt() time.Time

	// AggregateID возвращает ID агрегата (студента).
	AggregateID() string
}

// BaseEvent содержит общие поля для всех событий.
type BaseEvent struct {
	Timestamp   time.Time
	StudentID   string
	StudentName string
}

// OccurredAt возвращает время события.
func (e BaseEvent) OccurredAt() time.Time {
	return e.Timestamp
}

// AggregateID возвращает ID студента.
func (e BaseEvent) AggregateID() string {
	return e.StudentID
}

// ══════════════════════════════════════════════════════════════════════════════
// REGISTRATION EVENTS
// ══════════════════════════════════════════════════════════════════════════════

// StudentRegisteredEvent - студент зарегистрировался в системе.
type StudentRegisteredEvent struct {
	BaseEvent
	TelegramID TelegramID
	AlemLogin  AlemLogin
	Cohort     Cohort
}

// EventName возвращает имя события.
func (e StudentRegisteredEvent) EventName() string {
	return "student.registered"
}

// NewStudentRegisteredEvent создаёт событие регистрации студента.
func NewStudentRegisteredEvent(student *Student) StudentRegisteredEvent {
	return StudentRegisteredEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		TelegramID: student.TelegramID,
		AlemLogin:  student.AlemLogin,
		Cohort:     student.Cohort,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// XP & PROGRESS EVENTS
// ══════════════════════════════════════════════════════════════════════════════

// XPGainedEvent - студент получил XP.
type XPGainedEvent struct {
	BaseEvent
	OldXP      XP
	NewXP      XP
	Delta      XP
	Reason     string // task_completed, bonus, correction
	TaskID     string // Если применимо
	OldLevel   Level
	NewLevel   Level
	LeveledUp  bool
}

// EventName возвращает имя события.
func (e XPGainedEvent) EventName() string {
	return "student.xp_gained"
}

// NewXPGainedEvent создаёт событие получения XP.
func NewXPGainedEvent(student *Student, oldXP XP, reason string, taskID string) XPGainedEvent {
	oldLevel := CalculateLevel(oldXP)
	newLevel := student.Level()

	return XPGainedEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		OldXP:     oldXP,
		NewXP:     student.CurrentXP,
		Delta:     student.CurrentXP.Diff(oldXP),
		Reason:    reason,
		TaskID:    taskID,
		OldLevel:  oldLevel,
		NewLevel:  newLevel,
		LeveledUp: newLevel > oldLevel,
	}
}

// TaskCompletedEvent - студент выполнил задачу.
type TaskCompletedEvent struct {
	BaseEvent
	TaskID       string
	TaskName     string
	XPEarned     XP
	TotalTasks   int // Общее количество выполненных задач
}

// EventName возвращает имя события.
func (e TaskCompletedEvent) EventName() string {
	return "student.task_completed"
}

// NewTaskCompletedEvent создаёт событие выполнения задачи.
func NewTaskCompletedEvent(student *Student, taskID, taskName string, xpEarned XP, totalTasks int) TaskCompletedEvent {
	return TaskCompletedEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		TaskID:     taskID,
		TaskName:   taskName,
		XPEarned:   xpEarned,
		TotalTasks: totalTasks,
	}
}

// LevelUpEvent - студент повысил уровень.
type LevelUpEvent struct {
	BaseEvent
	OldLevel Level
	NewLevel Level
	TotalXP  XP
}

// EventName возвращает имя события.
func (e LevelUpEvent) EventName() string {
	return "student.level_up"
}

// NewLevelUpEvent создаёт событие повышения уровня.
func NewLevelUpEvent(student *Student, oldLevel Level) LevelUpEvent {
	return LevelUpEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		OldLevel: oldLevel,
		NewLevel: student.Level(),
		TotalXP:  student.CurrentXP,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// STREAK EVENTS
// ══════════════════════════════════════════════════════════════════════════════

// StreakExtendedEvent - серия активных дней продлена.
type StreakExtendedEvent struct {
	BaseEvent
	CurrentStreak int
	BestStreak    int
	IsNewRecord   bool
}

// EventName возвращает имя события.
func (e StreakExtendedEvent) EventName() string {
	return "student.streak_extended"
}

// NewStreakExtendedEvent создаёт событие продления серии.
func NewStreakExtendedEvent(student *Student, streak *Streak, wasRecord bool) StreakExtendedEvent {
	return StreakExtendedEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		CurrentStreak: streak.CurrentStreak,
		BestStreak:    streak.BestStreak,
		IsNewRecord:   wasRecord,
	}
}

// StreakBrokenEvent - серия активных дней прервана.
type StreakBrokenEvent struct {
	BaseEvent
	BrokenStreak   int // Какая серия была прервана
	LastActiveDate time.Time
	DaysMissed     int
}

// EventName возвращает имя события.
func (e StreakBrokenEvent) EventName() string {
	return "student.streak_broken"
}

// NewStreakBrokenEvent создаёт событие прерывания серии.
func NewStreakBrokenEvent(student *Student, brokenStreak int, lastActive time.Time) StreakBrokenEvent {
	daysMissed := int(time.Since(lastActive).Hours() / 24)

	return StreakBrokenEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		BrokenStreak:   brokenStreak,
		LastActiveDate: lastActive,
		DaysMissed:     daysMissed,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE STATUS EVENTS
// ══════════════════════════════════════════════════════════════════════════════

// StudentWentOnlineEvent - студент вошёл в систему.
type StudentWentOnlineEvent struct {
	BaseEvent
	PreviousState OnlineState
}

// EventName возвращает имя события.
func (e StudentWentOnlineEvent) EventName() string {
	return "student.went_online"
}

// NewStudentWentOnlineEvent создаёт событие входа в онлайн.
func NewStudentWentOnlineEvent(student *Student, previousState OnlineState) StudentWentOnlineEvent {
	return StudentWentOnlineEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		PreviousState: previousState,
	}
}

// StudentWentOfflineEvent - студент вышел из системы.
type StudentWentOfflineEvent struct {
	BaseEvent
	SessionDuration time.Duration
}

// EventName возвращает имя события.
func (e StudentWentOfflineEvent) EventName() string {
	return "student.went_offline"
}

// NewStudentWentOfflineEvent создаёт событие выхода из онлайна.
func NewStudentWentOfflineEvent(student *Student, sessionDuration time.Duration) StudentWentOfflineEvent {
	return StudentWentOfflineEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		SessionDuration: sessionDuration,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// INACTIVITY EVENTS
// ══════════════════════════════════════════════════════════════════════════════

// StudentBecameInactiveEvent - студент стал неактивным.
type StudentBecameInactiveEvent struct {
	BaseEvent
	LastSeenAt   time.Time
	InactiveDays int
}

// EventName возвращает имя события.
func (e StudentBecameInactiveEvent) EventName() string {
	return "student.became_inactive"
}

// NewStudentBecameInactiveEvent создаёт событие перехода в неактивное состояние.
func NewStudentBecameInactiveEvent(student *Student) StudentBecameInactiveEvent {
	return StudentBecameInactiveEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		LastSeenAt:   student.LastSeenAt,
		InactiveDays: student.DaysSinceLastSeen(),
	}
}

// StudentReturnedEvent - неактивный студент вернулся.
type StudentReturnedEvent struct {
	BaseEvent
	DaysAway     int
	PreviousXP   XP
	IsComebackKid bool // Был ли неактивен более 7 дней
}

// EventName возвращает имя события.
func (e StudentReturnedEvent) EventName() string {
	return "student.returned"
}

// NewStudentReturnedEvent создаёт событие возвращения студента.
func NewStudentReturnedEvent(student *Student, daysAway int, previousXP XP) StudentReturnedEvent {
	return StudentReturnedEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		DaysAway:      daysAway,
		PreviousXP:    previousXP,
		IsComebackKid: daysAway >= 7,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// ACHIEVEMENT EVENTS
// ══════════════════════════════════════════════════════════════════════════════

// AchievementUnlockedEvent - студент получил достижение.
type AchievementUnlockedEvent struct {
	BaseEvent
	Achievement     Achievement
	AchievementName string
	AchievementDesc string
	XPBonus         XP
}

// EventName возвращает имя события.
func (e AchievementUnlockedEvent) EventName() string {
	return "student.achievement_unlocked"
}

// NewAchievementUnlockedEvent создаёт событие получения достижения.
func NewAchievementUnlockedEvent(student *Student, achievement Achievement) AchievementUnlockedEvent {
	def, _ := GetAchievementDefinition(achievement.Type)

	return AchievementUnlockedEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		Achievement:     achievement,
		AchievementName: def.Name,
		AchievementDesc: def.Description,
		XPBonus:         def.XPBonus,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// HELP & SOCIAL EVENTS
// ══════════════════════════════════════════════════════════════════════════════

// HelpProvidedEvent - студент помог другому студенту.
type HelpProvidedEvent struct {
	BaseEvent
	HelpedStudentID   string
	HelpedStudentName string
	TaskID            string
	Rating            float64 // Оценка помощи (0-5)
	TotalHelpCount    int     // Общее количество помощей
}

// EventName возвращает имя события.
func (e HelpProvidedEvent) EventName() string {
	return "student.help_provided"
}

// NewHelpProvidedEvent создаёт событие оказания помощи.
func NewHelpProvidedEvent(helper *Student, helpedID, helpedName, taskID string, rating float64) HelpProvidedEvent {
	return HelpProvidedEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   helper.ID,
			StudentName: helper.DisplayName,
		},
		HelpedStudentID:   helpedID,
		HelpedStudentName: helpedName,
		TaskID:            taskID,
		Rating:            rating,
		TotalHelpCount:    helper.HelpCount,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// STATUS CHANGE EVENTS
// ══════════════════════════════════════════════════════════════════════════════

// StudentStatusChangedEvent - изменился статус студента.
type StudentStatusChangedEvent struct {
	BaseEvent
	OldStatus Status
	NewStatus Status
	Reason    string
}

// EventName возвращает имя события.
func (e StudentStatusChangedEvent) EventName() string {
	return "student.status_changed"
}

// NewStudentStatusChangedEvent создаёт событие изменения статуса.
func NewStudentStatusChangedEvent(student *Student, oldStatus Status, reason string) StudentStatusChangedEvent {
	return StudentStatusChangedEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		OldStatus: oldStatus,
		NewStatus: student.Status,
		Reason:    reason,
	}
}

// StudentGraduatedEvent - студент закончил программу.
type StudentGraduatedEvent struct {
	BaseEvent
	FinalXP          XP
	FinalLevel       Level
	TotalDaysInProgram int
	TotalTasksCompleted int
}

// EventName возвращает имя события.
func (e StudentGraduatedEvent) EventName() string {
	return "student.graduated"
}

// NewStudentGraduatedEvent создаёт событие выпуска.
func NewStudentGraduatedEvent(student *Student, totalTasks int) StudentGraduatedEvent {
	daysInProgram := int(time.Since(student.JoinedAt).Hours() / 24)

	return StudentGraduatedEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		FinalXP:            student.CurrentXP,
		FinalLevel:         student.Level(),
		TotalDaysInProgram: daysInProgram,
		TotalTasksCompleted: totalTasks,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// PREFERENCES EVENTS
// ══════════════════════════════════════════════════════════════════════════════

// PreferencesUpdatedEvent - студент обновил настройки.
type PreferencesUpdatedEvent struct {
	BaseEvent
	OldPreferences NotificationPreferences
	NewPreferences NotificationPreferences
}

// EventName возвращает имя события.
func (e PreferencesUpdatedEvent) EventName() string {
	return "student.preferences_updated"
}

// NewPreferencesUpdatedEvent создаёт событие обновления настроек.
func NewPreferencesUpdatedEvent(student *Student, oldPrefs NotificationPreferences) PreferencesUpdatedEvent {
	return PreferencesUpdatedEvent{
		BaseEvent: BaseEvent{
			Timestamp:   time.Now().UTC(),
			StudentID:   student.ID,
			StudentName: student.DisplayName,
		},
		OldPreferences: oldPrefs,
		NewPreferences: student.Preferences,
	}
}
