// Package callback contains inline button callback handlers.
// Callbacks handle user interactions with inline keyboards.
package callback

import (
	"github.com/alem-hub/alem-community-hub/internal/application/command"
	"github.com/alem-hub/alem-community-hub/internal/domain/social"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram/presenter"
	"context"
	"fmt"
	"strings"
)

// ══════════════════════════════════════════════════════════════════════════════
// CONNECT CALLBACK HANDLER
// Handles the "Write to @user" button click.
// This creates a connection between students, recording that they've interacted.
// Philosophy: Every connection is valuable. Track them to build a social graph
// that helps students find helpers more effectively.
// ══════════════════════════════════════════════════════════════════════════════

// ConnectHandler handles the connect button callback.
type ConnectHandler struct {
	connectCmd  *command.ConnectStudentsHandler
	studentRepo student.Repository
	keyboards   *presenter.KeyboardBuilder
}

// NewConnectHandler creates a new ConnectHandler with dependencies.
func NewConnectHandler(
	connectCmd *command.ConnectStudentsHandler,
	studentRepo student.Repository,
	keyboards *presenter.KeyboardBuilder,
) *ConnectHandler {
	return &ConnectHandler{
		connectCmd:  connectCmd,
		studentRepo: studentRepo,
		keyboards:   keyboards,
	}
}

// ConnectRequest contains the parsed callback data.
type ConnectRequest struct {
	// TelegramID is the user's Telegram ID who clicked the button.
	TelegramID int64

	// TargetStudentID is the ID of the student to connect with.
	TargetStudentID string

	// Context describes why the connection was made (e.g., "help_request").
	Context string

	// TaskID is the task related to this connection (optional).
	TaskID string

	// CallbackQueryID is the callback query ID for answering.
	CallbackQueryID string

	// ChatID is the chat ID for sending messages.
	ChatID int64

	// MessageID is the message ID for editing.
	MessageID int
}

// ConnectResponse contains the response data.
type ConnectResponse struct {
	// AnswerText is the text to show in the callback answer toast.
	AnswerText string

	// ShowAlert determines if the answer should be shown as an alert.
	ShowAlert bool

	// UpdatedText is the updated message text (optional).
	UpdatedText string

	// UpdatedKeyboard is the updated keyboard (optional).
	UpdatedKeyboard *presenter.InlineKeyboard

	// ParseMode is the parse mode for updated text.
	ParseMode string

	// TargetUsername is the username to direct the user to message.
	TargetUsername string

	// DeepLink is a deep link URL (optional).
	DeepLink string

	// IsError indicates if this is an error response.
	IsError bool
}

// Handle processes the connect callback.
func (h *ConnectHandler) Handle(ctx context.Context, req ConnectRequest) (*ConnectResponse, error) {
	// Get initiating student
	initiator, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(req.TelegramID))
	if err != nil {
		return &ConnectResponse{
			AnswerText: "❌ Ты не зарегистрирован. Используй /start",
			ShowAlert:  true,
			IsError:    true,
		}, nil
	}

	// Get target student
	target, err := h.studentRepo.GetByID(ctx, req.TargetStudentID)
	if err != nil {
		return &ConnectResponse{
			AnswerText: "❌ Студент не найден",
			ShowAlert:  true,
			IsError:    true,
		}, nil
	}

	// Cannot connect to self
	if initiator.ID == target.ID {
		return &ConnectResponse{
			AnswerText: "🤔 Ты не можешь написать сам себе",
			ShowAlert:  false,
			IsError:    true,
		}, nil
	}

	// Determine connection type based on context
	connectionType := social.ConnectionTypeStudyBuddy
	if req.Context == "help_request" || req.TaskID != "" {
		connectionType = social.ConnectionTypeHelper
	}

	// Create connection command
	cmd := command.ConnectStudentsCommand{
		InitiatorID:      initiator.ID,
		TargetID:         target.ID,
		Type:             connectionType,
		Context:          req.Context,
		TaskID:           req.TaskID,
		SkipConfirmation: true, // Auto-accept for help connections
	}

	// Execute connection
	result, err := h.connectCmd.Handle(ctx, cmd)
	if err != nil {
		// Log error but don't fail - still provide contact info
		// In production, this would be logged
	}

	// Build response with contact information
	response := h.buildResponse(target, result, req.TaskID)

	return response, nil
}

// buildResponse builds the response with contact information.
func (h *ConnectHandler) buildResponse(target *student.Student, connResult *command.ConnectStudentsResult, taskID string) *ConnectResponse {
	// Build toast message
	toastMsg := fmt.Sprintf("📨 Напиши %s в Telegram!", target.DisplayName)

	// Build detailed message
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📨 <b>Связь с %s</b>\n\n", escapeHTML(target.DisplayName)))

	// Profile info
	sb.WriteString(fmt.Sprintf("👤 <b>%s</b>\n", escapeHTML(target.DisplayName)))
	sb.WriteString(fmt.Sprintf("🎯 Уровень: %d\n", target.Level()))

	// Helper rating
	if target.HelpCount > 0 {
		sb.WriteString(fmt.Sprintf("⭐ Рейтинг помощника: %.1f (%d помощей)\n", target.HelpRating, target.HelpCount))
	}

	sb.WriteString("\n")

	// Connection status
	if connResult != nil {
		if connResult.IsNewConnection {
			sb.WriteString("✅ Контакт сохранён!\n\n")
		} else {
			sb.WriteString("🔄 Вы уже связывались раньше.\n\n")
		}
	}

	// Task context
	if taskID != "" {
		sb.WriteString(fmt.Sprintf("📋 Задача: <code>%s</code>\n\n", escapeHTML(taskID)))
	}

	// Instructions
	sb.WriteString("<b>Как связаться:</b>\n")
	sb.WriteString(fmt.Sprintf("1. <a href=\"tg://user?id=%d\">Нажми сюда</a>, чтобы открыть чат\n", target.TelegramID))
	sb.WriteString("2. Напиши своё сообщение\n")
	sb.WriteString("3. После помощи не забудь поблагодарить! 🙏\n\n")

	// Etiquette reminder
	sb.WriteString("<i>💡 Совет: Опиши проблему кратко и конкретно.\n")
	sb.WriteString("Это поможет получить помощь быстрее!</i>")

	return &ConnectResponse{
		AnswerText:     toastMsg,
		ShowAlert:      false,
		UpdatedText:    sb.String(),
		ParseMode:      "HTML",
		TargetUsername: "", // Removed usage of AlemLogin as username
		IsError:        false,
	}
}

// ParseCallbackData parses callback data string into ConnectRequest fields.
// Expected format: "connect:studentID:context:taskID"
func ParseConnectCallbackData(data string) (studentID, context, taskID string) {
	parts := strings.Split(data, ":")

	if len(parts) >= 2 {
		studentID = parts[1]
	}
	if len(parts) >= 3 {
		context = parts[2]
	}
	if len(parts) >= 4 {
		taskID = parts[3]
	}

	return
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// escapeHTML escapes HTML special characters.
func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return replacer.Replace(s)
}
