package store

import (
	"fmt"
	"time"
)

// ValidateTTL rejects lifetimes that would create an already-expired preview.
func ValidateTTL(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("ttl must be greater than zero, got %s", d)
	}
	return nil
}

// ParseTTL parses a duration like "6h" or "30m" and rejects zero or negative
// values.
func ParseTTL(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("bad ttl %q: %w", s, err)
	}
	if err := ValidateTTL(d); err != nil {
		return 0, fmt.Errorf("bad ttl %q: must be greater than zero", s)
	}
	return d, nil
}
