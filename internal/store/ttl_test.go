package store

import (
	"testing"
	"time"
)

func TestParseTTL(t *testing.T) {
	ok := map[string]time.Duration{"30m": 30 * time.Minute, "6h": 6 * time.Hour, "1s": time.Second,
		"3d": 72 * time.Hour, "1d12h": 36 * time.Hour, "0d1h": time.Hour}
	for in, want := range ok {
		got, err := ParseTTL(in)
		if err != nil || got != want {
			t.Errorf("ParseTTL(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "0s", "-1h", "-5m", "", "abc", "d", "-1d", "1d-2h", "1dx", "0d", "1.5d", "99999999999d"} {
		if _, err := ParseTTL(in); err == nil {
			t.Errorf("ParseTTL(%q) should fail", in)
		}
	}
	if err := ValidateTTL(0); err == nil {
		t.Error("ValidateTTL(0) should fail")
	}
	if err := ValidateTTL(-time.Second); err == nil {
		t.Error("ValidateTTL(-1s) should fail")
	}
}
