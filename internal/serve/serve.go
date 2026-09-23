package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

// PreviewCSP runs every preview as an opaque origin, top-level or framed, so a
// preview's scripts can never act with the dashboard's origin.
const PreviewCSP = "sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads"

// PreviewHandler serves live previews from st.Root. Unknown, invalid or
// expired slugs, directory listings and any dot-prefixed path segment 404.
func PreviewHandler(st *store.Store) http.Handler {
	files := http.FileServer(noListFS{http.Dir(st.Root)})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasDotSegment(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		slug, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if _, ok := st.Live(slug); !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", PreviewCSP)
		files.ServeHTTP(w, r)
	})
}

func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// RunGC prunes expired previews every interval until ctx is cancelled, so an
// expired preview's files disappear even when nothing new is published.
func RunGC(ctx context.Context, st *store.Store, every time.Duration, logf func(string, ...any)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := st.GC()
			if err != nil {
				logf("gc: %v", err)
			} else if n > 0 {
				logf("gc: pruned %d expired preview(s)", n)
			}
		}
	}
}

type Options struct {
	Bind  string
	Port  int
	Store *store.Store
	API   http.Handler // serves /_glim/api/*
	Web   http.Handler // serves / and the rest of /_glim/*
}

func NewRouter(o Options) http.Handler {
	previews := PreviewHandler(o.Store)
	zone := func(h http.Handler, w http.ResponseWriter, r *http.Request) {
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/_glim/api/"):
			zone(o.API, w, r)
		case p == "/" || strings.HasPrefix(p, "/_glim/"):
			zone(o.Web, w, r)
		default:
			previews.ServeHTTP(w, r)
		}
	})
}

func Serve(ctx context.Context, o Options) error {
	if err := os.MkdirAll(o.Store.Root, 0o755); err != nil {
		return err
	}
	bind := o.Bind
	if bind == "" {
		bind = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bind, o.Port))
	if err != nil {
		return err
	}
	actual := ln.Addr().(*net.TCPAddr).Port
	if o.Store.BaseURL == "" {
		o.Store.BaseURL = BaseURL(actual)
	}
	_ = WriteState(State{Port: actual, PID: os.Getpid(), Root: o.Store.Root})
	fmt.Printf("glim serving %s on %s:%d\n", o.Store.Root, bind, actual)
	go RunGC(ctx, o.Store, time.Minute, log.Printf)
	srv := &http.Server{Handler: NewRouter(o), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func BaseURL(port int) string { return fmt.Sprintf("http://127.0.0.1:%d", port) }

type State struct {
	Port int    `json:"port"`
	PID  int    `json:"pid"`
	Root string `json:"root"`
}

func StatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".glim", "serve.json")
}

func WriteState(s State) error {
	p := StatePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func ReadState() (State, bool) {
	var s State
	data, err := os.ReadFile(StatePath())
	if err != nil {
		return s, false
	}
	if json.Unmarshal(data, &s) != nil {
		return s, false
	}
	return s, true
}

func FreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

type noListFS struct{ fs http.FileSystem }

func (n noListFS) Open(name string) (http.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if s.IsDir() {
		index := strings.TrimSuffix(name, "/") + "/index.html"
		if idx, ierr := n.fs.Open(index); ierr != nil {
			f.Close()
			return nil, os.ErrNotExist
		} else {
			idx.Close()
		}
		return noReaddirFile{f}, nil
	}
	return f, nil
}

type noReaddirFile struct{ http.File }

func (noReaddirFile) Readdir(int) ([]os.FileInfo, error) { return nil, nil }
