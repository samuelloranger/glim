package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

const chromeUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120 Safari/537.36"

func TestIsBotAgent(t *testing.T) {
	for _, ua := range []string{
		"Googlebot/2.1", "facebookexternalhit/1.1", "Slackbot-LinkExpanding 1.0", "WhatsApp/2.23",
		"TelegramBot (like TwitterBot)", "Mozilla/5.0 (compatible; Discordbot/2.0)", "curl/8.4.0",
		"Wget/1.21", "python-requests/2.31", "Go-http-client/1.1", "Embedly/0.2", "Yahoo! Slurp",
		"SomeCrawler", "WebSpider", "LinkPreview", "BINGBOT",
	} {
		if !IsBotAgent(ua) {
			t.Errorf("%q not detected as bot", ua)
		}
	}
	if IsBotAgent(chromeUA) || IsBotAgent("") {
		t.Error("browser or empty UA flagged as bot")
	}
}

func req(method string, hdr map[string]string) *http.Request {
	r := httptest.NewRequest(method, "/demo-1234/", nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return r
}

func TestCountsAsView(t *testing.T) {
	cases := []struct {
		name   string
		method string
		hdr    map[string]string
		want   bool
	}{
		{"document nav", "GET", map[string]string{"Sec-Fetch-Dest": "document", "User-Agent": chromeUA}, true},
		{"iframe thumbnail", "GET", map[string]string{"Sec-Fetch-Dest": "iframe", "User-Agent": chromeUA}, false},
		{"script", "GET", map[string]string{"Sec-Fetch-Dest": "script", "User-Agent": chromeUA}, false},
		{"iframe with html accept", "GET", map[string]string{"Sec-Fetch-Dest": "iframe", "Accept": "text/html", "User-Agent": chromeUA}, false},
		{"no dest, html accept", "GET", map[string]string{"Accept": "text/html,application/xhtml+xml", "User-Agent": chromeUA}, true},
		{"no dest, no accept", "GET", map[string]string{"User-Agent": chromeUA}, false},
		{"no dest, json accept", "GET", map[string]string{"Accept": "application/json", "User-Agent": chromeUA}, false},
		{"HEAD document", "HEAD", map[string]string{"Sec-Fetch-Dest": "document", "User-Agent": chromeUA}, false},
		{"bot document", "GET", map[string]string{"Sec-Fetch-Dest": "document", "User-Agent": "Slackbot"}, false},
		{"bot html accept", "GET", map[string]string{"Accept": "text/html", "User-Agent": "curl/8"}, false},
	}
	for _, c := range cases {
		if got := CountsAsView(req(c.method, c.hdr)); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

type counter struct{ names []string }

func (c *counter) record(n string) { c.names = append(c.names, n) }

func trackedHandler(t *testing.T) (http.Handler, *counter, *auth.DB) {
	t.Helper()
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<h1>demo</h1>", "app.js": "1"}, time.Hour)
	db, err := auth.Open(t.TempDir() + "/glim.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := &counter{}
	return PreviewHandlerWith(st, &Views{Record: c.record, IsOwner: db.IsOwnerToken}), c, db
}

func do(h http.Handler, path string, hdr map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("User-Agent", chromeUA)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestPreviewHandlerCountsOnlyRealOpens(t *testing.T) {
	h, c, _ := trackedHandler(t)
	doc := map[string]string{"Sec-Fetch-Dest": "document"}

	do(h, "/demo-1234/", doc)
	if len(c.names) != 1 || c.names[0] != "demo-1234" {
		t.Fatalf("document open: %v", c.names)
	}
	do(h, "/demo-1234/", map[string]string{"Sec-Fetch-Dest": "iframe"})
	do(h, "/demo-1234/app.js", doc) // asset, not an HTML page
	do(h, "/demo-1234/app.js", map[string]string{"Sec-Fetch-Dest": "script"})
	do(h, "/demo-1234/", map[string]string{"Sec-Fetch-Dest": "document", "User-Agent": "Slackbot-LinkExpanding"})
	do(h, "/nope-0000/", doc) // unknown preview
	if len(c.names) != 1 {
		t.Fatalf("non-opens were counted: %v", c.names)
	}
	do(h, "/demo-1234/", map[string]string{"Accept": "text/html"}) // no Sec-Fetch-Dest
	if len(c.names) != 2 {
		t.Fatalf("accept fallback not counted: %v", c.names)
	}
}

func TestPreviewHandlerSkipsOwner(t *testing.T) {
	h, c, db := trackedHandler(t)
	doc := map[string]string{"Sec-Fetch-Dest": "document"}
	tok, err := db.OwnerToken(1)
	if err != nil {
		t.Fatal(err)
	}
	do(h, "/demo-1234/", doc, &http.Cookie{Name: auth.OwnerCookie, Value: tok})
	if len(c.names) != 0 {
		t.Fatalf("owner open counted: %v", c.names)
	}
	do(h, "/demo-1234/", doc, &http.Cookie{Name: auth.OwnerCookie, Value: "1.deadbeef"})
	do(h, "/demo-1234/", doc, &http.Cookie{Name: auth.OwnerCookie, Value: "garbage"})
	do(h, "/demo-1234/", doc, &http.Cookie{Name: "other", Value: tok})
	if len(c.names) != 3 {
		t.Fatalf("invalid or misnamed owner cookie should count: %v", c.names)
	}
}

func TestPreviewHandlerWithoutViewsStillServes(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<h1>x</h1>"}, time.Hour)
	if rec := do(PreviewHandlerWith(st, nil), "/demo-1234/", map[string]string{"Sec-Fetch-Dest": "document"}); rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
}
