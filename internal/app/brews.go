package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type Brew struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Mine        bool   `json:"mine"`

	CoffeeID   int64   `json:"coffeeId"`
	CoffeeDay  int     `json:"coffeeDay"`
	CoffeeName *string `json:"coffeeName"` // nil if the viewer hasn't opened it

	RecipeID *int64 `json:"recipeId"`
	RecipeFields
	ActualTimeS *int64 `json:"actualTimeS"`

	GrinderID    *int64 `json:"grinderId"`
	GrinderLabel string `json:"grinderLabel"`
	Burrset      string `json:"burrset"`
	GrindSetting string `json:"grindSetting"`

	Rating            *int   `json:"rating"` // 1–10 half-stars
	Notes             string `json:"notes"`
	IsPublic          bool   `json:"isPublic"`
	ExcludeFromGlobal bool   `json:"excludeFromGlobal"`
	BrewedAt          string `json:"brewedAt"`
}

// queryBrews returns brews matching where (which must already restrict
// visibility). Coffee names are hidden for coffees the viewer hasn't opened.
func (s *Server) queryBrews(ctx context.Context, viewer *User, where string, limit int, args ...any) ([]*Brew, error) {
	q := `
		SELECT b.id, b.user_id, u.username, u.display_name, b.coffee_id, c.day,
		       CASE WHEN o.user_id IS NULL THEN NULL ELSE c.name END,
		       b.recipe_id, b.method, b.dose_g, b.ratio, b.temp_c, b.target_time_s, b.actual_time_s,
		       b.grinder_id, b.grinder_label, b.burrset, b.grind_setting,
		       b.rating, b.notes, b.is_public, b.exclude_from_global, b.brewed_at
		FROM brews b
		JOIN users u ON u.id = b.user_id
		JOIN coffees c ON c.id = b.coffee_id
		LEFT JOIN coffee_opens o ON o.coffee_id = b.coffee_id AND o.user_id = ?
		WHERE ` + where + `
		ORDER BY b.brewed_at DESC, b.id DESC
		LIMIT ` + strconv.Itoa(limit)
	rows, err := s.db.QueryContext(ctx, q, append([]any{viewer.ID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []*Brew{}
	for rows.Next() {
		b := &Brew{}
		if err := rows.Scan(&b.ID, &b.UserID, &b.Username, &b.DisplayName, &b.CoffeeID, &b.CoffeeDay, &b.CoffeeName,
			&b.RecipeID, &b.Method, &b.DoseG, &b.Ratio, &b.TempC, &b.TargetTimeS, &b.ActualTimeS,
			&b.GrinderID, &b.GrinderLabel, &b.Burrset, &b.GrindSetting,
			&b.Rating, &b.Notes, &b.IsPublic, &b.ExcludeFromGlobal, &b.BrewedAt); err != nil {
			return nil, err
		}
		b.Mine = b.UserID == viewer.ID
		if !b.Mine {
			// Other users' recipe/grinder ids are meaningless to the viewer.
			b.RecipeID, b.GrinderID = nil, nil
			b.ExcludeFromGlobal = false
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (s *Server) listMyBrews(w http.ResponseWriter, r *http.Request, u *User) error {
	list, err := s.queryBrews(r.Context(), u, `b.user_id = ?`, 1000, u.ID)
	if err != nil {
		return err
	}
	writeJSON(w, 200, list)
	return nil
}

func (s *Server) getBrew(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	list, err := s.queryBrews(r.Context(), u, `b.id = ? AND (b.user_id = ? OR (b.is_public = 1 AND o.user_id IS NOT NULL))`, 1, id, u.ID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return errNotFound
	}
	writeJSON(w, 200, list[0])
	return nil
}

type brewInput struct {
	CoffeeID int64  `json:"coffeeId"`
	RecipeID *int64 `json:"recipeId"`
	RecipeFields
	ActualTimeS       *int64  `json:"actualTimeS"`
	GrinderID         *int64  `json:"grinderId"`
	Burrset           string  `json:"burrset"`
	GrindSetting      string  `json:"grindSetting"`
	Rating            *int    `json:"rating"`
	Notes             string  `json:"notes"`
	IsPublic          bool    `json:"isPublic"`
	ExcludeFromGlobal bool    `json:"excludeFromGlobal"`
	BrewedAt          *string `json:"brewedAt"`
}

func (s *Server) readBrew(w http.ResponseWriter, r *http.Request, u *User) (*brewInput, string, error) {
	var in brewInput
	if err := readJSON(w, r, &in); err != nil {
		return nil, "", err
	}
	if err := in.RecipeFields.validate(); err != nil {
		return nil, "", err
	}
	if err := s.coffeeExists(r.Context(), in.CoffeeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", errf(400, "pick a coffee")
		}
		return nil, "", err
	}
	if in.ActualTimeS != nil && (*in.ActualTimeS < 0 || *in.ActualTimeS > 86400*2) {
		return nil, "", errf(400, "brew time is out of range")
	}
	if in.Rating != nil && (*in.Rating < 1 || *in.Rating > 10) {
		return nil, "", errf(400, "rating must be between ½ and 5 stars")
	}
	var err error
	if in.GrindSetting, err = trimMax("grind setting", in.GrindSetting, 40); err != nil {
		return nil, "", err
	}
	if in.Burrset, err = trimMax("burrset", in.Burrset, 80); err != nil {
		return nil, "", err
	}
	if in.Notes, err = trimMax("notes", in.Notes, 4000); err != nil {
		return nil, "", err
	}
	if in.IsPublic {
		in.ExcludeFromGlobal = false
	}
	if in.RecipeID != nil {
		var x int
		if err := s.db.QueryRowContext(r.Context(), `SELECT 1 FROM recipes WHERE id = ? AND user_id = ?`, *in.RecipeID, u.ID).Scan(&x); err != nil {
			in.RecipeID = nil
		}
	}
	label := ""
	if in.GrinderID != nil {
		g, err := s.getGrinder(r, u.ID, *in.GrinderID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, "", errf(400, "unknown grinder")
			}
			return nil, "", err
		}
		label = g.Label()
	}
	if in.BrewedAt != nil {
		t, err := time.Parse(time.RFC3339, *in.BrewedAt)
		if err != nil {
			return nil, "", errf(400, "brewedAt must be an RFC 3339 timestamp")
		}
		v := t.UTC().Format(time.RFC3339)
		in.BrewedAt = &v
	} else {
		v := now()
		in.BrewedAt = &v
	}
	return &in, label, nil
}

func (s *Server) createBrew(w http.ResponseWriter, r *http.Request, u *User) error {
	in, label, err := s.readBrew(w, r, u)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Logging a brew implies the coffee has been opened.
	if _, err := tx.Exec(`INSERT OR IGNORE INTO coffee_opens(user_id, coffee_id, opened_at) VALUES (?, ?, ?)`, u.ID, in.CoffeeID, now()); err != nil {
		return err
	}
	res, err := tx.Exec(`
		INSERT INTO brews(user_id, coffee_id, recipe_id, method, dose_g, ratio, temp_c, target_time_s, actual_time_s,
		                  grinder_id, grinder_label, burrset, grind_setting, rating, notes, is_public, exclude_from_global, brewed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, in.CoffeeID, in.RecipeID, in.Method, in.DoseG, in.Ratio, in.TempC, in.TargetTimeS, in.ActualTimeS,
		in.GrinderID, label, in.Burrset, in.GrindSetting, in.Rating, in.Notes, boolInt(in.IsPublic), boolInt(in.ExcludeFromGlobal), *in.BrewedAt, now())
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	s.brewChanged(r, u)
	list, err := s.queryBrews(r.Context(), u, `b.id = ?`, 1, id)
	if err != nil {
		return err
	}
	writeJSON(w, 201, list[0])
	return nil
}

func (s *Server) updateBrew(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	in, label, err := s.readBrew(w, r, u)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`
		UPDATE brews SET coffee_id=?, recipe_id=?, method=?, dose_g=?, ratio=?, temp_c=?, target_time_s=?, actual_time_s=?,
		       grinder_id=?, grinder_label=CASE WHEN ? IS NULL THEN grinder_label ELSE ? END, burrset=?, grind_setting=?,
		       rating=?, notes=?, is_public=?, exclude_from_global=?, brewed_at=?
		WHERE id = ? AND user_id = ?`,
		in.CoffeeID, in.RecipeID, in.Method, in.DoseG, in.Ratio, in.TempC, in.TargetTimeS, in.ActualTimeS,
		in.GrinderID, in.GrinderID, label, in.Burrset, in.GrindSetting,
		in.Rating, in.Notes, boolInt(in.IsPublic), boolInt(in.ExcludeFromGlobal), *in.BrewedAt, id, u.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO coffee_opens(user_id, coffee_id, opened_at) VALUES (?, ?, ?)`, u.ID, in.CoffeeID, now()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.brewChanged(r, u)
	list, err := s.queryBrews(r.Context(), u, `b.id = ?`, 1, id)
	if err != nil {
		return err
	}
	writeJSON(w, 200, list[0])
	return nil
}

func (s *Server) deleteBrew(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM brews WHERE id = ? AND user_id = ?`, id, u.ID); err != nil {
		return err
	}
	s.brewChanged(r, u)
	return noContent(w)
}

// brewChanged covers stats, coffee brew lists, group feeds and the owner's
// own brew list. Private brews still move averages unless excluded, so
// everyone is told; they refetch only what they're allowed to see.
func (s *Server) brewChanged(r *http.Request, u *User) {
	s.notify(r, keyCoffees, keyGroups)
	s.notifyUser(r, u, keyBrews)
}
