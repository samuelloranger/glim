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

// sweep drops account entries whose backoff has fully elapsed, bounding memory
// when an attacker sprays many addresses.
func (l *Limiter) sweep(now time.Time) {
	for u, f := range l.users {
		if now.Sub(f.last) > userMaxBackoff {
			delete(l.users, u)
		}
	}
}
