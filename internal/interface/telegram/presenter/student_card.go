// Package presenter formats data for Telegram display.
// Presenters handle the conversion from domain objects to user-friendly
// Telegram messages, keyboards, and other UI elements.
package presenter

import (
	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"fmt"
	"strings"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT CARD PRESENTER
// Форматирует карточку студента для красивого отображения в Telegram.
// Показывает: позицию, XP, уровень, прогресс, статистику, достижения.
// Философия: карточка - это "паспорт" студента в сообществе.
// ══════════════════════════════════════════════════════════════════════════════

// StudentCardPresenter форматирует данные карточки студента для Telegram.
type StudentCardPresenter struct {
	keyboardBuilder *KeyboardBuilder
}

// NewStudentCardPresenter создаёт новый презентер карточки студента.
func NewStudentCardPresenter() *StudentCardPresenter {
	return &StudentCardPresenter{
		keyboardBuilder: NewKeyboardBuilder(),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MAIN STUDENT CARD VIEW
// ─────────────────────────────────────────────────────────────────────────────

// StudentCardView содержит отформатированные данные карточки студента.
type StudentCardView struct {
	// Text - основной текст сообщения (с HTML-разметкой).
	Text string

	// Keyboard - inline-клавиатура.
	Keyboard *InlineKeyboard

	// ParseMode - режим парсинга ("HTML" или "Markdown").
	ParseMode string
}

// FormatStudentCard форматирует полную карточку студента (команда /me).
func (p *StudentCardPresenter) FormatStudentCard(
	result *query.GetStudentRankResult,
	dailyGrind *student.DailyGrind,
	achievements []student.Achievement,
) *StudentCardView {
	var sb strings.Builder
	dto := result.Student

	// Заголовок с именем
	sb.WriteString(p.formatHeader(&dto))
	sb.WriteString("\n\n")

	// Блок позиции в рейтинге
	sb.WriteString(p.formatRankSection(&dto))
	sb.WriteString("\n\n")

	// Блок XP и уровня
	sb.WriteString(p.formatXPSection(&dto))
	sb.WriteString("\n\n")

	// Daily Grind (если есть данные)
	if dailyGrind != nil && dailyGrind.IsActive() {
		sb.WriteString(p.formatDailyGrindSection(dailyGrind))
		sb.WriteString("\n\n")
	}

	// Статистика помощи
	sb.WriteString(p.formatHelpSection(&dto))

	// Достижения (если есть)
	if len(achievements) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(p.formatAchievementsSection(achievements))
	}

	// Мотивационное сообщение
	if result.Message != "" {
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("💬 <i>%s</i>", p.escapeHTML(result.Message)))
	}

	// Клавиатура
	keyboard := p.keyboardBuilder.StudentCardKeyboard(dto.StudentID)

	return &StudentCardView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// HEADER SECTION
// ─────────────────────────────────────────────────────────────────────────────

// formatHeader форматирует заголовок карточки.
func (p *StudentCardPresenter) formatHeader(dto *query.StudentRankDTO) string {
	var sb strings.Builder

	// Онлайн-статус
	if dto.IsOnline {
		sb.WriteString("🟢 ")
	}

	// Имя
	sb.WriteString(fmt.Sprintf("<b>%s</b>", p.escapeHTML(dto.DisplayName)))

	// Логин
	if dto.AlemLogin != dto.DisplayName {
		sb.WriteString(fmt.Sprintf(" (@%s)", p.escapeHTML(dto.AlemLogin)))
	}

	// Когорта
	if dto.Cohort != "" {
		sb.WriteString(fmt.Sprintf(" • %s", dto.Cohort))
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// RANK SECTION
// ─────────────────────────────────────────────────────────────────────────────

// formatRankSection форматирует блок позиции в рейтинге.
func (p *StudentCardPresenter) formatRankSection(dto *query.StudentRankDTO) string {
	var sb strings.Builder

	sb.WriteString("🏆 <b>Позиция в рейтинге</b>\n")

	// Текущий ранг с эмодзи
	rankEmoji := p.getRankEmoji(dto.Rank)
	sb.WriteString(fmt.Sprintf("%s <b>#%d</b> из %d", rankEmoji, dto.Rank, dto.TotalStudents))

	// Изменение ранга
	if dto.RankChange != 0 {
		sb.WriteString(" ")
		sb.WriteString(p.formatRankChange(dto.RankChange))
	}

	sb.WriteString("\n")

	// Процентиль
	percentileStr := p.formatPercentile(dto.Percentile)
	sb.WriteString(fmt.Sprintf("📊 %s", percentileStr))

	// XP до следующего места
	if dto.XPToNextRank > 0 && dto.NextRankStudent != "" {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("🎯 До %s: <b>%d XP</b>",
			p.escapeHTML(dto.NextRankStudent),
			dto.XPToNextRank,
		))
	}

	// Лучший ранг
	if dto.BestRank > 0 && dto.BestRank < dto.Rank {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("⭐ Лучший результат: #%d", dto.BestRank))
	}

	return sb.String()
}

// getRankEmoji возвращает эмодзи для позиции.
func (p *StudentCardPresenter) getRankEmoji(rank int) string {
	switch rank {
	case 1:
		return "🥇"
	case 2:
		return "🥈"
	case 3:
		return "🥉"
	default:
		if rank <= 10 {
			return "🏅"
		}
		if rank <= 50 {
			return "⭐"
		}
		return "📍"
	}
}

// formatRankChange форматирует изменение позиции.
func (p *StudentCardPresenter) formatRankChange(change int) string {
	if change > 0 {
		return fmt.Sprintf("<b>↑%d</b>", change)
	}
	if change < 0 {
		return fmt.Sprintf("<i>↓%d</i>", -change)
	}
	return "➖"
}

// formatPercentile форматирует процентиль.
func (p *StudentCardPresenter) formatPercentile(percentile float64) string {
	if percentile >= 99 {
		return "🔥 Топ 1% — элита!"
	}
	if percentile >= 95 {
		return "💎 Топ 5% — отлично!"
	}
	if percentile >= 90 {
		return "🌟 Топ 10%"
	}
	if percentile >= 75 {
		return "✨ Топ 25%"
	}
	if percentile >= 50 {
		return "📈 Верхняя половина"
	}
	return "💪 Есть куда расти!"
}

// ─────────────────────────────────────────────────────────────────────────────
// XP SECTION
// ─────────────────────────────────────────────────────────────────────────────

// formatXPSection форматирует блок XP и уровня.
func (p *StudentCardPresenter) formatXPSection(dto *query.StudentRankDTO) string {
	var sb strings.Builder

	sb.WriteString("📊 <b>Прогресс</b>\n")

	// XP
	sb.WriteString(fmt.Sprintf("⚡ XP: <b>%s</b>\n", p.formatNumber(dto.XP)))

	// Уровень с прогресс-баром
	sb.WriteString(fmt.Sprintf("🎮 Уровень: <b>%d</b>\n", dto.Level))

	// Прогресс до следующего уровня
	progressBar := p.formatProgressBar(dto.LevelProgress)
	sb.WriteString(fmt.Sprintf("%s %d%%\n", progressBar, int(dto.LevelProgress*100)))

	// XP до следующего уровня
	if dto.XPToNextLevel > 0 {
		sb.WriteString(fmt.Sprintf("📈 До уровня %d: <b>%d XP</b>", dto.Level+1, dto.XPToNextLevel))
	}

	return sb.String()
}

// formatProgressBar форматирует прогресс-бар.
func (p *StudentCardPresenter) formatProgressBar(progress float64) string {
	const barLength = 10
	filled := int(progress * float64(barLength))
	if filled > barLength {
		filled = barLength
	}
	if filled < 0 {
		filled = 0
	}

	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < barLength; i++ {
		if i < filled {
			sb.WriteString("█")
		} else {
			sb.WriteString("░")
		}
	}
	sb.WriteString("]")

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// DAILY GRIND SECTION
// ─────────────────────────────────────────────────────────────────────────────

// formatDailyGrindSection форматирует секцию дневного прогресса.
func (p *StudentCardPresenter) formatDailyGrindSection(dg *student.DailyGrind) string {
	var sb strings.Builder

	sb.WriteString("📅 <b>Сегодня</b>\n")

	// XP за сегодня
	xpGained := int(dg.XPGained)
	if xpGained > 0 {
		sb.WriteString(fmt.Sprintf("⚡ +<b>%d XP</b>", xpGained))
	} else {
		sb.WriteString("⚡ Пока без XP")
	}

	// Задачи
	if dg.TasksCompleted > 0 {
		sb.WriteString(fmt.Sprintf(" • 📝 %d %s",
			dg.TasksCompleted,
			p.pluralize(dg.TasksCompleted, "задача", "задачи", "задач"),
		))
	}

	// Изменение ранга за день
	if dg.RankChange != 0 {
		sb.WriteString("\n")
		if dg.RankChange > 0 {
			sb.WriteString(fmt.Sprintf("📈 Поднялся на %d %s",
				dg.RankChange,
				p.pluralize(dg.RankChange, "место", "места", "мест"),
			))
		} else {
			sb.WriteString(fmt.Sprintf("📉 Опустился на %d %s",
				-dg.RankChange,
				p.pluralize(-dg.RankChange, "место", "места", "мест"),
			))
		}
	}

	// Время работы
	if dg.TotalSessionMinutes > 0 {
		hours := dg.TotalSessionMinutes / 60
		mins := dg.TotalSessionMinutes % 60
		sb.WriteString("\n⏱ ")
		if hours > 0 {
			sb.WriteString(fmt.Sprintf("%d ч %d мин активности", hours, mins))
		} else {
			sb.WriteString(fmt.Sprintf("%d мин активности", mins))
		}
	}

	// Streak
	if dg.StreakDay > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("🔥 День %d серии подряд!", dg.StreakDay))
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// HELP SECTION
// ─────────────────────────────────────────────────────────────────────────────

// formatHelpSection форматирует секцию статистики помощи.
func (p *StudentCardPresenter) formatHelpSection(dto *query.StudentRankDTO) string {
	var sb strings.Builder

	sb.WriteString("🤝 <b>Взаимопомощь</b>\n")

	// Статус готовности помогать
	if dto.IsAvailableForHelp {
		sb.WriteString("✅ Готов помогать\n")
	} else {
		sb.WriteString("⏸ Не готов помогать сейчас\n")
	}

	// Рейтинг как помощника (только если есть)
	// HelpCount берём из другого места, пока показываем только рейтинг
	if dto.HelpRating > 0 {
		sb.WriteString(fmt.Sprintf("⭐ Рейтинг помощника: <b>%.1f</b>/5.0", dto.HelpRating))
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// ACHIEVEMENTS SECTION
// ─────────────────────────────────────────────────────────────────────────────

// formatAchievementsSection форматирует секцию достижений.
func (p *StudentCardPresenter) formatAchievementsSection(achievements []student.Achievement) string {
	var sb strings.Builder

	sb.WriteString("🏅 <b>Достижения</b>\n")

	// Показываем первые 5 достижений
	count := 5
	if len(achievements) < count {
		count = len(achievements)
	}

	for i := 0; i < count; i++ {
		ach := achievements[i]
		def, ok := student.GetAchievementDefinition(ach.Type)
		if ok {
			sb.WriteString(fmt.Sprintf("%s %s\n", def.Emoji, def.Name))
		}
	}

	// Если есть ещё
	if len(achievements) > 5 {
		sb.WriteString(fmt.Sprintf("<i>...и ещё %d</i>", len(achievements)-5))
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// OTHER STUDENT PROFILE
// ─────────────────────────────────────────────────────────────────────────────

// ProfileView содержит отформатированные данные профиля другого студента.
type ProfileView struct {
	Text      string
	Keyboard  *InlineKeyboard
	ParseMode string
}

// FormatOtherStudentProfile форматирует профиль другого студента.
func (p *StudentCardPresenter) FormatOtherStudentProfile(
	dto *query.StudentRankDTO,
	taskContext string,
) *ProfileView {
	var sb strings.Builder

	// Заголовок
	if dto.IsOnline {
		sb.WriteString("🟢 ")
	}
	sb.WriteString(fmt.Sprintf("<b>%s</b>", p.escapeHTML(dto.DisplayName)))

	if dto.AlemLogin != dto.DisplayName {
		sb.WriteString(fmt.Sprintf(" (@%s)", p.escapeHTML(dto.AlemLogin)))
	}

	sb.WriteString("\n\n")

	// Позиция
	rankEmoji := p.getRankEmoji(dto.Rank)
	sb.WriteString(fmt.Sprintf("%s Позиция: <b>#%d</b>\n", rankEmoji, dto.Rank))

	// XP и уровень
	sb.WriteString(fmt.Sprintf("⚡ XP: <b>%s</b>\n", p.formatNumber(dto.XP)))
	sb.WriteString(fmt.Sprintf("🎮 Уровень: <b>%d</b>\n", dto.Level))

	// Рейтинг помощника
	if dto.HelpRating > 0 {
		sb.WriteString(fmt.Sprintf("⭐ Рейтинг помощника: <b>%.1f</b>/5.0\n", dto.HelpRating))
	}

	// Когорта
	if dto.Cohort != "" {
		sb.WriteString(fmt.Sprintf("📅 Поток: %s\n", dto.Cohort))
	}

	// Статус
	if dto.IsOnline {
		sb.WriteString("\n🟢 <i>Сейчас онлайн</i>")
	} else if dto.LastSeenAt != nil {
		elapsed := time.Since(*dto.LastSeenAt)
		sb.WriteString(fmt.Sprintf("\n⚪ <i>Был(а) %s назад</i>", p.formatDuration(elapsed)))
	}

	// Клавиатура
	keyboard := p.keyboardBuilder.ProfileKeyboard(dto.StudentID, taskContext)

	return &ProfileView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS LIST VIEW
// ─────────────────────────────────────────────────────────────────────────────

// HelpersView содержит отформатированные данные списка помощников.
type HelpersView struct {
	Text      string
	Keyboard  *InlineKeyboard
	ParseMode string
}

// FormatHelpersList форматирует список помощников для задачи.
func (p *StudentCardPresenter) FormatHelpersList(
	helpers []query.HelperDTO,
	taskID string,
) *HelpersView {
	var sb strings.Builder

	// Заголовок
	sb.WriteString(fmt.Sprintf("🆘 <b>Помощь по задаче %s</b>\n\n", p.escapeHTML(taskID)))

	if len(helpers) == 0 {
		sb.WriteString("<i>Пока никто не решил эту задачу или все оффлайн.</i>\n")
		sb.WriteString("\n💡 Попробуй позже или спроси в общем чате!")
	} else {
		sb.WriteString("Эти студенты уже решили задачу и могут помочь:\n\n")

		for _, helper := range helpers {
			sb.WriteString(p.formatHelperEntry(&helper))
			sb.WriteString("\n")
		}

		sb.WriteString("\n💡 <i>Выбери, кому написать</i>")
	}

	// Клавиатура
	keyboard := p.keyboardBuilder.HelpersKeyboard(helpers, taskID)

	return &HelpersView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// formatHelperEntry форматирует запись помощника.
func (p *StudentCardPresenter) formatHelperEntry(helper *query.HelperDTO) string {
	var sb strings.Builder

	// Онлайн-статус
	switch helper.OnlineStatus {
	case "online":
		sb.WriteString("🟢 ")
	case "away":
		sb.WriteString("🟡 ")
	default:
		sb.WriteString("⚪ ")
	}

	// Имя
	sb.WriteString(fmt.Sprintf("<b>%s</b>", p.escapeHTML(helper.DisplayName)))

	// Рейтинг
	if helper.HelpRating > 0 {
		sb.WriteString(fmt.Sprintf(" ⭐%.1f", helper.HelpRating))
	}

	// Позиция
	sb.WriteString(fmt.Sprintf(" • #%d", helper.Rank))

	// Когда решил задачу
	if !helper.CompletedTaskAt.IsZero() {
		elapsed := time.Since(helper.CompletedTaskAt)
		if elapsed < 24*time.Hour {
			sb.WriteString(fmt.Sprintf(" • решил %s назад", p.formatDuration(elapsed)))
		}
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// SETTINGS VIEW
// ─────────────────────────────────────────────────────────────────────────────

// SettingsView содержит отформатированные данные настроек.
type SettingsView struct {
	Text      string
	Keyboard  *InlineKeyboard
	ParseMode string
}

// FormatSettings форматирует настройки студента.
func (p *StudentCardPresenter) FormatSettings(stud *student.Student) *SettingsView {
	var sb strings.Builder

	sb.WriteString("⚙️ <b>Настройки уведомлений</b>\n\n")

	// Статусы настроек
	sb.WriteString(p.formatSettingStatus("Изменения рейтинга", stud.Preferences.RankChanges))
	sb.WriteString("\n")
	sb.WriteString(p.formatSettingStatus("Ежедневная сводка", stud.Preferences.DailyDigest))
	sb.WriteString("\n")
	sb.WriteString(p.formatSettingStatus("Запросы помощи", stud.Preferences.HelpRequests))
	sb.WriteString("\n")
	sb.WriteString(p.formatSettingStatus("Напоминания", stud.Preferences.InactivityReminders))
	sb.WriteString("\n\n")

	// Тихие часы
	sb.WriteString("🌙 <b>Тихие часы</b>\n")
	if stud.Preferences.QuietHoursStart == 0 && stud.Preferences.QuietHoursEnd == 0 {
		sb.WriteString("Отключены")
	} else {
		sb.WriteString(fmt.Sprintf("С %02d:00 до %02d:00",
			stud.Preferences.QuietHoursStart,
			stud.Preferences.QuietHoursEnd,
		))
	}

	sb.WriteString("\n\n")
	sb.WriteString("💡 <i>Нажимай кнопки ниже для изменения</i>")

	keyboard := p.keyboardBuilder.SettingsKeyboard(stud)

	return &SettingsView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// formatSettingStatus форматирует статус настройки.
func (p *StudentCardPresenter) formatSettingStatus(name string, enabled bool) string {
	if enabled {
		return fmt.Sprintf("✅ %s", name)
	}
	return fmt.Sprintf("❌ %s", name)
}

// ─────────────────────────────────────────────────────────────────────────────
// WELCOME / ONBOARDING
// ─────────────────────────────────────────────────────────────────────────────

// WelcomeView содержит отформатированные данные приветствия.
type WelcomeView struct {
	Text      string
	Keyboard  *InlineKeyboard
	ParseMode string
}

// FormatWelcomeNew форматирует приветствие для нового пользователя.
func (p *StudentCardPresenter) FormatWelcomeNew() *WelcomeView {
	text := `👋 <b>Добро пожаловать в Alem Community Hub!</b>

Это неофициальный бот сообщества студентов Alem School.

🎯 <b>Что я умею:</b>
• 🏆 Показывать твою позицию в рейтинге
• 👥 Находить тех, кто может помочь с задачей
• 🔔 Уведомлять об изменениях в рейтинге
• 🤝 Связывать студентов для взаимопомощи

📝 <b>Для начала:</b>
Введи свой логин с платформы Alem, чтобы я мог найти тебя в системе.

<i>Пример: /connect mylogin</i>`

	keyboard := NewInlineKeyboard().
		AddRow(
			CallbackButton("❓ Как это работает", "help:how"),
		)

	return &WelcomeView{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// FormatWelcomeBack форматирует приветствие для вернувшегося пользователя.
func (p *StudentCardPresenter) FormatWelcomeBack(dto *query.StudentRankDTO) *WelcomeView {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("👋 <b>С возвращением, %s!</b>\n\n", p.escapeHTML(dto.DisplayName)))

	// Текущая позиция
	rankEmoji := p.getRankEmoji(dto.Rank)
	sb.WriteString(fmt.Sprintf("%s Твоя позиция: <b>#%d</b>", rankEmoji, dto.Rank))

	// Изменение с прошлого раза
	if dto.RankChange != 0 {
		sb.WriteString(" ")
		sb.WriteString(p.formatRankChange(dto.RankChange))
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("⚡ XP: <b>%s</b>\n", p.formatNumber(dto.XP)))

	// Мотивация
	if dto.XPToNextRank > 0 && dto.XPToNextRank <= 100 {
		sb.WriteString(fmt.Sprintf("\n🎯 До следующего места всего <b>%d XP</b>!", dto.XPToNextRank))
	}

	keyboard := p.keyboardBuilder.WelcomeBackKeyboard()

	return &WelcomeView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// FormatOnboardingSuccess форматирует успешное завершение онбординга.
func (p *StudentCardPresenter) FormatOnboardingSuccess(dto *query.StudentRankDTO) *WelcomeView {
	var sb strings.Builder

	sb.WriteString("🎉 <b>Отлично! Ты зарегистрирован!</b>\n\n")

	sb.WriteString(fmt.Sprintf("👤 Имя: <b>%s</b>\n", p.escapeHTML(dto.DisplayName)))
	sb.WriteString(fmt.Sprintf("🏆 Позиция: <b>#%d</b>\n", dto.Rank))
	sb.WriteString(fmt.Sprintf("⚡ XP: <b>%s</b>\n", p.formatNumber(dto.XP)))
	sb.WriteString(fmt.Sprintf("🎮 Уровень: <b>%d</b>\n", dto.Level))

	sb.WriteString("\n✅ Теперь ты будешь получать уведомления об изменениях в рейтинге.")
	sb.WriteString("\n\n💡 <i>Используй /settings чтобы настроить уведомления</i>")

	keyboard := p.keyboardBuilder.OnboardingSuccessKeyboard()

	return &WelcomeView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// DAILY DIGEST
// ─────────────────────────────────────────────────────────────────────────────

// DailyDigestView содержит отформатированные данные ежедневной сводки.
type DailyDigestView struct {
	Text      string
	Keyboard  *InlineKeyboard
	ParseMode string
}

// FormatDailyDigest форматирует ежедневную сводку.
func (p *StudentCardPresenter) FormatDailyDigest(
	dto *query.StudentRankDTO,
	dg *student.DailyGrind,
) *DailyDigestView {
	var sb strings.Builder

	sb.WriteString("📊 <b>Твой день в Alem</b>\n")
	sb.WriteString(fmt.Sprintf("<i>%s</i>\n\n", time.Now().Format("02 января 2006")))

	// XP за день
	if dg != nil && dg.XPGained > 0 {
		sb.WriteString(fmt.Sprintf("⚡ Заработано: <b>+%d XP</b>\n", int(dg.XPGained)))
	} else {
		sb.WriteString("⚡ Сегодня без XP 😢\n")
	}

	// Задачи
	if dg != nil && dg.TasksCompleted > 0 {
		sb.WriteString(fmt.Sprintf("📝 Выполнено: <b>%d %s</b>\n",
			dg.TasksCompleted,
			p.pluralize(dg.TasksCompleted, "задача", "задачи", "задач"),
		))
	}

	// Позиция
	sb.WriteString(fmt.Sprintf("\n🏆 Позиция: <b>#%d</b>", dto.Rank))
	if dg != nil && dg.RankChange != 0 {
		sb.WriteString(" ")
		sb.WriteString(p.formatRankChange(dg.RankChange))
	}
	sb.WriteString("\n")

	// Streak
	if dg != nil && dg.StreakDay > 1 {
		sb.WriteString(fmt.Sprintf("\n🔥 Серия: <b>%d %s</b> подряд!",
			dg.StreakDay,
			p.pluralize(dg.StreakDay, "день", "дня", "дней"),
		))
	}

	// Мотивация на завтра
	sb.WriteString("\n\n💪 Завтра ещё больше возможностей!")

	keyboard := NewInlineKeyboard().
		AddRow(
			CallbackButton("📊 Полная статистика", "cmd:me"),
			CallbackButton("🏆 Лидерборд", "cmd:top"),
		)

	return &DailyDigestView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// UTILITY FUNCTIONS
// ─────────────────────────────────────────────────────────────────────────────

// formatNumber форматирует число с разделителями тысяч.
func (p *StudentCardPresenter) formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	str := fmt.Sprintf("%d", n)
	result := ""
	length := len(str)

	for i, c := range str {
		if i > 0 && (length-i)%3 == 0 {
			result += " "
		}
		result += string(c)
	}

	return result
}

// formatDuration форматирует длительность.
func (p *StudentCardPresenter) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "только что"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		return fmt.Sprintf("%d мин", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%d ч", hours)
	}

	days := int(d.Hours() / 24)
	return fmt.Sprintf("%d %s", days, p.pluralize(days, "день", "дня", "дней"))
}

// escapeHTML экранирует HTML-символы.
func (p *StudentCardPresenter) escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(s)
}

// pluralize возвращает правильную форму слова для числа.
func (p *StudentCardPresenter) pluralize(n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}

	mod10 := n % 10
	mod100 := n % 100

	if mod100 >= 11 && mod100 <= 19 {
		return many
	}

	switch mod10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ERROR STATES
// ─────────────────────────────────────────────────────────────────────────────

// FormatNotFound форматирует сообщение "не найден".
func (p *StudentCardPresenter) FormatNotFound() *StudentCardView {
	text := `❌ <b>Студент не найден</b>

Похоже, ты ещё не зарегистрирован в боте.

📝 <b>Что делать:</b>
Введи команду /start и следуй инструкциям для регистрации.`

	keyboard := NewInlineKeyboard().
		AddRow(
			CallbackButton("🚀 Начать регистрацию", "cmd:start"),
		)

	return &StudentCardView{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// FormatError форматирует сообщение об ошибке.
func (p *StudentCardPresenter) FormatError(message string) *StudentCardView {
	text := fmt.Sprintf(`⚠️ <b>Что-то пошло не так</b>

%s

Попробуй ещё раз через несколько секунд.`, p.escapeHTML(message))

	keyboard := NewInlineKeyboard().
		AddRow(
			CallbackButton("🔄 Попробовать снова", "cmd:me"),
		)

	return &StudentCardView{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}
