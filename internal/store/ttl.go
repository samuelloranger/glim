package store

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

// MaxTTL is the longest lifetime Extend accepts.
const MaxTTL = 8760 * time.Hour

// ValidateTTL rejects lifetimes that would create an already-expired preview.
func ValidateTTL(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("ttl must be greater than zero, got %s", d)
	}
	return nil
}

// dayTTLRE matches a whole-day lifetime with an optional Go-duration rest,
// like "3d" or "1d12h".
var dayTTLRE = regexp.MustCompile(`^(\d+)d(.*)$`)

// ParseTTL parses a duration like "6h", "30m", "3d" or "1d12h" and rejects zero
// or negative values. "d" is a whole 24-hour day and must lead.
func ParseTTL(s string) (time.Duration, error) {
	d, err := parseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("bad ttl %q: %w", s, err)
	}
	if err := ValidateTTL(d); err != nil {
		return 0, fmt.Errorf("bad ttl %q: must be greater than zero", s)
	}
	return d, nil
}

func parseDuration(s string) (time.Duration, error) {
	m := dayTTLRE.FindStringSubmatch(s)
	if m == nil {
		return time.ParseDuration(s)
	}
	days, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || days > int64(math.MaxInt64)/int64(24*time.Hour) {
		return 0, fmt.Errorf("too many days")
	}
	d := time.Duration(days) * 24 * time.Hour
	if m[2] == "" {
		return d, nil
	}
	rest, err := time.ParseDuration(m[2])
	if err != nil || m[2][0] == '-' || m[2][0] == '+' {
		return 0, fmt.Errorf("invalid duration")
	}
	if rest > math.MaxInt64-d {
		return 0, fmt.Errorf("duration too large")
	}
	return d + rest, nil
}
