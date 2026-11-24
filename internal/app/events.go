package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Live updates over Server-Sent Events.
//
// Events never carry data, only TanStack Query key prefixes for the client to
// invalidate. Each client then refetches through the normal API with its own
// session, so visibility rules (private brews, unopened coffees, private
// groups) are enforced exactly as for any other request.

// Key is a query-key prefix, e.g. {"coffees"} or {"coffees", 3}.
type Key []any

var (
	keyCoffees  = Key{"coffees"} // list, details and per-coffee brews
	keyGroups   = Key{"groups"}
	keyBrews    = Key{"brews"} // the user's own brew list
	keyRecipes  = Key{"recipes"}
	keyGrinders = Key{"grinders"}
	keyMe       = Key{"me"}
)

type event struct {
	Keys []Key `json:"keys"`

	userID int64  // 0 = everyone, else only this user's streams
	origin string // client id that caused it; that tab already refreshed itself
}

type subscriber struct {
	userID int64
	ip     string
	client string
	ch     chan []byte
}

type broker struct {
	mu      sync.Mutex
	subs    map[*subscriber]struct{}
	closed  bool
	perUser map[int64]int
	perIP   map[string]int
	maxUser int
	maxIP   int
}

func newBroker(l Limits) *broker {
	return &broker{
		subs: map[*subscriber]struct{}{}, perUser: map[int64]int{}, perIP: map[string]int{},
		maxUser: l.StreamsPerUser, maxIP: l.StreamsPerIP,
	}
}

// subscribe registers a stream, or returns nil if the user or IP already
// has too many open (each one holds a connection and some memory).
func (b *broker) subscribe(userID int64, ip, client string) *subscriber {
	sub := &subscriber{userID: userID, ip: ip, client: client, ch: make(chan []byte, 32)}
	b.mu.Lock()
	defer b.mu.Unlock()
	if (b.maxUser > 0 && b.perUser[userID] >= b.maxUser) || (b.maxIP > 0 && b.perIP[ip] >= b.maxIP) {
		return nil
	}
	if b.closed {
		close(sub.ch)
		return sub
	}
	b.subs[sub] = struct{}{}
	b.perUser[userID]++
	b.perIP[ip]++
	return sub
}

// removeLocked drops sub and its counts; callers hold b.mu.
func (b *broker) removeLocked(sub *subscriber) {
	if _, ok := b.subs[sub]; !ok {
		return
	}
	delete(b.subs, sub)
	close(sub.ch)
	if b.perUser[sub.userID]--; b.perUser[sub.userID] <= 0 {
		delete(b.perUser, sub.userID)
	}
	if b.perIP[sub.ip]--; b.perIP[sub.ip] <= 0 {
		delete(b.perIP, sub.ip)
	}
}

func (b *broker) unsubscribe(sub *subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removeLocked(sub)
}

func (b *broker) publish(ev event) {
	data, _ := json.Marshal(ev)
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.subs {
		if ev.userID != 0 && sub.userID != ev.userID {
			continue
		}
		if ev.origin != "" && sub.client == ev.origin {
			continue
		}
		select {
		case sub.ch <- data:
		default:
			// Slow consumer: drop it. The browser reconnects and refetches
			// everything, so nothing is lost.
			b.removeLocked(sub)
		}
	}
}

// closeAll ends every stream (used on shutdown so http.Server.Shutdown
// doesn't wait on connections that never finish on their own).
func (b *broker) closeAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for sub := range b.subs {
		b.removeLocked(sub)
	}
}

// CloseStreams ends all live-update streams.
func (s *Server) CloseStreams() { s.events.closeAll() }

func clientID(r *http.Request) string {
	id := r.Header.Get("X-Client-Id")
	if len(id) > 64 {
		return ""
	}
	return id
}

// notify tells every connected client to refetch the given keys.
func (s *Server) notify(r *http.Request, keys ...Key) {
	s.events.publish(event{Keys: keys, origin: clientID(r)})
}

// notifyUser tells only u's other tabs and devices.
func (s *Server) notifyUser(r *http.Request, u *User, keys ...Key) {
	s.events.publish(event{Keys: keys, userID: u.ID, origin: clientID(r)})
}

const (
	heartbeat = 25 * time.Second
	// coalesce is the minimum gap between batches sent to one stream. Hints
	// arriving in between are merged, so a busy community costs each open
	// page at most one refetch round per window.
	coalesce = 2 * time.Second
)

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request, u *User) error {
	client := r.URL.Query().Get("client")
	if len(client) > 64 {
		client = ""
	}
	sub := s.events.subscribe(u.ID, s.clientIP(r), client)
	if sub == nil {
		return tooMany(w, r, "streams", "Too many open live-update connections.", 30*time.Second)
	}
	defer s.events.unsubscribe(sub)
	rc := http.NewResponseController(w)
	// The server's WriteTimeout would otherwise cut the stream after 30s.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		return err
	}
	// Likewise ReadTimeout: when it expires the request context is canceled.
	rc.SetReadDeadline(time.Time{})

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("X-Accel-Buffering", "no") // nginx and friends: don't buffer
	w.WriteHeader(http.StatusOK)
	// Tell the browser how long to wait before reconnecting.
	fmt.Fprint(w, "retry: 3000\n: connected\n\n")
	if err := rc.Flush(); err != nil {
		return nil
	}

	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	pending := map[string]Key{}
	var flushTimer <-chan time.Time
	var lastSent time.Time
	send := func() error {
		keys := make([]Key, 0, len(pending))
		for _, k := range pending {
			keys = append(keys, k)
		}
		clear(pending)
		flushTimer = nil
		lastSent = time.Now()
		data, _ := json.Marshal(event{Keys: keys})
		_, err := fmt.Fprintf(w, "event: invalidate\ndata: %s\n\n", data)
		return err
	}
	for {
		select {
		case <-r.Context().Done():
			return nil
		case data, ok := <-sub.ch:
			if !ok {
				return nil
			}
			var ev event
			if json.Unmarshal(data, &ev) == nil {
				for _, k := range ev.Keys {
					b, _ := json.Marshal(k)
					pending[string(b)] = k
				}
			}
			if flushTimer != nil {
				continue // already scheduled
			}
			if wait := coalesce - time.Since(lastSent); wait > 0 {
				flushTimer = time.After(wait)
				continue
			}
			if err := send(); err != nil {
				return nil
			}
		case <-flushTimer:
			if err := send(); err != nil {
				return nil
			}
		case <-tick.C:
			// Comment line keeps proxies and NATs from idling us out.
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return nil
			}
		}
		if err := rc.Flush(); err != nil {
			return nil
		}
	}
}
