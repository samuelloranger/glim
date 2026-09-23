package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

func publishPreview(t *testing.T, st *store.Store, title string, ttl time.Duration) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.html")
	os.WriteFile(p, []byte("<p>"+title+"</p>"), 0o644)
	res, err := st.Publish(p, title, "proj", "", ttl, "")
	if err != nil {
		t.Fatal(err)
	}
	return res.Name
}

func TestBuildSnapshot(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	snap, err := BuildSnapshot(st)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(snap)
	if !strings.Contains(string(b), `"previews":[]`) || !strings.Contains(string(b), `"nextExpiry":null`) {
		t.Fatalf("empty snapshot json = %s", b)
	}
	short := publishPreview(t, st, "Short", time.Hour)
	long := publishPreview(t, st, "Long", 5*time.Hour)
	st.Pin(long)
	snap, _ = BuildSnapshot(st)
	if snap.Status.Live != 2 || snap.Status.Pinned != 1 || snap.Status.DiskBytes <= 0 {
		t.Fatalf("status = %+v", snap.Status)
	}
	m, _ := st.Get(short)
	if snap.Status.NextExpiry == nil || !snap.Status.NextExpiry.Equal(m.Expires) {
		t.Fatalf("nextExpiry = %v, want %v", snap.Status.NextExpiry, m.Expires)
	}
	for _, p := range snap.Previews {
		if p.URL != "https://glim.example.com/"+p.Name+"/" || p.Project != "proj" {
			t.Fatalf("preview = %+v", p)
		}
	}
}

func recv(t *testing.T, ch <-chan Snapshot) Snapshot {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(2 * time.Second):
		t.Fatal("no snapshot")
		return Snapshot{}
	}
}

func nothing(t *testing.T, ch <-chan Snapshot) {
	t.Helper()
	select {
	case s := <-ch:
		t.Fatalf("unexpected snapshot: %+v", s.Status)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestHubBroadcastsOnlyOnChange(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	now := time.Now()
	st.Now = func() time.Time { return now }
	h := NewHub(st, nil, time.Hour, t.Logf)
	ch, cur, cancel := h.Subscribe()
	defer cancel()
	if cur.Status.Live != 0 {
		t.Fatalf("initial = %+v", cur.Status)
	}
	h.Rescan(false) // first build
	recv(t, ch)
	h.Rescan(false)
	nothing(t, ch)

	publishPreview(t, st, "One", time.Minute)
	h.Rescan(false)
	if s := recv(t, ch); s.Status.Live != 1 {
		t.Fatalf("after publish = %+v", s.Status)
	}
	h.Rescan(false)
	nothing(t, ch)

	now = now.Add(2 * time.Minute) // expired, files still on disk
	h.Rescan(false)
	if s := recv(t, ch); s.Status.Live != 0 {
		t.Fatalf("after expiry = %+v", s.Status)
	}
	h.Rescan(true)
	recv(t, ch)
}

func TestHubKeepsOnlyLatestForSlowSubscriber(t *testing.T) {
	st := store.New(t.TempDir(), "https://glim.example.com")
	h := NewHub(st, nil, time.Hour, t.Logf)
	ch, _, cancel := h.Subscribe()
	defer cancel()
	publishPreview(t, st, "A", time.Hour)
	h.Rescan(true)
	publishPreview(t, st, "B", time.Hour)
	h.Rescan(true)
	if s := recv(t, ch); s.Status.Live != 2 {
		t.Fatalf("latest = %+v", s.Status)
	}
	nothing(t, ch)
}
