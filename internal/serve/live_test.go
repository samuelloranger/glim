package serve

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

func liveEnv(t *testing.T) (*store.Store, *Live, *httptest.Server) {
	t.Helper()
	st := store.New(t.TempDir(), "https://glim.example.com")
	publish(t, st, "demo-1234", map[string]string{"index.html": "<html><body>v1</body></html>"}, time.Hour)
	l := NewLive(st)
	l.Poll, l.Heartbeat = 10*time.Millisecond, 40*time.Millisecond
	srv := httptest.NewServer(NewRouter(Options{Store: st, LiveReload: true, Live: l}))
	t.Cleanup(srv.Close)
	return st, l, srv
}

// lines streams the response body's lines on a channel.
func lines(resp *http.Response) <-chan string {
	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			ch <- sc.Text()
		}
	}()
	return ch
}

func waitFor(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case l, ok := <-ch:
			if !ok {
				t.Fatalf("stream ended before %q", want)
			}
			if strings.Contains(l, want) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q", want)
		}
	}
}

func openLive(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestLiveEmitsChangedOnRepublishAndGoneOnRemove(t *testing.T) {
	st, _, srv := liveEnv(t)
	resp := openLive(t, srv, "/_glim/live/demo-1234")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
	ch := lines(resp)
	waitFor(t, ch, ": ok")
	time.Sleep(30 * time.Millisecond)
	publish(t, st, "demo-1234", map[string]string{"index.html": "<html><body>v2</body></html>"}, 2*time.Hour)
	waitFor(t, ch, "event: changed")
	if err := st.Remove("demo-1234"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, ch, "event: gone")
}

func TestLiveStaleVersionChangesImmediately(t *testing.T) {
	_, _, srv := liveEnv(t)
	resp := openLive(t, srv, "/_glim/live/demo-1234?v=stale")
	waitFor(t, lines(resp), "event: changed")
}

func TestLiveHeartbeat(t *testing.T) {
	_, _, srv := liveEnv(t)
	resp := openLive(t, srv, "/_glim/live/demo-1234")
	waitFor(t, lines(resp), ": hb")
}

func TestLiveUnknownSlugAndMethod(t *testing.T) {
	_, _, srv := liveEnv(t)
	for _, p := range []string{"/_glim/live/nope-0000", "/_glim/live/x", "/_glim/live/"} {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Errorf("%s: %d", p, resp.StatusCode)
		}
	}
	resp, err := http.Post(srv.URL+"/_glim/live/demo-1234", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", resp.StatusCode)
	}
}

func TestLiveCaps(t *testing.T) {
	_, l, srv := liveEnv(t)
	l.MaxPerIP, l.MaxTotal = 2, 3
	for i := 0; i < 2; i++ {
		openLive(t, srv, "/_glim/live/demo-1234")
	}
	resp, err := http.Get(srv.URL + "/_glim/live/demo-1234")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("per-IP cap: %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("429 should carry CORS header")
	}
	// Raise the per-IP cap so only the total cap (3) can reject.
	l.MaxPerIP = 10
	openLive(t, srv, "/_glim/live/demo-1234")
	resp, err = http.Get(srv.URL + "/_glim/live/demo-1234")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("total cap: %d", resp.StatusCode)
	}
}

func TestLiveReleasesSlotOnDisconnect(t *testing.T) {
	_, l, srv := liveEnv(t)
	l.MaxPerIP = 1
	resp, err := http.Get(srv.URL + "/_glim/live/demo-1234")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		r2, err := http.Get(srv.URL + "/_glim/live/demo-1234")
		if err != nil {
			t.Fatal(err)
		}
		r2.Body.Close()
		if r2.StatusCode == 200 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("slot never released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLiveScriptInjection(t *testing.T) {
	st := newPreviewStore(t)
	on := NewRouter(Options{Store: st, LiveReload: true})
	off := NewRouter(Options{Store: st})

	body := get(t, on, "/demo-1234/").Body.String()
	if !strings.Contains(body, `EventSource`) || !strings.Contains(body, `https://glim.example.com/_glim/live/demo-1234?v=`) {
		t.Errorf("script missing: %s", body)
	}
	if !strings.Contains(body, "</script></body>") {
		t.Errorf("script not before </body>: %s", body)
	}
	if js := get(t, on, "/demo-1234/app.js").Body.String(); strings.Contains(js, "EventSource") {
		t.Error("script injected into non-HTML")
	}
	if b := get(t, off, "/demo-1234/").Body.String(); strings.Contains(b, "EventSource") {
		t.Error("script injected with live reload disabled")
	}
	if rec := get(t, off, "/_glim/live/demo-1234"); rec.Code == 200 {
		t.Error("endpoint served with live reload disabled")
	}
}

func TestLiveScriptNotInUnlockPage(t *testing.T) {
	e := newLockEnv(t, false)
	h := NewRouter(Options{Store: e.st, LiveReload: true, Unlock: &Unlock{}})
	rec := get(t, h, "/"+e.slug+"/")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want locked page, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "EventSource") {
		t.Error("unlock page must not carry the live script")
	}
}

// Pin and extend rewrite only the manifest, so they must not reload open tabs
// nor change the version the page was served with.
func TestLiveIgnoresManifestOnlyChanges(t *testing.T) {
	st, _, srv := liveEnv(t)
	before, _ := liveVersion(st, "demo-1234")
	resp := openLive(t, srv, "/_glim/live/demo-1234")
	ch := lines(resp)
	waitFor(t, ch, ": ok")

	time.Sleep(30 * time.Millisecond)
	if err := st.Extend("demo-1234", 3*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := st.Pin("demo-1234"); err != nil {
		t.Fatal(err)
	}
	if after, _ := liveVersion(st, "demo-1234"); after != before {
		t.Fatalf("version changed on manifest-only rewrite: %q -> %q", before, after)
	}
	// Several polls pass with no "changed"; a republish then still reloads.
	deadline := time.After(150 * time.Millisecond)
wait:
	for {
		select {
		case l := <-ch:
			if strings.Contains(l, "event: changed") {
				t.Fatal("manifest-only change reloaded the tab")
			}
		case <-deadline:
			break wait
		}
	}
	publish(t, st, "demo-1234", map[string]string{"index.html": "<html><body>v2</body></html>"}, 2*time.Hour)
	waitFor(t, ch, "event: changed")
}
