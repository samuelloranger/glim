// Package thumb makes the dashboard's picture of each preview: a screenshot
// of its first screen, taken in headless Chrome and kept in the preview's
// folder, so the dashboard shows an image instead of running every preview
// in a frame.
package thumb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/samuelloranger/glim/internal/listen"
	"github.com/samuelloranger/glim/internal/store"
)

const (
	// The page is laid out at Width x Height and saved at Scale, which is
	// sharp enough for a card on a high-density screen.
	Width   = 1280
	Height  = 800
	Scale   = 0.625
	quality = 75
	// settle lets scripts, fonts and images finish after the load event.
	settle = time.Second
	// timeout bounds one capture, browser start included.
	timeout = 30 * time.Second
	// workers is how many pages render at once.
	workers = 2
	// idle is how long the browser stays up after its last capture.
	idle = 2 * time.Minute
	// retryAfter is how long a failed capture is remembered before the next
	// request tries again.
	retryAfter = 5 * time.Minute
)

// ErrOff means no browser is available.
var ErrOff = errors.New("thumbnails are off")

// prefix starts a thumbnail's file name in the preview's folder; the
// preview's version follows, so a capture that finishes after a republish
// is never served for the new version. Dot files are never served, and the
// folder is replaced on republish and removed with the preview.
const prefix = ".glim-thumb-"

func fileName(stamp string) string { return prefix + stamp + ".jpg" }

// Service takes and caches thumbnails. The zero browser path turns it off.
type Service struct {
	st     *store.Store
	chrome string
	logf   func(string, ...any)
	sem    chan struct{}

	mu    sync.Mutex
	calls map[string]*call

	bmu     sync.Mutex
	b       *browser
	users   int
	stopper *time.Timer
}

type call struct {
	done chan struct{}
	err  error
}

// New returns a Service that renders with chrome, a resolved browser path;
// empty turns thumbnails off.
func New(st *store.Store, chrome string, logf func(string, ...any)) *Service {
	return &Service{st: st, chrome: chrome, logf: logf,
		sem: make(chan struct{}, workers), calls: map[string]*call{}}
}

// Enabled reports whether a browser is available.
func (s *Service) Enabled() bool { return s != nil && s.chrome != "" }

// Path returns the file holding the thumbnail of the preview's current
// version, taking the screenshot first when there is none yet. The capture
// carries on if ctx ends, so the next request finds it done.
func (s *Service) Path(ctx context.Context, name string) (string, error) {
	if !s.Enabled() {
		return "", ErrOff
	}
	m, ok := s.st.Live(name)
	if !ok {
		return "", os.ErrNotExist
	}
	stamp := m.Stamp()
	p := filepath.Join(s.st.Dir(name), fileName(stamp))
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	key := name + "@" + stamp
	s.mu.Lock()
	c, ok := s.calls[key]
	if !ok {
		c = &call{done: make(chan struct{})}
		s.calls[key] = c
		go s.take(key, name, stamp, c)
	}
	s.mu.Unlock()
	select {
	case <-c.done:
		if c.err != nil {
			return "", c.err
		}
		return p, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *Service) take(key, name, stamp string, c *call) {
	s.sem <- struct{}{}
	c.err = s.capture(name, stamp)
	<-s.sem
	if c.err != nil {
		s.logf("thumbnail: %s: %v", name, c.err)
	}
	close(c.done)
	forget := func() {
		s.mu.Lock()
		delete(s.calls, key)
		s.mu.Unlock()
	}
	if c.err == nil {
		forget()
	} else {
		time.AfterFunc(retryAfter, forget)
	}
}

func (s *Service) capture(name, stamp string) error {
	b, err := s.acquire()
	if err != nil {
		return err
	}
	defer s.release()
	shot, err := b.shoot(name)
	if err != nil {
		return err
	}
	// A preview republished or removed while it rendered gets no file: the
	// new version takes its own, and a removed folder must not come back.
	if m, ok := s.st.Live(name); !ok || m.Stamp() != stamp {
		return nil
	}
	dir := s.st.Dir(name)
	if err := writeAtomic(dir, fileName(stamp), shot); err != nil {
		return err
	}
	// Drop older versions' files, which an in-place rewrite of the folder
	// would otherwise leave behind.
	old, _ := filepath.Glob(filepath.Join(dir, prefix+"*.jpg"))
	for _, f := range old {
		if filepath.Base(f) != fileName(stamp) {
			os.Remove(f)
		}
	}
	return nil
}

// acquire returns the running browser, starting it if needed, and holds it
// until release.
func (s *Service) acquire() (*browser, error) {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	if s.stopper != nil {
		s.stopper.Stop()
		s.stopper = nil
	}
	if s.b != nil && s.b.ctx.Err() != nil {
		s.b.close()
		s.b = nil
	}
	if s.b == nil {
		b, err := startBrowser(s.chrome, s.st.Root)
		if err != nil {
			return nil, err
		}
		s.b = b
	}
	s.users++
	return s.b, nil
}

// release lets the browser stop once it has been idle for a while, so it
// holds no memory between dashboard visits.
func (s *Service) release() {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	if s.users--; s.users > 0 {
		return
	}
	s.stopper = time.AfterFunc(idle, func() {
		s.bmu.Lock()
		defer s.bmu.Unlock()
		if s.users == 0 && s.b != nil {
			s.b.close()
			s.b = nil
		}
	})
}

// Close stops the browser.
func (s *Service) Close() {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	if s.stopper != nil {
		s.stopper.Stop()
	}
	if s.b != nil {
		s.b.close()
		s.b = nil
	}
}

// browser is a headless Chrome and the loopback file server it renders
// previews from, so pages load their own files without going through the
// public address.
type browser struct {
	ctx         context.Context
	cancel      func()
	cancelAlloc func()
	srv         *http.Server
	base        string
}

func startBrowser(chrome, root string) (*browser, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	files := http.FileServer(http.Dir(root))
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, seg := range strings.Split(r.URL.Path, "/") {
			if strings.HasPrefix(seg, ".") {
				http.NotFound(w, r)
				return
			}
		}
		h := w.Header()
		h.Set("Content-Security-Policy", listen.SandboxCSP)
		// The sandbox gives the page an opaque origin; let it fetch its own
		// files, as the real server does.
		if r.Header.Get("Origin") == "null" {
			h.Set("Access-Control-Allow-Origin", "null")
		}
		files.ServeHTTP(w, r)
	})}
	go srv.Serve(ln)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chrome),
		chromedp.WindowSize(Width, Height),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
	)
	if os.Geteuid() == 0 {
		// Chrome refuses to start its sandbox as root (typical in a container).
		opts = append(opts, chromedp.NoSandbox)
	}
	actx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(actx)
	b := &browser{ctx: ctx, cancel: cancel, cancelAlloc: cancelAlloc, srv: srv,
		base: "http://" + ln.Addr().String()}
	if err := chromedp.Run(ctx); err != nil {
		b.close()
		return nil, &listen.StartError{Browser: chrome, Err: err}
	}
	return b, nil
}

func (b *browser) close() {
	b.cancel()
	b.cancelAlloc()
	b.srv.Close()
}

// shoot renders /name/ in a new tab and returns its first screen as JPEG.
func (b *browser) shoot(name string) ([]byte, error) {
	tab, cancel := chromedp.NewContext(b.ctx)
	defer cancel()
	ctx, stop := context.WithTimeout(tab, timeout)
	defer stop()
	var out []byte
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(Width, Height),
		chromedp.Navigate(b.base+"/"+name+"/"),
		chromedp.Sleep(settle),
		chromedp.ActionFunc(func(ctx context.Context) error {
			shot, err := page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(quality).
				WithClip(&page.Viewport{Width: Width, Height: Height, Scale: Scale}).
				Do(ctx)
			out = shot
			return err
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("render the page: %w", err)
	}
	return out, nil
}

func writeAtomic(dir, name string, data []byte) error {
	f, err := os.CreateTemp(dir, name+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, name))
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
