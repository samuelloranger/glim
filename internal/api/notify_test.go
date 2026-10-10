package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
	e.n.after = func(time.Duration, func()) {}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go e.n.Run(ctx)
	e.n.now = func() time.Time { return e.now }
	e.hub = NewHub(e.st, db, time.Hour, t.Logf)
	e.hub.SetNotifier(e.n)
	return e
}

// rescan runs a hub rescan and waits for the notifier to finish sending.
func (e *notifyEnv) rescan() {
	e.hub.Rescan(true)
	e.n.jobs.Wait()
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
	e.rescan()
	e.rescan()
	if len(e.sender.sent) != 0 {
		t.Fatalf("sent %v for a preview that predates the server", e.bodies())
	}
}

func TestNotifyNewPreviewReachesEverySubscription(t *testing.T) {
	e := newNotifyEnv(t)
	e.rescan()
	e.publish(t, "Fresh", "")
	e.rescan()
	if len(e.sender.sent) != 2 {
		t.Fatalf("sent %d, want one per subscription", len(e.sender.sent))
	}
	m := e.sender.sent[0]
	if m.Title != "glim" || m.Body != "Fresh published" || m.URL == "" || m.URL[:len("https://glim.example.com/")] != "https://glim.example.com/" {
		t.Fatalf("payload = %+v", m)
	}
	e.rescan()
	if len(e.sender.sent) != 2 {
		t.Fatalf("an unchanged rescan notified again: %v", e.bodies())
	}
}

func TestNotifyRepublishSaysUpdated(t *testing.T) {
	e := newNotifyEnv(t)
	e.publish(t, "Doc", "doc")
	e.rescan()
	e.now = e.now.Add(time.Minute)
	e.publish(t, "Doc", "doc")
	e.rescan()
	if len(e.sender.sent) != 2 || e.sender.sent[0].Body != "Doc updated" {
		t.Fatalf("sent %v", e.bodies())
	}
}

func TestNotifyDebouncesPerPreview(t *testing.T) {
	e := newNotifyEnv(t)
	e.rescan()
	e.publish(t, "Doc", "doc")
	e.rescan()
	e.now = e.now.Add(10 * time.Second)
	e.publish(t, "Doc", "doc")
	e.rescan()
	if len(e.sender.sent) != 2 {
		t.Fatalf("republish inside the window notified: %v", e.bodies())
	}
	e.publish(t, "Other", "other")
	e.rescan()
	if len(e.sender.sent) != 4 {
		t.Fatalf("another preview was debounced: %v", e.bodies())
	}
	e.now = e.now.Add(NotifyDebounce)
	e.publish(t, "Doc", "doc")
	e.rescan()
	if len(e.sender.sent) != 6 || e.sender.sent[4].Body != "Doc updated" {
		t.Fatalf("after the window: %v", e.bodies())
	}
}

func TestNotifyDropsGoneSubscriptions(t *testing.T) {
	e := newNotifyEnv(t)
	e.sender.status["https://push.example/1"] = http.StatusGone
	e.rescan()
	e.publish(t, "Fresh", "")
	e.rescan()
	subs, err := e.db.PushSubs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].Endpoint != "https://push.example/2" {
		t.Fatalf("subs = %+v", subs)
	}
}

func TestNotifyHeldRepublishIsSentWhenWindowEnds(t *testing.T) {
	e := newNotifyEnv(t)
	e.rescan()
	e.publish(t, "Doc", "doc")
	e.rescan() // published at +0
	e.now = e.now.Add(10 * time.Second)
	e.publish(t, "Doc", "doc")
	e.rescan() // held
	if len(e.sender.sent) != 2 {
		t.Fatalf("held republish was sent early: %v", e.bodies())
	}
	e.now = e.now.Add(21 * time.Second) // +31s
	e.rescan()
	if len(e.sender.sent) != 4 || e.sender.sent[2].Body != "Doc updated" {
		t.Fatalf("after the window: %v", e.bodies())
	}
	e.now = e.now.Add(time.Minute)
	e.rescan()
	if len(e.sender.sent) != 4 {
		t.Fatalf("sent again: %v", e.bodies())
	}
}

func TestNotifyHeldRepublishFlushesOnTimer(t *testing.T) {
	e := newNotifyEnv(t)
	var fire func()
	var wait time.Duration
	e.n.after = func(d time.Duration, f func()) { wait, fire = d, f }
	e.rescan()
	e.publish(t, "Doc", "doc")
	e.rescan()
	e.now = e.now.Add(10 * time.Second)
	e.publish(t, "Doc", "doc")
	e.rescan()
	if fire == nil || wait != 20*time.Second {
		t.Fatalf("timer = %v armed=%v", wait, fire != nil)
	}
	e.now = e.now.Add(20 * time.Second)
	fire()
	e.n.jobs.Wait()
	if len(e.sender.sent) != 4 || e.sender.sent[2].Body != "Doc updated" {
		t.Fatalf("sent %v", e.bodies())
	}
}

type hangSender struct{ release chan struct{} }

func (h hangSender) Send(context.Context, auth.PushSub, []byte) (int, error) {
	<-h.release
	return 201, nil
}

func TestNotifyQueueIsBounded(t *testing.T) {
	e := newNotifyEnv(t)
	h := hangSender{release: make(chan struct{})}
	e.n.sender = h
	e.n.timeout = time.Hour
	var dropped int
	var mu sync.Mutex
	e.n.logf = func(f string, a ...any) { mu.Lock(); dropped++; mu.Unlock() }
	e.hub.Rescan(true)
	before := runtime.NumGoroutine()
	for i := 0; i < 60; i++ {
		e.n.Observe(Snapshot{Previews: []Preview{{Name: fmt.Sprintf("p%d", i), Stamp: "s", URL: "u"}}})
	}
	if grew := runtime.NumGoroutine() - before; grew > 10 {
		t.Fatalf("goroutines grew by %d", grew)
	}
	mu.Lock()
	d := dropped
	mu.Unlock()
	if d < 60-notifyQueueLen-2 {
		t.Fatalf("dropped %d of 60 with a hung sender", d)
	}
	close(h.release)
	e.n.jobs.Wait()
}

func TestNotifyFanoutIsCapped(t *testing.T) {
	e := newNotifyEnv(t)
	for i := 0; i < 20; i++ {
		_ = e.db.SavePushSub(t.Context(), auth.PushSub{Endpoint: fmt.Sprintf("https://push.example/x%d", i), P256dh: "p", Auth: "a", UserID: 1})
	}
	c := &countSender{release: make(chan struct{})}
	e.n.sender = c
	e.rescan()
	e.publish(t, "Fresh", "")
	e.hub.Rescan(true)
	time.Sleep(100 * time.Millisecond)
	if got := c.max(); got != notifyFanout {
		t.Fatalf("concurrent sends = %d, want %d", got, notifyFanout)
	}
	close(c.release)
	e.n.jobs.Wait()
}

type countSender struct {
	release chan struct{}
	mu      sync.Mutex
	cur, hi int
}

func (c *countSender) Send(context.Context, auth.PushSub, []byte) (int, error) {
	c.mu.Lock()
	c.cur++
	c.hi = max(c.hi, c.cur)
	c.mu.Unlock()
	<-c.release
	c.mu.Lock()
	c.cur--
	c.mu.Unlock()
	return 201, nil
}

func (c *countSender) max() int { c.mu.Lock(); defer c.mu.Unlock(); return c.hi }

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
