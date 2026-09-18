package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlerServesSlugIndexButNotRootOrListing(t *testing.T) {
	root := t.TempDir()
	slug := filepath.Join(root, "demo-1234")
	os.MkdirAll(slug, 0o755)
	os.WriteFile(filepath.Join(slug, "index.html"), []byte("<h1>demo</h1>"), 0o644)
	os.WriteFile(filepath.Join(slug, "app.js"), []byte("console.log(1)"), 0o644)

	srv := httptest.NewServer(Handler(root))
	defer srv.Close()

	cases := []struct {
		path string
		want int
		body string
	}{
		{"/demo-1234/", 200, "<h1>demo</h1>"},
		{"/demo-1234/app.js", 200, "console.log(1)"},
		{"/", 404, ""},          // no root index
		{"/nope-9999/", 404, ""}, // unknown slug
	}
	for _, c := range cases {
		resp, err := http.Get(srv.URL + c.path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != c.want {
			t.Errorf("GET %s = %d, want %d", c.path, resp.StatusCode, c.want)
		}
		resp.Body.Close()
	}
}

func TestHandlerDeniesDirWithoutIndex(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "noidx")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("x"), 0o644)

	srv := httptest.NewServer(Handler(root))
	defer srv.Close()

	// The dir has no index.html -> listing denied (404), but a named file inside
	// is still reachable (it's static content the publisher chose to include).
	resp, _ := http.Get(srv.URL + "/noidx/")
	if resp.StatusCode != 404 {
		t.Errorf("dir listing = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
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
