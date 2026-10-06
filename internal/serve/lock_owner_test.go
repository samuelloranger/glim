package serve

import (
	"net/http"
	"testing"

	"github.com/samuelloranger/glim/internal/auth"
)

func TestOwnerBypassIssuesUnlockCookieForDocuments(t *testing.T) {
	e := newLockEnv(t, true)
	tok, _ := ownerSession(t, e.db)
	oc := &http.Cookie{Name: auth.OwnerCookie, Value: tok}
	rec := e.do("GET", "/"+e.slug+"/", nil, oc)
	if rec.Code != 200 {
		t.Fatalf("owner document = %d", rec.Code)
	}
	var got *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == UnlockCookiePrefix+e.slug {
			got = c
		}
	}
	if got == nil || !got.Secure || got.SameSite != http.SameSiteNoneMode {
		t.Fatalf("unlock cookie = %+v", got)
	}
	if rec := e.do("GET", "/"+e.slug+"/app.js", nil, got); rec.Code != 200 {
		t.Fatalf("asset with unlock cookie = %d", rec.Code)
	}
	rec = e.do("GET", "/"+e.slug+"/app.js", nil, oc)
	for _, c := range rec.Result().Cookies() {
		if c.Name == UnlockCookiePrefix+e.slug {
			t.Fatal("asset request should not mint an unlock cookie")
		}
	}
}

func TestStaleOwnerCookieDoesNotBypassLock(t *testing.T) {
	e := newLockEnv(t, true)
	tok, session := ownerSession(t, e.db)
	if err := e.db.DeleteSession(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	rec := e.do("GET", "/"+e.slug+"/", nil, &http.Cookie{Name: auth.OwnerCookie, Value: tok})
	if rec.Code != 401 {
		t.Fatalf("stale owner cookie = %d, want 401", rec.Code)
	}
}
