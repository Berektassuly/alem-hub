// Package middleware contains Telegram bot middlewares for request processing.
// These middlewares form a chain that processes every incoming update before
// it reaches the handler, and can modify the response after the handler completes.
package middleware

import (
	"alem-hub/internal/domain/shared"
	"alem-hub/internal/domain/student"
	"context"
	"fmt"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// CONTEXT KEYS
// Used to pass data through the request context.
// ══════════════════════════════════════════════════════════════════════════════

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// StudentContextKey is the context key for the authenticated student.
	StudentContextKey contextKey = "student"

	// TelegramIDContextKey is the context key for the Telegram user ID.
	TelegramIDContextKey contextKey = "telegram_id"

	// RequestIDContextKey is the context key for request tracing.
	RequestIDContextKey contextKey = "request_id"

	// StartTimeContextKey is the context key for request start time.
	StartTimeContextKey contextKey = "start_time"
)

// ══════════════════════════════════════════════════════════════════════════════
// AUTH MIDDLEWARE
// Verifies that the user is registered in the system before allowing access
// to protected commands. Follows the philosophy of "supportive community" -
// unregistered users are gently guided to onboarding, not blocked.
// ══════════════════════════════════════════════════════════════════════════════

// AuthConfig holds configuration for the auth middleware.
type AuthConfig struct {
	// PublicCommands are commands that don't require authentication.
	// These allow new users to discover and join the community.
	PublicCommands map[string]bool

	// CacheTTL is how long to cache student data to reduce DB queries.
	CacheTTL time.Duration

	// OnUnauthorized is called when an unregistered user tries to access
	// a protected command. Returns the message to send to the user.
	OnUnauthorized func(telegramID int64) string
}

// DefaultAuthConfig returns sensible defaults for auth middleware.
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		PublicCommands: map[string]bool{
			"/start": true,
			"start":  true,
		},
		CacheTTL: 5 * time.Minute,
		OnUnauthorized: func(telegramID int64) string {
			return "👋 Привет! Похоже, ты ещё не зарегистрирован.\n\n" +
				"Используй /start чтобы присоединиться к Alem Community Hub!"
		},
	}
}

// AuthMiddleware provides authentication and authorization for bot commands.
// It checks if the Telegram user is registered as a student and injects
// the student data into the context for downstream handlers.
type AuthMiddleware struct {
	studentRepo student.Repository
	config      AuthConfig
	cache       *studentCache
}

// NewAuthMiddleware creates a new auth middleware with the given configuration.
func NewAuthMiddleware(repo student.Repository, config AuthConfig) *AuthMiddleware {
	return &AuthMiddleware{
		studentRepo: repo,
		config:      config,
		cache:       newStudentCache(config.CacheTTL),
	}
}

// AuthResult represents the result of authentication check.
type AuthResult struct {
	// IsAuthenticated indicates if the user is registered.
	IsAuthenticated bool

	// Student is the authenticated student (nil if not authenticated).
	Student *student.Student

	// ShouldContinue indicates if request processing should continue.
	ShouldContinue bool

	// ResponseMessage is the message to send if authentication failed.
	ResponseMessage string
}

// Authenticate checks if the user is registered and returns the student data.
// This is the main entry point for the auth middleware.
func (m *AuthMiddleware) Authenticate(
	ctx context.Context,
	telegramID int64,
	command string,
) (*AuthResult, error) {
	// Check if command is public (doesn't require auth)
	if m.isPublicCommand(command) {
		return &AuthResult{
			IsAuthenticated: false,
			ShouldContinue:  true,
		}, nil
	}

	// Try to get student from cache first
	if cachedStudent := m.cache.get(telegramID); cachedStudent != nil {
		return &AuthResult{
			IsAuthenticated: true,
			Student:         cachedStudent,
			ShouldContinue:  true,
		}, nil
	}

	// Fetch from repository
	stud, err := m.studentRepo.GetByTelegramID(ctx, student.TelegramID(telegramID))
	if err != nil {
		// Check if it's a "not found" error (user not registered)
		if shared.IsNotFound(err) {
			return &AuthResult{
				IsAuthenticated: false,
				ShouldContinue:  false,
				ResponseMessage: m.config.OnUnauthorized(telegramID),
			}, nil
		}

		// Other errors (database issues, etc.)
		return nil, fmt.Errorf("auth: failed to get student: %w", err)
	}

	// Cache the student for future requests
	m.cache.set(telegramID, stud)

	// Update last seen (non-blocking)
	go m.updateLastSeen(context.Background(), stud)

	return &AuthResult{
		IsAuthenticated: true,
		Student:         stud,
		ShouldContinue:  true,
	}, nil
}

// isPublicCommand checks if the command doesn't require authentication.
func (m *AuthMiddleware) isPublicCommand(command string) bool {
	return m.config.PublicCommands[command]
}

// updateLastSeen updates the student's last seen timestamp.
// This runs in a goroutine to not block the request.
func (m *AuthMiddleware) updateLastSeen(ctx context.Context, stud *student.Student) {
	// Only update if more than 1 minute since last update
	// to avoid excessive database writes
	if time.Since(stud.LastSeenAt) < time.Minute {
		return
	}

	stud.MarkOnline()
	_ = m.studentRepo.Update(ctx, stud)
}

// InvalidateCache removes a student from the auth cache.
// Call this when student data is updated.
func (m *AuthMiddleware) InvalidateCache(telegramID int64) {
	m.cache.delete(telegramID)
}

// InvalidateAllCache clears the entire auth cache.
func (m *AuthMiddleware) InvalidateAllCache() {
	m.cache.clear()
}

// ══════════════════════════════════════════════════════════════════════════════
// CONTEXT HELPERS
// Functions to work with authenticated data in context.
// ══════════════════════════════════════════════════════════════════════════════

// ContextWithStudent adds the authenticated student to the context.
func ContextWithStudent(ctx context.Context, stud *student.Student) context.Context {
	return context.WithValue(ctx, StudentContextKey, stud)
}

// StudentFromContext retrieves the authenticated student from context.
// Returns nil if no student is in the context.
func StudentFromContext(ctx context.Context) *student.Student {
	stud, ok := ctx.Value(StudentContextKey).(*student.Student)
	if !ok {
		return nil
	}
	return stud
}

// ContextWithTelegramID adds the Telegram ID to the context.
func ContextWithTelegramID(ctx context.Context, telegramID int64) context.Context {
	return context.WithValue(ctx, TelegramIDContextKey, telegramID)
}

// TelegramIDFromContext retrieves the Telegram ID from context.
// Returns 0 if not found.
func TelegramIDFromContext(ctx context.Context) int64 {
	id, ok := ctx.Value(TelegramIDContextKey).(int64)
	if !ok {
		return 0
	}
	return id
}

// MustStudentFromContext retrieves the student from context or panics.
// Use only when you're certain the student exists (after auth middleware).
func MustStudentFromContext(ctx context.Context) *student.Student {
	stud := StudentFromContext(ctx)
	if stud == nil {
		panic("auth: student not found in context - auth middleware not applied?")
	}
	return stud
}

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT CACHE
// Simple in-memory cache for authenticated students.
// For production with multiple instances, use Redis instead.
// ══════════════════════════════════════════════════════════════════════════════

// studentCache is a thread-safe cache for student data.
type studentCache struct {
	mu      sync.RWMutex
	entries map[int64]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	student   *student.Student
	expiresAt time.Time
}

func newStudentCache(ttl time.Duration) *studentCache {
	c := &studentCache{
		entries: make(map[int64]*cacheEntry),
		ttl:     ttl,
	}

	// Start background cleanup goroutine
	go c.cleanupLoop()

	return c
}

func (c *studentCache) get(telegramID int64) *student.Student {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[telegramID]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}

	return entry.student
}

func (c *studentCache) set(telegramID int64, stud *student.Student) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[telegramID] = &cacheEntry{
		student:   stud,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *studentCache) delete(telegramID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, telegramID)
}

func (c *studentCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[int64]*cacheEntry)
}

func (c *studentCache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

func (c *studentCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for id, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, id)
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// AUTHORIZATION CHECKS
// Additional authorization helpers beyond basic authentication.
// ══════════════════════════════════════════════════════════════════════════════

// RequireActiveStatus checks if the student is in active status.
func RequireActiveStatus(stud *student.Student) error {
	if stud == nil {
		return shared.ErrUnauthorized
	}
	if !stud.Status.IsEnrolled() {
		return shared.WrapError(
			"auth", "RequireActiveStatus",
			shared.ErrForbidden,
			"student is not enrolled in the program",
			nil,
		)
	}
	return nil
}

// RequireHelperStatus checks if the student can help others.
func RequireHelperStatus(stud *student.Student) error {
	if stud == nil {
		return shared.ErrUnauthorized
	}
	if !stud.CanHelp() {
		return shared.WrapError(
			"auth", "RequireHelperStatus",
			shared.ErrForbidden,
			"student cannot help others (disabled in settings or not enrolled)",
			nil,
		)
	}
	return nil
}

// RequireMinXP checks if the student has the minimum required XP.
func RequireMinXP(stud *student.Student, minXP int) error {
	if stud == nil {
		return shared.ErrUnauthorized
	}
	if int(stud.CurrentXP) < minXP {
		return shared.WrapError(
			"auth", "RequireMinXP",
			shared.ErrForbidden,
			fmt.Sprintf("student XP (%d) is below minimum required (%d)", stud.CurrentXP, minXP),
			nil,
		)
	}
	return nil
}
