package app

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "advent_session"
	sessionTTL    = 60 * 24 * time.Hour
)

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	// SpoilerMode controls when tasting notes and community scores appear:
	// "rated" (after you rate a coffee or choose to reveal it), "opened",
	// or "always" (scores even on unopened doors).
	SpoilerMode string `json:"spoilerMode"`

	// Set only for signed-out visitors (see anonViewer).
	anon         bool
	anonDay      int
	anonSpoilers bool
	TempUnit     string `json:"tempUnit"`
	CreatedAt    string `json:"createdAt"`
}

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

// dummyHash is compared against when a username doesn't exist so login
// timing doesn't reveal which usernames are registered.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func (s *Server) currentUser(r *http.Request) (*User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, nil
	}
	u := &User{}
	err = s.rdb.QueryRowContext(r.Context(), `
		SELECT u.id, u.username, u.display_name, u.spoiler_mode, u.temp_unit, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`,
		hashToken(c.Value), time.Now().Unix(),
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.SpoilerMode, &u.TempUnit, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token := randomToken(32)
	exp := time.Now().Add(sessionTTL)
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix()); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(),
		`INSERT INTO sessions(token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), userID, exp.Unix()); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

type credentials struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	DisplayName    string `json:"displayName"`
	TurnstileToken string `json:"turnstileToken"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) error {
	if ok, wait := s.guards.signup.allow(s.clientIP(r)); !ok {
		return tooMany(w, r, "signup", "Too many new accounts from your network.", wait)
	}
	var in credentials
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Username = strings.TrimSpace(in.Username)
	if !usernameRe.MatchString(in.Username) {
		return errf(400, "username must be 3–32 characters: letters, numbers, . _ -")
	}
	if len(in.Password) < 8 || len(in.Password) > 72 {
		return errf(400, "password must be 8–72 characters")
	}
	display, err := trimMax("display name", in.DisplayName, 50)
	if err != nil {
		return err
	}
	if display == "" {
		display = in.Username
	}
	if s.turnstile.enabled() {
		if err := s.verifyTurnstile(r, in.TurnstileToken, "signup"); err != nil {
			return err
		}
	}
	var hash []byte
	if err := s.guards.withPasswordCheck(r.Context(), func() {
		hash, err = bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	}); err != nil {
		return err
	}
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(r.Context(),
		`INSERT INTO users(username, display_name, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		in.Username, display, string(hash), now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errf(409, "that username is taken")
		}
		return err
	}
	id, _ := res.LastInsertId()
	count(r.Context(), mSignups)
	if err := s.startSession(w, r, id); err != nil {
		return err
	}
	return s.writeUser(w, r, id, http.StatusCreated)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	if ok, wait := s.guards.login.allow(s.clientIP(r)); !ok {
		return tooMany(w, r, "login", "Too many sign-in attempts from your network.", wait)
	}
	var in credentials
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSpace(in.Username))
	if blocked, wait := s.guards.loginFails.blocked(name); blocked {
		return tooMany(w, r, "login_failures", "Too many failed sign-ins for this account.", wait)
	}
	var id int64
	var hash string
	err := s.rdb.QueryRowContext(r.Context(),
		`SELECT id, password_hash FROM users WHERE username = ?`, name).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		// Same work and delay as a wrong password, so timing doesn't reveal
		// which usernames exist.
		hash = string(dummyHash)
		id = 0
	} else if err != nil {
		return err
	}
	var match bool
	if err := s.guards.withPasswordCheck(r.Context(), func() {
		match = bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) == nil
	}); err != nil {
		return err
	}
	if !match || id == 0 {
		count(r.Context(), mLogins, outcome("failure"))
		s.guards.loginFails.spend(name)
		s.guards.failDelay(r.Context())
		return errf(401, "wrong username or password")
	}
	count(r.Context(), mLogins, outcome("success"))
	if err := s.startSession(w, r, id); err != nil {
		return err
	}
	return s.writeUser(w, r, id, http.StatusOK)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash = ?`, hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode})
	return noContent(w)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, u *User) error {
	writeJSON(w, 200, u)
	return nil
}

// statsOnSealedDoors reports whether u sees scores for unopened coffees.
func (u *User) statsOnSealedDoors() bool { return u.SpoilerMode == "always" }

// revealedSQL is a SQL condition, over b.coffee_id and the viewer's
// coffee_opens row aliased o, that is true when u may see other people's
// ratings and notes for that coffee.
func (u *User) revealedSQL() string {
	if u.SpoilerMode != "rated" {
		return "(o.user_id IS NOT NULL)"
	}
	return fmt.Sprintf(`(o.user_id IS NOT NULL AND (o.revealed = 1 OR EXISTS (
		SELECT 1 FROM brews rb WHERE rb.user_id = %d AND rb.coffee_id = b.coffee_id AND rb.rating IS NOT NULL)))`, u.ID)
}

// isRevealed applies the same rule in Go.
func (u *User) isRevealed(opened, rated, manual bool) bool {
	return opened && (u.SpoilerMode != "rated" || rated || manual)
}

func (s *Server) writeUser(w http.ResponseWriter, r *http.Request, id int64, status int) error {
	u := &User{}
	err := s.rdb.QueryRowContext(r.Context(),
		`SELECT id, username, display_name, spoiler_mode, temp_unit, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.SpoilerMode, &u.TempUnit, &u.CreatedAt)
	if err != nil {
		return err
	}
	writeJSON(w, status, u)
	return nil
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		DisplayName *string `json:"displayName"`
		SpoilerMode *string `json:"spoilerMode"`
		TempUnit    *string `json:"tempUnit"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.DisplayName != nil {
		d, err := trimMax("display name", *in.DisplayName, 50)
		if err != nil {
			return err
		}
		if d == "" {
			return errf(400, "display name can't be empty")
		}
		u.DisplayName = d
	}
	if in.SpoilerMode != nil {
		switch *in.SpoilerMode {
		case "rated", "opened", "always":
			u.SpoilerMode = *in.SpoilerMode
		default:
			return errf(400, "spoilerMode must be rated, opened or always")
		}
	}
	if in.TempUnit != nil {
		if *in.TempUnit != "C" && *in.TempUnit != "F" {
			return errf(400, "tempUnit must be C or F")
		}
		u.TempUnit = *in.TempUnit
	}
	if _, err := s.db.ExecContext(r.Context(),
		`UPDATE users SET display_name = ?, spoiler_mode = ?, temp_unit = ? WHERE id = ?`,
		u.DisplayName, u.SpoilerMode, u.TempUnit, u.ID); err != nil {
		return err
	}
	s.notifyUser(r, u, keyMe, keyCoffees, keyGroups)
	if in.DisplayName != nil {
		s.notify(r, keyCoffees, keyGroups)
	}
	writeJSON(w, 200, u)
	return nil
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Current string `json:"currentPassword"`
		New     string `json:"newPassword"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if len(in.New) < 8 || len(in.New) > 72 {
		return errf(400, "password must be 8–72 characters")
	}
	key := strconv.FormatInt(u.ID, 10)
	if blocked, wait := s.guards.pwFails.blocked(key); blocked {
		return tooMany(w, r, "password_failures", "Too many wrong passwords.", wait)
	}
	var hash string
	if err := s.rdb.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id = ?`, u.ID).Scan(&hash); err != nil {
		return err
	}
	var match bool
	var nh []byte
	var err error
	if err := s.guards.withPasswordCheck(r.Context(), func() {
		if match = bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Current)) == nil; match {
			nh, err = bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
		}
	}); err != nil {
		return err
	}
	if !match {
		s.guards.pwFails.spend(key)
		s.guards.failDelay(r.Context())
		return errf(400, "current password is wrong")
	}
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET password_hash = ? WHERE id = ?`, string(nh), u.ID); err != nil {
		return err
	}
	// Sign out every other session.
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id = ?`, u.ID); err != nil {
		return err
	}
	if err := s.startSession(w, r, u.ID); err != nil {
		return err
	}
	return noContent(w)
}
