package egress

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
)

func TestPublicRefusesThisComputerAndPrivateNetworks(t *testing.T) {
	for _, a := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", "fe80::1", "fc00::1", "100.64.0.1", "::ffff:100.64.0.1", "0.0.0.0", "0.1.2.3", "::ffff:0.1.2.3",
		"224.0.0.1", "255.255.255.255",
		"64:ff9b::127.0.0.1", "64:ff9b::10.0.0.1", "64:ff9b::169.254.169.254", "64:ff9b:1::10.0.0.1",
		"2002:7f00:1::1", "2002:a00:1::1", "fec0::1", "::127.0.0.1", "::ffff:0:127.0.0.1",
		"198.18.0.1", "198.19.249.3", "240.0.0.1", "192.0.0.1", "192.0.2.1",
		"2001:db8::1", "2001::1", "100::1", "::ffff:127.0.0.1%lo0"} {
		if Public(netip.MustParseAddr(a)) {
			t.Errorf("Public(%s) = true", a)
		}
	}
	for _, a := range []string{"1.1.1.1", "160.79.104.10", "2606:4700:4700::1111"} {
		if !Public(netip.MustParseAddr(a)) {
			t.Errorf("Public(%s) = false", a)
		}
	}
}

// The guard checks the address it connects to, so a live server on loopback
// is refused even though a plain dial reaches it, by address and by name.
func TestDialPublicRefusesALiveLoopbackServer(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	addr := l.Addr().String()
	c, err := (&net.Dialer{}).Dial("tcp", addr)
	if err != nil {
		t.Fatalf("the loopback server is not up (the test cannot fail otherwise): %v", err)
	}
	c.Close()
	_, port, _ := net.SplitHostPort(addr)
	for _, target := range []string{addr, net.JoinHostPort("localhost", port)} {
		if c, err := DialPublic(context.Background(), "tcp", target); !errors.Is(err, ErrNotPublic) {
			if c != nil {
				c.Close()
			}
			t.Errorf("DialPublic(%s) = %v, want ErrNotPublic", target, err)
		}
	}
}

// An address this computer holds is refused even when its range alone is
// public, so a page can only reach this computer's services through the
// loopback names DialPublic already refuses.
func TestDialPublicRefusesThisComputersOwnAddresses(t *testing.T) {
	own := netip.MustParseAddr("2606:4700:4700::1111")
	listInterfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.IP(own.AsSlice()), Mask: net.CIDRMask(64, 128)}}, nil
	}
	selfMu.Lock()
	selfAddrs = nil
	selfMu.Unlock()
	t.Cleanup(func() {
		listInterfaceAddrs = net.InterfaceAddrs
		selfMu.Lock()
		selfAddrs = nil
		selfMu.Unlock()
	})
	if !Public(own) {
		t.Fatalf("Public(%s) = false; the test needs an address in a public range", own)
	}
	target := netip.AddrPortFrom(own, 9).String()
	if c, err := DialPublic(context.Background(), "tcp", target); !errors.Is(err, ErrNotPublic) {
		if c != nil {
			c.Close()
		}
		t.Errorf("DialPublic(%s) on this computer's own address = %v, want ErrNotPublic", target, err)
	}
}
