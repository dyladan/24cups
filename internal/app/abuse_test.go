package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// doIP sends a request as if from ip (via the trusted test header).
func (c *client) doIP(ip, method, path, body string) *http.Response {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.srv.URL+"/api"+path, strings.NewReader(body))
	req.Header.Set("X-Requested-With", "test")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-IP", ip)
	res, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	res.Body.Close()
	return res
}

func login(name, pw string) string {
	return fmt.Sprintf(`{"username":%q,"password":%q}`, name, pw)
}

func TestLoginRateLimitPerIP(t *testing.T) {
	srv := setupWith(t, Limits{LoginPerMinute: 3, PasswordChecks: 3})
	c := newClient(t, srv)
	for i := 0; i < 3; i++ {
		if s := c.doIP("1.1.1.1", "POST", "/auth/login", login("nobody", "x")).StatusCode; s != 401 {
			t.Fatalf("attempt %d: %d", i, s)
		}
	}
	res := c.doIP("1.1.1.1", "POST", "/auth/login", login("nobody", "x"))
	if res.StatusCode != 429 || res.Header.Get("Retry-After") == "" {
		t.Fatalf("4th attempt: %d retry-after=%q", res.StatusCode, res.Header.Get("Retry-After"))
	}
	if s := c.doIP("2.2.2.2", "POST", "/auth/login", login("nobody", "x")).StatusCode; s != 401 {
		t.Errorf("other IP affected: %d", s)
	}
}

func TestLoginLockoutPerUsernameAcrossIPs(t *testing.T) {
	srv := setupWith(t, Limits{LoginFailsPer15Min: 3, PasswordChecks: 3})
	c := newClient(t, srv)
	c.doIP("9.9.9.9", "POST", "/auth/register", `{"username":"victim","password":"password1"}`)
	c.doIP("9.9.9.9", "POST", "/auth/register", `{"username":"bystander","password":"password1"}`)
	for i := 0; i < 3; i++ { // a distributed guesser
		c.doIP(fmt.Sprintf("10.0.0.%d", i), "POST", "/auth/login", login("Victim", "wrong"))
	}
	if s := c.doIP("10.0.0.99", "POST", "/auth/login", login("victim", "password1")).StatusCode; s != 429 {
		t.Errorf("victim not locked after failures: %d", s)
	}
	if s := c.doIP("10.0.0.99", "POST", "/auth/login", login("bystander", "password1")).StatusCode; s != 200 {
		t.Errorf("other account affected: %d", s)
	}
}

func TestFailedLoginIsDelayed(t *testing.T) {
	srv := setupWith(t, Limits{FailedLoginDelay: 300 * time.Millisecond, PasswordChecks: 3})
	c := newClient(t, srv)
	c.doIP("1.1.1.1", "POST", "/auth/register", `{"username":"dan","password":"password1"}`)
	for _, tc := range []struct{ name, body string }{
		{"wrong password", login("dan", "nope")},
		{"unknown user", login("ghost", "nope")},
	} {
		t0 := time.Now()
		c.doIP("1.1.1.1", "POST", "/auth/login", tc.body)
		if d := time.Since(t0); d < 300*time.Millisecond {
			t.Errorf("%s answered in %v, want >= 300ms", tc.name, d)
		}
	}
	t0 := time.Now()
	if s := c.doIP("1.1.1.1", "POST", "/auth/login", login("dan", "password1")).StatusCode; s != 200 {
		t.Fatalf("good login: %d", s)
	}
	if d := time.Since(t0); d >= 300*time.Millisecond {
		t.Errorf("successful login was delayed: %v", d)
	}
}

func TestSignupRateLimitPerIP(t *testing.T) {
	srv := setupWith(t, Limits{SignupPerHour: 2, PasswordChecks: 3})
	c := newClient(t, srv)
	for i := 0; i < 2; i++ {
		if s := c.doIP("5.5.5.5", "POST", "/auth/register", fmt.Sprintf(`{"username":"spam%d","password":"password1"}`, i)).StatusCode; s != 201 {
			t.Fatalf("signup %d: %d", i, s)
		}
	}
	if s := c.doIP("5.5.5.5", "POST", "/auth/register", `{"username":"spam9","password":"password1"}`).StatusCode; s != 429 {
		t.Errorf("3rd signup: %d", s)
	}
	if s := c.doIP("6.6.6.6", "POST", "/auth/register", `{"username":"legit","password":"password1"}`).StatusCode; s != 201 {
		t.Errorf("other IP: %d", s)
	}
}

func TestGeneralAndWriteLimits(t *testing.T) {
	srv := setupWith(t, Limits{IPRate: 0.01, IPBurst: 6, WriteRate: 0.01, WriteBurst: 2, PasswordChecks: 3})
	c := newClient(t, srv)
	c.doIP("7.7.7.7", "POST", "/auth/register", `{"username":"writer","password":"password1"}`) // 1 IP token
	for i := 0; i < 2; i++ {
		if s := c.doIP("7.7.7.7", "POST", "/recipes", `{"name":"r","method":"V60","ratio":16}`).StatusCode; s != 201 {
			t.Fatalf("write %d: %d", i, s)
		}
	}
	if s := c.doIP("7.7.7.7", "POST", "/recipes", `{"name":"r","method":"V60","ratio":16}`).StatusCode; s != 429 {
		t.Errorf("3rd write: %d", s)
	}
	// 4 IP tokens used; 2 left, then blocked.
	c.doIP("7.7.7.7", "GET", "/coffees", "")
	c.doIP("7.7.7.7", "GET", "/coffees", "")
	if s := c.doIP("7.7.7.7", "GET", "/coffees", "").StatusCode; s != 429 {
		t.Errorf("IP limit not applied: %d", s)
	}
}

func TestStreamCapPerUser(t *testing.T) {
	srv := setupWith(t, Limits{StreamsPerUser: 2, PasswordChecks: 3})
	c := newClient(t, srv)
	c.do("POST", "/auth/register", `{"username":"streamer","password":"password1"}`, nil)
	c.stream(t, "a")
	c.stream(t, "b")
	time.Sleep(100 * time.Millisecond)
	if s := c.do("GET", "/events", "", nil); s != 429 {
		t.Errorf("3rd stream: %d", s)
	}
}

func TestContentCaps(t *testing.T) {
	srv := setupWith(t, Limits{MaxRecipes: 2, MaxBrewsPerCoffee: 1, PasswordChecks: 3})
	c := newClient(t, srv)
	c.do("POST", "/auth/register", `{"username":"capped","password":"password1"}`, nil)
	for i := 0; i < 2; i++ {
		c.do("POST", "/recipes", `{"name":"r","method":"V60","ratio":16}`, nil)
	}
	if s := c.do("POST", "/recipes", `{"name":"r","method":"V60","ratio":16}`, nil); s != 409 {
		t.Errorf("3rd recipe: %d", s)
	}
	var b Brew
	c.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16}`, &b)
	c.do("POST", "/brews", `{"coffeeId":2,"method":"V60","ratio":16}`, nil)
	if s := c.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16}`, nil); s != 409 {
		t.Errorf("2nd brew on coffee 1: %d", s)
	}
	if s := c.do("PUT", fmt.Sprintf("/brews/%d", b.ID), `{"coffeeId":2,"method":"V60","ratio":16}`, nil); s != 409 {
		t.Errorf("moving a brew onto a full coffee: %d", s)
	}
}

func TestClientIP(t *testing.T) {
	req := func(remote, hdr string) *http.Request {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if hdr != "" {
			r.Header.Set("CF-Connecting-IP", hdr)
		}
		return r
	}
	untrusted := &Server{}
	if got := untrusted.clientIP(req("203.0.113.5:1234", "1.2.3.4")); got != "203.0.113.5" {
		t.Errorf("spoofed header honored without opt-in: %s", got)
	}
	trusted := &Server{ipHeader: "Cf-Connecting-Ip"}
	if got := trusted.clientIP(req("127.0.0.1:1", "198.51.100.7")); got != "198.51.100.7" {
		t.Errorf("trusted header ignored: %s", got)
	}
	a := trusted.clientIP(req("127.0.0.1:1", "2001:db8:1:2::1"))
	b := trusted.clientIP(req("127.0.0.1:1", "2001:db8:1:2:ffff::9"))
	if a != b || a != "2001:db8:1:2::/64" {
		t.Errorf("IPv6 not grouped by /64: %s vs %s", a, b)
	}
}
