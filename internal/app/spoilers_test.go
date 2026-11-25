package app

import (
	"fmt"
	"testing"
)

func TestBlindUntilRated(t *testing.T) {
	srv := setup(t)
	s := srv.Config.Handler.(*Server)
	s.db.Exec(`UPDATE coffees SET tasting_notes = 'Cherry, Cocoa' WHERE id = 1`)
	alice, bob := newClient(t, srv), newClient(t, srv)
	alice.do("POST", "/auth/register", `{"username":"alice","password":"password1"}`, nil)
	bob.do("POST", "/auth/register", `{"username":"bob","password":"password1"}`, nil)

	var b Brew
	alice.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":9,"isPublic":true,"notes":"so good"}`, &b)
	alice.do("PUT", "/coffees/1/favorite", "", nil)
	var g Group
	alice.do("POST", "/groups", `{"name":"Crew","isPublic":true}`, &g)
	bob.do("POST", fmt.Sprintf("/groups/%d/join", g.ID), "", nil)

	// Bob opens without brewing: notes and scores hidden, opens visible.
	bob.do("POST", "/coffees/1/open", "", nil)
	var c CoffeeDetail
	bob.do("GET", "/coffees/1", "", &c)
	if c.Revealed || c.Info == nil || c.Info.Name == "" {
		t.Fatalf("want opened-but-hidden, got revealed=%v info=%+v", c.Revealed, c.Info)
	}
	if c.Info.TastingNotes != "" {
		t.Errorf("tasting notes leaked: %q", c.Info.TastingNotes)
	}
	if c.Stats == nil || c.Stats.AvgRating != nil || c.Stats.Favorites != 0 || c.Stats.RatingCount != 0 || c.Stats.Opens != 2 {
		t.Errorf("stats leaked or opens missing: %+v", c.Stats)
	}
	if c.Distribution != nil || c.GroupStats != nil {
		t.Errorf("distribution/group stats leaked")
	}
	var brews []Brew
	bob.do("GET", "/coffees/1/brews", "", &brews)
	if len(brews) != 0 {
		t.Errorf("community brews leaked: %d", len(brews))
	}
	if st := bob.do("GET", fmt.Sprintf("/brews/%d", b.ID), "", nil); st != 404 {
		t.Errorf("direct brew fetch: %d", st)
	}
	var gd GroupDetail
	bob.do("GET", fmt.Sprintf("/groups/%d", g.ID), "", &gd)
	if len(gd.Brews) != 0 || gd.Coffees[0].Revealed || gd.Coffees[0].Stats.AvgRating != nil {
		t.Errorf("group leaked: brews=%d coffee=%+v", len(gd.Brews), gd.Coffees[0])
	}

	// An unrated brew doesn't reveal; a rated one does.
	bob.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16}`, nil)
	bob.do("GET", "/coffees/1", "", &c)
	if c.Revealed {
		t.Errorf("unrated brew revealed spoilers")
	}
	bob.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":4}`, nil)
	c = CoffeeDetail{}
	bob.do("GET", "/coffees/1", "", &c)
	if !c.Revealed || c.Info.TastingNotes != "Cherry, Cocoa" || c.Stats.AvgRating == nil || c.Stats.Favorites != 1 {
		t.Errorf("rating didn't reveal: revealed=%v info=%+v stats=%+v", c.Revealed, c.Info, c.Stats)
	}
	bob.do("GET", "/coffees/1/brews", "", &brews)
	if len(brews) != 3 {
		t.Errorf("after rating, brews=%d want 3", len(brews))
	}
}

func TestRevealButtonAndModes(t *testing.T) {
	srv := setup(t)
	alice, bob := newClient(t, srv), newClient(t, srv)
	alice.do("POST", "/auth/register", `{"username":"alice","password":"password1"}`, nil)
	bob.do("POST", "/auth/register", `{"username":"bob","password":"password1"}`, nil)
	alice.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":9,"isPublic":true}`, nil)

	if st := bob.do("POST", "/coffees/1/reveal", "", nil); st != 409 {
		t.Errorf("reveal before opening: %d", st)
	}
	bob.do("POST", "/coffees/1/open", "", nil)
	bob.do("POST", "/coffees/1/reveal", "", nil)
	var c CoffeeDetail
	bob.do("GET", "/coffees/1", "", &c)
	if !c.Revealed || c.Stats.AvgRating == nil {
		t.Errorf("reveal button didn't reveal")
	}

	// "opened": no blind phase. "always": scores even on sealed doors.
	var list []Coffee
	bob.do("PATCH", "/me", `{"spoilerMode":"opened"}`, nil)
	bob.do("POST", "/coffees/2/open", "", nil)
	bob.do("GET", "/coffees", "", &list)
	if !list[1].Revealed {
		t.Errorf("opened mode: coffee 2 hidden")
	}
	carol := newClient(t, srv)
	carol.do("POST", "/auth/register", `{"username":"carol","password":"password1"}`, nil)
	carol.do("GET", "/coffees", "", &list)
	if list[0].Stats != nil {
		t.Errorf("rated mode: sealed door shows stats")
	}
	carol.do("PATCH", "/me", `{"spoilerMode":"always"}`, nil)
	carol.do("GET", "/coffees", "", &list)
	if list[0].Info != nil || list[0].Stats == nil || list[0].Stats.AvgRating == nil {
		t.Errorf("always mode: want scores but no info on sealed door, got %+v", list[0])
	}
	if st := carol.do("PATCH", "/me", `{"spoilerMode":"sometimes"}`, nil); st != 400 {
		t.Errorf("bad mode accepted: %d", st)
	}
}

func TestSpoilerMigrationKeepsOldToggle(t *testing.T) {
	path := t.TempDir() + "/old.db"
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	// Rebuild a v2 database by hand: undo v4 and v3, then mark it v2.
	for _, q := range []string{
		`DROP TABLE brew_descriptors`,
		`ALTER TABLE users DROP COLUMN spoiler_mode`,
		`ALTER TABLE users ADD COLUMN reveal_unopened INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE coffee_opens DROP COLUMN revealed`,
		`INSERT INTO users(username, display_name, password_hash, reveal_unopened, created_at) VALUES ('peeker','p','x',1,'t'), ('blind','b','x',0,'t')`,
		`PRAGMA user_version = 2`,
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
	rows, _ := db.Query(`SELECT username, spoiler_mode FROM users`)
	for rows.Next() {
		var u, m string
		rows.Scan(&u, &m)
		got[u] = m
	}
	rows.Close()
	if got["peeker"] != "always" || got["blind"] != "rated" {
		t.Errorf("migration: %v", got)
	}
}
