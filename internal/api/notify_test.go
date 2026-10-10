package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type fakeSender struct {
	mu     sync.Mutex
	sent   []pushPayload
	to     []string
	status map[string]int
}

func (f *fakeSender) Send(_ context.Context, sub auth.PushSub, payload []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var p pushPayload
	_ = json.Unmarshal(payload, &p)
	f.sent = append(f.sent, p)
	f.to = append(f.to, sub.Endpoint)
	if s, ok := f.status[sub.Endpoint]; ok {
		return s, nil
	}
	return http.StatusCreated, nil
}

type notifyEnv struct {
	db     *auth.DB
	st     *store.Store
	hub    *Hub
	n      *Notifier
	sender *fakeSender
	now    time.Time
}

func newNotifyEnv(t *testing.T) *notifyEnv {
	t.Helper()
	db, err := auth.Open(filepath.Join(t.TempDir(), "glim.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.BcryptCost = bcrypt.MinCost
	t.Cleanup(func() { db.Close() })
	u, err := db.CreateUser(t.Context(), "a@example.com", testPass)
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range []string{"https://push.example/1", "https://push.example/2"} {
		if err := db.SavePushSub(t.Context(), auth.PushSub{Endpoint: ep, P256dh: "p", Auth: "a", UserID: u.ID}); err != nil {
			t.Fatal(err)
		}
	}
	e := &notifyEnv{db: db, st: store.New(t.TempDir(), "https://glim.example.com"),
		sender: &fakeSender{status: map[string]int{}}, now: time.Now()}
	e.n = NewNotifier(db, e.sender, t.Logf)
	e.n.run = func(f func()) { f() }
	e.n.now = func() time.Time { return e.now }
	e.hub = NewHub(e.st, db, time.Hour, t.Logf)
	e.hub.SetNotifier(e.n)
	return e
}

func (e *notifyEnv) publish(t *testing.T, title, name string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.html")
	if err := os.WriteFile(p, []byte("<p>"+title+time.Now().String()+"</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Publish(p, title, "", "", time.Hour, name); err != nil {
		t.Fatal(err)
	}
}

func (e *notifyEnv) bodies() []string {
	var out []string
	for _, p := range e.sender.sent {
		out = append(out, p.Body)
	}
	return out
}

func TestNotifyIgnoresInitialSnapshot(t *testing.T) {
	e := newNotifyEnv(t)
	e.publish(t, "Existing", "")
	e.hub.Rescan(true)
	e.hub.Rescan(true)
	if len(e.sender.sent) != 0 {
		t.Fatalf("sent %v for a preview that predates the server", e.bodies())
	}
}

func TestNotifyNewPreviewReachesEverySubscription(t *testing.T) {
	e := newNotifyEnv(t)
	e.hub.Rescan(true)
	e.publish(t, "Fresh", "")
	e.hub.Rescan(true)
	if len(e.sender.sent) != 2 {
		t.Fatalf("sent %d, want one per subscription", len(e.sender.sent))
	}
	m := e.sender.sent[0]
	if m.Title != "glim" || m.Body != "Fresh published" || m.URL == "" || m.URL[:len("https://glim.example.com/")] != "https://glim.example.com/" {
		t.Fatalf("payload = %+v", m)
	}
	e.hub.Rescan(true)
	if len(e.sender.sent) != 2 {
		t.Fatalf("an unchanged rescan notified again: %v", e.bodies())
	}
}

func TestNotifyRepublishSaysUpdated(t *testing.T) {
	e := newNotifyEnv(t)
	e.publish(t, "Doc", "doc")
	e.hub.Rescan(true)
	e.now = e.now.Add(time.Minute)
	e.publish(t, "Doc", "doc")
	e.hub.Rescan(true)
	if len(e.sender.sent) != 2 || e.sender.sent[0].Body != "Doc updated" {
		t.Fatalf("sent %v", e.bodies())
	}
}

func TestNotifyDebouncesPerPreview(t *testing.T) {
	e := newNotifyEnv(t)
	e.hub.Rescan(true)
	e.publish(t, "Doc", "doc")
	e.hub.Rescan(true)
	e.now = e.now.Add(10 * time.Second)
	e.publish(t, "Doc", "doc")
	e.hub.Rescan(true)
	if len(e.sender.sent) != 2 {
		t.Fatalf("republish inside the window notified: %v", e.bodies())
	}
	e.publish(t, "Other", "other")
	e.hub.Rescan(true)
	if len(e.sender.sent) != 4 {
		t.Fatalf("another preview was debounced: %v", e.bodies())
	}
	e.now = e.now.Add(NotifyDebounce)
	e.publish(t, "Doc", "doc")
	e.hub.Rescan(true)
	if len(e.sender.sent) != 6 || e.sender.sent[4].Body != "Doc updated" {
		t.Fatalf("after the window: %v", e.bodies())
	}
}

func TestNotifyDropsGoneSubscriptions(t *testing.T) {
	e := newNotifyEnv(t)
	e.sender.status["https://push.example/1"] = http.StatusGone
	e.hub.Rescan(true)
	e.publish(t, "Fresh", "")
	e.hub.Rescan(true)
	subs, err := e.db.PushSubs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].Endpoint != "https://push.example/2" {
		t.Fatalf("subs = %+v", subs)
	}
}

func TestNotifyDoesNotBlockTheHub(t *testing.T) {
	e := newNotifyEnv(t)
	block := make(chan struct{})
	done := make(chan struct{})
	e.n.run = func(f func()) {
		go func() { <-block; f(); close(done) }()
	}
	e.hub.Rescan(true)
	e.publish(t, "Fresh", "")
	e.hub.Rescan(true) // would deadlock here if sending were inline
	close(block)
	<-done
}

func TestDeletingUserRemovesSubscriptions(t *testing.T) {
	e := newNotifyEnv(t)
	if err := e.db.DeleteUser(t.Context(), "a@example.com"); err != nil {
		t.Fatal(err)
	}
	subs, err := e.db.PushSubs(t.Context())
	if err != nil || len(subs) != 0 {
		t.Fatalf("subs = %+v, err %v", subs, err)
	}
}
