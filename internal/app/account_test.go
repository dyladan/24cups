package app

import "testing"

func TestDeleteAccount(t *testing.T) {
	srv := setup(t)
	a, b := newClient(t, srv), newClient(t, srv)
	a.do("POST", "/auth/register", `{"username":"Alice","password":"password1"}`, nil)
	b.do("POST", "/auth/register", `{"username":"bob","password":"password1"}`, nil)

	// Alice owns a group Bob is in, and is a member of Bob's group.
	var aliceGroup, bobGroup Group
	a.do("POST", "/groups", `{"name":"Alice's","isPublic":true}`, &aliceGroup)
	b.do("POST", "/groups", `{"name":"Bob's","isPublic":true}`, &bobGroup)
	b.do("POST", "/groups/"+itoa(aliceGroup.ID)+"/join", "", nil)
	a.do("POST", "/groups/"+itoa(bobGroup.ID)+"/join", "", nil)
	a.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16}`, nil)
	b.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16}`, nil)
	b.do("PATCH", "/me", `{"spoilerMode":"always"}`, nil)
	var before []Brew
	b.do("GET", "/coffees/1/brews", "", &before)
	if len(before) != 2 {
		t.Fatalf("brews on coffee 1 before delete: %d, want 2", len(before))
	}

	// A second signed-in device for Alice.
	a2 := newClient(t, srv)
	a2.do("POST", "/auth/login", `{"username":"alice","password":"password1"}`, nil)

	// The confirmation must match the username (case-insensitively).
	for _, body := range []string{`{}`, `{"username":"bob"}`, `{"username":"alic"}`} {
		if s := a.do("DELETE", "/me", body, nil); s != 400 {
			t.Fatalf("delete with %s: %d, want 400", body, s)
		}
	}
	if s := b.do("DELETE", "/me", `{"username":"alice"}`, nil); s != 400 {
		t.Fatalf("bob deleting with alice's name: %d, want 400", s)
	}
	if s := (newClient(t, srv)).do("DELETE", "/me", `{"username":"alice"}`, nil); s != 401 {
		t.Fatalf("anonymous delete: %d, want 401", s)
	}

	// Owning a group blocks deletion, and nothing is deleted.
	if s := a.do("DELETE", "/me", `{"username":"alice"}`, nil); s != 409 {
		t.Fatalf("delete while owning a group: %d, want 409", s)
	}
	if s := a.do("GET", "/auth/me", "", nil); s != 200 {
		t.Fatalf("/auth/me after refused delete: %d", s)
	}
	if s := a.do("PUT", "/groups/"+itoa(aliceGroup.ID)+"/owner", `{"userId":`+itoa(b.me().ID)+`}`, nil); s != 204 {
		t.Fatalf("transfer: %d", s)
	}

	if s := a.do("DELETE", "/me", `{"username":" alice "}`, nil); s != 204 {
		t.Fatalf("delete: %d", s)
	}

	// Every session is gone, including the other device.
	for name, c := range map[string]*client{"a": a, "a2": a2} {
		if s := c.do("GET", "/auth/me", "", nil); s != 401 {
			t.Fatalf("%s /auth/me after delete: %d, want 401", name, s)
		}
	}
	if s := a2.do("POST", "/auth/login", `{"username":"alice","password":"password1"}`, nil); s != 401 {
		t.Fatalf("login after delete: %d, want 401", s)
	}

	// Both groups survive with Bob as their only member.
	var mine []Group
	b.do("GET", "/groups", "", &mine)
	if len(mine) != 2 {
		t.Fatalf("bob's groups after delete: %+v", mine)
	}
	for _, g := range mine {
		if g.MemberCount != 1 || g.Role != "owner" {
			t.Fatalf("group after delete: %+v", g)
		}
	}

	// Only Bob's brew is left on the coffee.
	var brews []Brew
	b.do("GET", "/coffees/1/brews", "", &brews)
	if len(brews) != 1 {
		t.Fatalf("brews on coffee 1: %d, want 1", len(brews))
	}

	// The username is free again.
	if s := newClient(t, srv).do("POST", "/auth/register", `{"username":"alice","password":"password1"}`, nil); s != 201 {
		t.Fatalf("re-register alice: %d", s)
	}
}

func TestTransferOwnership(t *testing.T) {
	srv := setup(t)
	a, b, c := newClient(t, srv), newClient(t, srv), newClient(t, srv)
	a.do("POST", "/auth/register", `{"username":"alice","password":"password1"}`, nil)
	b.do("POST", "/auth/register", `{"username":"bob","password":"password1"}`, nil)
	c.do("POST", "/auth/register", `{"username":"carol","password":"password1"}`, nil)
	aID, bID, cID := a.me().ID, b.me().ID, c.me().ID

	var g Group
	a.do("POST", "/groups", `{"name":"Crew","isPublic":true}`, &g)
	b.do("POST", "/groups/"+itoa(g.ID)+"/join", "", nil)
	path := "/groups/" + itoa(g.ID) + "/owner"
	to := func(id int64) string { return `{"userId":` + itoa(id) + `}` }

	if s := b.do("PUT", path, to(bID), nil); s != 403 {
		t.Fatalf("non-owner transfer: %d, want 403", s)
	}
	if s := a.do("PUT", path, to(cID), nil); s != 400 {
		t.Fatalf("transfer to non-member: %d, want 400", s)
	}
	if s := a.do("PUT", path, to(aID), nil); s != 409 {
		t.Fatalf("transfer to self: %d, want 409", s)
	}
	if s := a.do("PUT", path, to(bID), nil); s != 204 {
		t.Fatalf("transfer: %d", s)
	}

	var d GroupDetail
	b.do("GET", "/groups/"+itoa(g.ID), "", &d)
	if d.OwnerID != bID || d.Role != "owner" {
		t.Fatalf("after transfer: owner %d role %q", d.OwnerID, d.Role)
	}
	roles := map[int64]string{}
	for _, m := range d.Members {
		roles[m.UserID] = m.Role
	}
	if roles[aID] != "member" || roles[bID] != "owner" {
		t.Fatalf("roles after transfer: %v", roles)
	}

	// The old owner is now an ordinary member: no owner actions, free to leave.
	if s := a.do("PUT", path, to(aID), nil); s != 403 {
		t.Fatalf("old owner transferring back: %d, want 403", s)
	}
	if s := a.do("DELETE", "/groups/"+itoa(g.ID), "", nil); s != 403 {
		t.Fatalf("old owner deleting: %d, want 403", s)
	}
	if s := b.do("POST", "/groups/"+itoa(g.ID)+"/leave", "", nil); s != 409 {
		t.Fatalf("new owner leaving: %d, want 409", s)
	}
	if s := a.do("POST", "/groups/"+itoa(g.ID)+"/leave", "", nil); s != 204 {
		t.Fatalf("old owner leaving: %d", s)
	}
}

func TestTransferOwnershipRespectsCap(t *testing.T) {
	srv := setupWith(t, Limits{PasswordChecks: 3, MaxOwnedGroups: 1})
	a, b := newClient(t, srv), newClient(t, srv)
	a.do("POST", "/auth/register", `{"username":"alice","password":"password1"}`, nil)
	b.do("POST", "/auth/register", `{"username":"bob","password":"password1"}`, nil)
	var g Group
	a.do("POST", "/groups", `{"name":"A","isPublic":true}`, &g)
	b.do("POST", "/groups", `{"name":"B","isPublic":true}`, nil)
	b.do("POST", "/groups/"+itoa(g.ID)+"/join", "", nil)
	if s := a.do("PUT", "/groups/"+itoa(g.ID)+"/owner", `{"userId":`+itoa(b.me().ID)+`}`, nil); s != 409 {
		t.Fatalf("transfer to someone at the cap: %d, want 409", s)
	}
}

// me returns the signed-in user.
func (c *client) me() User {
	c.t.Helper()
	var u User
	if s := c.do("GET", "/auth/me", "", &u); s != 200 {
		c.t.Fatalf("/auth/me: %d", s)
	}
	return u
}
