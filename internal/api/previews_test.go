package api

import (
	"net/http"
	"testing"
	"time"
)

func TestListPreviews(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	publishPreview(t, e.st, "Diff review", time.Hour)
	b := jsonBody(t, e.do(http.MethodGet, "/_glim/api/previews", nil, &s, nil))
	if len(b["previews"].([]any)) != 1 || b["status"].(map[string]any)["live"] != float64(1) {
		t.Fatalf("body = %v", b)
	}
}

func TestExtendValidation(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	name := publishPreview(t, e.st, "Ext", time.Hour)
	path := "/_glim/api/previews/" + name + "/extend"
	for _, ttl := range []string{"", "abc", "0s", "-1h", "8761h", "7d"} {
		if rec := e.do(http.MethodPost, path, map[string]string{"ttl": ttl}, &s, nil); rec.Code != 400 {
			t.Errorf("ttl %q = %d, want 400", ttl, rec.Code)
		}
	}
	for _, bad := range []string{"UPPER", "bad_name", "a%2Fb"} {
		rec := e.do(http.MethodPost, "/_glim/api/previews/"+bad+"/extend", map[string]string{"ttl": "1h"}, &s, nil)
		if rec.Code != 400 {
			t.Errorf("name %q = %d, want 400", bad, rec.Code)
		}
	}
	if rec := e.do(http.MethodPost, "/_glim/api/previews/nope-zzzz/extend", map[string]string{"ttl": "1h"}, &s, nil); rec.Code != 404 {
		t.Errorf("missing = %d", rec.Code)
	}
	before := time.Now()
	rec := e.do(http.MethodPost, path, map[string]string{"ttl": "48h"}, &s, nil)
	if rec.Code != 200 {
		t.Fatalf("extend = %d %s", rec.Code, rec.Body.String())
	}
	exp, _ := time.Parse(time.RFC3339Nano, jsonBody(t, rec)["expires"].(string))
	if exp.Before(before.Add(47*time.Hour)) || exp.After(time.Now().Add(49*time.Hour)) {
		t.Fatalf("expires = %v", exp)
	}
}

func TestPinAndRemove(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	name := publishPreview(t, e.st, "Pin me", time.Hour)
	rec := e.do(http.MethodPost, "/_glim/api/previews/"+name+"/pin", nil, &s, nil)
	if rec.Code != 200 || jsonBody(t, rec)["pinned"] != true {
		t.Fatalf("pin = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(http.MethodDelete, "/_glim/api/previews/"+name, nil, &s, nil); rec.Code != 204 {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := e.do(http.MethodDelete, "/_glim/api/previews/"+name, nil, &s, nil); rec.Code != 404 {
		t.Fatalf("delete twice = %d", rec.Code)
	}
	if rec := e.do(http.MethodDelete, "/_glim/api/previews/"+name, nil, &s, map[string]string{"X-Glim-CSRF": ""}); rec.Code != 403 {
		t.Fatalf("no csrf = %d", rec.Code)
	}
}
