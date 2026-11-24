package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
)

type CoffeeInfo struct {
	Name         string `json:"name"`
	Origin       string `json:"origin"`
	Region       string `json:"region"`
	Producer     string `json:"producer"`
	Process      string `json:"process"`
	Variety      string `json:"variety"`
	Elevation    string `json:"elevation"`
	RoastLevel   string `json:"roastLevel"`
	TastingNotes string `json:"tastingNotes"`
	Description  string `json:"description"`
}

type Stats struct {
	// AvgRating is on a 0–5 star scale. Each user's included brews are
	// averaged first so a single prolific brewer can't dominate.
	AvgRating   *float64 `json:"avgRating"`
	RatingCount int      `json:"ratingCount"` // users who rated
	BrewCount   int      `json:"brewCount"`   // rated brews counted
	Opens       int      `json:"opens"`
	Favorites   int      `json:"favorites"`
}

type Coffee struct {
	ID        int64       `json:"id"`
	Day       int         `json:"day"`
	Opened    bool        `json:"opened"`
	OpenedAt  *string     `json:"openedAt"`
	Favorited bool        `json:"favorited"`
	Info      *CoffeeInfo `json:"info"`  // nil until opened (no spoilers)
	Stats     *Stats      `json:"stats"` // nil until opened unless spoilerMode is "always"
	// Revealed is false while tasting notes and community scores are hidden
	// (opened but not yet rated, in the default "rated" spoiler mode). Info
	// then has no tasting notes and Stats carries only Opens.
	Revealed    bool     `json:"revealed"`
	MyAvg       *float64 `json:"myAvg"` // 0–5 scale
	MyBrewCount int      `json:"myBrewCount"`
}

type CoffeeDetail struct {
	Coffee
	Distribution []int        `json:"distribution,omitempty"` // index 0 = ½★ … 9 = 5★
	GroupStats   []GroupStats `json:"groupStats,omitempty"`
}

type GroupStats struct {
	GroupID   int64  `json:"groupId"`
	GroupName string `json:"groupName"`
	Stats
}

// ---- seeding ----

type seedCoffee struct {
	Day int `json:"day"`
	CoffeeInfo
}

// SeedCoffees upserts the calendar contents keyed by day.
func SeedCoffees(db *sql.DB, data []byte) error {
	var list []seedCoffee
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parse coffees: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range list {
		if c.Day <= 0 || c.Name == "" {
			return fmt.Errorf("coffee entries need a positive day and a name (got day %d)", c.Day)
		}
		_, err := tx.Exec(`
			INSERT INTO coffees(day, name, origin, region, producer, process, variety, elevation, roast_level, tasting_notes, description)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(day) DO UPDATE SET
				name=excluded.name, origin=excluded.origin, region=excluded.region, producer=excluded.producer,
				process=excluded.process, variety=excluded.variety, elevation=excluded.elevation,
				roast_level=excluded.roast_level, tasting_notes=excluded.tasting_notes, description=excluded.description`,
			c.Day, c.Name, c.Origin, c.Region, c.Producer, c.Process, c.Variety, c.Elevation, c.RoastLevel, c.TastingNotes, c.Description)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- stats ----

// includedRatings selects ratings that count toward aggregate scores: all
// public brews, plus private brews the owner didn't exclude.
const includedRatings = `rating IS NOT NULL AND (is_public = 1 OR exclude_from_global = 0)`

// computeStats returns stats per coffee id from the in-memory store. If
// memberOf > 0 only members of that group are counted.
func (s *Server) computeStats(ctx context.Context, coffeeID, memberOf int64) (map[int64]*Stats, error) {
	if memberOf <= 0 {
		return s.stats.all(), nil
	}
	rows, err := s.rdb.QueryContext(ctx, `SELECT user_id FROM group_members WHERE group_id = ?`, memberOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		users = append(users, id)
	}
	return s.stats.forUsers(users), rows.Err()
}

// ---- handlers ----

func (s *Server) loadCoffees(ctx context.Context, u *User, onlyID int64) ([]*Coffee, error) {
	q := `
		SELECT c.id, c.day, c.name, c.origin, c.region, c.producer, c.process, c.variety, c.elevation,
		       c.roast_level, c.tasting_notes, c.description,
		       o.opened_at, COALESCE(o.revealed, 0), f.user_id IS NOT NULL,
		       (SELECT AVG(rating) FROM brews b WHERE b.coffee_id = c.id AND b.user_id = ? AND b.rating IS NOT NULL),
		       (SELECT COUNT(*) FROM brews b WHERE b.coffee_id = c.id AND b.user_id = ?)
		FROM coffees c
		LEFT JOIN coffee_opens o ON o.coffee_id = c.id AND o.user_id = ?
		LEFT JOIN favorites f ON f.coffee_id = c.id AND f.user_id = ?`
	args := []any{u.ID, u.ID, u.ID, u.ID}
	if onlyID > 0 {
		q += ` WHERE c.id = ?`
		args = append(args, onlyID)
	}
	q += ` ORDER BY c.day`
	// Compute stats first: the pool has a single connection, so no other
	// query may run while rows is open.
	stats, err := s.computeStats(ctx, onlyID, 0)
	if err != nil {
		return nil, err
	}
	rows, err := s.rdb.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Coffee
	for rows.Next() {
		c := &Coffee{}
		info := &CoffeeInfo{}
		var myAvg *float64
		var manual bool
		if err := rows.Scan(&c.ID, &c.Day, &info.Name, &info.Origin, &info.Region, &info.Producer, &info.Process,
			&info.Variety, &info.Elevation, &info.RoastLevel, &info.TastingNotes, &info.Description,
			&c.OpenedAt, &manual, &c.Favorited, &myAvg, &c.MyBrewCount); err != nil {
			return nil, err
		}
		c.Opened = c.OpenedAt != nil
		if u.anon {
			c.Opened, c.Favorited = u.anonCanSee(c.Day), false
		}
		if myAvg != nil {
			v := *myAvg / 2
			c.MyAvg = &v
		}
		// myAvg is non-nil exactly when the user has a rated brew.
		c.Revealed = u.isRevealed(c.Opened, myAvg != nil, manual)
		if u.anon {
			c.Revealed = c.Opened
		}
		st := stats[c.ID]
		if st == nil {
			st = &Stats{}
		}
		switch {
		case c.Revealed:
			c.Info, c.Stats = info, st
		case c.Opened:
			// Signed-in tasters go in blind; for signed-out visitors only
			// the ratings are spoilers.
			if !u.anon {
				info.TastingNotes, info.Description = "", ""
			}
			c.Info, c.Stats = info, &Stats{Opens: st.Opens}
		case u.statsOnSealedDoors():
			c.Stats = st
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Server) listCoffees(w http.ResponseWriter, r *http.Request, u *User) error {
	list, err := s.loadCoffees(r.Context(), u, 0)
	if err != nil {
		return err
	}
	if list == nil {
		list = []*Coffee{}
	}
	writeJSON(w, 200, list)
	return nil
}

func (s *Server) getCoffee(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	list, err := s.loadCoffees(r.Context(), u, id)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return errNotFound
	}
	d := &CoffeeDetail{Coffee: *list[0]}
	if d.Revealed || (!d.Opened && d.Stats != nil) {
		d.Distribution = s.stats.distribution(id)

		groups, err := s.rdb.QueryContext(r.Context(), `
			SELECT g.id, g.name FROM groups g JOIN group_members m ON m.group_id = g.id
			WHERE m.user_id = ? ORDER BY g.name`, u.ID)
		if err != nil {
			return err
		}
		type gref struct {
			id   int64
			name string
		}
		var refs []gref
		for groups.Next() {
			var g gref
			if err := groups.Scan(&g.id, &g.name); err != nil {
				groups.Close()
				return err
			}
			refs = append(refs, g)
		}
		groups.Close()
		for _, g := range refs {
			st, err := s.computeStats(r.Context(), id, g.id)
			if err != nil {
				return err
			}
			gs := GroupStats{GroupID: g.id, GroupName: g.name}
			if v := st[id]; v != nil {
				gs.Stats = *v
			}
			d.GroupStats = append(d.GroupStats, gs)
		}
	}
	writeJSON(w, 200, d)
	return nil
}

func (s *Server) coffeeExists(ctx context.Context, id int64) error {
	var x int
	return s.rdb.QueryRowContext(ctx, `SELECT 1 FROM coffees WHERE id = ?`, id).Scan(&x)
}

func (s *Server) openCoffee(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.coffeeExists(r.Context(), id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(),
		`INSERT OR IGNORE INTO coffee_opens(user_id, coffee_id, opened_at) VALUES (?, ?, ?)`, u.ID, id, now()); err != nil {
		return err
	}
	s.refreshStats(r, u.ID, id)
	s.notify(r, keyCoffees, keyGroups)
	return noContent(w)
}

func (s *Server) unopenCoffee(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var n int
	s.rdb.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM brews WHERE user_id = ? AND coffee_id = ?`, u.ID, id).Scan(&n)
	if n > 0 {
		return errf(409, "you have brews logged for this coffee; delete them first")
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM coffee_opens WHERE user_id = ? AND coffee_id = ?`, u.ID, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM favorites WHERE user_id = ? AND coffee_id = ?`, u.ID, id); err != nil {
		return err
	}
	s.refreshStats(r, u.ID, id)
	s.notify(r, keyCoffees, keyGroups)
	return noContent(w)
}

func (s *Server) favoriteCoffee(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.coffeeExists(r.Context(), id); err != nil {
		return err
	}
	if !s.hasOpened(r.Context(), u.ID, id) {
		return errf(409, "open this coffee before favoriting it")
	}
	if _, err := s.db.ExecContext(r.Context(),
		`INSERT OR IGNORE INTO favorites(user_id, coffee_id, created_at) VALUES (?, ?, ?)`, u.ID, id, now()); err != nil {
		return err
	}
	s.refreshStats(r, u.ID, id)
	s.notify(r, keyCoffees, keyGroups)
	return noContent(w)
}

func (s *Server) unfavoriteCoffee(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM favorites WHERE user_id = ? AND coffee_id = ?`, u.ID, id); err != nil {
		return err
	}
	s.refreshStats(r, u.ID, id)
	s.notify(r, keyCoffees, keyGroups)
	return noContent(w)
}

// peekCoffee returns only a coffee's name and origin, so a sealed door can
// be peeked at without opening it (no notes, scores or brews, and it isn't
// recorded). Available to signed-out visitors too.
func (s *Server) peekCoffee(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var out struct {
		Name   string `json:"name"`
		Origin string `json:"origin"`
	}
	if err := s.rdb.QueryRowContext(r.Context(), `SELECT name, origin FROM coffees WHERE id = ?`, id).Scan(&out.Name, &out.Origin); err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	writeJSON(w, 200, out)
	return nil
}

// revealCoffee shows tasting notes and community scores for an opened
// coffee before the user has rated it.
func (s *Server) revealCoffee(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE coffee_opens SET revealed = 1 WHERE user_id = ? AND coffee_id = ?`, u.ID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errf(409, "open this coffee first")
	}
	s.notifyUser(r, u, keyCoffees, keyGroups)
	return noContent(w)
}

func (s *Server) hasOpened(ctx context.Context, userID, coffeeID int64) bool {
	var x int
	return s.rdb.QueryRowContext(ctx, `SELECT 1 FROM coffee_opens WHERE user_id = ? AND coffee_id = ?`, userID, coffeeID).Scan(&x) == nil
}

func (s *Server) coffeeBrews(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if u.anon {
		var day int
		if err := s.rdb.QueryRowContext(r.Context(), `SELECT day FROM coffees WHERE id = ?`, id).Scan(&day); err != nil {
			return err
		}
		if !u.anonCanSee(day) {
			writeJSON(w, 200, []*Brew{})
			return nil
		}
		list, err := s.queryBrews(r.Context(), u, `b.coffee_id = ? AND b.is_public = 1`, 100, id)
		if err != nil {
			return err
		}
		writeJSON(w, 200, list)
		return nil
	}
	if !s.hasOpened(r.Context(), u.ID, id) {
		writeJSON(w, 200, []*Brew{})
		return nil
	}
	list, err := s.queryBrews(r.Context(), u, `b.coffee_id = ? AND (b.user_id = ? OR (b.is_public = 1 AND `+u.revealedSQL()+`))`, 100, id, u.ID)
	if err != nil {
		return err
	}
	writeJSON(w, 200, list)
	return nil
}
