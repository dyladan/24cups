package app

import (
	"database/sql"
	"testing"
)

func TestBrewVisibility(t *testing.T) {
	srv := setup(t)
	alice, bob, carol := newClient(t, srv), newClient(t, srv), newClient(t, srv)
	alice.do("POST", "/auth/register", `{"username":"alice","password":"password1"}`, nil)
	bob.do("POST", "/auth/register", `{"username":"bob","password":"password1"}`, nil)
	carol.do("POST", "/auth/register", `{"username":"carol","password":"password1"}`, nil)

	// Alice and bob share a group; carol only visits it (it's public).
	var g Group
	alice.do("POST", "/groups", `{"name":"Crew","isPublic":true}`, &g)
	bob.do("POST", "/groups/"+itoa(g.ID)+"/join", "", nil)

	// Alice's default is groups; a brew without visibility picks it up.
	var me User
	if st := alice.do("PATCH", "/me", `{"defaultVisibility":"groups"}`, &me); st != 200 || me.DefaultVisibility != "groups" {
		t.Fatalf("set default: %d %+v", st, me)
	}
	if st := alice.do("PATCH", "/me", `{"defaultVisibility":"friends"}`, nil); st != 400 {
		t.Errorf("bad default accepted: %d", st)
	}
	var shared, priv, pub Brew
	alice.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":16,"notes":"groups"}`, &shared)
	if shared.Visibility != "groups" {
		t.Errorf("default not applied: %q", shared.Visibility)
	}
	alice.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":4,"visibility":"private","notes":"private"}`, &priv)
	// Older clients still send isPublic.
	alice.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":8,"isPublic":true,"excludeFromGlobal":true,"notes":"public"}`, &pub)
	if pub.Visibility != "public" || !pub.ExcludeFromGlobal {
		t.Errorf("legacy public+excluded brew: %+v", pub)
	}
	if st := alice.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"visibility":"friends"}`, nil); st != 400 {
		t.Errorf("bad visibility accepted: %d", st)
	}
	// Editing without visibility keeps it.
	var edited Brew
	alice.do("PUT", "/brews/"+itoa(shared.ID), `{"coffeeId":1,"method":"V60","ratio":15,"rating":16,"notes":"groups"}`, &edited)
	if edited.Visibility != "groups" {
		t.Errorf("edit changed visibility to %q", edited.Visibility)
	}

	notes := func(list []*Brew) map[string]bool {
		m := map[string]bool{}
		for _, b := range list {
			m[b.Notes] = true
		}
		return m
	}
	for _, c := range []*client{bob, carol} {
		c.do("POST", "/coffees/1/open", "", nil)
		c.do("POST", "/coffees/1/reveal", "", nil)
	}
	var list []*Brew
	bob.do("GET", "/coffees/1/brews", "", &list)
	if n := notes(list); !n["groups"] || !n["public"] || n["private"] {
		t.Errorf("bob (group member) sees %v", n)
	}
	carol.do("GET", "/coffees/1/brews", "", &list)
	if n := notes(list); n["groups"] || !n["public"] || n["private"] {
		t.Errorf("carol (no shared group) sees %v", n)
	}
	if st := carol.do("GET", "/brews/"+itoa(shared.ID), "", nil); st != 404 {
		t.Errorf("carol fetched groups brew: %d", st)
	}
	if st := bob.do("GET", "/brews/"+itoa(shared.ID), "", nil); st != 200 {
		t.Errorf("bob fetch groups brew: %d", st)
	}

	// Group page: members see groups brews, visitors don't.
	var d GroupDetail
	bob.do("GET", "/groups/"+itoa(g.ID), "", &d)
	if n := notes(d.Brews); !n["groups"] || !n["public"] || n["private"] {
		t.Errorf("group feed for member: %v", n)
	}
	for _, m := range d.Members {
		if m.Username == "alice" && m.Brews != 2 {
			t.Errorf("member brew count for member: %d, want 2", m.Brews)
		}
	}
	carol.do("GET", "/groups/"+itoa(g.ID), "", &d)
	if n := notes(d.Brews); n["groups"] || !n["public"] {
		t.Errorf("group feed for visitor: %v", n)
	}

	// Averages: groups (4 stars) and private (1 star) count; the excluded
	// public brew doesn't. Alice's average is 2.5 stars.
	var c Coffee
	bob.do("GET", "/coffees/1", "", &c)
	if c.Stats == nil || c.Stats.AvgRating == nil || *c.Stats.AvgRating != 2.5 {
		t.Errorf("stats = %+v", c.Stats)
	}
}

func TestVisibilityMigration(t *testing.T) {
	path := t.TempDir() + "/old.db"
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	// Rebuild a v7 database: undo v8, then mark it v7.
	for _, q := range []string{
		`ALTER TABLE brews ADD COLUMN is_public INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE brews DROP COLUMN visibility`,
		`ALTER TABLE users DROP COLUMN default_visibility`,
		`INSERT INTO users(username, display_name, password_hash, created_at) VALUES ('u','u','x','t')`,
		`INSERT INTO coffees(day, name) VALUES (1, 'One')`,
		`INSERT INTO brews(user_id, coffee_id, method, ratio, is_public, brewed_at, created_at) VALUES (1, 1, 'pub', 16, 1, 't', 't'), (1, 1, 'priv', 16, 0, 't', 't')`,
		`PRAGMA user_version = 7`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	db.Close()
	db, err = OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got := map[string]string{}
	rows, _ := db.Query(`SELECT method, visibility FROM brews`)
	for rows.Next() {
		var m, v string
		rows.Scan(&m, &v)
		got[m] = v
	}
	rows.Close()
	if got["pub"] != "public" || got["priv"] != "private" {
		t.Errorf("migration: %v", got)
	}
	var def string
	if err := db.QueryRow(`SELECT default_visibility FROM users`).Scan(&def); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if def != "public" {
		t.Errorf("default_visibility = %q", def)
	}
}
