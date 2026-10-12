package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/qrcode"
	"github.com/samuelloranger/glim/internal/store"
)

const barPage = `<html><head><title>T</title></head><body><p>hello</p></body></html>`

type barEnv struct {
	st    *store.Store
	db    *auth.DB
	owner *http.Cookie
	slug  string
	opts  Options
}

func newBarEnv(t *testing.T) *barEnv {
	t.Helper()
	e := &barEnv{slug: "demo-1234"}
	e.st = store.New(t.TempDir(), "https://glim.example.com")
	db, err := auth.Open(filepath.Join(t.TempDir(), "glim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e.db = db
	tok, _ := ownerSession(t, db)
	e.owner = &http.Cookie{Name: auth.OwnerCookie, Value: tok}
	e.opts = Options{Store: e.st, Views: &Views{IsOwner: db.IsOwnerToken}}
	return e
}

func (e *barEnv) publish(t *testing.T, title string, noHeader bool, files map[string]string) {
	t.Helper()
	src := t.TempDir()
	for rel, body := range files {
		writeTestFile(t, filepath.Join(src, rel), body)
	}
	if _, err := e.st.PublishFull(src, title, "", "", time.Hour, e.slug, "", noHeader); err != nil {
		t.Fatal(err)
	}
}

func (e *barEnv) do(path string, hdr map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	NewRouter(e.opts).ServeHTTP(rec, r)
	return rec
}

func hasBar(rec *httptest.ResponseRecorder) bool {
	return strings.Contains(rec.Body.String(), `"data-glim-bar"`)
}

func TestOwnerBarOwnerGetsBarVisitorDoesNot(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, "My Report", false, map[string]string{"index.html": barPage})
	path := "/" + e.slug + "/"

	owner := e.do(path, nil, e.owner)
	if owner.Code != 200 || !hasBar(owner) {
		t.Fatalf("owner = %d, bar = %v", owner.Code, hasBar(owner))
	}
	body := owner.Body.String()
	if !strings.Contains(body, `"title":"My Report"`) ||
		!strings.Contains(body, `"url":"https://glim.example.com/demo-1234/"`) ||
		!strings.Contains(body, `"dash":"https://glim.example.com/"`) {
		t.Fatalf("bar data missing: %s", body)
	}
	if i, j := strings.Index(body, "glim-bar"), strings.LastIndex(body, "</body>"); i < 0 || i > j {
		t.Fatal("bar must sit before </body>")
	}
	if cc := owner.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("owner Cache-Control = %q", cc)
	}
	if v := owner.Header().Values("Vary"); !contains(v, "Cookie") {
		t.Fatalf("owner Vary = %v", v)
	}

	visitor := e.do(path, nil)
	if hasBar(visitor) || strings.Contains(visitor.Body.String(), "glim-bar") {
		t.Fatal("visitor must not get the bar")
	}
	if !contains(visitor.Header().Values("Vary"), "Cookie") {
		t.Fatalf("visitor Vary = %v", visitor.Header().Values("Vary"))
	}
	if strings.Contains(visitor.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("visitor response must stay cacheable as before")
	}
}

func TestOwnerBarVisitorBytesMatchBarlessServer(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, "T", false, map[string]string{"index.html": barPage})
	path := "/" + e.slug + "/"
	with := e.do(path, nil)
	e.opts.NoHeader = true
	without := e.do(path, nil)
	if with.Body.String() != without.Body.String() {
		t.Fatal("visitor body differs from a server with the bar off")
	}
	if with.Header().Get("ETag") != without.Header().Get("ETag") {
		t.Fatal("visitor ETag differs")
	}
}

func TestOwnerBarInvalidCookieGetsNoBar(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, "T", false, map[string]string{"index.html": barPage})
	rec := e.do("/"+e.slug+"/", nil, &http.Cookie{Name: auth.OwnerCookie, Value: "forged"})
	if rec.Code != 200 || hasBar(rec) {
		t.Fatalf("forged cookie = %d, bar = %v", rec.Code, hasBar(rec))
	}
}

func TestOwnerBarOffSwitches(t *testing.T) {
	t.Run("manifest no_header", func(t *testing.T) {
		e := newBarEnv(t)
		e.publish(t, "T", true, map[string]string{"index.html": barPage})
		if rec := e.do("/"+e.slug+"/", nil, e.owner); hasBar(rec) {
			t.Fatal("no_header preview got a bar")
		}
	})
	t.Run("server config", func(t *testing.T) {
		e := newBarEnv(t)
		e.opts.NoHeader = true
		e.publish(t, "T", false, map[string]string{"index.html": barPage})
		rec := e.do("/"+e.slug+"/", nil, e.owner)
		if hasBar(rec) || contains(rec.Header().Values("Vary"), "Cookie") {
			t.Fatal("header disabled in config must leave pages untouched")
		}
	})
}

func TestOwnerBarSkipsSubDocuments(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, "T", false, map[string]string{"index.html": barPage})
	path := "/" + e.slug + "/"
	for dest, want := range map[string]bool{"iframe": false, "frame": false, "document": true, "": true} {
		hdr := map[string]string{}
		if dest != "" {
			hdr["Sec-Fetch-Dest"] = dest
		}
		if got := hasBar(e.do(path, hdr, e.owner)); got != want {
			t.Errorf("Sec-Fetch-Dest %q: bar = %v, want %v", dest, got, want)
		}
	}
}

func TestOwnerBarLeavesNonHTMLAlone(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, "T", false, map[string]string{"index.html": barPage, "app.js": "console.log(1)", "data.json": `{"a":1}`})
	for _, f := range []string{"app.js", "data.json"} {
		rec := e.do("/"+e.slug+"/"+f, nil, e.owner)
		if rec.Code != 200 || strings.Contains(rec.Body.String(), "glim-bar") {
			t.Errorf("%s: %d %q", f, rec.Code, rec.Body.String())
		}
		if contains(rec.Header().Values("Vary"), "Cookie") {
			t.Errorf("%s should not vary on cookie", f)
		}
	}
}

func TestOwnerBarKeepsExistingVary(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, "T", false, map[string]string{"index.html": barPage})
	rec := e.do("/"+e.slug+"/", nil, e.owner)
	v := strings.Join(rec.Header().Values("Vary"), ",")
	if !strings.Contains(v, "Origin") || !strings.Contains(v, "Cookie") {
		t.Fatalf("Vary = %q, want Origin and Cookie", v)
	}
}

func TestOwnerBarEscapesTitle(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, `</script><script>alert(1)</script>&`, false, map[string]string{"index.html": barPage})
	body := e.do("/"+e.slug+"/", nil, e.owner).Body.String()
	if strings.Contains(body, "</script><script>alert(1)") {
		t.Fatal("title broke out of the script element")
	}
	if !strings.Contains(body, `</script>`) {
		t.Fatal("title not JSON-escaped")
	}
}

func TestOwnerBarExpiryAndPinned(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, "T", false, map[string]string{"index.html": barPage})
	if body := e.do("/"+e.slug+"/", nil, e.owner).Body.String(); strings.Contains(body, `"exp":0,`) {
		t.Fatal("expiring preview carried no expiry")
	}
	if err := e.st.Pin(e.slug); err != nil {
		t.Fatal(err)
	}
	if body := e.do("/"+e.slug+"/", nil, e.owner).Body.String(); !strings.Contains(body, `"exp":0,`) {
		t.Fatal("pinned preview should carry exp 0")
	}
}

func TestOwnerBarOnConvertedMarkdown(t *testing.T) {
	e := newBarEnv(t)
	src := filepath.Join(t.TempDir(), "notes.md")
	writeTestFile(t, src, "# Notes\n\nhello")
	if _, err := e.st.Publish(src, "Notes", "", "", time.Hour, e.slug); err != nil {
		t.Fatal(err)
	}
	if rec := e.do("/"+e.slug+"/", nil, e.owner); !hasBar(rec) {
		t.Fatal("converted markdown preview got no bar")
	}
}

func TestOwnerBarShowsOnLockedPreviewForOwner(t *testing.T) {
	e := newBarEnv(t)
	e.opts.Unlock = &Unlock{Token: e.db.UnlockToken, Limiter: auth.NewLimiter(nil)}
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "index.html"), barPage)
	if _, err := e.st.PublishLocked(src, "T", "", "", time.Hour, e.slug, hashFor(t, "hunter22!")); err != nil {
		t.Fatal(err)
	}
	rec := e.do("/"+e.slug+"/", nil, e.owner)
	if rec.Code != 200 || !hasBar(rec) || !strings.Contains(rec.Body.String(), `"locked":true`) {
		t.Fatalf("owner on locked = %d, bar = %v", rec.Code, hasBar(rec))
	}
	if rec := e.do("/"+e.slug+"/", nil); rec.Code != 401 || hasBar(rec) {
		t.Fatalf("visitor on locked = %d, bar = %v", rec.Code, hasBar(rec))
	}
}

func TestOwnerBarSkippedOverMaxInjectSize(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, "T", false, map[string]string{"index.html": barPage + strings.Repeat("a", maxInjectSize)})
	if rec := e.do("/"+e.slug+"/", nil, e.owner); rec.Code != 200 || hasBar(rec) {
		t.Fatalf("oversized html = %d, bar = %v", rec.Code, hasBar(rec))
	}
}

func contains(vals []string, want string) bool {
	for _, v := range vals {
		for _, p := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(p), want) {
				return true
			}
		}
	}
	return false
}

func TestOwnerBarCarriesTheLinkAsAQRCode(t *testing.T) {
	e := newBarEnv(t)
	e.publish(t, "T", false, map[string]string{"index.html": barPage})
	want, err := qrcode.Encode("https://glim.example.com/" + e.slug + "/")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(want)
	if body := e.do("/"+e.slug+"/", nil, e.owner).Body.String(); !strings.Contains(body, `"qr":`+string(data)) {
		t.Fatal("bar data lacks the QR code of the preview's link")
	}
}
