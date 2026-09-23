package auth

import (
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
		l.Fail("", "9.9.9.9") // e.g. setup-code guesses
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
