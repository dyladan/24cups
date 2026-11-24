package app

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
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
	ID             int64  `json:"id"`
	Username       string `json:"username"`
	DisplayName    string `json:"displayName"`
	RevealUnopened bool   `json:"revealUnopened"`
	TempUnit       string `json:"tempUnit"`
	CreatedAt      string `json:"createdAt"`
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
		SELECT u.id, u.username, u.display_name, u.reveal_unopened, u.temp_unit, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`,
		hashToken(c.Value), time.Now().Unix(),
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.RevealUnopened, &u.TempUnit, &u.CreatedAt)
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
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) error {
	if ok, wait := s.guards.signup.allow(s.clientIP(r)); !ok {
		return tooMany(w, "Too many new accounts from your network.", wait)
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
	if err := s.startSession(w, r, id); err != nil {
		return err
	}
	return s.writeUser(w, r, id, http.StatusCreated)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	if ok, wait := s.guards.login.allow(s.clientIP(r)); !ok {
		return tooMany(w, "Too many sign-in attempts from your network.", wait)
	}
	var in credentials
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSpace(in.Username))
	if blocked, wait := s.guards.loginFails.blocked(name); blocked {
		return tooMany(w, "Too many failed sign-ins for this account.", wait)
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
		s.guards.loginFails.spend(name)
		s.guards.failDelay(r.Context())
		return errf(401, "wrong username or password")
	}
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

func (s *Server) writeUser(w http.ResponseWriter, r *http.Request, id int64, status int) error {
	u := &User{}
	err := s.rdb.QueryRowContext(r.Context(),
		`SELECT id, username, display_name, reveal_unopened, temp_unit, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.RevealUnopened, &u.TempUnit, &u.CreatedAt)
	if err != nil {
		return err
	}
	writeJSON(w, status, u)
	return nil
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		DisplayName    *string `json:"displayName"`
		RevealUnopened *bool   `json:"revealUnopened"`
		TempUnit       *string `json:"tempUnit"`
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
	if in.RevealUnopened != nil {
		u.RevealUnopened = *in.RevealUnopened
	}
	if in.TempUnit != nil {
		if *in.TempUnit != "C" && *in.TempUnit != "F" {
			return errf(400, "tempUnit must be C or F")
		}
		u.TempUnit = *in.TempUnit
	}
	if _, err := s.db.ExecContext(r.Context(),
		`UPDATE users SET display_name = ?, reveal_unopened = ?, temp_unit = ? WHERE id = ?`,
		u.DisplayName, boolInt(u.RevealUnopened), u.TempUnit, u.ID); err != nil {
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
		return tooMany(w, "Too many wrong passwords.", wait)
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
