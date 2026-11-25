package app

import (
	"net/http"
	"time"
)

// Anonymous (signed-out) visitors see everything for days before "today":
// coffee, tasting notes, ratings and public brews. Today's door stays sealed,
// so a member browsing signed out isn't spoiled before opening it, unless
// the visitor turns spoilers on. Later doors are always sealed.
//
// "Today" is the visitor's own local date, but never later than the date in
// the earliest time zone on Earth (UTC+14), so a faked date can't unlock a
// door early.

var earliestZone = time.FixedZone("UTC+14", 14*60*60)

// anonCanSee reports whether a signed-out visitor sees the given day.
func (u *User) anonCanSee(day int) bool {
	return day < u.anonDay || (day == u.anonDay && u.anonSpoilers)
}

// anonViewer builds the pseudo-user for a signed-out request.
func (s *Server) anonViewer(r *http.Request) *User {
	return &User{
		anon:         true,
		anonDay:      s.unlockedDay(r),
		anonSpoilers: r.URL.Query().Get("spoilers") == "1",
	}
}

// unlockedDay returns "today" as a door number: 0 before December 1 of the
// calendar year, the day of the month during December, and past every door
// afterwards. It decides which doors a signed-out visitor sees and, for
// everyone, which sealed door shows its name (Coffee.Preview).
func (s *Server) unlockedDay(r *http.Request) int {
	if s.devDay > 0 {
		return s.devDay
	}
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

// devToday is the pretend date for DevDay, as YYYY-MM-DD, or "".
func (s *Server) devToday() string {
	if s.devDay <= 0 {
		return ""
	}
	return time.Date(s.calendarYear, time.December, s.devDay, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
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
