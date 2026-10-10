package store

import (
	"testing"
	"time"
)

func TestExtendUnpinsPinnedPreview(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }
	res, _ := s.Publish(writeTemp(t, "a.html", "a"), "keep", "", "", time.Hour, "")
	if err := s.Pin(res.Name); err != nil {
		t.Fatal(err)
	}
	if err := s.Extend(res.Name, 3*time.Hour); err != nil {
		t.Fatal(err)
	}
	m, err := s.Get(res.Name)
	if err != nil {
		t.Fatal(err)
	}
	if m.Pinned || !m.Expires.Equal(base.Add(3*time.Hour)) {
		t.Fatalf("extend of a pinned preview = pinned %v expires %v, want unpinned at +3h", m.Pinned, m.Expires)
	}
}

func TestUnpinRestoresExpiry(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }
	res, _ := s.Publish(writeTemp(t, "a.html", "a"), "keep", "", "", time.Hour, "")
	if err := s.Pin(res.Name); err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return base.Add(100 * time.Hour) }
	if err := s.Unpin(res.Name, 6*time.Hour); err != nil {
		t.Fatal(err)
	}
	m, _ := s.Get(res.Name)
	if m.Pinned || !m.Expires.Equal(base.Add(106*time.Hour)) {
		t.Fatalf("unpin = pinned %v expires %v, want unpinned at now+6h", m.Pinned, m.Expires)
	}
	// Unpinning a preview that is not pinned leaves its expiry alone.
	if err := s.Unpin(res.Name, time.Hour); err != nil {
		t.Fatal(err)
	}
	m, _ = s.Get(res.Name)
	if !m.Expires.Equal(base.Add(106 * time.Hour)) {
		t.Fatalf("no-op unpin moved expiry to %v", m.Expires)
	}
}

func TestUnpinRejectsBadInput(t *testing.T) {
	s := newTestStore(t)
	res, _ := s.Publish(writeTemp(t, "a.html", "a"), "keep", "", "", time.Hour, "")
	if err := s.Unpin("nope-abcd", time.Hour); err == nil {
		t.Fatal("want error unpinning unknown preview")
	}
	if err := s.Unpin(res.Name, 0); err == nil {
		t.Fatal("want error for zero ttl")
	}
	if err := s.Unpin(res.Name, MaxTTL+time.Second); err == nil {
		t.Fatal("want error beyond MaxTTL")
	}
}
