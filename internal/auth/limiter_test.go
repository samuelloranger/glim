package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiterPerUserBackoff(t *testing.T) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	l := NewLimiter(c.now)
	for i := 0; i < 5; i++ {
		if w := l.Check("sam", "1.1.1.1"); w != 0 {
			t.Fatalf("attempt %d blocked for %v", i, w)
		}
		l.Fail("sam", "1.1.1.1")
	}
	if w := l.Check("sam", "2.2.2.2"); w != time.Second {
		t.Fatalf("after 5 fails wait = %v, want 1s (any IP)", w)
	}
	c.advance(time.Second)
	if w := l.Check("sam", "1.1.1.1"); w != 0 {
		t.Fatalf("after waiting = %v", w)
	}
	l.Fail("sam", "1.1.1.1")
	if w := l.Check("sam", "1.1.1.1"); w != 2*time.Second {
		t.Fatalf("6th fail wait = %v, want 2s", w)
	}
	for i := 0; i < 30; i++ {
		l.Fail("sam", "")
	}
	if w := l.Check("sam", ""); w != 15*time.Minute {
		t.Fatalf("cap = %v, want 15m", w)
	}
	l.Succeed("sam")
	if w := l.Check("sam", ""); w != 0 {
		t.Fatalf("after success = %v", w)
	}
	if w := l.Check("amy", "3.3.3.3"); w != 0 {
		t.Fatalf("other user blocked: %v", w)
	}
}

func TestLimiterPerIPWindow(t *testing.T) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	l := NewLimiter(c.now)
	for i := 0; i < 20; i++ {
		l.Fail("", "9.9.9.9") // failures not tied to one account
		c.advance(time.Second)
	}
	w := l.Check("anyone", "9.9.9.9")
	if w <= 0 || w > 15*time.Minute {
		t.Fatalf("ip wait = %v", w)
	}
	if w := l.Check("anyone", "8.8.8.8"); w != 0 {
		t.Fatalf("other ip blocked: %v", w)
	}
	c.advance(15 * time.Minute)
	if w := l.Check("anyone", "9.9.9.9"); w != 0 {
		t.Fatalf("after window = %v", w)
	}
}

func TestLimiterEvictsIdleEntries(t *testing.T) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	l := NewLimiter(c.now)
	for i := 0; i < 50; i++ {
		l.Fail(fmt.Sprintf("user%d", i), fmt.Sprintf("10.0.0.%d", i))
	}
	size := func() (int, int) {
		l.mu.Lock()
		defer l.mu.Unlock()
		return len(l.users), len(l.ips)
	}
	if u, i := size(); u != 50 || i != 50 {
		t.Fatalf("tracked = %d users, %d ips", u, i)
	}
	// Nothing touches the entries again; a later request for someone else
	// triggers the periodic sweep once they are all idle.
	c.advance(sweepEvery + userMaxBackoff + time.Second)
	l.Check("someone", "192.0.2.1")
	if u, i := size(); u != 0 || i != 0 {
		t.Fatalf("after idle sweep = %d users, %d ips, want 0, 0", u, i)
	}
}

func TestLimiterSweepKeepsActiveIPs(t *testing.T) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	l := NewLimiter(c.now)
	l.Check("", "")
	l.Fail("", "1.1.1.1")
	c.advance(ipWindow - time.Minute)
	l.Fail("", "2.2.2.2")
	c.advance(time.Minute) // 1.1.1.1 is now a full window old
	l.Check("", "")
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.ips["1.1.1.1"]; ok {
		t.Error("idle ip kept")
	}
	if _, ok := l.ips["2.2.2.2"]; !ok {
		t.Error("recent ip evicted")
	}
}
