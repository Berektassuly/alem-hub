// Package telegram implements Telegram Bot API wrapper.
// This package provides a clean interface for sending messages, handling updates,
// and managing inline keyboards for the Alem Community Hub bot.
package telegram

import (
	"github.com/alem-hub/alem-community-hub/internal/domain/notification"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// CONFIGURATION
// ══════════════════════════════════════════════════════════════════════════════

// ClientConfig contains configuration for the Telegram client.
type ClientConfig struct {
	// Token is the Telegram Bot API token
	Token string

	// BaseURL is the Telegram Bot API base URL (default: https://api.telegram.org)
	BaseURL string

	// Timeout is the HTTP request timeout
	Timeout time.Duration

	// RetryAttempts is the number of retry attempts for failed requests
	RetryAttempts int

	// RetryDelay is the initial delay between retries
	RetryDelay time.Duration

	// Logger for structured logging
	Logger *slog.Logger

	// Debug enables debug logging
	Debug bool
}

// DefaultClientConfig returns sensible defaults.
func DefaultClientConfig(token string) ClientConfig {
	return ClientConfig{
		Token:         token,
		BaseURL:       "https://api.telegram.org",
		Timeout:       60 * time.Second, // Must be > polling timeout (30s) + network latency
		RetryAttempts: 3,
		RetryDelay:    1 * time.Second,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// TELEGRAM API TYPES
// ══════════════════════════════════════════════════════════════════════════════

// Update represents a Telegram update.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
	EditedMessage *Message       `json:"edited_message,omitempty"`
}

// Message represents a Telegram message.
type Message struct {
	MessageID int64           `json:"message_id"`
	From      *User           `json:"from,omitempty"`
	Chat      *Chat           `json:"chat"`
	Date      int64           `json:"date"`
	Text      string          `json:"text,omitempty"`
	Entities  []MessageEntity `json:"entities,omitempty"`

	// Reply information
	ReplyToMessage *Message `json:"reply_to_message,omitempty"`
}

// User represents a Telegram user.
type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// FullName returns the user's full name.
func (u *User) FullName() string {
	if u.LastName != "" {
		return u.FirstName + " " + u.LastName
	}
	return u.FirstName
}

// Chat represents a Telegram chat.
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// MessageEntity represents a message entity (command, mention, etc.).
type MessageEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	User   *User  `json:"user,omitempty"`
}

// CallbackQuery represents a callback query from an inline keyboard.
type CallbackQuery struct {
	ID              string   `json:"id"`
	From            *User    `json:"from"`
	Message         *Message `json:"message,omitempty"`
	InlineMessageID string   `json:"inline_message_id,omitempty"`
	Data            string   `json:"data,omitempty"`
}

// InlineKeyboardMarkup represents an inline keyboard.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// InlineKeyboardButton represents a button in an inline keyboard.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// APIResponse represents a Telegram API response.
type APIResponse struct {
	OK          bool                `json:"ok"`
	Result      json.RawMessage     `json:"result,omitempty"`
	Description string              `json:"description,omitempty"`
	ErrorCode   int                 `json:"error_code,omitempty"`
	Parameters  *ResponseParameters `json:"parameters,omitempty"`
}

// ResponseParameters contains additional error parameters.
type ResponseParameters struct {
	MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
	RetryAfter      int   `json:"retry_after,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// CLIENT
// ══════════════════════════════════════════════════════════════════════════════

// Client is the Telegram Bot API client.
type Client struct {
	config     ClientConfig
	httpClient *http.Client
	logger     *slog.Logger

	// Update handling
	updateOffset int64
	updateMu     sync.Mutex
}

// NewClient creates a new Telegram client.
func NewClient(config ClientConfig) *Client {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://api.telegram.org"
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger: config.Logger,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// SENDING MESSAGES
// ══════════════════════════════════════════════════════════════════════════════

// SendMessageParams contains parameters for sending a message.
type SendMessageParams struct {
	ChatID              int64
	Text                string
	ParseMode           string // "HTML", "Markdown", "MarkdownV2"
	DisableNotification bool
	DisableWebPreview   bool
	ReplyToMessageID    int64
	ReplyMarkup         *InlineKeyboardMarkup
}

// SendMessage sends a text message.
func (c *Client) SendMessage(ctx context.Context, params SendMessageParams) (*Message, error) {
	body := map[string]interface{}{
		"chat_id": params.ChatID,
		"text":    params.Text,
	}

	if params.ParseMode != "" {
		body["parse_mode"] = params.ParseMode
	}
	if params.DisableNotification {
		body["disable_notification"] = true
	}
	if params.DisableWebPreview {
		body["disable_web_page_preview"] = true
	}
	if params.ReplyToMessageID > 0 {
		body["reply_to_message_id"] = params.ReplyToMessageID
	}
	if params.ReplyMarkup != nil {
		body["reply_markup"] = params.ReplyMarkup
	}

	var message Message
	if err := c.callAPI(ctx, "sendMessage", body, &message); err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	return &message, nil
}

// SendText is a convenience method for sending plain text.
func (c *Client) SendText(ctx context.Context, chatID int64, text string) (*Message, error) {
	return c.SendMessage(ctx, SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
}

// SendHTML sends an HTML-formatted message.
func (c *Client) SendHTML(ctx context.Context, chatID int64, html string) (*Message, error) {
	return c.SendMessage(ctx, SendMessageParams{
		ChatID:    chatID,
		Text:      html,
		ParseMode: "HTML",
	})
}

// SendMarkdown sends a Markdown-formatted message.
func (c *Client) SendMarkdown(ctx context.Context, chatID int64, markdown string) (*Message, error) {
	return c.SendMessage(ctx, SendMessageParams{
		ChatID:    chatID,
		Text:      markdown,
		ParseMode: "MarkdownV2",
	})
}

// SendWithKeyboard sends a message with an inline keyboard.
func (c *Client) SendWithKeyboard(ctx context.Context, chatID int64, text string, keyboard [][]InlineKeyboardButton) (*Message, error) {
	return c.SendMessage(ctx, SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
		ReplyMarkup: &InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})
}

// SendSilent sends a silent message (no notification).
func (c *Client) SendSilent(ctx context.Context, chatID int64, text string) (*Message, error) {
	return c.SendMessage(ctx, SendMessageParams{
		ChatID:              chatID,
		Text:                text,
		DisableNotification: true,
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// EDITING MESSAGES
// ══════════════════════════════════════════════════════════════════════════════

// EditMessageText edits the text of a message.
func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, parseMode string, keyboard *InlineKeyboardMarkup) (*Message, error) {
	body := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}

	if parseMode != "" {
		body["parse_mode"] = parseMode
	}
	if keyboard != nil {
		body["reply_markup"] = keyboard
	}

	var message Message
	if err := c.callAPI(ctx, "editMessageText", body, &message); err != nil {
		return nil, fmt.Errorf("edit message text: %w", err)
	}

	return &message, nil
}

// EditMessageKeyboard edits only the inline keyboard of a message.
func (c *Client) EditMessageKeyboard(ctx context.Context, chatID int64, messageID int64, keyboard *InlineKeyboardMarkup) (*Message, error) {
	body := map[string]interface{}{
		"chat_id":      chatID,
		"message_id":   messageID,
		"reply_markup": keyboard,
	}

	var message Message
	if err := c.callAPI(ctx, "editMessageReplyMarkup", body, &message); err != nil {
		return nil, fmt.Errorf("edit message keyboard: %w", err)
	}

	return &message, nil
}

// DeleteMessage deletes a message.
func (c *Client) DeleteMessage(ctx context.Context, chatID int64, messageID int64) error {
	body := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
	}

	var result bool
	if err := c.callAPI(ctx, "deleteMessage", body, &result); err != nil {
		return fmt.Errorf("delete message: %w", err)
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// CALLBACK QUERIES
// ══════════════════════════════════════════════════════════════════════════════

// AnswerCallbackQuery answers a callback query.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string, showAlert bool) error {
	body := map[string]interface{}{
		"callback_query_id": callbackQueryID,
	}

	if text != "" {
		body["text"] = text
		body["show_alert"] = showAlert
	}

	var result bool
	if err := c.callAPI(ctx, "answerCallbackQuery", body, &result); err != nil {
		return fmt.Errorf("answer callback query: %w", err)
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// GETTING UPDATES
// ══════════════════════════════════════════════════════════════════════════════

// GetUpdates fetches updates using long polling.
func (c *Client) GetUpdates(ctx context.Context, offset int64, limit int, timeout int) ([]Update, error) {
	body := map[string]interface{}{
		"timeout": timeout,
	}

	if offset > 0 {
		body["offset"] = offset
	}
	if limit > 0 {
		body["limit"] = limit
	}

	var updates []Update
	if err := c.callAPI(ctx, "getUpdates", body, &updates); err != nil {
		return nil, fmt.Errorf("get updates: %w", err)
	}

	return updates, nil
}

// SetWebhook sets a webhook for receiving updates.
func (c *Client) SetWebhook(ctx context.Context, url string, maxConnections int, allowedUpdates []string) error {
	body := map[string]interface{}{
		"url": url,
	}

	if maxConnections > 0 {
		body["max_connections"] = maxConnections
	}
	if len(allowedUpdates) > 0 {
		body["allowed_updates"] = allowedUpdates
	}

	var result bool
	if err := c.callAPI(ctx, "setWebhook", body, &result); err != nil {
		return fmt.Errorf("set webhook: %w", err)
	}

	return nil
}

// DeleteWebhook removes the webhook.
func (c *Client) DeleteWebhook(ctx context.Context, dropPendingUpdates bool) error {
	body := map[string]interface{}{
		"drop_pending_updates": dropPendingUpdates,
	}

	var result bool
	if err := c.callAPI(ctx, "deleteWebhook", body, &result); err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// BOT INFO
// ══════════════════════════════════════════════════════════════════════════════

// GetMe returns information about the bot.
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	var user User
	if err := c.callAPI(ctx, "getMe", nil, &user); err != nil {
		return nil, fmt.Errorf("get me: %w", err)
	}

	return &user, nil
}

// GetChat returns information about a chat.
func (c *Client) GetChat(ctx context.Context, chatID int64) (*Chat, error) {
	body := map[string]interface{}{
		"chat_id": chatID,
	}

	var chat Chat
	if err := c.callAPI(ctx, "getChat", body, &chat); err != nil {
		return nil, fmt.Errorf("get chat: %w", err)
	}

	return &chat, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// NOTIFICATION CHANNEL IMPLEMENTATION
// ══════════════════════════════════════════════════════════════════════════════

// Send implements notification.NotificationChannel interface.
func (c *Client) Send(ctx context.Context, notif *notification.Notification, opts notification.DeliveryOptions) notification.DeliveryResult {
	// Validate recipient
	if notif.TelegramChatID == 0 {
		return notification.NewFailureResult(
			notification.ChannelTypeTelegram,
			notification.ErrUnsupportedRecipient,
			false,
		)
	}

	// Build keyboard if provided
	var keyboard *InlineKeyboardMarkup
	if len(opts.InlineKeyboard) > 0 {
		keyboard = c.buildKeyboard(opts.InlineKeyboard)
	}

	// Send message
	msg, err := c.SendMessage(ctx, SendMessageParams{
		ChatID:              int64(notif.TelegramChatID),
		Text:                notif.Message,
		ParseMode:           opts.ParseMode,
		DisableNotification: opts.DisableNotification,
		DisableWebPreview:   opts.DisableWebPagePreview,
		ReplyMarkup:         keyboard,
	})
	if err != nil {
		// Check for specific errors
		retryable := c.isRetryableError(err)
		result := notification.NewFailureResult(notification.ChannelTypeTelegram, err, retryable)

		// Check for blocked/not found
		if c.isChatNotFound(err) {
			result.Error = notification.ErrChatNotFound
			result.Retryable = false
		} else if c.isUserBlocked(err) {
			result.Error = notification.ErrRecipientBlocked
			result.Retryable = false
		}

		return result
	}

	return notification.NewSuccessResult(
		notification.ChannelTypeTelegram,
		strconv.FormatInt(msg.MessageID, 10),
	)
}

// Type returns the channel type.
func (c *Client) Type() notification.ChannelType {
	return notification.ChannelTypeTelegram
}

// IsHealthy checks if the channel is healthy.
func (c *Client) IsHealthy(ctx context.Context) bool {
	_, err := c.GetMe(ctx)
	return err == nil
}

// SupportsRecipient checks if a notification can be sent to the recipient.
func (c *Client) SupportsRecipient(notif *notification.Notification) bool {
	return notif.TelegramChatID != 0
}

// buildKeyboard converts domain buttons to Telegram buttons.
func (c *Client) buildKeyboard(buttons [][]notification.InlineButton) *InlineKeyboardMarkup {
	keyboard := make([][]InlineKeyboardButton, len(buttons))
	for i, row := range buttons {
		keyboard[i] = make([]InlineKeyboardButton, len(row))
		for j, btn := range row {
			keyboard[i][j] = InlineKeyboardButton{
				Text:         btn.Text,
				CallbackData: btn.CallbackData,
				URL:          btn.URL,
			}
		}
	}
	return &InlineKeyboardMarkup{InlineKeyboard: keyboard}
}

// ══════════════════════════════════════════════════════════════════════════════
// KEYBOARD BUILDERS
// ══════════════════════════════════════════════════════════════════════════════

// KeyboardBuilder helps build inline keyboards fluently.
type KeyboardBuilder struct {
	rows [][]InlineKeyboardButton
}

// NewKeyboard creates a new keyboard builder.
func NewKeyboard() *KeyboardBuilder {
	return &KeyboardBuilder{
		rows: make([][]InlineKeyboardButton, 0),
	}
}

// Row adds a new row of buttons.
func (kb *KeyboardBuilder) Row(buttons ...InlineKeyboardButton) *KeyboardBuilder {
	kb.rows = append(kb.rows, buttons)
	return kb
}

// Button creates a callback button.
func Button(text, callbackData string) InlineKeyboardButton {
	return InlineKeyboardButton{
		Text:         text,
		CallbackData: callbackData,
	}
}

// URLButton creates a URL button.
func URLButton(text, url string) InlineKeyboardButton {
	return InlineKeyboardButton{
		Text: text,
		URL:  url,
	}
}

// Build returns the inline keyboard markup.
func (kb *KeyboardBuilder) Build() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: kb.rows}
}

// ══════════════════════════════════════════════════════════════════════════════
// API CALL HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// callAPI makes a call to the Telegram Bot API with retries.
func (c *Client) callAPI(ctx context.Context, method string, body map[string]interface{}, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			delay := c.config.RetryDelay * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := c.doAPICall(ctx, method, body, result)
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry on non-retryable errors
		if !c.isRetryableError(err) {
			return err
		}

		// Handle rate limiting
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(apiErr.RetryAfter) * time.Second):
			}
		}
	}

	return fmt.Errorf("api call failed after %d retries: %w", c.config.RetryAttempts, lastErr)
}

// doAPICall performs a single API call.
func (c *Client) doAPICall(ctx context.Context, method string, body map[string]interface{}, result interface{}) error {
	url := fmt.Sprintf("%s/bot%s/%s", c.config.BaseURL, c.config.Token, method)

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if c.config.Debug {
		c.logger.Debug("telegram api call", "method", method)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if !apiResp.OK {
		apiErr := &APIError{
			Code:        apiResp.ErrorCode,
			Description: apiResp.Description,
		}
		if apiResp.Parameters != nil {
			apiErr.RetryAfter = apiResp.Parameters.RetryAfter
		}
		return apiErr
	}

	if result != nil && len(apiResp.Result) > 0 {
		if err := json.Unmarshal(apiResp.Result, result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// ERRORS
// ══════════════════════════════════════════════════════════════════════════════

// APIError represents a Telegram API error.
type APIError struct {
	Code        int
	Description string
	RetryAfter  int
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("telegram api error %d: %s", e.Code, e.Description)
}

// isRetryableError checks if an error is retryable.
func (c *Client) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		// Rate limited - retryable
		if apiErr.Code == 429 {
			return true
		}
		// Server errors - retryable
		if apiErr.Code >= 500 {
			return true
		}
		// Client errors - generally not retryable
		if apiErr.Code >= 400 && apiErr.Code < 500 {
			return false
		}
	}

	// Network errors are retryable
	errStr := err.Error()
	return containsAny(errStr, []string{"timeout", "connection refused", "temporary", "reset"})
}

// isChatNotFound checks if the error indicates chat not found.
func (c *Client) isChatNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == 400 && containsAny(apiErr.Description, []string{
			"chat not found",
			"CHAT_NOT_FOUND",
		})
	}
	return false
}

// isUserBlocked checks if the error indicates user blocked the bot.
func (c *Client) isUserBlocked(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == 403 || containsAny(apiErr.Description, []string{
			"bot was blocked",
			"user is deactivated",
			"BLOCKED_BY_USER",
		})
	}
	return false
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if containsStr(s, sub) {
			return true
		}
	}
	return false
}

// containsStr checks if s contains substr (case-insensitive-ish).
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && findStr(s, substr) >= 0
}

// findStr finds substr in s.
func findStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ══════════════════════════════════════════════════════════════════════════════
// LONG POLLING RUNNER
// ══════════════════════════════════════════════════════════════════════════════

// UpdateHandler is a function that handles a Telegram update.
type UpdateHandler func(ctx context.Context, update *Update) error

// StartPolling starts long polling for updates.
func (c *Client) StartPolling(ctx context.Context, handler UpdateHandler) error {
	c.logger.Info("starting telegram long polling")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("stopping telegram long polling")
			return ctx.Err()
		default:
		}

		c.updateMu.Lock()
		offset := c.updateOffset
		c.updateMu.Unlock()

		updates, err := c.GetUpdates(ctx, offset, 100, 30)
		if err != nil {
			// Don't log context cancellation
			if errors.Is(err, context.Canceled) {
				return nil
			}
			c.logger.Error("failed to get updates", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			// Update offset
			c.updateMu.Lock()
			if update.UpdateID >= c.updateOffset {
				c.updateOffset = update.UpdateID + 1
			}
			c.updateMu.Unlock()

			// Handle update
			if err := handler(ctx, &update); err != nil {
				c.logger.Error("failed to handle update",
					"update_id", update.UpdateID,
					"error", err,
				)
			}
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// UTILITY METHODS
// ══════════════════════════════════════════════════════════════════════════════

// ExtractCommand extracts the command from a message (without the /).
func ExtractCommand(msg *Message) string {
	if msg == nil || msg.Text == "" {
		return ""
	}

	for _, entity := range msg.Entities {
		if entity.Type == "bot_command" && entity.Offset == 0 {
			cmd := msg.Text[1:entity.Length] // Skip the /
			// Remove bot username if present (@botname)
			for i, c := range cmd {
				if c == '@' {
					return cmd[:i]
				}
			}
			return cmd
		}
	}

	return ""
}

// ExtractCommandArgs extracts arguments after the command.
func ExtractCommandArgs(msg *Message) string {
	if msg == nil || msg.Text == "" {
		return ""
	}

	for _, entity := range msg.Entities {
		if entity.Type == "bot_command" && entity.Offset == 0 {
			// Return everything after the command
			if entity.Length < len(msg.Text) {
				args := msg.Text[entity.Length:]
				// Trim leading space
				if len(args) > 0 && args[0] == ' ' {
					return args[1:]
				}
				return args
			}
		}
	}

	return ""
}

// IsPrivateChat checks if the message is from a private chat.
func IsPrivateChat(msg *Message) bool {
	return msg != nil && msg.Chat != nil && msg.Chat.Type == "private"
}

// IsGroupChat checks if the message is from a group chat.
func IsGroupChat(msg *Message) bool {
	if msg == nil || msg.Chat == nil {
		return false
	}
	return msg.Chat.Type == "group" || msg.Chat.Type == "supergroup"
}
