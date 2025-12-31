// Package handler contains Telegram command handlers.
package handler

import (
	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram/presenter"
	"context"
	"fmt"
	"strings"
)

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE HANDLER
// Handles /online command - shows who is currently online.
// This is the "community pulse" - showing that you're not alone.
// Philosophy: Learning is better together. See who's working and join them.
// ══════════════════════════════════════════════════════════════════════════════

// OnlineHandler handles the /online command.
type OnlineHandler struct {
	onlineQuery *query.GetOnlineNowHandler
	studentRepo student.Repository
	keyboards   *presenter.KeyboardBuilder
}

// NewOnlineHandler creates a new OnlineHandler with dependencies.
func NewOnlineHandler(
	onlineQuery *query.GetOnlineNowHandler,
	studentRepo student.Repository,
	keyboards *presenter.KeyboardBuilder,
) *OnlineHandler {
	return &OnlineHandler{
		onlineQuery: onlineQuery,
		studentRepo: studentRepo,
		keyboards:   keyboards,
	}
}

// OnlineRequest contains the parsed /online command data.
type OnlineRequest struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// ChatID is the chat ID for sending responses.
	ChatID int64

	// MessageID is the original message ID (for editing).
	MessageID int

	// Cohort is the cohort filter (empty = all).
	Cohort string

	// IncludeAway includes "away" students (5-30 min inactive).
	IncludeAway bool

	// OnlyHelpers shows only students willing to help.
	OnlyHelpers bool

	// IsRefresh indicates if this is a refresh request.
	IsRefresh bool
}

// OnlineResponse contains the response to send back.
type OnlineResponse struct {
	// Text is the message text (HTML formatted).
	Text string

	// Keyboard is the inline keyboard to attach.
	Keyboard *presenter.InlineKeyboard

	// ParseMode is the parse mode (HTML).
	ParseMode string

	// IsError indicates if this is an error response.
	IsError bool
}

// Handle processes the /online command.
func (h *OnlineHandler) Handle(ctx context.Context, req OnlineRequest) (*OnlineResponse, error) {
	// Verify user is registered
	_, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(req.TelegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	// Build query
	onlineQuery := query.GetOnlineNowQuery{
		Cohort:               req.Cohort,
		IncludeAway:          req.IncludeAway,
		IncludeRecent:        false,
		OnlyAvailableForHelp: req.OnlyHelpers,
		Limit:                20,
		SortBy:               "last_seen",
		SortDesc:             true,
		IncludeRank:          true,
	}

	// Execute query
	result, err := h.onlineQuery.Handle(ctx, onlineQuery)
	if err != nil {
		return h.handleError(err)
	}

	// Build response
	text := h.buildOnlineView(result, req)
	keyboard := h.keyboards.OnlineKeyboard(req.IncludeAway, req.OnlyHelpers)

	return &OnlineResponse{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
		IsError:   false,
	}, nil
}

// handleNotRegistered handles the case when user is not registered.
func (h *OnlineHandler) handleNotRegistered() (*OnlineResponse, error) {
	text := "❌ <b>Ты ещё не зарегистрирован</b>\n\n" +
		"Используй /start чтобы присоединиться к сообществу."

	return &OnlineResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// handleError handles query errors.
func (h *OnlineHandler) handleError(err error) (*OnlineResponse, error) {
	text := "❌ <b>Ошибка загрузки</b>\n\n" +
		"Не удалось получить список онлайн.\n" +
		"Попробуй позже."

	return &OnlineResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// buildOnlineView builds the online students view text.
func (h *OnlineHandler) buildOnlineView(result *query.GetOnlineNowResult, req OnlineRequest) string {
	var sb strings.Builder

	// Header with activity indicator
	activityEmoji := h.getActivityEmoji(result.CommunityActivity)
	sb.WriteString(fmt.Sprintf("%s <b>Сейчас в сети</b>\n\n", activityEmoji))

	// Stats line
	sb.WriteString(fmt.Sprintf("🟢 Онлайн: %d", result.TotalOnline))
	if result.TotalAway > 0 && req.IncludeAway {
		sb.WriteString(fmt.Sprintf(" • 🟡 Отошли: %d", result.TotalAway))
	}
	sb.WriteString("\n\n")

	// No one online
	if len(result.Students) == 0 {
		sb.WriteString("<i>Сейчас никого нет онлайн 😴</i>\n\n")
		sb.WriteString("Ты можешь стать первым! Начни работать и другие увидят, что кто-то уже здесь.")
		return sb.String()
	}

	// List students
	for _, stud := range result.Students {
		line := h.formatOnlineStudent(stud)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Footer
	if result.HasMore {
		sb.WriteString(fmt.Sprintf("\n<i>...и ещё %d</i>", result.TotalCount-len(result.Students)))
	}

	// Message based on activity level
	sb.WriteString(fmt.Sprintf("\n\n<i>%s</i>", result.Message))

	return sb.String()
}

// formatOnlineStudent formats a single online student line.
func (h *OnlineHandler) formatOnlineStudent(stud query.OnlineStudentDTO) string {
	var sb strings.Builder

	// Status emoji
	sb.WriteString(stud.StatusEmoji)
	sb.WriteString(" ")

	// Name
	name := escapeHTML(stud.DisplayName)
	sb.WriteString(fmt.Sprintf("<b>%s</b>", name))

	// Rank (if available)
	if stud.Rank > 0 {
		sb.WriteString(fmt.Sprintf(" #%d", stud.Rank))
	}

	// Last seen
	sb.WriteString(fmt.Sprintf(" — %s", stud.LastSeenFormatted))

	// Today's progress
	if stud.TodayXPGained > 0 {
		sb.WriteString(fmt.Sprintf(" (+%d XP)", stud.TodayXPGained))
	}

	// Helper badge
	if stud.IsAvailableForHelp {
		if stud.HelpRating >= 4.5 {
			sb.WriteString(" 🌟")
		} else if stud.HelpRating >= 4.0 {
			sb.WriteString(" 🤝")
		} else {
			sb.WriteString(" 💬")
		}
	}

	return sb.String()
}

// getActivityEmoji returns emoji based on community activity level.
func (h *OnlineHandler) getActivityEmoji(activity string) string {
	switch activity {
	case "high":
		return "🔥"
	case "medium":
		return "✨"
	default:
		return "💤"
	}
}
