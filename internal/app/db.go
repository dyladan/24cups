package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"runtime"
	"time"

	"github.com/XSAM/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	_ "modernc.org/sqlite"
)

// OpenDB opens (creating if needed) the SQLite database and applies migrations.
func OpenDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", path)
	db, err := openInstrumented(dsn, "write")
	if err != nil {
		return nil, err
	}
	// A single connection keeps SQLite writes serialized and avoids SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// OpenReadDB opens a pool of read-only connections. With WAL, readers run
// in parallel with each other and with the single writer from OpenDB.
func OpenReadDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=query_only(1)", path)
	db, err := openInstrumented(dsn, "read")
	if err != nil {
		return nil, err
	}
	n := runtime.NumCPU() * 2
	db.SetMaxOpenConns(n)
	db.SetMaxIdleConns(n)
	return db, nil
}

// openInstrumented opens SQLite with OpenTelemetry spans per query (row
// iteration and connection-housekeeping spans omitted as noise) and
// connection-pool metrics labelled by pool.
func openInstrumented(dsn, pool string) (*sql.DB, error) {
	attrs := otelsql.WithAttributes(semconv.DBSystemNameSQLite)
	db, err := otelsql.Open("sqlite", dsn, attrs, otelsql.WithSpanOptions(otelsql.SpanOptions{
		OmitRows: true, OmitConnResetSession: true, OmitConnPrepare: true, DisableErrSkip: true,
		OmitConnectorConnect: true,
		// Only trace queries made on behalf of a traced request; startup work
		// (migrations, seeding, loading stats) would otherwise produce dozens
		// of single-span traces on every boot.
		SpanFilter: func(ctx context.Context, _ otelsql.Method, _ string, _ []driver.NamedValue) bool {
			return trace.SpanContextFromContext(ctx).IsValid()
		},
	}))
	if err != nil {
		return nil, err
	}
	if _, err := otelsql.RegisterDBStatsMetrics(db, otelsql.WithAttributes(
		semconv.DBSystemNameSQLite, semconv.DBClientConnectionPoolName(pool))); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

var migrations = []string{
	// v1: initial schema
	`
CREATE TABLE users (
	id              INTEGER PRIMARY KEY,
	username        TEXT NOT NULL UNIQUE COLLATE NOCASE,
	display_name    TEXT NOT NULL,
	password_hash   TEXT NOT NULL,
	reveal_unopened INTEGER NOT NULL DEFAULT 0,
	temp_unit       TEXT NOT NULL DEFAULT 'C',
	created_at      TEXT NOT NULL
);

CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at INTEGER NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE coffees (
	id            INTEGER PRIMARY KEY,
	day           INTEGER NOT NULL UNIQUE,
	name          TEXT NOT NULL,
	origin        TEXT NOT NULL DEFAULT '',
	region        TEXT NOT NULL DEFAULT '',
	producer      TEXT NOT NULL DEFAULT '',
	process       TEXT NOT NULL DEFAULT '',
	variety       TEXT NOT NULL DEFAULT '',
	elevation     TEXT NOT NULL DEFAULT '',
	roast_level   TEXT NOT NULL DEFAULT '',
	tasting_notes TEXT NOT NULL DEFAULT '',
	description   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE coffee_opens (
	user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	coffee_id INTEGER NOT NULL REFERENCES coffees(id) ON DELETE CASCADE,
	opened_at TEXT NOT NULL,
	PRIMARY KEY (user_id, coffee_id)
);
CREATE INDEX coffee_opens_coffee ON coffee_opens(coffee_id);

CREATE TABLE favorites (
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	coffee_id  INTEGER NOT NULL REFERENCES coffees(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	PRIMARY KEY (user_id, coffee_id)
);
CREATE INDEX favorites_coffee ON favorites(coffee_id);

CREATE TABLE recipes (
	id            INTEGER PRIMARY KEY,
	user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name          TEXT NOT NULL,
	method        TEXT NOT NULL,
	dose_g        REAL,
	ratio         REAL NOT NULL,
	temp_c        REAL,
	target_time_s INTEGER,
	notes         TEXT NOT NULL DEFAULT '',
	created_at    TEXT NOT NULL
);
CREATE INDEX recipes_user ON recipes(user_id);

CREATE TABLE grinders (
	id         INTEGER PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	model      TEXT NOT NULL,
	nickname   TEXT NOT NULL DEFAULT '',
	burrsets   TEXT NOT NULL DEFAULT '[]',
	notes      TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE INDEX grinders_user ON grinders(user_id);

CREATE TABLE brews (
	id                  INTEGER PRIMARY KEY,
	user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	coffee_id           INTEGER NOT NULL REFERENCES coffees(id) ON DELETE CASCADE,
	recipe_id           INTEGER REFERENCES recipes(id) ON DELETE SET NULL,
	method              TEXT NOT NULL,
	dose_g              REAL,
	ratio               REAL NOT NULL,
	temp_c              REAL,
	target_time_s       INTEGER,
	actual_time_s       INTEGER,
	grinder_id          INTEGER REFERENCES grinders(id) ON DELETE SET NULL,
	grinder_label       TEXT NOT NULL DEFAULT '',
	burrset             TEXT NOT NULL DEFAULT '',
	grind_setting       TEXT NOT NULL DEFAULT '',
	rating              INTEGER CHECK (rating IS NULL OR rating BETWEEN 1 AND 10),
	notes               TEXT NOT NULL DEFAULT '',
	is_public           INTEGER NOT NULL DEFAULT 1,
	exclude_from_global INTEGER NOT NULL DEFAULT 0,
	brewed_at           TEXT NOT NULL,
	created_at          TEXT NOT NULL
);
CREATE INDEX brews_coffee ON brews(coffee_id);
CREATE INDEX brews_user ON brews(user_id);

CREATE TABLE groups (
	id          INTEGER PRIMARY KEY,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	is_public   INTEGER NOT NULL DEFAULT 1,
	invite_code TEXT NOT NULL UNIQUE,
	owner_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at  TEXT NOT NULL
);

CREATE TABLE group_members (
	group_id  INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
	user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role      TEXT NOT NULL DEFAULT 'member',
	joined_at TEXT NOT NULL,
	PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user ON group_members(user_id);
`,
	// v2: indexes for per-user lookups and the coffee brew feed
	`
CREATE INDEX brews_user_coffee ON brews(user_id, coffee_id);
CREATE INDEX brews_coffee_brewed ON brews(coffee_id, brewed_at);
`,
	// v3: spoiler modes replace the reveal_unopened toggle; per-coffee reveal
	`
ALTER TABLE users ADD COLUMN spoiler_mode TEXT NOT NULL DEFAULT 'rated';
UPDATE users SET spoiler_mode = 'always' WHERE reveal_unopened = 1;
ALTER TABLE users DROP COLUMN reveal_unopened;
ALTER TABLE coffee_opens ADD COLUMN revealed INTEGER NOT NULL DEFAULT 0;
`,
	// v4: flavor descriptors tagged on brews (normalized lowercase)
	`
CREATE TABLE brew_descriptors (
	brew_id    INTEGER NOT NULL REFERENCES brews(id) ON DELETE CASCADE,
	descriptor TEXT NOT NULL,
	PRIMARY KEY (brew_id, descriptor)
);
`,
	// v5: per-group display names ('' = use the profile display name)
	`
ALTER TABLE group_members ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
`,
	// v6: passkeys and recovery codes. Passkey-only accounts have an empty
	// password_hash. webauthn_id is the random user handle passkeys carry.
	`
ALTER TABLE users ADD COLUMN webauthn_id BLOB;
CREATE UNIQUE INDEX users_webauthn_id ON users(webauthn_id);
CREATE TABLE passkeys (
	id            INTEGER PRIMARY KEY,
	user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	credential_id BLOB NOT NULL UNIQUE,
	credential    TEXT NOT NULL, -- JSON: public key, sign count, flags
	name          TEXT NOT NULL,
	created_at    TEXT NOT NULL,
	last_used_at  TEXT
);
CREATE INDEX passkeys_user ON passkeys(user_id);
CREATE TABLE recovery_codes (
	user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	code_hash TEXT NOT NULL,
	PRIMARY KEY (user_id, code_hash)
);
`,
	// v7: quarter-star ratings. rating moves from 1–10 (half-stars) to 1–20
	// (quarter-stars); SQLite can't alter a CHECK, so swap in a new column.
	`
ALTER TABLE brews ADD COLUMN rating_q INTEGER CHECK (rating_q IS NULL OR rating_q BETWEEN 1 AND 20);
UPDATE brews SET rating_q = rating * 2;
ALTER TABLE brews DROP COLUMN rating;
ALTER TABLE brews RENAME COLUMN rating_q TO rating;
`,
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
