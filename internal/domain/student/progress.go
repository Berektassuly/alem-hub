package student

import (
	"errors"
	"sort"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// PROGRESS VALUE OBJECTS
// ══════════════════════════════════════════════════════════════════════════════

// Progress представляет полный прогресс студента.
type Progress struct {
	// StudentID - идентификатор студента.
	StudentID string

	// CurrentXP - текущий XP.
	CurrentXP XP

	// CurrentLevel - текущий уровень.
	CurrentLevel Level

	// DailyHistory - история XP по дням (последние 30 дней).
	DailyHistory []DailyXPEntry

	// CurrentStreak - текущая серия дней активности.
	CurrentStreak int

	// BestStreak - лучшая серия дней активности.
	BestStreak int

	// TotalTasksCompleted - общее количество выполненных задач.
	TotalTasksCompleted int

	// Achievements - полученные достижения.
	Achievements []Achievement

	// LastActivityAt - время последней активности.
	LastActivityAt time.Time
}

// DailyXPEntry представляет запись XP за один день.
type DailyXPEntry struct {
	// Date - дата (без времени).
	Date time.Time

	// XPGained - заработано XP за день.
	XPGained XP

	// TasksCompleted - выполнено задач за день.
	TasksCompleted int

	// SessionMinutes - суммарное время сессий в минутах.
	SessionMinutes int
}

// XPHistory представляет историю изменений XP.
type XPHistory struct {
	// Entries - записи истории (от старых к новым).
	Entries []XPHistoryEntry
}

// XPHistoryEntry - одна запись в истории XP.
type XPHistoryEntry struct {
	// Timestamp - время изменения.
	Timestamp time.Time

	// OldXP - XP до изменения.
	OldXP XP

	// NewXP - XP после изменения.
	NewXP XP

	// Delta - изменение XP.
	Delta XP

	// Reason - причина изменения (task_completed, bonus, correction).
	Reason string

	// TaskID - ID задачи (если применимо).
	TaskID string
}

// ══════════════════════════════════════════════════════════════════════════════
// DAILY GRIND (Daily Progress Tracking)
// ══════════════════════════════════════════════════════════════════════════════

// DailyGrind представляет дневной прогресс студента ("Daily Grind").
// Это ключевая фича для отслеживания ежедневной активности.
type DailyGrind struct {
	// StudentID - идентификатор студента.
	StudentID string

	// Date - дата (начало дня в UTC).
	Date time.Time

	// XPStart - XP на начало дня.
	XPStart XP

	// XPCurrent - текущий XP.
	XPCurrent XP

	// XPGained - заработано XP сегодня.
	XPGained XP

	// TasksCompleted - задач выполнено сегодня.
	TasksCompleted int

	// SessionsCount - количество сессий за день.
	SessionsCount int

	// TotalSessionMinutes - общее время сессий в минутах.
	TotalSessionMinutes int

	// FirstActivityAt - время первой активности за день.
	FirstActivityAt time.Time

	// LastActivityAt - время последней активности за день.
	LastActivityAt time.Time

	// RankAtStart - позиция в рейтинге на начало дня.
	RankAtStart int

	// RankCurrent - текущая позиция в рейтинге.
	RankCurrent int

	// RankChange - изменение позиции за день.
	RankChange int

	// StreakDay - какой это день серии (1, 2, 3...).
	StreakDay int
}

// NewDailyGrind создаёт новый DailyGrind для студента.
func NewDailyGrind(studentID string, currentXP XP, currentRank int) *DailyGrind {
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	return &DailyGrind{
		StudentID:           studentID,
		Date:                startOfDay,
		XPStart:             currentXP,
		XPCurrent:           currentXP,
		XPGained:            0,
		TasksCompleted:      0,
		SessionsCount:       0,
		TotalSessionMinutes: 0,
		FirstActivityAt:     time.Time{},
		LastActivityAt:      time.Time{},
		RankAtStart:         currentRank,
		RankCurrent:         currentRank,
		RankChange:          0,
		StreakDay:           0,
	}
}

// RecordXPGain записывает получение XP.
func (dg *DailyGrind) RecordXPGain(newXP XP) {
	oldGained := dg.XPGained
	dg.XPCurrent = newXP
	dg.XPGained = newXP.Diff(dg.XPStart)

	// Если это первое изменение XP за день
	if oldGained == 0 && dg.XPGained > 0 {
		dg.FirstActivityAt = time.Now().UTC()
	}
	dg.LastActivityAt = time.Now().UTC()
}

// RecordTaskCompletion записывает выполнение задачи.
func (dg *DailyGrind) RecordTaskCompletion() {
	dg.TasksCompleted++
	now := time.Now().UTC()

	if dg.FirstActivityAt.IsZero() {
		dg.FirstActivityAt = now
	}
	dg.LastActivityAt = now
}

// RecordSession записывает сессию работы.
func (dg *DailyGrind) RecordSession(minutes int) {
	dg.SessionsCount++
	dg.TotalSessionMinutes += minutes
	dg.LastActivityAt = time.Now().UTC()

	if dg.FirstActivityAt.IsZero() {
		dg.FirstActivityAt = dg.LastActivityAt
	}
}

// UpdateRank обновляет текущий ранг и вычисляет изменение.
func (dg *DailyGrind) UpdateRank(newRank int) {
	dg.RankCurrent = newRank
	dg.RankChange = dg.RankAtStart - newRank // Положительное = поднялся
}

// IsActive возвращает true, если была активность сегодня.
func (dg *DailyGrind) IsActive() bool {
	return dg.XPGained > 0 || dg.TasksCompleted > 0 || dg.SessionsCount > 0
}

// ActiveHours возвращает количество часов активности.
func (dg *DailyGrind) ActiveHours() float64 {
	return float64(dg.TotalSessionMinutes) / 60.0
}

// Summary возвращает текстовое резюме дня.
func (dg *DailyGrind) Summary() string {
	if !dg.IsActive() {
		return "Сегодня пока без активности"
	}

	rankChangeStr := ""
	if dg.RankChange > 0 {
		rankChangeStr = " 📈"
	} else if dg.RankChange < 0 {
		rankChangeStr = " 📉"
	}

	return "+" + string(rune(dg.XPGained)) + " XP, " +
		string(rune(dg.TasksCompleted)) + " задач" + rankChangeStr
}

// ══════════════════════════════════════════════════════════════════════════════
// STREAK (Серия активных дней)
// ══════════════════════════════════════════════════════════════════════════════

// Streak представляет серию активных дней.
type Streak struct {
	// StudentID - идентификатор студента.
	StudentID string

	// CurrentStreak - текущая серия дней.
	CurrentStreak int

	// BestStreak - лучшая серия дней.
	BestStreak int

	// LastActiveDate - дата последней активности.
	LastActiveDate time.Time

	// StreakStartDate - дата начала текущей серии.
	StreakStartDate time.Time
}

// NewStreak создаёт новый трекер серии.
func NewStreak(studentID string) *Streak {
	return &Streak{
		StudentID:       studentID,
		CurrentStreak:   0,
		BestStreak:      0,
		LastActiveDate:  time.Time{},
		StreakStartDate: time.Time{},
	}
}

// RecordActivity записывает активность и обновляет серию.
func (s *Streak) RecordActivity(date time.Time) {
	dateOnly := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	// Если это первая активность
	if s.LastActiveDate.IsZero() {
		s.CurrentStreak = 1
		s.BestStreak = 1
		s.LastActiveDate = dateOnly
		s.StreakStartDate = dateOnly
		return
	}

	lastDateOnly := time.Date(
		s.LastActiveDate.Year(),
		s.LastActiveDate.Month(),
		s.LastActiveDate.Day(),
		0, 0, 0, 0, time.UTC,
	)

	daysDiff := int(dateOnly.Sub(lastDateOnly).Hours() / 24)

	switch daysDiff {
	case 0:
		// Тот же день - ничего не меняем
		return
	case 1:
		// Следующий день - продолжаем серию
		s.CurrentStreak++
		if s.CurrentStreak > s.BestStreak {
			s.BestStreak = s.CurrentStreak
		}
	default:
		// Пропущены дни - сбрасываем серию
		s.CurrentStreak = 1
		s.StreakStartDate = dateOnly
	}

	s.LastActiveDate = dateOnly
}

// IsBroken проверяет, сломана ли серия (пропущен вчерашний день).
func (s *Streak) IsBroken() bool {
	if s.LastActiveDate.IsZero() {
		return false
	}

	today := time.Now().UTC()
	todayOnly := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	lastOnly := time.Date(
		s.LastActiveDate.Year(),
		s.LastActiveDate.Month(),
		s.LastActiveDate.Day(),
		0, 0, 0, 0, time.UTC,
	)

	daysDiff := int(todayOnly.Sub(lastOnly).Hours() / 24)
	return daysDiff > 1
}

// DaysUntilStreakBreaks возвращает количество дней до сброса серии.
// Возвращает 0, если серия уже сброшена, или 1, если нужно быть активным сегодня.
func (s *Streak) DaysUntilStreakBreaks() int {
	if s.LastActiveDate.IsZero() || s.CurrentStreak == 0 {
		return 0
	}

	today := time.Now().UTC()
	todayOnly := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	lastOnly := time.Date(
		s.LastActiveDate.Year(),
		s.LastActiveDate.Month(),
		s.LastActiveDate.Day(),
		0, 0, 0, 0, time.UTC,
	)

	daysDiff := int(todayOnly.Sub(lastOnly).Hours() / 24)

	switch daysDiff {
	case 0:
		return 2 // Был активен сегодня, есть завтра целый день
	case 1:
		return 1 // Нужно быть активным сегодня
	default:
		return 0 // Серия уже сброшена
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// ACHIEVEMENTS (Достижения)
// ══════════════════════════════════════════════════════════════════════════════

// AchievementType представляет тип достижения.
type AchievementType string

const (
	// AchievementFirstTask - первая выполненная задача.
	AchievementFirstTask AchievementType = "first_task"
	// AchievementStreak7 - 7 дней подряд.
	AchievementStreak7 AchievementType = "streak_7"
	// AchievementStreak30 - 30 дней подряд.
	AchievementStreak30 AchievementType = "streak_30"
	// AchievementTop10 - вошёл в топ-10.
	AchievementTop10 AchievementType = "top_10"
	// AchievementTop50 - вошёл в топ-50.
	AchievementTop50 AchievementType = "top_50"
	// AchievementHelper5 - помог 5 студентам.
	AchievementHelper5 AchievementType = "helper_5"
	// AchievementHelper20 - помог 20 студентам.
	AchievementHelper20 AchievementType = "helper_20"
	// AchievementLevel5 - достиг 5 уровня.
	AchievementLevel5 AchievementType = "level_5"
	// AchievementLevel10 - достиг 10 уровня.
	AchievementLevel10 AchievementType = "level_10"
	// AchievementNightOwl - активность после полуночи.
	AchievementNightOwl AchievementType = "night_owl"
	// AchievementEarlyBird - активность до 7 утра.
	AchievementEarlyBird AchievementType = "early_bird"
	// AchievementComebackKid - вернулся после 7 дней неактивности.
	AchievementComebackKid AchievementType = "comeback_kid"
)

// Achievement представляет полученное достижение.
type Achievement struct {
	// Type - тип достижения.
	Type AchievementType

	// UnlockedAt - когда получено.
	UnlockedAt time.Time

	// Metadata - дополнительные данные (например, streak_count).
	Metadata map[string]interface{}
}

// AchievementDefinition описывает достижение.
type AchievementDefinition struct {
	Type        AchievementType
	Name        string
	Description string
	Emoji       string
	XPBonus     XP
}

// GetAchievementDefinitions возвращает все определения достижений.
func GetAchievementDefinitions() []AchievementDefinition {
	return []AchievementDefinition{
		{AchievementFirstTask, "Первая победа", "Выполнена первая задача", "🎯", 50},
		{AchievementStreak7, "Неделя огня", "7 дней подряд", "🔥", 100},
		{AchievementStreak30, "Железная воля", "30 дней подряд", "💪", 500},
		{AchievementTop10, "Элита", "Вошёл в топ-10", "🏆", 200},
		{AchievementTop50, "Восходящая звезда", "Вошёл в топ-50", "⭐", 100},
		{AchievementHelper5, "Добрый самаритянин", "Помог 5 студентам", "🤝", 150},
		{AchievementHelper20, "Наставник", "Помог 20 студентам", "🎓", 400},
		{AchievementLevel5, "Подмастерье", "Достиг 5 уровня", "📚", 100},
		{AchievementLevel10, "Мастер", "Достиг 10 уровня", "🧙", 250},
		{AchievementNightOwl, "Ночная сова", "Активность после полуночи", "🦉", 25},
		{AchievementEarlyBird, "Ранняя пташка", "Активность до 7 утра", "🐦", 25},
		{AchievementComebackKid, "Вернулся!", "Вернулся после недели", "🔄", 75},
	}
}

// GetAchievementDefinition возвращает определение достижения по типу.
func GetAchievementDefinition(t AchievementType) (AchievementDefinition, bool) {
	for _, def := range GetAchievementDefinitions() {
		if def.Type == t {
			return def, true
		}
	}
	return AchievementDefinition{}, false
}

// ══════════════════════════════════════════════════════════════════════════════
// PROGRESS CALCULATIONS
// ══════════════════════════════════════════════════════════════════════════════

// ProgressCalculator вычисляет прогресс студента.
type ProgressCalculator struct{}

// NewProgressCalculator создаёт калькулятор прогресса.
func NewProgressCalculator() *ProgressCalculator {
	return &ProgressCalculator{}
}

// CalculateProgress вычисляет полный прогресс студента.
func (pc *ProgressCalculator) CalculateProgress(
	student *Student,
	history []XPHistoryEntry,
	streak *Streak,
	achievements []Achievement,
) *Progress {
	dailyHistory := pc.aggregateDailyHistory(history)

	currentStreak := 0
	bestStreak := 0
	if streak != nil {
		currentStreak = streak.CurrentStreak
		bestStreak = streak.BestStreak
	}

	tasksCompleted := pc.countCompletedTasks(history)

	return &Progress{
		StudentID:           student.ID,
		CurrentXP:           student.CurrentXP,
		CurrentLevel:        student.Level(),
		DailyHistory:        dailyHistory,
		CurrentStreak:       currentStreak,
		BestStreak:          bestStreak,
		TotalTasksCompleted: tasksCompleted,
		Achievements:        achievements,
		LastActivityAt:      student.LastSeenAt,
	}
}

// aggregateDailyHistory агрегирует историю XP по дням.
func (pc *ProgressCalculator) aggregateDailyHistory(history []XPHistoryEntry) []DailyXPEntry {
	if len(history) == 0 {
		return nil
	}

	// Группируем по датам
	byDate := make(map[string]*DailyXPEntry)

	for _, entry := range history {
		dateKey := entry.Timestamp.Format("2006-01-02")
		dateStart := time.Date(
			entry.Timestamp.Year(),
			entry.Timestamp.Month(),
			entry.Timestamp.Day(),
			0, 0, 0, 0, time.UTC,
		)

		daily, exists := byDate[dateKey]
		if !exists {
			daily = &DailyXPEntry{
				Date:           dateStart,
				XPGained:       0,
				TasksCompleted: 0,
			}
			byDate[dateKey] = daily
		}

		if entry.Delta > 0 {
			daily.XPGained += entry.Delta
		}

		if entry.Reason == "task_completed" {
			daily.TasksCompleted++
		}
	}

	// Преобразуем в слайс и сортируем
	result := make([]DailyXPEntry, 0, len(byDate))
	for _, daily := range byDate {
		result = append(result, *daily)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Date.Before(result[j].Date)
	})

	// Ограничиваем последними 30 днями
	if len(result) > 30 {
		result = result[len(result)-30:]
	}

	return result
}

// countCompletedTasks подсчитывает количество выполненных задач.
func (pc *ProgressCalculator) countCompletedTasks(history []XPHistoryEntry) int {
	count := 0
	for _, entry := range history {
		if entry.Reason == "task_completed" {
			count++
		}
	}
	return count
}

// ══════════════════════════════════════════════════════════════════════════════
// ACHIEVEMENT CHECKER
// ══════════════════════════════════════════════════════════════════════════════

// AchievementChecker проверяет условия для разблокировки достижений.
type AchievementChecker struct{}

// NewAchievementChecker создаёт проверщик достижений.
func NewAchievementChecker() *AchievementChecker {
	return &AchievementChecker{}
}

// ErrAchievementAlreadyUnlocked - достижение уже разблокировано.
var ErrAchievementAlreadyUnlocked = errors.New("achievement already unlocked")

// CheckNewAchievements проверяет и возвращает список новых достижений.
func (ac *AchievementChecker) CheckNewAchievements(
	student *Student,
	streak *Streak,
	rank int,
	existingAchievements []Achievement,
) []Achievement {
	existing := make(map[AchievementType]bool)
	for _, a := range existingAchievements {
		existing[a.Type] = true
	}

	var newAchievements []Achievement
	now := time.Now().UTC()

	// Проверка уровней
	if student.Level() >= 5 && !existing[AchievementLevel5] {
		newAchievements = append(newAchievements, Achievement{
			Type:       AchievementLevel5,
			UnlockedAt: now,
		})
	}
	if student.Level() >= 10 && !existing[AchievementLevel10] {
		newAchievements = append(newAchievements, Achievement{
			Type:       AchievementLevel10,
			UnlockedAt: now,
		})
	}

	// Проверка серий
	if streak != nil {
		if streak.CurrentStreak >= 7 && !existing[AchievementStreak7] {
			newAchievements = append(newAchievements, Achievement{
				Type:       AchievementStreak7,
				UnlockedAt: now,
				Metadata:   map[string]interface{}{"streak": streak.CurrentStreak},
			})
		}
		if streak.CurrentStreak >= 30 && !existing[AchievementStreak30] {
			newAchievements = append(newAchievements, Achievement{
				Type:       AchievementStreak30,
				UnlockedAt: now,
				Metadata:   map[string]interface{}{"streak": streak.CurrentStreak},
			})
		}
	}

	// Проверка ранга
	if rank > 0 && rank <= 10 && !existing[AchievementTop10] {
		newAchievements = append(newAchievements, Achievement{
			Type:       AchievementTop10,
			UnlockedAt: now,
			Metadata:   map[string]interface{}{"rank": rank},
		})
	}
	if rank > 0 && rank <= 50 && !existing[AchievementTop50] {
		newAchievements = append(newAchievements, Achievement{
			Type:       AchievementTop50,
			UnlockedAt: now,
			Metadata:   map[string]interface{}{"rank": rank},
		})
	}

	// Проверка помощи
	if student.HelpCount >= 5 && !existing[AchievementHelper5] {
		newAchievements = append(newAchievements, Achievement{
			Type:       AchievementHelper5,
			UnlockedAt: now,
			Metadata:   map[string]interface{}{"help_count": student.HelpCount},
		})
	}
	if student.HelpCount >= 20 && !existing[AchievementHelper20] {
		newAchievements = append(newAchievements, Achievement{
			Type:       AchievementHelper20,
			UnlockedAt: now,
			Metadata:   map[string]interface{}{"help_count": student.HelpCount},
		})
	}

	// Проверка времени активности
	hour := now.Hour()
	if hour >= 0 && hour < 5 && !existing[AchievementNightOwl] {
		newAchievements = append(newAchievements, Achievement{
			Type:       AchievementNightOwl,
			UnlockedAt: now,
		})
	}
	if hour >= 5 && hour < 7 && !existing[AchievementEarlyBird] {
		newAchievements = append(newAchievements, Achievement{
			Type:       AchievementEarlyBird,
			UnlockedAt: now,
		})
	}

	return newAchievements
}

// CheckComebackKid проверяет достижение "Вернулся после недели".
func (ac *AchievementChecker) CheckComebackKid(
	lastSeen time.Time,
	existingAchievements []Achievement,
) *Achievement {
	for _, a := range existingAchievements {
		if a.Type == AchievementComebackKid {
			return nil // Уже есть
		}
	}

	daysSince := int(time.Since(lastSeen).Hours() / 24)
	if daysSince >= 7 {
		return &Achievement{
			Type:       AchievementComebackKid,
			UnlockedAt: time.Now().UTC(),
			Metadata:   map[string]interface{}{"days_away": daysSince},
		}
	}

	return nil
}

// CheckFirstTask проверяет достижение "Первая задача".
func (ac *AchievementChecker) CheckFirstTask(
	totalTasksCompleted int,
	existingAchievements []Achievement,
) *Achievement {
	for _, a := range existingAchievements {
		if a.Type == AchievementFirstTask {
			return nil // Уже есть
		}
	}

	if totalTasksCompleted >= 1 {
		return &Achievement{
			Type:       AchievementFirstTask,
			UnlockedAt: time.Now().UTC(),
		}
	}

	return nil
}
