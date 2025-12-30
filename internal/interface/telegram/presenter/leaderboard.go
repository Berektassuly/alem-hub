// Package presenter formats data for Telegram display.
// Presenters handle the conversion from domain objects to user-friendly
// Telegram messages, keyboards, and other UI elements.
package presenter

import (
	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"fmt"
	"strings"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD PRESENTER
// Форматирует лидерборд для красивого отображения в Telegram.
// Философия: "От конкуренции к сотрудничеству" - показываем не только место,
// но и возможности для взаимопомощи.
// ══════════════════════════════════════════════════════════════════════════════

// LeaderboardPresenter форматирует данные лидерборда для Telegram.
type LeaderboardPresenter struct {
	keyboardBuilder *KeyboardBuilder
}

// NewLeaderboardPresenter создаёт новый презентер лидерборда.
func NewLeaderboardPresenter() *LeaderboardPresenter {
	return &LeaderboardPresenter{
		keyboardBuilder: NewKeyboardBuilder(),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MAIN LEADERBOARD VIEW
// ─────────────────────────────────────────────────────────────────────────────

// LeaderboardView содержит отформатированные данные для отображения.
type LeaderboardView struct {
	// Text - основной текст сообщения (с HTML-разметкой).
	Text string

	// Keyboard - inline-клавиатура.
	Keyboard *InlineKeyboard

	// ParseMode - режим парсинга ("HTML" или "Markdown").
	ParseMode string
}

// FormatLeaderboard форматирует полный лидерборд.
func (p *LeaderboardPresenter) FormatLeaderboard(
	result *query.GetLeaderboardResult,
	currentUserID string,
	page int,
	onlyOnline bool,
) *LeaderboardView {
	var sb strings.Builder

	// Заголовок
	sb.WriteString(p.formatHeader(result, onlyOnline))
	sb.WriteString("\n\n")

	// Записи лидерборда
	if len(result.Entries) == 0 {
		sb.WriteString("📭 <i>Пока никого нет в списке</i>\n")
	} else {
		for _, entry := range result.Entries {
			isCurrentUser := entry.StudentID == currentUserID
			sb.WriteString(p.formatEntry(&entry, isCurrentUser))
			sb.WriteString("\n")
		}
	}

	// Статистика
	sb.WriteString("\n")
	sb.WriteString(p.formatStats(result))

	// Клавиатура
	keyboard := p.keyboardBuilder.LeaderboardKeyboard(
		page,
		result.HasMore,
		result.Cohort,
		onlyOnline,
	)

	return &LeaderboardView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// HEADER FORMATTING
// ─────────────────────────────────────────────────────────────────────────────

// formatHeader форматирует заголовок лидерборда.
func (p *LeaderboardPresenter) formatHeader(result *query.GetLeaderboardResult, onlyOnline bool) string {
	var sb strings.Builder

	// Основной заголовок
	sb.WriteString("🏆 <b>Лидерборд Alem</b>")

	// Фильтры
	filters := make([]string, 0, 2)

	if result.Cohort != "" {
		filters = append(filters, fmt.Sprintf("📅 %s", result.Cohort))
	}

	if onlyOnline {
		filters = append(filters, "🟢 онлайн")
	}

	if len(filters) > 0 {
		sb.WriteString(" • ")
		sb.WriteString(strings.Join(filters, " • "))
	}

	// Информация о странице
	if result.Page > 1 || result.HasMore {
		sb.WriteString(fmt.Sprintf("\n📄 Страница %d", result.Page))
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// ENTRY FORMATTING
// ─────────────────────────────────────────────────────────────────────────────

// formatEntry форматирует одну запись лидерборда.
func (p *LeaderboardPresenter) formatEntry(entry *query.LeaderboardEntryDTO, isCurrentUser bool) string {
	var sb strings.Builder

	// Позиция с эмодзи
	sb.WriteString(p.formatRank(entry.Rank))
	sb.WriteString(" ")

	// Онлайн-статус
	sb.WriteString(p.formatOnlineIndicator(entry.IsOnline, entry.IsAvailableForHelp))

	// Имя пользователя (выделяем текущего)
	if isCurrentUser {
		sb.WriteString(fmt.Sprintf("<b>→ %s ←</b>", p.escapeHTML(entry.DisplayName)))
	} else {
		sb.WriteString(p.escapeHTML(entry.DisplayName))
	}

	// XP
	sb.WriteString(fmt.Sprintf(" • <code>%s XP</code>", p.formatNumber(entry.XP)))

	// Уровень
	sb.WriteString(fmt.Sprintf(" • Lvl %d", entry.Level))

	// Изменение ранга
	if entry.RankChange != 0 {
		sb.WriteString(" ")
		sb.WriteString(p.formatRankChange(entry.RankChange))
	}

	// Рейтинг помощника (если есть)
	if entry.HelpRating > 0 && entry.IsAvailableForHelp {
		sb.WriteString(fmt.Sprintf(" ⭐%.1f", entry.HelpRating))
	}

	return sb.String()
}

// formatRank форматирует позицию с соответствующим эмодзи.
func (p *LeaderboardPresenter) formatRank(rank int) string {
	switch rank {
	case 1:
		return "🥇"
	case 2:
		return "🥈"
	case 3:
		return "🥉"
	default:
		if rank <= 10 {
			return fmt.Sprintf("🏅%d.", rank)
		}
		if rank <= 50 {
			return fmt.Sprintf("⭐%d.", rank)
		}
		return fmt.Sprintf("%d.", rank)
	}
}

// formatOnlineIndicator форматирует индикатор онлайн-статуса.
func (p *LeaderboardPresenter) formatOnlineIndicator(isOnline, isAvailableForHelp bool) string {
	if isOnline && isAvailableForHelp {
		return "🟢 " // Онлайн и готов помочь
	}
	if isOnline {
		return "🔵 " // Онлайн
	}
	return "" // Оффлайн - не показываем индикатор
}

// formatRankChange форматирует изменение позиции.
func (p *LeaderboardPresenter) formatRankChange(change int) string {
	if change > 0 {
		return fmt.Sprintf("<b>↑%d</b>", change)
	}
	if change < 0 {
		return fmt.Sprintf("<i>↓%d</i>", -change)
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// STATS FORMATTING
// ─────────────────────────────────────────────────────────────────────────────

// formatStats форматирует статистику лидерборда.
func (p *LeaderboardPresenter) formatStats(result *query.GetLeaderboardResult) string {
	var sb strings.Builder

	sb.WriteString("─────────────────────\n")
	sb.WriteString(fmt.Sprintf("👥 Всего: <b>%d</b>", result.TotalCount))

	if result.OnlineCount > 0 {
		sb.WriteString(fmt.Sprintf(" • 🟢 Онлайн: <b>%d</b>", result.OnlineCount))
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("📊 Средний XP: <b>%s</b>", p.formatNumber(result.AverageXP)))
	sb.WriteString(fmt.Sprintf(" • Медиана: <b>%s</b>", p.formatNumber(result.MedianXP)))

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// COMPACT VIEW (для уведомлений)
// ─────────────────────────────────────────────────────────────────────────────

// FormatCompactTop форматирует компактный топ-N для уведомлений.
func (p *LeaderboardPresenter) FormatCompactTop(entries []query.LeaderboardEntryDTO, limit int) string {
	var sb strings.Builder

	sb.WriteString("🏆 <b>Топ сейчас:</b>\n")

	count := limit
	if count > len(entries) {
		count = len(entries)
	}

	for i := 0; i < count; i++ {
		entry := entries[i]
		sb.WriteString(fmt.Sprintf("%s %s • %s XP\n",
			p.formatRank(entry.Rank),
			p.escapeHTML(entry.DisplayName),
			p.formatNumber(entry.XP),
		))
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// NEIGHBORS VIEW
// ─────────────────────────────────────────────────────────────────────────────

// NeighborsView содержит отформатированные данные для соседей.
type NeighborsView struct {
	Text      string
	Keyboard  *InlineKeyboard
	ParseMode string
}

// FormatNeighbors форматирует соседей по рангу.
func (p *LeaderboardPresenter) FormatNeighbors(
	entries []query.LeaderboardEntryDTO,
	currentUserID string,
	currentRank int,
) *NeighborsView {
	var sb strings.Builder

	sb.WriteString("👥 <b>Твои соседи по рейтингу</b>\n\n")

	if len(entries) == 0 {
		sb.WriteString("<i>Соседей не найдено</i>\n")
	} else {
		var userEntry *query.LeaderboardEntryDTO

		// Показываем соседей сверху
		aboveCount := 0
		for _, entry := range entries {
			entryCopy := entry
			if entry.StudentID == currentUserID {
				userEntry = &entryCopy
				continue
			}
			if entry.Rank < currentRank {
				sb.WriteString(p.formatNeighborEntry(&entryCopy, "above"))
				sb.WriteString("\n")
				aboveCount++
			}
		}

		// Текущий пользователь
		if userEntry != nil {
			if aboveCount > 0 {
				sb.WriteString("        ⬆️\n")
			}
			sb.WriteString(p.formatNeighborEntry(userEntry, "current"))
			sb.WriteString("\n")
		}

		// Показываем соседей снизу
		belowStarted := false
		for _, entry := range entries {
			entryCopy := entry
			if entry.StudentID == currentUserID {
				continue
			}
			if entry.Rank > currentRank {
				if !belowStarted && userEntry != nil {
					sb.WriteString("        ⬇️\n")
					belowStarted = true
				}
				sb.WriteString(p.formatNeighborEntry(&entryCopy, "below"))
				sb.WriteString("\n")
			}
		}
	}

	// Подсказка
	sb.WriteString("\n💡 <i>Нажми на имя, чтобы посмотреть профиль</i>")

	keyboard := p.keyboardBuilder.NeighborsKeyboard()

	return &NeighborsView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// formatNeighborEntry форматирует запись соседа.
func (p *LeaderboardPresenter) formatNeighborEntry(entry *query.LeaderboardEntryDTO, position string) string {
	var sb strings.Builder

	// Позиция
	sb.WriteString(p.formatRank(entry.Rank))
	sb.WriteString(" ")

	// Онлайн-статус
	if entry.IsOnline {
		sb.WriteString("🟢 ")
	}

	// Имя
	switch position {
	case "current":
		sb.WriteString(fmt.Sprintf("<b>→ %s ← (ты)</b>", p.escapeHTML(entry.DisplayName)))
	case "above":
		sb.WriteString(fmt.Sprintf("🎯 %s", p.escapeHTML(entry.DisplayName)))
	default:
		sb.WriteString(p.escapeHTML(entry.DisplayName))
	}

	// XP
	sb.WriteString(fmt.Sprintf(" • <code>%s XP</code>", p.formatNumber(entry.XP)))

	// XP разница (для соседей)
	if position == "above" {
		sb.WriteString(" ⬆️")
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// ONLINE VIEW
// ─────────────────────────────────────────────────────────────────────────────

// OnlineView содержит отформатированные данные для онлайн-списка.
type OnlineView struct {
	Text      string
	Keyboard  *InlineKeyboard
	ParseMode string
}

// FormatOnlineNow форматирует список студентов онлайн.
func (p *LeaderboardPresenter) FormatOnlineNow(
	entries []query.LeaderboardEntryDTO,
	includeAway bool,
	onlyHelpers bool,
) *OnlineView {
	var sb strings.Builder

	// Заголовок
	sb.WriteString("🟢 <b>Сейчас онлайн</b>")

	if onlyHelpers {
		sb.WriteString(" • 🤝 готовы помочь")
	}

	sb.WriteString("\n\n")

	if len(entries) == 0 {
		if onlyHelpers {
			sb.WriteString("<i>Сейчас нет никого, кто готов помочь.\nПопробуй позже или сними фильтр.</i>\n")
		} else {
			sb.WriteString("<i>Сейчас никого нет онлайн.\nВсе отдыхают 😴</i>\n")
		}
	} else {
		// Группируем по статусу
		online := make([]query.LeaderboardEntryDTO, 0)
		away := make([]query.LeaderboardEntryDTO, 0)

		for _, entry := range entries {
			if entry.IsOnline {
				online = append(online, entry)
			} else if includeAway {
				away = append(away, entry)
			}
		}

		// Онлайн
		if len(online) > 0 {
			sb.WriteString(fmt.Sprintf("🟢 <b>Онлайн (%d)</b>\n", len(online)))
			for _, entry := range online {
				entryCopy := entry
				sb.WriteString(p.formatOnlineEntry(&entryCopy))
				sb.WriteString("\n")
			}
		}

		// Отошедшие
		if len(away) > 0 {
			if len(online) > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("🟡 <b>Недавно были (%d)</b>\n", len(away)))
			for _, entry := range away {
				entryCopy := entry
				sb.WriteString(p.formatOnlineEntry(&entryCopy))
				sb.WriteString("\n")
			}
		}
	}

	// Статистика
	sb.WriteString("\n")
	sb.WriteString("─────────────────────\n")
	sb.WriteString(fmt.Sprintf("👁 Обновлено: %s", time.Now().Format("15:04")))

	keyboard := p.keyboardBuilder.OnlineKeyboard(includeAway, onlyHelpers)

	return &OnlineView{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// formatOnlineEntry форматирует запись онлайн-студента.
func (p *LeaderboardPresenter) formatOnlineEntry(entry *query.LeaderboardEntryDTO) string {
	var sb strings.Builder

	// Ранг (компактно)
	sb.WriteString(fmt.Sprintf("#%d ", entry.Rank))

	// Имя
	sb.WriteString(p.escapeHTML(entry.DisplayName))

	// XP и уровень
	sb.WriteString(fmt.Sprintf(" • Lvl %d", entry.Level))

	// Рейтинг помощника
	if entry.HelpRating > 0 && entry.IsAvailableForHelp {
		sb.WriteString(fmt.Sprintf(" • ⭐%.1f", entry.HelpRating))
	}

	// Last seen для away
	if !entry.IsOnline && entry.LastSeenAt != nil {
		elapsed := time.Since(*entry.LastSeenAt)
		sb.WriteString(fmt.Sprintf(" • %s назад", p.formatDuration(elapsed)))
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// RANK CHANGE NOTIFICATION
// ─────────────────────────────────────────────────────────────────────────────

// RankChangeNotification содержит данные для уведомления об изменении ранга.
type RankChangeNotification struct {
	Text      string
	Keyboard  *InlineKeyboard
	ParseMode string
}

// FormatRankUpNotification форматирует уведомление о повышении в рейтинге.
func (p *LeaderboardPresenter) FormatRankUpNotification(
	oldRank, newRank int,
	overtakenStudent string,
) *RankChangeNotification {
	var sb strings.Builder

	change := oldRank - newRank

	// Эмодзи в зависимости от достижения
	if newRank <= 10 {
		sb.WriteString("🏆🎉 ")
	} else if newRank <= 50 {
		sb.WriteString("⭐🚀 ")
	} else {
		sb.WriteString("📈 ")
	}

	sb.WriteString("<b>Ты поднялся в рейтинге!</b>\n\n")

	// Детали
	sb.WriteString(fmt.Sprintf("Позиция: <b>#%d</b> → <b>#%d</b> (+%d)\n", oldRank, newRank, change))

	if overtakenStudent != "" {
		sb.WriteString(fmt.Sprintf("Обогнал: %s\n", p.escapeHTML(overtakenStudent)))
	}

	// Мотивационное сообщение
	sb.WriteString("\n")
	if newRank <= 10 {
		sb.WriteString("🔥 Ты в топ-10! Элита Alem!")
	} else if newRank <= 50 {
		sb.WriteString("💪 Отличный прогресс! Топ-50 покорён!")
	} else {
		sb.WriteString("👍 Так держать! Продолжай в том же духе!")
	}

	keyboard := NewInlineKeyboard().
		AddRow(
			CallbackButton("📊 Моя позиция", "cmd:me"),
			CallbackButton("🏆 Лидерборд", "cmd:top"),
		)

	return &RankChangeNotification{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// FormatRankDownNotification форматирует уведомление о падении в рейтинге.
func (p *LeaderboardPresenter) FormatRankDownNotification(
	oldRank, newRank int,
	overtakingStudent string,
) *RankChangeNotification {
	var sb strings.Builder

	change := newRank - oldRank

	sb.WriteString("📉 <b>Изменение в рейтинге</b>\n\n")

	sb.WriteString(fmt.Sprintf("Позиция: #%d → #%d (-%d)\n", oldRank, newRank, change))

	if overtakingStudent != "" {
		sb.WriteString(fmt.Sprintf("Тебя обогнал: %s\n", p.escapeHTML(overtakingStudent)))
	}

	sb.WriteString("\n💪 Не сдавайся! Время вернуть позицию!")

	keyboard := NewInlineKeyboard().
		AddRow(
			CallbackButton("📊 Моя позиция", "cmd:me"),
			CallbackButton("👥 Соседи", "cmd:neighbors"),
		)

	return &RankChangeNotification{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MILESTONE NOTIFICATIONS
// ─────────────────────────────────────────────────────────────────────────────

// FormatMilestoneNotification форматирует уведомление о достижении milestone.
func (p *LeaderboardPresenter) FormatMilestoneNotification(
	milestoneType string,
	rank int,
) *RankChangeNotification {
	var sb strings.Builder

	switch milestoneType {
	case "top10":
		sb.WriteString("🏆🎊 <b>ПОЗДРАВЛЯЕМ!</b>\n\n")
		sb.WriteString("Ты вошёл в <b>ТОП-10</b> Alem!\n")
		sb.WriteString(fmt.Sprintf("Твоя позиция: <b>#%d</b>\n\n", rank))
		sb.WriteString("🌟 Ты - элита! Гордимся тобой!")

	case "top50":
		sb.WriteString("⭐ <b>Отличные новости!</b>\n\n")
		sb.WriteString("Ты вошёл в <b>ТОП-50</b>!\n")
		sb.WriteString(fmt.Sprintf("Твоя позиция: <b>#%d</b>\n\n", rank))
		sb.WriteString("💫 Продолжай в том же духе!")

	case "top100":
		sb.WriteString("✨ <b>Молодец!</b>\n\n")
		sb.WriteString("Ты в <b>ТОП-100</b>!\n")
		sb.WriteString(fmt.Sprintf("Твоя позиция: <b>#%d</b>\n\n", rank))
		sb.WriteString("📈 Отличный старт!")

	default:
		sb.WriteString(fmt.Sprintf("🎯 Твоя позиция: <b>#%d</b>", rank))
	}

	keyboard := NewInlineKeyboard().
		AddRow(
			CallbackButton("📊 Моя карточка", "cmd:me"),
			CallbackButton("🏆 Лидерборд", "cmd:top"),
		)

	return &RankChangeNotification{
		Text:      sb.String(),
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// UTILITY FUNCTIONS
// ─────────────────────────────────────────────────────────────────────────────

// formatNumber форматирует число с разделителями тысяч.
func (p *LeaderboardPresenter) formatNumber(n int) string {
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

// formatDuration форматирует длительность в человекочитаемый формат.
func (p *LeaderboardPresenter) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "менее минуты"
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
	return fmt.Sprintf("%d д", days)
}

// escapeHTML экранирует HTML-символы для безопасного отображения.
func (p *LeaderboardPresenter) escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(s)
}

// ─────────────────────────────────────────────────────────────────────────────
// EMPTY STATES
// ─────────────────────────────────────────────────────────────────────────────

// FormatEmptyLeaderboard форматирует пустой лидерборд.
func (p *LeaderboardPresenter) FormatEmptyLeaderboard() *LeaderboardView {
	text := `🏆 <b>Лидерборд Alem</b>

📭 <i>Пока здесь пусто...</i>

Лидерборд заполнится, когда студенты начнут зарабатывать XP!

💡 <b>Что делать?</b>
• Выполняй задачи на платформе
• Помогай другим студентам
• Поддерживай серию активности`

	keyboard := NewInlineKeyboard().
		AddRow(
			CallbackButton("🔄 Обновить", "top:refresh:1::false"),
		)

	return &LeaderboardView{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}

// FormatErrorState форматирует состояние ошибки.
func (p *LeaderboardPresenter) FormatErrorState(err error) *LeaderboardView {
	text := `⚠️ <b>Что-то пошло не так</b>

Не удалось загрузить лидерборд.
Попробуй ещё раз через несколько секунд.

<i>Если проблема повторяется, сообщи администратору.</i>`

	keyboard := NewInlineKeyboard().
		AddRow(
			CallbackButton("🔄 Попробовать снова", "top:refresh:1::false"),
		)

	return &LeaderboardView{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
	}
}
