// Package handler contains Telegram command handlers.
package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram/presenter"
)

// ══════════════════════════════════════════════════════════════════════════════
// NEIGHBORS HANDLER
// Handles /neighbors command - shows students around your rank (±N positions).
// This is the "motivation feature" - showing who you can catch up to and
// who is catching up to you.
// Philosophy: You're not alone in this race. These are your peers, not competitors.
// ══════════════════════════════════════════════════════════════════════════════

// NeighborsHandler handles the /neighbors command.
type NeighborsHandler struct {
	neighborsQuery *query.GetNeighborsHandler
	studentRepo    student.Repository
	keyboards      *presenter.KeyboardBuilder
}

// NewNeighborsHandler creates a new NeighborsHandler with dependencies.
func NewNeighborsHandler(
	neighborsQuery *query.GetNeighborsHandler,
	studentRepo student.Repository,
	keyboards *presenter.KeyboardBuilder,
) *NeighborsHandler {
	return &NeighborsHandler{
		neighborsQuery: neighborsQuery,
		studentRepo:    studentRepo,
		keyboards:      keyboards,
	}
}

// NeighborsRequest contains the parsed /neighbors command data.
type NeighborsRequest struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// ChatID is the chat ID for sending responses.
	ChatID int64

	// MessageID is the original message ID (for editing).
	MessageID int

	// RangeSize is how many neighbors to show on each side (default 5).
	RangeSize int

	// Cohort is the cohort filter (empty = all).
	Cohort string

	// IsRefresh indicates if this is a refresh request.
	IsRefresh bool
}

// NeighborsResponse contains the response to send back.
type NeighborsResponse struct {
	// Text is the message text (HTML formatted).
	Text string

	// Keyboard is the inline keyboard to attach.
	Keyboard *presenter.InlineKeyboard

	// ParseMode is the parse mode (HTML).
	ParseMode string

	// IsError indicates if this is an error response.
	IsError bool
}

// Handle processes the /neighbors command.
func (h *NeighborsHandler) Handle(ctx context.Context, req NeighborsRequest) (*NeighborsResponse, error) {
	// Verify user is registered
	_, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(req.TelegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	// Set defaults
	if req.RangeSize <= 0 {
		req.RangeSize = 5
	}

	// Build query
	neighborsQuery := query.GetNeighborsQuery{
		TelegramID:          req.TelegramID,
		RangeSize:           req.RangeSize,
		Cohort:              req.Cohort,
		IncludeOnlineStatus: true,
		IncludeXPGap:        true,
	}

	// Execute query
	result, err := h.neighborsQuery.Handle(ctx, neighborsQuery)
	if err != nil {
		return h.handleError(err)
	}

	// Build response
	text := h.buildNeighborsView(result)
	keyboard := h.keyboards.NeighborsKeyboard()

	return &NeighborsResponse{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
		IsError:   false,
	}, nil
}

// handleNotRegistered handles the case when user is not registered.
func (h *NeighborsHandler) handleNotRegistered() (*NeighborsResponse, error) {
	text := "❌ <b>Ты ещё не зарегистрирован</b>\n\n" +
		"Используй /start чтобы присоединиться к сообществу."

	return &NeighborsResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// handleError handles query errors.
func (h *NeighborsHandler) handleError(err error) (*NeighborsResponse, error) {
	text := "❌ <b>Ошибка загрузки</b>\n\n" +
		"Не удалось получить информацию о соседях.\n" +
		"Попробуй позже."

	return &NeighborsResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// buildNeighborsView builds the neighbors view text.
func (h *NeighborsHandler) buildNeighborsView(result *query.GetNeighborsResult) string {
	var sb strings.Builder

	// Header
	sb.WriteString("👥 <b>Твои соседи по рангу</b>\n\n")

	// Show neighbors
	for _, neighbor := range result.Neighbors {
		line := h.formatNeighborLine(neighbor)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Separator
	sb.WriteString("\n")

	// Motivational section
	if result.ClosestAbove != nil && result.XPToOvertakeNext > 0 {
		sb.WriteString("🎯 <b>Цель</b>\n")
		sb.WriteString(fmt.Sprintf("До @%s осталось <b>%d XP</b>",
			escapeHTML(result.ClosestAbove.DisplayName),
			result.XPToOvertakeNext))

		if result.XPToOvertakeNext <= 50 {
			sb.WriteString(" — одна задача! 🔥")
		}
		sb.WriteString("\n\n")
	}

	// Warning about chaser
	if result.ClosestBelow != nil && result.XPAheadOfChaser > 0 && result.XPAheadOfChaser <= 30 {
		sb.WriteString("⚠️ <b>Внимание</b>\n")
		sb.WriteString(fmt.Sprintf("@%s отстаёт всего на <b>%d XP</b>!\n\n",
			escapeHTML(result.ClosestBelow.DisplayName),
			result.XPAheadOfChaser))
	}

	// Stats
	sb.WriteString(fmt.Sprintf("📊 <i>Позиция #%d из %d</i>",
		result.CurrentStudent.Rank,
		result.TotalInCohort))

	if result.OnlineCount > 0 {
		sb.WriteString(fmt.Sprintf(" • 🟢 %d онлайн", result.OnlineCount))
	}

	// Motivational message
	if result.MotivationalMessage != "" {
		sb.WriteString(fmt.Sprintf("\n\n<i>%s</i>", result.MotivationalMessage))
	}

	return sb.String()
}

// formatNeighborLine formats a single neighbor line.
func (h *NeighborsHandler) formatNeighborLine(neighbor query.NeighborDTO) string {
	var sb strings.Builder

	// Position indicator and online status
	if neighbor.IsCurrentStudent {
		sb.WriteString("👤 ")
	} else if neighbor.Position < 0 {
		sb.WriteString("⬆️ ")
	} else {
		sb.WriteString("⬇️ ")
	}

	// Online indicator
	if neighbor.IsOnline {
		sb.WriteString("🟢 ")
	} else {
		sb.WriteString("⚪ ")
	}

	// Rank
	sb.WriteString(fmt.Sprintf("<b>#%d</b> ", neighbor.Rank))

	// Name
	name := escapeHTML(neighbor.DisplayName)
	if neighbor.IsCurrentStudent {
		sb.WriteString(fmt.Sprintf("<b>%s</b>", name))
	} else {
		sb.WriteString(name)
	}

	// XP
	sb.WriteString(fmt.Sprintf(" — %d XP", neighbor.XP))

	// XP gap (for non-current students)
	if !neighbor.IsCurrentStudent && neighbor.XPGap != 0 {
		if neighbor.XPGap > 0 {
			sb.WriteString(fmt.Sprintf(" (+%d)", neighbor.XPGap))
		} else {
			sb.WriteString(fmt.Sprintf(" (%d)", neighbor.XPGap))
		}
	}

	// Rank change
	if neighbor.RankChange != 0 {
		if neighbor.RankChange > 0 {
			sb.WriteString(fmt.Sprintf(" ↑%d", neighbor.RankChange))
		} else {
			sb.WriteString(fmt.Sprintf(" ↓%d", -neighbor.RankChange))
		}
	}

	// Helper badge
	if neighbor.IsAvailableForHelp && neighbor.HelpRating >= 4.0 {
		sb.WriteString(" 🤝")
	}

	return sb.String()
}
