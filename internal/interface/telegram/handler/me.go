// Package handler contains Telegram command handlers.
package handler

import (
	"alem-hub/internal/application/query"
	"alem-hub/internal/domain/student"
	"alem-hub/internal/interface/telegram/presenter"
	"context"
	"fmt"
	"strings"
)

// ══════════════════════════════════════════════════════════════════════════════
// ME HANDLER
// Handles /me command - shows the student's personal card with stats.
// This is the "mirror" - where students see their progress and achievements.
// Philosophy: Make progress visible and celebrate every step forward.
// ══════════════════════════════════════════════════════════════════════════════

// MeHandler handles the /me command for showing student card.
type MeHandler struct {
	studentRankQuery *query.GetStudentRankHandler
	dailyProgress    *query.GetDailyProgressHandler
	studentRepo      student.Repository
	keyboards        *presenter.KeyboardBuilder
	cardPresenter    *presenter.StudentCardPresenter
}

// NewMeHandler creates a new MeHandler with dependencies.
func NewMeHandler(
	studentRankQuery *query.GetStudentRankHandler,
	dailyProgress *query.GetDailyProgressHandler,
	studentRepo student.Repository,
	keyboards *presenter.KeyboardBuilder,
	cardPresenter *presenter.StudentCardPresenter,
) *MeHandler {
	return &MeHandler{
		studentRankQuery: studentRankQuery,
		dailyProgress:    dailyProgress,
		studentRepo:      studentRepo,
		keyboards:        keyboards,
		cardPresenter:    cardPresenter,
	}
}

// MeRequest contains the parsed /me command data.
type MeRequest struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// ChatID is the chat ID for sending responses.
	ChatID int64

	// MessageID is the original message ID (for editing).
	MessageID int

	// IsRefresh indicates if this is a refresh request (from callback).
	IsRefresh bool
}

// MeResponse contains the response to send back.
type MeResponse struct {
	// Text is the message text (HTML formatted).
	Text string

	// Keyboard is the inline keyboard to attach.
	Keyboard *presenter.InlineKeyboard

	// ParseMode is the parse mode (HTML).
	ParseMode string

	// IsError indicates if this is an error response.
	IsError bool
}

// Handle processes the /me command.
func (h *MeHandler) Handle(ctx context.Context, req MeRequest) (*MeResponse, error) {
	// Get student by Telegram ID
	stud, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(req.TelegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	// Get rank information
	rankQuery := query.GetStudentRankQuery{
		TelegramID:     req.TelegramID,
		IncludeHistory: false,
	}

	rankResult, err := h.studentRankQuery.Handle(ctx, rankQuery)
	if err != nil {
		// Continue without rank info
		rankResult = nil
	}

	// Get daily progress (optional)
	var dailyResult *query.GetDailyProgressResult
	if h.dailyProgress != nil {
		progressQuery := query.GetDailyProgressQuery{
			TelegramID: req.TelegramID,
		}
		dailyResult, _ = h.dailyProgress.Handle(ctx, progressQuery)
	}

	// Build the student card
	text := h.buildStudentCard(stud, rankResult, dailyResult)
	keyboard := h.keyboards.StudentCardKeyboard(stud.ID)

	return &MeResponse{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
		IsError:   false,
	}, nil
}

// handleNotRegistered handles the case when user is not registered.
func (h *MeHandler) handleNotRegistered() (*MeResponse, error) {
	text := "❌ <b>Ты ещё не зарегистрирован</b>\n\n" +
		"Используй /start чтобы присоединиться к сообществу."

	return &MeResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// buildStudentCard builds the student card text.
func (h *MeHandler) buildStudentCard(
	stud *student.Student,
	rankResult *query.GetStudentRankResult,
	dailyResult *query.GetDailyProgressResult,
) string {
	var sb strings.Builder

	// Header with name and status
	statusEmoji := getOnlineStatusEmoji(stud.OnlineState)
	sb.WriteString(fmt.Sprintf("👤 <b>%s</b> %s\n", escapeHTML(stud.DisplayName), statusEmoji))
	sb.WriteString(fmt.Sprintf("└ @%s\n\n", escapeHTML(string(stud.AlemLogin))))

	// XP and Level section
	sb.WriteString("📊 <b>Прогресс</b>\n")
	sb.WriteString(fmt.Sprintf("├ XP: <code>%d</code>\n", stud.CurrentXP))
	sb.WriteString(fmt.Sprintf("├ Уровень: <b>%d</b>\n", stud.Level()))

	// Level progress bar
	if rankResult != nil && rankResult.Student.XPToNextLevel > 0 {
		progressBar := formatProgressBar(rankResult.Student.LevelProgress)
		sb.WriteString(fmt.Sprintf("├ До уровня %d: %s %d XP\n", stud.Level()+1, progressBar, rankResult.Student.XPToNextLevel))
	}

	// Rank information
	if rankResult != nil {
		sb.WriteString(fmt.Sprintf("└ Позиция: <b>#%d</b>", rankResult.Student.Rank))

		// Rank change indicator
		if rankResult.Student.RankChange != 0 {
			changeEmoji := "📈"
			changeSign := "+"
			if rankResult.Student.RankChange < 0 {
				changeEmoji = "📉"
				changeSign = ""
			}
			sb.WriteString(fmt.Sprintf(" %s %s%d", changeEmoji, changeSign, rankResult.Student.RankChange))
		}

		// Percentile
		if rankResult.Student.TotalStudents > 0 {
			percentile := query.FormatPercentile(rankResult.Student.Percentile)
			sb.WriteString(fmt.Sprintf(" (%s)", percentile))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Daily Grind section (if available)
	if dailyResult != nil && dailyResult.Progress != nil {
		sb.WriteString("🔥 <b>Сегодня</b>\n")
		sb.WriteString(fmt.Sprintf("├ XP: +%d\n", dailyResult.Progress.XPGained))
		sb.WriteString(fmt.Sprintf("├ Задач: %d\n", dailyResult.Progress.TasksCompleted))

		if dailyResult.Streak != nil && dailyResult.Streak.CurrentStreak > 0 {
			streakEmoji := "🔥"
			if dailyResult.Streak.CurrentStreak >= 7 {
				streakEmoji = "🔥🔥"
			}
			if dailyResult.Streak.CurrentStreak >= 30 {
				streakEmoji = "🔥🔥🔥"
			}
			sb.WriteString(fmt.Sprintf("└ Серия: %d дней %s\n", dailyResult.Streak.CurrentStreak, streakEmoji))
		}
		sb.WriteString("\n")
	}

	// Neighbor info (who to catch up with)
	if rankResult != nil && rankResult.Student.XPToNextRank > 0 && rankResult.Student.NextRankStudent != "" {
		sb.WriteString("🎯 <b>Цель</b>\n")
		sb.WriteString(fmt.Sprintf("└ До @%s: %d XP\n\n",
			escapeHTML(rankResult.Student.NextRankStudent),
			rankResult.Student.XPToNextRank))
	}

	// Helper rating (if they've helped others)
	if stud.HelpCount > 0 {
		sb.WriteString("🤝 <b>Помощник</b>\n")
		sb.WriteString(fmt.Sprintf("├ Рейтинг: %s (%.1f)\n", formatStarRating(stud.HelpRating), stud.HelpRating))
		sb.WriteString(fmt.Sprintf("└ Помощей: %d\n\n", stud.HelpCount))
	}

	// Motivational message
	if rankResult != nil && rankResult.Message != "" {
		sb.WriteString(fmt.Sprintf("<i>%s</i>\n", rankResult.Message))
	}

	// Cohort
	sb.WriteString(fmt.Sprintf("\n🏫 Когорта: %s", string(stud.Cohort)))

	return sb.String()
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// getOnlineStatusEmoji returns emoji for online status.
func getOnlineStatusEmoji(state student.OnlineState) string {
	switch state {
	case student.OnlineStateOnline:
		return "🟢"
	case student.OnlineStateAway:
		return "🟡"
	default:
		return "⚪"
	}
}

// formatProgressBar formats a progress bar.
func formatProgressBar(progress float64) string {
	const barLength = 10
	filled := int(progress * barLength)
	if filled > barLength {
		filled = barLength
	}
	if filled < 0 {
		filled = 0
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barLength-filled)
	return fmt.Sprintf("[%s]", bar)
}

// formatStarRating formats rating as stars.
func formatStarRating(rating float64) string {
	if rating == 0 {
		return "☆☆☆☆☆"
	}

	fullStars := int(rating)
	hasHalf := (rating - float64(fullStars)) >= 0.5

	result := strings.Repeat("⭐", fullStars)
	if hasHalf && fullStars < 5 {
		result += "✨"
	}

	return result
}
