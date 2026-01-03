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
// HELP HANDLER
// Handles /help [task] command - finds students who can help with a specific task.
// This is the CORE FEATURE of the project, implementing the philosophy:
// "From Competition to Collaboration".
// The leaderboard becomes a "phone book of helpers" - not just a ranking.
// ══════════════════════════════════════════════════════════════════════════════

// HelpHandler handles the /help command.
type HelpHandler struct {
	findHelpersQuery *query.FindHelpersHandler
	studentRepo      student.Repository
	keyboards        *presenter.KeyboardBuilder
}

// NewHelpHandler creates a new HelpHandler with dependencies.
func NewHelpHandler(
	findHelpersQuery *query.FindHelpersHandler,
	studentRepo student.Repository,
	keyboards *presenter.KeyboardBuilder,
) *HelpHandler {
	return &HelpHandler{
		findHelpersQuery: findHelpersQuery,
		studentRepo:      studentRepo,
		keyboards:        keyboards,
	}
}

// HelpRequest contains the parsed /help command data.
type HelpRequest struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// ChatID is the chat ID for sending responses.
	ChatID int64

	// MessageID is the original message ID (for editing).
	MessageID int

	// TaskID is the task for which help is needed.
	TaskID string

	// PreferOnline prefers online helpers.
	PreferOnline bool

	// IsRefresh indicates if this is a refresh request.
	IsRefresh bool
}

// HelpResponse contains the response to send back.
type HelpResponse struct {
	// Text is the message text (HTML formatted).
	Text string

	// Keyboard is the inline keyboard to attach.
	Keyboard *presenter.InlineKeyboard

	// ParseMode is the parse mode (HTML).
	ParseMode string

	// IsError indicates if this is an error response.
	IsError bool

	// NeedsTaskInput indicates that we need task input from user.
	NeedsTaskInput bool
}

// Handle processes the /help command.
func (h *HelpHandler) Handle(ctx context.Context, req HelpRequest) (*HelpResponse, error) {
	// Verify user is registered
	currentStudent, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(req.TelegramID))
	if err != nil {
		return h.handleNotRegistered()
	}

	// Check if task ID is provided
	if req.TaskID == "" {
		return h.handleAskForTask()
	}

	// Normalize task ID
	taskID := normalizeTaskID(req.TaskID)

	// Build query
	helpQuery := query.FindHelpersQuery{
		RequesterID:        currentStudent.ID,
		TaskID:             taskID,
		Limit:              5,
		PreferOnline:       true,
		PreferKnownHelpers: true,
		MinHelpRating:      0,
	}

	// Execute query
	result, err := h.findHelpersQuery.Handle(ctx, helpQuery)
	if err != nil {
		return h.handleError(err, taskID)
	}

	// Build response
	text := h.buildHelpersView(result, taskID)
	keyboard := h.keyboards.HelpersKeyboard(result.Helpers, taskID)

	return &HelpResponse{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
		IsError:   false,
	}, nil
}

// handleNotRegistered handles the case when user is not registered.
func (h *HelpHandler) handleNotRegistered() (*HelpResponse, error) {
	text := "❌ <b>Ты ещё не зарегистрирован</b>\n\n" +
		"Используй /start чтобы присоединиться к сообществу."

	return &HelpResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// handleAskForTask handles the case when no task is specified.
func (h *HelpHandler) handleAskForTask() (*HelpResponse, error) {
	text := "🆘 <b>Поиск помощи по задаче</b>\n\n" +
		"Укажи название задачи, по которой тебе нужна помощь:\n\n" +
		"<code>/help название-задачи</code>\n\n" +
		"<b>Примеры:</b>\n" +
		"• <code>/help go-reloaded</code>\n" +
		"• <code>/help ascii-art</code>\n" +
		"• <code>/help math-skills</code>\n\n" +
		"<i>💡 Я найду студентов, которые уже решили эту задачу и готовы помочь.</i>"

	return &HelpResponse{
		Text:           text,
		ParseMode:      "HTML",
		IsError:        false,
		NeedsTaskInput: true,
	}, nil
}

// handleError handles query errors.
func (h *HelpHandler) handleError(err error, taskID string) (*HelpResponse, error) {
	text := fmt.Sprintf(
		"❌ <b>Не удалось найти помощников</b>\n\n"+
			"Задача: <code>%s</code>\n\n"+
			"Возможные причины:\n"+
			"• Никто ещё не решил эту задачу\n"+
			"• Название задачи указано неверно\n\n"+
			"<i>Попробуй проверить название или спросить в общем чате.</i>",
		escapeHTML(taskID),
	)

	return &HelpResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// buildHelpersView builds the helpers view text.
func (h *HelpHandler) buildHelpersView(result *query.FindHelpersResult, taskID string) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("🆘 <b>Помощь по задаче</b>\n"))
	sb.WriteString(fmt.Sprintf("📋 <code>%s</code>\n\n", escapeHTML(taskID)))

	// Stats
	sb.WriteString(fmt.Sprintf("👥 Решило задачу: %d\n", result.TotalSolvers))
	if result.OnlineSolvers > 0 {
		sb.WriteString(fmt.Sprintf("🟢 Сейчас онлайн: %d\n", result.OnlineSolvers))
	}
	sb.WriteString("\n")

	// No helpers found
	if len(result.Helpers) == 0 {
		sb.WriteString("<i>К сожалению, не удалось найти доступных помощников.</i>\n\n")
		sb.WriteString("Попробуй:\n")
		sb.WriteString("• Подождать — возможно, кто-то скоро появится\n")
		sb.WriteString("• Спросить в общем чате сообщества")
		return sb.String()
	}

	// List helpers
	sb.WriteString("<b>Кто может помочь:</b>\n\n")

	for i, helper := range result.Helpers {
		line := h.formatHelper(i+1, helper)
		sb.WriteString(line)
		sb.WriteString("\n\n")
	}

	// Footer
	sb.WriteString("<i>💡 Нажми на кнопку «Написать» чтобы связаться с помощником.</i>\n")
	sb.WriteString("<i>После получения помощи не забудь поблагодарить! 🙏</i>")

	return sb.String()
}

// formatHelper formats a single helper entry.
func (h *HelpHandler) formatHelper(num int, helper query.HelperDTO) string {
	var sb strings.Builder

	// Number and online status
	sb.WriteString(fmt.Sprintf("%d. ", num))

	// Online indicator
	switch helper.OnlineStatus {
	case "online":
		sb.WriteString("🟢 ")
	case "away":
		sb.WriteString("🟡 ")
	default:
		sb.WriteString("⚪ ")
	}

	// Name
	sb.WriteString(fmt.Sprintf("<b>%s</b>", escapeHTML(helper.DisplayName)))

	sb.WriteString("\n")

	// Rating and help count
	if helper.HelpRating > 0 {
		sb.WriteString(fmt.Sprintf("   %s %.1f", helper.HelpRatingFormatted, helper.HelpRating))
		if helper.TotalHelpCount > 0 {
			sb.WriteString(fmt.Sprintf(" (%d помощей)", helper.TotalHelpCount))
		}
		sb.WriteString("\n")
	}

	// Time since completion
	if helper.TimeSinceCompletion != "" {
		sb.WriteString(fmt.Sprintf("   ✅ Решил: %s\n", helper.TimeSinceCompletion))
	}

	// Online status
	if helper.LastSeenFormatted != "" {
		sb.WriteString(fmt.Sprintf("   🕐 %s\n", helper.LastSeenFormatted))
	}

	// Prior contact indicator
	if helper.HasPriorContact {
		sb.WriteString("   🤝 Помогал тебе раньше\n")
	}

	// Recommendation reason
	if helper.RecommendationReason != "" {
		sb.WriteString(fmt.Sprintf("   %s", helper.RecommendationReason))
	}

	return sb.String()
}

// HandleTaskMessage handles text messages with task name.
func (h *HelpHandler) HandleTaskMessage(ctx context.Context, req HelpRequest, taskText string) (*HelpResponse, error) {
	req.TaskID = strings.TrimSpace(taskText)
	return h.Handle(ctx, req)
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// normalizeTaskID normalizes task ID for consistent matching.
func normalizeTaskID(taskID string) string {
	// Lowercase
	result := strings.ToLower(taskID)
	// Trim whitespace
	result = strings.TrimSpace(result)
	// Replace spaces with hyphens
	result = strings.ReplaceAll(result, " ", "-")
	// Remove multiple hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return result
}
