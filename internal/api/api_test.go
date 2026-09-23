package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		remote, xff, want string
	}{
		{"203.0.113.7:1", "", "203.0.113.7"},
		{"203.0.113.7:1", "1.2.3.4", "203.0.113.7"},               // untrusted peer: XFF ignored
		{"172.18.0.2:1", "198.51.100.9", "198.51.100.9"},          // proxy on a private net
		{"127.0.0.1:1", "6.6.6.6, 198.51.100.9", "198.51.100.9"},  // right-most untrusted hop
		{"127.0.0.1:1", "198.51.100.9, 10.0.0.5", "198.51.100.9"}, // skip trusted hops
		{"127.0.0.1:1", "192.168.1.20", "192.168.1.20"},           // all trusted: left-most
		{"[::1]:1", "2001:db8::1", "2001:db8::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := clientIP(r); got != c.want {
			t.Errorf("clientIP(%s, %q) = %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	e := newEnv(t)
	rec := e.do(http.MethodGet, "/_glim/api/nope", nil, nil, nil)
	if rec.Code != 404 || jsonBody(t, rec)["code"] != "not_found" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("API responses must be no-store")
	}
}

func TestProtectedEndpointNeedsSession(t *testing.T) {
	e := newEnv(t)
	if rec := e.do(http.MethodGet, "/_glim/api/session", nil, nil, nil); rec.Code != 401 {
		t.Fatalf("no cookie = %d", rec.Code)
	}
	bogus := e.do(http.MethodGet, "/_glim/api/session", nil, nil, map[string]string{"Cookie": cookieName + "=bogus"})
	if bogus.Code != 401 {
		t.Fatalf("bogus cookie = %d", bogus.Code)
	}
	s := e.signIn("sam")
	rec := e.do(http.MethodGet, "/_glim/api/session", nil, &s, nil)
	if rec.Code != 200 {
		t.Fatalf("session = %d %s", rec.Code, rec.Body.String())
	}
	b := jsonBody(t, rec)
	if b["csrf"] != s.CSRF || b["user"].(map[string]any)["username"] != "sam" {
		t.Fatalf("body = %v", b)
	}
}

func TestMutationsNeedCSRFAndSameOrigin(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam")
	for name, hdr := range map[string]map[string]string{
		"missing csrf":   {"X-Glim-CSRF": ""},
		"wrong csrf":     {"X-Glim-CSRF": "nope"},
		"foreign origin": {"Origin": "https://evil.example"},
		"null origin":    {"Origin": "null"},
	} {
		if rec := e.do(http.MethodPost, "/_glim/api/logout", nil, &s, hdr); rec.Code != 403 {
			t.Errorf("%s = %d, want 403", name, rec.Code)
		}
	}
	if rec := e.do(http.MethodPost, "/_glim/api/logout", nil, &s, nil); rec.Code != 204 {
		t.Fatalf("logout = %d", rec.Code)
	}
	if rec := e.do(http.MethodGet, "/_glim/api/session", nil, &s, nil); rec.Code != 401 {
		t.Fatalf("after logout = %d", rec.Code)
	}
}

func TestOriginViaTrustedForwardedHost(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam")
	proxied := map[string]string{
		"RemoteAddr":       "172.18.0.2:5555",
		"Host":             "127.0.0.1:8787",
		"X-Forwarded-Host": "glim.example.com",
	}
	if rec := e.do(http.MethodPost, "/_glim/api/logout", nil, &s, proxied); rec.Code != 204 {
		t.Fatalf("proxied logout = %d %s", rec.Code, rec.Body.String())
	}
	s2, _ := e.db.CreateSession(t.Context(), s.User)
	spoofed := map[string]string{
		"RemoteAddr":       "203.0.113.7:5555",
		"Host":             "127.0.0.1:8787",
		"X-Forwarded-Host": "glim.example.com",
	}
	if rec := e.do(http.MethodPost, "/_glim/api/logout", nil, &s2, spoofed); rec.Code != 403 {
		t.Fatalf("untrusted X-Forwarded-Host = %d, want 403", rec.Code)
	}
}
