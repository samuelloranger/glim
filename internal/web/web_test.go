package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandler(t *testing.T) {
	h := handler(fstest.MapFS{
		"index.html":           {Data: []byte(`<div id="app"></div>`)},
		"icon.svg":             {Data: []byte(`<svg/>`)},
		"assets/app-abc123.js": {Data: []byte(`console.log(1)`)},
	})
	root := get(h, "/")
	if root.Code != 200 || !strings.Contains(root.Body.String(), `id="app"`) {
		t.Fatalf("/ = %d %q", root.Code, root.Body.String())
	}
	if root.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(root.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("/ headers = %v", root.Header())
	}
	js := get(h, "/_glim/assets/app-abc123.js")
	if js.Code != 200 || !strings.Contains(js.Header().Get("Cache-Control"), "immutable") ||
		!strings.Contains(js.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("asset = %d %v", js.Code, js.Header())
	}
	if icon := get(h, "/_glim/icon.svg"); icon.Code != 200 || icon.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("icon = %d %v", icon.Code, icon.Header())
	}
	for _, p := range []string{"/_glim/", "/_glim/assets/", "/_glim/index.html", "/_glim/nope.js", "/elsewhere"} {
		if rec := get(h, p); rec.Code != 404 {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
	for _, rec := range []*httptest.ResponseRecorder{root, js} {
		if rec.Header().Get("Content-Security-Policy") != CSP {
			t.Fatalf("CSP = %q", rec.Header().Get("Content-Security-Policy"))
		}
		if strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Fatal("dashboard must never be sandboxed")
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Referrer-Policy") != "same-origin" {
			t.Fatalf("headers = %v", rec.Header())
		}
	}
}

func TestEmbeddedDist(t *testing.T) {
	rec := get(Handler(), "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/_glim/assets/") {
		t.Fatalf("embedded index = %d %q", rec.Code, rec.Body.String())
	}
}
