package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"strings"
	"time"
)

type Server struct {
	db            *sql.DB // single writer
	rdb           *sql.DB // read-only pool
	stats         *statsStore
	web           fs.FS
	secureCookies bool
	mux           *http.ServeMux
	events        *broker
	guards        *guards
	ipHeader      string
	calendarYear  int
	now           func() time.Time
	turnstile     *TurnstileConfig
}

// Options configures a Server.
type Options struct {
	Web           fs.FS
	SecureCookies bool
	// ClientIPHeader names a header carrying the real client IP, set by a
	// trusted proxy (e.g. "CF-Connecting-IP" behind a Cloudflare Tunnel).
	// Leave empty unless the app is only reachable through that proxy.
	ClientIPHeader string
	Limits         Limits
	// CalendarYear is the December the calendar belongs to (for anonymous
	// visitors' door unlocking). Defaults to the current year.
	CalendarYear int
	// Now overrides the clock (tests).
	Now func() time.Time
	// Turnstile protects sign-up when its Secret is set.
	Turnstile TurnstileConfig
}

func NewServer(db, rdb *sql.DB, opts Options) (*Server, error) {
	if err := opts.Turnstile.Validate(); err != nil {
		return nil, err
	}
	stats, err := loadStats(context.Background(), rdb)
	if err != nil {
		return nil, fmt.Errorf("load stats: %w", err)
	}
	s := &Server{
		db: db, rdb: rdb, stats: stats, web: opts.Web, secureCookies: opts.SecureCookies,
		mux: http.NewServeMux(), events: newBroker(opts.Limits),
		guards: newGuards(opts.Limits), ipHeader: http.CanonicalHeaderKey(opts.ClientIPHeader),
		calendarYear: opts.CalendarYear, now: opts.Now, turnstile: &opts.Turnstile,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.calendarYear == 0 {
		s.calendarYear = s.now().Year()
	}
	s.routes()
	s.events.registerStreamGauge()
	return s, nil
}

// Run starts background housekeeping until ctx is done.
func (s *Server) Run(ctx context.Context) { s.guards.janitor(ctx) }

// refreshStats updates the in-memory aggregates after a write. Failures are
// logged rather than surfaced: the write itself succeeded.
func (s *Server) refreshStats(r *http.Request, userID int64, coffeeIDs ...int64) {
	for _, cid := range coffeeIDs {
		if err := s.stats.refresh(r.Context(), s.rdb, userID, cid); err != nil {
			slog.Error("refresh stats", "user", userID, "coffee", cid, "err", err)
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("X-Frame-Options", "DENY")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if ok, wait := s.guards.ip.allow(s.clientIP(r)); !ok {
			s.h(func(w http.ResponseWriter, r *http.Request) error {
				return tooMany(w, r, "ip", "Too many requests from your network.", wait)
			})(w, r)
			return
		}
	}
	if strings.HasPrefix(r.URL.Path, "/api/") && !isSafeMethod(r.Method) {
		// Cross-site requests cannot set custom headers without a CORS
		// preflight (which we never approve), so this blocks CSRF.
		if r.Header.Get("X-Requested-With") == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing X-Requested-With header"})
			return
		}
	}
	s.mux.ServeHTTP(w, r)
	// The mux records the matched pattern on r; add the route to the server
	// span (otelhttp uses it for the span name and metrics).
	if r.Pattern != "" {
		trace.SpanFromContext(r.Context()).SetAttributes(semconv.HTTPRoute(routeOf(r.Pattern)))
	}
}

// routeOf strips the method from a ServeMux pattern ("GET /api/x" -> "/api/x").
func routeOf(pattern string) string {
	if _, path, ok := strings.Cut(pattern, " "); ok {
		return path
	}
	return pattern
}

// Handler wraps the server with OpenTelemetry HTTP instrumentation. API
// requests get server spans named by route (e.g. "GET /api/coffees/{id}") and
// http.server.request.duration metrics. Static files, health checks and the
// long-lived live-update stream are excluded.
func (s *Server) Handler() http.Handler {
	return otelhttp.NewHandler(s, "advent-cup",
		otelhttp.WithFilter(func(r *http.Request) bool {
			p := r.URL.Path
			return strings.HasPrefix(p, "/api/") && p != "/api/health" && p != "/api/events"
		}),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			if r.Pattern == "" {
				return r.Method
			}
			return r.Method + " " + routeOf(r.Pattern)
		}),
	)
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	// auth
	m.HandleFunc("POST /api/auth/register", s.h(s.register))
	m.HandleFunc("POST /api/auth/login", s.h(s.login))
	m.HandleFunc("POST /api/auth/logout", s.h(s.logout))
	m.HandleFunc("GET /api/auth/me", s.authed(s.me))
	m.HandleFunc("PATCH /api/me", s.authed(s.updateMe))
	m.HandleFunc("POST /api/me/password", s.authed(s.changePassword))

	m.HandleFunc("GET /api/catalog", s.h(s.catalog))
	m.HandleFunc("GET /api/config", s.h(s.publicConfig))
	m.HandleFunc("GET /api/events", s.authed(s.streamEvents))

	// coffees
	m.HandleFunc("GET /api/coffees", s.maybeAuthed(s.listCoffees))
	m.HandleFunc("GET /api/coffees/{id}", s.maybeAuthed(s.getCoffee))
	m.HandleFunc("POST /api/coffees/{id}/open", s.authed(s.openCoffee))
	m.HandleFunc("DELETE /api/coffees/{id}/open", s.authed(s.unopenCoffee))
	m.HandleFunc("POST /api/coffees/{id}/reveal", s.authed(s.revealCoffee))
	m.HandleFunc("GET /api/coffees/{id}/peek", s.maybeAuthed(s.peekCoffee))
	m.HandleFunc("PUT /api/coffees/{id}/favorite", s.authed(s.favoriteCoffee))
	m.HandleFunc("DELETE /api/coffees/{id}/favorite", s.authed(s.unfavoriteCoffee))
	m.HandleFunc("GET /api/coffees/{id}/brews", s.maybeAuthed(s.coffeeBrews))

	// recipes
	m.HandleFunc("GET /api/recipes", s.authed(s.listRecipes))
	m.HandleFunc("POST /api/recipes", s.authed(s.createRecipe))
	m.HandleFunc("PUT /api/recipes/{id}", s.authed(s.updateRecipe))
	m.HandleFunc("DELETE /api/recipes/{id}", s.authed(s.deleteRecipe))

	// grinders
	m.HandleFunc("GET /api/grinders", s.authed(s.listGrinders))
	m.HandleFunc("POST /api/grinders", s.authed(s.createGrinder))
	m.HandleFunc("PUT /api/grinders/{id}", s.authed(s.updateGrinder))
	m.HandleFunc("DELETE /api/grinders/{id}", s.authed(s.deleteGrinder))

	// brews
	m.HandleFunc("GET /api/brews", s.authed(s.listMyBrews))
	m.HandleFunc("POST /api/brews", s.authed(s.createBrew))
	m.HandleFunc("GET /api/brews/{id}", s.authed(s.getBrew))
	m.HandleFunc("PUT /api/brews/{id}", s.authed(s.updateBrew))
	m.HandleFunc("DELETE /api/brews/{id}", s.authed(s.deleteBrew))

	// groups
	m.HandleFunc("GET /api/groups", s.authed(s.myGroups))
	m.HandleFunc("GET /api/groups/discover", s.authed(s.discoverGroups))
	m.HandleFunc("POST /api/groups", s.authed(s.createGroup))
	m.HandleFunc("POST /api/groups/join", s.authed(s.joinGroupByCode))
	m.HandleFunc("GET /api/groups/{id}", s.authed(s.getGroup))
	m.HandleFunc("PUT /api/groups/{id}", s.authed(s.updateGroup))
	m.HandleFunc("DELETE /api/groups/{id}", s.authed(s.deleteGroup))
	m.HandleFunc("POST /api/groups/{id}/join", s.authed(s.joinPublicGroup))
	m.HandleFunc("POST /api/groups/{id}/leave", s.authed(s.leaveGroup))
	m.HandleFunc("POST /api/groups/{id}/invite-code", s.authed(s.regenerateInvite))
	m.HandleFunc("DELETE /api/groups/{id}/members/{userId}", s.authed(s.removeMember))

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})
	m.HandleFunc("/", s.static)
}

// static serves the built SPA, falling back to index.html for client routes.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.web == nil {
		http.NotFound(w, r)
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p != "" {
		if f, err := s.web.Open(p); err == nil {
			st, _ := f.Stat()
			f.Close()
			if st != nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, s.web, p)
				return
			}
		}
		if strings.HasPrefix(p, "assets/") {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.web, "index.html")
}

// capCheck rejects a create when the user already has max rows (0 = no cap).
func (s *Server) capCheck(r *http.Request, countSQL string, max int, msg string, args ...any) error {
	if max <= 0 {
		return nil
	}
	var n int
	if err := s.rdb.QueryRowContext(r.Context(), countSQL, args...).Scan(&n); err != nil {
		return err
	}
	if n >= max {
		return errf(http.StatusConflict, msg, max)
	}
	return nil
}

// ---- handler plumbing ----

type apiError struct {
	Status int
	Msg    string
}

func (e *apiError) Error() string { return e.Msg }

func errf(status int, format string, a ...any) error {
	return &apiError{Status: status, Msg: fmt.Sprintf(format, a...)}
}

var (
	errNotFound     = errf(http.StatusNotFound, "not found")
	errUnauthorized = errf(http.StatusUnauthorized, "not signed in")
	errForbidden    = errf(http.StatusForbidden, "forbidden")
)

type handlerFunc func(w http.ResponseWriter, r *http.Request) error
type authedFunc func(w http.ResponseWriter, r *http.Request, u *User) error

func (s *Server) h(fn handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			var ae *apiError
			if errors.As(err, &ae) {
				writeJSON(w, ae.Status, map[string]string{"error": ae.Msg})
				return
			}
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
			span := trace.SpanFromContext(r.Context())
			span.RecordError(err)
			span.SetStatus(codes.Error, "internal error")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
	}
}

type ctxKey struct{}

func (s *Server) authed(fn authedFunc) http.HandlerFunc {
	return s.h(func(w http.ResponseWriter, r *http.Request) error {
		u, err := s.currentUser(r)
		if err != nil {
			return err
		}
		if u == nil {
			return errUnauthorized
		}
		return s.authedAs(w, r, u, fn)
	})
}

// authedAs applies per-user limits and calls fn for a signed-in user.
func (s *Server) authedAs(w http.ResponseWriter, r *http.Request, u *User, fn authedFunc) error {
	{
		key := strconv.FormatInt(u.ID, 10)
		if ok, wait := s.guards.user.allow(key); !ok {
			return tooMany(w, r, "user", "You're going a bit fast.", wait)
		}
		if !isSafeMethod(r.Method) {
			if ok, wait := s.guards.write.allow(key); !ok {
				return tooMany(w, r, "write", "You're making changes too quickly.", wait)
			}
		}
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
		return fn(w, r, u)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return errf(http.StatusBadRequest, "invalid JSON body: %v", err)
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errNotFound
	}
	return id, nil
}

func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// trimMax trims whitespace and rejects strings longer than max runes.
func trimMax(field, v string, max int) (string, error) {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > max {
		return "", errf(http.StatusBadRequest, "%s must be at most %d characters", field, max)
	}
	return v, nil
}
