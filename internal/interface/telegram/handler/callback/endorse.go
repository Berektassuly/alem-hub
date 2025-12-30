// Package callback contains inline button callback handlers.
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
// ENDORSE CALLBACK HANDLER
// Handles the "Thanks for help ⭐" button click.
// This is how students thank each other for help, building the social capital
// that makes helpers more visible and trusted.
// Philosophy: Recognition is the fuel of collaboration. Make it easy to say thanks.
// ══════════════════════════════════════════════════════════════════════════════

// EndorseHandler handles the endorse/thanks button callback.
type EndorseHandler struct {
	endorseCmd  *command.GiveEndorsementHandler
	studentRepo student.Repository
	keyboards   *presenter.KeyboardBuilder
}

// NewEndorseHandler creates a new EndorseHandler with dependencies.
func NewEndorseHandler(
	endorseCmd *command.GiveEndorsementHandler,
	studentRepo student.Repository,
	keyboards *presenter.KeyboardBuilder,
) *EndorseHandler {
	return &EndorseHandler{
		endorseCmd:  endorseCmd,
		studentRepo: studentRepo,
		keyboards:   keyboards,
	}
}

// EndorseRequest contains the parsed callback data.
type EndorseRequest struct {
	// TelegramID is the user's Telegram ID who clicked the button.
	TelegramID int64

	// HelperStudentID is the ID of the student being thanked.
	HelperStudentID string

	// TaskID is the task for which help was given (optional).
	TaskID string

	// Rating is the rating to give (1-5).
	Rating float64

	// CallbackQueryID is the callback query ID for answering.
	CallbackQueryID string

	// ChatID is the chat ID for sending messages.
	ChatID int64

	// MessageID is the message ID for editing.
	MessageID int
}

// EndorseResponse contains the response data.
type EndorseResponse struct {
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

	// IsError indicates if this is an error response.
	IsError bool

	// NeedsRating indicates that we need the user to select a rating.
	NeedsRating bool
}

// Handle processes the endorse callback.
func (h *EndorseHandler) Handle(ctx context.Context, req EndorseRequest) (*EndorseResponse, error) {
	// Get giving student
	giver, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(req.TelegramID))
	if err != nil {
		return &EndorseResponse{
			AnswerText: "❌ Ты не зарегистрирован. Используй /start",
			ShowAlert:  true,
			IsError:    true,
		}, nil
	}

	// Get helper student
	helper, err := h.studentRepo.GetByID(ctx, req.HelperStudentID)
	if err != nil {
		return &EndorseResponse{
			AnswerText: "❌ Студент не найден",
			ShowAlert:  true,
			IsError:    true,
		}, nil
	}

	// Cannot endorse self
	if giver.ID == helper.ID {
		return &EndorseResponse{
			AnswerText: "🤔 Нельзя благодарить самого себя",
			ShowAlert:  false,
			IsError:    true,
		}, nil
	}

	// If no rating provided, show rating selection
	if req.Rating <= 0 {
		return h.showRatingSelection(helper, req.TaskID)
	}

	// Validate rating
	if req.Rating < 1 || req.Rating > 5 {
		return &EndorseResponse{
			AnswerText: "❌ Рейтинг должен быть от 1 до 5",
			ShowAlert:  true,
			IsError:    true,
		}, nil
	}

	// Create endorsement command
	cmd := command.GiveEndorsementCommand{
		GiverID:    giver.ID,
		ReceiverID: helper.ID,
		TaskID:     req.TaskID,
		Type:       social.EndorsementTypeClear,
		Rating:     req.Rating,
		IsPublic:   true,
	}

	// Execute endorsement
	result, err := h.endorseCmd.Handle(ctx, cmd)
	if err != nil {
		return &EndorseResponse{
			AnswerText: "❌ Не удалось сохранить благодарность",
			ShowAlert:  true,
			IsError:    true,
		}, nil
	}

	// Build success response
	return h.buildSuccessResponse(helper, result, req.Rating, req.TaskID)
}

// showRatingSelection shows the rating selection keyboard.
func (h *EndorseHandler) showRatingSelection(helper *student.Student, taskID string) (*EndorseResponse, error) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("⭐ <b>Оцени помощь от %s</b>\n\n", escapeHTML(helper.DisplayName)))

	if taskID != "" {
		sb.WriteString(fmt.Sprintf("📋 Задача: <code>%s</code>\n\n", escapeHTML(taskID)))
	}

	sb.WriteString("Насколько полезной была помощь?\n\n")
	sb.WriteString("⭐ — Помог немного\n")
	sb.WriteString("⭐⭐ — Неплохо\n")
	sb.WriteString("⭐⭐⭐ — Хорошо\n")
	sb.WriteString("⭐⭐⭐⭐ — Очень помог!\n")
	sb.WriteString("⭐⭐⭐⭐⭐ — Превосходно!\n")

	keyboard := h.keyboards.RatingKeyboard(helper.ID, taskID)

	return &EndorseResponse{
		UpdatedText:     sb.String(),
		UpdatedKeyboard: keyboard,
		ParseMode:       "HTML",
		NeedsRating:     true,
		IsError:         false,
	}, nil
}

// buildSuccessResponse builds the success response after endorsement.
func (h *EndorseHandler) buildSuccessResponse(
	helper *student.Student,
	result *command.GiveEndorsementResult,
	rating float64,
	taskID string,
) (*EndorseResponse, error) {
	// Toast message
	toastMsg := fmt.Sprintf("🙏 Спасибо! %s получил %.0f⭐", helper.DisplayName, rating)

	// Build detailed message
	var sb strings.Builder

	sb.WriteString("✅ <b>Благодарность отправлена!</b>\n\n")

	sb.WriteString(fmt.Sprintf("👤 %s\n", escapeHTML(helper.DisplayName)))
	sb.WriteString(fmt.Sprintf("⭐ Твоя оценка: %s\n", formatRatingStars(rating)))

	if taskID != "" {
		sb.WriteString(fmt.Sprintf("📋 Задача: <code>%s</code>\n", escapeHTML(taskID)))
	}

	sb.WriteString("\n")

	// Updated helper stats
	sb.WriteString("<b>Статистика помощника:</b>\n")
	sb.WriteString(fmt.Sprintf("├ Новый рейтинг: %.1f ⭐\n", result.ReceiverNewRating))
	sb.WriteString(fmt.Sprintf("└ Всего благодарностей: %d\n\n", result.ReceiverTotalEndorsements))

	// Motivational message based on rating
	switch {
	case rating >= 5:
		sb.WriteString("🌟 <i>Отлично! Такие оценки мотивируют помогать больше!</i>")
	case rating >= 4:
		sb.WriteString("👍 <i>Хорошая оценка! Помощь ценится.</i>")
	case rating >= 3:
		sb.WriteString("🙏 <i>Спасибо за обратную связь!</i>")
	default:
		sb.WriteString("💬 <i>Обратная связь помогает улучшать качество помощи.</i>")
	}

	return &EndorseResponse{
		AnswerText:  toastMsg,
		ShowAlert:   false,
		UpdatedText: sb.String(),
		ParseMode:   "HTML",
		IsError:     false,
	}, nil
}

// ParseEndorseCallbackData parses callback data string into EndorseRequest fields.
// Expected format: "endorse:studentID:rating:taskID"
func ParseEndorseCallbackData(data string) (studentID string, rating float64, taskID string) {
	parts := strings.Split(data, ":")

	if len(parts) >= 2 {
		studentID = parts[1]
	}
	if len(parts) >= 3 {
		fmt.Sscanf(parts[2], "%f", &rating)
	}
	if len(parts) >= 4 {
		taskID = parts[3]
	}

	return
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// formatRatingStars formats rating as star emojis.
func formatRatingStars(rating float64) string {
	stars := int(rating)
	if stars < 1 {
		stars = 1
	}
	if stars > 5 {
		stars = 5
	}
	return strings.Repeat("⭐", stars)
}
