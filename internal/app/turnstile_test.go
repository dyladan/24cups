package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeSiteverify mimics Cloudflare: each token maps to a canned result, and
// a token can be redeemed once (replays fail like timeout-or-duplicate).
type fakeSiteverify struct {
	mu       sync.Mutex
	used     map[string]bool
	status   int
	last     map[string]string
	contentT string
}

func (f *fakeSiteverify) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.ParseForm()
	f.contentT = r.Header.Get("Content-Type")
	f.last = map[string]string{"secret": r.PostForm.Get("secret"), "response": r.PostForm.Get("response"), "remoteip": r.PostForm.Get("remoteip")}
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	tok := r.PostForm.Get("response")
	res := map[string]any{"success": false, "error-codes": []string{"invalid-input-response"}}
	if r.PostForm.Get("secret") != "real-secret" {
		res["error-codes"] = []string{"invalid-input-secret"}
	} else if !f.used[tok] && strings.Contains(tok, "@") {
		f.used[tok] = true
		// token format in tests: "<action>@<hostname>"
		if a, h, ok := strings.Cut(tok, "@"); ok {
			res = map[string]any{"success": true, "action": a, "hostname": h}
		}
	} else if f.used[tok] {
		res["error-codes"] = []string{"timeout-or-duplicate"}
	}
	json.NewEncoder(w).Encode(res)
}

func setupTurnstile(t *testing.T, cfg TurnstileConfig) (*httptest.Server, *fakeSiteverify) {
	fake := &fakeSiteverify{used: map[string]bool{}}
	sv := httptest.NewServer(fake)
	t.Cleanup(sv.Close)
	if cfg.VerifyURL == "" {
		cfg.VerifyURL = sv.URL
	}
	path := filepath.Join(t.TempDir(), "t.db")
	db, _ := OpenDB(path)
	t.Cleanup(func() { db.Close() })
	rdb, _ := OpenReadDB(path)
	t.Cleanup(func() { rdb.Close() })
	h, err := NewServer(db, rdb, Options{Limits: testLimits, ClientIPHeader: "X-Test-IP", Turnstile: cfg})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, fake
}

func signup(c *client, name, token string) int {
	return c.doIP("203.0.113.9", "POST", "/auth/register",
		fmt.Sprintf(`{"username":%q,"password":"password1","turnstileToken":%q}`, name, token)).StatusCode
}

func TestTurnstileSignup(t *testing.T) {
	srv, fake := setupTurnstile(t, TurnstileConfig{SiteKey: "site-key", Secret: "real-secret", Hostnames: []string{"advent.example.com"}})
	c := newClient(t, srv)

	var cfg map[string]string
	c.do("GET", "/config", "", &cfg)
	if cfg["turnstileSiteKey"] != "site-key" || len(cfg) != 1 {
		t.Errorf("public config: %v", cfg)
	}

	if s := signup(c, "ok-user", "signup@advent.example.com"); s != 201 {
		t.Fatalf("valid token: %d", s)
	}
	if fake.last["secret"] != "real-secret" || fake.last["remoteip"] != "203.0.113.9" ||
		fake.contentT != "application/x-www-form-urlencoded" {
		t.Errorf("siteverify request: %v content-type=%q", fake.last, fake.contentT)
	}
	for _, tc := range []struct{ name, token string }{
		{"missing token", ""},
		{"replayed token", "signup@advent.example.com"},
		{"wrong action", "login@advent.example.com"},
		{"wrong hostname", "signup@evil.example.net"},
		{"rejected by Cloudflare", "garbage"},
		{"oversized token", strings.Repeat("x", 2049)},
	} {
		if s := signup(newClient(t, srv), "user-"+strings.ReplaceAll(tc.name, " ", ""), tc.token); s != 403 {
			t.Errorf("%s: %d, want 403", tc.name, s)
		}
	}

	fake.status = 500
	if s := signup(c, "cf-down", "signup@advent.example.com"); s != 403 {
		t.Errorf("siteverify 500: %d, want 403", s)
	}
}

func TestTurnstileNetworkFailureRejects(t *testing.T) {
	srv, _ := setupTurnstile(t, TurnstileConfig{SiteKey: "k", Secret: "real-secret", Hostnames: []string{"a.example"}, VerifyURL: "http://127.0.0.1:1/unreachable"})
	if s := signup(newClient(t, srv), "netfail", "signup@a.example"); s != 403 {
		t.Errorf("unreachable siteverify: %d, want 403", s)
	}
}

func TestTurnstileConfigValidation(t *testing.T) {
	if err := (&TurnstileConfig{SiteKey: "k", Secret: "s"}).Validate(); err == nil {
		t.Error("empty hostname allowlist accepted")
	}
	if err := (&TurnstileConfig{Secret: "s", Hostnames: []string{"a"}}).Validate(); err == nil {
		t.Error("missing site key accepted")
	}
	if err := (&TurnstileConfig{}).Validate(); err != nil {
		t.Errorf("disabled config rejected: %v", err)
	}
	if got := ParseHostnames(" Advent.Example.com, ,b.example "); len(got) != 2 || got[0] != "advent.example.com" {
		t.Errorf("ParseHostnames: %v", got)
	}
	// Disabled: no site key exposed, sign-up works without a token.
	srv := setup(t)
	c := newClient(t, srv)
	var cfg map[string]string
	c.do("GET", "/config", "", &cfg)
	if len(cfg) != 0 {
		t.Errorf("config leaks without secret: %v", cfg)
	}
	if s := signup(c, "plain-user", ""); s != 201 {
		t.Errorf("signup with Turnstile off: %d", s)
	}
}

func TestTurnstileStartupCheck(t *testing.T) {
	srv, _ := setupTurnstile(t, TurnstileConfig{SiteKey: "k", Secret: "real-secret", Hostnames: []string{"a.example"}})
	if err := srv.Config.Handler.(*Server).CheckTurnstile(context.Background()); err != nil {
		t.Errorf("healthy config: %v", err)
	}
	srv, _ = setupTurnstile(t, TurnstileConfig{SiteKey: "k", Secret: "wrong-secret", Hostnames: []string{"a.example"}})
	if err := srv.Config.Handler.(*Server).CheckTurnstile(context.Background()); err == nil || !strings.Contains(err.Error(), "rejected TURNSTILE_SECRET_KEY") {
		t.Errorf("wrong secret: %v", err)
	}
	// The original production bug: no route to Cloudflare from the container.
	srv, _ = setupTurnstile(t, TurnstileConfig{SiteKey: "k", Secret: "real-secret", Hostnames: []string{"a.example"}, VerifyURL: "http://127.0.0.1:1/x"})
	if err := srv.Config.Handler.(*Server).CheckTurnstile(context.Background()); err == nil || !strings.Contains(err.Error(), "can't reach Cloudflare Siteverify") {
		t.Errorf("unreachable: %v", err)
	}
}

func TestTurnstileRejectionIsLoggedWithReason(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	srv, _ := setupTurnstile(t, TurnstileConfig{SiteKey: "k", Secret: "real-secret", Hostnames: []string{"advent.example.com"}})
	signup(newClient(t, srv), "wronghost", "signup@evil.example.net")
	out := buf.String()
	for _, want := range []string{"turnstile rejected sign-up", "reason=hostname_mismatch", "hostname=evil.example.net"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "real-secret") || strings.Contains(out, "signup@evil") {
		t.Errorf("log leaked the secret or token:\n%s", out)
	}
}
