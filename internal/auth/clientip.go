package auth

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// TrustedPeer reports whether ip is loopback or private (a reverse proxy).
func TrustedPeer(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate()
}

// RemoteIP is the host part of the direct peer address.
func RemoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ClientIP is the address throttling keys on. X-Forwarded-For is believed only
// when the direct peer is loopback or private (a reverse proxy); then the
// right-most untrusted hop wins.
func ClientIP(r *http.Request) string {
	peer := RemoteIP(r)
	if !TrustedPeer(peer) {
		return peer
	}
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hops = append(hops, h)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		if !TrustedPeer(hops[i]) {
			return hops[i]
		}
	}
	// Every hop is a trusted proxy address: the client is on a private network
	// or the header was forged. Never take the left-most entry, which the
	// client controls; key on the direct peer instead.
	return peer
}
