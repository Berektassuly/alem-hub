package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment represents the application environment.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// Config holds all application configuration.
type Config struct {
	// Application
	App AppConfig

	// Database
	Database DatabaseConfig

	// Redis
	Redis RedisConfig

	// Telegram Bot
	Telegram TelegramConfig

	// Alem Platform API
	Alem AlemConfig

	// Scheduler
	Scheduler SchedulerConfig

	// Feature Flags
	Features *FeatureFlags

	// Observability
	Observability ObservabilityConfig
}

// AppConfig holds general application settings.
type AppConfig struct {
	Name        string
	Environment Environment
	Debug       bool
	Version     string

	// Timezone for cron jobs and notifications (default: Asia/Almaty)
	Timezone string
	Location *time.Location

	// Graceful shutdown timeout
	ShutdownTimeout time.Duration
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	// Connection string (Supabase format)
	// Example: postgres://user:pass@host:5432/dbname?sslmode=require
	URL string

	// Connection pool settings
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration

	// Query timeout
	QueryTimeout time.Duration

	// Enable query logging in debug mode
	LogQueries bool
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	// Connection URL
	// Example: redis://user:pass@host:6379/0
	URL string

	// Alternative: individual settings
	Host     string
	Port     int
	Password string
	DB       int

	// Pool settings
	PoolSize     int
	MinIdleConns int

	// Timeouts
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// Enable for development without Redis
	Disabled bool
}

// TelegramConfig holds Telegram Bot settings.
type TelegramConfig struct {
	// Bot token from @BotFather
	Token string

	// Webhook settings (production)
	WebhookURL    string
	WebhookSecret string
	UseWebhook    bool

	// Long polling settings (development)
	PollingTimeout time.Duration

	// Rate limiting
	GlobalRateLimit  int           // messages per second globally
	UserRateLimit    int           // messages per minute per user
	UserRateLimitBan time.Duration // ban duration for spammers

	// Bot behavior
	ParseMode string // "HTML" or "MarkdownV2"

	// Admin user IDs (for admin commands)
	AdminIDs []int64
}

// AlemConfig holds Alem Platform API settings.
type AlemConfig struct {
	// Base URL of Alem Platform
	BaseURL string

	// Authentication (if needed)
	APIKey   string
	Username string
	Password string

	// Rate limiting (protect from being blocked)
	RateLimit      int // requests per minute
	RateLimitBurst int // burst size
	RequestTimeout time.Duration
	MaxRetries     int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration

	// Circuit breaker settings
	CircuitBreakerThreshold   int           // failures before opening
	CircuitBreakerTimeout     time.Duration // time before half-open
	CircuitBreakerHalfOpenMax int           // max requests in half-open

	// Cache settings
	CacheTTL time.Duration // how long to cache responses
}

// SchedulerConfig holds background job settings.
type SchedulerConfig struct {
	// Enable/disable scheduler
	Enabled bool

	// Job intervals
	SyncStudentsInterval       time.Duration // sync with Alem API
	RebuildLeaderboardInterval time.Duration // recalculate rankings
	DetectInactiveInterval     time.Duration // find inactive students
	CleanupInterval            time.Duration // cleanup old data

	// Daily digest time (in configured timezone)
	DailyDigestHour   int // 0-23
	DailyDigestMinute int // 0-59

	// Concurrency
	MaxConcurrentJobs int
	JobTimeout        time.Duration
}

// ObservabilityConfig holds logging and metrics settings.
type ObservabilityConfig struct {
	// Logging
	LogLevel  string // debug, info, warn, error
	LogFormat string // json, text

	// Metrics (future: Prometheus)
	MetricsEnabled bool
	MetricsPort    int

	// Tracing (future: OpenTelemetry)
	TracingEnabled  bool
	TracingEndpoint string
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}

	// Load App config
	cfg.App = loadAppConfig()

	// Load Database config
	var err error
	cfg.Database, err = loadDatabaseConfig()
	if err != nil {
		return nil, fmt.Errorf("database config: %w", err)
	}

	// Load Redis config
	cfg.Redis = loadRedisConfig()

	// Load Telegram config
	cfg.Telegram, err = loadTelegramConfig()
	if err != nil {
		return nil, fmt.Errorf("telegram config: %w", err)
	}

	// Load Alem config
	cfg.Alem = loadAlemConfig()

	// Load Scheduler config
	cfg.Scheduler = loadSchedulerConfig()

	// Load Feature Flags
	cfg.Features = LoadFeatureFlags()

	// Load Observability config
	cfg.Observability = loadObservabilityConfig()

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func loadAppConfig() AppConfig {
	env := Environment(getEnv("APP_ENV", "development"))
	timezone := getEnv("APP_TIMEZONE", "Asia/Almaty")

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	return AppConfig{
		Name:            getEnv("APP_NAME", "alem-community-hub"),
		Environment:     env,
		Debug:           env == EnvDevelopment || getEnvBool("APP_DEBUG", false),
		Version:         getEnv("APP_VERSION", "0.1.0"),
		Timezone:        timezone,
		Location:        loc,
		ShutdownTimeout: getEnvDuration("APP_SHUTDOWN_TIMEOUT", 30*time.Second),
	}
}

func loadDatabaseConfig() (DatabaseConfig, error) {
	url := getEnv("DATABASE_URL", "")
	if url == "" {
		// Try to build from individual components (Supabase style)
		host := getEnv("DB_HOST", "")
		port := getEnv("DB_PORT", "5432")
		user := getEnv("DB_USER", "")
		pass := getEnv("DB_PASSWORD", "")
		name := getEnv("DB_NAME", "postgres")
		sslmode := getEnv("DB_SSLMODE", "require")

		if host != "" && user != "" {
			url = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
				user, pass, host, port, name, sslmode)
		}
	}

	return DatabaseConfig{
		URL:             url,
		MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		ConnMaxIdleTime: getEnvDuration("DB_CONN_MAX_IDLE_TIME", 1*time.Minute),
		QueryTimeout:    getEnvDuration("DB_QUERY_TIMEOUT", 30*time.Second),
		LogQueries:      getEnvBool("DB_LOG_QUERIES", false),
	}, nil
}

func loadRedisConfig() RedisConfig {
	return RedisConfig{
		URL:          getEnv("REDIS_URL", ""),
		Host:         getEnv("REDIS_HOST", "localhost"),
		Port:         getEnvInt("REDIS_PORT", 6379),
		Password:     getEnv("REDIS_PASSWORD", ""),
		DB:           getEnvInt("REDIS_DB", 0),
		PoolSize:     getEnvInt("REDIS_POOL_SIZE", 10),
		MinIdleConns: getEnvInt("REDIS_MIN_IDLE_CONNS", 2),
		DialTimeout:  getEnvDuration("REDIS_DIAL_TIMEOUT", 5*time.Second),
		ReadTimeout:  getEnvDuration("REDIS_READ_TIMEOUT", 3*time.Second),
		WriteTimeout: getEnvDuration("REDIS_WRITE_TIMEOUT", 3*time.Second),
		Disabled:     getEnvBool("REDIS_DISABLED", false),
	}
}

func loadTelegramConfig() (TelegramConfig, error) {
	token := getEnv("TELEGRAM_BOT_TOKEN", "")

	return TelegramConfig{
		Token:            token,
		WebhookURL:       getEnv("TELEGRAM_WEBHOOK_URL", ""),
		WebhookSecret:    getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		UseWebhook:       getEnvBool("TELEGRAM_USE_WEBHOOK", false),
		PollingTimeout:   getEnvDuration("TELEGRAM_POLLING_TIMEOUT", 60*time.Second),
		GlobalRateLimit:  getEnvInt("TELEGRAM_GLOBAL_RATE_LIMIT", 30),
		UserRateLimit:    getEnvInt("TELEGRAM_USER_RATE_LIMIT", 20),
		UserRateLimitBan: getEnvDuration("TELEGRAM_USER_RATE_LIMIT_BAN", 5*time.Minute),
		ParseMode:        getEnv("TELEGRAM_PARSE_MODE", "HTML"),
		AdminIDs:         getEnvInt64Slice("TELEGRAM_ADMIN_IDS", nil),
	}, nil
}

func loadAlemConfig() AlemConfig {
	return AlemConfig{
		BaseURL:                   getEnv("ALEM_BASE_URL", "https://platform.alem.school"),
		APIKey:                    getEnv("ALEM_API_KEY", ""),
		Username:                  getEnv("ALEM_USERNAME", ""),
		Password:                  getEnv("ALEM_PASSWORD", ""),
		RateLimit:                 getEnvInt("ALEM_RATE_LIMIT", 10),
		RateLimitBurst:            getEnvInt("ALEM_RATE_LIMIT_BURST", 3),
		RequestTimeout:            getEnvDuration("ALEM_REQUEST_TIMEOUT", 30*time.Second),
		MaxRetries:                getEnvInt("ALEM_MAX_RETRIES", 3),
		RetryBaseDelay:            getEnvDuration("ALEM_RETRY_BASE_DELAY", 1*time.Second),
		RetryMaxDelay:             getEnvDuration("ALEM_RETRY_MAX_DELAY", 30*time.Second),
		CircuitBreakerThreshold:   getEnvInt("ALEM_CB_THRESHOLD", 5),
		CircuitBreakerTimeout:     getEnvDuration("ALEM_CB_TIMEOUT", 60*time.Second),
		CircuitBreakerHalfOpenMax: getEnvInt("ALEM_CB_HALF_OPEN_MAX", 3),
		CacheTTL:                  getEnvDuration("ALEM_CACHE_TTL", 5*time.Minute),
	}
}

func loadSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		Enabled:                    getEnvBool("SCHEDULER_ENABLED", true),
		SyncStudentsInterval:       getEnvDuration("SCHEDULER_SYNC_INTERVAL", 5*time.Minute),
		RebuildLeaderboardInterval: getEnvDuration("SCHEDULER_LEADERBOARD_INTERVAL", 10*time.Minute),
		DetectInactiveInterval:     getEnvDuration("SCHEDULER_INACTIVE_INTERVAL", 1*time.Hour),
		CleanupInterval:            getEnvDuration("SCHEDULER_CLEANUP_INTERVAL", 24*time.Hour),
		DailyDigestHour:            getEnvInt("SCHEDULER_DIGEST_HOUR", 21),
		DailyDigestMinute:          getEnvInt("SCHEDULER_DIGEST_MINUTE", 0),
		MaxConcurrentJobs:          getEnvInt("SCHEDULER_MAX_CONCURRENT", 5),
		JobTimeout:                 getEnvDuration("SCHEDULER_JOB_TIMEOUT", 5*time.Minute),
	}
}

func loadObservabilityConfig() ObservabilityConfig {
	return ObservabilityConfig{
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		LogFormat:       getEnv("LOG_FORMAT", "json"),
		MetricsEnabled:  getEnvBool("METRICS_ENABLED", false),
		MetricsPort:     getEnvInt("METRICS_PORT", 9090),
		TracingEnabled:  getEnvBool("TRACING_ENABLED", false),
		TracingEndpoint: getEnv("TRACING_ENDPOINT", ""),
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	var errs []string

	// Validate required fields
	if c.Telegram.Token == "" {
		errs = append(errs, "TELEGRAM_BOT_TOKEN is required")
	}

	// Database URL is required in production
	if c.App.Environment == EnvProduction {
		if c.Database.URL == "" {
			errs = append(errs, "DATABASE_URL is required in production")
		}
	}

	// Validate ranges
	if c.Scheduler.DailyDigestHour < 0 || c.Scheduler.DailyDigestHour > 23 {
		errs = append(errs, "SCHEDULER_DIGEST_HOUR must be 0-23")
	}

	if c.Scheduler.DailyDigestMinute < 0 || c.Scheduler.DailyDigestMinute > 59 {
		errs = append(errs, "SCHEDULER_DIGEST_MINUTE must be 0-59")
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.App.Environment == EnvDevelopment
}

// IsProduction returns true if running in production mode.
func (c *Config) IsProduction() bool {
	return c.App.Environment == EnvProduction
}

// --- Helper functions for environment variable parsing ---

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

func getEnvInt64Slice(key string, defaultVal []int64) []int64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	parts := strings.Split(val, ",")
	result := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		i, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			continue
		}
		result = append(result, i)
	}
	return result
}
