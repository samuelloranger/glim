package thumb

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/samuelloranger/glim/internal/listen"
)

// hostSuffix ends the made-up host name each capture's page is served from.
// .invalid never resolves, so only the proxy can answer for it.
const hostSuffix = ".glim-thumb.invalid"

// proxy is the browser's only way out. Each capture's page is served from a
// random host name that maps to that one preview's folder; anything else is
// forwarded only to public addresses. A page being captured can load scripts
// and fonts from a CDN, but can't reach this machine, the local network or
// another preview, which a visitor's browser could not reach either.
type proxy struct {
	files http.FileSystem
	// public decides which addresses requests may be forwarded to.
	public func(net.IP) bool

	mu    sync.Mutex
	hosts map[string]string // host name -> preview name

	dialer *net.Dialer
	tr     *http.Transport
}

var errBlocked = errors.New("address is not public")

func newProxy(root string) *proxy {
	p := &proxy{files: noList{http.Dir(root)}, public: isPublic, hosts: map[string]string{}}
	p.dialer = &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		// Control sees the resolved address, so a name that resolves to a
		// private one is refused too.
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || !p.public(ip) {
			return errBlocked
		}
		return nil
	}}
	p.tr = &http.Transport{DialContext: p.dialer.DialContext, ResponseHeaderTimeout: timeout}
	return p
}

// add gives the preview a fresh host name for one capture.
func (p *proxy) add(name string) (host string, remove func()) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	host = hex.EncodeToString(b) + hostSuffix
	p.mu.Lock()
	p.hosts[host] = name
	p.mu.Unlock()
	return host, func() {
		p.mu.Lock()
		delete(p.hosts, host)
		p.mu.Unlock()
	}
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.tunnel(w, r)
		return
	}
	// Requests through a proxy carry an absolute URL; anything else was
	// sent straight to the port and is refused.
	if r.URL.Scheme != "http" || r.URL.Host == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	host := r.URL.Hostname()
	if strings.HasSuffix(host, hostSuffix) {
		p.serve(w, r, host)
		return
	}
	p.forward(w, r)
}

// serve answers for a capture's host name with that preview's files only.
func (p *proxy) serve(w http.ResponseWriter, r *http.Request, host string) {
	p.mu.Lock()
	name, ok := p.hosts[host]
	p.mu.Unlock()
	if !ok || (r.URL.Path != "/"+name && !strings.HasPrefix(r.URL.Path, "/"+name+"/")) {
		http.NotFound(w, r)
		return
	}
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
	http.FileServer(p.files).ServeHTTP(w, r)
}

func (p *proxy) forward(w http.ResponseWriter, r *http.Request) {
	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.Header.Del("Proxy-Connection")
	out.Header.Del("Proxy-Authorization")
	resp, err := p.tr.RoundTrip(out)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// tunnel carries HTTPS and WebSocket traffic to a public address.
func (p *proxy) tunnel(w http.ResponseWriter, r *http.Request) {
	up, err := p.dialer.DialContext(r.Context(), "tcp", r.Host)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		up.Close()
		http.Error(w, "tunnel unsupported", http.StatusInternalServerError)
		return
	}
	down, buf, err := hj.Hijack()
	if err != nil {
		up.Close()
		return
	}
	if _, err := down.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
		up.Close()
		down.Close()
		return
	}
	go func() {
		_, _ = io.Copy(up, buf)
		up.Close()
	}()
	_, _ = io.Copy(down, up)
	down.Close()
}

// cgnat is 100.64.0.0/10, shared address space that VPN overlays use.
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func isPublic(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !cgnat.Contains(ip)
}

// noList serves a folder only through its index.html, never as a listing.
type noList struct{ http.FileSystem }

func (fs noList) Open(name string) (http.File, error) {
	f, err := fs.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err == nil && st.IsDir() {
		idx, err := fs.FileSystem.Open(path.Join(name, "index.html"))
		if err != nil {
			f.Close()
			return nil, os.ErrNotExist
		}
		idx.Close()
	}
	return f, nil
}
