package api

import (
	"net/http"
	"testing"
	"time"
)

func TestUnpinGivesDefaultLifetime(t *testing.T) {
	e := newEnvWith(t, func(d *Deps) { d.DefaultTTL = 3 * time.Hour })
	s := e.signIn("sam@example.com")
	name := publishPreview(t, e.st, "Pin me", time.Hour)
	if rec := e.do(http.MethodPost, "/_glim/api/previews/"+name+"/pin", nil, &s, nil); rec.Code != 200 {
		t.Fatalf("pin = %d", rec.Code)
	}
	rec := e.do(http.MethodPost, "/_glim/api/previews/"+name+"/unpin", nil, &s, nil)
	if rec.Code != 200 || jsonBody(t, rec)["pinned"] != false {
		t.Fatalf("unpin = %d %s", rec.Code, rec.Body.String())
	}
	m, _ := e.st.Get(name)
	if m.Pinned || time.Until(m.Expires) < 2*time.Hour+30*time.Minute {
		t.Fatalf("manifest after unpin = %+v", m)
	}
	if rec := e.do(http.MethodPost, "/_glim/api/previews/nope-abcd/unpin", nil, &s, nil); rec.Code != 404 {
		t.Fatalf("unpin missing = %d", rec.Code)
	}
}

func TestExtendUnpinsPinnedPreview(t *testing.T) {
	e := newEnv(t)
	s := e.signIn("sam@example.com")
	name := publishPreview(t, e.st, "Pin me", time.Hour)
	e.do(http.MethodPost, "/_glim/api/previews/"+name+"/pin", nil, &s, nil)
	rec := e.do(http.MethodPost, "/_glim/api/previews/"+name+"/extend", map[string]string{"ttl": "12h"}, &s, nil)
	if rec.Code != 200 || jsonBody(t, rec)["pinned"] != false {
		t.Fatalf("extend = %d %s", rec.Code, rec.Body.String())
	}
}
