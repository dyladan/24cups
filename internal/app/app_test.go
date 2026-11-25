package app

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type client struct {
	t   *testing.T
	srv *httptest.Server
	c   *http.Client
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t, srv, &http.Client{Jar: jar}}
}

func (c *client) do(method, path, body string, out any) int {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.srv.URL+"/api"+path, strings.NewReader(body))
	req.Header.Set("X-Requested-With", "test")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

// testLimits keeps account protections but no general throttling, so tests
// that hammer the API from one IP aren't rate limited.
var testLimits = Limits{PasswordChecks: 3}

func setup(t *testing.T) *httptest.Server {
	return setupWith(t, testLimits)
}

func setupWith(t *testing.T, limits Limits) *httptest.Server {
	dbPath := filepath.Join(t.TempDir(), "t.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := SeedCoffees(db, []byte(`[{"day":1,"name":"One"},{"day":2,"name":"Two"}]`)); err != nil {
		t.Fatal(err)
	}
	rdb, err := OpenReadDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rdb.Close() })
	h, err := NewServer(db, rdb, Options{Limits: limits, ClientIPHeader: "X-Test-IP"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestRatingsAndSpoilers(t *testing.T) {
	srv := setup(t)
	a, b := newClient(t, srv), newClient(t, srv)
	if s := a.do("POST", "/auth/register", `{"username":"alice","password":"password1"}`, nil); s != 201 {
		t.Fatalf("register: %d", s)
	}
	b.do("POST", "/auth/register", `{"username":"bob","password":"password1"}`, nil)

	// Alice: two public brews (avg 4.0 stars) and an excluded private one.
	a.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":20,"isPublic":true}`, nil)
	a.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":12,"isPublic":true}`, nil)
	a.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":1,"isPublic":false,"excludeFromGlobal":true}`, nil)
	// Bob: a private brew that still counts (2.0 stars).
	b.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":8,"isPublic":false}`, nil)

	var c CoffeeDetail
	a.do("GET", "/coffees/1", "", &c)
	if c.Stats == nil || c.Stats.AvgRating == nil {
		t.Fatalf("missing stats: %+v", c)
	}
	// Per-user averages: alice 4.0, bob 2.0 → 3.0.
	if got := *c.Stats.AvgRating; got != 3.0 {
		t.Errorf("avg = %v, want 3", got)
	}
	if c.Stats.RatingCount != 2 || c.Stats.BrewCount != 3 || c.Stats.Opens != 2 {
		t.Errorf("stats = %+v", c.Stats)
	}

	// Alice sees bob's private brew? No: only public + own.
	var brews []Brew
	a.do("GET", "/coffees/1/brews", "", &brews)
	if len(brews) != 3 {
		t.Errorf("alice sees %d brews, want 3", len(brews))
	}

	// Coffee 2 is unopened for bob: no info, no stats, no brews.
	var list []Coffee
	b.do("GET", "/coffees", "", &list)
	if list[1].Info != nil || list[1].Stats != nil {
		t.Errorf("unopened coffee leaked: %+v", list[1])
	}
	// Favorite requires opening.
	if s := b.do("PUT", "/coffees/2/favorite", "", nil); s != 409 {
		t.Errorf("favorite unopened: %d", s)
	}
	b.do("POST", "/coffees/2/open", "", nil)
	if s := b.do("PUT", "/coffees/2/favorite", "", nil); s != 204 {
		t.Errorf("favorite: %d", s)
	}
}

func TestGroupsAndCSRF(t *testing.T) {
	srv := setup(t)
	a, b := newClient(t, srv), newClient(t, srv)
	a.do("POST", "/auth/register", `{"username":"alice","password":"password1"}`, nil)
	b.do("POST", "/auth/register", `{"username":"bob","password":"password1"}`, nil)

	var g Group
	a.do("POST", "/groups", `{"name":"Secret","isPublic":false}`, &g)
	if s := b.do("GET", "/groups/"+itoa(g.ID), "", nil); s != 404 {
		t.Errorf("private group visible to non-member: %d", s)
	}
	if s := b.do("POST", "/groups/"+itoa(g.ID)+"/join", "", nil); s != 404 {
		t.Errorf("joined private group without code: %d", s)
	}
	if s := b.do("POST", "/groups/join", `{"code":"https://x/join/`+g.InviteCode+`"}`, nil); s != 200 {
		t.Errorf("join by link: %d", s)
	}
	var d GroupDetail
	b.do("GET", "/groups/"+itoa(g.ID), "", &d)
	if len(d.Members) != 2 {
		t.Errorf("members = %d", len(d.Members))
	}

	// Missing X-Requested-With is rejected.
	req, _ := http.NewRequest("POST", srv.URL+"/api/groups", strings.NewReader(`{"name":"x"}`))
	res, _ := a.c.Do(req)
	if res.StatusCode != 403 {
		t.Errorf("csrf: %d", res.StatusCode)
	}
}

func itoa(i int64) string { b, _ := json.Marshal(i); return string(b) }
