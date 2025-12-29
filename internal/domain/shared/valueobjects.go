// Package shared contains common domain types, errors, events, and value objects
// that are used across all domain packages.
package shared

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════
// ID Value Objects
// ═══════════════════════════════════════════════════════════════════════════

// TelegramID represents a unique Telegram user identifier.
type TelegramID int64

// IsValid checks if the Telegram ID is valid (positive number).
func (t TelegramID) IsValid() bool {
	return t > 0
}

// Int64 returns the underlying int64 value.
func (t TelegramID) Int64() int64 {
	return int64(t)
}

// String returns the string representation.
func (t TelegramID) String() string {
	return fmt.Sprintf("%d", t)
}

// NewTelegramID creates a new TelegramID with validation.
func NewTelegramID(id int64) (TelegramID, error) {
	if id <= 0 {
		return 0, ErrInvalidTelegramID
	}
	return TelegramID(id), nil
}

// AlemID represents a unique Alem platform user identifier (login).
type AlemID string

// Regular expression for valid Alem login format.
var alemIDRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{2,29}$`)

// IsValid checks if the Alem ID is valid.
func (a AlemID) IsValid() bool {
	return alemIDRegex.MatchString(string(a))
}

// String returns the string representation.
func (a AlemID) String() string {
	return string(a)
}

// Normalize returns a normalized (lowercase) version of the Alem ID.
func (a AlemID) Normalize() AlemID {
	return AlemID(strings.ToLower(string(a)))
}

// NewAlemID creates a new AlemID with validation.
func NewAlemID(login string) (AlemID, error) {
	id := AlemID(strings.TrimSpace(login))
	if !id.IsValid() {
		return "", ErrInvalidAlemID
	}
	return id.Normalize(), nil
}

// StudentID represents a unique student identifier (UUID format).
type StudentID string

// UUID validation regex (simple version).
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsValid checks if the student ID is a valid UUID.
func (s StudentID) IsValid() bool {
	return uuidRegex.MatchString(string(s))
}

// String returns the string representation.
func (s StudentID) String() string {
	return string(s)
}

// IsEmpty checks if the ID is empty.
func (s StudentID) IsEmpty() bool {
	return s == ""
}

// NewStudentID creates a new StudentID with validation.
func NewStudentID(id string) (StudentID, error) {
	sid := StudentID(strings.ToLower(strings.TrimSpace(id)))
	if !sid.IsValid() {
		return "", NewDomainError("shared", "NewStudentID", ErrInvalidID, "invalid student ID format")
	}
	return sid, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// XP Value Object (Experience Points)
// ═══════════════════════════════════════════════════════════════════════════

// XP represents experience points earned by a student.
type XP int

const (
	// XP boundaries
	MinXP XP = 0
	MaxXP XP = 1000000 // 1 million XP cap
)

// IsValid checks if the XP value is within valid range.
func (x XP) IsValid() bool {
	return x >= MinXP && x <= MaxXP
}

// Int returns the underlying int value.
func (x XP) Int() int {
	return int(x)
}

// Add adds XP and returns the result, capped at MaxXP.
func (x XP) Add(amount int) XP {
	result := XP(int(x) + amount)
	if result > MaxXP {
		return MaxXP
	}
	if result < MinXP {
		return MinXP
	}
	return result
}

// Subtract subtracts XP and returns the result, floored at MinXP.
func (x XP) Subtract(amount int) XP {
	result := XP(int(x) - amount)
	if result < MinXP {
		return MinXP
	}
	return result
}

// Level calculates the level based on XP.
// Uses a simple progression: Level = sqrt(XP / 100)
func (x XP) Level() Level {
	if x <= 0 {
		return 1
	}
	// Simple leveling formula: every 100 XP = 1 level, with diminishing returns
	level := 1
	requiredXP := 100
	totalRequired := 0
	for totalRequired+requiredXP <= int(x) {
		totalRequired += requiredXP
		level++
		requiredXP = 100 * level // Each level requires more XP
	}
	return Level(level)
}

// ProgressToNextLevel returns percentage progress to next level (0-100).
func (x XP) ProgressToNextLevel() int {
	currentLevel := x.Level()
	currentLevelXP := currentLevel.RequiredXP()
	nextLevelXP := (currentLevel + 1).RequiredXP()

	xpInCurrentLevel := int(x) - currentLevelXP
	xpNeededForLevel := nextLevelXP - currentLevelXP

	if xpNeededForLevel == 0 {
		return 100
	}

	return (xpInCurrentLevel * 100) / xpNeededForLevel
}

// NewXP creates a new XP value with validation.
func NewXP(amount int) (XP, error) {
	if amount < int(MinXP) {
		return 0, NewDomainError("shared", "NewXP", ErrNegativeValue, "XP cannot be negative")
	}
	if amount > int(MaxXP) {
		return MaxXP, nil // Cap at max
	}
	return XP(amount), nil
}

// ═══════════════════════════════════════════════════════════════════════════
// Level Value Object
// ═══════════════════════════════════════════════════════════════════════════

// Level represents a student's level.
type Level int

const (
	MinLevel Level = 1
	MaxLevel Level = 100
)

// IsValid checks if the level is within valid range.
func (l Level) IsValid() bool {
	return l >= MinLevel && l <= MaxLevel
}

// Int returns the underlying int value.
func (l Level) Int() int {
	return int(l)
}

// RequiredXP returns the total XP required to reach this level.
func (l Level) RequiredXP() int {
	if l <= 1 {
		return 0
	}
	total := 0
	for i := Level(1); i < l; i++ {
		total += 100 * int(i)
	}
	return total
}

// Title returns a human-readable title for the level.
func (l Level) Title() string {
	switch {
	case l < 5:
		return "Новичок"
	case l < 10:
		return "Ученик"
	case l < 20:
		return "Студент"
	case l < 30:
		return "Практик"
	case l < 50:
		return "Специалист"
	case l < 75:
		return "Эксперт"
	default:
		return "Мастер"
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Rank Value Object
// ═══════════════════════════════════════════════════════════════════════════

// Rank represents a student's position in the leaderboard.
type Rank int

const (
	MinRank  Rank = 1
	Unranked Rank = 0 // Not yet ranked
)

// IsValid checks if the rank is valid.
func (r Rank) IsValid() bool {
	return r >= MinRank
}

// Int returns the underlying int value.
func (r Rank) Int() int {
	return int(r)
}

// IsUnranked checks if the student is not yet ranked.
func (r Rank) IsUnranked() bool {
	return r == Unranked
}

// IsTop returns true if the rank is in the top N.
func (r Rank) IsTop(n int) bool {
	return r.IsValid() && int(r) <= n
}

// IsTop10 checks if in top 10.
func (r Rank) IsTop10() bool {
	return r.IsTop(10)
}

// IsTop50 checks if in top 50.
func (r Rank) IsTop50() bool {
	return r.IsTop(50)
}

// IsTop100 checks if in top 100.
func (r Rank) IsTop100() bool {
	return r.IsTop(100)
}

// Medal returns a medal emoji for top ranks.
func (r Rank) Medal() string {
	switch r {
	case 1:
		return "🥇"
	case 2:
		return "🥈"
	case 3:
		return "🥉"
	default:
		return ""
	}
}

// Compare returns the difference between two ranks.
// Positive value means improvement (moved up), negative means dropped.
func (r Rank) Compare(other Rank) int {
	return int(other) - int(r)
}

// NewRank creates a new Rank with validation.
func NewRank(position int) (Rank, error) {
	if position < 0 {
		return Unranked, NewDomainError("shared", "NewRank", ErrNegativeValue, "rank cannot be negative")
	}
	return Rank(position), nil
}

// ═══════════════════════════════════════════════════════════════════════════
// Cohort Value Object
// ═══════════════════════════════════════════════════════════════════════════

// Cohort represents a student cohort (enrollment period).
type Cohort string

// Common cohort format: "2024-spring", "2024-fall"
var cohortRegex = regexp.MustCompile(`^\d{4}-(spring|summer|fall|winter)$`)

// IsValid checks if the cohort format is valid.
func (c Cohort) IsValid() bool {
	return cohortRegex.MatchString(string(c))
}

// String returns the string representation.
func (c Cohort) String() string {
	return string(c)
}

// Year extracts the year from the cohort.
func (c Cohort) Year() int {
	if len(c) < 4 {
		return 0
	}
	year := 0
	fmt.Sscanf(string(c[:4]), "%d", &year)
	return year
}

// Season extracts the season from the cohort.
func (c Cohort) Season() string {
	parts := strings.Split(string(c), "-")
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

// NewCohort creates a new Cohort with validation.
func NewCohort(value string) (Cohort, error) {
	c := Cohort(strings.ToLower(strings.TrimSpace(value)))
	if !c.IsValid() {
		return "", NewDomainError("shared", "NewCohort", ErrInvalidFormat, "invalid cohort format, expected YYYY-season")
	}
	return c, nil
}

// CurrentCohort returns the cohort for the current date.
func CurrentCohort() Cohort {
	now := time.Now()
	year := now.Year()
	month := now.Month()

	var season string
	switch {
	case month >= 3 && month <= 5:
		season = "spring"
	case month >= 6 && month <= 8:
		season = "summer"
	case month >= 9 && month <= 11:
		season = "fall"
	default:
		season = "winter"
		if month == 1 || month == 2 {
			// January/February belong to winter of the previous year's cohort
		}
	}

	return Cohort(fmt.Sprintf("%d-%s", year, season))
}

// ═══════════════════════════════════════════════════════════════════════════
// Rating Value Object (for endorsements)
// ═══════════════════════════════════════════════════════════════════════════

// Rating represents a rating value (1-5 stars).
type Rating int

const (
	MinRating Rating = 1
	MaxRating Rating = 5
)

// IsValid checks if the rating is within valid range.
func (r Rating) IsValid() bool {
	return r >= MinRating && r <= MaxRating
}

// Int returns the underlying int value.
func (r Rating) Int() int {
	return int(r)
}

// Stars returns the rating as star emojis.
func (r Rating) Stars() string {
	filled := int(r)
	empty := int(MaxRating) - filled
	return strings.Repeat("⭐", filled) + strings.Repeat("☆", empty)
}

// NewRating creates a new Rating with validation.
func NewRating(value int) (Rating, error) {
	if value < int(MinRating) || value > int(MaxRating) {
		return 0, ErrInvalidRating
	}
	return Rating(value), nil
}

// AverageRating calculates the average from a slice of ratings.
func AverageRating(ratings []Rating) float64 {
	if len(ratings) == 0 {
		return 0
	}
	sum := 0
	for _, r := range ratings {
		sum += int(r)
	}
	return float64(sum) / float64(len(ratings))
}

// ═══════════════════════════════════════════════════════════════════════════
// TaskID Value Object
// ═══════════════════════════════════════════════════════════════════════════

// TaskID represents a unique task identifier.
type TaskID string

// Task ID format: category-name-number (e.g., "go-intro-01", "graph-bfs-03")
var taskIDRegex = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// IsValid checks if the task ID format is valid.
func (t TaskID) IsValid() bool {
	s := string(t)
	return len(s) >= 3 && len(s) <= 50 && taskIDRegex.MatchString(s)
}

// String returns the string representation.
func (t TaskID) String() string {
	return string(t)
}

// Category extracts the category from the task ID.
func (t TaskID) Category() string {
	parts := strings.Split(string(t), "-")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// NewTaskID creates a new TaskID with validation.
func NewTaskID(id string) (TaskID, error) {
	tid := TaskID(strings.ToLower(strings.TrimSpace(id)))
	if !tid.IsValid() {
		return "", NewDomainError("shared", "NewTaskID", ErrInvalidID, "invalid task ID format")
	}
	return tid, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TimeRange Value Object
// ═══════════════════════════════════════════════════════════════════════════

// TimeRange represents a time period.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// IsValid checks if the time range is valid.
func (t TimeRange) IsValid() bool {
	return !t.From.IsZero() && !t.To.IsZero() && !t.From.After(t.To)
}

// Duration returns the duration of the time range.
func (t TimeRange) Duration() time.Duration {
	return t.To.Sub(t.From)
}

// Contains checks if a time is within the range.
func (t TimeRange) Contains(tm time.Time) bool {
	return (tm.Equal(t.From) || tm.After(t.From)) && (tm.Equal(t.To) || tm.Before(t.To))
}

// NewTimeRange creates a new TimeRange with validation.
func NewTimeRange(from, to time.Time) (TimeRange, error) {
	tr := TimeRange{From: from, To: to}
	if !tr.IsValid() {
		return TimeRange{}, NewDomainError("shared", "NewTimeRange", ErrInvalidInput, "'from' must be before 'to'")
	}
	return tr, nil
}

// Today returns a TimeRange for today (local time).
func Today() TimeRange {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(24 * time.Hour).Add(-time.Nanosecond)
	return TimeRange{From: start, To: end}
}

// Last24Hours returns a TimeRange for the last 24 hours.
func Last24Hours() TimeRange {
	now := time.Now()
	return TimeRange{
		From: now.Add(-24 * time.Hour),
		To:   now,
	}
}

// LastNDays returns a TimeRange for the last N days.
func LastNDays(n int) TimeRange {
	now := time.Now()
	return TimeRange{
		From: now.AddDate(0, 0, -n),
		To:   now,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Pagination Value Object
// ═══════════════════════════════════════════════════════════════════════════

// Pagination represents pagination parameters.
type Pagination struct {
	Page     int
	PageSize int
}

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Offset returns the offset for database queries.
func (p Pagination) Offset() int {
	if p.Page <= 0 {
		return 0
	}
	return (p.Page - 1) * p.Limit()
}

// Limit returns the limit for database queries.
func (p Pagination) Limit() int {
	if p.PageSize <= 0 {
		return DefaultPageSize
	}
	if p.PageSize > MaxPageSize {
		return MaxPageSize
	}
	return p.PageSize
}

// NewPagination creates a new Pagination with defaults.
func NewPagination(page, pageSize int) Pagination {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return Pagination{Page: page, PageSize: pageSize}
}

// DefaultPagination returns default pagination.
func DefaultPagination() Pagination {
	return NewPagination(1, DefaultPageSize)
}
