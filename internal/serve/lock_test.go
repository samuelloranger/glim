package serve

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type lockEnv struct {
	st    *store.Store
	db    *auth.DB
	h     http.Handler
	views int
	slug  string
}

func hashFor(t *testing.T, pw string) string {
	t.Helper()
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newLockEnv(t *testing.T, secure bool) *lockEnv {
	t.Helper()
	e := &lockEnv{slug: "secret-abcd"}
	e.st = store.New(t.TempDir(), "https://glim.example.com")
	db, err := auth.Open(filepath.Join(t.TempDir(), "glim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e.db = db
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "index.html"), `<html><head><title>Quarterly numbers</title></head><body>SECRET-BODY</body></html>`)
	writeTestFile(t, filepath.Join(src, "app.js"), "SECRET-JS")
	if _, err := e.st.PublishLocked(src, "Quarterly numbers", "acme-project", "", time.Hour, e.slug, hashFor(t, "hunter22!")); err != nil {
		t.Fatal(err)
	}
	e.h = PreviewHandlerFull(e.st, &Views{Record: func(string) { e.views++ }, IsOwner: db.IsOwnerToken},
		&Unlock{Token: db.UnlockToken, Limiter: auth.NewLimiter(nil), Secure: secure})
	return e
}

func (e *lockEnv) do(method, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	body := strings.NewReader("")
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, path, body)
	r.RemoteAddr = "203.0.113.9:1234"
	r.Header.Set("Sec-Fetch-Dest", "document")
	r.Header.Set("User-Agent", "Mozilla/5.0")
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return rec
}

func (e *lockEnv) unlockCookie(t *testing.T) *http.Cookie {
	t.Helper()
	rec := e.do("POST", "/"+e.slug+"/", url.Values{"password": {"hunter22!"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unlock = %d, want 303", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == UnlockCookiePrefix+e.slug {
			return c
		}
	}
	t.Fatal("no unlock cookie set")
	return nil
}

func TestLockedPreviewServingMatrix(t *testing.T) {
	e := newLockEnv(t, true)
	page := "/" + e.slug + "/"

	rec := e.do("GET", page, nil)
	if rec.Code != 401 || strings.Contains(rec.Body.String(), "SECRET-BODY") || !strings.Contains(rec.Body.String(), `type="password"`) {
		t.Fatalf("no cookie page = %d %q", rec.Code, rec.Body.String())
	}
	if rec := e.do("GET", page+"app.js", nil); rec.Code != 401 || strings.Contains(rec.Body.String(), "SECRET-JS") {
		t.Fatalf("no cookie asset = %d", rec.Code)
	}
	bad := &http.Cookie{Name: UnlockCookiePrefix + e.slug, Value: "deadbeef"}
	if rec := e.do("GET", page, nil, bad); rec.Code != 401 {
		t.Fatalf("bad cookie = %d", rec.Code)
	}
	if rec := e.do("GET", "/"+e.slug, nil); rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != page {
		t.Fatalf("bare slug = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	c := e.unlockCookie(t)
	if c.Path != page || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteNoneMode {
		t.Fatalf("cookie attrs = %+v", c)
	}
	if rec := e.do("GET", page, nil, c); rec.Code != 200 || !strings.Contains(rec.Body.String(), "SECRET-BODY") {
		t.Fatalf("good cookie page = %d", rec.Code)
	}
	if rec := e.do("GET", page+"app.js", nil, c); rec.Code != 200 || rec.Body.String() != "SECRET-JS" {
		t.Fatalf("good cookie asset = %d", rec.Code)
	}

	// Changing the password invalidates earlier unlocks.
	if err := e.st.SetPasswordHash(e.slug, hashFor(t, "another-pass")); err != nil {
		t.Fatal(err)
	}
	if rec := e.do("GET", page, nil, c); rec.Code != 401 {
		t.Fatalf("stale cookie = %d, want 401", rec.Code)
	}
	// Removing it opens the preview to everyone.
	if err := e.st.SetPasswordHash(e.slug, ""); err != nil {
		t.Fatal(err)
	}
	if rec := e.do("GET", page, nil); rec.Code != 200 {
		t.Fatalf("unlocked = %d", rec.Code)
	}
}

func TestLockedPreviewOwnerBypass(t *testing.T) {
	e := newLockEnv(t, true)
	tok, _ := ownerSession(t, e.db)
	rec := e.do("GET", "/"+e.slug+"/app.js", nil, &http.Cookie{Name: auth.OwnerCookie, Value: tok})
	if rec.Code != 200 {
		t.Fatalf("owner = %d, want 200", rec.Code)
	}
}

func TestUnlockAttemptsAreNotViews(t *testing.T) {
	e := newLockEnv(t, true)
	page := "/" + e.slug + "/"
	e.do("GET", page, nil)
	e.do("POST", page, url.Values{"password": {"nope-nope"}})
	c := e.unlockCookie(t)
	if e.views != 0 {
		t.Fatalf("views after lock page and attempts = %d, want 0", e.views)
	}
	e.do("GET", page, nil, c)
	if e.views != 1 {
		t.Fatalf("views after unlocked open = %d, want 1", e.views)
	}
}

func TestUnlockRedirectsBackAndWrongPasswordFails(t *testing.T) {
	e := newLockEnv(t, false)
	rec := e.do("POST", "/"+e.slug+"/app.js?x=1", url.Values{"password": {"hunter22!"}})
	if rec.Code != 303 || rec.Header().Get("Location") != "/"+e.slug+"/app.js?x=1" {
		t.Fatalf("redirect = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	c := rec.Result().Cookies()[0]
	if c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("http cookie = %+v, want Lax and not Secure", c)
	}
	rec = e.do("POST", "/"+e.slug+"/", url.Values{"password": {"wrong-pass"}})
	if rec.Code != 401 || len(rec.Result().Cookies()) != 0 || !strings.Contains(rec.Body.String(), "Wrong password") {
		t.Fatalf("wrong password = %d", rec.Code)
	}
}

func TestUnlockRateLimited(t *testing.T) {
	e := newLockEnv(t, true)
	page := "/" + e.slug + "/"
	for i := 0; i < 5; i++ {
		if rec := e.do("POST", page, url.Values{"password": {"wrong-pass"}}); rec.Code != 401 {
			t.Fatalf("attempt %d = %d", i, rec.Code)
		}
	}
	rec := e.do("POST", page, url.Values{"password": {"hunter22!"}})
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("throttled = %d", rec.Code)
	}
}

func TestLockPageDoesNotLeakMetadata(t *testing.T) {
	e := newLockEnv(t, true)
	rec := e.do("GET", "/"+e.slug+"/", nil)
	body := rec.Body.String()
	for _, leak := range []string{"Quarterly", "acme-project", "expires in", "SECRET"} {
		if strings.Contains(body, leak) {
			t.Errorf("lock page leaks %q", leak)
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "og:") && strings.Contains(line, e.slug) {
			t.Errorf("card tag leaks the slug: %s", line)
		}
	}
	if !strings.Contains(body, `<meta property="og:title" content="Password-protected preview">`) {
		t.Errorf("missing locked og:title:\n%s", body)
	}
	if strings.Contains(body, "og:url") {
		t.Error("lock page must not declare og:url")
	}
	if rec.Header().Get("X-Robots-Tag") == "" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("headers = %v", rec.Header())
	}
}

func TestLockedWithoutUnlockConfigFailsClosed(t *testing.T) {
	e := newLockEnv(t, true)
	h := PreviewHandlerWith(e.st, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/"+e.slug+"/", nil))
	if rec.Code != 401 || strings.Contains(rec.Body.String(), "SECRET-BODY") {
		t.Fatalf("code = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/"+e.slug+"/", strings.NewReader("password=hunter22!"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, r)
	if rec.Code == 303 {
		t.Fatal("unlock must not succeed without an Unlock config")
	}
}
