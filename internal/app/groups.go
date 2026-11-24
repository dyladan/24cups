package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

type Group struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"isPublic"`
	OwnerID     int64  `json:"ownerId"`
	OwnerName   string `json:"ownerName"`
	MemberCount int    `json:"memberCount"`
	Role        string `json:"role"`                 // "", "member", "owner"
	InviteCode  string `json:"inviteCode,omitempty"` // members only
	CreatedAt   string `json:"createdAt"`
}

type GroupMember struct {
	UserID      int64  `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	JoinedAt    string `json:"joinedAt"`
	Opens       int    `json:"opens"`
	Brews       int    `json:"brews"`
}

type GroupCoffee struct {
	CoffeeID int64   `json:"coffeeId"`
	Day      int     `json:"day"`
	Name     *string `json:"name"`
	Stats    *Stats  `json:"stats"`
}

type GroupDetail struct {
	Group
	Members []GroupMember `json:"members"`
	Coffees []GroupCoffee `json:"coffees"`
	Brews   []*Brew       `json:"brews"`
}

const groupSelect = `
	SELECT g.id, g.name, g.description, g.is_public, g.owner_id, ou.display_name,
	       (SELECT COUNT(*) FROM group_members WHERE group_id = g.id),
	       COALESCE(m.role, ''), g.invite_code, g.created_at
	FROM groups g
	JOIN users ou ON ou.id = g.owner_id
	LEFT JOIN group_members m ON m.group_id = g.id AND m.user_id = ?`

func scanGroups(rows *sql.Rows) ([]Group, error) {
	defer rows.Close()
	list := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.IsPublic, &g.OwnerID, &g.OwnerName,
			&g.MemberCount, &g.Role, &g.InviteCode, &g.CreatedAt); err != nil {
			return nil, err
		}
		if g.Role == "" {
			g.InviteCode = ""
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

func (s *Server) loadGroup(ctx context.Context, u *User, id int64) (*Group, error) {
	rows, err := s.rdb.QueryContext(ctx, groupSelect+` WHERE g.id = ?`, u.ID, id)
	if err != nil {
		return nil, err
	}
	list, err := scanGroups(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 || (!list[0].IsPublic && list[0].Role == "") {
		return nil, errNotFound
	}
	return &list[0], nil
}

func (s *Server) myGroups(w http.ResponseWriter, r *http.Request, u *User) error {
	rows, err := s.rdb.QueryContext(r.Context(), groupSelect+` WHERE m.user_id IS NOT NULL ORDER BY g.name COLLATE NOCASE`, u.ID)
	if err != nil {
		return err
	}
	list, err := scanGroups(rows)
	if err != nil {
		return err
	}
	writeJSON(w, 200, list)
	return nil
}

func (s *Server) discoverGroups(w http.ResponseWriter, r *http.Request, u *User) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	where := ` WHERE g.is_public = 1`
	args := []any{u.ID}
	if q != "" {
		where += ` AND (g.name LIKE ? ESCAPE '\' OR g.description LIKE ? ESCAPE '\')`
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		args = append(args, "%"+esc+"%", "%"+esc+"%")
	}
	rows, err := s.rdb.QueryContext(r.Context(), groupSelect+where+
		` ORDER BY (SELECT COUNT(*) FROM group_members WHERE group_id = g.id) DESC, g.name LIMIT 50`, args...)
	if err != nil {
		return err
	}
	list, err := scanGroups(rows)
	if err != nil {
		return err
	}
	writeJSON(w, 200, list)
	return nil
}

type groupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"isPublic"`
}

func (in *groupInput) validate() error {
	var err error
	if in.Name, err = trimMax("name", in.Name, 60); err != nil {
		return err
	}
	if in.Name == "" {
		return errf(400, "group name is required")
	}
	in.Description, err = trimMax("description", in.Description, 500)
	return err
}

func inviteCode() string { return randomToken(9) } // 12 url-safe chars

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request, u *User) error {
	var in groupInput
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := now()
	res, err := tx.Exec(`INSERT INTO groups(name, description, is_public, invite_code, owner_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		in.Name, in.Description, boolInt(in.IsPublic), inviteCode(), u.ID, ts)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)`, id, u.ID, ts); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	g, err := s.loadGroup(r.Context(), u, id)
	if err != nil {
		return err
	}
	s.notify(r, keyGroups, keyCoffees)
	writeJSON(w, 201, g)
	return nil
}

func (s *Server) getGroup(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	g, err := s.loadGroup(r.Context(), u, id)
	if err != nil {
		return err
	}
	d := &GroupDetail{Group: *g, Members: []GroupMember{}, Coffees: []GroupCoffee{}}

	rows, err := s.rdb.QueryContext(r.Context(), `
		SELECT u.id, u.username, u.display_name, m.role, m.joined_at,
		       (SELECT COUNT(*) FROM coffee_opens o WHERE o.user_id = u.id),
		       (SELECT COUNT(*) FROM brews b WHERE b.user_id = u.id AND b.is_public = 1)
		FROM group_members m JOIN users u ON u.id = m.user_id
		WHERE m.group_id = ?
		ORDER BY m.role = 'owner' DESC, u.display_name COLLATE NOCASE`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.Role, &m.JoinedAt, &m.Opens, &m.Brews); err != nil {
			rows.Close()
			return err
		}
		d.Members = append(d.Members, m)
	}
	rows.Close()

	stats, err := s.computeStats(r.Context(), 0, id)
	if err != nil {
		return err
	}
	rows, err = s.rdb.QueryContext(r.Context(), `
		SELECT c.id, c.day, c.name, o.user_id IS NOT NULL
		FROM coffees c LEFT JOIN coffee_opens o ON o.coffee_id = c.id AND o.user_id = ?
		ORDER BY c.day`, u.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var gc GroupCoffee
		var name string
		var opened bool
		if err := rows.Scan(&gc.CoffeeID, &gc.Day, &name, &opened); err != nil {
			rows.Close()
			return err
		}
		if opened {
			gc.Name = &name
		}
		if opened || u.RevealUnopened {
			gc.Stats = stats[gc.CoffeeID]
			if gc.Stats == nil {
				gc.Stats = &Stats{}
			}
		}
		d.Coffees = append(d.Coffees, gc)
	}
	rows.Close()

	// Recent public brews by members, only for coffees the viewer has opened.
	d.Brews, err = s.queryBrews(r.Context(), u, `
		b.is_public = 1 AND o.user_id IS NOT NULL
		AND b.user_id IN (SELECT user_id FROM group_members WHERE group_id = ?)`, 30, id)
	if err != nil {
		return err
	}
	writeJSON(w, 200, d)
	return nil
}

func (s *Server) requireOwner(ctx context.Context, u *User, id int64) (*Group, error) {
	g, err := s.loadGroup(ctx, u, id)
	if err != nil {
		return nil, err
	}
	if g.OwnerID != u.ID {
		return nil, errf(403, "only the group owner can do that")
	}
	return g, nil
}

func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.requireOwner(r.Context(), u, id); err != nil {
		return err
	}
	var in groupInput
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE groups SET name=?, description=?, is_public=? WHERE id = ?`,
		in.Name, in.Description, boolInt(in.IsPublic), id); err != nil {
		return err
	}
	g, err := s.loadGroup(r.Context(), u, id)
	if err != nil {
		return err
	}
	s.notify(r, keyGroups, keyCoffees)
	writeJSON(w, 200, g)
	return nil
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.requireOwner(r.Context(), u, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM groups WHERE id = ?`, id); err != nil {
		return err
	}
	s.notify(r, keyGroups, keyCoffees)
	return noContent(w)
}

func (s *Server) addMember(ctx context.Context, groupID, userID int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`,
		groupID, userID, now())
	return err
}

func (s *Server) joinPublicGroup(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	g, err := s.loadGroup(r.Context(), u, id)
	if err != nil {
		return err
	}
	if !g.IsPublic && g.Role == "" {
		return errNotFound
	}
	if err := s.addMember(r.Context(), id, u.ID); err != nil {
		return err
	}
	g, err = s.loadGroup(r.Context(), u, id)
	if err != nil {
		return err
	}
	s.notify(r, keyGroups, keyCoffees)
	writeJSON(w, 200, g)
	return nil
}

func (s *Server) joinGroupByCode(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Code string `json:"code"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	code := strings.TrimSpace(in.Code)
	// Accept a pasted invite link as well as a bare code.
	if i := strings.LastIndex(code, "/"); i >= 0 {
		code = code[i+1:]
	}
	var id int64
	err := s.rdb.QueryRowContext(r.Context(), `SELECT id FROM groups WHERE invite_code = ?`, code).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return errf(404, "no group has that invite code")
	} else if err != nil {
		return err
	}
	if err := s.addMember(r.Context(), id, u.ID); err != nil {
		return err
	}
	g, err := s.loadGroup(r.Context(), u, id)
	if err != nil {
		return err
	}
	s.notify(r, keyGroups, keyCoffees)
	writeJSON(w, 200, g)
	return nil
}

func (s *Server) leaveGroup(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	g, err := s.loadGroup(r.Context(), u, id)
	if err != nil {
		return err
	}
	if g.OwnerID == u.ID {
		return errf(409, "owners can't leave their group; delete it instead")
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM group_members WHERE group_id = ? AND user_id = ?`, id, u.ID); err != nil {
		return err
	}
	s.notify(r, keyGroups, keyCoffees)
	return noContent(w)
}

func (s *Server) regenerateInvite(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.requireOwner(r.Context(), u, id); err != nil {
		return err
	}
	code := inviteCode()
	if _, err := s.db.ExecContext(r.Context(), `UPDATE groups SET invite_code = ? WHERE id = ?`, code, id); err != nil {
		return err
	}
	s.notify(r, keyGroups, keyCoffees)
	writeJSON(w, 200, map[string]string{"inviteCode": code})
	return nil
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	uid, err := pathID(r, "userId")
	if err != nil {
		return err
	}
	if _, err := s.requireOwner(r.Context(), u, id); err != nil {
		return err
	}
	if uid == u.ID {
		return errf(409, "you can't remove yourself")
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM group_members WHERE group_id = ? AND user_id = ?`, id, uid); err != nil {
		return err
	}
	s.notify(r, keyGroups, keyCoffees)
	return noContent(w)
}
