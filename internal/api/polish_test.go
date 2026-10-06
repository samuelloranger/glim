package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChangePasswordThrottlesWrongGuesses(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	wrong := map[string]string{"current": "not it at all", "next": "a fresh passphrase"}
	for i := 0; i < 5; i++ {
		if rec := e.do(http.MethodPost, "/_glim/api/account/password", wrong, &s, nil); rec.Code != 400 {
			t.Fatalf("guess %d = %d", i, rec.Code)
		}
	}
	right := map[string]string{"current": testPass, "next": "a fresh passphrase"}
	rec := e.do(http.MethodPost, "/_glim/api/account/password", right, &s, nil)
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("after 5 wrong guesses = %d, want 429 with Retry-After", rec.Code)
	}
	// Sign-in for the same account is not locked by password-change guesses.
	login := map[string]string{"email": "sam@example.com", "password": testPass}
	if rec := e.do(http.MethodPost, "/_glim/api/login", login, nil, nil); rec.Code != 200 {
		t.Fatalf("login = %d", rec.Code)
	}
}

func TestPlainHTTPDetection(t *testing.T) {
	mk := func(hdr map[string]string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		return r
	}
	if !PlainHTTP(mk(nil)) {
		t.Error("bare http request should count as plain")
	}
	if PlainHTTP(mk(map[string]string{"X-Forwarded-Proto": "https"})) {
		t.Error("X-Forwarded-Proto: https is proxied")
	}
	if PlainHTTP(mk(map[string]string{"Forwarded": "for=1.2.3.4;proto=https"})) {
		t.Error("Forwarded proto=https is proxied")
	}
	if !PlainHTTP(mk(map[string]string{"X-Forwarded-Proto": "http"})) {
		t.Error("X-Forwarded-Proto: http is plain")
	}
}

func TestWarnsOncePlainHTTPBehindHTTPSDomain(t *testing.T) {
	var logs []string
	e := newEnvWith(t, func(d *Deps) {
		d.Logf = func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }
	})
	e.do(http.MethodGet, "/_glim/api/setup", nil, nil, map[string]string{"X-Forwarded-Proto": "https"})
	if len(logs) != 0 {
		t.Fatalf("proxied request warned: %v", logs)
	}
	e.do(http.MethodGet, "/_glim/api/setup", nil, nil, nil)
	e.do(http.MethodGet, "/_glim/api/setup", nil, nil, nil)
	if len(logs) != 1 {
		t.Fatalf("want exactly one warning, got %v", logs)
	}
}
