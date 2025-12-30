// Package presenter formats data for Telegram display.
// Presenters handle the conversion from domain objects to user-friendly
// Telegram messages, keyboards, and other UI elements.
package presenter

import (
	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"fmt"
)

// ══════════════════════════════════════════════════════════════════════════════
// INLINE KEYBOARD TYPES
// These types represent Telegram inline keyboards in a library-agnostic way.
// The actual Telegram bot implementation will convert these to the library's format.
// ══════════════════════════════════════════════════════════════════════════════

// InlineKeyboard represents an inline keyboard.
type InlineKeyboard struct {
	Rows [][]InlineButton
}

// InlineButton represents a single inline button.
type InlineButton struct {
	// Text is the button text.
	Text string

	// CallbackData is the callback data (for callback buttons).
	CallbackData string

	// URL is the URL to open (for URL buttons).
	URL string

	// SwitchInlineQuery starts inline mode (for inline buttons).
	SwitchInlineQuery string
}

// NewInlineKeyboard creates a new empty inline keyboard.
func NewInlineKeyboard() *InlineKeyboard {
	return &InlineKeyboard{
		Rows: make([][]InlineButton, 0),
	}
}

// AddRow adds a row of buttons.
func (k *InlineKeyboard) AddRow(buttons ...InlineButton) *InlineKeyboard {
	k.Rows = append(k.Rows, buttons)
	return k
}

// CallbackButton creates a callback button.
func CallbackButton(text, callbackData string) InlineButton {
	return InlineButton{
		Text:         text,
		CallbackData: callbackData,
	}
}

// URLButton creates a URL button.
func URLButton(text, url string) InlineButton {
	return InlineButton{
		Text: text,
		URL:  url,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// KEYBOARD BUILDER
// Builds keyboards for different use cases.
// ══════════════════════════════════════════════════════════════════════════════

// KeyboardBuilder builds inline keyboards for various handlers.
type KeyboardBuilder struct{}

// NewKeyboardBuilder creates a new KeyboardBuilder.
func NewKeyboardBuilder() *KeyboardBuilder {
	return &KeyboardBuilder{}
}

// ─────────────────────────────────────────────────────────────────────────────
// START / ONBOARDING KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// WelcomeBackKeyboard creates keyboard for returning users.
func (b *KeyboardBuilder) WelcomeBackKeyboard() *InlineKeyboard {
	return NewInlineKeyboard().
		AddRow(
			CallbackButton("📊 Моя карточка", "cmd:me"),
			CallbackButton("🏆 Лидерборд", "cmd:top"),
		).
		AddRow(
			CallbackButton("👥 Соседи", "cmd:neighbors"),
			CallbackButton("🟢 Онлайн", "cmd:online"),
		).
		AddRow(
			CallbackButton("⚙️ Настройки", "cmd:settings"),
		)
}

// OnboardingSuccessKeyboard creates keyboard after successful onboarding.
func (b *KeyboardBuilder) OnboardingSuccessKeyboard() *InlineKeyboard {
	return NewInlineKeyboard().
		AddRow(
			CallbackButton("📊 Моя карточка", "cmd:me"),
			CallbackButton("🏆 Лидерборд", "cmd:top"),
		).
		AddRow(
			CallbackButton("👥 Найти соседей", "cmd:neighbors"),
		)
}

// ─────────────────────────────────────────────────────────────────────────────
// STUDENT CARD KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// StudentCardKeyboard creates keyboard for student card (/me).
func (b *KeyboardBuilder) StudentCardKeyboard(studentID string) *InlineKeyboard {
	return NewInlineKeyboard().
		AddRow(
			CallbackButton("🔄 Обновить", "refresh:me"),
			CallbackButton("👥 Соседи", "cmd:neighbors"),
		).
		AddRow(
			CallbackButton("🏆 Лидерборд", "cmd:top"),
			CallbackButton("⚙️ Настройки", "cmd:settings"),
		)
}

// ─────────────────────────────────────────────────────────────────────────────
// LEADERBOARD KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// LeaderboardKeyboard creates keyboard for leaderboard (/top).
func (b *KeyboardBuilder) LeaderboardKeyboard(page int, hasMore bool, cohort string, onlyOnline bool) *InlineKeyboard {
	kb := NewInlineKeyboard()

	// Navigation row
	navRow := make([]InlineButton, 0, 3)

	if page > 1 {
		navRow = append(navRow, CallbackButton("◀️ Назад", fmt.Sprintf("top:page:%d:%s:%t", page-1, cohort, onlyOnline)))
	}

	navRow = append(navRow, CallbackButton("🔄", fmt.Sprintf("top:refresh:%d:%s:%t", page, cohort, onlyOnline)))

	if hasMore {
		navRow = append(navRow, CallbackButton("Вперёд ▶️", fmt.Sprintf("top:page:%d:%s:%t", page+1, cohort, onlyOnline)))
	}

	if len(navRow) > 0 {
		kb.AddRow(navRow...)
	}

	// Filter row
	onlineText := "🟢 Только онлайн"
	if onlyOnline {
		onlineText = "👥 Показать всех"
	}
	kb.AddRow(CallbackButton(onlineText, fmt.Sprintf("top:filter:%d:%s:%t", page, cohort, !onlyOnline)))

	// Actions row
	kb.AddRow(
		CallbackButton("📊 Моя позиция", "cmd:me"),
		CallbackButton("👥 Мои соседи", "cmd:neighbors"),
	)

	return kb
}

// ─────────────────────────────────────────────────────────────────────────────
// NEIGHBORS KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// NeighborsKeyboard creates keyboard for neighbors view (/neighbors).
func (b *KeyboardBuilder) NeighborsKeyboard() *InlineKeyboard {
	return NewInlineKeyboard().
		AddRow(
			CallbackButton("🔄 Обновить", "refresh:neighbors"),
		).
		AddRow(
			CallbackButton("🏆 Лидерборд", "cmd:top"),
			CallbackButton("📊 Моя карточка", "cmd:me"),
		)
}

// ─────────────────────────────────────────────────────────────────────────────
// ONLINE KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// OnlineKeyboard creates keyboard for online view (/online).
func (b *KeyboardBuilder) OnlineKeyboard(includeAway, onlyHelpers bool) *InlineKeyboard {
	kb := NewInlineKeyboard()

	// Filter row
	filterRow := make([]InlineButton, 0, 2)

	awayText := "🟡 + Отошедшие"
	if includeAway {
		awayText = "🟢 Только онлайн"
	}
	filterRow = append(filterRow, CallbackButton(awayText, fmt.Sprintf("online:away:%t:%t", !includeAway, onlyHelpers)))

	helpersText := "🤝 Помощники"
	if onlyHelpers {
		helpersText = "👥 Все"
	}
	filterRow = append(filterRow, CallbackButton(helpersText, fmt.Sprintf("online:helpers:%t:%t", includeAway, !onlyHelpers)))

	kb.AddRow(filterRow...)

	// Actions row
	kb.AddRow(
		CallbackButton("🔄 Обновить", fmt.Sprintf("online:refresh:%t:%t", includeAway, onlyHelpers)),
	)

	return kb
}

// ─────────────────────────────────────────────────────────────────────────────
// HELP / HELPERS KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// HelpersKeyboard creates keyboard for helpers list (/help).
func (b *KeyboardBuilder) HelpersKeyboard(helpers []query.HelperDTO, taskID string) *InlineKeyboard {
	kb := NewInlineKeyboard()

	// Add a button for each helper (up to 5)
	for i, helper := range helpers {
		if i >= 5 {
			break
		}

		// Status indicator
		statusEmoji := "⚪"
		if helper.IsOnline {
			statusEmoji = "🟢"
		} else if helper.OnlineStatus == "away" {
			statusEmoji = "🟡"
		}

		buttonText := fmt.Sprintf("%s Написать %s", statusEmoji, helper.DisplayName)

		kb.AddRow(
			CallbackButton(buttonText, fmt.Sprintf("connect:%s:help_request:%s", helper.StudentID, taskID)),
		)
	}

	// Add refresh button
	kb.AddRow(
		CallbackButton("🔄 Обновить", fmt.Sprintf("help:refresh:%s", taskID)),
	)

	return kb
}

// ─────────────────────────────────────────────────────────────────────────────
// SETTINGS KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// SettingsKeyboard creates keyboard for settings (/settings).
func (b *KeyboardBuilder) SettingsKeyboard(stud *student.Student) *InlineKeyboard {
	kb := NewInlineKeyboard()

	// Notification toggles
	rankIcon := "✅"
	if !stud.Preferences.RankChanges {
		rankIcon = "❌"
	}
	kb.AddRow(CallbackButton(fmt.Sprintf("%s Изменения рейтинга", rankIcon), "settings:toggle:rank_changes"))

	dailyIcon := "✅"
	if !stud.Preferences.DailyDigest {
		dailyIcon = "❌"
	}
	kb.AddRow(CallbackButton(fmt.Sprintf("%s Ежедневная сводка", dailyIcon), "settings:toggle:daily_digest"))

	helpIcon := "✅"
	if !stud.Preferences.HelpRequests {
		helpIcon = "❌"
	}
	kb.AddRow(CallbackButton(fmt.Sprintf("%s Запросы помощи", helpIcon), "settings:toggle:help_requests"))

	inactivityIcon := "✅"
	if !stud.Preferences.InactivityReminders {
		inactivityIcon = "❌"
	}
	kb.AddRow(CallbackButton(fmt.Sprintf("%s Напоминания", inactivityIcon), "settings:toggle:inactivity_reminders"))

	// Quiet hours
	kb.AddRow(CallbackButton(fmt.Sprintf("🌙 Тихие часы: %02d:00-%02d:00",
		stud.Preferences.QuietHoursStart,
		stud.Preferences.QuietHoursEnd), "settings:quiet_hours"))

	// Bulk actions
	kb.AddRow(
		CallbackButton("🔔 Вкл. все", "settings:enable_all"),
		CallbackButton("🔕 Выкл. все", "settings:disable_all"),
	)

	// Reset
	kb.AddRow(CallbackButton("🔄 Сбросить настройки", "settings:reset"))

	return kb
}

// QuietHoursKeyboard creates keyboard for selecting quiet hours.
func (b *KeyboardBuilder) QuietHoursKeyboard() *InlineKeyboard {
	kb := NewInlineKeyboard()

	// Common presets
	kb.AddRow(
		CallbackButton("22:00 - 08:00", "settings:quiet:22:8"),
		CallbackButton("23:00 - 09:00", "settings:quiet:23:9"),
	)
	kb.AddRow(
		CallbackButton("00:00 - 08:00", "settings:quiet:0:8"),
		CallbackButton("01:00 - 10:00", "settings:quiet:1:10"),
	)
	kb.AddRow(
		CallbackButton("🚫 Отключить тихие часы", "settings:quiet:0:0"),
	)
	kb.AddRow(
		CallbackButton("◀️ Назад", "cmd:settings"),
	)

	return kb
}

// ─────────────────────────────────────────────────────────────────────────────
// RATING KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// RatingKeyboard creates keyboard for rating selection.
func (b *KeyboardBuilder) RatingKeyboard(helperID, taskID string) *InlineKeyboard {
	kb := NewInlineKeyboard()

	// Rating buttons
	kb.AddRow(
		CallbackButton("⭐", fmt.Sprintf("endorse:%s:1:%s", helperID, taskID)),
		CallbackButton("⭐⭐", fmt.Sprintf("endorse:%s:2:%s", helperID, taskID)),
		CallbackButton("⭐⭐⭐", fmt.Sprintf("endorse:%s:3:%s", helperID, taskID)),
	)
	kb.AddRow(
		CallbackButton("⭐⭐⭐⭐", fmt.Sprintf("endorse:%s:4:%s", helperID, taskID)),
		CallbackButton("⭐⭐⭐⭐⭐", fmt.Sprintf("endorse:%s:5:%s", helperID, taskID)),
	)

	// Cancel
	kb.AddRow(
		CallbackButton("❌ Отмена", "endorse:cancel"),
	)

	return kb
}

// ─────────────────────────────────────────────────────────────────────────────
// PROFILE / CONNECT KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// ProfileKeyboard creates keyboard for viewing another student's profile.
func (b *KeyboardBuilder) ProfileKeyboard(studentID string, taskID string) *InlineKeyboard {
	kb := NewInlineKeyboard()

	// Contact button
	kb.AddRow(
		CallbackButton("📨 Написать", fmt.Sprintf("connect:%s:profile:%s", studentID, taskID)),
	)

	// Thanks button (if from help context)
	if taskID != "" {
		kb.AddRow(
			CallbackButton("🙏 Сказать спасибо", fmt.Sprintf("endorse:%s:0:%s", studentID, taskID)),
		)
	}

	return kb
}

// ─────────────────────────────────────────────────────────────────────────────
// CONFIRMATION KEYBOARDS
// ─────────────────────────────────────────────────────────────────────────────

// ConfirmationKeyboard creates a yes/no confirmation keyboard.
func (b *KeyboardBuilder) ConfirmationKeyboard(yesCallback, noCallback string) *InlineKeyboard {
	return NewInlineKeyboard().
		AddRow(
			CallbackButton("✅ Да", yesCallback),
			CallbackButton("❌ Нет", noCallback),
		)
}
