// Package egress is how HTML Clay connects to the network on a page's behalf,
// such as a carried program's clay.fetch. It reaches the public internet only:
// never this computer, where HTML Clay's own server answers a local caller as
// trusted, and never a private or link-local network.
package egress

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"syscall"
	"time"
)

const dialTimeout = 10 * time.Second

// ErrNotPublic is the refusal for an address on this computer or a private
// network.
var ErrNotPublic = errors.New("not a public internet address")

// self caches the addresses assigned to this computer's interfaces, so a
// dial never reaches a service this computer listens on, even when its
// address is in a public range.
var (
	selfMu    sync.Mutex
	selfAt    time.Time
	selfAddrs map[netip.Addr]bool
)

const selfTTL = 5 * time.Second

func onThisComputer(ip netip.Addr) bool {
	selfMu.Lock()
	defer selfMu.Unlock()
	if selfAddrs == nil || time.Since(selfAt) > selfTTL {
		selfAddrs = interfaceAddresses()
		selfAt = time.Now()
	}
	return selfAddrs[ip.Unmap()]
}

// listInterfaceAddrs is net.InterfaceAddrs; a test gives this computer an
// address in a public range.
var listInterfaceAddrs = net.InterfaceAddrs

func interfaceAddresses() map[netip.Addr]bool {
	addrs, err := listInterfaceAddrs()
	if err != nil {
		return map[netip.Addr]bool{}
	}
	m := make(map[netip.Addr]bool, len(addrs))
	for _, a := range addrs {
		p, err := netip.ParsePrefix(a.String())
		if err != nil {
			continue
		}
		m[p.Addr().Unmap()] = true
	}
	return m
}

var refusedV4 = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

var (
	globalV6  = netip.MustParsePrefix("2000::/3")
	refusedV6 = []netip.Prefix{
		netip.MustParsePrefix("2001::/23"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("2002::/16"),
	}
)

// Public reports whether ip is on the public internet: not loopback, not a
// private, shared (100.64/10) or link-local network, not multicast.
func Public(ip netip.Addr) bool {
	if ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	if ip.Is4() {
		for _, p := range refusedV4 {
			if p.Contains(ip) {
				return false
			}
		}
		return true
	}
	if !globalV6.Contains(ip) {
		return false
	}
	for _, p := range refusedV6 {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// DialPublic dials like net.Dialer and refuses any address that is not
// Public. The check runs on the address being connected, after the name is
// resolved, so a name that resolves to 127.0.0.1 is refused too. An
// http.Transport that uses it must leave Proxy nil: a proxy on this computer
// would be refused, and one elsewhere would reach what DialPublic refuses.
func DialPublic(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: dialTimeout, Control: refuseNonPublic}
	return d.DialContext(ctx, network, addr)
}

func refuseNonPublic(network, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || !Public(ap.Addr()) || onThisComputer(ap.Addr()) {
		return ErrNotPublic
	}
	return nil
}
