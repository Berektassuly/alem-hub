// Package main - точка входа для фоновых процессов (Worker) Alem Community Hub.
//
// Worker отвечает за периодические задачи:
// - Синхронизация данных студентов с Alem Platform API
// - Пересчёт лидерборда и рангов
// - Детектирование неактивных студентов
// - Отправка ежедневных дайджестов
//
// Философия: "От конкуренции к сотрудничеству" - Worker обеспечивает
// актуальность данных, чтобы студенты могли быстро находить помощь
// и оставаться на связи с сообществом.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	// Infrastructure layer
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/external/alem"
	extTelegram "github.com/alem-hub/alem-community-hub/internal/infrastructure/external/telegram"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/messaging"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/persistence/postgres"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/persistence/redis"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/scheduler"

	// Packages
	"github.com/alem-hub/alem-community-hub/pkg/timeutil"
)

// ══════════════════════════════════════════════════════════════════════════════
// CONFIGURATION
// ══════════════════════════════════════════════════════════════════════════════

// Config содержит всю конфигурацию Worker приложения.
type Config struct {
	// App
	AppEnv      string // development, staging, production
	AppDebug    bool
	AppTimezone string

	// PostgreSQL (Supabase)
	DatabaseURL string

	// Redis (опционально, для кеширования)
	RedisURL     string
	RedisEnabled bool

	// Alem Platform API
	AlemAPIURL       string
	AlemAPIToken     string
	AlemRateLimit    int           // requests per minute
	AlemSyncInterval time.Duration // как часто синхронизировать

	// Telegram (для отправки уведомлений)
	TelegramToken string

	// Scheduler
	SyncStudentsInterval    time.Duration
	RebuildLeaderboardCron  string // cron expression
	DetectInactiveInterval  time.Duration
	DailyDigestTime         string // время в формате "HH:MM"
	DailyDigestEnabled      bool
	InactivityThresholdDays int

	// Graceful Shutdown
	ShutdownTimeout time.Duration
}

// LoadConfig загружает конфигурацию из переменных окружения.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		AppEnv:                  getEnv("APP_ENV", "development"),
		AppDebug:                getEnvBool("APP_DEBUG", false),
		AppTimezone:             getEnv("APP_TIMEZONE", "Asia/Almaty"),
		DatabaseURL:             getEnv("DATABASE_URL", ""),
		RedisURL:                getEnv("REDIS_URL", ""),
		RedisEnabled:            getEnvBool("REDIS_ENABLED", false),
		AlemAPIURL:              getEnv("ALEM_API_URL", "https://platform.alem.school"),
		AlemAPIToken:            getEnv("ALEM_API_TOKEN", ""),
		AlemRateLimit:           getEnvInt("ALEM_RATE_LIMIT", 10),
		AlemSyncInterval:        getEnvDuration("ALEM_SYNC_INTERVAL", 5*time.Minute),
		TelegramToken:           getEnv("TELEGRAM_BOT_TOKEN", ""),
		SyncStudentsInterval:    getEnvDuration("SYNC_STUDENTS_INTERVAL", 5*time.Minute),
		RebuildLeaderboardCron:  getEnv("REBUILD_LEADERBOARD_CRON", "*/10 * * * *"),
		DetectInactiveInterval:  getEnvDuration("DETECT_INACTIVE_INTERVAL", 1*time.Hour),
		DailyDigestTime:         getEnv("DAILY_DIGEST_TIME", "21:00"),
		DailyDigestEnabled:      getEnvBool("DAILY_DIGEST_ENABLED", true),
		InactivityThresholdDays: getEnvInt("INACTIVITY_THRESHOLD_DAYS", 3),
		ShutdownTimeout:         getEnvDuration("SHUTDOWN_TIMEOUT", 60*time.Second),
	}

	// Валидация обязательных полей
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
	log.Info("starting Alem Community Hub Worker",
		"env", cfg.AppEnv,
		"debug", cfg.AppDebug,
		"timezone", cfg.AppTimezone,
	)

	// Используем временную зону Almaty (UTC+5) для всех операций
	timezone := timeutil.AlmatyTZ

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
	// 4. ЗАПУСК МИГРАЦИЙ (Worker также должен иметь актуальную схему)
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("checking database migrations...")
	migrator := postgres.NewMigrator(dbConn)
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	log.Info("database schema is up to date")

	// ─────────────────────────────────────────────────────────────────────────
	// 5. ИНИЦИАЛИЗАЦИЯ REDIS (опционально)
	// ─────────────────────────────────────────────────────────────────────────
	var redisCache *redis.Cache
	var onlineTracker *redis.OnlineTracker
	var leaderboardCache *redis.LeaderboardCache

	if cfg.RedisEnabled && cfg.RedisURL != "" {
		log.Info("connecting to Redis...")
		redisCfg := redis.DefaultConfig()
		// Parse Redis URL components if needed
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

	// ─────────────────────────────────────────────────────────────────────────
	// 6. ИНИЦИАЛИЗАЦИЯ РЕПОЗИТОРИЕВ
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing repositories...")
	studentRepo := postgres.NewStudentRepository(dbConn)
	progressRepo := postgres.NewProgressRepository(dbConn)
	leaderboardRepo := postgres.NewLeaderboardRepository(dbConn)
	// Note: SyncRepository and NotificationRepository are not yet implemented
	_ = studentRepo // use repos below

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

	// Alem Platform API Client с Rate Limiter
	alemConfig := alem.DefaultClientConfig(cfg.AlemAPIURL)
	alemConfig.Logger = log
	alemClient := alem.NewClient(alemConfig)

	// Telegram Client (для отправки уведомлений)
	var telegramClient *extTelegram.Client
	if cfg.TelegramToken != "" {
		telegramConfig := extTelegram.DefaultClientConfig(cfg.TelegramToken)
		telegramConfig.Logger = log
		telegramClient = extTelegram.NewClient(telegramConfig)
		log.Info("Telegram client initialized")
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 9. ИНИЦИАЛИЗАЦИЯ QUERY HANDLERS (для jobs)
	// ─────────────────────────────────────────────────────────────────────────
	// TODO: Initialize query handlers when all repositories are implemented
	// The handlers require: student.Repository, activity.Repository, activity.OnlineTracker,
	// activity.TaskIndex, social.Repository, and more

	// ─────────────────────────────────────────────────────────────────────────
	// 10. РЕГИСТРАЦИЯ EVENT HANDLERS
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("registering event handlers...")
	// TODO: Event handlers require additional infrastructure:
	// - notification.NotificationSender
	// - leaderboard.LeaderboardCache
	// - activity.Repository, activity.TaskIndex
	// - social.Repository, social.HelpRequestRepository
	// These need to be implemented before handlers can be registered
	_ = eventBus // silence unused warning

	// ─────────────────────────────────────────────────────────────────────────
	// 11. СОЗДАНИЕ SCHEDULER
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing scheduler...")

	schedulerConfig := scheduler.SchedulerConfig{
		Logger:         log,
		Timezone:       timezone,
		MaxHistorySize: 1000,
		EnableMetrics:  true,
	}
	sched := scheduler.NewScheduler(schedulerConfig)

	// Устанавливаем хуки для логирования
	sched.OnJobStart(func(jobName string) {
		log.Debug("job started", "job", jobName)
	})

	sched.OnJobComplete(func(result scheduler.JobResult) {
		if result.Success {
			log.Info("job completed",
				"job", result.JobName,
				"duration", result.Duration.String(),
			)
		}
	})

	sched.OnJobError(func(jobName string, err error) {
		log.Error("job failed", "job", jobName, "error", err)
	})

	// ─────────────────────────────────────────────────────────────────────────
	// 12. РЕГИСТРАЦИЯ JOBS
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("registering jobs...")

	// TODO: Jobs require additional infrastructure components that are not yet implemented:
	// - student.SyncRepository
	// - notification.NotificationRepository
	// - notification.NotificationService
	// - social.Repository
	// - student.OnlineTracker
	// - leaderboard.RankChangeNotifier
	//
	// The following jobs will be registered once infrastructure is complete:
	// - SyncAllStudentsJob: requires SyncRepository
	// - RebuildLeaderboardJob: requires OnlineTracker, RankChangeNotifier
	// - DetectInactiveJob: requires NotificationService, NotificationRepository, social.Repository
	// - DailyDigestJob: requires SocialRepository, NotificationService
	_ = alemClient        // silence unused
	_ = progressRepo      // silence unused
	_ = leaderboardRepo   // silence unused
	_ = leaderboardCache  // silence unused
	_ = onlineTracker     // silence unused
	_ = telegramClient    // silence unused
	_ = cfg               // silence unused

	// ─────────────────────────────────────────────────────────────────────────
	// 13. ЗАПУСК SCHEDULER
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("starting scheduler...")

	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// Выводим информацию о зарегистрированных jobs
	jobsList := sched.ListJobs()
	log.Info("scheduler started",
		"jobs_count", len(jobsList),
	)
	for _, job := range jobsList {
		log.Info("registered job",
			"name", job.Name,
			"description", job.Description,
			"schedule", job.Schedule,
			"next_run", job.NextRun.Format(time.RFC3339),
		)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 14. ЗАПУСК НАЧАЛЬНОЙ СИНХРОНИЗАЦИИ (опционально)
	// ─────────────────────────────────────────────────────────────────────────
	// TODO: Enable initial sync once SyncAllStudentsJob is implemented
	_ = timezone

	// ─────────────────────────────────────────────────────────────────────────
	// 15. GRACEFUL SHUTDOWN
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("Alem Community Hub Worker is running",
		"jobs", len(jobsList),
	)

	// Ожидаем сигнал завершения
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	sig := <-sigCh
	log.Info("received shutdown signal", "signal", sig.String())

	// Начинаем graceful shutdown
	log.Info("starting graceful shutdown...", "timeout", cfg.ShutdownTimeout.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	// Создаём канал для отслеживания завершения shutdown
	done := make(chan struct{})

	go func() {
		defer close(done)

		// 1. Останавливаем scheduler (ждём завершения текущих jobs)
		log.Info("stopping scheduler...")
		if err := sched.Stop(); err != nil {
			log.Error("failed to stop scheduler gracefully", "error", err)
		}

		// 2. Event bus закроется через defer

		// 3. База данных закроется через defer
	}()

	// Ожидаем завершения или таймаут
	select {
	case <-done:
		log.Info("shutdown completed successfully")
	case <-shutdownCtx.Done():
		log.Warn("shutdown timed out, forcing exit")
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
