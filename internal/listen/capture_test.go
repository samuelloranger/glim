package listen

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writePage builds a preview folder with a page, a script-rendered line, a
// chart image and a hidden dot folder that must never be served.
func writePage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.RGBA{uint8(x / 2), 80, 200, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	must(t, os.WriteFile(filepath.Join(dir, "chart.png"), buf.Bytes(), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".glim-listen"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".glim-listen", "secret.txt"), []byte("hidden"), 0o644))
	page := `<!doctype html><html><head><title>Quarterly report</title>
<style>body{font:18px sans-serif;margin:40px} .tall{height:2400px}</style></head><body>
<h1>Revenue grew</h1><p>Revenue rose 12 percent in the third quarter.</p>
<figure><img src="chart.png" alt="Revenue by month"><figcaption>Monthly revenue</figcaption></figure>
<div class="tall"></div><p id="js"></p>
<script>document.getElementById("js").textContent = "Rendered by script.";</script>
</body></html>`
	must(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte(page), 0o644))
	return dir
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestStaticPage(t *testing.T) {
	dir := writePage(t)
	p, err := StaticPage(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Quarterly report" || p.Rendered {
		t.Fatalf("title %q rendered %v", p.Title, p.Rendered)
	}
	if !strings.Contains(p.Text, "Revenue rose 12 percent") || !strings.Contains(p.Text, "[image: Revenue by month]") {
		t.Fatalf("text = %q", p.Text)
	}
	if strings.Contains(p.Text, "getElementById") || strings.Contains(p.Text, "font:18px") {
		t.Fatalf("script or style leaked into text: %q", p.Text)
	}
	if len(p.Images) != 1 || p.Images[0].MIME != "image/png" || !strings.Contains(p.Images[0].Label, "Revenue by month") {
		t.Fatalf("images = %+v", p.Images)
	}
}

func TestLoadImageStaysInside(t *testing.T) {
	dir := writePage(t)
	for _, src := range []string{"../chart.png", "/etc/passwd", "https://example.com/a.png", ".glim-listen/x.png", "chart.svg", ""} {
		if _, ok := loadImage(dir, src); ok {
			t.Errorf("loadImage(%q) accepted", src)
		}
	}
	if _, ok := loadImage(dir, "./chart.png?v=2"); !ok {
		t.Error("relative path with a query was refused")
	}
}

func TestRenderPage(t *testing.T) {
	chrome := FindChrome(os.Getenv("GLIM_CHROME"))
	if chrome == "" {
		t.Skip("no Chrome or Chromium on PATH")
	}
	dir := writePage(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p, err := RenderPage(ctx, chrome, dir, "report-ab12")
	var start *StartError
	if errors.As(err, &start) {
		t.Skipf("the browser on this machine doesn't start: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !p.Rendered || p.Title != "Quarterly report" {
		t.Fatalf("rendered %v title %q", p.Rendered, p.Title)
	}
	if !strings.Contains(p.Text, "Rendered by script.") {
		t.Fatalf("script output missing from text: %q", p.Text)
	}
	// A 2.4k px page is two screenshot tiles, plus the chart close-up.
	if len(p.Images) != 3 {
		t.Fatalf("got %d pictures: %v", len(p.Images), labels(p.Images))
	}
	if !strings.Contains(p.Images[2].Label, "Monthly revenue") {
		t.Fatalf("close-up label = %q", p.Images[2].Label)
	}
	for _, img := range p.Images {
		if _, _, err := image.DecodeConfig(bytes.NewReader(img.Data)); err != nil && !bytes.HasPrefix(img.Data, []byte{0xFF, 0xD8}) {
			t.Fatalf("%s is not a JPEG", img.Label)
		}
	}
}

func labels(imgs []Image) []string {
	var out []string
	for _, i := range imgs {
		out = append(out, i.Label)
	}
	return out
}

func TestDescribeChrome(t *testing.T) {
	if got := DescribeChrome(""); got != "" {
		t.Errorf("no browser = %q", got)
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "chrome")
	must(t, os.WriteFile(fake, []byte("#!/bin/sh\necho 'Google Chrome for Testing 155.0.1.2'\n"), 0o755))
	if got, want := DescribeChrome(fake), fake+" (Google Chrome for Testing 155.0.1.2)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	broken := filepath.Join(dir, "broken")
	must(t, os.WriteFile(broken, []byte("#!/bin/sh\nexit 1\n"), 0o755))
	if got := DescribeChrome(broken); got != broken {
		t.Errorf("failing browser = %q, want the path alone", got)
	}
}
