package thumb

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type proxyTest struct {
	px   *proxy
	addr string // the proxy's host:port
	c    *http.Client
	host string // the capture host of mine-1234
}

// proxyEnv returns a proxy over a root holding two previews and a client
// that sends every request through it.
func proxyEnv(t *testing.T) proxyTest {
	t.Helper()
	root := t.TempDir()
	for name, files := range map[string]map[string]string{
		"mine-1234":  {"index.html": "<p>mine</p>", "data.js": "var d = 1", ".glim.json": "{}", "assets/x.css": "p{}"},
		"other-5678": {"index.html": "<p>other</p>", "secret.txt": "locked content"},
	} {
		for rel, body := range files {
			p := filepath.Join(root, name, rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	px := newProxy(root)
	srv := httptest.NewServer(px)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	tr := &http.Transport{Proxy: http.ProxyURL(u)}
	t.Cleanup(tr.CloseIdleConnections)
	host, _ := px.add("mine-1234")
	return proxyTest{px: px, addr: u.Host, c: &http.Client{Transport: tr}, host: host}
}

func get(t *testing.T, c *http.Client, u string) (int, string) {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestProxyServesOnlyTheCapturedPreview(t *testing.T) {
	e := proxyEnv(t)
	base := "http://" + e.host
	if code, body := get(t, e.c, base+"/mine-1234/"); code != 200 || !strings.Contains(body, "mine") {
		t.Fatalf("own page = %d %q", code, body)
	}
	if code, _ := get(t, e.c, base+"/mine-1234/data.js"); code != 200 {
		t.Fatalf("own file = %d", code)
	}
	for _, p := range []string{
		"/", "/other-5678/", "/other-5678/secret.txt", "/mine-1234/.glim.json",
		"/mine-1234/assets/", "/mine-1234/../other-5678/secret.txt",
	} {
		if code, body := get(t, e.c, base+p); code != 404 || strings.Contains(body, "locked") {
			t.Errorf("%s = %d %q, want 404", p, code, body)
		}
	}
	if code, _ := get(t, e.c, "http://deadbeef"+hostSuffix+"/other-5678/"); code != 404 {
		t.Errorf("unknown capture host = %d, want 404", code)
	}
	host, remove := e.px.add("other-5678")
	remove()
	if code, _ := get(t, e.c, "http://"+host+"/other-5678/"); code != 404 {
		t.Errorf("removed capture host = %d, want 404", code)
	}
}

func TestProxyRefusesPrivateAddresses(t *testing.T) {
	hits := 0
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer local.Close()
	e := proxyEnv(t)

	if code, _ := get(t, e.c, local.URL+"/admin"); code != http.StatusBadGateway {
		t.Errorf("forward to loopback = %d, want 502", code)
	}

	conn, err := net.Dial("tcp", e.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	target := strings.TrimPrefix(local.URL, "http://")
	_, _ = io.WriteString(conn, "CONNECT "+target+" HTTP/1.1\r\nHost: "+target+"\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("CONNECT to loopback = %d, want 502", resp.StatusCode)
	}
	if hits != 0 {
		t.Errorf("a loopback server was reached %d times", hits)
	}

	// Straight to the port, not as a proxy request.
	resp2, err := http.Get("http://" + e.addr + "/mine-1234/")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Errorf("direct request = %d, want 403", resp2.StatusCode)
	}
}

func TestProxyForwardsToPublicAddresses(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "library")
	}))
	defer cdn.Close()
	e := proxyEnv(t)
	// Stand the loopback test server in for a public CDN.
	e.px.public = func(ip net.IP) bool { return ip.IsLoopback() }
	if code, body := get(t, e.c, cdn.URL+"/lib.js"); code != 200 || body != "library" {
		t.Fatalf("forward = %d %q", code, body)
	}
}

func TestIsPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.0.0.5": false, "192.168.50.1": false, "172.25.0.1": false,
		"100.100.1.1": false, "169.254.169.254": false, "::1": false, "fd00::1": false,
		"0.0.0.0": false, "::ffff:127.0.0.1": false,
	} {
		if got := isPublic(net.ParseIP(addr)); got != want {
			t.Errorf("isPublic(%s) = %v, want %v", addr, got, want)
		}
	}
}
