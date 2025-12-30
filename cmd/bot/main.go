// Package main - точка входа для Telegram Bot приложения Alem Community Hub.
//
// Философия: "От конкуренции к сотрудничеству" - бот превращает холодный
// лидерборд в тёплое сообщество взаимопомощи, где каждый студент знает,
// к кому обратиться за помощью, и никто не остаётся один.
//
// Архитектура следует принципам Clean Architecture и DDD:
// - Domain: чистая бизнес-логика без внешних зависимостей
// - Application: оркестрация use cases (Commands/Queries/Sagas)
// - Infrastructure: реализация репозиториев, внешние API
// - Interface: Telegram Bot handlers, HTTP endpoints
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	// Application layer
	"github.com/alem-hub/alem-community-hub/internal/application/command"
	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"github.com/alem-hub/alem-community-hub/internal/application/saga"

	// Infrastructure layer
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/external/alem"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/messaging"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/persistence/postgres"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/persistence/redis"

	// Interface layer
	httpserver "github.com/alem-hub/alem-community-hub/internal/interface/http"
	"github.com/alem-hub/alem-community-hub/internal/interface/telegram"

	// Packages
	"github.com/alem-hub/alem-community-hub/pkg/logger"
)

// ══════════════════════════════════════════════════════════════════════════════
// CONFIGURATION
// ══════════════════════════════════════════════════════════════════════════════

// Config содержит всю конфигурацию приложения.
type Config struct {
	// App
	AppEnv      string // development, staging, production
	AppDebug    bool
	AppTimezone string

	// Telegram Bot
	TelegramToken   string
	TelegramMode    string // polling или webhook
	TelegramWebhook string

	// PostgreSQL (Supabase)
	DatabaseURL string

	// Redis (опционально, для кеширования)
	RedisURL     string
	RedisEnabled bool

	// HTTP Server
	HTTPHost string
	HTTPPort int

	// Alem Platform API
	AlemAPIURL   string
	AlemAPIToken string

	// Graceful Shutdown
	ShutdownTimeout time.Duration
}

// LoadConfig загружает конфигурацию из переменных окружения.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		AppEnv:          getEnv("APP_ENV", "development"),
		AppDebug:        getEnvBool("APP_DEBUG", false),
		AppTimezone:     getEnv("APP_TIMEZONE", "Asia/Almaty"),
		TelegramToken:   getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramMode:    getEnv("TELEGRAM_MODE", "polling"),
		TelegramWebhook: getEnv("TELEGRAM_WEBHOOK_URL", ""),
		DatabaseURL:     getEnv("DATABASE_URL", ""),
		RedisURL:        getEnv("REDIS_URL", ""),
		RedisEnabled:    getEnvBool("REDIS_ENABLED", false),
		HTTPHost:        getEnv("HTTP_HOST", "0.0.0.0"),
		HTTPPort:        getEnvInt("HTTP_PORT", 8080),
		AlemAPIURL:      getEnv("ALEM_API_URL", "https://platform.alem.school"),
		AlemAPIToken:    getEnv("ALEM_API_TOKEN", ""),
		ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
	}

	// Валидация обязательных полей
	if cfg.TelegramToken == "" {
		return nil, errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	return cfg, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// MAIN
// ══════════════════════════════════════════════════════════════════════════════

func main() {
	// Создаём корневой контекст с возможностью отмены
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Запускаем приложение
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// ─────────────────────────────────────────────────────────────────────────
	// 1. ЗАГРУЗКА КОНФИГУРАЦИИ
	// ─────────────────────────────────────────────────────────────────────────
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 2. НАСТРОЙКА ЛОГИРОВАНИЯ
	// ─────────────────────────────────────────────────────────────────────────
	log := setupLogger(cfg)
	log.Info("starting Alem Community Hub Bot",
		"env", cfg.AppEnv,
		"debug", cfg.AppDebug,
		"timezone", cfg.AppTimezone,
	)

	// Timezone setting is handled by pkg/timeutil.AlmatyTZ (UTC+5)
	// The package uses a fixed timezone for Almaty, no global setting needed
	log.Info("using Almaty timezone (UTC+5)", "configured", cfg.AppTimezone)

	// ─────────────────────────────────────────────────────────────────────────
	// 3. ПОДКЛЮЧЕНИЕ К БАЗЕ ДАННЫХ (PostgreSQL/Supabase)
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("connecting to database...")
	dbConn, err := postgres.NewConnectionFromURL(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		log.Info("closing database connection...")
		dbConn.Close()
	}()

	// Проверяем соединение
	if err := dbConn.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	log.Info("database connection established")

	// ─────────────────────────────────────────────────────────────────────────
	// 4. ЗАПУСК МИГРАЦИЙ
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("running database migrations...")
	migrator := postgres.NewMigrator(dbConn)
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	status, err := migrator.Status(ctx)
	if err != nil {
		log.Warn("failed to get migration status", "error", err)
	} else {
		appliedCount := 0
		for _, m := range status {
			if m.IsApplied {
				appliedCount++
			}
		}
		log.Info("migrations completed", "applied", appliedCount, "total", len(status))
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 5. ИНИЦИАЛИЗАЦИЯ REDIS (опционально)
	// ─────────────────────────────────────────────────────────────────────────
	var redisCache *redis.Cache
	var onlineTracker *redis.OnlineTracker
	var leaderboardCache *redis.LeaderboardCache

	if cfg.RedisEnabled && cfg.RedisURL != "" {
		log.Info("connecting to Redis...")
		redisCfg := redis.DefaultConfig()
		// TODO: Parse cfg.RedisURL to extract host, port, password
		// For now, use default config
		redisCache, err = redis.NewCache(redisCfg)
		if err != nil {
			log.Warn("failed to connect to Redis, caching disabled", "error", err)
		} else {
			defer redisCache.Close()
			onlineTracker = redis.NewOnlineTracker(redisCache)
			leaderboardCache = redis.NewLeaderboardCache(redisCache)
			log.Info("Redis connection established")
		}
	}
	// Suppress unused variable warnings for now (will be used when handlers are implemented)
	_ = onlineTracker
	_ = leaderboardCache

	// ─────────────────────────────────────────────────────────────────────────
	// 6. ИНИЦИАЛИЗАЦИЯ РЕПОЗИТОРИЕВ
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing repositories...")
	studentRepo := postgres.NewStudentRepository(dbConn)
	progressRepo := postgres.NewProgressRepository(dbConn)
	leaderboardRepo := postgres.NewLeaderboardRepository(dbConn)

	// ─────────────────────────────────────────────────────────────────────────
	// 7. ИНИЦИАЛИЗАЦИЯ EVENT BUS
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing event bus...")
	eventBusConfig := messaging.DefaultInMemoryEventBusConfig()
	eventBusConfig.Logger = log
	eventBusConfig.AsyncMode = true
	eventBus := messaging.NewInMemoryEventBus(eventBusConfig)
	defer func() {
		log.Info("closing event bus...")
		_ = eventBus.Close()
	}()

	// ─────────────────────────────────────────────────────────────────────────
	// 8. ИНИЦИАЛИЗАЦИЯ ВНЕШНИХ КЛИЕНТОВ
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing external clients...")

	// Alem Platform API Client
	alemConfig := alem.DefaultClientConfig(cfg.AlemAPIURL)
	alemConfig.APIKey = cfg.AlemAPIToken
	alemConfig.Logger = log
	alemClient := alem.NewClient(alemConfig)
	_ = alemClient // TODO: Used when handlers are properly initialized

	// ─────────────────────────────────────────────────────────────────────────
	// 9. ИНИЦИАЛИЗАЦИЯ APPLICATION LAYER (Commands, Queries, Sagas)
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing application layer...")

	// Create a leaderboard service adapter for sync handler
	leaderboardService := &leaderboardServiceAdapter{repo: leaderboardRepo}

	// TODO: Commands require infrastructure implementations that are not yet complete.
	// Most command handlers need dependencies like:
	// - social.Repository, activity.Repository
	// - notification.NotificationService
	// - Various cache implementations
	// For now, all commands are stubbed as nil.
	var syncStudentCmd *command.SyncStudentHandler = nil
	var requestHelpCmd *command.RequestHelpHandler = nil
	var connectStudentsCmd *command.ConnectStudentsHandler = nil
	var updatePrefsCmd *command.UpdatePreferencesHandler = nil
	_ = syncStudentCmd
	_ = requestHelpCmd
	_ = connectStudentsCmd
	_ = updatePrefsCmd
	_ = leaderboardService

	// Initialize query handlers with available repositories
	// Cache and tracker can be nil - handlers handle that gracefully
	// Note: leaderboardCache doesn't fully implement domain interface yet, using nil
	leaderboardQuery := query.NewGetLeaderboardHandler(leaderboardRepo, nil, nil)
	studentRankQuery := query.NewGetStudentRankHandler(studentRepo, leaderboardRepo, nil, nil)
	neighborsQuery := query.NewGetNeighborsHandler(studentRepo, leaderboardRepo, nil)
	dailyProgressQuery := query.NewGetDailyProgressHandler(studentRepo, progressRepo, leaderboardRepo)

	// TODO: These handlers require activity.Repository which is not yet implemented
	var findHelpersQuery *query.FindHelpersHandler = nil
	var onlineNowQuery *query.GetOnlineNowHandler = nil
	_ = findHelpersQuery
	_ = onlineNowQuery

	// TODO: Saga requires many dependencies that are not yet implemented
	var onboardingSaga *saga.OnboardingSaga = nil
	_ = onboardingSaga

	// ─────────────────────────────────────────────────────────────────────────
	// 10. РЕГИСТРАЦИЯ EVENT HANDLERS
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("registering event handlers...")

	// TODO: Event handlers require dependencies that are not yet implemented:
	// - notification.NotificationSender
	// - leaderboard.LeaderboardCache
	// - activity.Repository, activity.TaskIndex
	// - social.Repository, social.HelpRequestRepository
	// For now, event handlers are not registered.
	// Uncomment and fix when infrastructure is ready.
	/*
	rankChangedHandler := eventhandler.NewOnRankChangedHandler(...)
	taskCompletedHandler := eventhandler.NewOnTaskCompletedHandler(...)
	studentStuckHandler := eventhandler.NewOnStudentStuckHandler(...)
	*/
	log.Info("event handlers not registered - infrastructure dependencies missing")

	// ─────────────────────────────────────────────────────────────────────────
	// 11. СОЗДАНИЕ TELEGRAM BOT
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing Telegram bot...")

	botConfig := telegram.DefaultBotConfig(cfg.TelegramToken)
	botConfig.Mode = cfg.TelegramMode
	botConfig.WebhookURL = cfg.TelegramWebhook
	botConfig.Debug = cfg.AppDebug
	botConfig.Logger = log

	botDeps := telegram.BotDependencies{
		StudentRepo:        studentRepo,
		SyncStudentCmd:     syncStudentCmd,
		RequestHelpCmd:     requestHelpCmd,
		ConnectStudentsCmd: connectStudentsCmd,
		UpdatePrefsCmd:     updatePrefsCmd,
		LeaderboardQuery:   leaderboardQuery,
		StudentRankQuery:   studentRankQuery,
		NeighborsQuery:     neighborsQuery,
		FindHelpersQuery:   findHelpersQuery,
		OnlineNowQuery:     onlineNowQuery,
		DailyProgressQuery: dailyProgressQuery,
		OnboardingSaga:     onboardingSaga,
	}

	bot, err := telegram.NewBot(botConfig, botDeps)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}
	_ = bot

	// ─────────────────────────────────────────────────────────────────────────
	// 12. СОЗДАНИЕ HTTP SERVER
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing HTTP server...")

	httpConfig := httpserver.DefaultConfig()
	httpConfig.Host = cfg.HTTPHost
	httpConfig.Port = cfg.HTTPPort

	httpDeps := httpserver.Dependencies{
		GetLeaderboardHandler:   leaderboardQuery,
		GetStudentRankHandler:   studentRankQuery,
		GetOnlineNowHandler:     onlineNowQuery,
		GetNeighborsHandler:     neighborsQuery,
		GetDailyProgressHandler: dailyProgressQuery,
		FindHelpersHandler:      findHelpersQuery,
		Logger:                  logger.Default(),
	}

	httpServer := httpserver.NewServer(httpConfig, httpDeps)

	// ─────────────────────────────────────────────────────────────────────────
	// 13. ЗАПУСК СЕРВИСОВ
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("starting services...")

	// Канал для ошибок
	errCh := make(chan error, 2)

	// Запускаем HTTP сервер
	go func() {
		log.Info("starting HTTP server", "address", httpServer.Address())
		if err := httpServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server error: %w", err)
		}
	}()

	// Запускаем Telegram бота
	go func() {
		log.Info("starting Telegram bot", "mode", cfg.TelegramMode)
		if err := bot.Start(ctx); err != nil {
			errCh <- fmt.Errorf("telegram bot error: %w", err)
		}
	}()

	// ─────────────────────────────────────────────────────────────────────────
	// 14. GRACEFUL SHUTDOWN
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("Alem Community Hub Bot is running",
		"http_address", httpServer.Address(),
		"telegram_mode", cfg.TelegramMode,
	)

	// Ожидаем сигнал завершения или ошибку
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	select {
	case sig := <-sigCh:
		log.Info("received shutdown signal", "signal", sig.String())
	case err := <-errCh:
		log.Error("service error", "error", err)
		return err
	}

	// Начинаем graceful shutdown
	log.Info("starting graceful shutdown...", "timeout", cfg.ShutdownTimeout.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	// Останавливаем сервисы
	var shutdownErr error

	// 1. Останавливаем бота (перестаём принимать новые запросы)
	log.Info("stopping Telegram bot...")
	if err := bot.Stop(shutdownCtx); err != nil {
		log.Error("failed to stop bot gracefully", "error", err)
		shutdownErr = err
	}

	// 2. Останавливаем HTTP сервер
	log.Info("stopping HTTP server...")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("failed to stop HTTP server gracefully", "error", err)
		shutdownErr = err
	}

	// 3. Event bus закроется через defer

	// 4. База данных закроется через defer

	if shutdownErr != nil {
		log.Warn("shutdown completed with errors")
	} else {
		log.Info("shutdown completed successfully")
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// setupLogger настраивает структурированное логирование.
func setupLogger(cfg *Config) *slog.Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	if cfg.AppDebug {
		opts.Level = slog.LevelDebug
	}

	if cfg.AppEnv == "production" {
		// JSON формат для production (лучше для агрегаторов логов)
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		// Текстовый формат для development (лучше читается)
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	log := slog.New(handler)
	slog.SetDefault(log)

	return log
}

// getEnv возвращает переменную окружения или значение по умолчанию.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool возвращает boolean переменную окружения.
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes"
}

// getEnvInt возвращает int переменную окружения.
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	return defaultValue
}

// getEnvDuration возвращает time.Duration переменную окружения.
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	if d, err := time.ParseDuration(value); err == nil {
		return d
	}
	return defaultValue
}

// ══════════════════════════════════════════════════════════════════════════════
// ADAPTERS
// These adapt infrastructure implementations to domain/application interfaces.
// ══════════════════════════════════════════════════════════════════════════════

// leaderboardServiceAdapter adapts postgres.LeaderboardRepository to command.LeaderboardService.
type leaderboardServiceAdapter struct {
	repo *postgres.LeaderboardRepository
}

// GetStudentRank implements command.LeaderboardService.
func (a *leaderboardServiceAdapter) GetStudentRank(ctx context.Context, studentID string) (int, error) {
	entry, err := a.repo.GetStudentRank(ctx, studentID, "")
	if err != nil {
		return 0, err
	}
	if entry == nil {
		return 0, nil
	}
	return int(entry.Rank), nil
}

// InvalidateCache implements command.LeaderboardService.
func (a *leaderboardServiceAdapter) InvalidateCache(ctx context.Context) error {
	return nil // No-op for now, cache invalidation is handled separately
}
