package config

import (
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FeatureFlags manages feature toggles and A/B testing.
// Supports gradual rollout, user targeting, and cohort-based experiments.
//
// Philosophy alignment: Features should support "От конкуренции к сотрудничеству"
// - Social features prioritized
// - Gamification balanced with collaboration
// - Notifications tuned for motivation, not spam
type FeatureFlags struct {
	mu sync.RWMutex

	// Core features
	features map[string]*Feature

	// Override rules (for testing/debugging)
	userOverrides map[int64]map[string]bool // telegramID -> feature -> enabled
}

// Feature represents a single feature flag.
type Feature struct {
	Name        string
	Description string
	Enabled     bool

	// Rollout percentage (0-100)
	// Users are assigned based on hash of their ID
	RolloutPercent int

	// Cohort targeting (e.g., "2024-spring", "2024-fall")
	// Empty means all cohorts
	TargetCohorts []string

	// Time-based activation
	EnabledFrom  *time.Time
	EnabledUntil *time.Time

	// A/B test variant (for experiments)
	Variants []string
}

// FeatureContext provides context for feature flag evaluation.
type FeatureContext struct {
	UserID    int64  // Telegram ID

	Cohort    string // Student cohort (e.g., "2024-spring")
	IsAdmin   bool   // Is admin user
}

// Predefined feature flag names.
const (
	// === Leaderboard Features ===
	FeatureLeaderboardRankChange    = "leaderboard.rank_change"    // Show rank changes (+2, -1)
	FeatureLeaderboardOnlineStatus  = "leaderboard.online_status"  // Show who's online
	FeatureLeaderboardNeighbors     = "leaderboard.neighbors"      // /neighbors command
	FeatureLeaderboardDailyProgress = "leaderboard.daily_progress" // Daily XP in leaderboard

	// === Social Features (core to project philosophy) ===
	FeatureSocialHelpFinder   = "social.help_finder"  // Find helpers for tasks
	FeatureSocialStudyBuddy   = "social.study_buddy"  // Study buddy matching
	FeatureSocialEndorsements = "social.endorsements" // "Thanks for help" system
	FeatureSocialConnections  = "social.connections"  // Direct messaging prompts

	// === Notification Features ===
	FeatureNotifyRankUp        = "notify.rank_up"       // "You moved up!"
	FeatureNotifyRankDown      = "notify.rank_down"     // "X passed you"
	FeatureNotifyTopEntry      = "notify.top_entry"     // "You're in top 50!"
	FeatureNotifyInactive      = "notify.inactive"      // Inactivity reminders
	FeatureNotifyDailyDigest   = "notify.daily_digest"  // End of day summary
	FeatureNotifyEncouragement = "notify.encouragement" // "Almost caught up with X!"
	FeatureNotifyHelpRequest   = "notify.help_request"  // Someone needs help

	// === Gamification Features ===
	FeatureGamificationStreaks      = "gamification.streaks"      // Daily streaks
	FeatureGamificationAchievements = "gamification.achievements" // Badges/achievements
	FeatureGamificationXPBonus      = "gamification.xp_bonus"     // Bonus XP for helping

	// === Experimental Features ===
	FeatureExperimentalAI        = "experimental.ai_suggestions" // AI-powered suggestions
	FeatureExperimentalAnalytics = "experimental.analytics"      // Advanced analytics
	FeatureExperimentalWebhooks  = "experimental.webhooks"       // Real-time webhooks
)

// LoadFeatureFlags loads feature flags from environment variables.
func LoadFeatureFlags() *FeatureFlags {
	ff := &FeatureFlags{
		features:      make(map[string]*Feature),
		userOverrides: make(map[int64]map[string]bool),
	}

	// Initialize all features with defaults
	ff.initializeDefaults()

	// Load overrides from environment
	ff.loadFromEnvironment()

	return ff
}

// initializeDefaults sets up all features with default values.
func (ff *FeatureFlags) initializeDefaults() {
	// Leaderboard features - mostly enabled by default
	ff.features[FeatureLeaderboardRankChange] = &Feature{
		Name:           FeatureLeaderboardRankChange,
		Description:    "Show rank changes in leaderboard",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureLeaderboardOnlineStatus] = &Feature{
		Name:           FeatureLeaderboardOnlineStatus,
		Description:    "Show online status indicators",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureLeaderboardNeighbors] = &Feature{
		Name:           FeatureLeaderboardNeighbors,
		Description:    "Enable /neighbors command",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureLeaderboardDailyProgress] = &Feature{
		Name:           FeatureLeaderboardDailyProgress,
		Description:    "Show daily XP progress",
		Enabled:        true,
		RolloutPercent: 100,
	}

	// Social features - CORE to project, enabled by default
	ff.features[FeatureSocialHelpFinder] = &Feature{
		Name:           FeatureSocialHelpFinder,
		Description:    "Find students who solved specific tasks",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureSocialStudyBuddy] = &Feature{
		Name:           FeatureSocialStudyBuddy,
		Description:    "Study buddy matching system",
		Enabled:        true,
		RolloutPercent: 50, // Gradual rollout
	}

	ff.features[FeatureSocialEndorsements] = &Feature{
		Name:           FeatureSocialEndorsements,
		Description:    "Thank helpers with endorsements",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureSocialConnections] = &Feature{
		Name:           FeatureSocialConnections,
		Description:    "Track student connections",
		Enabled:        true,
		RolloutPercent: 100,
	}

	// Notification features - carefully tuned to avoid spam
	ff.features[FeatureNotifyRankUp] = &Feature{
		Name:           FeatureNotifyRankUp,
		Description:    "Notify when rank improves",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureNotifyRankDown] = &Feature{
		Name:           FeatureNotifyRankDown,
		Description:    "Notify when someone passes you",
		Enabled:        false, // Disabled by default - can be demotivating
		RolloutPercent: 0,
	}

	ff.features[FeatureNotifyTopEntry] = &Feature{
		Name:           FeatureNotifyTopEntry,
		Description:    "Notify when entering top 50",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureNotifyInactive] = &Feature{
		Name:           FeatureNotifyInactive,
		Description:    "Send inactivity reminders",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureNotifyDailyDigest] = &Feature{
		Name:           FeatureNotifyDailyDigest,
		Description:    "Daily progress summary",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureNotifyEncouragement] = &Feature{
		Name:           FeatureNotifyEncouragement,
		Description:    "Encouraging messages",
		Enabled:        true,
		RolloutPercent: 50, // A/B test
	}

	ff.features[FeatureNotifyHelpRequest] = &Feature{
		Name:           FeatureNotifyHelpRequest,
		Description:    "Notify potential helpers",
		Enabled:        true,
		RolloutPercent: 100,
	}

	// Gamification features
	ff.features[FeatureGamificationStreaks] = &Feature{
		Name:           FeatureGamificationStreaks,
		Description:    "Track daily streaks",
		Enabled:        true,
		RolloutPercent: 100,
	}

	ff.features[FeatureGamificationAchievements] = &Feature{
		Name:           FeatureGamificationAchievements,
		Description:    "Unlock achievements",
		Enabled:        false, // Phase 2
		RolloutPercent: 0,
	}

	ff.features[FeatureGamificationXPBonus] = &Feature{
		Name:           FeatureGamificationXPBonus,
		Description:    "Bonus XP for helping others",
		Enabled:        false, // Phase 2
		RolloutPercent: 0,
	}

	// Experimental features - disabled by default
	ff.features[FeatureExperimentalAI] = &Feature{
		Name:           FeatureExperimentalAI,
		Description:    "AI-powered suggestions",
		Enabled:        false,
		RolloutPercent: 0,
	}

	ff.features[FeatureExperimentalAnalytics] = &Feature{
		Name:           FeatureExperimentalAnalytics,
		Description:    "Advanced analytics dashboard",
		Enabled:        false,
		RolloutPercent: 0,
	}

	ff.features[FeatureExperimentalWebhooks] = &Feature{
		Name:           FeatureExperimentalWebhooks,
		Description:    "Real-time webhook updates",
		Enabled:        false,
		RolloutPercent: 0,
	}
}

// loadFromEnvironment loads feature flag overrides from env vars.
// Format: FEATURE_<NAME>=true|false|<percent>
// Example: FEATURE_SOCIAL_HELP_FINDER=true
// Example: FEATURE_NOTIFY_ENCOURAGEMENT=50 (50% rollout)
func (ff *FeatureFlags) loadFromEnvironment() {
	for name, feature := range ff.features {
		envKey := featureNameToEnvKey(name)
		if val := os.Getenv(envKey); val != "" {
			// Try parsing as boolean
			if b, err := strconv.ParseBool(val); err == nil {
				feature.Enabled = b
				if b {
					feature.RolloutPercent = 100
				} else {
					feature.RolloutPercent = 0
				}
				continue
			}

			// Try parsing as percentage
			if p, err := strconv.Atoi(val); err == nil && p >= 0 && p <= 100 {
				feature.Enabled = p > 0
				feature.RolloutPercent = p
			}
		}
	}
}

// featureNameToEnvKey converts feature name to environment variable key.
// "social.help_finder" -> "FEATURE_SOCIAL_HELP_FINDER"
func featureNameToEnvKey(name string) string {
	key := strings.ToUpper(name)
	key = strings.ReplaceAll(key, ".", "_")
	return "FEATURE_" + key
}

// IsEnabled checks if a feature is enabled for the given context.
func (ff *FeatureFlags) IsEnabled(featureName string, ctx *FeatureContext) bool {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	// Check user overrides first
	if ctx != nil && ctx.UserID != 0 {
		if userOverrides, ok := ff.userOverrides[ctx.UserID]; ok {
			if enabled, ok := userOverrides[featureName]; ok {
				return enabled
			}
		}
	}

	feature, ok := ff.features[featureName]
	if !ok {
		return false
	}

	// Admin users get all features
	if ctx != nil && ctx.IsAdmin {
		return true
	}

	// Check if feature is enabled at all
	if !feature.Enabled {
		return false
	}

	// Check time-based activation
	now := time.Now()
	if feature.EnabledFrom != nil && now.Before(*feature.EnabledFrom) {
		return false
	}
	if feature.EnabledUntil != nil && now.After(*feature.EnabledUntil) {
		return false
	}

	// Check cohort targeting
	if len(feature.TargetCohorts) > 0 && ctx != nil && ctx.Cohort != "" {
		cohortMatch := false
		for _, c := range feature.TargetCohorts {
			if c == ctx.Cohort {
				cohortMatch = true
				break
			}
		}
		if !cohortMatch {
			return false
		}
	}

	// Check rollout percentage
	if feature.RolloutPercent < 100 && ctx != nil && ctx.UserID != 0 {
		return ff.isInRollout(ctx.UserID, featureName, feature.RolloutPercent)
	}

	return feature.RolloutPercent > 0
}

// isInRollout determines if a user is in the rollout percentage.
// Uses consistent hashing so users stay in their bucket.
func (ff *FeatureFlags) isInRollout(userID int64, featureName string, percent int) bool {
	// Create a consistent hash for this user+feature combination
	h := fnv.New32a()
	h.Write([]byte(featureName))
	h.Write([]byte(strconv.FormatInt(userID, 10)))
	hash := h.Sum32()

	// Map to 0-99 range
	bucket := int(hash % 100)

	return bucket < percent
}

// GetVariant returns the A/B test variant for a user.
// Returns empty string if no variants defined or feature disabled.
func (ff *FeatureFlags) GetVariant(featureName string, ctx *FeatureContext) string {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	feature, ok := ff.features[featureName]
	if !ok || !ff.IsEnabled(featureName, ctx) {
		return ""
	}

	if len(feature.Variants) == 0 {
		return ""
	}

	// Use consistent hashing to assign variant
	h := fnv.New32a()
	h.Write([]byte(featureName + "_variant"))
	h.Write([]byte(strconv.FormatInt(ctx.UserID, 10)))
	hash := h.Sum32()

	variantIndex := int(hash % uint32(len(feature.Variants)))
	return feature.Variants[variantIndex]
}

// SetUserOverride sets a feature override for a specific user.
// Useful for testing and debugging.
func (ff *FeatureFlags) SetUserOverride(userID int64, featureName string, enabled bool) {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	if _, ok := ff.userOverrides[userID]; !ok {
		ff.userOverrides[userID] = make(map[string]bool)
	}
	ff.userOverrides[userID][featureName] = enabled
}

// ClearUserOverrides removes all overrides for a user.
func (ff *FeatureFlags) ClearUserOverrides(userID int64) {
	ff.mu.Lock()
	defer ff.mu.Unlock()
	delete(ff.userOverrides, userID)
}

// SetRolloutPercent updates the rollout percentage for a feature.
// Thread-safe for live updates.
func (ff *FeatureFlags) SetRolloutPercent(featureName string, percent int) error {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	feature, ok := ff.features[featureName]
	if !ok {
		return ErrFeatureNotFound
	}

	if percent < 0 || percent > 100 {
		return ErrInvalidRolloutPercent
	}

	feature.RolloutPercent = percent
	feature.Enabled = percent > 0

	return nil
}

// EnableFeature enables a feature at 100% rollout.
func (ff *FeatureFlags) EnableFeature(featureName string) error {
	return ff.SetRolloutPercent(featureName, 100)
}

// DisableFeature disables a feature completely.
func (ff *FeatureFlags) DisableFeature(featureName string) error {
	return ff.SetRolloutPercent(featureName, 0)
}

// GetAllFeatures returns a copy of all feature configurations.
func (ff *FeatureFlags) GetAllFeatures() map[string]*Feature {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	result := make(map[string]*Feature, len(ff.features))
	for k, v := range ff.features {
		// Return a copy
		featureCopy := *v
		result[k] = &featureCopy
	}
	return result
}

// --- Convenience methods for common checks ---

// SocialFeaturesEnabled checks if core social features are enabled.
func (ff *FeatureFlags) SocialFeaturesEnabled(ctx *FeatureContext) bool {
	return ff.IsEnabled(FeatureSocialHelpFinder, ctx) ||
		ff.IsEnabled(FeatureSocialStudyBuddy, ctx) ||
		ff.IsEnabled(FeatureSocialEndorsements, ctx)
}

// NotificationsEnabled checks if any notifications are enabled.
func (ff *FeatureFlags) NotificationsEnabled(ctx *FeatureContext) bool {
	return ff.IsEnabled(FeatureNotifyRankUp, ctx) ||
		ff.IsEnabled(FeatureNotifyTopEntry, ctx) ||
		ff.IsEnabled(FeatureNotifyDailyDigest, ctx) ||
		ff.IsEnabled(FeatureNotifyInactive, ctx)
}

// --- Errors ---

var (
	ErrFeatureNotFound       = &FeatureFlagError{Message: "feature not found"}
	ErrInvalidRolloutPercent = &FeatureFlagError{Message: "rollout percent must be 0-100"}
)

// FeatureFlagError represents a feature flag error.
type FeatureFlagError struct {
	Message string
}

func (e *FeatureFlagError) Error() string {
	return e.Message
}
