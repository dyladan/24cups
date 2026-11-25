package app

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func users(t *testing.T, s *client, n int) []*client {
	out := make([]*client, n)
	for i := range out {
		out[i] = newClient(t, s.srv)
		if st := out[i].do("POST", "/auth/register", fmt.Sprintf(`{"username":"taster%d","password":"password1"}`, i), nil); st != 201 {
			t.Fatalf("register: %d", st)
		}
	}
	return out
}

func brewWith(c *client, coffee int, flavors string, extra string) (Brew, int) {
	var b Brew
	st := c.do("POST", "/brews", fmt.Sprintf(`{"coffeeId":%d,"method":"V60","ratio":16,"rating":8,"isPublic":true,"descriptors":[%s]%s}`, coffee, flavors, extra), &b)
	return b, st
}

func flavorsOf(t *testing.T, c *client, coffee int) *Flavors {
	t.Helper()
	var d CoffeeDetail
	if st := c.do("GET", fmt.Sprintf("/coffees/%d", coffee), "", &d); st != 200 {
		t.Fatalf("get coffee: %d", st)
	}
	return d.Flavors
}

func topMap(f *Flavors) map[string]int {
	m := map[string]int{}
	for _, x := range f.Top {
		m[x.Name] = x.Count
	}
	return m
}

func TestFlavorValidation(t *testing.T) {
	srv := setup(t)
	u := users(t, &client{t: t, srv: srv}, 1)[0]

	b, st := brewWith(u, 1, `"  Black   TEA ","black tea","Peach",""`, "")
	if st != 201 || !reflect.DeepEqual(b.Descriptors, []string{"black tea", "peach"}) {
		t.Fatalf("normalize: %d %v", st, b.Descriptors)
	}
	many := make([]string, 11)
	for i := range many {
		many[i] = fmt.Sprintf(`"f%d"`, i)
	}
	if _, st := brewWith(u, 1, strings.Join(many, ","), ""); st != 400 {
		t.Errorf("11 flavors accepted: %d", st)
	}
	if _, st := brewWith(u, 1, `"`+strings.Repeat("x", 31)+`"`, ""); st != 400 {
		t.Errorf("long flavor accepted: %d", st)
	}
	// Edits replace the set, keeping the given order.
	var e Brew
	u.do("PUT", fmt.Sprintf("/brews/%d", b.ID), `{"coffeeId":1,"method":"V60","ratio":16,"isPublic":true,"descriptors":["Honey","jasmine"]}`, &e)
	if !reflect.DeepEqual(e.Descriptors, []string{"honey", "jasmine"}) {
		t.Errorf("edit: %v", e.Descriptors)
	}
	var mine []Brew
	u.do("GET", "/brews", "", &mine)
	if len(mine) != 1 || !reflect.DeepEqual(mine[0].Descriptors, []string{"honey", "jasmine"}) {
		t.Errorf("list: %+v", mine)
	}
}

func TestFlavorCounts(t *testing.T) {
	srv := setup(t)
	us := users(t, &client{t: t, srv: srv}, 5)
	a, b, c, d, e := us[0], us[1], us[2], us[3], us[4]

	// a tags peach twice: still one taster.
	brewWith(a, 1, `"peach","grilled peach"`, "")
	brewWith(a, 1, `"peach"`, "")
	brewWith(b, 1, `"peach","honey"`, "")

	f := flavorsOf(t, a, 1)
	if f == nil || f.Tasters != 2 || f.Top != nil {
		t.Fatalf("below threshold: %+v", f)
	}

	// A private brew that's excluded doesn't count; a private included one does.
	brewWith(c, 1, `"cocoa"`, `,"isPublic":false,"excludeFromGlobal":true`)
	if f := flavorsOf(t, a, 1); f.Tasters != 2 {
		t.Fatalf("excluded brew counted: %+v", f)
	}
	brewWith(d, 1, `"honey"`, `,"isPublic":false`)

	f = flavorsOf(t, a, 1)
	got := topMap(f)
	// "grilled peach" is custom with one user: hidden.
	want := map[string]int{"peach": 2, "honey": 2}
	if f.Tasters != 3 || !reflect.DeepEqual(got, want) {
		t.Fatalf("counts: tasters=%d %v", f.Tasters, got)
	}
	for _, x := range f.Top {
		if x.Mine != (x.Name == "peach") {
			t.Errorf("mine flag wrong for %s", x.Name)
		}
	}
	// A custom flavor shows once a second person uses it.
	brewWith(e, 1, `"grilled peach"`, "")
	f = flavorsOf(t, c, 1)
	if topMap(f)["grilled peach"] != 2 {
		t.Errorf("custom flavor used twice should show: %v", topMap(f))
	}

	// e opened coffee 2 without rating: blind, so only the count.
	for _, u := range []*client{a, b, c} {
		brewWith(u, 2, `"lemon"`, "")
	}
	e.do("POST", "/coffees/2/open", "", nil)
	if f := flavorsOf(t, e, 2); f == nil || f.Tasters != 3 || f.Top != nil {
		t.Errorf("blind viewer: %+v", f)
	}
	e.do("POST", "/coffees/2/reveal", "", nil)
	if f := flavorsOf(t, e, 2); topMap(f)["lemon"] != 3 {
		t.Errorf("revealed viewer: %+v", f)
	}

	// Sealed door (default spoiler mode): nothing at all.
	fresh := newClient(t, srv)
	fresh.do("POST", "/auth/register", `{"username":"fresh","password":"password1"}`, nil)
	if f := flavorsOf(t, fresh, 2); f != nil {
		t.Errorf("sealed door leaked flavors: %+v", f)
	}
}

// The in-memory flavor sets must match a straight SQL recount after random
// creates, edits (incl. moving coffees and toggling privacy) and deletes.
func TestFlavorStoreMatchesDatabase(t *testing.T) {
	srv := setup(t)
	s := srv.Config.Handler.(*Server)
	us := users(t, &client{t: t, srv: srv}, 4)
	rng := rand.New(rand.NewSource(7))
	pool := []string{"peach", "honey", "lemon", "cocoa", "odd thing"}
	ids := map[int][]int64{}
	for step := 0; step < 200; step++ {
		ui := rng.Intn(len(us))
		var tags []string
		for _, p := range pool {
			if rng.Intn(3) == 0 {
				tags = append(tags, `"`+p+`"`)
			}
		}
		body := fmt.Sprintf(`{"coffeeId":%d,"method":"V60","ratio":16,"isPublic":%t,"excludeFromGlobal":%t,"descriptors":[%s]}`,
			rng.Intn(2)+1, rng.Intn(2) == 0, rng.Intn(2) == 0, strings.Join(tags, ","))
		switch op := rng.Intn(4); {
		case op <= 1:
			var b Brew
			us[ui].do("POST", "/brews", body, &b)
			if b.ID != 0 {
				ids[ui] = append(ids[ui], b.ID)
			}
		case op == 2 && len(ids[ui]) > 0:
			us[ui].do("PUT", fmt.Sprintf("/brews/%d", ids[ui][rng.Intn(len(ids[ui]))]), body, nil)
		case op == 3 && len(ids[ui]) > 0:
			i := rng.Intn(len(ids[ui]))
			us[ui].do("DELETE", fmt.Sprintf("/brews/%d", ids[ui][i]), "", nil)
			ids[ui] = append(ids[ui][:i], ids[ui][i+1:]...)
		}
	}
	for coffee := int64(1); coffee <= 2; coffee++ {
		tasters, counts := s.stats.flavors(coffee)
		var wantTasters int
		s.rdb.QueryRow(`SELECT COUNT(DISTINCT b.user_id) FROM brew_descriptors d JOIN brews b ON b.id = d.brew_id
			WHERE b.coffee_id = ? AND `+includedFlavors, coffee).Scan(&wantTasters)
		want := map[string]int{}
		rows, _ := s.rdb.Query(`SELECT d.descriptor, COUNT(DISTINCT b.user_id) FROM brew_descriptors d JOIN brews b ON b.id = d.brew_id
			WHERE b.coffee_id = ? AND `+includedFlavors+` GROUP BY 1`, coffee)
		for rows.Next() {
			var d string
			var n int
			rows.Scan(&d, &n)
			want[d] = n
		}
		rows.Close()
		if tasters != wantTasters || !reflect.DeepEqual(counts, want) {
			t.Errorf("coffee %d: store %d %v, db %d %v", coffee, tasters, counts, wantTasters, want)
		}
	}
}

func TestFlavorCatalogIsClean(t *testing.T) {
	seen := map[string]string{}
	for _, g := range catalogFlavors {
		for _, f := range g.Items {
			if f != normalizeDescriptor(f) || len([]rune(f)) > maxDescriptorLen {
				t.Errorf("%s: %q isn't normalized or is too long", g.Group, f)
			}
			if prev, ok := seen[f]; ok {
				t.Errorf("%q is in both %s and %s", f, prev, g.Group)
			}
			seen[f] = g.Group
		}
	}
}
