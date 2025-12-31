// Package handler contains Telegram command handlers.
package handler

import (
	"github.com/alem-hub/alem-community-hub/internal/application/command"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram/presenter"
	"context"
	"fmt"
	"strings"
)

// ══════════════════════════════════════════════════════════════════════════════
// SETTINGS HANDLER
// Handles /settings command - manages user preferences.
// Philosophy: Give students control over their experience. Respect their time
// and attention by allowing them to customize notifications.
// ══════════════════════════════════════════════════════════════════════════════

// SettingsHandler handles the /settings command.
type SettingsHandler struct {
	updatePrefsCmd *command.UpdatePreferencesHandler
	resetPrefsCmd  *command.ResetPreferencesHandler
	studentRepo    student.Repository
	keyboards      *presenter.KeyboardBuilder
}

// NewSettingsHandler creates a new SettingsHandler with dependencies.
func NewSettingsHandler(
	updatePrefsCmd *command.UpdatePreferencesHandler,
	resetPrefsCmd *command.ResetPreferencesHandler,
	studentRepo student.Repository,
	keyboards *presenter.KeyboardBuilder,
) *SettingsHandler {
	return &SettingsHandler{
		updatePrefsCmd: updatePrefsCmd,
		resetPrefsCmd:  resetPrefsCmd,
		studentRepo:    studentRepo,
		keyboards:      keyboards,
	}
}

// SettingsRequest contains the parsed /settings command data.
type SettingsRequest struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// ChatID is the chat ID for sending responses.
	ChatID int64

	// MessageID is the original message ID (for editing).
	MessageID int

	// IsRefresh indicates if this is a refresh request.
	IsRefresh bool
}

// SettingsResponse contains the response to send back.
type SettingsResponse struct {
	// Text is the message text (HTML formatted).
	Text string

	// Keyboard is the inline keyboard to attach.
	Keyboard *presenter.InlineKeyboard

	// ParseMode is the parse mode (HTML).
	ParseMode string

	// IsError indicates if this is an error response.
	IsError bool
}

// Handle processes the /settings command.
func (h *SettingsHandler) Handle(ctx context.Context, req SettingsRequest) (*SettingsResponse, error) {
	// Get current student
	stud, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(req.TelegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	// Build settings view
	text := h.buildSettingsView(stud)
	keyboard := h.keyboards.SettingsKeyboard(stud)

	return &SettingsResponse{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
		IsError:   false,
	}, nil
}

// handleNotRegistered handles the case when user is not registered.
func (h *SettingsHandler) handleNotRegistered() (*SettingsResponse, error) {
	text := "❌ <b>Ты ещё не зарегистрирован</b>\n\n" +
		"Используй /start чтобы присоединиться к сообществу."

	return &SettingsResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// buildSettingsView builds the settings view text.
func (h *SettingsHandler) buildSettingsView(stud *student.Student) string {
	var sb strings.Builder

	// Header
	sb.WriteString("⚙️ <b>Настройки</b>\n\n")

	// Profile info
	sb.WriteString(fmt.Sprintf("👤 <b>%s</b> (@%s)\n\n",
		escapeHTML(stud.DisplayName),
		escapeHTML(string(stud.AlemLogin))))

	// Notifications section
	sb.WriteString("🔔 <b>Уведомления</b>\n")

	sb.WriteString(h.formatSettingLine("Изменения рейтинга", stud.Preferences.RankChanges))
	sb.WriteString(h.formatSettingLine("Ежедневная сводка", stud.Preferences.DailyDigest))
	sb.WriteString(h.formatSettingLine("Запросы помощи", stud.Preferences.HelpRequests))
	sb.WriteString(h.formatSettingLine("Напоминания", stud.Preferences.InactivityReminders))

	sb.WriteString("\n")

	// Quiet hours
	sb.WriteString("🌙 <b>Тихие часы</b>\n")
	sb.WriteString(fmt.Sprintf("   %02d:00 — %02d:00\n\n",
		stud.Preferences.QuietHoursStart,
		stud.Preferences.QuietHoursEnd))

	// Helper status
	sb.WriteString("🤝 <b>Помощь другим</b>\n")
	helperStatus := "Готов помогать"
	helperEmoji := "✅"
	if !stud.Preferences.HelpRequests {
		helperStatus = "Не беспокоить"
		helperEmoji = "⛔"
	}
	sb.WriteString(fmt.Sprintf("   %s %s\n\n", helperEmoji, helperStatus))

	// Stats
	if stud.HelpCount > 0 {
		sb.WriteString("📊 <b>Статистика помощи</b>\n")
		sb.WriteString(fmt.Sprintf("   Помог %d раз • Рейтинг: %.1f ⭐\n\n", stud.HelpCount, stud.HelpRating))
	}

	// Footer
	sb.WriteString("<i>Используй кнопки ниже для изменения настроек.</i>")

	return sb.String()
}

// formatSettingLine formats a single setting line.
func (h *SettingsHandler) formatSettingLine(name string, enabled bool) string {
	status := "✅"
	if !enabled {
		status = "❌"
	}
	return fmt.Sprintf("   %s %s\n", status, name)
}

// ToggleSetting handles toggling a specific setting.
func (h *SettingsHandler) ToggleSetting(ctx context.Context, telegramID int64, setting string) (*SettingsResponse, error) {
	// Get current student
	stud, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(telegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	// Build update command based on setting
	var updates command.PreferenceUpdates

	switch setting {
	case "rank_changes":
		newValue := !stud.Preferences.RankChanges
		updates.RankChanges = &newValue
	case "daily_digest":
		newValue := !stud.Preferences.DailyDigest
		updates.DailyDigest = &newValue
	case "help_requests":
		newValue := !stud.Preferences.HelpRequests
		updates.HelpRequests = &newValue
	case "inactivity_reminders":
		newValue := !stud.Preferences.InactivityReminders
		updates.InactivityReminders = &newValue
	default:
		return &SettingsResponse{
			Text:      "❌ Неизвестная настройка",
			ParseMode: "HTML",
			IsError:   true,
		}, nil
	}

	// Execute update
	cmd := command.UpdatePreferencesCommand{
		StudentID:   stud.ID,
		Preferences: updates,
	}

	_, err = h.updatePrefsCmd.Handle(ctx, cmd)
	if err != nil {
		return &SettingsResponse{
			Text:      "❌ Не удалось обновить настройки",
			ParseMode: "HTML",
			IsError:   true,
		}, nil
	}

	// Refresh settings view
	return h.Handle(ctx, SettingsRequest{TelegramID: telegramID})
}

// SetQuietHours handles setting quiet hours.
func (h *SettingsHandler) SetQuietHours(ctx context.Context, telegramID int64, startHour, endHour int) (*SettingsResponse, error) {
	// Validate hours
	if startHour < 0 || startHour > 23 || endHour < 0 || endHour > 23 {
		return &SettingsResponse{
			Text:      "❌ Некорректное время. Укажи часы от 0 до 23.",
			ParseMode: "HTML",
			IsError:   true,
		}, nil
	}

	// Get current student
	stud, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(telegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	// Execute update
	cmd := command.UpdatePreferencesCommand{
		StudentID: stud.ID,
		Preferences: command.PreferenceUpdates{
			QuietHoursStart: &startHour,
			QuietHoursEnd:   &endHour,
		},
	}

	_, err = h.updatePrefsCmd.Handle(ctx, cmd)
	if err != nil {
		return &SettingsResponse{
			Text:      "❌ Не удалось обновить настройки",
			ParseMode: "HTML",
			IsError:   true,
		}, nil
	}

	// Refresh settings view
	return h.Handle(ctx, SettingsRequest{TelegramID: telegramID})
}

// ResetSettings handles resetting all settings to defaults.
func (h *SettingsHandler) ResetSettings(ctx context.Context, telegramID int64) (*SettingsResponse, error) {
	// Get current student
	stud, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(telegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	// Execute reset
	cmd := command.ResetPreferencesCommand{
		StudentID: stud.ID,
	}

	_, err = h.resetPrefsCmd.Handle(ctx, cmd)
	if err != nil {
		return &SettingsResponse{
			Text:      "❌ Не удалось сбросить настройки",
			ParseMode: "HTML",
			IsError:   true,
		}, nil
	}

	// Success message + refresh
	result, _ := h.Handle(ctx, SettingsRequest{TelegramID: telegramID})
	result.Text = "✅ Настройки сброшены!\n\n" + result.Text

	return result, nil
}

// EnableAllNotifications enables all notifications.
func (h *SettingsHandler) EnableAllNotifications(ctx context.Context, telegramID int64) (*SettingsResponse, error) {
	stud, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(telegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	t := true
	cmd := command.UpdatePreferencesCommand{
		StudentID: stud.ID,
		Preferences: command.PreferenceUpdates{
			RankChanges:         &t,
			DailyDigest:         &t,
			HelpRequests:        &t,
			InactivityReminders: &t,
		},
	}

	_, err = h.updatePrefsCmd.Handle(ctx, cmd)
	if err != nil {
		return &SettingsResponse{
			Text:      "❌ Не удалось обновить настройки",
			ParseMode: "HTML",
			IsError:   true,
		}, nil
	}

	result, _ := h.Handle(ctx, SettingsRequest{TelegramID: telegramID})
	result.Text = "✅ Все уведомления включены!\n\n" + result.Text

	return result, nil
}

// DisableAllNotifications disables all notifications.
func (h *SettingsHandler) DisableAllNotifications(ctx context.Context, telegramID int64) (*SettingsResponse, error) {
	stud, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(telegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	f := false
	cmd := command.UpdatePreferencesCommand{
		StudentID: stud.ID,
		Preferences: command.PreferenceUpdates{
			RankChanges:         &f,
			DailyDigest:         &f,
			HelpRequests:        &f,
			InactivityReminders: &f,
		},
	}

	_, err = h.updatePrefsCmd.Handle(ctx, cmd)
	if err != nil {
		return &SettingsResponse{
			Text:      "❌ Не удалось обновить настройки",
			ParseMode: "HTML",
			IsError:   true,
		}, nil
	}

	result, _ := h.Handle(ctx, SettingsRequest{TelegramID: telegramID})
	result.Text = "🔕 Все уведомления отключены!\n\n" + result.Text

	return result, nil
}
