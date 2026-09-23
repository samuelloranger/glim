package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

func publish(t *testing.T, st *store.Store, name string, files map[string]string, ttl time.Duration) {
	t.Helper()
	src := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.Publish(src, "", "", "", ttl, name); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestPreviewHandlerServesLivePreviewWithSandbox(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{
		"index.html":     "<h1>demo</h1>",
		"app.js":         "console.log(1)",
		".env":           "SECRET=1",
		".git/config":    "x",
		"assets/.hidden": "x",
	}, time.Hour)
	h := PreviewHandler(st)

	cases := []struct {
		path string
		want int
		body string
	}{
		{"/demo-1234/", 200, "<h1>demo</h1>"},
		{"/demo-1234/app.js", 200, "console.log(1)"},
		{"/demo-1234/.glim.json", 404, ""},
		{"/demo-1234/.env", 404, ""},
		{"/demo-1234/.git/config", 404, ""},
		{"/demo-1234/assets/.hidden", 404, ""},
		{"/", 404, ""},
		{"/nope-9999/", 404, ""},
		{"/UPPER/", 404, ""},
	}
	for _, c := range cases {
		rec := get(t, h, c.path)
		if rec.Code != c.want {
			t.Errorf("GET %s = %d, want %d", c.path, rec.Code, c.want)
		}
		if c.body != "" && !strings.Contains(rec.Body.String(), c.body) {
			t.Errorf("GET %s body = %q, want %q", c.path, rec.Body.String(), c.body)
		}
		if c.want == 200 && rec.Header().Get("Content-Security-Policy") != PreviewCSP {
			t.Errorf("GET %s CSP = %q, want %q", c.path, rec.Header().Get("Content-Security-Policy"), PreviewCSP)
		}
	}
}

func TestPreviewHandlerHidesExpiredBeforeGC(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	now := time.Now()
	st.Now = func() time.Time { return now }
	publish(t, st, "old-abcd", map[string]string{"index.html": "old"}, time.Minute)
	h := PreviewHandler(st)
	if rec := get(t, h, "/old-abcd/"); rec.Code != 200 {
		t.Fatalf("live = %d, want 200", rec.Code)
	}
	now = now.Add(2 * time.Minute)
	if rec := get(t, h, "/old-abcd/"); rec.Code != 404 {
		t.Fatalf("expired = %d, want 404", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(st.Root, "old-abcd")); err != nil {
		t.Fatal("dir should still exist (not yet GC'd):", err)
	}
}

func TestPreviewHandlerDeniesDirWithoutIndex(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "site-abcd", map[string]string{"index.html": "root", "sub/file.txt": "x"}, time.Hour)
	h := PreviewHandler(st)
	if rec := get(t, h, "/site-abcd/sub/"); rec.Code != 404 {
		t.Errorf("dir listing = %d, want 404", rec.Code)
	}
	if rec := get(t, h, "/site-abcd/sub/file.txt"); rec.Code != 200 {
		t.Errorf("named file = %d, want 200", rec.Code)
	}
}

func TestStateRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := State{Port: 12345, PID: os.Getpid(), Root: "/x"}
	if err := WriteState(want); err != nil {
		t.Fatal(err)
	}
	got, ok := ReadState()
	if !ok || got != want {
		t.Fatalf("ReadState = %+v ok=%v, want %+v", got, ok, want)
	}
}

func TestFreePortIsUsable(t *testing.T) {
	p, err := FreePort()
	if err != nil || p <= 0 {
		t.Fatalf("FreePort = %d err=%v", p, err)
	}
}
