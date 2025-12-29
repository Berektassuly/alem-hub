// Package telegram implements Telegram Bot interface for Alem Community Hub.
package telegram

import (
	"alem-hub/internal/infrastructure/external/telegram"
	"alem-hub/internal/interface/telegram/handler"
	"alem-hub/internal/interface/telegram/presenter"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
)

// ══════════════════════════════════════════════════════════════════════════════
// ROUTER CONFIGURATION
// ══════════════════════════════════════════════════════════════════════════════

// RouterConfig contains configuration for the router.
type RouterConfig struct {
	// Logger for structured logging.
	Logger *slog.Logger

	// Debug enables debug logging for routing decisions.
	Debug bool
}

// ══════════════════════════════════════════════════════════════════════════════
// CONTEXT TYPES
// These types carry context information through the routing process.
// ══════════════════════════════════════════════════════════════════════════════

// CommandContext contains context for command handling.
type CommandContext struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// ChatID is the chat ID where the command was sent.
	ChatID int64

	// MessageID is the ID of the message containing the command.
	MessageID int

	// Args is the command arguments (text after the command).
	Args string

	// Message is the original Telegram message.
	Message *telegram.Message

	// Client is the Telegram client for sending responses.
	Client *telegram.Client
}

// CallbackContext contains context for callback query handling.
type CallbackContext struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// ChatID is the chat ID where the callback originated.
	ChatID int64

	// MessageID is the ID of the message with the inline keyboard.
	MessageID int

	// QueryID is the callback query ID (for answering).
	QueryID string

	// Data is the callback data string.
	Data string

	// Query is the original callback query.
	Query *telegram.CallbackQuery

	// Client is the Telegram client for sending responses.
	Client *telegram.Client
}

// TextInputContext contains context for text input handling (e.g., onboarding).
type TextInputContext struct {
	// TelegramID is the user's Telegram ID.
	TelegramID int64

	// ChatID is the chat ID.
	ChatID int64

	// MessageID is the message ID.
	MessageID int

	// Text is the input text.
	Text string

	// Message is the original message.
	Message *telegram.Message

	// Client is the Telegram client.
	Client *telegram.Client
}

// ══════════════════════════════════════════════════════════════════════════════
// HANDLER INTERFACES
// Interfaces that handlers must implement to be registered with the router.
// ══════════════════════════════════════════════════════════════════════════════

// CommandHandler is the interface for command handlers.
type CommandHandler interface {
	// Handle processes the command and returns a response.
	// The handler should use ctx.Client to send responses.
	Handle(ctx context.Context, cmdCtx CommandContext) error
}

// CallbackHandler is the interface for callback handlers.
type CallbackHandler interface {
	// Handle processes the callback query.
	Handle(ctx context.Context, cbCtx CallbackContext) error
}

// TextInputHandler is the interface for text input handlers.
type TextInputHandler interface {
	// Handle processes text input.
	Handle(ctx context.Context, inputCtx TextInputContext) error
}

// ══════════════════════════════════════════════════════════════════════════════
// ROUTER
// Routes incoming updates to appropriate handlers.
// ══════════════════════════════════════════════════════════════════════════════

// Router routes Telegram updates to appropriate handlers.
type Router struct {
	config RouterConfig
	logger *slog.Logger

	// Command handlers by command name (without /)
	commandHandlers   map[string]interface{}
	commandHandlersMu sync.RWMutex

	// Callback handlers by prefix
	callbackPrefixHandlers   map[string]interface{}
	callbackPrefixHandlersMu sync.RWMutex

	// Text input handler (for onboarding)
	textInputHandler   interface{}
	textInputHandlerMu sync.RWMutex

	// Default handlers for unknown commands/callbacks
	defaultCommandHandler  func(ctx context.Context, cmdCtx CommandContext) error
	defaultCallbackHandler func(ctx context.Context, cbCtx CallbackContext) error
}

// NewRouter creates a new router.
func NewRouter(config RouterConfig) *Router {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	r := &Router{
		config:                 config,
		logger:                 config.Logger,
		commandHandlers:        make(map[string]interface{}),
		callbackPrefixHandlers: make(map[string]interface{}),
	}

	// Set default handlers
	r.defaultCommandHandler = r.handleUnknownCommand
	r.defaultCallbackHandler = r.handleUnknownCallback

	return r
}

// ══════════════════════════════════════════════════════════════════════════════
// REGISTRATION METHODS
// ══════════════════════════════════════════════════════════════════════════════

// RegisterCommand registers a handler for a specific command.
// The command should be without the leading "/".
func (r *Router) RegisterCommand(command string, handler interface{}) {
	r.commandHandlersMu.Lock()
	defer r.commandHandlersMu.Unlock()

	r.commandHandlers[command] = handler

	if r.config.Debug {
		r.logger.Debug("registered command handler", "command", command)
	}
}

// RegisterCallbackPrefix registers a handler for callbacks matching a prefix.
// The prefix should include the trailing delimiter (e.g., "connect:").
func (r *Router) RegisterCallbackPrefix(prefix string, handler interface{}) {
	r.callbackPrefixHandlersMu.Lock()
	defer r.callbackPrefixHandlersMu.Unlock()

	r.callbackPrefixHandlers[prefix] = handler

	if r.config.Debug {
		r.logger.Debug("registered callback prefix handler", "prefix", prefix)
	}
}

// RegisterTextInputHandler registers a handler for text input during onboarding.
func (r *Router) RegisterTextInputHandler(handler interface{}) {
	r.textInputHandlerMu.Lock()
	defer r.textInputHandlerMu.Unlock()

	r.textInputHandler = handler
}

// SetDefaultCommandHandler sets the handler for unknown commands.
func (r *Router) SetDefaultCommandHandler(handler func(ctx context.Context, cmdCtx CommandContext) error) {
	r.defaultCommandHandler = handler
}

// SetDefaultCallbackHandler sets the handler for unknown callbacks.
func (r *Router) SetDefaultCallbackHandler(handler func(ctx context.Context, cbCtx CallbackContext) error) {
	r.defaultCallbackHandler = handler
}

// ══════════════════════════════════════════════════════════════════════════════
// ROUTING METHODS
// ══════════════════════════════════════════════════════════════════════════════

// HandleCommand routes a command to its handler.
func (r *Router) HandleCommand(ctx context.Context, command string, cmdCtx CommandContext) error {
	r.commandHandlersMu.RLock()
	h, ok := r.commandHandlers[command]
	r.commandHandlersMu.RUnlock()

	if !ok {
		if r.config.Debug {
			r.logger.Debug("no handler for command", "command", command)
		}
		return r.defaultCommandHandler(ctx, cmdCtx)
	}

	return r.executeCommandHandler(ctx, h, command, cmdCtx)
}

// executeCommandHandler executes a command handler based on its type.
func (r *Router) executeCommandHandler(ctx context.Context, h interface{}, command string, cmdCtx CommandContext) error {
	switch handler := h.(type) {
	case *handler.StartHandler:
		return r.handleStartCommand(ctx, handler, cmdCtx)
	case *handler.MeHandler:
		return r.handleMeCommand(ctx, handler, cmdCtx)
	case *handler.TopHandler:
		return r.handleTopCommand(ctx, handler, cmdCtx)
	case *handler.NeighborsHandler:
		return r.handleNeighborsCommand(ctx, handler, cmdCtx)
	case *handler.OnlineHandler:
		return r.handleOnlineCommand(ctx, handler, cmdCtx)
	case *handler.HelpHandler:
		return r.handleHelpCommand(ctx, handler, cmdCtx)
	case *handler.SettingsHandler:
		return r.handleSettingsCommand(ctx, handler, cmdCtx)
	case CommandHandler:
		return handler.Handle(ctx, cmdCtx)
	default:
		r.logger.Warn("unknown handler type", "command", command, "type", fmt.Sprintf("%T", h))
		return r.defaultCommandHandler(ctx, cmdCtx)
	}
}

// HandleCallback routes a callback to its handler.
func (r *Router) HandleCallback(ctx context.Context, data string, cbCtx CallbackContext) error {
	r.callbackPrefixHandlersMu.RLock()
	var matchedPrefix string
	var matchedHandler interface{}
	for prefix, h := range r.callbackPrefixHandlers {
		if strings.HasPrefix(data, prefix) {
			// Find the longest matching prefix
			if len(prefix) > len(matchedPrefix) {
				matchedPrefix = prefix
				matchedHandler = h
			}
		}
	}
	r.callbackPrefixHandlersMu.RUnlock()

	if matchedHandler == nil {
		if r.config.Debug {
			r.logger.Debug("no handler for callback", "data", data)
		}
		return r.defaultCallbackHandler(ctx, cbCtx)
	}

	return r.executeCallbackHandler(ctx, matchedHandler, matchedPrefix, cbCtx)
}

// executeCallbackHandler executes a callback handler based on its type.
func (r *Router) executeCallbackHandler(ctx context.Context, h interface{}, prefix string, cbCtx CallbackContext) error {
	switch handler := h.(type) {
	case CallbackHandler:
		return handler.Handle(ctx, cbCtx)
	case func(ctx context.Context, cbCtx CallbackContext) error:
		return handler(ctx, cbCtx)
	default:
		r.logger.Warn("unknown callback handler type", "prefix", prefix, "type", fmt.Sprintf("%T", h))
		return r.defaultCallbackHandler(ctx, cbCtx)
	}
}

// HandleTextInput routes text input to the appropriate handler.
func (r *Router) HandleTextInput(ctx context.Context, inputCtx TextInputContext) error {
	r.textInputHandlerMu.RLock()
	h := r.textInputHandler
	r.textInputHandlerMu.RUnlock()

	if h == nil {
		// Default: treat as onboarding login input
		return r.handleOnboardingInput(ctx, inputCtx)
	}

	switch handler := h.(type) {
	case TextInputHandler:
		return handler.Handle(ctx, inputCtx)
	case func(ctx context.Context, inputCtx TextInputContext) error:
		return handler(ctx, inputCtx)
	default:
		return nil
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// COMMAND HANDLER ADAPTERS
// Convert specific handler types to the generic routing interface.
// ══════════════════════════════════════════════════════════════════════════════

func (r *Router) handleStartCommand(ctx context.Context, h *handler.StartHandler, cmdCtx CommandContext) error {
	req := handler.StartRequest{
		TelegramID:       cmdCtx.TelegramID,
		TelegramUsername: "",
		FirstName:        "",
		LastName:         "",
		DeepLinkParam:    cmdCtx.Args,
		ChatID:           cmdCtx.ChatID,
		MessageID:        cmdCtx.MessageID,
	}

	if cmdCtx.Message != nil && cmdCtx.Message.From != nil {
		req.TelegramUsername = cmdCtx.Message.From.Username
		req.FirstName = cmdCtx.Message.From.FirstName
		req.LastName = cmdCtx.Message.From.LastName
	}

	resp, err := h.Handle(ctx, req)
	if err != nil {
		return err
	}

	return r.sendResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, resp.Text, resp.ParseMode, resp.Keyboard)
}

func (r *Router) handleMeCommand(ctx context.Context, h *handler.MeHandler, cmdCtx CommandContext) error {
	req := handler.MeRequest{
		TelegramID: cmdCtx.TelegramID,
		ChatID:     cmdCtx.ChatID,
		MessageID:  cmdCtx.MessageID,
		IsRefresh:  false,
	}

	resp, err := h.Handle(ctx, req)
	if err != nil {
		return err
	}

	return r.sendResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, resp.Text, resp.ParseMode, resp.Keyboard)
}

func (r *Router) handleTopCommand(ctx context.Context, h *handler.TopHandler, cmdCtx CommandContext) error {
	req := handler.TopRequest{
		TelegramID: cmdCtx.TelegramID,
		ChatID:     cmdCtx.ChatID,
		MessageID:  cmdCtx.MessageID,
		Page:       1,
		OnlyOnline: false,
	}

	resp, err := h.Handle(ctx, req)
	if err != nil {
		return err
	}

	return r.sendResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, resp.Text, resp.ParseMode, resp.Keyboard)
}

func (r *Router) handleNeighborsCommand(ctx context.Context, h *handler.NeighborsHandler, cmdCtx CommandContext) error {
	req := handler.NeighborsRequest{
		TelegramID: cmdCtx.TelegramID,
		ChatID:     cmdCtx.ChatID,
		MessageID:  cmdCtx.MessageID,
	}

	resp, err := h.Handle(ctx, req)
	if err != nil {
		return err
	}

	return r.sendResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, resp.Text, resp.ParseMode, resp.Keyboard)
}

func (r *Router) handleOnlineCommand(ctx context.Context, h *handler.OnlineHandler, cmdCtx CommandContext) error {
	req := handler.OnlineRequest{
		TelegramID:  cmdCtx.TelegramID,
		ChatID:      cmdCtx.ChatID,
		MessageID:   cmdCtx.MessageID,
		IncludeAway: false,
		OnlyHelpers: false,
	}

	resp, err := h.Handle(ctx, req)
	if err != nil {
		return err
	}

	return r.sendResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, resp.Text, resp.ParseMode, resp.Keyboard)
}

func (r *Router) handleHelpCommand(ctx context.Context, h *handler.HelpHandler, cmdCtx CommandContext) error {
	req := handler.HelpRequest{
		TelegramID: cmdCtx.TelegramID,
		ChatID:     cmdCtx.ChatID,
		MessageID:  cmdCtx.MessageID,
		TaskID:     strings.TrimSpace(cmdCtx.Args),
	}

	resp, err := h.Handle(ctx, req)
	if err != nil {
		return err
	}

	return r.sendResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, resp.Text, resp.ParseMode, resp.Keyboard)
}

func (r *Router) handleSettingsCommand(ctx context.Context, h *handler.SettingsHandler, cmdCtx CommandContext) error {
	req := handler.SettingsRequest{
		TelegramID: cmdCtx.TelegramID,
		ChatID:     cmdCtx.ChatID,
		MessageID:  cmdCtx.MessageID,
	}

	resp, err := h.Handle(ctx, req)
	if err != nil {
		return err
	}

	return r.sendResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, resp.Text, resp.ParseMode, resp.Keyboard)
}

// ══════════════════════════════════════════════════════════════════════════════
// CALLBACK HANDLER FACTORY METHODS
// Create callback handlers for inline keyboard interactions.
// ══════════════════════════════════════════════════════════════════════════════

// createCommandCallbackHandler creates a handler for "cmd:" callbacks.
// These are callbacks that trigger commands (e.g., "cmd:me", "cmd:top").
func (r *Router) createCommandCallbackHandler() func(ctx context.Context, cbCtx CallbackContext) error {
	return func(ctx context.Context, cbCtx CallbackContext) error {
		// Extract command from callback data: "cmd:me" -> "me"
		parts := strings.SplitN(cbCtx.Data, ":", 2)
		if len(parts) < 2 {
			return nil
		}

		command := parts[1]

		// Convert to command context and route
		cmdCtx := CommandContext{
			TelegramID: cbCtx.TelegramID,
			ChatID:     cbCtx.ChatID,
			MessageID:  cbCtx.MessageID,
			Args:       "",
			Client:     cbCtx.Client,
		}

		// Get the handler and execute with edit
		return r.HandleCommandWithEdit(ctx, command, cmdCtx)
	}
}

// HandleCommandWithEdit handles a command but edits existing message instead of sending new.
func (r *Router) HandleCommandWithEdit(ctx context.Context, command string, cmdCtx CommandContext) error {
	r.commandHandlersMu.RLock()
	h, ok := r.commandHandlers[command]
	r.commandHandlersMu.RUnlock()

	if !ok {
		return nil
	}

	// Special handling for editing vs sending
	switch handler := h.(type) {
	case *handler.MeHandler:
		req := handler.MeRequest{
			TelegramID: cmdCtx.TelegramID,
			ChatID:     cmdCtx.ChatID,
			MessageID:  cmdCtx.MessageID,
			IsRefresh:  true,
		}
		resp, err := handler.Handle(ctx, req)
		if err != nil {
			return err
		}
		return r.editResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, cmdCtx.MessageID, resp.Text, resp.ParseMode, resp.Keyboard)

	case *handler.TopHandler:
		req := handler.TopRequest{
			TelegramID: cmdCtx.TelegramID,
			ChatID:     cmdCtx.ChatID,
			MessageID:  cmdCtx.MessageID,
			Page:       1,
		}
		resp, err := handler.Handle(ctx, req)
		if err != nil {
			return err
		}
		return r.editResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, cmdCtx.MessageID, resp.Text, resp.ParseMode, resp.Keyboard)

	case *handler.NeighborsHandler:
		req := handler.NeighborsRequest{
			TelegramID: cmdCtx.TelegramID,
			ChatID:     cmdCtx.ChatID,
			MessageID:  cmdCtx.MessageID,
		}
		resp, err := handler.Handle(ctx, req)
		if err != nil {
			return err
		}
		return r.editResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, cmdCtx.MessageID, resp.Text, resp.ParseMode, resp.Keyboard)

	case *handler.OnlineHandler:
		req := handler.OnlineRequest{
			TelegramID: cmdCtx.TelegramID,
			ChatID:     cmdCtx.ChatID,
			MessageID:  cmdCtx.MessageID,
		}
		resp, err := handler.Handle(ctx, req)
		if err != nil {
			return err
		}
		return r.editResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, cmdCtx.MessageID, resp.Text, resp.ParseMode, resp.Keyboard)

	case *handler.SettingsHandler:
		req := handler.SettingsRequest{
			TelegramID: cmdCtx.TelegramID,
			ChatID:     cmdCtx.ChatID,
			MessageID:  cmdCtx.MessageID,
		}
		resp, err := handler.Handle(ctx, req)
		if err != nil {
			return err
		}
		return r.editResponse(ctx, cmdCtx.Client, cmdCtx.ChatID, cmdCtx.MessageID, resp.Text, resp.ParseMode, resp.Keyboard)

	default:
		return r.executeCommandHandler(ctx, h, command, cmdCtx)
	}
}

// createRefreshCallbackHandler creates a handler for "refresh:" callbacks.
func (r *Router) createRefreshCallbackHandler() func(ctx context.Context, cbCtx CallbackContext) error {
	return func(ctx context.Context, cbCtx CallbackContext) error {
		// Extract what to refresh: "refresh:me", "refresh:neighbors"
		parts := strings.SplitN(cbCtx.Data, ":", 2)
		if len(parts) < 2 {
			return nil
		}

		target := parts[1]
		cmdCtx := CommandContext{
			TelegramID: cbCtx.TelegramID,
			ChatID:     cbCtx.ChatID,
			MessageID:  cbCtx.MessageID,
			Client:     cbCtx.Client,
		}

		return r.HandleCommandWithEdit(ctx, target, cmdCtx)
	}
}

// createTopCallbackHandler creates a handler for "top:" callbacks (pagination, filtering).
func (r *Router) createTopCallbackHandler(topHandler *handler.TopHandler) func(ctx context.Context, cbCtx CallbackContext) error {
	return func(ctx context.Context, cbCtx CallbackContext) error {
		// Parse callback data: "top:page:2:cohort:true" or "top:filter:1:cohort:false"
		parts := strings.Split(cbCtx.Data, ":")
		if len(parts) < 2 {
			return nil
		}

		action := parts[1]
		page := 1
		cohort := ""
		onlyOnline := false

		switch action {
		case "page", "refresh", "filter":
			if len(parts) >= 3 {
				page, _ = strconv.Atoi(parts[2])
			}
			if len(parts) >= 4 {
				cohort = parts[3]
			}
			if len(parts) >= 5 {
				onlyOnline = parts[4] == "true"
			}
		}

		if page < 1 {
			page = 1
		}

		req := handler.TopRequest{
			TelegramID: cbCtx.TelegramID,
			ChatID:     cbCtx.ChatID,
			MessageID:  cbCtx.MessageID,
			Page:       page,
			Cohort:     cohort,
			OnlyOnline: onlyOnline,
		}

		resp, err := topHandler.Handle(ctx, req)
		if err != nil {
			return err
		}

		return r.editResponse(ctx, cbCtx.Client, cbCtx.ChatID, cbCtx.MessageID, resp.Text, resp.ParseMode, resp.Keyboard)
	}
}

// createOnlineCallbackHandler creates a handler for "online:" callbacks.
func (r *Router) createOnlineCallbackHandler(onlineHandler *handler.OnlineHandler) func(ctx context.Context, cbCtx CallbackContext) error {
	return func(ctx context.Context, cbCtx CallbackContext) error {
		// Parse: "online:away:true:false", "online:helpers:false:true", "online:refresh:true:false"
		parts := strings.Split(cbCtx.Data, ":")
		if len(parts) < 2 {
			return nil
		}

		includeAway := false
		onlyHelpers := false

		if len(parts) >= 3 {
			includeAway = parts[2] == "true"
		}
		if len(parts) >= 4 {
			onlyHelpers = parts[3] == "true"
		}

		req := handler.OnlineRequest{
			TelegramID:  cbCtx.TelegramID,
			ChatID:      cbCtx.ChatID,
			MessageID:   cbCtx.MessageID,
			IncludeAway: includeAway,
			OnlyHelpers: onlyHelpers,
		}

		resp, err := onlineHandler.Handle(ctx, req)
		if err != nil {
			return err
		}

		return r.editResponse(ctx, cbCtx.Client, cbCtx.ChatID, cbCtx.MessageID, resp.Text, resp.ParseMode, resp.Keyboard)
	}
}

// createSettingsCallbackHandler creates a handler for "settings:" callbacks.
func (r *Router) createSettingsCallbackHandler(settingsHandler *handler.SettingsHandler) func(ctx context.Context, cbCtx CallbackContext) error {
	return func(ctx context.Context, cbCtx CallbackContext) error {
		// Parse: "settings:toggle:rank_changes", "settings:quiet:22:8", "settings:enable_all"
		parts := strings.Split(cbCtx.Data, ":")
		if len(parts) < 2 {
			return nil
		}

		action := parts[1]

		req := handler.SettingsRequest{
			TelegramID: cbCtx.TelegramID,
			ChatID:     cbCtx.ChatID,
			MessageID:  cbCtx.MessageID,
		}

		switch action {
		case "toggle":
			if len(parts) >= 3 {
				req.Action = "toggle"
				req.ToggleKey = parts[2]
			}
		case "quiet":
			if len(parts) >= 4 {
				req.Action = "quiet_hours"
				req.QuietStart, _ = strconv.Atoi(parts[2])
				req.QuietEnd, _ = strconv.Atoi(parts[3])
			}
		case "quiet_hours":
			req.Action = "show_quiet_hours"
		case "enable_all":
			req.Action = "enable_all"
		case "disable_all":
			req.Action = "disable_all"
		case "reset":
			req.Action = "reset"
		}

		resp, err := settingsHandler.Handle(ctx, req)
		if err != nil {
			return err
		}

		return r.editResponse(ctx, cbCtx.Client, cbCtx.ChatID, cbCtx.MessageID, resp.Text, resp.ParseMode, resp.Keyboard)
	}
}

// createHelpCallbackHandler creates a handler for "help:" callbacks.
func (r *Router) createHelpCallbackHandler(helpHandler *handler.HelpHandler) func(ctx context.Context, cbCtx CallbackContext) error {
	return func(ctx context.Context, cbCtx CallbackContext) error {
		// Parse: "help:refresh:task_id"
		parts := strings.Split(cbCtx.Data, ":")
		if len(parts) < 3 {
			return nil
		}

		taskID := parts[2]

		req := handler.HelpRequest{
			TelegramID: cbCtx.TelegramID,
			ChatID:     cbCtx.ChatID,
			MessageID:  cbCtx.MessageID,
			TaskID:     taskID,
		}

		resp, err := helpHandler.Handle(ctx, req)
		if err != nil {
			return err
		}

		return r.editResponse(ctx, cbCtx.Client, cbCtx.ChatID, cbCtx.MessageID, resp.Text, resp.ParseMode, resp.Keyboard)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// TEXT INPUT HANDLING
// ══════════════════════════════════════════════════════════════════════════════

// handleOnboardingInput handles text input during onboarding (Alem login).
func (r *Router) handleOnboardingInput(ctx context.Context, inputCtx TextInputContext) error {
	r.commandHandlersMu.RLock()
	h, ok := r.commandHandlers["start"]
	r.commandHandlersMu.RUnlock()

	if !ok {
		return nil
	}

	startHandler, ok := h.(*handler.StartHandler)
	if !ok {
		return nil
	}

	req := handler.StartRequest{
		TelegramID:       inputCtx.TelegramID,
		ChatID:           inputCtx.ChatID,
		MessageID:        inputCtx.MessageID,
		TelegramUsername: "",
		FirstName:        "",
		LastName:         "",
	}

	if inputCtx.Message != nil && inputCtx.Message.From != nil {
		req.TelegramUsername = inputCtx.Message.From.Username
		req.FirstName = inputCtx.Message.From.FirstName
		req.LastName = inputCtx.Message.From.LastName
	}

	resp, err := startHandler.HandleTextMessage(ctx, req, inputCtx.Text)
	if err != nil {
		return err
	}

	return r.sendResponse(ctx, inputCtx.Client, inputCtx.ChatID, resp.Text, resp.ParseMode, resp.Keyboard)
}

// ══════════════════════════════════════════════════════════════════════════════
// DEFAULT HANDLERS
// ══════════════════════════════════════════════════════════════════════════════

// handleUnknownCommand handles commands that don't have a registered handler.
func (r *Router) handleUnknownCommand(ctx context.Context, cmdCtx CommandContext) error {
	text := "❓ <b>Неизвестная команда</b>\n\n" +
		"Доступные команды:\n" +
		"• /me — твоя карточка\n" +
		"• /top — лидерборд\n" +
		"• /neighbors — соседи по рангу\n" +
		"• /online — кто сейчас онлайн\n" +
		"• /help [задача] — найти помощь\n" +
		"• /settings — настройки"

	_, err := cmdCtx.Client.SendHTML(ctx, cmdCtx.ChatID, text)
	return err
}

// handleUnknownCallback handles callbacks that don't have a registered handler.
func (r *Router) handleUnknownCallback(ctx context.Context, cbCtx CallbackContext) error {
	// Just log it, don't send a message to avoid spam
	r.logger.Warn("unknown callback", "data", cbCtx.Data)
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// RESPONSE HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// sendResponse sends a new message with optional inline keyboard.
func (r *Router) sendResponse(
	ctx context.Context,
	client *telegram.Client,
	chatID int64,
	text, parseMode string,
	keyboard *presenter.InlineKeyboard,
) error {
	params := telegram.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: parseMode,
	}

	if keyboard != nil {
		params.ReplyMarkup = convertKeyboard(keyboard)
	}

	_, err := client.SendMessage(ctx, params)
	return err
}

// editResponse edits an existing message with optional inline keyboard.
func (r *Router) editResponse(
	ctx context.Context,
	client *telegram.Client,
	chatID int64,
	messageID int,
	text, parseMode string,
	keyboard *presenter.InlineKeyboard,
) error {
	var kb *telegram.InlineKeyboardMarkup
	if keyboard != nil {
		kb = convertKeyboard(keyboard)
	}

	_, err := client.EditMessageText(ctx, chatID, int64(messageID), text, parseMode, kb)
	return err
}

// convertKeyboard converts presenter.InlineKeyboard to telegram.InlineKeyboardMarkup.
func convertKeyboard(kb *presenter.InlineKeyboard) *telegram.InlineKeyboardMarkup {
	if kb == nil || len(kb.Rows) == 0 {
		return nil
	}

	markup := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: make([][]telegram.InlineKeyboardButton, len(kb.Rows)),
	}

	for i, row := range kb.Rows {
		markup.InlineKeyboard[i] = make([]telegram.InlineKeyboardButton, len(row))
		for j, btn := range row {
			markup.InlineKeyboard[i][j] = telegram.InlineKeyboardButton{
				Text:         btn.Text,
				CallbackData: btn.CallbackData,
				URL:          btn.URL,
			}
		}
	}

	return markup
}

// ══════════════════════════════════════════════════════════════════════════════
// ROUTE INFO (for introspection)
// ══════════════════════════════════════════════════════════════════════════════

// GetRegisteredCommands returns a list of registered command names.
func (r *Router) GetRegisteredCommands() []string {
	r.commandHandlersMu.RLock()
	defer r.commandHandlersMu.RUnlock()

	commands := make([]string, 0, len(r.commandHandlers))
	for cmd := range r.commandHandlers {
		commands = append(commands, cmd)
	}
	return commands
}

// GetRegisteredCallbackPrefixes returns a list of registered callback prefixes.
func (r *Router) GetRegisteredCallbackPrefixes() []string {
	r.callbackPrefixHandlersMu.RLock()
	defer r.callbackPrefixHandlersMu.RUnlock()

	prefixes := make([]string, 0, len(r.callbackPrefixHandlers))
	for prefix := range r.callbackPrefixHandlers {
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}
