package app

import "testing"

func TestGroupDisplayNames(t *testing.T) {
	srv := setup(t)
	a, b, c := newClient(t, srv), newClient(t, srv), newClient(t, srv)
	a.do("POST", "/auth/register", `{"username":"alice","displayName":"Alice","password":"password1"}`, nil)
	b.do("POST", "/auth/register", `{"username":"bob","displayName":"Bob","password":"password1"}`, nil)
	c.do("POST", "/auth/register", `{"username":"carol","password":"password1"}`, nil)

	var g, other Group
	a.do("POST", "/groups", `{"name":"Crew","isPublic":true}`, &g)
	a.do("POST", "/groups", `{"name":"Other","isPublic":true}`, &other)
	b.do("POST", "/groups/"+itoa(g.ID)+"/join", "", nil)
	path := "/groups/" + itoa(g.ID) + "/name"

	if s := c.do("PUT", path, `{"displayName":"Lurker"}`, nil); s != 403 {
		t.Errorf("non-member set a name: %d", s)
	}
	if s := a.do("PUT", path, `{"displayName":"  Captain  "}`, nil); s != 200 {
		t.Fatalf("set name: %d", s)
	}
	// Bob can't take another member's name (group or profile), in any case.
	for _, n := range []string{"captain", "ALICE"} {
		if s := b.do("PUT", path, `{"displayName":"`+n+`"}`, nil); s != 409 {
			t.Errorf("bob took %q: %d", n, s)
		}
	}
	b.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":8,"isPublic":true}`, nil)
	a.do("POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":8,"isPublic":true}`, nil)
	b.do("PUT", path, `{"displayName":"Bobby"}`, nil)

	var d GroupDetail
	b.do("GET", "/groups/"+itoa(g.ID), "", &d)
	if d.OwnerName != "Captain" {
		t.Errorf("owner name = %q", d.OwnerName)
	}
	got := map[string]GroupMember{}
	for _, m := range d.Members {
		got[m.Username] = m
	}
	if m := got["alice"]; m.DisplayName != "Captain" || m.GroupName != "Captain" || m.ProfileName != "Alice" {
		t.Errorf("alice = %+v", m)
	}
	if m := got["bob"]; m.DisplayName != "Bobby" {
		t.Errorf("bob = %+v", m)
	}
	for _, br := range d.Brews {
		if want := map[string]string{"alice": "Captain", "bob": "Bobby"}[br.Username]; br.DisplayName != want {
			t.Errorf("brew by %s shows %q, want %q", br.Username, br.DisplayName, want)
		}
	}

	// Names stay in their group: elsewhere alice is still Alice.
	b.do("GET", "/groups/"+itoa(other.ID), "", &d)
	if d.OwnerName != "Alice" {
		t.Errorf("other group owner name = %q", d.OwnerName)
	}
	var brews []Brew
	b.do("GET", "/coffees/1/brews", "", &brews)
	for _, br := range brews {
		if br.Username == "alice" && br.DisplayName != "Alice" {
			t.Errorf("coffee feed shows %q", br.DisplayName)
		}
	}

	// Clearing falls back to the profile name.
	a.do("PUT", path, `{"displayName":""}`, nil)
	b.do("GET", "/groups/"+itoa(g.ID), "", &d)
	if d.OwnerName != "Alice" {
		t.Errorf("after clear owner name = %q", d.OwnerName)
	}
}
