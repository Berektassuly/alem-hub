package scheduler

import (
	"fmt"
	"time"
)

// IntervalSchedule schedules a job to run at a fixed interval.
type IntervalSchedule struct {
	Interval time.Duration
}

// NewIntervalSchedule creates a new IntervalSchedule.
func NewIntervalSchedule(interval time.Duration) *IntervalSchedule {
	return &IntervalSchedule{
		Interval: interval,
	}
}

// Next returns the next scheduled time.
func (s *IntervalSchedule) Next(t time.Time) time.Time {
	return t.Add(s.Interval)
}

// String returns the string representation of the schedule.
func (s *IntervalSchedule) String() string {
	return fmt.Sprintf("@every %s", s.Interval.String())
}
