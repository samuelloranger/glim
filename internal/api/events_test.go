package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
)

type sse struct {
	resp *http.Response
	rd   *bufio.Reader
}

func openEvents(t *testing.T, srv *httptest.Server, s auth.Session) *sse {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/_glim/api/events", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token})
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("events = %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	return &sse{resp: resp, rd: bufio.NewReader(resp.Body)}
}

// next returns the next snapshot event, skipping comments; io.EOF when closed.
func (s *sse) next(t *testing.T) (Snapshot, error) {
	t.Helper()
	var event string
	for {
		line, err := s.rd.ReadString('\n')
		if err != nil {
			return Snapshot{}, err
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: ") && event == "snapshot":
			var snap Snapshot
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &snap); err != nil {
				t.Fatal(err)
			}
			return snap, nil
		}
	}
}

func liveEnv(t *testing.T) (*env, *httptest.Server) {
	t.Helper()
	var hub *Hub
	e := newEnvWith(t, func(d *Deps) {
		hub = NewHub(d.Store, d.Auth, 20*time.Millisecond, t.Logf)
		d.Hub = hub
		d.CheckEvery = 20 * time.Millisecond
		d.PingEvery = 30 * time.Millisecond
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	srv := httptest.NewServer(e.srv)
	t.Cleanup(srv.Close)
	return e, srv
}

func TestEventsStreamsSnapshots(t *testing.T) {
	e, srv := liveEnv(t)
	s := e.signIn("sam")
	stream := openEvents(t, srv, s)
	first, err := stream.next(t)
	if err != nil || first.Status.Live != 0 {
		t.Fatalf("first = %+v, %v", first.Status, err)
	}
	publishPreview(t, e.st, "From the CLI", time.Hour) // another process in real life
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := stream.next(t)
		if err != nil {
			t.Fatal(err)
		}
		if snap.Status.Live == 1 {
			return
		}
	}
	t.Fatal("publish never reached the stream")
}

func TestEventsCloseAfterSessionDeleted(t *testing.T) {
	e, srv := liveEnv(t)
	s := e.signIn("sam")
	stream := openEvents(t, srv, s)
	if _, err := stream.next(t); err != nil {
		t.Fatal(err)
	}
	if err := e.db.DeleteSession(t.Context(), s.Token); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, stream.rd)
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream stayed open after the session was deleted")
	}
}

func TestEventsNeedSession(t *testing.T) {
	_, srv := liveEnv(t)
	resp, err := http.Get(srv.URL + "/_glim/api/events")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("anon events = %d", resp.StatusCode)
	}
}
