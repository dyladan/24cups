package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Cloudflare Turnstile on sign-up. The browser renders the widget and sends
// its single-use token with the registration request; the server verifies it
// with Siteverify before creating the account. The browser never talks to
// Siteverify.

const siteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// TurnstileConfig enables bot protection on sign-up when Secret is set.
type TurnstileConfig struct {
	SiteKey   string   // public; sent to the browser
	Secret    string   // from TURNSTILE_SECRET; never logged or sent to clients
	Hostnames []string // allowed hostnames reported by Siteverify
	VerifyURL string   // override for tests
}

func (t *TurnstileConfig) enabled() bool { return t != nil && t.Secret != "" }

// Validate rejects configurations that would weaken verification.
func (t *TurnstileConfig) Validate() error {
	if !t.enabled() {
		return nil
	}
	if t.SiteKey == "" {
		return fmt.Errorf("TURNSTILE_SECRET is set but TURNSTILE_SITE_KEY is empty")
	}
	if len(t.Hostnames) == 0 {
		return fmt.Errorf("TURNSTILE_SECRET is set but TURNSTILE_HOSTNAMES is empty; list the site's hostname(s)")
	}
	return nil
}

// ParseHostnames splits a comma-separated list.
func ParseHostnames(s string) []string {
	var out []string
	for _, h := range strings.Split(s, ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			out = append(out, h)
		}
	}
	return out
}

var siteverifyClient = &http.Client{Timeout: 10 * time.Second}

// verifyTurnstile checks a token for the given action. Any failure,
// including network errors and non-200 responses, is a rejection.
func (s *Server) verifyTurnstile(r *http.Request, token, action string) error {
	t := s.turnstile
	reject := errf(http.StatusForbidden, "The bot check failed. Please try again.")
	if len(token) == 0 || len(token) > 2048 || len(t.Hostnames) == 0 {
		return reject
	}
	form := url.Values{
		"secret":   {t.Secret},
		"response": {token},
		"remoteip": {s.rawClientIP(r)},
	}
	endpoint := t.VerifyURL
	if endpoint == "" {
		endpoint = siteverifyURL
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return reject
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := siteverifyClient.Do(req)
	if err != nil {
		return reject
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return reject
	}
	var out struct {
		Success  bool   `json:"success"`
		Action   string `json:"action"`
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 64<<10)).Decode(&out); err != nil {
		return reject
	}
	if !out.Success || out.Action != action || !slices.Contains(t.Hostnames, strings.ToLower(out.Hostname)) {
		return reject
	}
	return nil
}

// publicConfig tells the browser what it needs to render (the site key is
// public by design).
func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) error {
	cfg := map[string]string{}
	if s.turnstile.enabled() {
		cfg["turnstileSiteKey"] = s.turnstile.SiteKey
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, 200, cfg)
	return nil
}
