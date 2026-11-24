package app

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// OpenDB opens (creating if needed) the SQLite database and applies migrations.
func OpenDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
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
