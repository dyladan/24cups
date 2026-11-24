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

	// A member rates day 1 (public) and day 2 (private).
	m := newClient(t, srv)
	m.do("POST", "/auth/register", `{"username":"member","password":"password1"}`, nil)
	m.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":8,"isPublic":true,"notes":"public"}`, nil)
	m.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":2,"isPublic":false,"notes":"private"}`, nil)

	list := anonGet(t, srv, "")
	if n := opened(list); n != 3 {
		t.Errorf("default (UTC date): %d doors open, want 3", n)
	}
	d1, d4 := list[0], list[3]
	if d1.Info == nil || d1.Info.TastingNotes != "Notes 1" {
		t.Errorf("anon should see tasting notes on open doors: %+v", d1.Info)
	}
	if d1.Revealed || d1.Stats == nil || d1.Stats.AvgRating != nil || d1.Stats.Opens != 1 {
		t.Errorf("ratings leaked without spoilers: %+v", d1.Stats)
	}
	if d4.Info != nil || d4.Stats != nil {
		t.Errorf("future door leaked: %+v", d4)
	}

	if n := opened(anonGet(t, srv, "?today=2026-12-02")); n != 2 {
		t.Errorf("client behind UTC: %d open, want 2", n)
	}
	if n := opened(anonGet(t, srv, "?today=2026-12-20")); n != 4 {
		t.Errorf("faked future date: %d open, want 4 (capped at UTC+14)", n)
	}

	// Spoilers on: ratings and public brews only.
	list = anonGet(t, srv, "?spoilers=1")
	if !list[0].Revealed || list[0].Stats.AvgRating == nil {
		t.Errorf("spoilers on but ratings hidden: %+v", list[0].Stats)
	}
	c := newClient(t, srv)
	var brews []Brew
	c.do("GET", "/coffees/1/brews", "", &brews)
	if len(brews) != 0 {
		t.Errorf("brews shown without spoilers: %d", len(brews))
	}
	c.do("GET", "/coffees/1/brews?spoilers=1", "", &brews)
	if len(brews) != 1 || brews[0].Notes != "public" {
		t.Errorf("want only the public brew, got %+v", brews)
	}
	c.do("GET", "/coffees/10/brews?spoilers=1", "", &brews)
	if len(brews) != 0 {
		t.Errorf("future coffee brews: %d", len(brews))
	}
	var detail CoffeeDetail
	c.do("GET", "/coffees/10?spoilers=1", "", &detail)
	if detail.Info != nil || detail.Stats != nil {
		t.Errorf("future coffee detail leaked: %+v", detail)
	}

	// Signed-out visitors can't change anything.
	if s := c.do("POST", "/coffees/1/open", "", nil); s != http.StatusUnauthorized {
		t.Errorf("anon open: %d", s)
	}
}

func TestAnonymousBeforeAndAfterDecember(t *testing.T) {
	if n := opened(anonGet(t, setupAt(t, time.Date(2026, 11, 30, 9, 0, 0, 0, time.UTC)), "")); n != 0 {
		t.Errorf("Nov 30: %d open", n)
	}
	if n := opened(anonGet(t, setupAt(t, time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC)), "")); n != 24 {
		t.Errorf("January: %d open", n)
	}
}
