package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/samuelloranger/glim/internal/auth"
)

const (
	// NotifyDebounce is the minimum gap between two notifications for one preview.
	NotifyDebounce = 30 * time.Second
	notifyQueueLen = 16
	notifyFanout   = 4
	sendTimeout    = 15 * time.Second
)

// PushSender delivers one encrypted web push message and reports the push
// service's HTTP status.
type PushSender interface {
	Send(ctx context.Context, sub auth.PushSub, payload []byte) (status int, err error)
}

// Notifier turns hub snapshots into web push notifications: one per new
// preview or republish, to every subscription. Publishing happens in other
// processes, so it only sees changes the hub's rescans see. Sends run on one
// worker goroutine (see Run) fed by a bounded queue.
type Notifier struct {
	db      *auth.DB
	sender  PushSender
	logf    func(string, ...any)
	now     func() time.Time
	after   func(d time.Duration, f func())
	timeout time.Duration
	queue   chan []pushPayload
	jobs    sync.WaitGroup // queued or running jobs; lets tests wait for idle

	mu      sync.Mutex
	seen    map[string]string       // preview name -> last observed stamp
	last    map[string]time.Time    // preview name -> last notification
	pending map[string]*pendingNote // changes held back by the debounce
	ready   bool
}

type pendingNote struct {
	msg   pushPayload
	timer bool // a flush timer is already armed
}

func NewNotifier(db *auth.DB, sender PushSender, logf func(string, ...any)) *Notifier {
	return &Notifier{db: db, sender: sender, logf: logf, now: time.Now,
		after:   func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		timeout: sendTimeout,
		queue:   make(chan []pushPayload, notifyQueueLen),
		seen:    map[string]string{}, last: map[string]time.Time{}, pending: map[string]*pendingNote{}}
}

type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// Run is the single send worker; it returns when ctx ends.
func (n *Notifier) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msgs := <-n.queue:
			n.sendAll(ctx, msgs)
			n.jobs.Done()
		}
	}
}

// Observe compares a snapshot with the previous one. The first snapshot only
// sets the baseline, so previews that predate the server never notify. A
// change inside a preview's debounce window is held and sent once the window
// ends, carrying the latest state.
func (n *Notifier) Observe(snap Snapshot) {
	n.mu.Lock()
	now := n.now()
	current := make(map[string]string, len(snap.Previews))
	for _, p := range snap.Previews {
		current[p.Name] = p.Stamp
		if !n.ready {
			continue
		}
		old, existed := n.seen[p.Name]
		if existed && old == p.Stamp {
			continue
		}
		verb := "published"
		if existed {
			verb = "updated"
		}
		name := strings.TrimSpace(p.Title)
		if name == "" {
			name = p.Name
		}
		msg := pushPayload{Title: "glim", Body: name + " " + verb, URL: p.URL}
		if pn := n.pending[p.Name]; pn != nil {
			if strings.HasSuffix(pn.msg.Body, " published") {
				msg.Body = name + " published"
			}
			pn.msg = msg
			continue
		}
		n.pending[p.Name] = &pendingNote{msg: msg}
	}
	n.seen, n.ready = current, true
	for name := range n.pending {
		if _, ok := current[name]; !ok {
			delete(n.pending, name)
		}
	}
	out := n.dueLocked(now)
	n.mu.Unlock()
	n.submit(out)
}

// dueLocked takes every pending note whose window has ended and arms a timer
// for the rest. Callers hold n.mu.
func (n *Notifier) dueLocked(now time.Time) []pushPayload {
	var out []pushPayload
	for name, pn := range n.pending {
		wait := time.Duration(0)
		if t, ok := n.last[name]; ok {
			wait = NotifyDebounce - now.Sub(t)
		}
		if wait <= 0 {
			out = append(out, pn.msg)
			n.last[name] = now
			delete(n.pending, name)
			continue
		}
		if !pn.timer {
			pn.timer = true
			n.after(wait, n.flush)
		}
	}
	for name, t := range n.last {
		if _, held := n.pending[name]; !held && now.Sub(t) >= NotifyDebounce {
			delete(n.last, name)
		}
	}
	return out
}

// flush sends held notifications whose window has ended. It runs from timers
// so a republish is announced even when nothing else changes afterwards.
func (n *Notifier) flush() {
	n.mu.Lock()
	for _, pn := range n.pending {
		pn.timer = false
	}
	out := n.dueLocked(n.now())
	n.mu.Unlock()
	n.submit(out)
}

// submit queues one job; when the worker is backed up the job is dropped.
func (n *Notifier) submit(msgs []pushPayload) {
	if len(msgs) == 0 {
		return
	}
	n.jobs.Add(1)
	select {
	case n.queue <- msgs:
	default:
		n.jobs.Done()
		n.logf("push: send queue full, dropping %d notification(s)", len(msgs))
	}
}

func (n *Notifier) sendAll(ctx context.Context, msgs []pushPayload) {
	subs, err := n.db.PushSubs(ctx)
	if err != nil {
		n.logf("push: list subscriptions: %v", err)
		return
	}
	sem := make(chan struct{}, notifyFanout)
	var wg sync.WaitGroup
	for _, m := range msgs {
		body, _ := json.Marshal(m)
		for _, sub := range subs {
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer func() { <-sem; wg.Done() }()
				n.sendOne(ctx, sub, body)
			}()
		}
	}
	wg.Wait()
}

func (n *Notifier) sendOne(ctx context.Context, sub auth.PushSub, body []byte) {
	cctx, cancel := context.WithTimeout(ctx, n.timeout)
	status, err := n.sender.Send(cctx, sub, body)
	cancel()
	switch {
	case status == http.StatusNotFound || status == http.StatusGone:
		if derr := n.db.DeletePushEndpoint(ctx, sub.Endpoint); derr != nil {
			n.logf("push: drop dead subscription: %v", derr)
		}
	case err != nil:
		n.logf("push: send: %v", err)
	case status >= 300:
		n.logf("push: send: push service answered %d", status)
	}
}

// WebPushSender sends through webpush-go with the server's VAPID keys.
type WebPushSender struct {
	DB *auth.DB
	// Subscriber is the VAPID "sub" claim: a mailto: or https: contact URL.
	Subscriber string
	// Client reaches the push service; nil selects the SSRF-guarded client.
	Client *http.Client
	once   sync.Once
}

func (s *WebPushSender) Send(ctx context.Context, sub auth.PushSub, payload []byte) (int, error) {
	s.once.Do(func() {
		if s.Client == nil {
			s.Client = newPushHTTPClient()
		}
	})
	priv, pub, err := s.DB.VAPIDKeys(ctx)
	if err != nil {
		return 0, err
	}
	res, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
	}, &webpush.Options{
		HTTPClient: s.Client,
		Subscriber: s.Subscriber, VAPIDPublicKey: pub, VAPIDPrivateKey: priv,
		TTL: 3600, Urgency: webpush.UrgencyNormal,
	})
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	return res.StatusCode, nil
}
