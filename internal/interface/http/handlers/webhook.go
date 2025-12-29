// Package handlers contains HTTP handler interfaces and implementations.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ══════════════════════════════════════════════════════════════════════════════
// TELEGRAM WEBHOOK HANDLER
// ══════════════════════════════════════════════════════════════════════════════

// TelegramUpdate represents a Telegram webhook update.
type TelegramUpdate struct {
	UpdateID      int64                  `json:"update_id"`
	Message       *TelegramMessage       `json:"message,omitempty"`
	CallbackQuery *TelegramCallbackQuery `json:"callback_query,omitempty"`
	InlineQuery   *TelegramInlineQuery   `json:"inline_query,omitempty"`
}

// TelegramMessage represents a Telegram message.
type TelegramMessage struct {
	MessageID int64           `json:"message_id"`
	From      *TelegramUser   `json:"from,omitempty"`
	Chat      *TelegramChat   `json:"chat,omitempty"`
	Date      int64           `json:"date"`
	Text      string          `json:"text,omitempty"`
	Entities  []MessageEntity `json:"entities,omitempty"`
}

// TelegramUser represents a Telegram user.
type TelegramUser struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// TelegramChat represents a Telegram chat.
type TelegramChat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"` // "private", "group", "supergroup", "channel"
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// TelegramCallbackQuery represents a callback query from an inline keyboard button.
type TelegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    *TelegramUser    `json:"from"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data,omitempty"`
}

// TelegramInlineQuery represents an inline query.
type TelegramInlineQuery struct {
	ID     string        `json:"id"`
	From   *TelegramUser `json:"from"`
	Query  string        `json:"query"`
	Offset string        `json:"offset"`
}

// MessageEntity represents a message entity (command, mention, etc).
type MessageEntity struct {
	Type   string `json:"type"` // "bot_command", "mention", "url", etc.
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// ══════════════════════════════════════════════════════════════════════════════
// WEBHOOK HANDLER IMPLEMENTATION
// ══════════════════════════════════════════════════════════════════════════════

// UpdateHandler is a function that handles a specific type of update.
type UpdateHandler func(ctx context.Context, update *TelegramUpdate) error

// CommandHandler is a function that handles a bot command.
type CommandHandler func(ctx context.Context, message *TelegramMessage, args string) error

// CallbackHandler is a function that handles a callback query.
type CallbackHandler func(ctx context.Context, callback *TelegramCallbackQuery) error

// TelegramWebhookHandlerImpl implements WebhookHandler for Telegram.
type TelegramWebhookHandlerImpl struct {
	mu              sync.RWMutex
	commandHandlers map[string]CommandHandler
	callbackHandler CallbackHandler
	defaultHandler  UpdateHandler
	errorHandler    func(error)
}

// NewTelegramWebhookHandler creates a new Telegram webhook handler.
func NewTelegramWebhookHandler() *TelegramWebhookHandlerImpl {
	return &TelegramWebhookHandlerImpl{
		commandHandlers: make(map[string]CommandHandler),
	}
}

// RegisterCommand registers a handler for a specific bot command.
func (h *TelegramWebhookHandlerImpl) RegisterCommand(command string, handler CommandHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commandHandlers[command] = handler
}

// RegisterCallback registers a handler for callback queries.
func (h *TelegramWebhookHandlerImpl) RegisterCallback(handler CallbackHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbackHandler = handler
}

// RegisterDefault registers a default handler for unhandled updates.
func (h *TelegramWebhookHandlerImpl) RegisterDefault(handler UpdateHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.defaultHandler = handler
}

// SetErrorHandler sets the error handler.
func (h *TelegramWebhookHandlerImpl) SetErrorHandler(handler func(error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errorHandler = handler
}

// HandleTelegramUpdate processes a Telegram webhook update.
func (h *TelegramWebhookHandlerImpl) HandleTelegramUpdate(ctx context.Context, payload []byte) error {
	var update TelegramUpdate
	if err := json.Unmarshal(payload, &update); err != nil {
		return fmt.Errorf("failed to parse update: %w", err)
	}

	return h.processUpdate(ctx, &update)
}

// processUpdate routes the update to the appropriate handler.
func (h *TelegramWebhookHandlerImpl) processUpdate(ctx context.Context, update *TelegramUpdate) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var err error

	// Handle callback queries
	if update.CallbackQuery != nil && h.callbackHandler != nil {
		err = h.callbackHandler(ctx, update.CallbackQuery)
		if err != nil {
			h.handleError(err)
		}
		return err
	}

	// Handle messages
	if update.Message != nil {
		// Check if it's a command
		if command, args := h.extractCommand(update.Message); command != "" {
			if handler, ok := h.commandHandlers[command]; ok {
				err = handler(ctx, update.Message, args)
				if err != nil {
					h.handleError(err)
				}
				return err
			}
		}
	}

	// Use default handler if available
	if h.defaultHandler != nil {
		err = h.defaultHandler(ctx, update)
		if err != nil {
			h.handleError(err)
		}
		return err
	}

	return nil
}

// extractCommand extracts the command and arguments from a message.
func (h *TelegramWebhookHandlerImpl) extractCommand(msg *TelegramMessage) (string, string) {
	if msg == nil || msg.Text == "" {
		return "", ""
	}

	for _, entity := range msg.Entities {
		if entity.Type == "bot_command" && entity.Offset == 0 {
			command := msg.Text[entity.Offset : entity.Offset+entity.Length]

			// Remove bot username if present (e.g., /start@mybot)
			if atIndex := indexOf(command, '@'); atIndex != -1 {
				command = command[:atIndex]
			}

			// Extract arguments
			args := ""
			if entity.Offset+entity.Length < len(msg.Text) {
				args = msg.Text[entity.Offset+entity.Length+1:] // +1 for space
			}

			return command, args
		}
	}

	// Check for simple command format (starts with /)
	if len(msg.Text) > 0 && msg.Text[0] == '/' {
		spaceIndex := indexOf(msg.Text, ' ')
		if spaceIndex == -1 {
			return msg.Text, ""
		}
		return msg.Text[:spaceIndex], msg.Text[spaceIndex+1:]
	}

	return "", ""
}

// handleError calls the error handler if set.
func (h *TelegramWebhookHandlerImpl) handleError(err error) {
	if h.errorHandler != nil && err != nil {
		h.errorHandler(err)
	}
}

// indexOf returns the index of the first occurrence of char in s, or -1.
func indexOf(s string, char rune) int {
	for i, c := range s {
		if c == char {
			return i
		}
	}
	return -1
}

// ══════════════════════════════════════════════════════════════════════════════
// WEBHOOK DISPATCHER
// ══════════════════════════════════════════════════════════════════════════════

// WebhookDispatcher dispatches webhooks to multiple handlers.
type WebhookDispatcher struct {
	mu       sync.RWMutex
	handlers map[string]WebhookHandler
}

// NewWebhookDispatcher creates a new webhook dispatcher.
func NewWebhookDispatcher() *WebhookDispatcher {
	return &WebhookDispatcher{
		handlers: make(map[string]WebhookHandler),
	}
}

// Register registers a webhook handler for a specific type.
func (d *WebhookDispatcher) Register(webhookType string, handler WebhookHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[webhookType] = handler
}

// Dispatch dispatches a webhook to the appropriate handler.
func (d *WebhookDispatcher) Dispatch(ctx context.Context, webhookType string, payload []byte) error {
	d.mu.RLock()
	handler, ok := d.handlers[webhookType]
	d.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no handler registered for webhook type: %s", webhookType)
	}

	return handler.HandleTelegramUpdate(ctx, payload)
}

// HandleTelegramUpdate implements WebhookHandler by dispatching to "telegram" handler.
func (d *WebhookDispatcher) HandleTelegramUpdate(ctx context.Context, payload []byte) error {
	return d.Dispatch(ctx, "telegram", payload)
}
