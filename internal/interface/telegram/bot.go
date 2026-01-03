// Package telegram implements Telegram Bot interface for Alem Community Hub.
// This package is the entry point for all Telegram interactions, handling
// updates, routing them to appropriate handlers, and managing the bot lifecycle.
//
// Philosophy: "От конкуренции к сотрудничеству" - The bot is designed to
// foster a supportive community where students help each other succeed.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/alem-hub/alem-community-hub/internal/application/command"
	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"github.com/alem-hub/alem-community-hub/internal/application/saga"
	"github.com/alem-hub/alem-community-hub/internal/domain/student"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/external/telegram"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram/handler"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram/handler/callback"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram/middleware"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram/presenter"
)

// ══════════════════════════════════════════════════════════════════════════════
// BOT CONFIGURATION
// ══════════════════════════════════════════════════════════════════════════════

// BotConfig contains configuration for the Telegram bot.
type BotConfig struct {
	// Token is the Telegram Bot API token.
	Token string

	// Mode is the update receiving mode: "polling" or "webhook".
	Mode string

	// WebhookURL is the URL for webhook mode (required if Mode is "webhook").
	WebhookURL string

	// WebhookPort is the port to listen on for webhook updates.
	WebhookPort int

	// PollingTimeout is the timeout for long polling (in seconds).
	PollingTimeout int

	// Debug enables debug logging.
	Debug bool

	// Logger for structured logging.
	Logger *slog.Logger

	// AllowedUpdates specifies which update types to receive.
	AllowedUpdates []string

	// MaxConcurrentUpdates limits concurrent update processing.
	MaxConcurrentUpdates int

	// GracefulShutdownTimeout is the timeout for graceful shutdown.
	GracefulShutdownTimeout time.Duration
}

// DefaultBotConfig returns sensible defaults.
func DefaultBotConfig(token string) BotConfig {
	return BotConfig{
		Token:                   token,
		Mode:                    "polling",
		PollingTimeout:          30,
		Debug:                   false,
		Logger:                  slog.Default(),
		AllowedUpdates:          []string{"message", "callback_query"},
		MaxConcurrentUpdates:    100,
		GracefulShutdownTimeout: 30 * time.Second,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// BOT DEPENDENCIES
// Aggregates all dependencies needed by handlers.
// ══════════════════════════════════════════════════════════════════════════════

// BotDependencies contains all dependencies for the bot handlers.
type BotDependencies struct {
	// Repositories
	StudentRepo student.Repository

	// Commands
	SyncStudentCmd     *command.SyncStudentHandler
	RequestHelpCmd     *command.RequestHelpHandler
	ConnectStudentsCmd *command.ConnectStudentsHandler
	UpdatePrefsCmd     *command.UpdatePreferencesHandler
	ResetPrefsCmd      *command.ResetPreferencesHandler
	GiveEndorsementCmd *command.GiveEndorsementHandler

	// Queries
	LeaderboardQuery   *query.GetLeaderboardHandler
	StudentRankQuery   *query.GetStudentRankHandler
	NeighborsQuery     *query.GetNeighborsHandler
	FindHelpersQuery   *query.FindHelpersHandler
	OnlineNowQuery     *query.GetOnlineNowHandler
	DailyProgressQuery *query.GetDailyProgressHandler

	// Sagas
	OnboardingSaga *saga.OnboardingSaga
}

// ══════════════════════════════════════════════════════════════════════════════
// BOT
// Main bot structure that orchestrates Telegram interactions.
// ══════════════════════════════════════════════════════════════════════════════

// Bot is the main Telegram bot controller.
type Bot struct {
	config BotConfig
	client *telegram.Client
	router *Router
	logger *slog.Logger

	// Middleware chain
	authMiddleware     *middleware.AuthMiddleware
	rateLimiter        *middleware.RateLimiter
	recoveryMiddleware *middleware.RecoveryMiddleware
	metricsMiddleware  *middleware.MetricsMiddleware

	// Lifecycle management
	running   bool
	runningMu sync.RWMutex
	stopCh    chan struct{}
	updateSem chan struct{} // Semaphore for concurrent update limiting
	wg        sync.WaitGroup

	// Statistics
	stats *BotStats
}

// BotStats holds runtime statistics.
type BotStats struct {
	mu              sync.RWMutex
	StartedAt       time.Time
	UpdatesReceived int64
	UpdatesHandled  int64
	ErrorsCount     int64
	CommandsCount   map[string]int64
}

// NewBot creates a new Telegram bot with all dependencies.
func NewBot(config BotConfig, deps BotDependencies) (*Bot, error) {
	if config.Token == "" {
		return nil, errors.New("telegram token is required")
	}

	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	// Create Telegram client
	clientConfig := telegram.DefaultClientConfig(config.Token)
	clientConfig.Logger = config.Logger
	clientConfig.Debug = config.Debug
	client := telegram.NewClient(clientConfig)

	// Create presenters
	keyboards := presenter.NewKeyboardBuilder()
	cardPresenter := presenter.NewStudentCardPresenter()
	leaderboardPresenter := presenter.NewLeaderboardPresenter()

	// Create handlers
	startHandler := handler.NewStartHandler(
		deps.OnboardingSaga,
		deps.StudentRepo,
		keyboards,
	)

	meHandler := handler.NewMeHandler(
		deps.StudentRankQuery,
		deps.DailyProgressQuery,
		deps.StudentRepo,
		keyboards,
		cardPresenter,
	)

	topHandler := handler.NewTopHandler(
		deps.LeaderboardQuery,
		keyboards,
	)
	_ = leaderboardPresenter // may be used for detailed view later

	neighborsHandler := handler.NewNeighborsHandler(
		deps.NeighborsQuery,
		deps.StudentRepo,
		keyboards,
	)

	onlineHandler := handler.NewOnlineHandler(
		deps.OnlineNowQuery,
		deps.StudentRepo,
		keyboards,
	)

	helpHandler := handler.NewHelpHandler(
		deps.FindHelpersQuery,
		deps.StudentRepo,
		keyboards,
	)

	settingsHandler := handler.NewSettingsHandler(
		deps.UpdatePrefsCmd,
		deps.ResetPrefsCmd,
		deps.StudentRepo,
		keyboards,
	)

	// Create callback handlers
	connectCallback := callback.NewConnectHandler(
		deps.ConnectStudentsCmd,
		deps.StudentRepo,
		keyboards,
	)

	endorseCallback := callback.NewEndorseHandler(
		deps.GiveEndorsementCmd,
		deps.StudentRepo,
		keyboards,
	)

	// Create middleware
	authMiddleware := middleware.NewAuthMiddleware(
		deps.StudentRepo,
		middleware.DefaultAuthConfig(),
	)

	rateLimiter := middleware.NewRateLimiter(
		middleware.DefaultRateLimitConfig(),
	)

	recoveryMiddleware := middleware.NewRecoveryMiddleware(
		middleware.DefaultRecoveryConfig(),
	)

	metricsMiddleware := middleware.NewMetricsMiddleware(
		middleware.DefaultMetricsConfig(),
	)

	// Create router with all handlers
	routerConfig := RouterConfig{
		Logger: config.Logger,
		Debug:  config.Debug,
	}

	router := NewRouter(routerConfig)

	// Register command handlers
	router.RegisterCommand("start", startHandler)
	router.RegisterCommand("me", meHandler)
	router.RegisterCommand("top", topHandler)
	router.RegisterCommand("neighbors", neighborsHandler)
	router.RegisterCommand("online", onlineHandler)
	router.RegisterCommand("help", helpHandler)
	router.RegisterCommand("settings", settingsHandler)

	// Register callback handlers
	router.RegisterCallbackPrefix("connect:", connectCallback)
	router.RegisterCallbackPrefix("endorse:", endorseCallback)
	router.RegisterCallbackPrefix("cmd:", router.createCommandCallbackHandler())
	router.RegisterCallbackPrefix("refresh:", router.createRefreshCallbackHandler())
	router.RegisterCallbackPrefix("top:", router.createTopCallbackHandler(topHandler))
	router.RegisterCallbackPrefix("online:", router.createOnlineCallbackHandler(onlineHandler))
	router.RegisterCallbackPrefix("settings:", router.createSettingsCallbackHandler(settingsHandler))
	router.RegisterCallbackPrefix("help:", router.createHelpCallbackHandler(helpHandler))

	// Create bot
	bot := &Bot{
		config:             config,
		client:             client,
		router:             router,
		logger:             config.Logger,
		authMiddleware:     authMiddleware,
		rateLimiter:        rateLimiter,
		recoveryMiddleware: recoveryMiddleware,
		metricsMiddleware:  metricsMiddleware,
		stopCh:             make(chan struct{}),
		updateSem:          make(chan struct{}, config.MaxConcurrentUpdates),
		stats: &BotStats{
			CommandsCount: make(map[string]int64),
		},
	}

	return bot, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// LIFECYCLE MANAGEMENT
// ══════════════════════════════════════════════════════════════════════════════

// Start starts the bot and begins receiving updates.
func (b *Bot) Start(ctx context.Context) error {
	b.runningMu.Lock()
	if b.running {
		b.runningMu.Unlock()
		return errors.New("bot is already running")
	}
	b.running = true
	b.stats.StartedAt = time.Now()
	b.runningMu.Unlock()

	b.logger.Info("starting telegram bot",
		"mode", b.config.Mode,
		"debug", b.config.Debug,
	)

	// Verify bot token with getMe
	if err := b.verifyToken(ctx); err != nil {
		return fmt.Errorf("failed to verify bot token: %w", err)
	}

	// Start based on mode
	switch b.config.Mode {
	case "polling":
		return b.startPolling(ctx)
	case "webhook":
		return b.startWebhook(ctx)
	default:
		return fmt.Errorf("unknown bot mode: %s", b.config.Mode)
	}
}

// Stop gracefully stops the bot.
func (b *Bot) Stop(ctx context.Context) error {
	b.runningMu.Lock()
	if !b.running {
		b.runningMu.Unlock()
		return nil
	}
	b.running = false
	b.runningMu.Unlock()

	b.logger.Info("stopping telegram bot")

	// Signal stop
	close(b.stopCh)

	// Wait for all handlers to complete with timeout
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		b.logger.Info("all handlers completed gracefully")
	case <-time.After(b.config.GracefulShutdownTimeout):
		b.logger.Warn("graceful shutdown timeout exceeded")
	case <-ctx.Done():
		b.logger.Warn("context cancelled during shutdown")
		return ctx.Err()
	}

	return nil
}

// IsRunning returns whether the bot is currently running.
func (b *Bot) IsRunning() bool {
	b.runningMu.RLock()
	defer b.runningMu.RUnlock()
	return b.running
}

// verifyToken verifies the bot token by calling getMe.
func (b *Bot) verifyToken(ctx context.Context) error {
	me, err := b.client.GetMe(ctx)
	if err != nil {
		return err
	}

	b.logger.Info("bot verified",
		"id", me.ID,
		"username", me.Username,
		"first_name", me.FirstName,
	)

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// POLLING MODE
// ══════════════════════════════════════════════════════════════════════════════

// startPolling starts long polling for updates.
func (b *Bot) startPolling(ctx context.Context) error {
	b.logger.Info("starting long polling")

	return b.client.StartPolling(ctx, func(ctx context.Context, update *telegram.Update) error {
		return b.handleUpdate(ctx, update)
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// WEBHOOK MODE
// ══════════════════════════════════════════════════════════════════════════════

// startWebhook starts the webhook server.
func (b *Bot) startWebhook(ctx context.Context) error {
	if b.config.WebhookURL == "" {
		return errors.New("webhook URL is required for webhook mode")
	}

	b.logger.Info("starting webhook server",
		"url", b.config.WebhookURL,
		"port", b.config.WebhookPort,
	)

	// Set webhook
	err := b.client.SetWebhook(ctx, b.config.WebhookURL, 0, b.config.AllowedUpdates)
	if err != nil {
		return fmt.Errorf("failed to set webhook: %w", err)
	}

	// Start HTTP server for webhook
	// TODO: Implement webhook HTTP server when Client.StartWebhookServer is available
	// For now, webhook mode requires external HTTP server setup
	b.logger.Info("webhook mode configured - ensure external HTTP server routes to handleUpdate")
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// UPDATE HANDLING
// ══════════════════════════════════════════════════════════════════════════════

// handleUpdate processes a single Telegram update.
func (b *Bot) handleUpdate(ctx context.Context, update *telegram.Update) error {
	// Acquire semaphore slot
	select {
	case b.updateSem <- struct{}{}:
		defer func() { <-b.updateSem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	b.wg.Add(1)
	defer b.wg.Done()

	// Update statistics
	b.stats.mu.Lock()
	b.stats.UpdatesReceived++
	b.stats.mu.Unlock()

	// Start timing
	startTime := time.Now()

	// Add context values
	ctx = middleware.ContextWithTelegramID(ctx, b.extractTelegramID(update))
	ctx = context.WithValue(ctx, middleware.StartTimeContextKey, startTime)

	// Determine update type and handle
	var err error
	switch {
	case update.Message != nil:
		err = b.handleMessage(ctx, update.Message)
	case update.CallbackQuery != nil:
		err = b.handleCallbackQuery(ctx, update.CallbackQuery)
	default:
		// Unknown update type - ignore
		return nil
	}

	// Record metrics (using metrics context if available)
	duration := time.Since(startTime)
	_ = duration // metrics tracking done via Start/Finish pattern

	if err != nil {
		b.stats.mu.Lock()
		b.stats.ErrorsCount++
		b.stats.mu.Unlock()
		b.logger.Error("failed to handle update",
			"update_id", update.UpdateID,
			"error", err,
			"duration", duration,
		)
	} else {
		b.stats.mu.Lock()
		b.stats.UpdatesHandled++
		b.stats.mu.Unlock()
	}

	return err
}

// handleMessage processes a Telegram message.
func (b *Bot) handleMessage(ctx context.Context, msg *telegram.Message) error {
	if msg == nil || msg.From == nil {
		return nil
	}

	telegramID := msg.From.ID
	chatID := msg.Chat.ID

	// DEBUG: Log all incoming messages
	b.logger.Info("📨 INCOMING MESSAGE",
		"telegram_id", telegramID,
		"chat_id", chatID,
		"text", msg.Text,
		"from", msg.From.Username,
	)

	// Extract command
	command := telegram.ExtractCommand(msg)
	args := telegram.ExtractCommandArgs(msg)

	// If it's a command
	if command != "" {
		b.logger.Info("📌 COMMAND DETECTED", "command", command, "args", args)
		return b.handleCommand(ctx, telegramID, chatID, int(msg.MessageID), command, args, msg)
	}

	// If it's a text message (might be onboarding input)
	if msg.Text != "" {
		b.logger.Info("📝 TEXT MESSAGE - forwarding to handleTextMessage", "text", msg.Text)
		return b.handleTextMessage(ctx, telegramID, chatID, msg)
	}

	return nil
}

// handleCommand processes a bot command.
func (b *Bot) handleCommand(
	ctx context.Context,
	telegramID, chatID int64,
	messageID int,
	command, args string,
	msg *telegram.Message,
) error {
	// Record command statistics
	b.stats.mu.Lock()
	b.stats.CommandsCount[command]++
	b.stats.mu.Unlock()

	// Rate limiting
	rateLimitResult := b.rateLimiter.Check(ctx, telegramID)
	if !rateLimitResult.Allowed {
		return b.sendRateLimitMessage(ctx, chatID, rateLimitResult.RetryAfter)
	}

	// Authentication
	authResult, err := b.authMiddleware.Authenticate(ctx, telegramID, "/"+command)
	if err != nil {
		b.logger.Error("auth error", "error", err)
		return b.sendErrorMessage(ctx, chatID)
	}

	if !authResult.ShouldContinue {
		_, err := b.client.SendHTML(ctx, chatID, authResult.ResponseMessage)
		return err
	}

	// Add authenticated student to context
	if authResult.Student != nil {
		ctx = middleware.ContextWithStudent(ctx, authResult.Student)
	}

	// Recovery wrapper
	recoveryResult := b.recoveryMiddleware.RecoverWithHandler(ctx, telegramID, command, func() error {
		return b.router.HandleCommand(ctx, command, CommandContext{
			TelegramID: telegramID,
			ChatID:     chatID,
			MessageID:  messageID,
			Args:       args,
			Message:    msg,
			Client:     b.client,
		})
	})

	if recoveryResult.Recovered {
		b.logger.Error("panic recovered in command handler",
			"command", command,
			"telegram_id", telegramID,
		)
		_, err := b.client.SendHTML(ctx, chatID, recoveryResult.UserMessage)
		return err
	}

	return nil
}

// handleTextMessage processes a non-command text message.
func (b *Bot) handleTextMessage(ctx context.Context, telegramID, chatID int64, msg *telegram.Message) error {
	b.logger.Info("🔍 handleTextMessage CALLED",
		"telegram_id", telegramID,
		"text", msg.Text,
	)

	// Check if user is in onboarding state (not registered)
	authResult, err := b.authMiddleware.Authenticate(ctx, telegramID, "")
	if err != nil {
		b.logger.Error("❌ Auth error in handleTextMessage", "error", err)
		return nil // Ignore errors for text messages
	}

	b.logger.Info("🔐 Auth result",
		"is_authenticated", authResult.IsAuthenticated,
		"should_continue", authResult.ShouldContinue,
	)

	// If user is not registered, treat as Alem login input
	if !authResult.IsAuthenticated {
		b.logger.Info("✅ User NOT authenticated - forwarding to HandleTextInput")
		err := b.router.HandleTextInput(ctx, TextInputContext{
			TelegramID: telegramID,
			ChatID:     chatID,
			MessageID:  int(msg.MessageID),
			Text:       msg.Text,
			Message:    msg,
			Client:     b.client,
		})
		if err != nil {
			b.logger.Error("❌ HandleTextInput error", "error", err)
		}
		return err
	}

	// User is registered but sent a text message - might be a query or just chat
	b.logger.Info("⏭️ User IS authenticated - ignoring text message")
	return nil
}

// handleCallbackQuery processes a callback query from inline keyboard.
func (b *Bot) handleCallbackQuery(ctx context.Context, cq *telegram.CallbackQuery) error {
	if cq == nil || cq.From == nil {
		return nil
	}

	telegramID := cq.From.ID
	chatID := int64(0)
	messageID := int64(0)

	if cq.Message != nil {
		chatID = cq.Message.Chat.ID
		messageID = cq.Message.MessageID
	}

	// Answer callback query first (removes loading state)
	defer func() {
		_ = b.client.AnswerCallbackQuery(ctx, cq.ID, "", false)
	}()

	// Rate limiting for callbacks
	rateLimitResult := b.rateLimiter.Check(ctx, telegramID)
	if !rateLimitResult.Allowed {
		_ = b.client.AnswerCallbackQuery(ctx, cq.ID, "⏳ Слишком быстро! Подожди немного.", true)
		return nil
	}

	// Authentication
	authResult, err := b.authMiddleware.Authenticate(ctx, telegramID, "callback")
	if err != nil {
		b.logger.Error("auth error", "error", err)
		return nil
	}

	// Add authenticated student to context
	if authResult.Student != nil {
		ctx = middleware.ContextWithStudent(ctx, authResult.Student)
	}

	// Recovery wrapper
	recoveryResult := b.recoveryMiddleware.RecoverWithHandler(ctx, telegramID, "callback:"+cq.Data, func() error {
		return b.router.HandleCallback(ctx, cq.Data, CallbackContext{
			TelegramID: telegramID,
			ChatID:     chatID,
			MessageID:  int(messageID),
			QueryID:    cq.ID,
			Data:       cq.Data,
			Query:      cq,
			Client:     b.client,
		})
	})

	if recoveryResult.Recovered {
		b.logger.Error("panic recovered in callback handler",
			"data", cq.Data,
			"telegram_id", telegramID,
		)
		if chatID > 0 {
			_, _ = b.client.SendHTML(ctx, chatID, recoveryResult.UserMessage)
		}
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ══════════════════════════════════════════════════════════════════════════════

// extractTelegramID extracts the Telegram user ID from an update.
func (b *Bot) extractTelegramID(update *telegram.Update) int64 {
	if update.Message != nil && update.Message.From != nil {
		return update.Message.From.ID
	}
	if update.CallbackQuery != nil && update.CallbackQuery.From != nil {
		return update.CallbackQuery.From.ID
	}
	return 0
}

// sendRateLimitMessage sends a rate limit warning message.
func (b *Bot) sendRateLimitMessage(ctx context.Context, chatID int64, waitTime time.Duration) error {
	text := fmt.Sprintf("⏳ Слишком много запросов!\nПопробуй через %d секунд.", int(waitTime.Seconds()))
	_, err := b.client.SendHTML(ctx, chatID, text)
	return err
}

// sendErrorMessage sends a generic error message.
func (b *Bot) sendErrorMessage(ctx context.Context, chatID int64) error {
	text := "😔 Произошла ошибка. Попробуй позже."
	_, err := b.client.SendHTML(ctx, chatID, text)
	return err
}

// ══════════════════════════════════════════════════════════════════════════════
// STATISTICS
// ══════════════════════════════════════════════════════════════════════════════

// GetStats returns current bot statistics.
func (b *Bot) GetStats() map[string]interface{} {
	b.stats.mu.RLock()
	defer b.stats.mu.RUnlock()

	uptime := time.Since(b.stats.StartedAt)

	commandsCopy := make(map[string]int64)
	for k, v := range b.stats.CommandsCount {
		commandsCopy[k] = v
	}

	return map[string]interface{}{
		"started_at":       b.stats.StartedAt,
		"uptime":           uptime.String(),
		"updates_received": b.stats.UpdatesReceived,
		"updates_handled":  b.stats.UpdatesHandled,
		"errors_count":     b.stats.ErrorsCount,
		"commands_count":   commandsCopy,
		"running":          b.IsRunning(),
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// CLIENT ACCESS
// ══════════════════════════════════════════════════════════════════════════════

// Client returns the Telegram client for direct API access.
// Use sparingly - prefer going through handlers.
func (b *Bot) Client() *telegram.Client {
	return b.client
}

// Router returns the router for handler registration.
func (b *Bot) Router() *Router {
	return b.router
}

// InvalidateAuthCache invalidates the auth cache for a specific user.
func (b *Bot) InvalidateAuthCache(telegramID int64) {
	b.authMiddleware.InvalidateCache(telegramID)
}
