package app

import (
	"net/http"
	"time"
)

// Anonymous (signed-out) visitors see the calendar opened up to "today":
// their own local date, but never later than the date in the earliest time
// zone on Earth (UTC+14), so a faked date can't unlock tomorrow's door.
// Community ratings are spoilers for them unless they turn spoilers on.

var earliestZone = time.FixedZone("UTC+14", 14*60*60)

// anonViewer builds the pseudo-user for a signed-out request.
func (s *Server) anonViewer(r *http.Request) *User {
	return &User{
		anon:         true,
		anonDay:      s.unlockedDay(r),
		anonSpoilers: r.URL.Query().Get("spoilers") == "1",
	}
}

// unlockedDay returns how many doors are open for an anonymous visitor:
// 0 before December 1 of the calendar year, the day of the month during
// December, and effectively all of them afterwards.
func (s *Server) unlockedDay(r *http.Request) int {
	now := s.now()
	latest := civil(now.In(earliestZone))
	day := civil(now.UTC())
	if q := r.URL.Query().Get("today"); q != "" {
		if t, err := time.Parse(time.DateOnly, q); err == nil {
			day = civil(t)
		}
	}
	if day.After(latest) {
		day = latest
	}
	start := time.Date(s.calendarYear, time.December, 1, 0, 0, 0, 0, time.UTC)
	switch {
	case day.Before(start):
		return 0
	case day.Year() > s.calendarYear:
		return 1 << 20
	default:
		return day.Day()
	}
}

// civil drops the time of day, keeping the calendar date (as UTC midnight).
func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// maybeAuthed serves signed-in users as authed does, and signed-out visitors
// as an anonymous viewer (subject only to the per-IP limit).
func (s *Server) maybeAuthed(fn authedFunc) http.HandlerFunc {
	return s.h(func(w http.ResponseWriter, r *http.Request) error {
		u, err := s.currentUser(r)
		if err != nil {
			return err
		}
		if u == nil {
			return fn(w, r, s.anonViewer(r))
		}
		return s.authedAs(w, r, u, fn)
	})
}
