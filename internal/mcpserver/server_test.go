package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	return store.New(t.TempDir(), "https://glim.example.com")
}

func publish(t *testing.T, s *store.Store, title, project string, ttl time.Duration) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(p, []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := s.Publish(p, title, project, "", ttl, "")
	if err != nil {
		t.Fatal(err)
	}
	return res.Name
}

func TestListPreviews(t *testing.T) {
	s := newTestStore(t)
	publish(t, s, "First", "alpha", time.Hour)
	publish(t, s, "Second", "beta", time.Hour)

	out, err := listPreviews(s, ListInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Previews) != 2 {
		t.Fatalf("want 2 previews, got %d", len(out.Previews))
	}
	for _, p := range out.Previews {
		if p.URL == "" || p.Name == "" || p.Expires == "" {
			t.Fatalf("incomplete preview: %+v", p)
		}
		if p.URL != s.URL(p.Name) {
			t.Fatalf("url %q != %q", p.URL, s.URL(p.Name))
		}
	}
}

func TestListPreviewsIncludesViews(t *testing.T) {
	s := newTestStore(t)
	seen := publish(t, s, "Seen", "alpha", time.Hour)
	publish(t, s, "Fresh", "alpha", time.Hour)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	out, err := listPreviews(s, ListInput{}, map[string]auth.ViewStat{seen: {Count: 4, LastSeen: at}})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range out.Previews {
		if p.Name == seen {
			if p.Views != 4 || p.LastSeen != "2026-01-02T03:04:05Z" {
				t.Fatalf("seen = %+v", p)
			}
		} else if p.Views != 0 || p.LastSeen != "" {
			t.Fatalf("fresh = %+v", p)
		}
	}
}

func TestListPreviewsFiltersByProject(t *testing.T) {
	s := newTestStore(t)
	publish(t, s, "First", "alpha", time.Hour)
	publish(t, s, "Second", "beta", time.Hour)

	out, err := listPreviews(s, ListInput{Project: "beta"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Previews) != 1 {
		t.Fatalf("want 1 preview, got %d", len(out.Previews))
	}
	if out.Previews[0].Project != "beta" {
		t.Fatalf("want project beta, got %q", out.Previews[0].Project)
	}
}

func TestRevokePreview(t *testing.T) {
	s := newTestStore(t)
	name := publish(t, s, "Doomed", "", time.Hour)

	out, err := revokePreview(s, RevokeInput{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if out.Revoked != name {
		t.Fatalf("want revoked %q, got %q", name, out.Revoked)
	}
	list, err := listPreviews(s, ListInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Previews) != 0 {
		t.Fatalf("preview still listed after revoke: %+v", list.Previews)
	}
}

func TestRevokeMissing(t *testing.T) {
	s := newTestStore(t)
	if _, err := revokePreview(s, RevokeInput{Name: "nope-abcd"}); err == nil {
		t.Fatal("want error revoking missing preview, got nil")
	}
}

func TestPinPreview(t *testing.T) {
	s := newTestStore(t)
	name := publish(t, s, "Keep", "", time.Hour)
	out, err := pinPreview(s, time.Hour, PinInput{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if out.Pinned != name {
		t.Fatalf("want pinned %q, got %q", name, out.Pinned)
	}
	m, _ := s.Get(name)
	if !m.Pinned {
		t.Fatal("manifest not pinned after pin")
	}
	if _, err := pinPreview(s, time.Hour, PinInput{Name: "nope-abcd"}); err == nil {
		t.Fatal("want error pinning missing preview")
	}
}

func TestExtendPreview(t *testing.T) {
	s := newTestStore(t)
	name := publish(t, s, "Live", "", time.Hour)
	out, err := extendPreview(s, ExtendInput{Name: name, TTL: "5h"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != name || out.Expires == "" {
		t.Fatalf("bad extend output: %+v", out)
	}
	if _, err := extendPreview(s, ExtendInput{Name: name, TTL: "not-a-duration"}); err == nil {
		t.Fatal("want error on bad ttl")
	}
	if _, err := extendPreview(s, ExtendInput{Name: "nope-abcd", TTL: "1h"}); err == nil {
		t.Fatal("want error extending missing preview")
	}
}

func TestPresentRejectsNonPositiveTTL(t *testing.T) {
	s := newTestStore(t)
	p := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(p, []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, ttl := range []string{"0s", "-1h", "bogus"} {
		if _, err := publishPreview(s, time.Hour, PresentInput{Path: p, TTL: ttl}); err == nil {
			t.Errorf("present ttl %q should fail", ttl)
		}
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Errorf("rejected ttl must not create previews, got %d", len(list))
	}
	if _, err := publishPreview(s, time.Hour, PresentInput{Path: p, TTL: "30m"}); err != nil {
		t.Errorf("valid ttl failed: %v", err)
	}
}

func TestPresentRejectsNonPositiveDefaultTTL(t *testing.T) {
	s := newTestStore(t)
	p := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(p, []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []time.Duration{0, -time.Hour} {
		if _, err := publishPreview(s, d, PresentInput{Path: p}); err == nil {
			t.Errorf("default ttl %v should fail", d)
		}
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Errorf("rejected default ttl must not create previews, got %d", len(list))
	}
	// An explicit valid ttl still works when the default is bad.
	if _, err := publishPreview(s, 0, PresentInput{Path: p, TTL: "30m"}); err != nil {
		t.Errorf("explicit ttl should override bad default: %v", err)
	}
}

func TestExtendRejectsNonPositiveTTL(t *testing.T) {
	s := newTestStore(t)
	name := publish(t, s, "T", "", time.Hour)
	for _, ttl := range []string{"0s", "-1h"} {
		if _, err := extendPreview(s, ExtendInput{Name: name, TTL: ttl}); err == nil {
			t.Errorf("extend ttl %q should fail", ttl)
		}
	}
	if _, err := extendPreview(s, ExtendInput{Name: name, TTL: "2h"}); err != nil {
		t.Errorf("valid extend failed: %v", err)
	}
}
