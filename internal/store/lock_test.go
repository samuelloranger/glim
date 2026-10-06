package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManifestPasswordHashRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, Manifest{Name: "a-1234", PasswordHash: "$2a$hash"}); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(dir)
	if err != nil || m.PasswordHash != "$2a$hash" || !m.Locked() {
		t.Fatalf("round trip = %+v, %v", m, err)
	}
	if err := writeManifest(dir, Manifest{Name: "a-1234"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
	if strings.Contains(string(raw), "password_hash") {
		t.Fatalf("empty hash must be omitted: %s", raw)
	}
}

func TestRepublishKeepsOrReplacesPasswordHash(t *testing.T) {
	s := newTestStore(t)
	entry := writeTemp(t, "p.html", "<h1>x</h1>")
	res, err := s.PublishLocked(entry, "", "", "", time.Hour, "keep-me", "hash-1")
	if err != nil || !res.Locked {
		t.Fatalf("publish locked = %+v, %v", res, err)
	}
	if _, err := s.Publish(entry, "", "", "", time.Hour, "keep-me"); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Get("keep-me"); m.PasswordHash != "hash-1" {
		t.Fatalf("republish dropped the hash: %q", m.PasswordHash)
	}
	if _, err := s.PublishLocked(entry, "", "", "", time.Hour, "keep-me", "hash-2"); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Get("keep-me"); m.PasswordHash != "hash-2" {
		t.Fatalf("new password not stored: %q", m.PasswordHash)
	}
	if err := s.SetPasswordHash("keep-me", ""); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Get("keep-me"); m.Locked() {
		t.Fatal("still locked after unlock")
	}
	if err := s.SetPasswordHash("missing-9999", "x"); err == nil {
		t.Fatal("want error for missing preview")
	}
}
