package api

import (
	"context"
	"sync"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

// Hub watches the preview root and fans snapshots out to SSE subscribers.
// Publishes happen in other processes, so it polls a cheap fingerprint.
type Hub struct {
	st       *store.Store
	auth     *auth.DB
	interval time.Duration
	logf     func(string, ...any)
	poke     chan struct{}

	mu      sync.Mutex
	subs    map[chan Snapshot]struct{}
	current Snapshot
	fp      string
	built   bool
}

func NewHub(st *store.Store, a *auth.DB, interval time.Duration, logf func(string, ...any)) *Hub {
	return &Hub{st: st, auth: a, interval: interval, logf: logf,
		poke: make(chan struct{}, 1), subs: map[chan Snapshot]struct{}{}}
}

func (h *Hub) Run(ctx context.Context) {
	h.Rescan(true)
	tick := time.NewTicker(h.interval)
	defer tick.Stop()
	prune := time.NewTicker(time.Minute)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			h.Rescan(false)
		case <-h.poke:
			h.Rescan(true)
		case <-prune.C:
			if h.auth != nil {
				if _, err := h.auth.PruneSessions(ctx); err != nil {
					h.logf("hub: prune sessions: %v", err)
				}
			}
		}
	}
}

// Poke asks Run to rebuild and broadcast now (after a dashboard mutation).
func (h *Hub) Poke() {
	select {
	case h.poke <- struct{}{}:
	default:
	}
}

// Rescan rebuilds and broadcasts when previews changed on disk, the earliest
// expiry has passed, or force is set.
func (h *Hub) Rescan(force bool) {
	fp, err := h.st.Fingerprint()
	if err != nil {
		h.logf("hub: %v", err)
		return
	}
	h.mu.Lock()
	next := h.current.Status.NextExpiry
	stale := force || !h.built || fp != h.fp || (next != nil && !storeNow(h.st).Before(*next))
	h.mu.Unlock()
	if !stale {
		return
	}
	snap, err := BuildSnapshot(h.st)
	if err != nil {
		h.logf("hub: %v", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fp, h.current, h.built = fp, snap, true
	for ch := range h.subs {
		offer(ch, snap)
	}
}

// offer replaces any unread snapshot so a slow client only ever gets the latest.
func offer(ch chan Snapshot, s Snapshot) {
	select {
	case ch <- s:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- s:
	default:
	}
}

func (h *Hub) Subscribe() (<-chan Snapshot, Snapshot, func()) {
	ch := make(chan Snapshot, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	cur, built := h.current, h.built
	h.mu.Unlock()
	if !built {
		if snap, err := BuildSnapshot(h.st); err == nil {
			cur = snap
		}
	}
	return ch, cur, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}
