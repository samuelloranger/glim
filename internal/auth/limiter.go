package auth

import (
	"sync"
	"time"
)

const (
	userFreeFails  = 5
	userMaxBackoff = 15 * time.Minute
	ipMaxFails     = 20
	ipWindow       = 15 * time.Minute
	maxTracked     = 10_000
	sweepEvery     = ipWindow
	// userIdleHorizon is how long a throttled account's failure count survives
	// without a new attempt before the periodic sweep evicts it.
	userIdleHorizon = 24 * time.Hour
)

type userFails struct {
	count int
	last  time.Time
}

// Limiter throttles sign-in attempts in memory. The per-account
// backoff can't be dodged by rotating IPs; the per-IP window stops spraying.
type Limiter struct {
	mu    sync.Mutex
	now   func() time.Time
	users map[string]*userFails
	ips   map[string][]time.Time
	// lastSweep is when idle entries were last evicted; see maybeSweep.
	lastSweep time.Time
}

func NewLimiter(now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{now: now, users: map[string]*userFails{}, ips: map[string][]time.Time{}}
}

func (l *Limiter) Check(user, ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.maybeSweep(now)
	var wait time.Duration
	if f := l.users[user]; user != "" && f != nil && f.count >= userFreeFails {
		shift := f.count - userFreeFails
		if shift > 10 {
			shift = 10
		}
		backoff := time.Second << shift
		if backoff > userMaxBackoff {
			backoff = userMaxBackoff
		}
		if until := f.last.Add(backoff); until.After(now) {
			wait = until.Sub(now)
		}
	}
	if fails := l.pruneIP(ip, now); len(fails) >= ipMaxFails {
		if d := fails[len(fails)-ipMaxFails].Add(ipWindow).Sub(now); d > wait {
			wait = d
		}
	}
	return wait
}

func (l *Limiter) Fail(user, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.maybeSweep(now)
	if user != "" {
		f := l.users[user]
		if f == nil {
			if len(l.users) >= maxTracked {
				l.sweep(now)
			}
			f = &userFails{}
			l.users[user] = f
		}
		f.count++
		f.last = now
	}
	if ip != "" {
		l.ips[ip] = append(l.pruneIP(ip, now), now)
	}
}

func (l *Limiter) Succeed(user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.users, user)
}

func (l *Limiter) pruneIP(ip string, now time.Time) []time.Time {
	fails := l.ips[ip]
	i := 0
	for i < len(fails) && now.Sub(fails[i]) >= ipWindow {
		i++
	}
	fails = fails[i:]
	if len(fails) == 0 {
		delete(l.ips, ip)
		return nil
	}
	l.ips[ip] = fails
	return fails
}

// sweep is the emergency eviction at maxTracked: it drops every account entry
// whose backoff has fully elapsed, bounding memory when an attacker sprays
// many addresses.
func (l *Limiter) sweep(now time.Time) {
	for u, f := range l.users {
		if now.Sub(f.last) > userMaxBackoff {
			delete(l.users, u)
		}
	}
}

// maybeSweep evicts idle entries at most once per sweepEvery, so an address that
// failed once and never returned can't stay in the map forever.
func (l *Limiter) maybeSweep(now time.Time) {
	if l.lastSweep.IsZero() {
		l.lastSweep = now
		return
	}
	if now.Sub(l.lastSweep) < sweepEvery {
		return
	}
	l.lastSweep = now
	for u, f := range l.users {
		// Throttled accounts keep their count so rotating IPs and waiting out
		// the backoff can't reset it; only a long idle horizon evicts them.
		idle := now.Sub(f.last)
		if (f.count < userFreeFails && idle > userMaxBackoff) || idle > userIdleHorizon {
			delete(l.users, u)
		}
	}
	for ip := range l.ips {
		l.pruneIP(ip, now)
	}
}
