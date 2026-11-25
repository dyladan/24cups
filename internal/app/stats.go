package app

import (
	"context"
	"database/sql"
	"sync"
)

// statsStore keeps community aggregates in memory so page views don't scan
// every brew. It's loaded once at startup and then updated after each write
// by re-reading only the affected (user, coffee) pair from the database, so
// it can't drift from the source of truth.
type statsStore struct {
	mu      sync.RWMutex
	coffees map[int64]*coffeeAgg

	// refreshMu serializes read-then-apply so two refreshes of the same pair
	// can't land out of order.
	refreshMu sync.Mutex
}

// pairStat is one user's included ratings for one coffee.
type pairStat struct {
	n    int
	sum  int
	hist [10]int
}

type coffeeAgg struct {
	pairs map[int64]*pairStat // users with at least one included rating
	opens map[int64]struct{}
	favs  map[int64]struct{}
	// flavors holds each user's descriptor set (users with at least one
	// descriptor on an included brew).
	flavors map[int64]map[string]struct{}

	// derived from pairs
	avgSum    float64 // sum of per-user averages (1–10 scale)
	brewCount int
	hist      [10]int
}

func newCoffeeAgg() *coffeeAgg {
	return &coffeeAgg{pairs: map[int64]*pairStat{}, opens: map[int64]struct{}{}, favs: map[int64]struct{}{}, flavors: map[int64]map[string]struct{}{}}
}

func (a *coffeeAgg) recompute() {
	a.avgSum, a.brewCount, a.hist = 0, 0, [10]int{}
	for _, p := range a.pairs {
		a.avgSum += float64(p.sum) / float64(p.n)
		a.brewCount += p.n
		for i, c := range p.hist {
			a.hist[i] += c
		}
	}
}

func (s *statsStore) agg(coffeeID int64) *coffeeAgg {
	a := s.coffees[coffeeID]
	if a == nil {
		a = newCoffeeAgg()
		s.coffees[coffeeID] = a
	}
	return a
}

func loadStats(ctx context.Context, db *sql.DB) (*statsStore, error) {
	s := &statsStore{coffees: map[int64]*coffeeAgg{}}
	rows, err := db.QueryContext(ctx, `SELECT coffee_id, user_id, rating, COUNT(*) FROM brews WHERE `+includedRatings+` GROUP BY 1, 2, 3`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var cid, uid int64
		var rating, n int
		if err := rows.Scan(&cid, &uid, &rating, &n); err != nil {
			rows.Close()
			return nil, err
		}
		a := s.agg(cid)
		p := a.pairs[uid]
		if p == nil {
			p = &pairStat{}
			a.pairs[uid] = p
		}
		p.n += n
		p.sum += rating * n
		p.hist[rating-1] += n
	}
	rows.Close()
	for _, t := range []struct {
		table string
		set   func(*coffeeAgg) map[int64]struct{}
	}{
		{"coffee_opens", func(a *coffeeAgg) map[int64]struct{} { return a.opens }},
		{"favorites", func(a *coffeeAgg) map[int64]struct{} { return a.favs }},
	} {
		rows, err := db.QueryContext(ctx, `SELECT coffee_id, user_id FROM `+t.table)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var cid, uid int64
			if err := rows.Scan(&cid, &uid); err != nil {
				rows.Close()
				return nil, err
			}
			t.set(s.agg(cid))[uid] = struct{}{}
		}
		rows.Close()
	}
	rows, err = db.QueryContext(ctx, `
		SELECT DISTINCT b.coffee_id, b.user_id, d.descriptor
		FROM brew_descriptors d JOIN brews b ON b.id = d.brew_id
		WHERE `+includedFlavors)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var cid, uid int64
		var d string
		if err := rows.Scan(&cid, &uid, &d); err != nil {
			rows.Close()
			return nil, err
		}
		a := s.agg(cid)
		set := a.flavors[uid]
		if set == nil {
			set = map[string]struct{}{}
			a.flavors[uid] = set
		}
		set[d] = struct{}{}
	}
	rows.Close()
	for _, a := range s.coffees {
		a.recompute()
	}
	return s, nil
}

// refresh re-reads one (user, coffee) pair after a write.
func (s *statsStore) refresh(ctx context.Context, db *sql.DB, userID, coffeeID int64) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	p := &pairStat{}
	rows, err := db.QueryContext(ctx, `SELECT rating, COUNT(*) FROM brews WHERE user_id = ? AND coffee_id = ? AND `+includedRatings+` GROUP BY rating`, userID, coffeeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var rating, n int
		if err := rows.Scan(&rating, &n); err != nil {
			rows.Close()
			return err
		}
		p.n += n
		p.sum += rating * n
		p.hist[rating-1] += n
	}
	rows.Close()
	flavors := map[string]struct{}{}
	rows, err = db.QueryContext(ctx, `
		SELECT DISTINCT d.descriptor FROM brew_descriptors d JOIN brews b ON b.id = d.brew_id
		WHERE b.user_id = ? AND b.coffee_id = ? AND `+includedFlavors, userID, coffeeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			rows.Close()
			return err
		}
		flavors[d] = struct{}{}
	}
	rows.Close()
	var opened, fav bool
	if err := db.QueryRowContext(ctx, `SELECT
			EXISTS(SELECT 1 FROM coffee_opens WHERE user_id = ? AND coffee_id = ?),
			EXISTS(SELECT 1 FROM favorites WHERE user_id = ? AND coffee_id = ?)`,
		userID, coffeeID, userID, coffeeID).Scan(&opened, &fav); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.agg(coffeeID)
	if p.n > 0 {
		a.pairs[userID] = p
	} else {
		delete(a.pairs, userID)
	}
	if len(flavors) > 0 {
		a.flavors[userID] = flavors
	} else {
		delete(a.flavors, userID)
	}
	setMember(a.opens, userID, opened)
	setMember(a.favs, userID, fav)
	a.recompute()
	return nil
}

func setMember(m map[int64]struct{}, id int64, in bool) {
	if in {
		m[id] = struct{}{}
	} else {
		delete(m, id)
	}
}

func (a *coffeeAgg) stats() *Stats {
	st := &Stats{RatingCount: len(a.pairs), BrewCount: a.brewCount, Opens: len(a.opens), Favorites: len(a.favs)}
	if len(a.pairs) > 0 {
		avg := a.avgSum / float64(len(a.pairs)) / 2
		st.AvgRating = &avg
	}
	return st
}

// all returns stats for every coffee with any activity.
func (s *statsStore) all() map[int64]*Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[int64]*Stats, len(s.coffees))
	for id, a := range s.coffees {
		out[id] = a.stats()
	}
	return out
}

func (s *statsStore) distribution(coffeeID int64) []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d := make([]int, 10)
	if a := s.coffees[coffeeID]; a != nil {
		copy(d, a.hist[:])
	}
	return d
}

// forUsers computes stats counting only the given users (a group).
func (s *statsStore) forUsers(users []int64) map[int64]*Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[int64]*Stats{}
	for id, a := range s.coffees {
		st := &Stats{}
		var avgSum float64
		for _, u := range users {
			if p := a.pairs[u]; p != nil {
				st.RatingCount++
				st.BrewCount += p.n
				avgSum += float64(p.sum) / float64(p.n)
			}
			if _, ok := a.opens[u]; ok {
				st.Opens++
			}
			if _, ok := a.favs[u]; ok {
				st.Favorites++
			}
		}
		if st.RatingCount > 0 {
			avg := avgSum / float64(st.RatingCount) / 2
			st.AvgRating = &avg
		}
		out[id] = st
	}
	return out
}
