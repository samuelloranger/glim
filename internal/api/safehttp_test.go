package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestPublicAddr(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"100.64.0.1", "100.127.255.255", "0.0.0.0", "224.0.0.1", "::1", "::", "fe80::1", "fc00::1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:100.64.0.1", "64:ff9b::7f00:1", "198.18.0.1", "fe80::1%eth0"} {
		ip, err := netip.ParseAddr(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if publicAddr(ip) {
			t.Errorf("%s accepted", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "17.0.0.1", "2606:4700::1111", "::ffff:8.8.8.8"} {
		if !publicAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s refused", s)
		}
	}
}

func TestValidEndpointRejectsInternalLiterals(t *testing.T) {
	for _, u := range []string{"https://100.64.0.1/x", "https://[fe80::1%25eth0]/x", "https://[::ffff:127.0.0.1]/x",
		"https://0.0.0.0/x", "https://169.254.169.254/x", "https://[::1]/x", "https://user@push.example/x"} {
		if validEndpoint(u) {
			t.Errorf("%s accepted", u)
		}
	}
	if !validEndpoint("https://fcm.googleapis.com/fcm/send/abc") || !validEndpoint("https://8.8.8.8/x") {
		t.Fatal("public endpoint refused")
	}
}

// The guard must stop a connection to a loopback server even though the URL
// is syntactically fine: this is what defeats hostnames that resolve or
// rebind inward.
func TestPushClientRefusesNonPublicDestinations(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	_, err := newPushHTTPClient().Post(srv.URL, "text/plain", nil)
	if err == nil || !errors.Is(err, errBlockedAddr) {
		t.Fatalf("err = %v, want errBlockedAddr", err)
	}
	if hit {
		t.Fatal("the request reached the loopback server")
	}
}

func TestPushClientDoesNotFollowRedirects(t *testing.T) {
	c := newPushHTTPClient()
	req, _ := http.NewRequest("POST", "https://push.example/x", nil)
	if err := c.CheckRedirect(req, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect = %v", err)
	}
	if c.Timeout == 0 {
		t.Fatal("no overall timeout")
	}
}
