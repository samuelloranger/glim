package api

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/samuelloranger/glim/internal/auth"
)

func TestSetupFlow(t *testing.T) {
	e := newEnv(t)
	code, err := e.db.EnsureSetupCode(t.Context(), e.codePath)
	if err != nil {
		t.Fatal(err)
	}
	if b := jsonBody(t, e.do(http.MethodGet, "/_glim/api/setup", nil, nil, nil)); b["needed"] != true {
		t.Fatalf("needed = %v", b)
	}
	body := map[string]string{"code": "WRONGWRONG", "username": "sam", "password": testPass}
	if rec := e.do(http.MethodPost, "/_glim/api/setup", body, nil, nil); rec.Code != 400 || jsonBody(t, rec)["code"] != "invalid" {
		t.Fatalf("wrong code = %d %s", rec.Code, rec.Body.String())
	}
	body["code"] = auth.FormatSetupCode(code)
	body["password"] = "short"
	if rec := e.do(http.MethodPost, "/_glim/api/setup", body, nil, nil); rec.Code != 400 {
		t.Fatalf("weak password = %d", rec.Code)
	}
	body["password"] = testPass
	if rec := e.do(http.MethodPost, "/_glim/api/setup", body, nil, map[string]string{"Origin": "https://evil.example"}); rec.Code != 403 {
		t.Fatalf("foreign origin setup = %d", rec.Code)
	}
	rec := e.do(http.MethodPost, "/_glim/api/setup", body, nil, nil)
	if rec.Code != 201 {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	c := rec.Result().Cookies()
	if len(c) != 1 || c[0].Name != cookieName || !c[0].HttpOnly || !c[0].Secure ||
		c[0].SameSite != http.SameSiteStrictMode || c[0].Path != "/_glim" {
		t.Fatalf("cookie = %+v", c)
	}
	if jsonBody(t, rec)["csrf"] == "" {
		t.Fatal("no csrf in body")
	}
	if b := jsonBody(t, e.do(http.MethodGet, "/_glim/api/setup", nil, nil, nil)); b["needed"] != false {
		t.Fatalf("needed after = %v", b)
	}
	if rec := e.do(http.MethodPost, "/_glim/api/setup", body, nil, nil); rec.Code != 409 {
		t.Fatalf("second setup = %d", rec.Code)
	}
}

func TestSetupRequiresJSON(t *testing.T) {
	e := newEnv(t)
	rec := e.do(http.MethodPost, "/_glim/api/setup", nil, nil, map[string]string{"Content-Type": "text/plain"})
	if rec.Code != 400 {
		t.Fatalf("non-JSON = %d", rec.Code)
	}
}

func TestLoginAndThrottle(t *testing.T) {
	e := newEnv(t)
	if _, err := e.db.CreateUser(t.Context(), "sam", testPass); err != nil {
		t.Fatal(err)
	}
	bad := map[string]string{"username": "sam", "password": "not the password"}
	for i := 0; i < 5; i++ {
		if rec := e.do(http.MethodPost, "/_glim/api/login", bad, nil, nil); rec.Code != 401 {
			t.Fatalf("bad login %d = %d", i, rec.Code)
		}
	}
	rec := e.do(http.MethodPost, "/_glim/api/login", bad, nil, nil)
	if rec.Code != 429 || jsonBody(t, rec)["code"] != "rate_limited" {
		t.Fatalf("6th = %d %s", rec.Code, rec.Body.String())
	}
	if ra, _ := strconv.Atoi(rec.Header().Get("Retry-After")); ra < 1 {
		t.Fatalf("Retry-After = %q", rec.Header().Get("Retry-After"))
	}
	// Unknown users get the same answer as a wrong password.
	unk := e.do(http.MethodPost, "/_glim/api/login", map[string]string{"username": "ghost", "password": testPass}, nil,
		map[string]string{"RemoteAddr": "198.51.100.1:1"})
	if unk.Code != 401 || jsonBody(t, unk)["error"] != "Wrong username or password." {
		t.Fatalf("unknown = %d %s", unk.Code, unk.Body.String())
	}
}

func TestLoginSuccessSetsCookie(t *testing.T) {
	e := newEnv(t)
	e.db.CreateUser(t.Context(), "sam", testPass)
	rec := e.do(http.MethodPost, "/_glim/api/login", map[string]string{"username": "Sam", "password": testPass}, nil, nil)
	if rec.Code != 200 || len(rec.Result().Cookies()) != 1 {
		t.Fatalf("login = %d %s", rec.Code, rec.Body.String())
	}
}
