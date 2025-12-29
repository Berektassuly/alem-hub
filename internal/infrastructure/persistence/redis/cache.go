// Package redis implements Redis caching, pub/sub, and online tracking functionality.
// This package provides infrastructure components for high-performance data access
// in the Alem Community Hub project.
//
// Key components:
//   - Cache: General-purpose caching with TTL management
//   - OnlineTracker: Real-time student presence tracking
//   - LeaderboardCache: Hot leaderboard data with sorted sets
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ══════════════════════════════════════════════════════════════════════════════
// CONFIGURATION
// ══════════════════════════════════════════════════════════════════════════════

// Config holds Redis connection configuration.
type Config struct {
	// Host is the Redis server hostname.
	Host string

	// Port is the Redis server port.
	Port int

	// Password is the Redis authentication password (empty if no auth).
	Password string

	// DB is the Redis database number (0-15).
	DB int

	// PoolSize is the maximum number of socket connections.
	PoolSize int

	// MinIdleConns is the minimum number of idle connections.
	MinIdleConns int

	// MaxRetries is the maximum number of retries before giving up.
	MaxRetries int

	// DialTimeout is the timeout for establishing new connections.
	DialTimeout time.Duration

	// ReadTimeout is the timeout for socket reads.
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for socket writes.
	WriteTimeout time.Duration

	// PoolTimeout is the timeout for getting a connection from the pool.
	PoolTimeout time.Duration
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		Host:         "localhost",
		Port:         6379,
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	}
}

// Addr returns the Redis address in "host:port" format.
func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// ══════════════════════════════════════════════════════════════════════════════
// ERRORS
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ErrCacheMiss is returned when the requested key is not found in cache.
	ErrCacheMiss = errors.New("cache: key not found")

	// ErrCacheConnection is returned when Redis connection fails.
	ErrCacheConnection = errors.New("cache: connection failed")

	// ErrCacheSerialization is returned when serialization/deserialization fails.
	ErrCacheSerialization = errors.New("cache: serialization failed")

	// ErrCacheInvalidTTL is returned when an invalid TTL is provided.
	ErrCacheInvalidTTL = errors.New("cache: invalid TTL")

	// ErrCacheKeyEmpty is returned when an empty key is provided.
	ErrCacheKeyEmpty = errors.New("cache: key cannot be empty")

	// ErrCacheNilValue is returned when attempting to cache a nil value.
	ErrCacheNilValue = errors.New("cache: value cannot be nil")
)

// ══════════════════════════════════════════════════════════════════════════════
// KEY PREFIXES
// ══════════════════════════════════════════════════════════════════════════════

// Key prefixes for namespacing Redis keys.
const (
	// PrefixStudent is the prefix for student-related keys.
	PrefixStudent = "student:"

	// PrefixLeaderboard is the prefix for leaderboard-related keys.
	PrefixLeaderboard = "leaderboard:"

	// PrefixOnline is the prefix for online tracking keys.
	PrefixOnline = "online:"

	// PrefixSession is the prefix for session-related keys.
	PrefixSession = "session:"

	// PrefixActivity is the prefix for activity-related keys.
	PrefixActivity = "activity:"

	// PrefixNotification is the prefix for notification-related keys.
	PrefixNotification = "notification:"

	// PrefixRateLimit is the prefix for rate limiting keys.
	PrefixRateLimit = "ratelimit:"

	// PrefixLock is the prefix for distributed lock keys.
	PrefixLock = "lock:"

	// PrefixPubSub is the prefix for pub/sub channels.
	PrefixPubSub = "pubsub:"
)

// ══════════════════════════════════════════════════════════════════════════════
// DEFAULT TTLs
// ══════════════════════════════════════════════════════════════════════════════

// Default TTL values for different types of cached data.
const (
	// TTLOnlineStatus is the TTL for online status (student is considered offline after this).
	TTLOnlineStatus = 5 * time.Minute

	// TTLAwayStatus is the TTL after which a student is considered "away".
	TTLAwayStatus = 30 * time.Minute

	// TTLLeaderboardCache is the TTL for leaderboard cache.
	TTLLeaderboardCache = 5 * time.Minute

	// TTLStudentCache is the TTL for student data cache.
	TTLStudentCache = 10 * time.Minute

	// TTLSnapshotCache is the TTL for leaderboard snapshots.
	TTLSnapshotCache = 30 * time.Minute

	// TTLSessionData is the TTL for session data.
	TTLSessionData = 24 * time.Hour

	// TTLRateLimitWindow is the default rate limit window.
	TTLRateLimitWindow = 1 * time.Minute

	// TTLDistributedLock is the default lock TTL.
	TTLDistributedLock = 30 * time.Second
)

// ══════════════════════════════════════════════════════════════════════════════
// CACHE CLIENT
// ══════════════════════════════════════════════════════════════════════════════

// Cache provides general-purpose caching functionality with Redis.
// It handles serialization, TTL management, and error handling.
type Cache struct {
	client *redis.Client
	config Config
}

// NewCache creates a new Cache instance with the given configuration.
func NewCache(cfg Config) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolTimeout:  cfg.PoolTimeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheConnection, err)
	}

	return &Cache{
		client: client,
		config: cfg,
	}, nil
}

// Client returns the underlying Redis client for advanced operations.
// Use with caution - prefer using Cache methods when possible.
func (c *Cache) Client() *redis.Client {
	return c.client
}

// Close closes the Redis connection.
func (c *Cache) Close() error {
	return c.client.Close()
}

// Ping checks if Redis is reachable.
func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// ══════════════════════════════════════════════════════════════════════════════
// BASIC OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// Set stores a value with the given key and TTL.
// The value is serialized to JSON before storage.
func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if key == "" {
		return ErrCacheKeyEmpty
	}
	if value == nil {
		return ErrCacheNilValue
	}
	if ttl < 0 {
		return ErrCacheInvalidTTL
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCacheSerialization, err)
	}

	return c.client.Set(ctx, key, data, ttl).Err()
}

// SetString stores a string value directly without JSON serialization.
func (c *Cache) SetString(ctx context.Context, key string, value string, ttl time.Duration) error {
	if key == "" {
		return ErrCacheKeyEmpty
	}
	if ttl < 0 {
		return ErrCacheInvalidTTL
	}

	return c.client.Set(ctx, key, value, ttl).Err()
}

// Get retrieves and deserializes a value by key.
// Returns ErrCacheMiss if the key doesn't exist.
func (c *Cache) Get(ctx context.Context, key string, dest interface{}) error {
	if key == "" {
		return ErrCacheKeyEmpty
	}

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}
		return err
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("%w: %v", ErrCacheSerialization, err)
	}

	return nil
}

// GetString retrieves a string value directly without JSON deserialization.
func (c *Cache) GetString(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", ErrCacheKeyEmpty
	}

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrCacheMiss
		}
		return "", err
	}

	return val, nil
}

// Delete removes a key from the cache.
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	return c.client.Del(ctx, keys...).Err()
}

// Exists checks if a key exists in the cache.
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, ErrCacheKeyEmpty
	}

	count, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Expire sets a new TTL on an existing key.
func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if key == "" {
		return ErrCacheKeyEmpty
	}
	if ttl < 0 {
		return ErrCacheInvalidTTL
	}

	return c.client.Expire(ctx, key, ttl).Err()
}

// TTL returns the remaining TTL for a key.
// Returns -2 if the key doesn't exist, -1 if the key has no TTL.
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	if key == "" {
		return 0, ErrCacheKeyEmpty
	}

	return c.client.TTL(ctx, key).Result()
}

// ══════════════════════════════════════════════════════════════════════════════
// BATCH OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// MSet stores multiple key-value pairs.
// All values are serialized to JSON.
func (c *Cache) MSet(ctx context.Context, pairs map[string]interface{}, ttl time.Duration) error {
	if len(pairs) == 0 {
		return nil
	}
	if ttl < 0 {
		return ErrCacheInvalidTTL
	}

	pipe := c.client.Pipeline()

	for key, value := range pairs {
		if key == "" {
			continue
		}

		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("%w: key %s: %v", ErrCacheSerialization, key, err)
		}

		pipe.Set(ctx, key, data, ttl)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// MGet retrieves multiple values by keys.
// Returns a map of key -> value, skipping missing keys.
func (c *Cache) MGet(ctx context.Context, keys ...string) (map[string]string, error) {
	if len(keys) == 0 {
		return make(map[string]string), nil
	}

	values, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(keys))
	for i, val := range values {
		if val != nil {
			result[keys[i]] = val.(string)
		}
	}

	return result, nil
}

// DeleteByPattern deletes all keys matching a pattern.
// Use with caution in production as SCAN can be slow on large datasets.
func (c *Cache) DeleteByPattern(ctx context.Context, pattern string) error {
	if pattern == "" {
		return ErrCacheKeyEmpty
	}

	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) >= 100 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}

	if err := iter.Err(); err != nil {
		return err
	}

	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// ATOMIC OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// SetNX sets a value only if the key doesn't exist (for distributed locks).
func (c *Cache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	if key == "" {
		return false, ErrCacheKeyEmpty
	}
	if ttl < 0 {
		return false, ErrCacheInvalidTTL
	}

	data, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrCacheSerialization, err)
	}

	return c.client.SetNX(ctx, key, data, ttl).Result()
}

// Incr increments a counter and returns the new value.
func (c *Cache) Incr(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, ErrCacheKeyEmpty
	}

	return c.client.Incr(ctx, key).Result()
}

// IncrBy increments a counter by a specific amount.
func (c *Cache) IncrBy(ctx context.Context, key string, delta int64) (int64, error) {
	if key == "" {
		return 0, ErrCacheKeyEmpty
	}

	return c.client.IncrBy(ctx, key, delta).Result()
}

// Decr decrements a counter and returns the new value.
func (c *Cache) Decr(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, ErrCacheKeyEmpty
	}

	return c.client.Decr(ctx, key).Result()
}

// GetSet atomically sets a value and returns the old value.
func (c *Cache) GetSet(ctx context.Context, key string, value interface{}) (string, error) {
	if key == "" {
		return "", ErrCacheKeyEmpty
	}

	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCacheSerialization, err)
	}

	return c.client.GetSet(ctx, key, data).Result()
}

// ══════════════════════════════════════════════════════════════════════════════
// HASH OPERATIONS (for structured data)
// ══════════════════════════════════════════════════════════════════════════════

// HSet stores a hash field.
func (c *Cache) HSet(ctx context.Context, key, field string, value interface{}) error {
	if key == "" || field == "" {
		return ErrCacheKeyEmpty
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCacheSerialization, err)
	}

	return c.client.HSet(ctx, key, field, data).Err()
}

// HGet retrieves a hash field.
func (c *Cache) HGet(ctx context.Context, key, field string, dest interface{}) error {
	if key == "" || field == "" {
		return ErrCacheKeyEmpty
	}

	data, err := c.client.HGet(ctx, key, field).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}
		return err
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("%w: %v", ErrCacheSerialization, err)
	}

	return nil
}

// HGetAll retrieves all fields from a hash.
func (c *Cache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if key == "" {
		return nil, ErrCacheKeyEmpty
	}

	return c.client.HGetAll(ctx, key).Result()
}

// HDel deletes hash fields.
func (c *Cache) HDel(ctx context.Context, key string, fields ...string) error {
	if key == "" {
		return ErrCacheKeyEmpty
	}

	return c.client.HDel(ctx, key, fields...).Err()
}

// HExists checks if a hash field exists.
func (c *Cache) HExists(ctx context.Context, key, field string) (bool, error) {
	if key == "" || field == "" {
		return false, ErrCacheKeyEmpty
	}

	return c.client.HExists(ctx, key, field).Result()
}

// ══════════════════════════════════════════════════════════════════════════════
// SET OPERATIONS (for unique collections)
// ══════════════════════════════════════════════════════════════════════════════

// SAdd adds members to a set.
func (c *Cache) SAdd(ctx context.Context, key string, members ...interface{}) error {
	if key == "" {
		return ErrCacheKeyEmpty
	}

	return c.client.SAdd(ctx, key, members...).Err()
}

// SRem removes members from a set.
func (c *Cache) SRem(ctx context.Context, key string, members ...interface{}) error {
	if key == "" {
		return ErrCacheKeyEmpty
	}

	return c.client.SRem(ctx, key, members...).Err()
}

// SMembers returns all members of a set.
func (c *Cache) SMembers(ctx context.Context, key string) ([]string, error) {
	if key == "" {
		return nil, ErrCacheKeyEmpty
	}

	return c.client.SMembers(ctx, key).Result()
}

// SIsMember checks if a member exists in a set.
func (c *Cache) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	if key == "" {
		return false, ErrCacheKeyEmpty
	}

	return c.client.SIsMember(ctx, key, member).Result()
}

// SCard returns the number of members in a set.
func (c *Cache) SCard(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, ErrCacheKeyEmpty
	}

	return c.client.SCard(ctx, key).Result()
}

// ══════════════════════════════════════════════════════════════════════════════
// PUB/SUB OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// Publish publishes a message to a channel.
func (c *Cache) Publish(ctx context.Context, channel string, message interface{}) error {
	if channel == "" {
		return ErrCacheKeyEmpty
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCacheSerialization, err)
	}

	return c.client.Publish(ctx, channel, data).Err()
}

// Subscribe creates a subscription to channels.
// Remember to call Close() on the returned PubSub when done.
func (c *Cache) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.client.Subscribe(ctx, channels...)
}

// PSubscribe creates a pattern-based subscription.
func (c *Cache) PSubscribe(ctx context.Context, patterns ...string) *redis.PubSub {
	return c.client.PSubscribe(ctx, patterns...)
}

// ══════════════════════════════════════════════════════════════════════════════
// UTILITY FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// FlushDB removes all keys from the current database.
// Use with extreme caution! Primarily for testing.
func (c *Cache) FlushDB(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}

// DBSize returns the number of keys in the current database.
func (c *Cache) DBSize(ctx context.Context) (int64, error) {
	return c.client.DBSize(ctx).Result()
}

// Info returns Redis server information.
func (c *Cache) Info(ctx context.Context, section ...string) (string, error) {
	return c.client.Info(ctx, section...).Result()
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ══════════════════════════════════════════════════════════════════════════════

// StudentKey generates a cache key for student data.
func StudentKey(studentID string) string {
	return PrefixStudent + studentID
}

// LeaderboardKey generates a cache key for leaderboard data.
func LeaderboardKey(cohort string) string {
	if cohort == "" {
		return PrefixLeaderboard + "all"
	}
	return PrefixLeaderboard + cohort
}

// OnlineKey generates a cache key for online status.
func OnlineKey(studentID string) string {
	return PrefixOnline + studentID
}

// SessionKey generates a cache key for session data.
func SessionKey(sessionID string) string {
	return PrefixSession + sessionID
}

// RateLimitKey generates a cache key for rate limiting.
func RateLimitKey(identifier, action string) string {
	return PrefixRateLimit + identifier + ":" + action
}

// LockKey generates a cache key for distributed locks.
func LockKey(resource string) string {
	return PrefixLock + resource
}

// PubSubChannel generates a pub/sub channel name.
func PubSubChannel(eventType string) string {
	return PrefixPubSub + eventType
}
