package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

const testOrigin = "http://localhost"

func setupPasskeys(t *testing.T) *httptest.Server {
	path := filepath.Join(t.TempDir(), "t.db")
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rdb, _ := OpenReadDB(path)
	t.Cleanup(func() { rdb.Close() })
	h, err := NewServer(db, rdb, Options{
		Limits: testLimits, ClientIPHeader: "X-Test-IP",
		Passkeys: PasskeyConfig{RPID: "localhost", Origins: []string{testOrigin}},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// softKey is a minimal software authenticator: one ES256 discoverable
// credential, "none" attestation.
type softKey struct {
	key        *ecdsa.PrivateKey
	id         []byte
	userHandle []byte
	count      uint32
}

var b64 = base64.RawURLEncoding

func newSoftKey() *softKey {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	id := make([]byte, 16)
	rand.Read(id)
	return &softKey{key: k, id: id}
}

func rpHash() []byte { h := sha256.Sum256([]byte("localhost")); return h[:] }

func clientData(typ, challenge string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": testOrigin, "crossOrigin": false})
	return b
}

type startResp struct {
	Ceremony string `json:"ceremony"`
	Options  struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			User      struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	} `json:"options"`
}

// create answers navigator.credentials.create().
func (k *softKey) create(t *testing.T, s startResp) string {
	t.Helper()
	k.userHandle, _ = b64.DecodeString(s.Options.PublicKey.User.ID)
	cose, _ := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1,
		-2: k.key.X.FillBytes(make([]byte, 32)), -3: k.key.Y.FillBytes(make([]byte, 32))})
	auth := append(rpHash(), 0x45) // UP | UV | AT
	auth = binary.BigEndian.AppendUint32(auth, 0)
	auth = append(auth, make([]byte, 16)...) // AAGUID
	auth = binary.BigEndian.AppendUint16(auth, uint16(len(k.id)))
	auth = append(auth, k.id...)
	auth = append(auth, cose...)
	att, _ := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": auth})
	return k.wrap(s.Ceremony, map[string]any{
		"clientDataJSON":    b64.EncodeToString(clientData("webauthn.create", s.Options.PublicKey.Challenge)),
		"attestationObject": b64.EncodeToString(att),
	})
}

// get answers navigator.credentials.get().
func (k *softKey) get(t *testing.T, s startResp) string {
	t.Helper()
	k.count++
	auth := append(rpHash(), 0x05) // UP | UV
	auth = binary.BigEndian.AppendUint32(auth, k.count)
	cd := clientData("webauthn.get", s.Options.PublicKey.Challenge)
	cdHash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, auth...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return k.wrap(s.Ceremony, map[string]any{
		"clientDataJSON":    b64.EncodeToString(cd),
		"authenticatorData": b64.EncodeToString(auth),
		"signature":         b64.EncodeToString(sig),
		"userHandle":        b64.EncodeToString(k.userHandle),
	})
}

func (k *softKey) wrap(ceremony string, response map[string]any) string {
	b, _ := json.Marshal(map[string]any{"ceremony": ceremony, "credential": map[string]any{
		"id": b64.EncodeToString(k.id), "rawId": b64.EncodeToString(k.id), "type": "public-key", "response": response,
	}})
	return string(b)
}

func passkeySignup(t *testing.T, c *client, k *softKey, name string) []string {
	t.Helper()
	var s startResp
	if st := c.do("POST", "/auth/passkey/register/begin", fmt.Sprintf(`{"username":%q}`, name), &s); st != 200 {
		t.Fatalf("register begin: %d", st)
	}
	var out struct {
		User          User     `json:"user"`
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if st := c.do("POST", "/auth/passkey/register/finish", k.create(t, s), &out); st != 201 {
		t.Fatalf("register finish: %d", st)
	}
	if out.User.Username != name || len(out.RecoveryCodes) != recoveryCodeN {
		t.Fatalf("register result: %+v", out)
	}
	return out.RecoveryCodes
}

func passkeyLogin(t *testing.T, c *client, k *softKey) int {
	t.Helper()
	var s startResp
	if st := c.do("POST", "/auth/passkey/login/begin", `{}`, &s); st != 200 {
		t.Fatalf("login begin: %d", st)
	}
	return c.do("POST", "/auth/passkey/login/finish", k.get(t, s), nil)
}

func TestPasskeySignupAndLogin(t *testing.T) {
	srv := setupPasskeys(t)
	c := newClient(t, srv)
	k := newSoftKey()
	codes := passkeySignup(t, c, k, "pk-user")

	var sec securityInfo
	c.do("GET", "/me/security", "", &sec)
	if sec.HasPassword || len(sec.Passkeys) != 1 || sec.RecoveryCodesLeft != recoveryCodeN {
		t.Fatalf("security after signup: %+v", sec)
	}
	// Passkey-only accounts can't sign in with any password, including empty.
	if st := c.do("POST", "/auth/login", `{"username":"pk-user","password":""}`, nil); st != 401 {
		t.Errorf("empty password sign-in: %d", st)
	}

	// Fresh browser: sign in with the passkey, no username.
	c2 := newClient(t, srv)
	if st := passkeyLogin(t, c2, k); st != 200 {
		t.Fatalf("passkey login: %d", st)
	}
	var me User
	c2.do("GET", "/auth/me", "", &me)
	if me.Username != "pk-user" {
		t.Fatalf("me after passkey login: %+v", me)
	}

	// A challenge answers once.
	var s startResp
	c2.do("POST", "/auth/passkey/login/begin", `{}`, &s)
	body := k.get(t, s)
	if st := c2.do("POST", "/auth/passkey/login/finish", body, nil); st != 200 {
		t.Fatalf("login: %d", st)
	}
	if st := c2.do("POST", "/auth/passkey/login/finish", body, nil); st != 400 {
		t.Errorf("replayed challenge: %d", st)
	}

	// An unknown key is rejected.
	stranger := newSoftKey()
	stranger.userHandle = k.userHandle
	if st := passkeyLogin(t, newClient(t, srv), stranger); st != 401 {
		t.Errorf("unknown passkey: %d", st)
	}

	// Recovery code: works once, signs out other sessions.
	c3 := newClient(t, srv)
	rec := fmt.Sprintf(`{"username":"PK-user","code":%q}`, " "+codes[0]+" ")
	if st := c3.do("POST", "/auth/recover", rec, nil); st != 200 {
		t.Fatalf("recover: %d", st)
	}
	if st := c2.do("GET", "/auth/me", "", nil); st != 401 {
		t.Errorf("old session survived recovery: %d", st)
	}
	if st := newClient(t, srv).do("POST", "/auth/recover", rec, nil); st != 401 {
		t.Errorf("reused recovery code: %d", st)
	}
	c3.do("GET", "/me/security", "", &sec)
	if sec.RecoveryCodesLeft != recoveryCodeN-1 {
		t.Errorf("codes left: %d", sec.RecoveryCodesLeft)
	}

	// The last passkey can't be removed without a password.
	if st := c3.do("DELETE", fmt.Sprintf("/me/passkeys/%d", sec.Passkeys[0].ID), "", nil); st != 409 {
		t.Errorf("delete last passkey: %d", st)
	}
	// Adding a first password needs no current password; then it can go.
	if st := c3.do("POST", "/me/password", `{"newPassword":"password1"}`, nil); st != 204 {
		t.Fatalf("set first password: %d", st)
	}
	if st := c3.do("DELETE", fmt.Sprintf("/me/passkeys/%d", sec.Passkeys[0].ID), "", &sec); st != 200 || len(sec.Passkeys) != 0 {
		t.Errorf("delete passkey with password set: %d %+v", st, sec)
	}
	if st := passkeyLogin(t, newClient(t, srv), k); st != 401 {
		t.Errorf("deleted passkey still signs in: %d", st)
	}
}

func TestPasskeyAddToPasswordAccount(t *testing.T) {
	srv := setupPasskeys(t)
	c := newClient(t, srv)
	c.do("POST", "/auth/register", `{"username":"pw-user","password":"password1"}`, nil)

	// No passkey yet: the password can't be removed.
	if st := c.do("DELETE", "/me/password", "", nil); st != 409 {
		t.Errorf("remove only password: %d", st)
	}
	k := newSoftKey()
	var s startResp
	if st := c.do("POST", "/me/passkeys/begin", `{"name":"Laptop"}`, &s); st != 200 {
		t.Fatalf("add begin: %d", st)
	}
	var sec securityInfo
	if st := c.do("POST", "/me/passkeys/finish", k.create(t, s), &sec); st != 200 {
		t.Fatalf("add finish: %d", st)
	}
	if !sec.HasPassword || len(sec.Passkeys) != 1 || sec.Passkeys[0].Name != "Laptop" {
		t.Fatalf("after add: %+v", sec)
	}
	// Another account can't finish this user's ceremony.
	other := newClient(t, srv)
	other.do("POST", "/auth/register", `{"username":"other","password":"password1"}`, nil)
	c.do("POST", "/me/passkeys/begin", `{}`, &s)
	if st := other.do("POST", "/me/passkeys/finish", newSoftKey().create(t, s), nil); st != 400 {
		t.Errorf("cross-account finish: %d", st)
	}

	// Going passkey-only hands out recovery codes.
	var out struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if st := c.do("DELETE", "/me/password", "", &out); st != 200 || len(out.RecoveryCodes) != recoveryCodeN {
		t.Fatalf("remove password: %d %+v", st, out)
	}
	if st := newClient(t, srv).do("POST", "/auth/login", `{"username":"pw-user","password":"password1"}`, nil); st != 401 {
		t.Errorf("removed password still works: %d", st)
	}
	if st := passkeyLogin(t, newClient(t, srv), k); st != 200 {
		t.Errorf("passkey login: %d", st)
	}
}

func TestPasskeysDisabled(t *testing.T) {
	c := newClient(t, setup(t))
	if st := c.do("POST", "/auth/passkey/login/begin", `{}`, nil); st != 404 {
		t.Errorf("disabled passkeys: %d", st)
	}
}

func TestRecoveryCodeFormat(t *testing.T) {
	codes := newRecoveryCodes()
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 14 || len(normalizeRecoveryCode(c)) != recoveryCodeLen || seen[c] {
			t.Errorf("bad code %q", c)
		}
		seen[c] = true
	}
	if hashRecoveryCode("abcd-efgh-jkmn") != hashRecoveryCode("ABCD EFGH JKMN") {
		t.Error("normalization")
	}
}
