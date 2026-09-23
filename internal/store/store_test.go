package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir(), "https://glim.example.com")
	return s
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPublishFile(t *testing.T) {
	s := newTestStore(t)
	entry := writeTemp(t, "report.html", "<h1>hi</h1>")
	res, err := s.Publish(entry, "Rawkoon audiobooks new feature", "rawkoon", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Name, "rawkoon-audiobooks-new-feature-") {
		t.Fatalf("name %q lacks readable slug", res.Name)
	}
	wantURL := "https://glim.example.com/" + res.Name + "/"
	if res.URL != wantURL {
		t.Fatalf("URL = %q, want %q", res.URL, wantURL)
	}
	got, err := os.ReadFile(filepath.Join(s.Root, res.Name, "index.html"))
	if err != nil || string(got) != "<h1>hi</h1>" {
		t.Fatalf("index.html not copied correctly: %q err=%v", got, err)
	}
}

func TestPublishWithNameUsesExactSlug(t *testing.T) {
	s := newTestStore(t)
	entry := writeTemp(t, "report.html", "<h1>hi</h1>")
	res, err := s.Publish(entry, "Ignored Title", "proj", "", time.Hour, "dashboard-k3n7")
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "dashboard-k3n7" {
		t.Fatalf("Name = %q, want exact %q", res.Name, "dashboard-k3n7")
	}
	if res.URL != "https://glim.example.com/dashboard-k3n7/" {
		t.Fatalf("URL = %q", res.URL)
	}
	got, err := os.ReadFile(filepath.Join(s.Root, "dashboard-k3n7", "index.html"))
	if err != nil || string(got) != "<h1>hi</h1>" {
		t.Fatalf("index.html not copied: %q err=%v", got, err)
	}
}

func TestPublishWithNameUpsertsInPlaceAndDropsStaleFiles(t *testing.T) {
	s := newTestStore(t)

	// First publish: a directory carrying an extra asset.
	dir1 := t.TempDir()
	os.WriteFile(filepath.Join(dir1, "index.html"), []byte("v1"), 0o644)
	os.WriteFile(filepath.Join(dir1, "old.css"), []byte("stale"), 0o644)
	first, err := s.Publish(dir1, "", "", "", time.Hour, "site")
	if err != nil {
		t.Fatal(err)
	}

	// Second publish to the same name: new content, no old.css.
	entry2 := writeTemp(t, "index.html", "v2")
	second, err := s.Publish(entry2, "", "", "", time.Hour, "site")
	if err != nil {
		t.Fatal(err)
	}
	if second.Name != first.Name || second.URL != first.URL {
		t.Fatalf("URL changed on update: %q -> %q", first.URL, second.URL)
	}
	got, _ := os.ReadFile(filepath.Join(s.Root, "site", "index.html"))
	if string(got) != "v2" {
		t.Fatalf("index.html = %q, want v2", got)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "site", "old.css")); !os.IsNotExist(err) {
		t.Fatalf("stale old.css survived update (err=%v)", err)
	}
}

func TestPublishWithNameResetsClock(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }
	e := writeTemp(t, "a.html", "a")
	if _, err := s.Publish(e, "", "", "", time.Hour, "site"); err != nil {
		t.Fatal(err)
	}

	s.Now = func() time.Time { return base.Add(30 * time.Minute) }
	if _, err := s.Publish(e, "", "", "", time.Hour, "site"); err != nil {
		t.Fatal(err)
	}
	m, err := s.Get("site")
	if err != nil {
		t.Fatal(err)
	}
	want := base.Add(30 * time.Minute).Add(time.Hour)
	if !m.Expires.Equal(want) {
		t.Fatalf("Expires = %v, want reset to %v", m.Expires, want)
	}
}

func TestPublishRejectsInvalidName(t *testing.T) {
	s := newTestStore(t)
	e := writeTemp(t, "a.html", "a")
	for _, bad := range []string{"../evil", "a/b", "Bad Name", "."} {
		if _, err := s.Publish(e, "", "", "", time.Hour, bad); err == nil {
			t.Fatalf("Publish with name %q: expected error, got nil", bad)
		}
	}
	// No rejected publish left anything behind under the store root.
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("store root not empty after rejected publishes: %v", entries)
	}
	// And nothing escaped the root via traversal.
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.Root), "evil")); err == nil {
		t.Fatal("../evil escaped the store root")
	}
}

func TestPublishTitleFallsBackToFilename(t *testing.T) {
	s := newTestStore(t)
	entry := writeTemp(t, "diff-review.html", "x")
	res, err := s.Publish(entry, "", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Name, "diff-review-") {
		t.Fatalf("name %q should derive from filename", res.Name)
	}
}

func TestPublishDirRequiresIndex(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "other.html"), []byte("x"), 0o644)
	if _, err := s.Publish(dir, "t", "", "", time.Hour, ""); err == nil {
		t.Fatal("expected error for dir without index.html")
	}
}

func TestPublishDirCopiesTree(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>root</h1>"), 0o644)
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "assets", "app.css"), []byte("body{}"), 0o644)
	res, err := s.Publish(dir, "t", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, res.Name, "assets", "app.css")); err != nil {
		t.Fatalf("nested asset not copied: %v", err)
	}
}

func TestListExcludesExpiredAndSortsNewestFirst(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }

	e := writeTemp(t, "a.html", "a")
	s.Now = func() time.Time { return base } // created at base, expires base+1h
	first, _ := s.Publish(e, "first", "", "", time.Hour, "")

	s.Now = func() time.Time { return base.Add(10 * time.Minute) }
	second, _ := s.Publish(e, "second", "", "", time.Hour, "")

	// Advance so `first` expired but `second` still live.
	s.Now = func() time.Time { return base.Add(70 * time.Minute) }
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("live count = %d, want 1 (%v)", len(list), list)
	}
	if list[0].Name != second.Name {
		t.Fatalf("expected only %q live, got %q", second.Name, list[0].Name)
	}
	_ = first
}

func TestGCRemovesExpired(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }
	e := writeTemp(t, "a.html", "a")
	res, _ := s.Publish(e, "gone", "", "", time.Hour, "")

	s.Now = func() time.Time { return base.Add(2 * time.Hour) }
	n, err := s.GC()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("GC removed %d, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(s.Root, res.Name)); !os.IsNotExist(err) {
		t.Fatalf("expired dir still present")
	}
}

func TestRemoveByName(t *testing.T) {
	s := newTestStore(t)
	e := writeTemp(t, "a.html", "a")
	res, _ := s.Publish(e, "bye", "", "", time.Hour, "")
	if err := s.Remove(res.Name); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("nope-abcd"); err == nil {
		t.Fatal("expected error removing unknown preview")
	}
}

func TestPinSurvivesExpiryAndGC(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }
	e := writeTemp(t, "a.html", "a")
	res, _ := s.Publish(e, "keep", "", "", time.Hour, "")

	if err := s.Pin(res.Name); err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return base.Add(100 * time.Hour) }

	n, err := s.GC()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("GC removed %d pinned previews, want 0", n)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Pinned {
		t.Fatalf("pinned preview not live/pinned: %+v", list)
	}
}

func TestExtendPushesExpiry(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }
	e := writeTemp(t, "a.html", "a")
	res, _ := s.Publish(e, "live", "", "", time.Hour, "")

	s.Now = func() time.Time { return base.Add(30 * time.Minute) }
	if err := s.Extend(res.Name, 2*time.Hour); err != nil {
		t.Fatal(err)
	}

	s.Now = func() time.Time { return base.Add(90 * time.Minute) }
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("extended preview expired early: %+v", list)
	}
}

func TestPinExtendMissing(t *testing.T) {
	s := newTestStore(t)
	if err := s.Pin("nope-abcd"); err == nil {
		t.Fatal("want error pinning unknown preview")
	}
	if err := s.Extend("nope-abcd", time.Hour); err == nil {
		t.Fatal("want error extending unknown preview")
	}
}

func TestGetAndDiskUsage(t *testing.T) {
	s := newTestStore(t)
	e := writeTemp(t, "a.html", "hello world")
	res, _ := s.Publish(e, "info", "proj", "", time.Hour, "")

	m, err := s.Get(res.Name)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != res.Name || m.Project != "proj" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if _, err := s.Get("nope-abcd"); err == nil {
		t.Fatal("want error getting unknown preview")
	}

	n, err := s.DiskUsage()
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Fatalf("disk usage = %d, want > 0", n)
	}
}

func TestLive(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	entry := writeTemp(t, "p.html", "<p>x</p>")
	res, err := s.Publish(entry, "Live test", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Live(res.Name); !ok {
		t.Fatal("fresh preview should be live")
	}
	for _, bad := range []string{"", "..", "../x", "UPPER", "a/b", ".glim.json", "nope-zzzz"} {
		if _, ok := s.Live(bad); ok {
			t.Errorf("Live(%q) = true, want false", bad)
		}
	}
	now = now.Add(2 * time.Hour)
	if _, ok := s.Live(res.Name); ok {
		t.Fatal("expired preview should not be live")
	}
	if err := s.Pin(res.Name); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Live(res.Name); !ok {
		t.Fatal("pinned preview should be live even past expiry")
	}
}
