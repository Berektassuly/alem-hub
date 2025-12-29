// Package middleware contains Telegram bot middlewares for request processing.
package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// RATE LIMITER MIDDLEWARE
// Protects the bot from spam and abuse using a token bucket algorithm.
// Philosophy: Be gentle with legitimate users who might accidentally
// send multiple messages, but firm with actual spammers.
// ══════════════════════════════════════════════════════════════════════════════

// RateLimitConfig holds configuration for the rate limiter.
type RateLimitConfig struct {
	// RequestsPerMinute is the maximum number of requests per user per minute.
	RequestsPerMinute int

	// BurstSize is the maximum burst size (tokens in bucket at start).
	BurstSize int

	// CleanupInterval is how often to clean up expired entries.
	CleanupInterval time.Duration

	// BanDuration is how long to temporarily ban users who exceed limits.
	BanDuration time.Duration

	// BanThreshold is the number of limit violations before temporary ban.
	BanThreshold int

	// WhitelistedUsers are users exempt from rate limiting (e.g., admins).
	WhitelistedUsers map[int64]bool

	// OnRateLimited is called when a user hits the rate limit.
	// Returns the message to send to the user.
	OnRateLimited func(telegramID int64, retryAfter time.Duration) string
}

// DefaultRateLimitConfig returns sensible defaults for rate limiting.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute: 20, // 20 requests per minute
		BurstSize:         5,  // Allow burst of 5 requests
		CleanupInterval:   5 * time.Minute,
		BanDuration:       10 * time.Minute,
		BanThreshold:      3, // 3 violations = temp ban
		WhitelistedUsers:  make(map[int64]bool),
		OnRateLimited: func(telegramID int64, retryAfter time.Duration) string {
			seconds := int(retryAfter.Seconds())
			if seconds < 60 {
				return fmt.Sprintf(
					"⏳ Слишком много запросов!\n\n"+
						"Подожди %d секунд и попробуй снова.\n"+
						"<i>Это защита от спама.</i>",
					seconds,
				)
			}
			minutes := seconds / 60
			return fmt.Sprintf(
				"⏳ Слишком много запросов!\n\n"+
					"Подожди %d минут и попробуй снова.\n"+
					"<i>Это защита от спама.</i>",
				minutes,
			)
		},
	}
}

// RateLimiter implements per-user rate limiting using the token bucket algorithm.
type RateLimiter struct {
	config  RateLimitConfig
	buckets sync.Map // map[int64]*tokenBucket
	bans    sync.Map // map[int64]*banEntry
}

// tokenBucket represents a user's rate limit state.
type tokenBucket struct {
	mu           sync.Mutex
	tokens       float64
	lastRefill   time.Time
	refillRate   float64 // tokens per second
	maxTokens    float64
	violations   int
	lastViolated time.Time
}

// banEntry represents a temporary ban for a user.
type banEntry struct {
	bannedAt  time.Time
	expiresAt time.Time
	reason    string
}

// NewRateLimiter creates a new rate limiter with the given configuration.
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config: config,
	}

	// Start background cleanup
	go rl.cleanupLoop()

	return rl
}

// RateLimitResult represents the result of a rate limit check.
type RateLimitResult struct {
	// Allowed indicates if the request is allowed.
	Allowed bool

	// RetryAfter is how long the user should wait before retrying.
	RetryAfter time.Duration

	// IsBanned indicates if the user is temporarily banned.
	IsBanned bool

	// BanExpiresAt is when the ban expires (if banned).
	BanExpiresAt time.Time

	// ResponseMessage is the message to send if rate limited.
	ResponseMessage string

	// RemainingTokens is the number of tokens remaining in the bucket.
	RemainingTokens int
}

// Check checks if a request from the given user is allowed.
func (rl *RateLimiter) Check(ctx context.Context, telegramID int64) *RateLimitResult {
	// Check whitelist
	if rl.config.WhitelistedUsers[telegramID] {
		return &RateLimitResult{
			Allowed:         true,
			RemainingTokens: rl.config.BurstSize,
		}
	}

	// Check if user is banned
	if ban := rl.getBan(telegramID); ban != nil {
		return &RateLimitResult{
			Allowed:         false,
			IsBanned:        true,
			BanExpiresAt:    ban.expiresAt,
			RetryAfter:      time.Until(ban.expiresAt),
			ResponseMessage: rl.config.OnRateLimited(telegramID, time.Until(ban.expiresAt)),
		}
	}

	// Get or create bucket for user
	bucket := rl.getBucket(telegramID)

	// Try to consume a token
	allowed, retryAfter, remaining := bucket.consume()

	if !allowed {
		// Record violation
		bucket.recordViolation()

		// Check if should ban
		if bucket.violations >= rl.config.BanThreshold {
			rl.banUser(telegramID, "exceeded rate limit threshold")
		}

		return &RateLimitResult{
			Allowed:         false,
			RetryAfter:      retryAfter,
			ResponseMessage: rl.config.OnRateLimited(telegramID, retryAfter),
			RemainingTokens: 0,
		}
	}

	return &RateLimitResult{
		Allowed:         true,
		RemainingTokens: remaining,
	}
}

// getBucket returns the token bucket for a user, creating one if needed.
func (rl *RateLimiter) getBucket(telegramID int64) *tokenBucket {
	// Try to load existing bucket
	if val, ok := rl.buckets.Load(telegramID); ok {
		return val.(*tokenBucket)
	}

	// Create new bucket
	bucket := &tokenBucket{
		tokens:     float64(rl.config.BurstSize),
		lastRefill: time.Now(),
		refillRate: float64(rl.config.RequestsPerMinute) / 60.0, // tokens per second
		maxTokens:  float64(rl.config.BurstSize),
	}

	// Store, handling race condition
	actual, _ := rl.buckets.LoadOrStore(telegramID, bucket)
	return actual.(*tokenBucket)
}

// consume tries to consume a token from the bucket.
// Returns (allowed, retryAfter, remainingTokens).
func (b *tokenBucket) consume() (bool, time.Duration, int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now

	// Try to consume
	if b.tokens >= 1.0 {
		b.tokens--
		return true, 0, int(b.tokens)
	}

	// Calculate when next token will be available
	deficit := 1.0 - b.tokens
	retryAfter := time.Duration(deficit/b.refillRate) * time.Second

	return false, retryAfter, 0
}

// recordViolation records a rate limit violation.
func (b *tokenBucket) recordViolation() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Reset violations if last violation was more than 5 minutes ago
	if time.Since(b.lastViolated) > 5*time.Minute {
		b.violations = 0
	}

	b.violations++
	b.lastViolated = time.Now()
}

// getBan returns the ban entry for a user, or nil if not banned.
func (rl *RateLimiter) getBan(telegramID int64) *banEntry {
	val, ok := rl.bans.Load(telegramID)
	if !ok {
		return nil
	}

	ban := val.(*banEntry)
	if time.Now().After(ban.expiresAt) {
		rl.bans.Delete(telegramID)
		return nil
	}

	return ban
}

// banUser temporarily bans a user.
func (rl *RateLimiter) banUser(telegramID int64, reason string) {
	ban := &banEntry{
		bannedAt:  time.Now(),
		expiresAt: time.Now().Add(rl.config.BanDuration),
		reason:    reason,
	}
	rl.bans.Store(telegramID, ban)
}

// Unban removes a temporary ban for a user.
func (rl *RateLimiter) Unban(telegramID int64) {
	rl.bans.Delete(telegramID)
}

// Reset resets the rate limit state for a user.
func (rl *RateLimiter) Reset(telegramID int64) {
	rl.buckets.Delete(telegramID)
	rl.bans.Delete(telegramID)
}

// AddToWhitelist adds a user to the whitelist.
func (rl *RateLimiter) AddToWhitelist(telegramID int64) {
	rl.config.WhitelistedUsers[telegramID] = true
}

// RemoveFromWhitelist removes a user from the whitelist.
func (rl *RateLimiter) RemoveFromWhitelist(telegramID int64) {
	delete(rl.config.WhitelistedUsers, telegramID)
}

// GetStats returns rate limiting statistics for a user.
func (rl *RateLimiter) GetStats(telegramID int64) (remaining int, violations int, isBanned bool) {
	// Check ban
	if ban := rl.getBan(telegramID); ban != nil {
		return 0, 0, true
	}

	// Check bucket
	if val, ok := rl.buckets.Load(telegramID); ok {
		bucket := val.(*tokenBucket)
		bucket.mu.Lock()
		remaining = int(bucket.tokens)
		violations = bucket.violations
		bucket.mu.Unlock()
	} else {
		remaining = rl.config.BurstSize
	}

	return remaining, violations, false
}

// cleanupLoop periodically cleans up expired entries.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

// cleanup removes expired buckets and bans.
func (rl *RateLimiter) cleanup() {
	now := time.Now()
	inactiveThreshold := 10 * time.Minute

	// Clean up old buckets
	rl.buckets.Range(func(key, value interface{}) bool {
		bucket := value.(*tokenBucket)
		bucket.mu.Lock()
		inactive := now.Sub(bucket.lastRefill) > inactiveThreshold
		bucket.mu.Unlock()

		if inactive {
			rl.buckets.Delete(key)
		}
		return true
	})

	// Clean up expired bans
	rl.bans.Range(func(key, value interface{}) bool {
		ban := value.(*banEntry)
		if now.After(ban.expiresAt) {
			rl.bans.Delete(key)
		}
		return true
	})
}

// ══════════════════════════════════════════════════════════════════════════════
// COMMAND-SPECIFIC RATE LIMITS
// Some commands might need different rate limits than the default.
// ══════════════════════════════════════════════════════════════════════════════

// CommandRateLimits holds command-specific rate limit configurations.
type CommandRateLimits struct {
	limiters map[string]*RateLimiter
	default_ *RateLimiter
}

// NewCommandRateLimits creates a new command-specific rate limiter.
func NewCommandRateLimits(defaultConfig RateLimitConfig) *CommandRateLimits {
	return &CommandRateLimits{
		limiters: make(map[string]*RateLimiter),
		default_: NewRateLimiter(defaultConfig),
	}
}

// AddCommand adds rate limiting for a specific command.
func (c *CommandRateLimits) AddCommand(command string, config RateLimitConfig) {
	c.limiters[command] = NewRateLimiter(config)
}

// Check checks rate limit for a specific command.
func (c *CommandRateLimits) Check(ctx context.Context, telegramID int64, command string) *RateLimitResult {
	if limiter, ok := c.limiters[command]; ok {
		return limiter.Check(ctx, telegramID)
	}
	return c.default_.Check(ctx, telegramID)
}

// ══════════════════════════════════════════════════════════════════════════════
// ADAPTIVE RATE LIMITING
// Adjusts rate limits based on user behavior over time.
// Good users get higher limits, suspicious users get stricter limits.
// ══════════════════════════════════════════════════════════════════════════════

// UserReputation tracks user behavior for adaptive rate limiting.
type UserReputation struct {
	mu          sync.RWMutex
	reputations map[int64]*reputation
}

type reputation struct {
	score         float64   // 0.0 to 1.0 (1.0 = fully trusted)
	totalRequests int       // total number of requests
	violations    int       // number of violations
	lastUpdated   time.Time // when last updated
}

// NewUserReputation creates a new reputation tracker.
func NewUserReputation() *UserReputation {
	return &UserReputation{
		reputations: make(map[int64]*reputation),
	}
}

// GetMultiplier returns the rate limit multiplier for a user.
// Trusted users get higher multipliers (more lenient limits).
func (r *UserReputation) GetMultiplier(telegramID int64) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rep, ok := r.reputations[telegramID]
	if !ok {
		return 1.0 // default multiplier for new users
	}

	// Multiplier ranges from 0.5 (strict) to 2.0 (lenient)
	return 0.5 + (rep.score * 1.5)
}

// RecordGoodBehavior increases a user's reputation.
func (r *UserReputation) RecordGoodBehavior(telegramID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rep := r.getOrCreateReputation(telegramID)
	rep.totalRequests++
	rep.score = min(1.0, rep.score+0.01) // slowly increase
	rep.lastUpdated = time.Now()
}

// RecordViolation decreases a user's reputation.
func (r *UserReputation) RecordViolation(telegramID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rep := r.getOrCreateReputation(telegramID)
	rep.violations++
	rep.score = max(0.0, rep.score-0.1) // quickly decrease
	rep.lastUpdated = time.Now()
}

func (r *UserReputation) getOrCreateReputation(telegramID int64) *reputation {
	rep, ok := r.reputations[telegramID]
	if !ok {
		rep = &reputation{
			score:       0.5, // start at neutral
			lastUpdated: time.Now(),
		}
		r.reputations[telegramID] = rep
	}
	return rep
}

// min returns the minimum of two float64 values.
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two float64 values.
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
