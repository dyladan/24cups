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

	"log/slog"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Cloudflare Turnstile on sign-up. The browser renders the widget and sends
// its single-use token with the registration request; the server verifies it
// with Siteverify before creating the account. The browser never talks to
// Siteverify.

const siteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// TurnstileConfig enables bot protection on sign-up when Secret is set.
type TurnstileConfig struct {
	SiteKey   string   // public; sent to the browser
	Secret    string   // from TURNSTILE_SECRET_KEY; never logged or sent to clients
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
		return fmt.Errorf("TURNSTILE_SECRET_KEY is set but TURNSTILE_SITE_KEY is empty")
	}
	if len(t.Hostnames) == 0 {
		return fmt.Errorf("TURNSTILE_SECRET_KEY is set but TURNSTILE_HOSTNAMES is empty; list the site's hostname(s)")
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

// Outbound Siteverify calls get client spans (child of the sign-up request).
var siteverifyClient = &http.Client{Timeout: 10 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)}

type siteverifyResult struct {
	Success    bool     `json:"success"`
	Action     string   `json:"action"`
	Hostname   string   `json:"hostname"`
	ErrorCodes []string `json:"error-codes"`
}

// siteverify asks Cloudflare about a token. A non-nil error means no usable
// answer (network failure, non-200, unparseable body); reason names which.
func (s *Server) siteverify(ctx context.Context, token, remoteIP string) (res siteverifyResult, reason string, err error) {
	t := s.turnstile
	form := url.Values{"secret": {t.Secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	endpoint := t.VerifyURL
	if endpoint == "" {
		endpoint = siteverifyURL
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return res, "request", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := siteverifyClient.Do(req)
	if err != nil {
		return res, "unreachable", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Sprintf("siteverify_http_%d", resp.StatusCode), fmt.Errorf("siteverify returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 64<<10)).Decode(&res); err != nil {
		return res, "bad_response", err
	}
	return res, "", nil
}

// verifyTurnstile checks a token for the given action. Any failure,
// including network errors and non-200 responses, is a rejection. The
// reason is logged, recorded on the span and counted (never the token or
// secret) so failed sign-ups can be diagnosed.
func (s *Server) verifyTurnstile(r *http.Request, token, action string) error {
	t := s.turnstile
	res, reason, err := s.checkToken(r, token, action)
	span := trace.SpanFromContext(r.Context())
	if reason == "" {
		count(r.Context(), mTurnstile, outcome("success"))
		span.SetAttributes(attribute.String("onyx_advent.turnstile.outcome", "success"))
		return nil
	}
	count(r.Context(), mTurnstile, outcome("rejected"), attribute.String("onyx_advent.turnstile.reason", reason))
	span.SetAttributes(
		attribute.String("onyx_advent.turnstile.outcome", "rejected"),
		attribute.String("onyx_advent.turnstile.reason", reason),
		attribute.StringSlice("onyx_advent.turnstile.error_codes", res.ErrorCodes),
	)
	attrs := []any{"reason", reason, "error_codes", res.ErrorCodes,
		"hostname", res.Hostname, "allowed_hostnames", t.Hostnames, "action", res.Action, "want_action", action}
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	slog.WarnContext(r.Context(), "turnstile rejected sign-up", attrs...)
	return errf(http.StatusForbidden, "The bot check failed. Please try again.")
}

func (s *Server) checkToken(r *http.Request, token, action string) (siteverifyResult, string, error) {
	if len(token) == 0 || len(token) > 2048 {
		return siteverifyResult{}, "invalid_token", nil
	}
	res, reason, err := s.siteverify(r.Context(), token, s.rawClientIP(r))
	switch {
	case err != nil:
		return res, reason, err
	case !res.Success:
		return res, "not_success", nil
	case res.Action != action:
		return res, "action_mismatch", nil
	case !slices.Contains(s.turnstile.Hostnames, strings.ToLower(res.Hostname)):
		return res, "hostname_mismatch", nil
	}
	return res, "", nil
}

// CheckTurnstile verifies at startup that Siteverify is reachable and the
// secret is accepted, using Cloudflare's documented probe: a dummy token
// must be rejected as invalid-input-response, not invalid-input-secret.
func (s *Server) CheckTurnstile(ctx context.Context) error {
	if !s.turnstile.enabled() {
		return nil
	}
	res, reason, err := s.siteverify(ctx, "XXXX.DUMMY.TOKEN.XXXX", "")
	if err != nil {
		return fmt.Errorf("can't reach Cloudflare Siteverify (%s): %w; sign-ups will fail until the app can reach challenges.cloudflare.com", reason, err)
	}
	if slices.Contains(res.ErrorCodes, "invalid-input-secret") || slices.Contains(res.ErrorCodes, "missing-input-secret") {
		return fmt.Errorf("Cloudflare rejected TURNSTILE_SECRET_KEY (%v); check it belongs to the widget for site key %s", res.ErrorCodes, s.turnstile.SiteKey)
	}
	if res.Success || !slices.Contains(res.ErrorCodes, "invalid-input-response") {
		return fmt.Errorf("unexpected Siteverify answer to the probe token: success=%v codes=%v", res.Success, res.ErrorCodes)
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
	if d := s.devToday(); d != "" {
		cfg["devToday"] = d
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, 200, cfg)
	return nil
}
