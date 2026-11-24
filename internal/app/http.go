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
	"strings"
)

type Server struct {
	db            *sql.DB // single writer
	rdb           *sql.DB // read-only pool
	stats         *statsStore
	web           fs.FS
	secureCookies bool
	mux           *http.ServeMux
	events        *broker
}

func NewServer(db, rdb *sql.DB, web fs.FS, secureCookies bool) (*Server, error) {
	stats, err := loadStats(context.Background(), rdb)
	if err != nil {
		return nil, fmt.Errorf("load stats: %w", err)
	}
	s := &Server{db: db, rdb: rdb, stats: stats, web: web, secureCookies: secureCookies, mux: http.NewServeMux(), events: newBroker()}
	s.routes()
	return s, nil
}

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
	if strings.HasPrefix(r.URL.Path, "/api/") && !isSafeMethod(r.Method) {
		// Cross-site requests cannot set custom headers without a CORS
		// preflight (which we never approve), so this blocks CSRF.
		if r.Header.Get("X-Requested-With") == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing X-Requested-With header"})
			return
		}
	}
	s.mux.ServeHTTP(w, r)
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
	m.HandleFunc("GET /api/events", s.authed(s.streamEvents))

	// coffees
	m.HandleFunc("GET /api/coffees", s.authed(s.listCoffees))
	m.HandleFunc("GET /api/coffees/{id}", s.authed(s.getCoffee))
	m.HandleFunc("POST /api/coffees/{id}/open", s.authed(s.openCoffee))
	m.HandleFunc("DELETE /api/coffees/{id}/open", s.authed(s.unopenCoffee))
	m.HandleFunc("PUT /api/coffees/{id}/favorite", s.authed(s.favoriteCoffee))
	m.HandleFunc("DELETE /api/coffees/{id}/favorite", s.authed(s.unfavoriteCoffee))
	m.HandleFunc("GET /api/coffees/{id}/brews", s.authed(s.coffeeBrews))

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
			slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
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
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
		return fn(w, r, u)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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
