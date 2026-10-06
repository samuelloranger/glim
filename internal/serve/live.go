package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

// LivePathPrefix is where the live-reload event stream is served; the slug
// follows it.
const LivePathPrefix = "/_glim/live/"

// Live serves GET /_glim/live/<slug>, a Server-Sent Events stream that tells
// an open preview tab when its preview was republished ("changed") or removed
// or expired ("gone"). It carries no content, needs no cookie and sets
// Access-Control-Allow-Origin: * because previews run in an opaque origin.
type Live struct {
	st *store.Store
	// Poll is how often a stream re-checks its manifest; Heartbeat how often it
	// writes a comment to keep proxies from closing an idle connection.
	Poll, Heartbeat time.Duration
	// MaxTotal and MaxPerIP cap concurrent streams; beyond them the endpoint
	// answers 429.
	MaxTotal, MaxPerIP int

	mu    sync.Mutex
	total int
	perIP map[string]int
}

// NewLive returns a Live with production defaults.
func NewLive(st *store.Store) *Live {
	return &Live{st: st, Poll: time.Second, Heartbeat: 25 * time.Second,
		MaxTotal: 200, MaxPerIP: 20, perIP: map[string]int{}}
}

// liveVersion fingerprints a preview's manifest (mtime and size), the signal
// every republish changes.
func liveVersion(st *store.Store, slug string) (string, bool) {
	info, err := os.Stat(filepath.Join(st.Root, slug, store.ManifestFile))
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size()), true
}

func (l *Live) acquire(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= l.MaxTotal || l.perIP[ip] >= l.MaxPerIP {
		return false
	}
	l.total++
	l.perIP[ip]++
	return true
}

func (l *Live) release(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total--
	if l.perIP[ip]--; l.perIP[ip] <= 0 {
		delete(l.perIP, ip)
	}
}

func (l *Live) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, LivePathPrefix)
	if !store.ValidName(slug) {
		http.NotFound(w, r)
		return
	}
	if _, ok := l.st.Live(slug); !ok {
		http.NotFound(w, r)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ip := auth.ClientIP(r)
	if !l.acquire(ip) {
		h.Set("Retry-After", "30")
		http.Error(w, "too many live streams", http.StatusTooManyRequests)
		return
	}
	defer l.release(ip)

	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	last, _ := liveVersion(l.st, slug)
	// A page that loaded before a republish passes the version it was served
	// with, so the change is not missed between load and connect.
	if v := r.URL.Query().Get("v"); v != "" && v != last {
		_, _ = fmt.Fprint(w, "event: changed\ndata: 1\n\n")
	} else {
		_, _ = fmt.Fprint(w, ": ok\n\n")
	}
	fl.Flush()

	poll := time.NewTicker(l.Poll)
	defer poll.Stop()
	beat := time.NewTicker(l.Heartbeat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-beat.C:
			_, _ = fmt.Fprint(w, ": hb\n\n")
			fl.Flush()
		case <-poll.C:
			if _, live := l.st.Live(slug); !live {
				_, _ = fmt.Fprint(w, "event: gone\ndata: 1\n\n")
				fl.Flush()
				return
			}
			if v, ok := liveVersion(l.st, slug); ok && v != last {
				last = v
				_, _ = fmt.Fprint(w, "event: changed\ndata: 1\n\n")
				fl.Flush()
			}
		}
	}
}

// liveScript is the snippet injected into a live preview's HTML pages. It is
// an IIFE, so it leaks no globals, and it swallows every error so it can never
// break the page it rides on.
func liveScript(streamURL string) string {
	u, _ := json.Marshal(streamURL) // escapes <, > and & so it is safe inline
	return `<script>(function(){try{var E=window.EventSource;if(!E)return;var u=` + string(u) +
		`,n=0,d;function c(){var s=new E(u);s.onopen=function(){n=0};` +
		`s.addEventListener("changed",function(){clearTimeout(d);d=setTimeout(function(){location.reload()},200)});` +
		`s.addEventListener("gone",function(){s.close()});` +
		`s.onerror=function(){s.close();if(++n>8)return;setTimeout(c,Math.min(30000,1000*Math.pow(2,n)))}}c()}catch(e){}})();</script>`
}
