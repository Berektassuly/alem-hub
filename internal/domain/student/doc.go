// Package student содержит доменную модель студента Alem School.
//
// Это ядро бизнес-логики системы "Alem Community Hub". Пакет определяет:
//
//   - Сущности (Entities): Student, Progress, DailyGrind, Streak, Achievement
//   - Value Objects: TelegramID, XP, Level, Cohort, Status
//   - Доменные события (Events): StudentRegistered, XPGained, TaskCompleted и др.
//   - Интерфейсы репозиториев: Repository, ProgressRepository, OnlineTracker
//
// # Архитектурные принципы
//
// Пакет следует принципам Clean Architecture и DDD:
//
//  1. Нулевые внешние зависимости - только стандартная библиотека Go
//  2. Dependency Inversion - определяет интерфейсы, которые реализуются в infrastructure
//  3. Rich Domain Model - бизнес-логика инкапсулирована в сущностях
//
// # Философия проекта
//
// "От конкуренции к сотрудничеству" - система превращает сухой лидерборд
// в инструмент взаимопомощи. Студенты могут найти помощников среди тех,
// кто уже решил сложную задачу, а лидеры получают признание как эксперты.
//
// # Основные сущности
//
// Student - центральная сущность, представляющая студента:
//
//	student, err := NewStudent(NewStudentParams{
//	    ID:          uuid.New().String(),
//	    TelegramID:  TelegramID(123456789),
//	    Email:       "student@alem.school",
//	    DisplayName: "Имя Студента",
//	    Cohort:      Cohort("2024-spring"),
//	    InitialXP:   XP(0),
//	})
//
// DailyGrind - дневной прогресс студента:
//
//	grind := NewDailyGrind(student.ID, student.CurrentXP, currentRank)
//	grind.RecordXPGain(newXP)
//	grind.RecordTaskCompletion()
//
// Streak - серия активных дней:
//
//	streak := NewStreak(student.ID)
//	streak.RecordActivity(time.Now())
//
// # Доменные события
//
// Система использует Event-Driven подход. При изменении состояния
// создаются события, на которые реагируют обработчики:
//
//	// При изменении XP
//	event := NewXPGainedEvent(student, oldXP, "task_completed", taskID)
//
//	// При получении достижения
//	event := NewAchievementUnlockedEvent(student, achievement)
//
// # Репозитории
//
// Пакет определяет интерфейсы репозиториев (реализации в infrastructure):
//
//   - Repository: CRUD операции для студентов
//   - ProgressRepository: история XP, стрики, достижения
//   - OnlineTracker: отслеживание онлайн-статуса
//   - SyncRepository: синхронизация с Alem API
//   - Cache: кеширование данных студентов
//
// # Пример использования
//
// Создание и обновление студента:
//
//	// Создание
//	student, err := NewStudent(params)
//	if err != nil {
//	    return err
//	}
//
//	// Обновление XP
//	oldXP := student.CurrentXP
//	delta, err := student.UpdateXP(XP(1500))
//	if delta > 0 {
//	    event := NewXPGainedEvent(student, oldXP, "task_completed", "task-123")
//	    eventBus.Publish(event)
//	}
//
//	// Проверка достижений
//	checker := NewAchievementChecker()
//	newAchievements := checker.CheckNewAchievements(student, streak, rank, existing)
//	for _, achievement := range newAchievements {
//	    event := NewAchievementUnlockedEvent(student, achievement)
//	    eventBus.Publish(event)
//	}
package student
