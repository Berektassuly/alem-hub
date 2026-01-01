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
	"strings"
	"syscall"
	"time"

	// Infrastructure layer
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/external/alem"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/messaging"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/persistence/postgres"
	"github.com/alem-hub/alem-community-hub/internal/infrastructure/persistence/redis"
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
	var leaderboardCache *redis.LeaderboardCache

	if cfg.RedisEnabled && cfg.RedisURL != "" {
		log.Info("connecting to Redis...")
		redisCfg := redis.DefaultConfig()
		// Parse host/port from URL
		if strings.Contains(cfg.RedisURL, ":") {
			parts := strings.Split(cfg.RedisURL, ":")
			if len(parts) == 2 {
				redisCfg.Host = parts[0]
				if p, err := strconv.Atoi(parts[1]); err == nil {
					redisCfg.Port = p
				}
			}
		}

		redisCache, err = redis.NewCache(redisCfg)
		if err != nil {
			log.Warn("failed to connect to Redis, caching disabled", "error", err)
		} else {
			defer redisCache.Close()
			leaderboardCache = redis.NewLeaderboardCache(redisCache)
			log.Info("Redis connection established")
		}
	}

	// Suppress unused variable warnings
	_ = leaderboardCache

	// ─────────────────────────────────────────────────────────────────────────
	// 6. ИНИЦИАЛИЗАЦИЯ РЕПОЗИТОРИЕВ
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing repositories...")
	studentRepo := postgres.NewStudentRepository(dbConn)
	progressRepo := postgres.NewProgressRepository(dbConn)
	leaderboardRepo := postgres.NewLeaderboardRepository(dbConn)

	// Suppress unused variable warnings
	_ = studentRepo
	_ = progressRepo
	_ = leaderboardRepo

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

	// Suppress unused variable warning
	_ = eventBus

	// ─────────────────────────────────────────────────────────────────────────
	// 8. ИНИЦИАЛИЗАЦИЯ ВНЕШНИХ КЛИЕНТОВ
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("initializing external clients...")

	// Alem Platform API Client
	alemConfig := alem.DefaultClientConfig(cfg.AlemAPIURL)
	alemConfig.APIKey = cfg.AlemAPIToken
	alemConfig.Logger = log
	alemClient := alem.NewClient(alemConfig)

	// Suppress unused variable warning
	_ = alemClient

	// ─────────────────────────────────────────────────────────────────────────
	// 9. WORKER LOOP (Simplified - scheduler needs full implementation)
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("starting worker main loop...")
	log.Info("NOTE: Full scheduler/jobs implementation is TODO - worker will just stay alive for now")

	// ─────────────────────────────────────────────────────────────────────────
	// 10. GRACEFUL SHUTDOWN
	// ─────────────────────────────────────────────────────────────────────────
	log.Info("Alem Community Hub Worker is running",
		"timezone", cfg.AppTimezone,
	)

	// Ожидаем сигнал завершения
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	sig := <-sigCh
	log.Info("received shutdown signal", "signal", sig.String())

	// Начинаем graceful shutdown
	log.Info("starting graceful shutdown...", "timeout", cfg.ShutdownTimeout.String())

	log.Info("shutdown completed successfully")
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
