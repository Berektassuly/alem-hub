// Package postgres implements PostgreSQL persistence layer for Alem Community Hub.
// Uses Supabase as the PostgreSQL provider for simplicity and free tier benefits.
// Philosophy: "From Competition to Collaboration" - the database supports
// social features like finding helpers and building connections.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ══════════════════════════════════════════════════════════════════════════════
// ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrConnectionClosed indicates the connection pool is closed.
	ErrConnectionClosed = errors.New("postgres: connection pool is closed")

	// ErrMigrationFailed indicates a migration failure.
	ErrMigrationFailed = errors.New("postgres: migration failed")

	// ErrTransactionFailed indicates a transaction failure.
	ErrTransactionFailed = errors.New("postgres: transaction failed")

	// ErrNoRows is returned when a query returns no rows.
	ErrNoRows = pgx.ErrNoRows
)

// ══════════════════════════════════════════════════════════════════════════════
// CONNECTION POOL
// ══════════════════════════════════════════════════════════════════════════════

// Config holds PostgreSQL connection configuration.
type Config struct {
	// Host is the database host (e.g., "db.xxxx.supabase.co").
	Host string

	// Port is the database port (default 5432, or 6543 for Supabase pooler).
	Port int

	// Database is the database name.
	Database string

	// User is the database user.
	User string

	// Password is the database password.
	Password string

	// SSLMode is the SSL mode (disable, require, verify-ca, verify-full).
	SSLMode string

	// MaxConns is the maximum number of connections in the pool.
	MaxConns int32

	// MinConns is the minimum number of connections in the pool.
	MinConns int32

	// MaxConnLifetime is the maximum lifetime of a connection.
	MaxConnLifetime time.Duration

	// MaxConnIdleTime is the maximum idle time of a connection.
	MaxConnIdleTime time.Duration

	// HealthCheckPeriod is the interval between health checks.
	HealthCheckPeriod time.Duration

	// ConnectTimeout is the timeout for establishing a connection.
	ConnectTimeout time.Duration
}

// DefaultConfig returns a sensible default configuration for Supabase.
func DefaultConfig() Config {
	return Config{
		Port:              5432,
		Database:          "postgres",
		User:              "postgres",
		SSLMode:           "require", // Supabase requires SSL
		MaxConns:          10,
		MinConns:          2,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: time.Minute,
		ConnectTimeout:    10 * time.Second,
	}
}

// DSN returns the connection string for PostgreSQL.
func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s connect_timeout=%d",
		c.Host,
		c.Port,
		c.Database,
		c.User,
		c.Password,
		c.SSLMode,
		int(c.ConnectTimeout.Seconds()),
	)
}

// PoolConfig returns pgxpool configuration.
func (c Config) PoolConfig() (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(c.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	config.MaxConns = c.MaxConns
	config.MinConns = c.MinConns
	config.MaxConnLifetime = c.MaxConnLifetime
	config.MaxConnIdleTime = c.MaxConnIdleTime
	config.HealthCheckPeriod = c.HealthCheckPeriod

	return config, nil
}

// Connection represents a PostgreSQL connection pool with health checks.
type Connection struct {
	pool   *pgxpool.Pool
	config Config
	closed bool
	mu     sync.RWMutex
}

// NewConnection creates a new PostgreSQL connection pool.
func NewConnection(ctx context.Context, cfg Config) (*Connection, error) {
	poolConfig, err := cfg.PoolConfig()
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: failed to ping database: %w", err)
	}

	conn := &Connection{
		pool:   pool,
		config: cfg,
		closed: false,
	}

	return conn, nil
}

// NewConnectionFromURL creates a connection from a database URL.
// Useful for Supabase connection strings.
func NewConnectionFromURL(ctx context.Context, databaseURL string) (*Connection, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to parse database URL: %w", err)
	}

	// Apply sensible defaults
	if poolConfig.MaxConns == 0 {
		poolConfig.MaxConns = 10
	}
	if poolConfig.MinConns == 0 {
		poolConfig.MinConns = 2
	}
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: failed to ping database: %w", err)
	}

	conn := &Connection{
		pool:   pool,
		config: Config{}, // URL-based config
		closed: false,
	}

	return conn, nil
}

// Pool returns the underlying connection pool.
func (c *Connection) Pool() *pgxpool.Pool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pool
}

// Close closes the connection pool.
func (c *Connection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	c.pool.Close()
}

// IsClosed returns true if the connection pool is closed.
func (c *Connection) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// Ping checks if the database connection is alive.
func (c *Connection) Ping(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return ErrConnectionClosed
	}

	return c.pool.Ping(ctx)
}

// Health returns detailed health information.
func (c *Connection) Health(ctx context.Context) (*HealthStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, ErrConnectionClosed
	}

	status := &HealthStatus{
		CheckedAt: time.Now().UTC(),
	}

	// Check ping
	start := time.Now()
	if err := c.pool.Ping(ctx); err != nil {
		status.Healthy = false
		status.Error = err.Error()
		return status, nil
	}
	status.PingLatency = time.Since(start)

	// Get pool stats
	stats := c.pool.Stat()
	status.TotalConns = stats.TotalConns()
	status.IdleConns = stats.IdleConns()
	status.AcquiredConns = stats.AcquiredConns()
	status.MaxConns = stats.MaxConns()
	status.AcquireCount = stats.AcquireCount()
	status.AcquireDuration = stats.AcquireDuration()
	status.EmptyAcquireCount = stats.EmptyAcquireCount()

	// Get database size (Supabase specific)
	var dbSize int64
	err := c.pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&dbSize)
	if err == nil {
		status.DatabaseSize = dbSize
	}

	// Get active connections count
	var activeConns int
	err = c.pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_stat_activity 
		WHERE datname = current_database() AND state = 'active'
	`).Scan(&activeConns)
	if err == nil {
		status.ActiveQueries = activeConns
	}

	status.Healthy = true
	return status, nil
}

// HealthStatus contains database health information.
type HealthStatus struct {
	Healthy           bool
	Error             string
	CheckedAt         time.Time
	PingLatency       time.Duration
	TotalConns        int32
	IdleConns         int32
	AcquiredConns     int32
	MaxConns          int32
	AcquireCount      int64
	AcquireDuration   time.Duration
	EmptyAcquireCount int64
	DatabaseSize      int64 // in bytes
	ActiveQueries     int
}

// ══════════════════════════════════════════════════════════════════════════════
// TRANSACTION SUPPORT
// ══════════════════════════════════════════════════════════════════════════════

// TxOptions holds transaction options.
type TxOptions struct {
	IsoLevel       pgx.TxIsoLevel
	AccessMode     pgx.TxAccessMode
	DeferrableMode pgx.TxDeferrableMode
}

// DefaultTxOptions returns default transaction options.
func DefaultTxOptions() TxOptions {
	return TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	}
}

// ReadOnlyTxOptions returns read-only transaction options.
func ReadOnlyTxOptions() TxOptions {
	return TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadOnly,
	}
}

// BeginTx starts a new transaction with the given options.
func (c *Connection) BeginTx(ctx context.Context, opts TxOptions) (pgx.Tx, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, ErrConnectionClosed
	}

	txOptions := pgx.TxOptions{
		IsoLevel:       opts.IsoLevel,
		AccessMode:     opts.AccessMode,
		DeferrableMode: opts.DeferrableMode,
	}

	tx, err := c.pool.BeginTx(ctx, txOptions)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTransactionFailed, err)
	}

	return tx, nil
}

// WithTx executes a function within a transaction.
// The transaction is committed if the function returns nil, rolled back otherwise.
func (c *Connection) WithTx(ctx context.Context, opts TxOptions, fn func(pgx.Tx) error) error {
	tx, err := c.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx error: %v, rollback error: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit error: %w", err)
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// QUERY HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// Querier is an interface that both *pgxpool.Pool and pgx.Tx implement.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

// Exec executes a query that doesn't return rows.
func (c *Connection) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return pgconn.CommandTag{}, ErrConnectionClosed
	}

	return c.pool.Exec(ctx, sql, args...)
}

// Query executes a query that returns rows.
func (c *Connection) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, ErrConnectionClosed
	}

	return c.pool.Query(ctx, sql, args...)
}

// QueryRow executes a query that returns a single row.
func (c *Connection) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.pool.QueryRow(ctx, sql, args...)
}

// ══════════════════════════════════════════════════════════════════════════════
// MIGRATION SUPPORT
// ══════════════════════════════════════════════════════════════════════════════

// Migration represents a database migration.
type Migration struct {
	Version   int
	Name      string
	UpSQL     string
	DownSQL   string
	AppliedAt time.Time
	IsApplied bool
}

// Migrator handles database migrations.
type Migrator struct {
	conn       *Connection
	migrations []Migration
	tableName  string
}

// NewMigrator creates a new migrator with embedded migrations.
func NewMigrator(conn *Connection) *Migrator {
	return &Migrator{
		conn:       conn,
		migrations: GetMigrations(),
		tableName:  "schema_migrations",
	}
}

// NewMigratorWithMigrations creates a migrator with custom migrations.
func NewMigratorWithMigrations(conn *Connection, migrations []Migration) *Migrator {
	return &Migrator{
		conn:       conn,
		migrations: migrations,
		tableName:  "schema_migrations",
	}
}

// EnsureMigrationTable creates the migration tracking table if it doesn't exist.
func (m *Migrator) EnsureMigrationTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)
	`, m.tableName)

	_, err := m.conn.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	return nil
}

// GetAppliedMigrations returns all applied migrations.
func (m *Migrator) GetAppliedMigrations(ctx context.Context) (map[int]time.Time, error) {
	query := fmt.Sprintf("SELECT version, applied_at FROM %s ORDER BY version", m.tableName)

	rows, err := m.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]time.Time)
	for rows.Next() {
		var version int
		var appliedAt time.Time

		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, fmt.Errorf("failed to scan migration row: %w", err)
		}

		applied[version] = appliedAt
	}

	return applied, rows.Err()
}

// Migrate applies all pending migrations.
func (m *Migrator) Migrate(ctx context.Context) error {
	if err := m.EnsureMigrationTable(ctx); err != nil {
		return err
	}

	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	for _, mig := range m.migrations {
		if _, isApplied := applied[mig.Version]; isApplied {
			continue
		}

		if mig.UpSQL == "" {
			return fmt.Errorf("%w: missing up SQL for migration %d", ErrMigrationFailed, mig.Version)
		}

		// Apply migration in transaction
		err := m.conn.WithTx(ctx, DefaultTxOptions(), func(tx pgx.Tx) error {
			// Execute migration SQL
			if _, err := tx.Exec(ctx, mig.UpSQL); err != nil {
				return fmt.Errorf("failed to execute migration %d: %w", mig.Version, err)
			}

			// Record migration
			insertQuery := fmt.Sprintf(
				"INSERT INTO %s (version, name) VALUES ($1, $2)",
				m.tableName,
			)
			_, err := tx.Exec(ctx, insertQuery, mig.Version, mig.Name)
			return err
		})
		if err != nil {
			return fmt.Errorf("%w: version %d: %v", ErrMigrationFailed, mig.Version, err)
		}
	}

	return nil
}

// Rollback rolls back the last applied migration.
func (m *Migrator) Rollback(ctx context.Context) error {
	if err := m.EnsureMigrationTable(ctx); err != nil {
		return err
	}

	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Find the last applied migration
	var lastVersion int
	for v := range applied {
		if v > lastVersion {
			lastVersion = v
		}
	}

	if lastVersion == 0 {
		return nil // Nothing to rollback
	}

	// Find the migration
	var migration *Migration
	for i := range m.migrations {
		if m.migrations[i].Version == lastVersion {
			migration = &m.migrations[i]
			break
		}
	}

	if migration == nil || migration.DownSQL == "" {
		return fmt.Errorf("%w: missing down SQL for migration %d", ErrMigrationFailed, lastVersion)
	}

	// Rollback in transaction
	return m.conn.WithTx(ctx, DefaultTxOptions(), func(tx pgx.Tx) error {
		// Execute rollback SQL
		if _, err := tx.Exec(ctx, migration.DownSQL); err != nil {
			return fmt.Errorf("failed to rollback migration %d: %w", lastVersion, err)
		}

		// Remove migration record
		deleteQuery := fmt.Sprintf("DELETE FROM %s WHERE version = $1", m.tableName)
		_, err := tx.Exec(ctx, deleteQuery, lastVersion)
		return err
	})
}

// Status returns the migration status.
func (m *Migrator) Status(ctx context.Context) ([]Migration, error) {
	if err := m.EnsureMigrationTable(ctx); err != nil {
		return nil, err
	}

	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Migration, len(m.migrations))
	copy(result, m.migrations)

	for i := range result {
		if appliedAt, ok := applied[result[i].Version]; ok {
			result[i].IsApplied = true
			result[i].AppliedAt = appliedAt
		}
	}

	return result, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// ERROR HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// IsUniqueViolation checks if the error is a unique constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" // unique_violation
	}
	return false
}

// IsForeignKeyViolation checks if the error is a foreign key violation.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503" // foreign_key_violation
	}
	return false
}

// IsNotNullViolation checks if the error is a not null violation.
func IsNotNullViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23502" // not_null_violation
	}
	return false
}

// IsNoRows checks if the error is a "no rows" error.
func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// ══════════════════════════════════════════════════════════════════════════════
// EMBEDDED MIGRATIONS
// ══════════════════════════════════════════════════════════════════════════════

// GetMigrations returns all embedded migrations.
func GetMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "create_students",
			UpSQL:   migration001Up,
			DownSQL: migration001Down,
		},
		{
			Version: 2,
			Name:    "create_leaderboard",
			UpSQL:   migration002Up,
			DownSQL: migration002Down,
		},
		{
			Version: 3,
			Name:    "create_social",
			UpSQL:   migration003Up,
			DownSQL: migration003Down,
		},
	}
}
