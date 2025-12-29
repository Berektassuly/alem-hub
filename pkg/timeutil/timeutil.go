// Package timeutil provides timezone utilities for Almaty timezone (UTC+5).
// This is essential for Alem Community Hub as all students are located in Almaty.
// Handles date formatting, business hours, and timezone-aware time operations.
// No external dependencies - uses only standard library.
package timeutil

import (
	"fmt"
	"time"
)

// AlmatyTZ is the Almaty timezone (UTC+5, no DST).
// Kazakhstan abolished DST in 2005, so this is constant year-round.
var AlmatyTZ = time.FixedZone("Asia/Almaty", 5*60*60)

// Now returns the current time in Almaty timezone.
func Now() time.Time {
	return time.Now().In(AlmatyTZ)
}

// ToAlmaty converts a time to Almaty timezone.
func ToAlmaty(t time.Time) time.Time {
	return t.In(AlmatyTZ)
}

// ToUTC converts a time to UTC.
func ToUTC(t time.Time) time.Time {
	return t.UTC()
}

// Date creates a time in Almaty timezone with the given date.
func Date(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, AlmatyTZ)
}

// DateTime creates a time in Almaty timezone with the given date and time.
func DateTime(year, month, day, hour, min, sec int) time.Time {
	return time.Date(year, time.Month(month), day, hour, min, sec, 0, AlmatyTZ)
}

// StartOfDay returns the start of the day (00:00:00) in Almaty timezone.
func StartOfDay(t time.Time) time.Time {
	almaty := ToAlmaty(t)
	return time.Date(almaty.Year(), almaty.Month(), almaty.Day(), 0, 0, 0, 0, AlmatyTZ)
}

// EndOfDay returns the end of the day (23:59:59.999999999) in Almaty timezone.
func EndOfDay(t time.Time) time.Time {
	almaty := ToAlmaty(t)
	return time.Date(almaty.Year(), almaty.Month(), almaty.Day(), 23, 59, 59, 999999999, AlmatyTZ)
}

// StartOfWeek returns the start of the week (Monday 00:00:00) in Almaty timezone.
func StartOfWeek(t time.Time) time.Time {
	almaty := ToAlmaty(t)
	weekday := int(almaty.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday
	}
	daysToSubtract := weekday - 1 // Monday = 1
	return StartOfDay(almaty.AddDate(0, 0, -daysToSubtract))
}

// EndOfWeek returns the end of the week (Sunday 23:59:59) in Almaty timezone.
func EndOfWeek(t time.Time) time.Time {
	start := StartOfWeek(t)
	return EndOfDay(start.AddDate(0, 0, 6))
}

// StartOfMonth returns the start of the month in Almaty timezone.
func StartOfMonth(t time.Time) time.Time {
	almaty := ToAlmaty(t)
	return time.Date(almaty.Year(), almaty.Month(), 1, 0, 0, 0, 0, AlmatyTZ)
}

// EndOfMonth returns the end of the month in Almaty timezone.
func EndOfMonth(t time.Time) time.Time {
	start := StartOfMonth(t)
	return EndOfDay(start.AddDate(0, 1, -1))
}

// IsToday checks if the given time is today in Almaty timezone.
func IsToday(t time.Time) bool {
	now := Now()
	almaty := ToAlmaty(t)
	return almaty.Year() == now.Year() &&
		almaty.Month() == now.Month() &&
		almaty.Day() == now.Day()
}

// IsYesterday checks if the given time is yesterday in Almaty timezone.
func IsYesterday(t time.Time) bool {
	yesterday := Now().AddDate(0, 0, -1)
	almaty := ToAlmaty(t)
	return almaty.Year() == yesterday.Year() &&
		almaty.Month() == yesterday.Month() &&
		almaty.Day() == yesterday.Day()
}

// IsThisWeek checks if the given time is in the current week.
func IsThisWeek(t time.Time) bool {
	now := Now()
	weekStart := StartOfWeek(now)
	weekEnd := EndOfWeek(now)
	almaty := ToAlmaty(t)
	return !almaty.Before(weekStart) && !almaty.After(weekEnd)
}

// DaysSince calculates the number of days since the given time.
func DaysSince(t time.Time) int {
	now := StartOfDay(Now())
	then := StartOfDay(t)
	duration := now.Sub(then)
	return int(duration.Hours() / 24)
}

// Business hours for Alem bootcamp.
const (
	// WorkdayStart is when the workday starts (9:00 AM).
	WorkdayStart = 9
	// WorkdayEnd is when the workday ends (9:00 PM).
	WorkdayEnd = 21
	// OfficeOpenTime is when the physical office opens.
	OfficeOpenTime = 8
	// OfficeCloseTime is when the physical office closes.
	OfficeCloseTime = 23
)

// IsBusinessHours checks if the given time is within business hours (9:00-21:00).
func IsBusinessHours(t time.Time) bool {
	almaty := ToAlmaty(t)
	hour := almaty.Hour()
	return hour >= WorkdayStart && hour < WorkdayEnd
}

// IsOfficeOpen checks if the Alem office is open (8:00-23:00).
func IsOfficeOpen(t time.Time) bool {
	almaty := ToAlmaty(t)
	hour := almaty.Hour()
	return hour >= OfficeOpenTime && hour < OfficeCloseTime
}

// IsWeekend checks if the given time is on a weekend.
func IsWeekend(t time.Time) bool {
	almaty := ToAlmaty(t)
	weekday := almaty.Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}

// IsWorkday checks if the given time is on a workday (Mon-Fri).
func IsWorkday(t time.Time) bool {
	return !IsWeekend(t)
}

// NextWorkday returns the next workday (skipping weekends).
func NextWorkday(t time.Time) time.Time {
	next := ToAlmaty(t).AddDate(0, 0, 1)
	for IsWeekend(next) {
		next = next.AddDate(0, 0, 1)
	}
	return StartOfDay(next)
}

// Common date/time formats.
const (
	// FormatDate is the standard date format (YYYY-MM-DD).
	FormatDate = "2006-01-02"
	// FormatTime is the standard time format (HH:MM).
	FormatTime = "15:04"
	// FormatDateTime is the standard datetime format.
	FormatDateTime = "2006-01-02 15:04"
	// FormatDateTimeSeconds includes seconds.
	FormatDateTimeSeconds = "2006-01-02 15:04:05"
	// FormatRussianDate is the Russian date format (DD.MM.YYYY).
	FormatRussianDate = "02.01.2006"
	// FormatRussianDateTime is the Russian datetime format.
	FormatRussianDateTime = "02.01.2006 15:04"
	// FormatHumanDate is a human-readable format.
	FormatHumanDate = "2 January 2006"
	// FormatShortDate is a short format (Jan 2).
	FormatShortDate = "Jan 2"
)

// FormatAlmaty formats a time in Almaty timezone with the given layout.
func FormatAlmaty(t time.Time, layout string) string {
	return ToAlmaty(t).Format(layout)
}

// FormatDateStr formats a time as a date string (YYYY-MM-DD) in Almaty timezone.
func FormatDateStr(t time.Time) string {
	return FormatAlmaty(t, FormatDate)
}

// FormatTimeStr formats a time as a time string (HH:MM) in Almaty timezone.
func FormatTimeStr(t time.Time) string {
	return FormatAlmaty(t, FormatTime)
}

// FormatDateTimeStr formats a time as datetime string in Almaty timezone.
func FormatDateTimeStr(t time.Time) string {
	return FormatAlmaty(t, FormatDateTime)
}

// FormatRussian formats a time in Russian format (DD.MM.YYYY).
func FormatRussian(t time.Time) string {
	return FormatAlmaty(t, FormatRussianDate)
}

// FormatRelative returns a human-readable relative time string.
func FormatRelative(t time.Time) string {
	now := Now()
	almaty := ToAlmaty(t)
	duration := now.Sub(almaty)

	if duration < 0 {
		duration = -duration
		return formatFutureDuration(duration)
	}

	return formatPastDuration(duration)
}

func formatPastDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "только что"
	case d < time.Hour:
		mins := int(d.Minutes())
		return fmt.Sprintf("%d мин назад", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		return fmt.Sprintf("%d ч назад", hours)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "вчера"
		}
		return fmt.Sprintf("%d дн назад", days)
	case d < 30*24*time.Hour:
		weeks := int(d.Hours() / 24 / 7)
		return fmt.Sprintf("%d нед назад", weeks)
	default:
		months := int(d.Hours() / 24 / 30)
		if months < 12 {
			return fmt.Sprintf("%d мес назад", months)
		}
		years := months / 12
		return fmt.Sprintf("%d г назад", years)
	}
}

func formatFutureDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "сейчас"
	case d < time.Hour:
		mins := int(d.Minutes())
		return fmt.Sprintf("через %d мин", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		return fmt.Sprintf("через %d ч", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "завтра"
		}
		return fmt.Sprintf("через %d дн", days)
	}
}

// ParseAlmaty parses a time string in Almaty timezone.
func ParseAlmaty(layout, value string) (time.Time, error) {
	return time.ParseInLocation(layout, value, AlmatyTZ)
}

// ParseDateAlmaty parses a date string (YYYY-MM-DD) in Almaty timezone.
func ParseDateAlmaty(value string) (time.Time, error) {
	return ParseAlmaty(FormatDate, value)
}

// ParseDateTimeAlmaty parses a datetime string in Almaty timezone.
func ParseDateTimeAlmaty(value string) (time.Time, error) {
	return ParseAlmaty(FormatDateTime, value)
}

// Streak-related utilities for Daily Grind tracking.

// IsSameDay checks if two times are on the same day in Almaty timezone.
func IsSameDay(t1, t2 time.Time) bool {
	a1, a2 := ToAlmaty(t1), ToAlmaty(t2)
	return a1.Year() == a2.Year() && a1.YearDay() == a2.YearDay()
}

// IsConsecutiveDay checks if t2 is the day after t1.
func IsConsecutiveDay(t1, t2 time.Time) bool {
	a1, a2 := ToAlmaty(t1), ToAlmaty(t2)
	nextDay := a1.AddDate(0, 0, 1)
	return IsSameDay(nextDay, a2)
}

// DaysBetween calculates the number of days between two times.
func DaysBetween(t1, t2 time.Time) int {
	a1 := StartOfDay(t1)
	a2 := StartOfDay(t2)
	duration := a2.Sub(a1)
	days := int(duration.Hours() / 24)
	if days < 0 {
		days = -days
	}
	return days
}

// Notification timing helpers.

// IsSafeNotificationTime checks if it's appropriate to send notifications (9:00-22:00).
func IsSafeNotificationTime(t time.Time) bool {
	almaty := ToAlmaty(t)
	hour := almaty.Hour()
	return hour >= 9 && hour < 22
}

// NextSafeNotificationTime returns the next time when notifications are appropriate.
func NextSafeNotificationTime(t time.Time) time.Time {
	almaty := ToAlmaty(t)
	hour := almaty.Hour()

	if hour < 9 {
		// Before 9 AM - return 9 AM today
		return DateTime(almaty.Year(), int(almaty.Month()), almaty.Day(), 9, 0, 0)
	} else if hour >= 22 {
		// After 10 PM - return 9 AM tomorrow
		tomorrow := almaty.AddDate(0, 0, 1)
		return DateTime(tomorrow.Year(), int(tomorrow.Month()), tomorrow.Day(), 9, 0, 0)
	}

	// Already in safe time window
	return almaty
}

// WeekdayNameRu returns the Russian name for a weekday.
func WeekdayNameRu(t time.Time) string {
	almaty := ToAlmaty(t)
	switch almaty.Weekday() {
	case time.Monday:
		return "Понедельник"
	case time.Tuesday:
		return "Вторник"
	case time.Wednesday:
		return "Среда"
	case time.Thursday:
		return "Четверг"
	case time.Friday:
		return "Пятница"
	case time.Saturday:
		return "Суббота"
	case time.Sunday:
		return "Воскресенье"
	default:
		return ""
	}
}

// MonthNameRu returns the Russian name for a month.
func MonthNameRu(m time.Month) string {
	names := []string{
		"", "Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
		"Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь",
	}
	if int(m) >= 1 && int(m) <= 12 {
		return names[m]
	}
	return ""
}
