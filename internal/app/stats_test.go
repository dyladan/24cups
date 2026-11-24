package app

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// groundTruth recomputes stats with SQL straight from the tables.
func groundTruth(t *testing.T, s *Server) map[int64]Stats {
	out := map[int64]Stats{}
	rows, err := s.rdb.Query(`
		WITH per_user AS (
			SELECT coffee_id, user_id, AVG(rating) AS r, COUNT(*) AS n FROM brews
			WHERE ` + includedRatings + ` GROUP BY coffee_id, user_id)
		SELECT coffee_id, AVG(r), COUNT(*), SUM(n) FROM per_user GROUP BY coffee_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		var avg float64
		var st Stats
		rows.Scan(&id, &avg, &st.RatingCount, &st.BrewCount)
		a := avg / 2
		st.AvgRating = &a
		out[id] = st
	}
	rows.Close()
	for _, q := range []string{"coffee_opens", "favorites"} {
		rows, _ := s.rdb.Query(`SELECT coffee_id, COUNT(*) FROM ` + q + ` GROUP BY 1`)
		for rows.Next() {
			var id int64
			var n int
			rows.Scan(&id, &n)
			st := out[id]
			if q == "coffee_opens" {
				st.Opens = n
			} else {
				st.Favorites = n
			}
			out[id] = st
		}
		rows.Close()
	}
	return out
}

func TestStatsStoreMatchesDatabase(t *testing.T) {
	srv := setup(t)
	s := srv.Config.Handler.(*Server)
	rng := rand.New(rand.NewSource(42))
	users := make([]*client, 6)
	for i := range users {
		users[i] = newClient(t, srv)
		users[i].do("POST", "/auth/register", fmt.Sprintf(`{"username":"user%d","password":"password1"}`, i), nil)
	}
	brewIDs := map[int][]int64{}
	for step := 0; step < 400; step++ {
		ui := rng.Intn(len(users))
		u := users[ui]
		coffee := rng.Intn(2) + 1
		pub := rng.Intn(2) == 0
		excl := rng.Intn(2) == 0
		body := fmt.Sprintf(`{"coffeeId":%d,"method":"V60","ratio":16,"rating":%d,"isPublic":%t,"excludeFromGlobal":%t}`, coffee, rng.Intn(10)+1, pub, excl)
		switch op := rng.Intn(7); {
		case op <= 2:
			var b Brew
			u.do("POST", "/brews", body, &b)
			brewIDs[ui] = append(brewIDs[ui], b.ID)
		case op == 3 && len(brewIDs[ui]) > 0: // edit, possibly moving coffees
			id := brewIDs[ui][rng.Intn(len(brewIDs[ui]))]
			u.do("PUT", fmt.Sprintf("/brews/%d", id), body, nil)
		case op == 4 && len(brewIDs[ui]) > 0:
			i := rng.Intn(len(brewIDs[ui]))
			u.do("DELETE", fmt.Sprintf("/brews/%d", brewIDs[ui][i]), "", nil)
			brewIDs[ui] = append(brewIDs[ui][:i], brewIDs[ui][i+1:]...)
		case op == 5:
			u.do("POST", fmt.Sprintf("/coffees/%d/open", coffee), "", nil)
			u.do("PUT", fmt.Sprintf("/coffees/%d/favorite", coffee), "", nil)
		default:
			u.do("DELETE", fmt.Sprintf("/coffees/%d/favorite", coffee), "", nil)
			u.do("DELETE", fmt.Sprintf("/coffees/%d/open", coffee), "", nil) // 409 if brews exist
		}
	}
	got := s.stats.all()
	want := groundTruth(t, s)
	// Also check a fresh load from disk agrees.
	fresh, err := loadStats(context.Background(), s.rdb)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		w := want[id]
		for name, g := range map[string]*Stats{"live": got[id], "reloaded": fresh.all()[id]} {
			if g == nil {
				g = &Stats{}
			}
			if g.RatingCount != w.RatingCount || g.BrewCount != w.BrewCount || g.Opens != w.Opens || g.Favorites != w.Favorites ||
				(g.AvgRating == nil) != (w.AvgRating == nil) || (g.AvgRating != nil && math.Abs(*g.AvgRating-*w.AvgRating) > 1e-9) {
				t.Errorf("coffee %d %s: got %+v avg=%v, want %+v avg=%v", id, name, *g, deref(g.AvgRating), w, deref(w.AvgRating))
			}
		}
		if w.BrewCount == 0 {
			t.Errorf("coffee %d: test produced no included brews", id)
		}
	}
}

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}
