package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

func publishFile(t *testing.T, s *store.Store, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := s.Publish(p, "T", "proj", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	return res.Name
}

func TestGetConvertedMarkdownReturnsOriginal(t *testing.T) {
	s := newTestStore(t)
	n := publishFile(t, s, "notes.md", "# Heading\n\nbody")
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	out, err := getPreview(s, GetInput{Name: n, IncludeSource: true}, map[string]auth.ViewStat{n: {Count: 3, LastSeen: at}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "# Heading\n\nbody" || out.SourceFile != "notes.md" || out.Files != nil {
		t.Fatalf("out = %+v", out)
	}
	if out.Title != "T" || out.Project != "proj" || out.Views != 3 || out.LastSeen == "" || out.Created == "" || out.Expires == "" || out.URL != s.URL(n) {
		t.Fatalf("metadata = %+v", out)
	}
}

func TestGetDirectoryReturnsIndexAndFileList(t *testing.T) {
	s := newTestStore(t)
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "index.html"), []byte("<h1>dir</h1>"), 0o644)
	os.MkdirAll(filepath.Join(d, "css"), 0o755)
	os.WriteFile(filepath.Join(d, "css", "a.css"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(d, "b.js"), []byte("y"), 0o644)
	res, err := s.Publish(d, "D", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := getPreview(s, GetInput{Name: res.Name, IncludeSource: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "<h1>dir</h1>" || out.SourceFile != "index.html" {
		t.Fatalf("out = %+v", out)
	}
	if strings.Join(out.Files, ",") != "b.js,css/a.css" {
		t.Fatalf("files = %v", out.Files)
	}
}

func TestGetDirectoryFileListIsCapped(t *testing.T) {
	s := newTestStore(t)
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "index.html"), []byte("x"), 0o644)
	for i := 0; i < maxFileList+5; i++ {
		os.WriteFile(filepath.Join(d, fmt.Sprintf("f%03d.txt", i)), []byte("x"), 0o644)
	}
	res, err := s.Publish(d, "D", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := getPreview(s, GetInput{Name: res.Name, IncludeSource: true}, nil)
	if len(out.Files) != maxFileList || !out.FilesTruncated {
		t.Fatalf("files = %d truncated=%v", len(out.Files), out.FilesTruncated)
	}
}

func TestGetImageReturnsNameAndSizeOnly(t *testing.T) {
	s := newTestStore(t)
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 20)
	n := publishFile(t, s, "pic.png", png)
	out, err := getPreview(s, GetInput{Name: n, IncludeSource: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Binary || out.Source != "" || out.SourceFile != "pic.png" || out.SourceSize != int64(len(png)) {
		t.Fatalf("out = %+v", out)
	}
}

func TestGetWithoutSourceOmitsIt(t *testing.T) {
	s := newTestStore(t)
	n := publishFile(t, s, "notes.md", "# hi")
	out, err := getPreview(s, GetInput{Name: n}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "" || out.SourceFile != "" || out.Files != nil {
		t.Fatalf("out = %+v", out)
	}
}

func TestGetTruncatesLargeSource(t *testing.T) {
	s := newTestStore(t)
	n := publishFile(t, s, "big.txt", strings.Repeat("é", maxSourceBytes)) // 2 bytes per rune
	out, err := getPreview(s, GetInput{Name: n, IncludeSource: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated || len(out.Source) > maxSourceBytes || len(out.Source) < maxSourceBytes-1 {
		t.Fatalf("truncated=%v len=%d", out.Truncated, len(out.Source))
	}
	if out.SourceSize != int64(2*maxSourceBytes) {
		t.Fatalf("size = %d", out.SourceSize)
	}
}

func TestGetLockedDoesNotLeakHash(t *testing.T) {
	s := newTestStore(t)
	p := filepath.Join(t.TempDir(), "page.html")
	os.WriteFile(p, []byte("<h1>x</h1>"), 0o644)
	res, err := s.PublishLocked(p, "L", "", "", time.Hour, "", "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012345")
	if err != nil {
		t.Fatal(err)
	}
	out, err := getPreview(s, GetInput{Name: res.Name, IncludeSource: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Locked {
		t.Fatal("locked = false")
	}
	if text := getText(out); strings.Contains(text, "$2a$") || strings.Contains(out.Source, "$2a$") {
		t.Fatalf("hash leaked: %s", text)
	}
	for _, f := range out.Files {
		if strings.Contains(f, ".glim") {
			t.Fatalf("manifest listed: %v", out.Files)
		}
	}
}

func TestGetPinnedHasNoExpiry(t *testing.T) {
	s := newTestStore(t)
	n := publish(t, s, "P", "", time.Hour)
	if err := s.Pin(n); err != nil {
		t.Fatal(err)
	}
	out, err := getPreview(s, GetInput{Name: n}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Pinned || out.Expires != "" {
		t.Fatalf("out = %+v", out)
	}
}

func TestGetUnknownOrExpiredSlug(t *testing.T) {
	s := newTestStore(t)
	for _, name := range []string{"nope", "../etc", ""} {
		if _, err := getPreview(s, GetInput{Name: name}, nil); err == nil || !strings.Contains(err.Error(), "no such preview") {
			t.Fatalf("%q: err = %v", name, err)
		}
	}
	n := publish(t, s, "Gone", "", time.Hour)
	s.Now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := getPreview(s, GetInput{Name: n}, nil); err == nil || !strings.Contains(err.Error(), "no such preview") {
		t.Fatalf("expired: err = %v", err)
	}
}

func TestPresentSummaryHasNameAndExpiry(t *testing.T) {
	exp := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	got := presentSummary(store.PublishResult{Name: "my-page-abc", Expires: exp})
	if got != "name: my-page-abc · expires: 2026-03-04T05:06:07Z" {
		t.Fatalf("summary = %q", got)
	}
	if got := presentSummary(store.PublishResult{Name: "x", Expires: exp, Pinned: true}); got != "name: x · pinned" {
		t.Fatalf("pinned summary = %q", got)
	}
}
