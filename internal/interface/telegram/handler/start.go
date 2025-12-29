// Package handler contains Telegram command handlers.
// Each handler follows the pattern: receive update → validate → call application layer → format response.
package handler

import (
	"alem-hub/internal/application/saga"
	"alem-hub/internal/domain/student"
	"alem-hub/internal/interface/telegram/presenter"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ══════════════════════════════════════════════════════════════════════════════
// START HANDLER
// Handles /start command - the onboarding flow for new students.
// Philosophy: First impression matters. Make students feel welcome and part
// of a supportive community from the very first interaction.
// ══════════════════════════════════════════════════════════════════════════════

// StartHandler handles the /start command for onboarding.
type StartHandler struct {
	onboardingSaga *saga.OnboardingSaga
	studentRepo    student.Repository
	keyboards      *presenter.KeyboardBuilder
}

// NewStartHandler creates a new StartHandler with dependencies.
func NewStartHandler(
	onboardingSaga *saga.OnboardingSaga,
	studentRepo student.Repository,
	keyboards *presenter.KeyboardBuilder,
) *StartHandler {
	return &StartHandler{
		onboardingSaga: onboardingSaga,
		studentRepo:    studentRepo,
		keyboards:      keyboards,
	}
}

// StartRequest contains the parsed /start command data.
type StartRequest struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// TelegramUsername is the user's Telegram username (without @).
	TelegramUsername string

	// FirstName is the user's first name from Telegram.
	FirstName string

	// LastName is the user's last name from Telegram.
	LastName string

	// DeepLinkParam is the parameter passed via deep link (e.g., /start alemlogin).
	DeepLinkParam string

	// ChatID is the chat ID for sending responses.
	ChatID int64

	// MessageID is the original message ID (for editing).
	MessageID int
}

// StartResponse contains the response to send back.
type StartResponse struct {
	// Text is the message text (HTML formatted).
	Text string

	// Keyboard is the inline keyboard to attach.
	Keyboard *presenter.InlineKeyboard

	// ParseMode is the parse mode (HTML).
	ParseMode string

	// IsError indicates if this is an error response.
	IsError bool
}

// Handle processes the /start command.
func (h *StartHandler) Handle(ctx context.Context, req StartRequest) (*StartResponse, error) {
	// Check if user is already registered
	existingStudent, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(req.TelegramID))
	if err == nil && existingStudent != nil {
		// User is already registered - show welcome back message
		return h.handleExistingUser(ctx, existingStudent)
	}

	// New user - check if they provided Alem login
	if req.DeepLinkParam != "" {
		// Deep link with Alem login provided
		return h.handleOnboarding(ctx, req)
	}

	// No login provided - ask for it
	return h.handleAskForLogin(ctx, req)
}

// handleExistingUser handles the case when user is already registered.
func (h *StartHandler) handleExistingUser(ctx context.Context, stud *student.Student) (*StartResponse, error) {
	text := fmt.Sprintf(
		"С возвращением, <b>%s</b>! 👋\n\n"+
			"Ты уже зарегистрирован в Alem Community Hub.\n\n"+
			"📊 <b>Твой XP:</b> %d\n"+
			"🎯 <b>Уровень:</b> %d\n\n"+
			"<b>Доступные команды:</b>\n"+
			"• /me — твоя карточка\n"+
			"• /top — лидерборд\n"+
			"• /neighbors — соседи по рангу\n"+
			"• /online — кто сейчас работает\n"+
			"• /help — найти помощь по задаче\n"+
			"• /settings — настройки\n\n"+
			"Удачи в обучении! 🚀",
		stud.DisplayName,
		stud.CurrentXP,
		stud.Level(),
	)

	keyboard := h.keyboards.WelcomeBackKeyboard()

	return &StartResponse{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
		IsError:   false,
	}, nil
}

// handleAskForLogin handles the case when no Alem login is provided.
func (h *StartHandler) handleAskForLogin(ctx context.Context, req StartRequest) (*StartResponse, error) {
	greeting := "там"
	if req.FirstName != "" {
		greeting = req.FirstName
	}

	text := fmt.Sprintf(
		"Привет, %s! 👋\n\n"+
			"Добро пожаловать в <b>Alem Community Hub</b> — неофициальное сообщество студентов Alem School.\n\n"+
			"🎯 <b>Что это такое?</b>\n"+
			"Это место, где лидерборд — не про соревнование, а про взаимопомощь. "+
			"Здесь ты можешь найти тех, кто решил задачу, на которой ты застрял, "+
			"и помочь другим в ответ.\n\n"+
			"📝 <b>Для регистрации отправь свой логин Alem:</b>\n"+
			"Просто напиши его в чат (например: <code>ivanov_i</code>)\n\n"+
			"<i>Или используй ссылку:</i>\n"+
			"<code>https://t.me/AlemHubBot?start=твой_логин</code>",
		greeting,
	)

	return &StartResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   false,
	}, nil
}

// handleOnboarding handles the full onboarding process.
func (h *StartHandler) handleOnboarding(ctx context.Context, req StartRequest) (*StartResponse, error) {
	// Validate and clean the Alem login
	alemLogin := cleanAlemLogin(req.DeepLinkParam)
	if !isValidAlemLogin(alemLogin) {
		return h.handleInvalidLogin(alemLogin)
	}

	// Execute onboarding saga
	input := saga.OnboardingInput{
		TelegramID:       req.TelegramID,
		TelegramUsername: req.TelegramUsername,
		AlemLogin:        alemLogin,
	}

	result, err := h.onboardingSaga.Execute(ctx, input)
	if err != nil {
		return h.handleOnboardingError(err, alemLogin)
	}

	// Success - build welcome message
	return h.handleOnboardingSuccess(result)
}

// handleInvalidLogin handles invalid Alem login input.
func (h *StartHandler) handleInvalidLogin(login string) (*StartResponse, error) {
	text := fmt.Sprintf(
		"❌ <b>Некорректный логин</b>\n\n"+
			"Логин <code>%s</code> не соответствует формату.\n\n"+
			"Логин Alem должен:\n"+
			"• Содержать от 2 до 50 символов\n"+
			"• Не содержать пробелов\n\n"+
			"Попробуй ещё раз, отправив правильный логин.",
		escapeHTML(login),
	)

	return &StartResponse{
		Text:      text,
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// handleOnboardingError handles errors during onboarding.
func (h *StartHandler) handleOnboardingError(err error, login string) (*StartResponse, error) {
	var onboardingErr *saga.OnboardingError
	if errors.As(err, &onboardingErr) {
		switch {
		case errors.Is(onboardingErr.Cause, saga.ErrStudentAlreadyRegistered):
			return &StartResponse{
				Text: "⚠️ <b>Аккаунт уже зарегистрирован</b>\n\n" +
					"Этот Telegram аккаунт уже связан с логином Alem.\n" +
					"Используй /me чтобы посмотреть свой профиль.",
				ParseMode: "HTML",
				IsError:   true,
			}, nil

		case errors.Is(onboardingErr.Cause, saga.ErrAlemLoginAlreadyLinked):
			return &StartResponse{
				Text: fmt.Sprintf(
					"⚠️ <b>Логин уже используется</b>\n\n"+
						"Логин <code>%s</code> уже связан с другим Telegram аккаунтом.\n\n"+
						"Если это твой логин и ты потерял доступ к старому аккаунту, "+
						"обратись к администратору.",
					escapeHTML(login),
				),
				ParseMode: "HTML",
				IsError:   true,
			}, nil

		case errors.Is(onboardingErr.Cause, saga.ErrAlemLoginNotFound):
			return &StartResponse{
				Text: fmt.Sprintf(
					"❌ <b>Логин не найден</b>\n\n"+
						"Логин <code>%s</code> не найден на платформе Alem.\n\n"+
						"Проверь правильность написания и попробуй снова.",
					escapeHTML(login),
				),
				ParseMode: "HTML",
				IsError:   true,
			}, nil

		case errors.Is(onboardingErr.Cause, saga.ErrAlemAPIUnavailable):
			return &StartResponse{
				Text: "⚠️ <b>Сервис временно недоступен</b>\n\n" +
					"Не удалось связаться с платформой Alem.\n" +
					"Попробуй через несколько минут.",
				ParseMode: "HTML",
				IsError:   true,
			}, nil
		}
	}

	// Generic error
	return &StartResponse{
		Text: "❌ <b>Произошла ошибка</b>\n\n" +
			"Не удалось завершить регистрацию.\n" +
			"Попробуй позже или обратись к администратору.",
		ParseMode: "HTML",
		IsError:   true,
	}, nil
}

// handleOnboardingSuccess handles successful onboarding.
func (h *StartHandler) handleOnboardingSuccess(result *saga.OnboardingResult) (*StartResponse, error) {
	stud := result.Student

	rankInfo := ""
	if result.InitialRank > 0 {
		rankInfo = fmt.Sprintf("📍 <b>Твоя позиция:</b> #%d\n", result.InitialRank)
	}

	text := fmt.Sprintf(
		"🎉 <b>Добро пожаловать, %s!</b>\n\n"+
			"Ты успешно присоединился к Alem Community Hub!\n\n"+
			"📊 <b>Твой XP:</b> %d\n"+
			"🎯 <b>Уровень:</b> %d\n"+
			"%s\n"+
			"<b>Что ты можешь делать:</b>\n"+
			"• /me — твоя карточка и статистика\n"+
			"• /top — лидерборд когорты\n"+
			"• /neighbors — твои соседи по рангу\n"+
			"• /online — кто сейчас работает\n"+
			"• /help [задача] — найти того, кто решил задачу\n"+
			"• /settings — настройки уведомлений\n\n"+
			"<i>💡 Философия Hub: «От конкуренции к сотрудничеству».\n"+
			"Здесь лидерборд — не про соревнование, а про поиск помощи.</i>\n\n"+
			"Удачи в обучении! 🚀",
		stud.DisplayName,
		stud.CurrentXP,
		stud.Level(),
		rankInfo,
	)

	keyboard := h.keyboards.OnboardingSuccessKeyboard()

	return &StartResponse{
		Text:      text,
		Keyboard:  keyboard,
		ParseMode: "HTML",
		IsError:   false,
	}, nil
}

// HandleTextMessage handles text messages (Alem login input during onboarding).
func (h *StartHandler) HandleTextMessage(ctx context.Context, req StartRequest, text string) (*StartResponse, error) {
	// Check if already registered
	existingStudent, err := h.studentRepo.GetByTelegramID(ctx, student.TelegramID(req.TelegramID))
	if err == nil && existingStudent != nil {
		// Already registered - suggest using commands
		return &StartResponse{
			Text: "Ты уже зарегистрирован! 👋\n\n" +
				"Используй /me чтобы посмотреть свой профиль.",
			ParseMode: "HTML",
			IsError:   false,
		}, nil
	}

	// Treat text as Alem login attempt
	req.DeepLinkParam = text
	return h.handleOnboarding(ctx, req)
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// alemLoginRegex matches valid Alem logins.
var alemLoginRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{2,50}$`)

// cleanAlemLogin cleans and normalizes Alem login input.
func cleanAlemLogin(input string) string {
	login := strings.TrimSpace(input)
	login = strings.ToLower(login)
	// Remove @ if user accidentally added it
	login = strings.TrimPrefix(login, "@")
	return login
}

// isValidAlemLogin checks if the login is valid.
func isValidAlemLogin(login string) bool {
	if len(login) < 2 || len(login) > 50 {
		return false
	}
	return alemLoginRegex.MatchString(login)
}

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
