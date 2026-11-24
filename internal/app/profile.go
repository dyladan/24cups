package app

import (
	"encoding/json"
	"math"
	"net/http"
)

// ---- shared recipe fields ----

// RecipeFields is shared by presets and brews. Ratio is grams of water per
// gram of coffee (the X in 1:X). For espresso it's beverage out per gram in.
type RecipeFields struct {
	Method      string   `json:"method"`
	DoseG       *float64 `json:"doseG"`
	Ratio       float64  `json:"ratio"`
	TempC       *float64 `json:"tempC"`
	TargetTimeS *int64   `json:"targetTimeS"`
}

func (f *RecipeFields) validate() error {
	var err error
	if f.Method, err = trimMax("equipment", f.Method, 60); err != nil {
		return err
	}
	if f.Method == "" {
		return errf(400, "equipment is required")
	}
	if math.IsNaN(f.Ratio) || f.Ratio < 0.5 || f.Ratio > 100 {
		return errf(400, "ratio must be between 1:0.5 and 1:100")
	}
	if f.DoseG != nil && (*f.DoseG <= 0 || *f.DoseG > 5000) {
		return errf(400, "dose must be between 0 and 5000 g")
	}
	if f.TempC != nil && (*f.TempC < 0 || *f.TempC > 100) {
		return errf(400, "temperature must be between 0 and 100 °C")
	}
	if f.TargetTimeS != nil && (*f.TargetTimeS < 0 || *f.TargetTimeS > 86400*2) {
		return errf(400, "target time is out of range")
	}
	return nil
}

// ---- recipes ----

type Recipe struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	RecipeFields
	Notes string `json:"notes"`
}

func (s *Server) listRecipes(w http.ResponseWriter, r *http.Request, u *User) error {
	rows, err := s.rdb.QueryContext(r.Context(),
		`SELECT id, name, method, dose_g, ratio, temp_c, target_time_s, notes FROM recipes WHERE user_id = ? ORDER BY name COLLATE NOCASE`, u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []Recipe{}
	for rows.Next() {
		var x Recipe
		if err := rows.Scan(&x.ID, &x.Name, &x.Method, &x.DoseG, &x.Ratio, &x.TempC, &x.TargetTimeS, &x.Notes); err != nil {
			return err
		}
		list = append(list, x)
	}
	writeJSON(w, 200, list)
	return rows.Err()
}

func (s *Server) readRecipe(w http.ResponseWriter, r *http.Request) (*Recipe, error) {
	var in Recipe
	if err := readJSON(w, r, &in); err != nil {
		return nil, err
	}
	var err error
	if in.Name, err = trimMax("name", in.Name, 80); err != nil {
		return nil, err
	}
	if in.Name == "" {
		return nil, errf(400, "name is required")
	}
	if in.Notes, err = trimMax("notes", in.Notes, 2000); err != nil {
		return nil, err
	}
	if err := in.RecipeFields.validate(); err != nil {
		return nil, err
	}
	return &in, nil
}

func (s *Server) createRecipe(w http.ResponseWriter, r *http.Request, u *User) error {
	in, err := s.readRecipe(w, r)
	if err != nil {
		return err
	}
	if err := s.capCheck(r, `SELECT COUNT(*) FROM recipes WHERE user_id = ?`, s.guards.cfg.MaxRecipes, "You can save up to %d recipe presets.", u.ID); err != nil {
		return err
	}
	res, err := s.db.ExecContext(r.Context(), `
		INSERT INTO recipes(user_id, name, method, dose_g, ratio, temp_c, target_time_s, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, in.Name, in.Method, in.DoseG, in.Ratio, in.TempC, in.TargetTimeS, in.Notes, now())
	if err != nil {
		return err
	}
	in.ID, _ = res.LastInsertId()
	s.notifyUser(r, u, keyRecipes)
	writeJSON(w, 201, in)
	return nil
}

func (s *Server) updateRecipe(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	in, err := s.readRecipe(w, r)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(r.Context(), `
		UPDATE recipes SET name=?, method=?, dose_g=?, ratio=?, temp_c=?, target_time_s=?, notes=?
		WHERE id = ? AND user_id = ?`,
		in.Name, in.Method, in.DoseG, in.Ratio, in.TempC, in.TargetTimeS, in.Notes, id, u.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	in.ID = id
	s.notifyUser(r, u, keyRecipes)
	writeJSON(w, 200, in)
	return nil
}

func (s *Server) deleteRecipe(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM recipes WHERE id = ? AND user_id = ?`, id, u.ID); err != nil {
		return err
	}
	s.notifyUser(r, u, keyRecipes)
	return noContent(w)
}

// ---- grinders ----

type Grinder struct {
	ID       int64    `json:"id"`
	Model    string   `json:"model"`
	Nickname string   `json:"nickname"`
	Burrsets []string `json:"burrsets"`
	Notes    string   `json:"notes"`
}

func (g *Grinder) Label() string {
	if g.Nickname != "" {
		return g.Nickname + " (" + g.Model + ")"
	}
	return g.Model
}

func (s *Server) getGrinder(r *http.Request, userID, id int64) (*Grinder, error) {
	g := &Grinder{}
	var burrs string
	err := s.rdb.QueryRowContext(r.Context(),
		`SELECT id, model, nickname, burrsets, notes FROM grinders WHERE id = ? AND user_id = ?`, id, userID,
	).Scan(&g.ID, &g.Model, &g.Nickname, &burrs, &g.Notes)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(burrs), &g.Burrsets)
	if g.Burrsets == nil {
		g.Burrsets = []string{}
	}
	return g, nil
}

func (s *Server) listGrinders(w http.ResponseWriter, r *http.Request, u *User) error {
	rows, err := s.rdb.QueryContext(r.Context(),
		`SELECT id, model, nickname, burrsets, notes FROM grinders WHERE user_id = ? ORDER BY id`, u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []Grinder{}
	for rows.Next() {
		var g Grinder
		var burrs string
		if err := rows.Scan(&g.ID, &g.Model, &g.Nickname, &burrs, &g.Notes); err != nil {
			return err
		}
		json.Unmarshal([]byte(burrs), &g.Burrsets)
		if g.Burrsets == nil {
			g.Burrsets = []string{}
		}
		list = append(list, g)
	}
	writeJSON(w, 200, list)
	return rows.Err()
}

func (s *Server) readGrinder(w http.ResponseWriter, r *http.Request) (*Grinder, string, error) {
	var in Grinder
	if err := readJSON(w, r, &in); err != nil {
		return nil, "", err
	}
	var err error
	if in.Model, err = trimMax("grinder", in.Model, 80); err != nil {
		return nil, "", err
	}
	if in.Model == "" {
		return nil, "", errf(400, "grinder model is required")
	}
	if in.Nickname, err = trimMax("nickname", in.Nickname, 50); err != nil {
		return nil, "", err
	}
	if in.Notes, err = trimMax("notes", in.Notes, 1000); err != nil {
		return nil, "", err
	}
	if len(in.Burrsets) > 20 {
		return nil, "", errf(400, "too many burrsets")
	}
	clean := []string{}
	seen := map[string]bool{}
	for _, b := range in.Burrsets {
		b, err := trimMax("burrset", b, 80)
		if err != nil {
			return nil, "", err
		}
		if b != "" && !seen[b] {
			seen[b] = true
			clean = append(clean, b)
		}
	}
	in.Burrsets = clean
	enc, _ := json.Marshal(clean)
	return &in, string(enc), nil
}

func (s *Server) createGrinder(w http.ResponseWriter, r *http.Request, u *User) error {
	in, burrs, err := s.readGrinder(w, r)
	if err != nil {
		return err
	}
	if err := s.capCheck(r, `SELECT COUNT(*) FROM grinders WHERE user_id = ?`, s.guards.cfg.MaxGrinders, "You can add up to %d grinders.", u.ID); err != nil {
		return err
	}
	res, err := s.db.ExecContext(r.Context(),
		`INSERT INTO grinders(user_id, model, nickname, burrsets, notes, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, in.Model, in.Nickname, burrs, in.Notes, now())
	if err != nil {
		return err
	}
	in.ID, _ = res.LastInsertId()
	s.notifyUser(r, u, keyGrinders)
	writeJSON(w, 201, in)
	return nil
}

func (s *Server) updateGrinder(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	in, burrs, err := s.readGrinder(w, r)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(r.Context(),
		`UPDATE grinders SET model=?, nickname=?, burrsets=?, notes=? WHERE id = ? AND user_id = ?`,
		in.Model, in.Nickname, burrs, in.Notes, id, u.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	in.ID = id
	s.notifyUser(r, u, keyGrinders)
	writeJSON(w, 200, in)
	return nil
}

func (s *Server) deleteGrinder(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM grinders WHERE id = ? AND user_id = ?`, id, u.ID); err != nil {
		return err
	}
	s.notifyUser(r, u, keyGrinders)
	return noContent(w)
}
