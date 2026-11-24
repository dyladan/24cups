package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func setupAt(t *testing.T, now time.Time) *httptest.Server {
	path := filepath.Join(t.TempDir(), "t.db")
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var seed = `[`
	for d := 1; d <= 24; d++ {
		if d > 1 {
			seed += ","
		}
		seed += `{"day":` + itoa(int64(d)) + `,"name":"C` + itoa(int64(d)) + `","tastingNotes":"Notes ` + itoa(int64(d)) + `"}`
	}
	if err := SeedCoffees(db, []byte(seed+`]`)); err != nil {
		t.Fatal(err)
	}
	rdb, _ := OpenReadDB(path)
	t.Cleanup(func() { rdb.Close() })
	h, err := NewServer(db, rdb, Options{Limits: testLimits, CalendarYear: 2026, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func anonGet(t *testing.T, srv *httptest.Server, query string) []Coffee {
	t.Helper()
	var list []Coffee
	c := newClient(t, srv)
	if s := c.do("GET", "/coffees"+query, "", &list); s != 200 {
		t.Fatalf("anon GET /coffees%s: %d", query, s)
	}
	return list
}

func opened(list []Coffee) int {
	n := 0
	for _, c := range list {
		if c.Opened {
			n++
		}
	}
	return n
}

func TestAnonymousCalendar(t *testing.T) {
	// Dec 3, 12:00 UTC: it's already Dec 4 in UTC+14.
	srv := setupAt(t, time.Date(2026, 12, 3, 12, 0, 0, 0, time.UTC))

	// A member rates day 1 (public + private) and today's coffee, day 3.
	m := newClient(t, srv)
	m.do("POST", "/auth/register", `{"username":"member","password":"password1"}`, nil)
	m.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":8,"isPublic":true,"notes":"public"}`, nil)
	m.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":2,"isPublic":false,"notes":"private"}`, nil)
	m.do("POST", "/brews", `{"coffeeId":3,"method":"V60","ratio":16,"rating":7,"isPublic":true,"notes":"today"}`, nil)

	// Past days: everything, no toggle needed. Today and later: sealed.
	list := anonGet(t, srv, "")
	if n := opened(list); n != 2 {
		t.Errorf("default (UTC date Dec 3): %d doors visible, want 2 (days before today)", n)
	}
	d1, d3 := list[0], list[2]
	if !d1.Revealed || d1.Info == nil || d1.Info.TastingNotes != "Notes 1" || d1.Stats == nil || d1.Stats.AvgRating == nil {
		t.Errorf("past day should be fully visible: revealed=%v info=%+v stats=%+v", d1.Revealed, d1.Info, d1.Stats)
	}
	if d3.Opened || d3.Info != nil || d3.Stats != nil {
		t.Errorf("today's door leaked without spoilers: %+v", d3)
	}
	c := newClient(t, srv)
	var brews []Brew
	c.do("GET", "/coffees/1/brews", "", &brews)
	if len(brews) != 1 || brews[0].Notes != "public" {
		t.Errorf("past day: want only the public brew, got %+v", brews)
	}
	c.do("GET", "/coffees/3/brews", "", &brews)
	if len(brews) != 0 {
		t.Errorf("today's brews leaked: %d", len(brews))
	}
	var detail CoffeeDetail
	c.do("GET", "/coffees/3", "", &detail)
	if detail.Info != nil || detail.Stats != nil {
		t.Errorf("today's detail leaked: %+v", detail)
	}

	// Spoilers on: today's door too, but nothing later.
	list = anonGet(t, srv, "?spoilers=1")
	if n := opened(list); n != 3 || !list[2].Revealed || list[2].Info.TastingNotes != "Notes 3" {
		t.Errorf("spoilers on: %d visible, today=%+v", n, list[2])
	}
	c.do("GET", "/coffees/3/brews?spoilers=1", "", &brews)
	if len(brews) != 1 {
		t.Errorf("spoilers on: today's brews %d, want 1", len(brews))
	}
	c.do("GET", "/coffees/4/brews?spoilers=1", "", &brews)
	c.do("GET", "/coffees/4?spoilers=1", "", &detail)
	if len(brews) != 0 || detail.Info != nil {
		t.Errorf("tomorrow leaked with spoilers on")
	}

	// The visitor's own date is honored when behind, capped when ahead.
	if n := opened(anonGet(t, srv, "?today=2026-12-02")); n != 1 {
		t.Errorf("client on Dec 2: %d visible, want 1", n)
	}
	if n := opened(anonGet(t, srv, "?today=2026-12-20&spoilers=1")); n != 4 {
		t.Errorf("faked Dec 20: %d visible, want 4 (capped at Dec 4 in UTC+14)", n)
	}

	// Signed-out visitors can't change anything.
	if s := c.do("POST", "/coffees/1/open", "", nil); s != http.StatusUnauthorized {
		t.Errorf("anon open: %d", s)
	}
}

func TestAnonymousBeforeAndAfterDecember(t *testing.T) {
	if n := opened(anonGet(t, setupAt(t, time.Date(2026, 11, 30, 9, 0, 0, 0, time.UTC)), "?spoilers=1")); n != 0 {
		t.Errorf("Nov 30: %d open", n)
	}
	if n := opened(anonGet(t, setupAt(t, time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC)), "")); n != 24 {
		t.Errorf("January: %d open", n)
	}
}
