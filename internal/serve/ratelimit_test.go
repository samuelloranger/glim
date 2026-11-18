package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

func getFrom(h http.Handler, path, remote, xff string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func exhaust(h http.Handler, remote, xff string) {
	for i := 0; i < getLimit; i++ {
		getFrom(h, "/demo-1234/", remote, xff)
	}
}

func TestGetRateLimit(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<html>hi</html>"}, time.Hour)
	h := NewRouter(Options{Store: st, LiveReload: true})

	exhaust(h, "203.0.113.5:1234", "")
	w := getFrom(h, "/demo-1234/", "203.0.113.5:1234", "")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("got %d retry-after=%q, want 429 with Retry-After", w.Code, w.Header().Get("Retry-After"))
	}
	if w := getFrom(h, LivePathPrefix+"demo-1234", "203.0.113.5:1234", ""); w.Code != 429 {
		t.Fatalf("live stream not limited: %d", w.Code)
	}
	if w := getFrom(h, "/demo-1234/", "203.0.113.6:1234", ""); w.Code != 200 {
		t.Fatalf("other IP throttled: %d", w.Code)
	}
	// Loopback/private peers without a public forwarded address are never throttled.
	exhaust(h, "127.0.0.1:1", "")
	if w := getFrom(h, "/demo-1234/", "127.0.0.1:1", ""); w.Code != 200 {
		t.Fatalf("loopback throttled: %d", w.Code)
	}
}

func TestGetRateLimitForwardedFor(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<html>hi</html>"}, time.Hour)
	h := NewRouter(Options{Store: st})

	// Behind a trusted proxy the forwarded client is what gets throttled.
	exhaust(h, "10.0.0.2:1", "198.51.100.7")
	if w := getFrom(h, "/demo-1234/", "10.0.0.2:1", "198.51.100.7"); w.Code != 429 {
		t.Fatalf("forwarded client not throttled: %d", w.Code)
	}
	if w := getFrom(h, "/demo-1234/", "10.0.0.2:1", "198.51.100.8"); w.Code != 200 {
		t.Fatalf("other forwarded client throttled: %d", w.Code)
	}
	// An untrusted peer cannot dodge the limit by rotating X-Forwarded-For.
	exhaust(h, "203.0.113.9:1", "")
	if w := getFrom(h, "/demo-1234/", "203.0.113.9:1", "192.0.2.1"); w.Code != 429 {
		t.Fatalf("spoofed XFF from untrusted peer bypassed limit: %d", w.Code)
	}
}
