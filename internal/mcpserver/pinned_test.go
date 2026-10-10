package mcpserver

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListPreviewsReportsPinnedWithoutExpiry(t *testing.T) {
	s := newTestStore(t)
	pinned := publish(t, s, "Pinned", "", time.Hour)
	live := publish(t, s, "Live", "", time.Hour)
	if err := s.Pin(pinned); err != nil {
		t.Fatal(err)
	}
	out, err := listPreviews(s, ListInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range out.Previews {
		switch p.Name {
		case pinned:
			if !p.Pinned || p.Expires != "" {
				t.Fatalf("pinned preview = %+v, want pinned with no expires", p)
			}
		case live:
			if p.Pinned || p.Expires == "" {
				t.Fatalf("live preview = %+v, want unpinned with expires", p)
			}
		}
	}
}

func TestPinPreviewFalseUnpins(t *testing.T) {
	s := newTestStore(t)
	name := publish(t, s, "Keep", "", time.Hour)
	if _, err := pinPreview(s, 2*time.Hour, PinInput{Name: name}); err != nil {
		t.Fatal(err)
	}
	no := false
	out, err := pinPreview(s, 2*time.Hour, PinInput{Name: name, Pinned: &no})
	if err != nil {
		t.Fatal(err)
	}
	if out.Unpinned != name || out.Pinned != "" || out.Expires == "" {
		t.Fatalf("unpin output = %+v", out)
	}
	m, _ := s.Get(name)
	if m.Pinned {
		t.Fatal("manifest still pinned after unpin")
	}
	if time.Until(m.Expires) < time.Hour+30*time.Minute {
		t.Fatalf("unpinned expiry %v should be about the default ttl from now", m.Expires)
	}
}

func TestExtendPinnedPreviewUnpins(t *testing.T) {
	s := newTestStore(t)
	name := publish(t, s, "Keep", "", time.Hour)
	if err := s.Pin(name); err != nil {
		t.Fatal(err)
	}
	out, err := extendPreview(s, ExtendInput{Name: name, TTL: "5h"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Unpinned {
		t.Fatalf("extend output = %+v, want unpinned", out)
	}
	if m, _ := s.Get(name); m.Pinned {
		t.Fatal("manifest still pinned after extend")
	}
	// An unpinned preview is not reported as unpinned by extend.
	out, err = extendPreview(s, ExtendInput{Name: name, TTL: "6h"})
	if err != nil || out.Unpinned {
		t.Fatalf("second extend = %+v, %v", out, err)
	}
}

func TestPresentRejectsRelativePath(t *testing.T) {
	s := newTestStore(t)
	_, err := publishPreview(s, time.Hour, PresentInput{Path: "out/page.html"})
	if err == nil || !strings.Contains(err.Error(), "path must be absolute: out/page.html; glim mcp runs in ") {
		t.Fatalf("err = %v, want absolute-path error", err)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Fatalf("rejected path must not create previews, got %d", len(list))
	}
}

func TestPresentExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "page.html"), []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newTestStore(t)
	if _, err := publishPreview(s, time.Hour, PresentInput{Path: "~/page.html"}); err != nil {
		t.Fatalf("~/ path failed: %v", err)
	}
}

func TestPresentPreviewWarnsWhenServerWillNotStart(t *testing.T) {
	s := newTestStore(t)
	p := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(p, []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, warning, err := presentPreview(s, time.Hour, func() (string, error) { return "", errors.New("boom") }, PresentInput{Path: p})
	if err != nil || res.Name == "" {
		t.Fatalf("publish should still succeed: %v", err)
	}
	if !strings.Contains(warning, "glim serve") || !strings.Contains(warning, "boom") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestPresentPreviewUsesEnsuredBase(t *testing.T) {
	s := newTestStore(t)
	p := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(p, []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, warning, err := presentPreview(s, time.Hour, func() (string, error) { return "http://127.0.0.1:4242", nil }, PresentInput{Path: p})
	if err != nil || warning != "" {
		t.Fatalf("err %v warning %q", err, warning)
	}
	if !strings.HasPrefix(res.URL, "http://127.0.0.1:4242/") {
		t.Fatalf("url = %q", res.URL)
	}
}
