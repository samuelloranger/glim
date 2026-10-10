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

// NotifyDebounce is the minimum gap between two notifications for one preview.
const NotifyDebounce = 30 * time.Second

// PushSender delivers one encrypted web push message and reports the push
// service's HTTP status.
type PushSender interface {
	Send(ctx context.Context, sub auth.PushSub, payload []byte) (status int, err error)
}

// Notifier turns hub snapshots into web push notifications: one per new
// preview or republish, to every subscription. Publishing happens in other
// processes, so it only sees changes the hub's rescans see.
type Notifier struct {
	db     *auth.DB
	sender PushSender
	logf   func(string, ...any)
	now    func() time.Time
	// run executes a send job; the default is a goroutine so the hub never
	// waits on a push service. Tests substitute a synchronous runner.
	run func(func())

	mu    sync.Mutex
	seen  map[string]string    // preview name -> last seen stamp
	last  map[string]time.Time // preview name -> last notification
	ready bool
}

func NewNotifier(db *auth.DB, sender PushSender, logf func(string, ...any)) *Notifier {
	return &Notifier{db: db, sender: sender, logf: logf, now: time.Now,
		run:  func(f func()) { go f() },
		seen: map[string]string{}, last: map[string]time.Time{}}
}

type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// Observe compares a snapshot with the previous one. The first snapshot only
// sets the baseline, so previews that predate the server never notify.
func (n *Notifier) Observe(snap Snapshot) {
	n.mu.Lock()
	var out []pushPayload
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
		if t, ok := n.last[p.Name]; ok && now.Sub(t) < NotifyDebounce {
			continue
		}
		n.last[p.Name] = now
		verb := "published"
		if existed {
			verb = "updated"
		}
		name := strings.TrimSpace(p.Title)
		if name == "" {
			name = p.Name
		}
		out = append(out, pushPayload{Title: "glim", Body: name + " " + verb, URL: p.URL})
	}
	n.seen, n.ready = current, true
	for name, t := range n.last {
		if now.Sub(t) >= NotifyDebounce {
			delete(n.last, name)
		}
	}
	n.mu.Unlock()
	if len(out) > 0 {
		n.run(func() { n.sendAll(out) })
	}
}

func (n *Notifier) sendAll(msgs []pushPayload) {
	ctx := context.Background()
	subs, err := n.db.PushSubs(ctx)
	if err != nil {
		n.logf("push: list subscriptions: %v", err)
		return
	}
	for _, m := range msgs {
		body, _ := json.Marshal(m)
		for _, sub := range subs {
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
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
	}
}

// WebPushSender sends through webpush-go with the server's VAPID keys.
type WebPushSender struct {
	DB *auth.DB
	// Subscriber is the VAPID "sub" claim: a mailto: or https: contact URL.
	Subscriber string
}

func (s WebPushSender) Send(ctx context.Context, sub auth.PushSub, payload []byte) (int, error) {
	priv, pub, err := s.DB.VAPIDKeys(ctx)
	if err != nil {
		return 0, err
	}
	res, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
	}, &webpush.Options{
		Subscriber: s.Subscriber, VAPIDPublicKey: pub, VAPIDPrivateKey: priv,
		TTL: 3600, Urgency: webpush.UrgencyNormal,
	})
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	return res.StatusCode, nil
}
