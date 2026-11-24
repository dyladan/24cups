package app

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// stream opens /api/events for c and returns a channel of "data:" payloads.
func (c *client) stream(t *testing.T, clientID string) <-chan string {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", c.srv.URL+"/api/events?client="+clientID, nil)
	res, err := c.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("events: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	out := make(chan string, 16)
	go func() {
		defer res.Body.Close()
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if d, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				out <- d
			}
		}
	}()
	return out
}

func (c *client) doAs(clientID, method, path, body string) int {
	req, _ := http.NewRequest(method, c.srv.URL+"/api"+path, strings.NewReader(body))
	req.Header.Set("X-Requested-With", "test")
	req.Header.Set("X-Client-Id", clientID)
	res, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func expect(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if !strings.Contains(got, want) {
			t.Errorf("got %s, want it to contain %s", got, want)
		}
	case <-time.After(4 * time.Second):
		t.Errorf("no event (wanted %s)", want)
	}
}

func expectNone(t *testing.T, ch <-chan string) {
	t.Helper()
	select {
	case got := <-ch:
		t.Errorf("unexpected event %s", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestLiveEvents(t *testing.T) {
	srv := setup(t)
	alice, bob := newClient(t, srv), newClient(t, srv)
	alice.do("POST", "/auth/register", `{"username":"alice","password":"password1"}`, nil)
	bob.do("POST", "/auth/register", `{"username":"bob","password":"password1"}`, nil)

	if s := newClient(t, srv).do("GET", "/events", "", nil); s != 401 {
		t.Fatalf("anonymous stream allowed: %d", s)
	}

	aliceTab1 := alice.stream(t, "a1")
	aliceTab2 := alice.stream(t, "a2")
	bobTab := bob.stream(t, "b1")
	time.Sleep(100 * time.Millisecond) // let subscriptions register

	// A brew from alice's tab 1: everyone else hears about coffees/groups,
	// alice's other tab also hears about her brew list, tab 1 hears nothing.
	alice.doAs("a1", "POST", "/brews", `{"coffeeId":1,"method":"V60","ratio":16,"rating":8,"isPublic":false}`)
	expect(t, bobTab, `["coffees"]`)
	expectNone(t, bobTab) // bob must not get alice's ["brews"]
	got := []string{<-aliceTab2, <-aliceTab2}
	if !strings.Contains(strings.Join(got, ""), `["brews"]`) {
		t.Errorf("alice's other tab missed her brews key: %v", got)
	}
	expectNone(t, aliceTab1)

	// Recipes are private to alice.
	alice.doAs("a1", "POST", "/recipes", `{"name":"x","method":"V60","ratio":16}`)
	expect(t, aliceTab2, `["recipes"]`)
	expectNone(t, bobTab)
}
