package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
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

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestGetRateLimitWindow(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<html>hi</html>"}, time.Hour)
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := NewRouter(Options{Store: st, GetLimiter: auth.NewLimiter(c.now), API: stub, Web: stub})
	peer := "203.0.113.5:1"

	exhaust(h, peer, "")
	if w := getFrom(h, "/demo-1234/", peer, ""); w.Code != 429 || w.Header().Get("Retry-After") != "60" {
		t.Fatalf("got %d retry-after=%q, want 429 / 60", w.Code, w.Header().Get("Retry-After"))
	}
	c.t = c.t.Add(20 * time.Second)
	if w := getFrom(h, "/demo-1234/", peer, ""); w.Header().Get("Retry-After") != "40" {
		t.Fatalf("retry-after=%q after 20s, want 40", w.Header().Get("Retry-After"))
	}
	// HEAD counts too.
	r := httptest.NewRequest("HEAD", "/demo-1234/", nil)
	r.RemoteAddr = peer
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 429 {
		t.Fatalf("HEAD not limited: %d", w.Code)
	}
	// Dashboard and API zones are never throttled.
	for _, p := range []string{"/_glim/api/session", "/_glim/", "/"} {
		if w := getFrom(h, p, peer, ""); w.Code != 200 {
			t.Fatalf("%s throttled: %d", p, w.Code)
		}
	}
	c.t = c.t.Add(41 * time.Second)
	if w := getFrom(h, "/demo-1234/", peer, ""); w.Code != 200 {
		t.Fatalf("no recovery after the window: %d", w.Code)
	}
}

func TestGetKey(t *testing.T) {
	cases := map[string]string{
		"203.0.113.5":               "203.0.113.5",
		"::ffff:203.0.113.5":        "203.0.113.5",
		"2001:db8:1:2:aaaa::1":      "2001:db8:1:2::/64",
		"2001:db8:1:2:bbbb:cccc::9": "2001:db8:1:2::/64",
		"2001:db8:1:3::1":           "2001:db8:1:3::/64",
		"not-an-ip":                 "not-an-ip",
	}
	for in, want := range cases {
		if got := getKey(in); got != want {
			t.Errorf("getKey(%q) = %q, want %q", in, got, want)
		}
	}
}
