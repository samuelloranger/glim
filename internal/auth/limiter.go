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
	// caps holds the sliding windows behind ReserveWindow, keyed by caller.
	caps map[string][]time.Time
	// lastSweep is when idle entries were last evicted; see maybeSweep.
	lastSweep time.Time
}

func NewLimiter(now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{now: now, users: map[string]*userFails{}, ips: map[string][]time.Time{}, caps: map[string][]time.Time{}}
}

// Check reports how long the caller must wait, without counting an attempt.
// Callers that go on to verify a password must use Reserve instead: a Check
// followed by a later Fail lets parallel requests all pass the check.
func (l *Limiter) Check(user, ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.maybeSweep(now)
	return l.wait(user, ip, now)
}

// Reserve is Check and Fail in one critical section: when the attempt is
// allowed (wait == 0) it is counted as a failure immediately, before the
// password is verified, so concurrent attempts cannot all slip past the limit.
// The caller undoes the count with Release when the attempt turns out not to be
// a credential failure, and with Release plus Succeed when it succeeds. A
// blocked attempt (wait > 0) is not counted.
func (l *Limiter) Reserve(user, ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.maybeSweep(now)
	if wait := l.wait(user, ip, now); wait > 0 {
		return wait
	}
	l.fail(user, ip, now)
	return 0
}

// Release undoes one Reserve: it drops one failure from the account and the
// newest one from the address.
func (l *Limiter) Release(user, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if f := l.users[user]; user != "" && f != nil {
		if f.count--; f.count <= 0 {
			delete(l.users, user)
		}
	}
	if fails := l.ips[ip]; ip != "" && len(fails) > 1 {
		l.ips[ip] = fails[:len(fails)-1]
	} else {
		delete(l.ips, ip)
	}
}

// ReserveWindow counts one attempt against key, allowing at most limit per
// window. It returns the wait until the oldest counted attempt leaves the
// window, or 0 when the attempt was admitted (and counted). It is independent
// of any client address, so rotating addresses cannot buy more attempts.
func (l *Limiter) ReserveWindow(key string, limit int, window time.Duration) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.maybeSweep(now)
	hits := l.caps[key]
	i := 0
	for i < len(hits) && now.Sub(hits[i]) >= window {
		i++
	}
	hits = hits[i:]
	if len(hits) >= limit {
		l.caps[key] = hits
		return hits[len(hits)-limit].Add(window).Sub(now)
	}
	l.caps[key] = append(hits, now)
	return 0
}

// ReleaseWindow undoes one admitted ReserveWindow.
func (l *Limiter) ReleaseWindow(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if hits := l.caps[key]; len(hits) > 1 {
		l.caps[key] = hits[:len(hits)-1]
	} else {
		delete(l.caps, key)
	}
}

func (l *Limiter) wait(user, ip string, now time.Time) time.Duration {
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
	l.fail(user, ip, now)
}

func (l *Limiter) fail(user, ip string, now time.Time) {
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
	// Window caps are pruned lazily on use; drop keys idle past the horizon so
	// abandoned slugs don't accumulate.
	for k, hits := range l.caps {
		if len(hits) == 0 || now.Sub(hits[len(hits)-1]) >= userIdleHorizon {
			delete(l.caps, k)
		}
	}
}
