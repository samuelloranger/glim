package api

import (
	"net/http"
	"testing"
)

func TestUserManagement(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	rec := e.do(http.MethodPost, "/_glim/api/users", map[string]string{"email": "Amy@Example.com", "password": testPass}, &s, nil)
	if rec.Code != 201 || jsonBody(t, rec)["email"] != "amy@example.com" {
		t.Fatalf("add = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(http.MethodPost, "/_glim/api/users", map[string]string{"email": "amy@example.com", "password": testPass}, &s, nil); rec.Code != 409 {
		t.Fatalf("dup = %d", rec.Code)
	}
	if rec := e.do(http.MethodPost, "/_glim/api/users", map[string]string{"email": "not-an-email", "password": testPass}, &s, nil); rec.Code != 400 {
		t.Fatalf("bad name = %d", rec.Code)
	}
	list := jsonBody(t, e.do(http.MethodGet, "/_glim/api/users", nil, &s, nil))["users"].([]any)
	if len(list) != 2 {
		t.Fatalf("list = %v", list)
	}
	if rec := e.do(http.MethodDelete, "/_glim/api/users/sam@example.com", nil, &s, nil); rec.Code != 409 {
		t.Fatalf("self delete = %d", rec.Code)
	}
	if rec := e.do(http.MethodDelete, "/_glim/api/users/amy@example.com", nil, &s, nil); rec.Code != 204 {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := e.do(http.MethodDelete, "/_glim/api/users/amy@example.com", nil, &s, nil); rec.Code != 404 {
		t.Fatalf("delete twice = %d", rec.Code)
	}
}

func TestChangePasswordEndpoint(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	other, _ := e.db.CreateSession(t.Context(), s.User)
	wrong := map[string]string{"current": "not it at all", "next": "a fresh passphrase"}
	if rec := e.do(http.MethodPost, "/_glim/api/account/password", wrong, &s, nil); rec.Code != 400 {
		t.Fatalf("wrong current = %d (must not be 401)", rec.Code)
	}
	weak := map[string]string{"current": testPass, "next": "short"}
	if rec := e.do(http.MethodPost, "/_glim/api/account/password", weak, &s, nil); rec.Code != 400 {
		t.Fatalf("weak = %d", rec.Code)
	}
	ok := map[string]string{"current": testPass, "next": "a fresh passphrase"}
	if rec := e.do(http.MethodPost, "/_glim/api/account/password", ok, &s, nil); rec.Code != 204 {
		t.Fatalf("change = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(http.MethodGet, "/_glim/api/session", nil, &s, nil); rec.Code != 200 {
		t.Fatal("current session should survive")
	}
	if rec := e.do(http.MethodGet, "/_glim/api/session", nil, &other, nil); rec.Code != 401 {
		t.Fatal("other session should be signed out")
	}
}
