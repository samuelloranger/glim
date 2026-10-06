package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

func TestInjectHTMLPositions(t *testing.T) {
	cases := []struct{ name, doc, want string }{
		{"head", "<html><head><title>x</title></head><body>b</body></html>", "<html><head><title>x</title>H</head><body>bB</body></html>"},
		{"uppercase", "<HTML><HEAD></HEAD><BODY>b</BODY></HTML>", "<HTML><HEAD>H</HEAD><BODY>bB</BODY></HTML>"},
		{"no head", "<html lang=en><p>hi</p>", "<html lang=en>H<p>hi</p>B"},
		{"no html", "<p>hi</p>", "H<p>hi</p>B"},
		{"spaced close", "<head></head ><body></body >", "<head>H</head ><body>B</body >"},
	}
	for _, c := range cases {
		if got := string(injectHTML([]byte(c.doc), "H", "B")); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	if got := string(injectHTML([]byte("<head></head>"), "", "")); got != "<head></head>" {
		t.Errorf("empty snippets changed doc: %q", got)
	}
}

func TestDeclaredMeta(t *testing.T) {
	m := declaredMeta([]byte(`<meta property="og:title" content="x"><META NAME='Twitter:Card' content=a><meta charset=utf-8><meta property=og:image content=y>`))
	for _, k := range []string{"og:title", "twitter:card", "og:image"} {
		if !m[k] {
			t.Errorf("missing %s in %v", k, m)
		}
	}
	if m["og:url"] {
		t.Error("og:url should not be declared")
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newPreviewStore(t *testing.T) *store.Store {
	st := store.New(t.TempDir(), "https://glim.example.com")
	now := time.Now()
	st.Now = func() time.Time { return now }
	publish(t, st, "demo-1234", map[string]string{
		"index.html": "<html><head><title>t</title></head><body>hello</body></html>",
		"page.html":  `<html><head><meta property="og:title" content="Mine"></head><body></body></html>`,
		"app.js":     "console.log(1)",
		"big.html":   "<html><head></head>" + strings.Repeat("x", maxInjectSize+1) + "</html>",
	}, time.Hour)
	return st
}

func TestPreviewInjectsCards(t *testing.T) {
	h := PreviewHandler(newPreviewStore(t))
	body := get(t, h, "/demo-1234/").Body.String()
	for _, w := range []string{
		`<meta property="og:title" content="demo-1234">`,
		`og:description" content="expires in 1 hour"`,
		`og:site_name" content="glim"`,
		`og:type" content="website"`,
		`og:url" content="https://glim.example.com/demo-1234/"`,
		`og:image" content="https://glim.example.com/_glim/og.png"`,
		`<meta name="twitter:card" content="summary">`,
	} {
		if !strings.Contains(body, w) {
			t.Errorf("missing %s in %s", w, body)
		}
	}
	if !strings.Contains(body, "</title><meta property=\"og:title\"") || !strings.HasSuffix(body, "hello</body></html>") {
		t.Errorf("unexpected layout: %s", body)
	}
}

func TestPreviewDoesNotDuplicatePageTags(t *testing.T) {
	h := PreviewHandler(newPreviewStore(t))
	body := get(t, h, "/demo-1234/page.html").Body.String()
	if n := strings.Count(body, `og:title`); n != 1 {
		t.Errorf("og:title appears %d times: %s", n, body)
	}
	if !strings.Contains(body, `og:site_name`) {
		t.Error("other tags should still be injected")
	}
}

func TestPreviewTitleProjectAndPinned(t *testing.T) {
	st := store.New(t.TempDir(), "")
	src := t.TempDir()
	writeFile(t, src, "index.html", "<html><head></head></html>")
	if _, err := st.Publish(src, "My <Title>", "proj", "", time.Hour, "pin-1234"); err != nil {
		t.Fatal(err)
	}
	if err := st.Pin("pin-1234"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pin-1234/", nil)
	req.Host = "example.com"
	PreviewHandler(st).ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, w := range []string{`content="My &lt;Title&gt;"`, `content="proj · pinned"`, `og:url" content="http://example.com/pin-1234/"`} {
		if !strings.Contains(body, w) {
			t.Errorf("missing %s in %s", w, body)
		}
	}
}

func TestPreviewLeavesNonHTMLAndLargeUntouched(t *testing.T) {
	h := PreviewHandler(newPreviewStore(t))
	if b := get(t, h, "/demo-1234/app.js").Body.String(); b != "console.log(1)" {
		t.Errorf("js = %q", b)
	}
	if b := get(t, h, "/demo-1234/big.html").Body.String(); strings.Contains(b, "og:") {
		t.Error("oversized html was injected")
	}
}

func TestPreviewHeadAndRange(t *testing.T) {
	h := PreviewHandler(newPreviewStore(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/demo-1234/", nil))
	if rec.Code != 200 || rec.Body.Len() != 0 || rec.Header().Get("Content-Length") == "" || rec.Header().Get("Last-Modified") == "" {
		t.Errorf("HEAD: code=%d len=%d headers=%v", rec.Code, rec.Body.Len(), rec.Header())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type = %q", ct)
	}
	req := httptest.NewRequest(http.MethodGet, "/demo-1234/", nil)
	req.Header.Set("Range", "bytes=0-5")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "<html>" {
		t.Errorf("range: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestPreviewRobotsHeaderEverywhere(t *testing.T) {
	h := PreviewHandler(newPreviewStore(t))
	for _, p := range []string{"/demo-1234/", "/demo-1234/app.js", "/demo-1234/page.html", "/demo-1234/missing", "/demo-1234/.glim.json", "/nope-9999/", "/"} {
		if got := get(t, h, p).Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
			t.Errorf("%s X-Robots-Tag = %q", p, got)
		}
	}
}

func TestRouterRobotsAndOGImage(t *testing.T) {
	h := NewRouter(Options{Store: newPreviewStore(t), Web: http.NotFoundHandler()})
	rec := get(t, h, "/robots.txt")
	if rec.Code != 200 || rec.Body.String() != "User-agent: *\nDisallow: /\n" {
		t.Errorf("robots: %d %q", rec.Code, rec.Body.String())
	}
	rec = get(t, h, "/_glim/og.png")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("og: %d %v", rec.Code, rec.Header())
	}
	if n := rec.Body.Len(); n == 0 || n > 50*1024 {
		t.Errorf("og.png size = %d", n)
	}
	if b := rec.Body.Bytes(); string(b[1:4]) != "PNG" {
		t.Error("not a PNG")
	}
}

func TestHumanLeft(t *testing.T) {
	cases := map[time.Duration]string{
		-time.Second:     "expires soon",
		30 * time.Second: "expires in under a minute",
		time.Minute:      "expires in 1 minute",
		90 * time.Minute: "expires in 1 hour",
		5 * time.Hour:    "expires in 5 hours",
		72 * time.Hour:   "expires in 3 days",
	}
	for d, want := range cases {
		if got := humanLeft(d); got != want {
			t.Errorf("%v: %q want %q", d, got, want)
		}
	}
}
