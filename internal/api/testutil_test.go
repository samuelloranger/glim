package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const testPass = "correct horse battery"

type env struct {
	t   *testing.T
	db  *auth.DB
	st  *store.Store
	srv *Server
}

func newEnv(t *testing.T) *env {
	return newEnvWith(t, func(*Deps) {})
}

func newEnvWith(t *testing.T, tweak func(*Deps)) *env {
	t.Helper()
	db, err := auth.Open(filepath.Join(t.TempDir(), "glim.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.BcryptCost = bcrypt.MinCost
	t.Cleanup(func() { db.Close() })
	e := &env{
		t:  t,
		db: db,
		st: store.New(t.TempDir(), "https://glim.example.com"),
	}
	d := Deps{Store: e.st, Auth: db, Limiter: auth.NewLimiter(nil),
		SecureCookies: true, Logf: t.Logf}
	tweak(&d)
	e.srv = New(d)
	return e
}

// do sends a request as a browser on https://glim.example.com would. With a
// session it attaches the cookie and CSRF header; hdr overrides/extends headers
// ("" deletes one). "RemoteAddr" in hdr sets the peer address.
func (e *env) do(method, path string, body any, sess *auth.Session, hdr map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Host = "glim.example.com"
	req.RemoteAddr = "203.0.113.7:40000"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("Origin", "https://glim.example.com")
	}
	if sess != nil {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: sess.Token})
		req.Header.Set("X-Glim-CSRF", sess.CSRF)
	}
	for k, v := range hdr {
		switch {
		case k == "RemoteAddr":
			req.RemoteAddr = v
		case k == "Host":
			req.Host = v
		case v == "":
			req.Header.Del(k)
		default:
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

func (e *env) signIn(email string) auth.Session {
	e.t.Helper()
	u, err := e.db.CreateUser(e.t.Context(), email, testPass)
	if err != nil {
		e.t.Fatal(err)
	}
	s, err := e.db.CreateSession(e.t.Context(), u)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func jsonBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return m
}
