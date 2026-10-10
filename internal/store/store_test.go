package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestOnRemoveFiresForRemoveAndGCButNotRepublish(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }
	var got []string
	s.OnRemove = func(name string) { got = append(got, name) }
	e := writeTemp(t, "a.html", "a")
	if _, err := s.Publish(e, "t", "", "", time.Hour, "keep-me"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(e, "t", "", "", time.Hour, "keep-me"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("republish fired OnRemove: %v", got)
	}
	if err := s.Remove("keep-me"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(e, "t", "", "", time.Hour, "expires"); err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return base.Add(2 * time.Hour) }
	if _, err := s.GC(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "keep-me" || got[1] != "expires" {
		t.Fatalf("OnRemove calls = %v", got)
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
	if err := s.Pin(res.Name); err == nil {
		t.Fatal("pinning an expired preview should fail")
	}
	now = now.Add(-90 * time.Minute)
	if err := s.Pin(res.Name); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Hour)
	if _, ok := s.Live(res.Name); !ok {
		t.Fatal("pinned preview should be live even past expiry")
	}
}

func TestFingerprintTracksChanges(t *testing.T) {
	s := newTestStore(t)
	empty, err := s.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	entry := writeTemp(t, "p.html", "x")
	res, _ := s.Publish(entry, "Fp", "", "", time.Hour, "")
	one, _ := s.Fingerprint()
	if one == empty {
		t.Fatal("publish did not change fingerprint")
	}
	if again, _ := s.Fingerprint(); again != one {
		t.Fatal("fingerprint not stable")
	}
	time.Sleep(5 * time.Millisecond)
	s.Pin(res.Name)
	pinned, _ := s.Fingerprint()
	if pinned == one {
		t.Fatal("manifest rewrite did not change fingerprint")
	}
	s.Remove(res.Name)
	if gone, _ := s.Fingerprint(); gone != empty {
		t.Fatal("remove should restore the empty fingerprint")
	}
}

func TestPublishRefusals(t *testing.T) {
	tmp := t.TempDir()
	write := func(rel, c string) string {
		p := filepath.Join(tmp, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	real := write("real.html", "x")
	link := filepath.Join(tmp, "link.html")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	dot := write(".hidden.html", "x")
	key := write("id_rsa", "secret")
	exe := write("run.sh", "x")
	symDir := filepath.Join(tmp, "symdir")
	write("symdir/index.html", "x")
	os.Symlink(key, filepath.Join(symDir, "leak.txt"))
	bigDir := filepath.Join(tmp, "big")
	write("big/index.html", "x")
	for i := 0; i < MaxPublishFiles; i++ {
		write(filepath.Join("big", "f", fmt.Sprintf("%d.txt", i)), "")
	}

	cases := []struct {
		name, entry, want string
	}{
		{"symlink entry", link, "symlink"},
		{"dotfile entry", dot, "hidden"},
		{"extensionless key", key, "only"},
		{"disallowed extension", exe, "only"},
		{"symlink inside dir", symDir, "symlink"},
		{"file count cap", bigDir, "too large"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newTestStore(t)
			_, err := s.Publish(c.entry, "", "", "", time.Hour, "")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want containing %q", err, c.want)
			}
			ms, _ := s.List()
			if len(ms) != 0 {
				t.Fatalf("refused publish left a preview: %v", ms)
			}
			entries, _ := os.ReadDir(s.Root)
			if len(entries) != 0 {
				t.Fatalf("refused publish left residue in store root: %v", entries)
			}
		})
	}
}

func TestPublishSizeCap(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644)
	f, _ := os.Create(filepath.Join(dir, "blob.bin"))
	f.Truncate(MaxPublishBytes + 1)
	f.Close()
	s := newTestStore(t)
	if _, err := s.Publish(dir, "", "", "", time.Hour, ""); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v", err)
	}
}

func TestPublishDirSkipsDotEntries(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET"), 0o600)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub", ".ssh"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", ".ssh", "k"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "a.css"), []byte("x"), 0o644)
	s := newTestStore(t)
	res, err := s.Publish(dir, "", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(s.Root, res.Name)
	for _, p := range []string{".env", ".git", filepath.Join("sub", ".ssh")} {
		if _, err := os.Lstat(filepath.Join(root, p)); err == nil {
			t.Fatalf("%s was published", p)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "a.css")); err != nil {
		t.Fatal(err)
	}
}

func TestRepublishAtomic(t *testing.T) {
	s := newTestStore(t)
	good := t.TempDir()
	os.WriteFile(filepath.Join(good, "index.html"), []byte("v1"), 0o644)
	if _, err := s.Publish(good, "", "", "", time.Hour, "site"); err != nil {
		t.Fatal(err)
	}

	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, "index.html"), []byte("v2"), 0o644)
	os.Symlink("/etc/passwd", filepath.Join(bad, "leak"))
	if _, err := s.Publish(bad, "", "", "", time.Hour, "site"); err == nil {
		t.Fatal("expected failure")
	}
	got, err := os.ReadFile(filepath.Join(s.Root, "site", "index.html"))
	if err != nil || string(got) != "v1" {
		t.Fatalf("old contents lost: %q %v", got, err)
	}
	if _, err := s.Get("site"); err != nil {
		t.Fatalf("manifest lost: %v", err)
	}
	entries, _ := os.ReadDir(s.Root)
	if len(entries) != 1 {
		t.Fatalf("residue: %v", entries)
	}

	os.Remove(filepath.Join(bad, "leak"))
	if _, err := s.Publish(bad, "", "", "", time.Hour, "site"); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(filepath.Join(s.Root, "site", "index.html"))
	if string(got) != "v2" {
		t.Fatalf("not replaced: %q", got)
	}
	entries, _ = os.ReadDir(s.Root)
	if len(entries) != 1 {
		t.Fatalf("residue after swap: %v", entries)
	}
}

func TestHiddenDirsIgnoredAndStaleOnesGCd(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	s.Now = func() time.Time { return now }
	mk := func(name string, age time.Duration) string {
		d := filepath.Join(s.Root, name)
		os.MkdirAll(d, 0o755)
		writeManifest(d, Manifest{Name: name, Created: now, Expires: now.Add(time.Hour)})
		os.WriteFile(filepath.Join(d, "f"), []byte("12345"), 0o644)
		os.Chtimes(d, now.Add(-age), now.Add(-age))
		return d
	}
	fresh := mk(".tmp-a-1", time.Minute)
	stale := mk(".tmp-b-2", 2*time.Hour)
	staleOld := mk(".old-c-3", 2*time.Hour)
	if ms, _ := s.List(); len(ms) != 0 {
		t.Fatalf("List included hidden dirs: %v", ms)
	}
	if n, _ := s.DiskUsage(); n != 0 {
		t.Fatalf("DiskUsage counted hidden dirs: %d", n)
	}
	if fp, _ := s.Fingerprint(); fp == "" {
		t.Fatal("empty fingerprint")
	}
	s.GC()
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh tmp removed")
	}
	for _, p := range []string{stale, staleOld} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s not collected", p)
		}
	}
}

func TestLongTitleNameIsLive(t *testing.T) {
	s := newTestStore(t)
	entry := writeTemp(t, "a.html", "<p>x</p>")
	res, err := s.Publish(entry, strings.Repeat("word ", 20), "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if !ValidName(res.Name) {
		t.Fatalf("name %q (len %d) fails ValidName", res.Name, len(res.Name))
	}
	if _, ok := s.Live(res.Name); !ok {
		t.Fatal("long-titled preview is not live")
	}
	if err := s.Pin(res.Name); err != nil {
		t.Fatal(err)
	}
}

func TestRepublishKeepsPinTitleProject(t *testing.T) {
	for _, hash := range []string{"", "$2a$10$newhashnewhashnewhashnewhashnewhashnewhashnewhash"} {
		s := newTestStore(t)
		entry := writeTemp(t, "a.html", "a")
		if _, err := s.Publish(entry, "Title", "proj", "", time.Hour, "keep-abcd"); err != nil {
			t.Fatal(err)
		}
		if err := s.Pin("keep-abcd"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PublishLocked(entry, "", "", "", time.Hour, "keep-abcd", hash); err != nil {
			t.Fatal(err)
		}
		m, _ := s.Get("keep-abcd")
		if !m.Pinned || m.Title != "Title" || m.Project != "proj" {
			t.Errorf("hash %q: republish lost metadata: %+v", hash, m)
		}
		if _, err := s.PublishLocked(entry, "New", "other", "", time.Hour, "keep-abcd", hash); err != nil {
			t.Fatal(err)
		}
		m, _ = s.Get("keep-abcd")
		if m.Title != "New" || m.Project != "other" {
			t.Errorf("hash %q: explicit title/project not applied: %+v", hash, m)
		}
	}
}

func TestPinExtendRefuseExpired(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }
	res, _ := s.Publish(writeTemp(t, "a.html", "a"), "x", "", "", time.Hour, "")
	s.Now = func() time.Time { return base.Add(2 * time.Hour) }
	if err := s.Extend(res.Name, time.Hour); err == nil || !strings.Contains(err.Error(), "no such preview") {
		t.Fatalf("Extend on expired = %v, want no such preview", err)
	}
	if err := s.Pin(res.Name); err == nil || !strings.Contains(err.Error(), "no such preview") {
		t.Fatalf("Pin on expired = %v, want no such preview", err)
	}
	if _, ok := s.Live(res.Name); ok {
		t.Fatal("expired preview was revived")
	}
}

func TestExtendCapsTTL(t *testing.T) {
	s := newTestStore(t)
	res, _ := s.Publish(writeTemp(t, "a.html", "a"), "x", "", "", time.Hour, "")
	if err := s.Extend(res.Name, MaxTTL+time.Second); err == nil {
		t.Fatal("want error beyond MaxTTL")
	}
	if err := s.Extend(res.Name, MaxTTL); err != nil {
		t.Fatal(err)
	}
	if err := s.Extend(res.Name, 0); err == nil {
		t.Fatal("want error for zero ttl")
	}
}

// An extend racing GC over an expired-looking preview must never lose the
// extension: either the extend fails (GC won) or the preview survives.
func TestGCNeverRemovesConcurrentlyExtended(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 30; i++ {
		s := newTestStore(t)
		s.Now = func() time.Time { return base }
		res, _ := s.Publish(writeTemp(t, "a.html", "a"), "x", "", "", time.Hour, "")
		s.Now = func() time.Time { return base.Add(time.Hour + time.Second) }
		var wg sync.WaitGroup
		var extendErr error
		wg.Add(2)
		go func() { defer wg.Done(); extendErr = s.Extend(res.Name, time.Hour) }()
		go func() { defer wg.Done(); _, _ = s.GC() }()
		wg.Wait()
		_, live := s.Live(res.Name)
		if extendErr == nil && !live {
			t.Fatal("extend succeeded but GC removed the preview")
		}
	}
}

// GC must re-check expiry under the slug lock: a preview extended after the
// directory scan but before removal survives.
func TestReapExpiredRechecksUnderLock(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := newTestStore(t)
	s.Now = func() time.Time { return base }
	res, _ := s.Publish(writeTemp(t, "a.html", "a"), "x", "", "", time.Hour, "")
	stale := base.Add(2 * time.Hour) // the scan's view of "now"
	s.Now = func() time.Time { return base.Add(30 * time.Minute) }
	if err := s.Extend(res.Name, 3*time.Hour); err != nil {
		t.Fatal(err)
	}
	if s.reapExpired(res.Name, stale) {
		t.Fatal("reaped a preview that was extended past the scan time")
	}
	if _, ok := s.Live(res.Name); !ok {
		t.Fatal("extended preview lost")
	}
}

func TestManifestWriteIsAtomic(t *testing.T) {
	s := newTestStore(t)
	res, _ := s.Publish(writeTemp(t, "a.html", "a"), "x", "", "", time.Hour, "")
	dir := filepath.Join(s.Root, res.Name)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.Extend(res.Name, time.Hour)
			}
		}
	}()
	for i := 0; i < 2000; i++ {
		if _, err := readManifest(dir); err != nil {
			close(stop)
			<-done
			t.Fatalf("reader saw a partial manifest: %v", err)
		}
	}
	close(stop)
	<-done
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), manifestTmpPrefix) {
			t.Errorf("temp manifest left behind: %s", e.Name())
		}
	}
}
