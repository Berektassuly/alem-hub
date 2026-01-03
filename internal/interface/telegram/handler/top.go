// Package handler contains Telegram command handlers.
package handler

import (
	"context"
	"fmt"

	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram/presenter"
)

// ══════════════════════════════════════════════════════════════════════════════
// TOP HANDLER
// Handles /top command - shows the leaderboard.
// This shows the competitive aspect while enabling "From Competition to Collaboration".
// Philosophy: The leaderboard is not just a ranking, but a "phonebook of helpers".
// ══════════════════════════════════════════════════════════════════════════════

// TopHandler handles the /top command for showing leaderboard.
type TopHandler struct {
	leaderboardQuery *query.GetLeaderboardHandler
	keyboards        *presenter.KeyboardBuilder
}

// NewTopHandler creates a new TopHandler with dependencies.
func NewTopHandler(
	leaderboardQuery *query.GetLeaderboardHandler,
	keyboards *presenter.KeyboardBuilder,
) *TopHandler {
	return &TopHandler{
		leaderboardQuery: leaderboardQuery,
		keyboards:        keyboards,
	}
}

// TopRequest contains the parsed /top command data.
type TopRequest struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// ChatID is the chat ID for sending responses.
	ChatID int64

	// MessageID is the original message ID (for editing).
	MessageID int

	// Cohort is an optional cohort filter.
	Cohort string

	// Limit is the number of entries to show.
	Limit int

	// IsRefresh indicates if this is a refresh request (from callback).
	IsRefresh bool
}

// TopResponse contains the response to send back.
type TopResponse struct {
	// Text is the message text (HTML formatted).
	Text string

	// Keyboard is the inline keyboard to attach.
	Keyboard *presenter.InlineKeyboard

	// ParseMode is the parse mode (HTML).
	ParseMode string

	// IsError indicates if this is an error response.
	IsError bool
}

// Handle processes the /top command.
func (h *TopHandler) Handle(ctx context.Context, req TopRequest) (*TopResponse, error) {
	// Set default limit
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	// Get leaderboard
	leaderboardQuery := query.GetLeaderboardQuery{
		Cohort: req.Cohort,
		Limit:  limit,
		Offset: 0,
	}

	result, err := h.leaderboardQuery.Handle(ctx, leaderboardQuery)
	if err != nil {
		return &TopResponse{
			Text:      "❌ Не удалось загрузить рейтинг. Попробуйте позже.",
			ParseMode: "HTML",
			IsError:   true,
		}, nil
	}

	// Build response text
	text := h.formatLeaderboard(result, req.Cohort)

	return &TopResponse{
		Text:      text,
		Keyboard:  h.keyboards.LeaderboardKeyboard(0, result.HasMore, req.Cohort, false),
		ParseMode: "HTML",
	}, nil
}

// formatLeaderboard formats the leaderboard for display.
func (h *TopHandler) formatLeaderboard(result *query.GetLeaderboardResult, cohort string) string {
	var text string

	// Header
	if cohort != "" {
		text = fmt.Sprintf("🏆 <b>Рейтинг - %s</b>\n\n", cohort)
	} else {
		text = "🏆 <b>Общий рейтинг</b>\n\n"
	}

	// Entries
	for _, entry := range result.Entries {
		// Position emoji
		posEmoji := h.getPositionEmoji(entry.Rank)

		// Online indicator
		onlineIndicator := ""
		if entry.IsOnline {
			onlineIndicator = " 🟢"
		}

		text += fmt.Sprintf("%s <b>%s</b>%s\n", posEmoji, entry.DisplayName, onlineIndicator)
		text += fmt.Sprintf("   ⚡ %d XP • 🎮 Ур. %d\n", entry.XP, entry.Level)
	}

	// Footer with total count
	if result.TotalCount > len(result.Entries) {
		text += fmt.Sprintf("\n<i>Показано %d из %d студентов</i>", len(result.Entries), result.TotalCount)
	}

	return text
}

// getPositionEmoji returns an emoji for the position.
func (h *TopHandler) getPositionEmoji(position int) string {
	switch position {
	case 1:
		return "🥇"
	case 2:
		return "🥈"
	case 3:
		return "🥉"
	default:
		return fmt.Sprintf("%d.", position)
	}
}
