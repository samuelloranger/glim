package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeIn(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func TestPublishMarkdownCopiesSiblingAssets(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "proj")
	writeIn(t, src, "report.md", "# R\n\n![a](shot.png)\n\n![b](img/deep%20one.png?x=1#f)\n\n[d](data.csv)\n\n![gone](missing.png)\n\n![up](../secret.png)\n\n![abs]("+filepath.Join(root, "abs.png")+")\n\n![ext](https://example.com/x.png)\n\n![dir](img)\n\n![c](chart%3Av2.png)\n")
	writeIn(t, src, "shot.png", "PNG1")
	writeIn(t, src, "img/deep one.png", "PNG2")
	writeIn(t, src, "data.csv", "a,b")
	writeIn(t, src, "chart:v2.png", "PNG3")
	writeIn(t, src, "unreferenced.txt", "nope")
	writeIn(t, root, "secret.png", "SECRET")
	writeIn(t, root, "abs.png", "ABS")

	s := newTestStore(t)
	res, err := s.Publish(filepath.Join(src, "report.md"), "", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	pub := filepath.Join(s.Root, res.Name)
	for rel, want := range map[string]string{"shot.png": "PNG1", "img/deep one.png": "PNG2", "data.csv": "a,b", "chart:v2.png": "PNG3"} {
		if b, err := os.ReadFile(filepath.Join(pub, rel)); err != nil || string(b) != want {
			t.Errorf("%s = %q (%v), want %q", rel, b, err, want)
		}
	}
	for _, rel := range []string{"unreferenced.txt", "secret.png", "../secret.png", "abs.png", "missing.png"} {
		if exists(filepath.Join(pub, rel)) {
			t.Errorf("%s must not be published", rel)
		}
	}
	if fi, err := os.Lstat(filepath.Join(pub, "img")); err != nil || !fi.IsDir() {
		t.Errorf("img should exist only as the parent of the copied file")
	}
	if entries, _ := os.ReadDir(s.Root); len(entries) != 1 {
		t.Errorf("stray files next to the preview: %v", entries)
	}
}

func TestPublishHTMLCopiesSiblingAssets(t *testing.T) {
	src := t.TempDir()
	writeIn(t, src, "page.html", `<link href="style.css"><img src='a/b.png'><script src=app.js></script><a href="https://x.test/y">x</a><a href="/etc/passwd">p</a>`)
	writeIn(t, src, "style.css", "body{}")
	writeIn(t, src, "a/b.png", "B")
	writeIn(t, src, "app.js", "1")
	s := newTestStore(t)
	res, err := s.Publish(filepath.Join(src, "page.html"), "", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"style.css", "a/b.png", "app.js"} {
		if !exists(filepath.Join(s.Root, res.Name, rel)) {
			t.Errorf("%s not copied", rel)
		}
	}
}

func TestSiblingAssetsSkipSymlinks(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "p")
	writeIn(t, src, "r.md", "![a](link.png)\n\n![b](linkdir/x.png)\n")
	writeIn(t, root, "outside.png", "OUT")
	writeIn(t, root, "outdir/x.png", "OUT")
	if err := os.Symlink(filepath.Join(root, "outside.png"), filepath.Join(src, "link.png")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.Symlink(filepath.Join(root, "outdir"), filepath.Join(src, "linkdir")); err != nil {
		t.Fatal(err)
	}
	s := newTestStore(t)
	res, err := s.Publish(filepath.Join(src, "r.md"), "", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"link.png", "linkdir"} {
		if exists(filepath.Join(s.Root, res.Name, rel)) {
			t.Errorf("symlink %s was followed", rel)
		}
	}
}

func TestSiblingAssetsSizeCaps(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeIn(t, src, "ok.bin", "x")
	for name, size := range map[string]int64{"big.bin": maxAssetBytes + 1, "a.bin": maxAssetBytes, "b.bin": maxAssetBytes, "c.bin": maxAssetBytes, "d.bin": maxAssetBytes} {
		f, err := os.Create(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	page := `<a href="big.bin"><a href="ok.bin"><a href="a.bin"><a href="b.bin"><a href="c.bin"><a href="d.bin">`
	copySiblingAssets(src, dst, []byte(page))
	if exists(filepath.Join(dst, "big.bin")) {
		t.Error("oversized file copied")
	}
	if !exists(filepath.Join(dst, "ok.bin")) {
		t.Error("small file after an oversized one must still be copied")
	}
	var n int
	var total int64
	for _, name := range []string{"a.bin", "b.bin", "c.bin", "d.bin"} {
		if fi, err := os.Lstat(filepath.Join(dst, name)); err == nil {
			n++
			total += fi.Size()
		}
	}
	if total > maxAssetTotal || n != 3 {
		t.Errorf("copied %d files, %d bytes; want 3 within %d", n, total, maxAssetTotal)
	}
}

func TestLocalRef(t *testing.T) {
	good := map[string]string{"a.png": "a.png", "./a/../b.png": "b.png", "a%20b.png?v=1": "a b.png", "d/x.csv#row": "d/x.csv"}
	for in, want := range good {
		got, ok := localRef(in)
		if in == "chart:v2.png" {
			if ok {
				t.Errorf("%q has a scheme and must be rejected", in)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("localRef(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "/a", "//h/a", "../a", "a/../../b", "https://x/y", "mailto:a@b", "#top", ".hidden", "d/.env", `a\b`, "."} {
		if got, ok := localRef(in); ok {
			t.Errorf("localRef(%q) = %q, want rejected", in, got)
		}
	}
}
