package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

func corsDo(h http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func corsEnv(t *testing.T) http.Handler {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<h1>x</h1>", "data.json": `{"a":1}`}, time.Hour)
	return NewRouter(Options{Store: st})
}

func TestOpaqueOriginMayFetchUnlockedPreviewFiles(t *testing.T) {
	h := corsEnv(t)
	null := map[string]string{"Origin": "null"}
	for _, p := range []string{"/demo-1234/data.json", "/demo-1234/"} {
		for _, m := range []string{"GET", "HEAD"} {
			rec := corsDo(h, m, p, null)
			if rec.Code != 200 {
				t.Fatalf("%s %s: status %d", m, p, rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "null" {
				t.Errorf("%s %s: ACAO = %q", m, p, got)
			}
			if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Errorf("%s %s: credentials allowed", m, p)
			}
			if rec.Header().Get("Vary") != "Origin" {
				t.Errorf("%s %s: Vary = %q", m, p, rec.Header().Get("Vary"))
			}
		}
	}
	for name, hdr := range map[string]map[string]string{
		"other origin": {"Origin": "https://evil.example"},
		"no origin":    nil,
	} {
		rec := corsDo(h, "GET", "/demo-1234/data.json", hdr)
		if rec.Code != 200 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: status %d ACAO %q", name, rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
		}
	}
	if rec := corsDo(h, "GET", "/demo-1234/missing.json", null); rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("404 carries ACAO")
	}
}

func TestPreviewPreflight(t *testing.T) {
	h := corsEnv(t)
	hdr := map[string]string{"Origin": "null", "Access-Control-Request-Method": "GET"}
	rec := corsDo(h, "OPTIONS", "/demo-1234/data.json", hdr)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "null" ||
		rec.Header().Get("Access-Control-Allow-Methods") != "GET, HEAD" {
		t.Errorf("preflight = %d %v", rec.Code, rec.Header())
	}
	hdr["Access-Control-Request-Method"] = "PUT"
	if rec := corsDo(h, "OPTIONS", "/demo-1234/data.json", hdr); rec.Code == 204 {
		t.Error("PUT preflight answered 204")
	}
	hdr["Access-Control-Request-Method"] = "GET"
	hdr["Origin"] = "https://evil.example"
	if rec := corsDo(h, "OPTIONS", "/demo-1234/data.json", hdr); rec.Code == 204 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("foreign-origin preflight answered")
	}
}

func TestLockedPreviewNeverGetsCORS(t *testing.T) {
	e := newLockEnv(t, true)
	c := e.unlockCookie(t)
	for _, withCookie := range []bool{false, true} {
		for _, m := range []string{"GET", "OPTIONS"} {
			r := httptest.NewRequest(m, "/"+e.slug+"/app.js", nil)
			r.RemoteAddr = "203.0.113.9:1234"
			r.Header.Set("Origin", "null")
			r.Header.Set("Access-Control-Request-Method", "GET")
			if withCookie {
				r.AddCookie(c)
			}
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, r)
			if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Code == 204 {
				t.Errorf("%s cookie=%v: status %d ACAO %q", m, withCookie, rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
			}
			if got := rec.Header().Get("Cache-Control"); got != "private, no-store" && got != "no-store" {
				t.Errorf("%s cookie=%v: Cache-Control = %q", m, withCookie, got)
			}
		}
	}
}

func TestDashboardAndAPINeverGetCORS(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	h := NewRouter(Options{Store: st, API: ok, Web: ok})
	for _, p := range []string{"/", "/_glim/assets/app.js", "/_glim/api/previews"} {
		rec := corsDo(h, "GET", p, map[string]string{"Origin": "null"})
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s carries ACAO", p)
		}
		rec = corsDo(h, "OPTIONS", p, map[string]string{"Origin": "null", "Access-Control-Request-Method": "GET"})
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s preflight carries ACAO", p)
		}
	}
}

func TestSelfFetchIsNotAView(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<h1>x</h1>", "data.json": "{}"}, time.Hour)
	n := 0
	h := PreviewHandlerWith(st, &Views{Record: func(string) { n++ }})
	hdr := map[string]string{"Origin": "null", "Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors", "User-Agent": chromeUA, "Accept": "*/*"}
	corsDo(h, "GET", "/demo-1234/data.json", hdr)
	corsDo(h, "GET", "/demo-1234/", hdr)
	if n != 0 {
		t.Errorf("self-fetches counted %d views", n)
	}
}

func TestSelfFetchRedirectAndOtherMethods(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<h1>x</h1>", "data/index.html": "<p>d</p>"}, time.Hour)
	h := NewRouter(Options{Store: st})
	null := map[string]string{"Origin": "null"}
	rec := corsDo(h, "GET", "/demo-1234/data", null)
	if rec.Code < 300 || rec.Code >= 400 {
		t.Fatalf("directory without slash: status %d, want a redirect", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "null" {
		t.Errorf("redirect ACAO = %q, want null so a CORS fetch can follow it", got)
	}
	rec = corsDo(h, "OPTIONS", "/demo-1234/", map[string]string{"Origin": "null", "Access-Control-Request-Method": "PUT"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("non-GET preflight got ACAO %q", got)
	}
}
