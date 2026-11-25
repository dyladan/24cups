package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Passkeys (WebAuthn). The browser's authenticator (phone, laptop, password
// manager or security key) keeps a private key; the server stores only the
// public key and checks signatures over one-time challenges, so nothing the
// server holds can be replayed or used to sign in elsewhere.
//
// Accounts may have a password, passkeys, or both. Passkey-only accounts
// have an empty password_hash and get one-time recovery codes at sign-up.

// PasskeyConfig enables passkeys when RPID is set.
type PasskeyConfig struct {
	// RPID is the site's hostname (e.g. "advent.example.com"). Passkeys are
	// bound to it: changing it later orphans every registered passkey.
	RPID string
	// Origins the browser may report, e.g. "https://advent.example.com".
	// Defaults to https://RPID.
	Origins []string
}

func newWebAuthn(cfg PasskeyConfig) (*webauthn.WebAuthn, error) {
	if cfg.RPID == "" {
		return nil, nil
	}
	origins := cfg.Origins
	if len(origins) == 0 {
		origins = []string{"https://" + cfg.RPID}
	}
	return webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: "24 Cups",
		RPOrigins:     origins,
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL},
		},
	})
}

// ---------- ceremonies ----------

// A ceremony is one begin/finish round trip. Its challenge lives only in
// memory, so a restart just makes in-flight attempts fail and retry.
const (
	ceremonyTTL     = 5 * time.Minute
	maxCeremonies   = 20_000
	ceremonySignup  = "signup"
	ceremonyAdd     = "add"
	ceremonyLogin   = "login"
	recoveryCodeLen = 12
	recoveryCodeN   = 10
)

type ceremony struct {
	kind    string
	session webauthn.SessionData
	expires time.Time
	// add: the signed-in user. signup: the account to create.
	userID      int64
	webauthnID  []byte
	username    string
	displayName string
	passkeyName string
}

type ceremonies struct {
	mu sync.Mutex
	m  map[string]*ceremony
}

func newCeremonies() *ceremonies { return &ceremonies{m: map[string]*ceremony{}} }

func (c *ceremonies) put(cer *ceremony) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if len(c.m) >= maxCeremonies {
		for k, v := range c.m {
			if now.After(v.expires) {
				delete(c.m, k)
			}
		}
		if len(c.m) >= maxCeremonies {
			return "", errf(http.StatusServiceUnavailable, "the server is busy; try again in a moment")
		}
	}
	id := randomToken(24)
	cer.expires = now.Add(ceremonyTTL)
	c.m[id] = cer
	return id, nil
}

// take removes and returns a live ceremony of the given kind. Each
// challenge can be answered once.
func (c *ceremonies) take(id, kind string) (*ceremony, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cer := c.m[id]
	delete(c.m, id)
	if cer == nil || cer.kind != kind || time.Now().After(cer.expires) {
		return nil, errf(http.StatusBadRequest, "that passkey request expired; try again")
	}
	return cer, nil
}

func (c *ceremonies) prune() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, v := range c.m {
		if now.After(v.expires) {
			delete(c.m, k)
		}
	}
}

// ---------- WebAuthn user ----------

type waUser struct {
	id          []byte
	name        string
	displayName string
	creds       []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return u.id }
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (s *Server) passkeysOn() error {
	if s.wa == nil {
		return errf(http.StatusNotFound, "passkeys aren't enabled on this server")
	}
	return nil
}

// registrationOpts asks for a discoverable credential (so sign-in needs no
// username) and excludes ones the user already has.
func registrationOpts(existing []webauthn.Credential) []webauthn.RegistrationOption {
	return []webauthn.RegistrationOption{
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationPreferred,
		}),
		webauthn.WithExclusions(webauthn.Credentials(existing).CredentialDescriptors()),
	}
}

type ceremonyStart struct {
	Ceremony string `json:"ceremony"`
	Options  any    `json:"options"`
}

type ceremonyFinish struct {
	Ceremony   string          `json:"ceremony"`
	Credential json.RawMessage `json:"credential"`
}

func passkeyName(n string) (string, error) {
	n, err := trimMax("passkey name", n, 50)
	if err != nil {
		return "", err
	}
	if n == "" {
		n = "Passkey"
	}
	return n, nil
}

// ---------- sign-up ----------

func (s *Server) passkeySignupBegin(w http.ResponseWriter, r *http.Request) error {
	if err := s.passkeysOn(); err != nil {
		return err
	}
	if ok, wait := s.guards.signup.allow(s.clientIP(r)); !ok {
		return tooMany(w, r, "signup", "Too many new accounts from your network.", wait)
	}
	var in struct {
		Username       string `json:"username"`
		DisplayName    string `json:"displayName"`
		PasskeyName    string `json:"passkeyName"`
		TurnstileToken string `json:"turnstileToken"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Username = strings.TrimSpace(in.Username)
	if !usernameRe.MatchString(in.Username) {
		return errf(400, "username must be 3–32 characters: letters, numbers, . _ -")
	}
	display, err := trimMax("display name", in.DisplayName, 50)
	if err != nil {
		return err
	}
	if display == "" {
		display = in.Username
	}
	pkName, err := passkeyName(in.PasskeyName)
	if err != nil {
		return err
	}
	var taken bool
	if err := s.rdb.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)`, in.Username).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return errf(409, "that username is taken")
	}
	if s.turnstile.enabled() {
		if err := s.verifyTurnstile(r, in.TurnstileToken, "signup"); err != nil {
			return err
		}
	}
	u := &waUser{id: newWebAuthnID(), name: in.Username, displayName: display}
	opts, sess, err := s.wa.BeginRegistration(u, registrationOpts(nil)...)
	if err != nil {
		return err
	}
	id, err := s.ceremonies.put(&ceremony{
		kind: ceremonySignup, session: *sess, webauthnID: u.id,
		username: in.Username, displayName: display, passkeyName: pkName,
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, ceremonyStart{id, opts})
	return nil
}

func (s *Server) passkeySignupFinish(w http.ResponseWriter, r *http.Request) error {
	if err := s.passkeysOn(); err != nil {
		return err
	}
	var in ceremonyFinish
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	cer, err := s.ceremonies.take(in.Ceremony, ceremonySignup)
	if err != nil {
		return err
	}
	u := &waUser{id: cer.webauthnID, name: cer.username, displayName: cer.displayName}
	cred, err := s.verifyRegistration(r, u, cer, in.Credential)
	if err != nil {
		return err
	}
	credJSON, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	codes := newRecoveryCodes()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(),
		`INSERT INTO users(username, display_name, password_hash, webauthn_id, created_at) VALUES (?, ?, '', ?, ?)`,
		cer.username, cer.displayName, cer.webauthnID, now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errf(409, "that username is taken")
		}
		return err
	}
	userID, _ := res.LastInsertId()
	if _, err := tx.ExecContext(r.Context(),
		`INSERT INTO passkeys(user_id, credential_id, credential, name, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, cred.ID, string(credJSON), cer.passkeyName, now()); err != nil {
		return err
	}
	if err := storeRecoveryCodes(r.Context(), tx, userID, codes); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	count(r.Context(), mSignups, authMethod("passkey"))
	if err := s.startSession(w, r, userID); err != nil {
		return err
	}
	user, err := s.loadUser(r.Context(), userID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user, "recoveryCodes": codes})
	return nil
}

func (s *Server) verifyRegistration(r *http.Request, u *waUser, cer *ceremony, raw json.RawMessage) (*webauthn.Credential, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBytes(raw)
	if err != nil {
		slog.InfoContext(r.Context(), "passkey registration rejected", "err", err)
		return nil, errf(400, "the passkey response couldn't be read")
	}
	cred, err := s.wa.CreateCredential(u, cer.session, parsed)
	if err != nil {
		slog.InfoContext(r.Context(), "passkey registration rejected", "err", err)
		return nil, errf(400, "the passkey couldn't be verified")
	}
	return cred, nil
}

// ---------- sign-in ----------

func (s *Server) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) error {
	if err := s.passkeysOn(); err != nil {
		return err
	}
	if ok, wait := s.guards.login.allow(s.clientIP(r)); !ok {
		return tooMany(w, r, "login", "Too many sign-in attempts from your network.", wait)
	}
	opts, sess, err := s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		return err
	}
	id, err := s.ceremonies.put(&ceremony{kind: ceremonyLogin, session: *sess})
	if err != nil {
		return err
	}
	writeJSON(w, 200, ceremonyStart{id, opts})
	return nil
}

func (s *Server) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) error {
	if err := s.passkeysOn(); err != nil {
		return err
	}
	var in ceremonyFinish
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	cer, err := s.ceremonies.take(in.Ceremony, ceremonyLogin)
	if err != nil {
		return err
	}
	fail := func(reason string, err error) error {
		slog.InfoContext(r.Context(), "passkey sign-in rejected", "reason", reason, "err", err)
		count(r.Context(), mLogins, outcome("failure"), authMethod("passkey"))
		s.guards.failDelay(r.Context())
		return errf(401, "that passkey isn't registered here, or it couldn't be verified")
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(in.Credential)
	if err != nil {
		return fail("parse", err)
	}
	var userID, passkeyID int64
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		var wid []byte
		var credJSON, username, display string
		err := s.rdb.QueryRowContext(r.Context(), `
			SELECT p.id, p.credential, u.id, u.webauthn_id, u.username, u.display_name
			FROM passkeys p JOIN users u ON u.id = p.user_id
			WHERE p.credential_id = ?`, rawID,
		).Scan(&passkeyID, &credJSON, &userID, &wid, &username, &display)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(wid, userHandle) {
			return nil, errors.New("user handle does not match the credential's account")
		}
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(credJSON), &c); err != nil {
			return nil, err
		}
		return &waUser{id: wid, name: username, displayName: display, creds: []webauthn.Credential{c}}, nil
	}
	_, cred, err := s.wa.ValidatePasskeyLogin(handler, cer.session, parsed)
	if err != nil {
		return fail("verify", err)
	}
	if cred.Authenticator.CloneWarning {
		return fail("clone_warning", fmt.Errorf("signature counter went backwards"))
	}
	credJSON, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(),
		`UPDATE passkeys SET credential = ?, last_used_at = ? WHERE id = ?`,
		string(credJSON), now(), passkeyID); err != nil {
		return err
	}
	count(r.Context(), mLogins, outcome("success"), authMethod("passkey"))
	if err := s.startSession(w, r, userID); err != nil {
		return err
	}
	return s.writeUser(w, r, userID, http.StatusOK)
}

// ---------- recovery codes ----------

// Recovery codes are 12 characters from a 31-symbol alphabet without
// look-alikes (~59 bits each). That's far beyond guessing range, so a fast
// hash is enough, like session tokens.
const recoveryAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newRecoveryCodes() []string {
	codes := make([]string, recoveryCodeN)
	for i := range codes {
		b := make([]byte, recoveryCodeLen)
		if _, err := rand.Read(b); err != nil {
			panic(err)
		}
		var sb strings.Builder
		for j, c := range b {
			if j > 0 && j%4 == 0 {
				sb.WriteByte('-')
			}
			// 256 % 31 leaves a slight bias; irrelevant at this length.
			sb.WriteByte(recoveryAlphabet[int(c)%len(recoveryAlphabet)])
		}
		codes[i] = sb.String()
	}
	return codes
}

// normalizeRecoveryCode drops separators and case so "abcd efgh-jkmn" works.
func normalizeRecoveryCode(c string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(c) {
		if strings.ContainsRune(recoveryAlphabet, r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func hashRecoveryCode(c string) string {
	sum := sha256.Sum256([]byte("recovery:" + normalizeRecoveryCode(c)))
	return hex.EncodeToString(sum[:])
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// storeRecoveryCodes replaces the user's codes.
func storeRecoveryCodes(ctx context.Context, db execer, userID int64, codes []string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, c := range codes {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO recovery_codes(user_id, code_hash) VALUES (?, ?)`, userID, hashRecoveryCode(c)); err != nil {
			return err
		}
	}
	return nil
}

// recover signs in with a username and a one-time recovery code, for
// someone who lost their passkeys. Every other session is signed out.
func (s *Server) recoverLogin(w http.ResponseWriter, r *http.Request) error {
	if ok, wait := s.guards.login.allow(s.clientIP(r)); !ok {
		return tooMany(w, r, "login", "Too many sign-in attempts from your network.", wait)
	}
	var in struct {
		Username string `json:"username"`
		Code     string `json:"code"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSpace(in.Username))
	if blocked, wait := s.guards.loginFails.blocked(name); blocked {
		return tooMany(w, r, "login_failures", "Too many failed sign-ins for this account.", wait)
	}
	var userID int64
	err := s.rdb.QueryRowContext(r.Context(), `SELECT id FROM users WHERE username = ?`, name).Scan(&userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var used int64
	if userID != 0 && len(normalizeRecoveryCode(in.Code)) == recoveryCodeLen {
		res, err := s.db.ExecContext(r.Context(),
			`DELETE FROM recovery_codes WHERE user_id = ? AND code_hash = ?`, userID, hashRecoveryCode(in.Code))
		if err != nil {
			return err
		}
		used, _ = res.RowsAffected()
	}
	if used != 1 {
		count(r.Context(), mLogins, outcome("failure"), authMethod("recovery_code"))
		s.guards.loginFails.spend(name)
		s.guards.failDelay(r.Context())
		return errf(401, "wrong username or recovery code")
	}
	count(r.Context(), mLogins, outcome("success"), authMethod("recovery_code"))
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if err := s.startSession(w, r, userID); err != nil {
		return err
	}
	return s.writeUser(w, r, userID, http.StatusOK)
}

// ---------- account security (signed in) ----------

type passkeyInfo struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
}

type securityInfo struct {
	PasskeysEnabled   bool          `json:"passkeysEnabled"`
	HasPassword       bool          `json:"hasPassword"`
	Passkeys          []passkeyInfo `json:"passkeys"`
	RecoveryCodesLeft int           `json:"recoveryCodesLeft"`
}

func (s *Server) loadSecurity(ctx context.Context, userID int64) (*securityInfo, error) {
	info := &securityInfo{PasskeysEnabled: s.wa != nil, Passkeys: []passkeyInfo{}}
	if err := s.rdb.QueryRowContext(ctx, `
		SELECT password_hash != '', (SELECT COUNT(*) FROM recovery_codes WHERE user_id = u.id)
		FROM users u WHERE id = ?`, userID).Scan(&info.HasPassword, &info.RecoveryCodesLeft); err != nil {
		return nil, err
	}
	rows, err := s.rdb.QueryContext(ctx,
		`SELECT id, name, created_at, last_used_at FROM passkeys WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p passkeyInfo
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &p.LastUsedAt); err != nil {
			return nil, err
		}
		info.Passkeys = append(info.Passkeys, p)
	}
	return info, rows.Err()
}

func (s *Server) getSecurity(w http.ResponseWriter, r *http.Request, u *User) error {
	info, err := s.loadSecurity(r.Context(), u.ID)
	if err != nil {
		return err
	}
	writeJSON(w, 200, info)
	return nil
}

// userCredentials loads the user's passkeys and WebAuthn ID, creating the
// ID for accounts that predate passkeys.
func (s *Server) userCredentials(ctx context.Context, u *User) (*waUser, error) {
	wu := &waUser{name: u.Username, displayName: u.DisplayName}
	if err := s.db.QueryRowContext(ctx, `SELECT webauthn_id FROM users WHERE id = ?`, u.ID).Scan(&wu.id); err != nil {
		return nil, err
	}
	if len(wu.id) == 0 {
		wu.id = newWebAuthnID()
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET webauthn_id = ? WHERE id = ?`, wu.id, u.ID); err != nil {
			return nil, err
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT credential FROM passkeys WHERE user_id = ?`, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, err
		}
		wu.creds = append(wu.creds, c)
	}
	return wu, rows.Err()
}

const maxPasskeys = 20

func (s *Server) addPasskeyBegin(w http.ResponseWriter, r *http.Request, u *User) error {
	if err := s.passkeysOn(); err != nil {
		return err
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	name, err := passkeyName(in.Name)
	if err != nil {
		return err
	}
	wu, err := s.userCredentials(r.Context(), u)
	if err != nil {
		return err
	}
	if len(wu.creds) >= maxPasskeys {
		return errf(409, "you can have at most %d passkeys", maxPasskeys)
	}
	opts, sess, err := s.wa.BeginRegistration(wu, registrationOpts(wu.creds)...)
	if err != nil {
		return err
	}
	id, err := s.ceremonies.put(&ceremony{kind: ceremonyAdd, session: *sess, userID: u.ID, webauthnID: wu.id, passkeyName: name})
	if err != nil {
		return err
	}
	writeJSON(w, 200, ceremonyStart{id, opts})
	return nil
}

func (s *Server) addPasskeyFinish(w http.ResponseWriter, r *http.Request, u *User) error {
	if err := s.passkeysOn(); err != nil {
		return err
	}
	var in ceremonyFinish
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	cer, err := s.ceremonies.take(in.Ceremony, ceremonyAdd)
	if err != nil {
		return err
	}
	if cer.userID != u.ID {
		return errf(400, "that passkey request belongs to another account")
	}
	wu, err := s.userCredentials(r.Context(), u)
	if err != nil {
		return err
	}
	cred, err := s.verifyRegistration(r, wu, cer, in.Credential)
	if err != nil {
		return err
	}
	credJSON, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(),
		`INSERT INTO passkeys(user_id, credential_id, credential, name, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, cred.ID, string(credJSON), cer.passkeyName, now()); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errf(409, "that passkey is already registered")
		}
		return err
	}
	return s.getSecurity(w, r, u)
}

func (s *Server) renamePasskey(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	name, err := passkeyName(in.Name)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE passkeys SET name = ? WHERE id = ? AND user_id = ?`, name, id, u.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return s.getSecurity(w, r, u)
}

// deletePasskey refuses to remove the last way to sign in.
func (s *Server) deletePasskey(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	info, err := s.loadSecurity(r.Context(), u.ID)
	if err != nil {
		return err
	}
	if !info.HasPassword && len(info.Passkeys) <= 1 {
		return errf(409, "add a password or another passkey before removing your last passkey")
	}
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM passkeys WHERE id = ? AND user_id = ?`, id, u.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return s.getSecurity(w, r, u)
}

// removePassword makes the account passkey-only. It hands out recovery
// codes if the user has none left.
func (s *Server) removePassword(w http.ResponseWriter, r *http.Request, u *User) error {
	info, err := s.loadSecurity(r.Context(), u.ID)
	if err != nil {
		return err
	}
	if len(info.Passkeys) == 0 {
		return errf(409, "add a passkey before removing your password")
	}
	out := map[string]any{}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET password_hash = '' WHERE id = ?`, u.ID); err != nil {
		return err
	}
	if info.RecoveryCodesLeft == 0 {
		codes := newRecoveryCodes()
		if err := storeRecoveryCodes(r.Context(), s.db, u.ID, codes); err != nil {
			return err
		}
		out["recoveryCodes"] = codes
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) regenerateRecoveryCodes(w http.ResponseWriter, r *http.Request, u *User) error {
	codes := newRecoveryCodes()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := storeRecoveryCodes(r.Context(), tx, u.ID, codes); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"recoveryCodes": codes})
	return nil
}

func newWebAuthnID() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
