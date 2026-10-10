package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

var (
	cgnat = netip.MustParsePrefix("100.64.0.0/10")
	// 64:ff9b::/96 embeds an IPv4 address that a NAT64 gateway would reach.
	nat64 = netip.MustParsePrefix("64:ff9b::/96")
	// 192.0.0.0/24 (IETF protocol assignments) and 198.18.0.0/15 (benchmarking).
	special1 = netip.MustParsePrefix("192.0.0.0/24")
	special2 = netip.MustParsePrefix("198.18.0.0/15")
)

// publicAddr reports whether ip is a globally routable unicast address. It
// rejects loopback, private, link-local, multicast, unspecified, CGNAT and
// other special ranges, and judges IPv4-mapped and NAT64 forms by the IPv4
// address inside.
func publicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	if nat64.Contains(ip) {
		b := ip.As16()
		return publicAddr(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
	}
	switch {
	case ip.IsLoopback(), ip.IsPrivate(), ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast(),
		ip.IsInterfaceLocalMulticast(), ip.IsMulticast(), ip.IsUnspecified(),
		cgnat.Contains(ip), special1.Contains(ip), special2.Contains(ip):
		return false
	}
	return ip.IsGlobalUnicast()
}

var errBlockedAddr = errors.New("push endpoint resolves to a non-public address")

// guardControl is a net.Dialer Control hook: it runs after name resolution,
// on the exact address about to be connected, so DNS rebinding and hostnames
// that point inward are refused too.
func guardControl(network, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %v", errBlockedAddr, err)
	}
	if !publicAddr(ap.Addr()) {
		return errBlockedAddr
	}
	return nil
}

// newPushHTTPClient is the client used to reach push services: it connects
// only to public addresses, never follows redirects (the encrypted body must
// not be replayed elsewhere), ignores proxy environment variables, and has
// firm timeouts.
func newPushHTTPClient() *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: guardControl}
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           d.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			MaxIdleConns:          4,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
