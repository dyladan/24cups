package app

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limits configures abuse protection. Zero values disable a limit.
type Limits struct {
	// General API traffic.
	IPRate, IPBurst     float64 // per client IP, all /api requests
	UserRate, UserBurst float64 // per signed-in user
	WriteRate           float64 // per user, POST/PUT/PATCH/DELETE
	WriteBurst          float64

	// Accounts.
	SignupPerHour      float64 // per IP
	LoginPerMinute     float64 // per IP, all attempts
	LoginFailsPer15Min float64 // per username, failures only
	FailedLoginDelay   time.Duration
	PasswordChecks     int // concurrent bcrypt operations

	// Live updates.
	StreamsPerUser, StreamsPerIP int

	// Content caps per user.
	MaxBrewsPerCoffee, MaxRecipes, MaxGrinders, MaxOwnedGroups int
}

// DefaultLimits are generous for real people (a household or office behind
// one IP, a page refetching every couple of seconds) and tight for scripts.
func DefaultLimits() Limits {
	return Limits{
		IPRate: 30, IPBurst: 150,
		UserRate: 15, UserBurst: 75,
		WriteRate: 1, WriteBurst: 20,

		SignupPerHour:      5,
		LoginPerMinute:     10,
		LoginFailsPer15Min: 10,
		FailedLoginDelay:   time.Second,
		PasswordChecks:     3,

		StreamsPerUser: 10,
		StreamsPerIP:   40,

		MaxBrewsPerCoffee: 30,
		MaxRecipes:        50,
		MaxGrinders:       20,
		MaxOwnedGroups:    10,
	}
}

// keyedLimiter is a set of token buckets keyed by IP, user, etc. Idle
// buckets are pruned, and the set has a hard size cap so rotating keys
// (e.g. IPv6 addresses) can't exhaust memory.
type keyedLimiter struct {
	mu      sync.Mutex
	r       rate.Limit
	burst   int
	m       map[string]*bucket
	maxKeys int
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

func newKeyedLimiter(perSecond, burst float64) *keyedLimiter {
	if perSecond <= 0 {
		return nil
	}
	b := int(math.Max(1, burst))
	return &keyedLimiter{r: rate.Limit(perSecond), burst: b, m: map[string]*bucket{}, maxKeys: 200_000}
}

func (k *keyedLimiter) get(key string) *rate.Limiter {
	now := time.Now()
	b := k.m[key]
	if b == nil {
		if len(k.m) >= k.maxKeys {
			k.pruneLocked(now, 0)
		}
		b = &bucket{lim: rate.NewLimiter(k.r, k.burst)}
		k.m[key] = b
	}
	b.seen = now
	return b.lim
}

// allow takes one token, or reports how long until one is available.
func (k *keyedLimiter) allow(key string) (bool, time.Duration) {
	if k == nil {
		return true, 0
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	res := k.get(key).Reserve()
	if d := res.Delay(); d > 0 {
		res.Cancel()
		return false, d
	}
	return true, 0
}

// blocked reports whether key has no tokens left, without taking one.
func (k *keyedLimiter) blocked(key string) (bool, time.Duration) {
	if k == nil {
		return false, 0
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	lim := k.get(key)
	if t := lim.Tokens(); t < 1 {
		return true, time.Duration((1 - t) / float64(k.r) * float64(time.Second))
	}
	return false, 0
}

// spend takes a token if one is available (used to count failures).
func (k *keyedLimiter) spend(key string) {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.get(key).Allow()
}

// pruneLocked drops buckets idle longer than maxIdle (or, with maxIdle 0,
// the oldest half when over capacity).
func (k *keyedLimiter) pruneLocked(now time.Time, maxIdle time.Duration) {
	if maxIdle > 0 {
		for key, b := range k.m {
			if now.Sub(b.seen) > maxIdle {
				delete(k.m, key)
			}
		}
		return
	}
	// Over capacity: evict anything idle more than a few seconds, then
	// arbitrary entries until half full.
	for key, b := range k.m {
		if now.Sub(b.seen) > 5*time.Second || len(k.m) > k.maxKeys/2 {
			delete(k.m, key)
		}
	}
}

func (k *keyedLimiter) prune(maxIdle time.Duration) {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.pruneLocked(time.Now(), maxIdle)
}

// guards bundles the limiters a Server uses.
type guards struct {
	cfg        Limits
	ip         *keyedLimiter
	user       *keyedLimiter
	write      *keyedLimiter
	signup     *keyedLimiter
	login      *keyedLimiter
	loginFails *keyedLimiter
	pwFails    *keyedLimiter // change-password failures, per user
	bcrypt     chan struct{}
}

func newGuards(cfg Limits) *guards {
	g := &guards{
		cfg:        cfg,
		ip:         newKeyedLimiter(cfg.IPRate, cfg.IPBurst),
		user:       newKeyedLimiter(cfg.UserRate, cfg.UserBurst),
		write:      newKeyedLimiter(cfg.WriteRate, cfg.WriteBurst),
		signup:     newKeyedLimiter(cfg.SignupPerHour/3600, cfg.SignupPerHour),
		login:      newKeyedLimiter(cfg.LoginPerMinute/60, cfg.LoginPerMinute),
		loginFails: newKeyedLimiter(cfg.LoginFailsPer15Min/900, cfg.LoginFailsPer15Min),
		pwFails:    newKeyedLimiter(cfg.LoginFailsPer15Min/900, cfg.LoginFailsPer15Min),
	}
	if cfg.PasswordChecks > 0 {
		g.bcrypt = make(chan struct{}, cfg.PasswordChecks)
	}
	return g
}

// janitor prunes idle buckets until ctx is done.
func (g *guards) janitor(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, k := range []*keyedLimiter{g.ip, g.user, g.write} {
				k.prune(10 * time.Minute)
			}
			for _, k := range []*keyedLimiter{g.signup, g.login, g.loginFails, g.pwFails} {
				k.prune(2 * time.Hour)
			}
		}
	}
}

// withPasswordCheck bounds concurrent bcrypt work, the most CPU-expensive
// thing an anonymous client can trigger.
func (g *guards) withPasswordCheck(ctx context.Context, fn func()) error {
	if g.bcrypt == nil {
		fn()
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case g.bcrypt <- struct{}{}:
		defer func() { <-g.bcrypt }()
		fn()
		return nil
	case <-ctx.Done():
		return errf(http.StatusServiceUnavailable, "the server is busy; try again in a moment")
	}
}

// failDelay slows down failed sign-ins without holding a bcrypt slot.
func (g *guards) failDelay(ctx context.Context) {
	if g.cfg.FailedLoginDelay <= 0 {
		return
	}
	t := time.NewTimer(g.cfg.FailedLoginDelay)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// tooMany builds a 429 with a human-readable wait.
func tooMany(w http.ResponseWriter, what string, wait time.Duration) error {
	secs := int(math.Ceil(wait.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	return errf(http.StatusTooManyRequests, "%s Try again in %s.", what, humanWait(secs))
}

func humanWait(secs int) string {
	switch {
	case secs < 90:
		return fmt.Sprintf("%d seconds", secs)
	case secs < 90*60:
		return fmt.Sprintf("%d minutes", (secs+59)/60)
	default:
		return fmt.Sprintf("%d hours", (secs+3599)/3600)
	}
}

// clientIP returns the caller's address used as a rate-limit key. The
// configured header (e.g. CF-Connecting-IP) is trusted only when set by the
// operator; otherwise anyone could spoof it. IPv6 is grouped by /64 because
// one host can trivially rotate through its whole prefix.
func (s *Server) clientIP(r *http.Request) string {
	raw := s.rawClientIP(r)
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return raw
	}
	addr = addr.Unmap()
	if addr.Is6() {
		p, _ := addr.Prefix(64)
		return p.String()
	}
	return addr.String()
}

// rawClientIP is the caller's exact address (trusted header or RemoteAddr).
func (s *Server) rawClientIP(r *http.Request) string {
	raw := ""
	if s.ipHeader != "" {
		raw = strings.TrimSpace(strings.Split(r.Header.Get(s.ipHeader), ",")[0])
	}
	if raw == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		raw = host
	}
	return raw
}
