package node

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Andyccr/RainIRC/internal/netutil"
	"github.com/Andyccr/RainIRC/internal/stun"
	"github.com/Andyccr/RainIRC/internal/upnp"
)

func (n *Node) refreshAdvertise() {
	n.addrsMu.Lock()
	list := netutil.SanitizeAddrs(append(netutil.AdvertiseCandidates(n.port), n.upnpExt), 8)
	n.addrs = list
	n.addrsMu.Unlock()
	n.peers.SetAdvertiseAddrs(list)
}

// Candidates is loopback, LAN, and any UPnP TCP mapping.
func (n *Node) Candidates() []string {
	n.addrsMu.Lock()
	extra := append([]string{n.upnpExt}, n.addrs...)
	n.addrsMu.Unlock()
	return netutil.Unique(append(netutil.LocalCandidates(n.port), extra...))
}

// LANHint is a non-loopback TCP address other peers on the same network can dial.
func (n *Node) LANHint() string {
	for _, a := range n.Candidates() {
		host, _, err := net.SplitHostPort(a)
		if err != nil {
			continue
		}
		if host != "127.0.0.1" && host != "::1" {
			return a
		}
	}
	return n.DialAddr()
}

func (n *Node) advertisedTCP() []string {
	n.addrsMu.Lock()
	defer n.addrsMu.Unlock()
	out := make([]string, len(n.addrs))
	copy(out, n.addrs)
	return out
}

func (n *Node) stunMapped() string {
	n.addrsMu.Lock()
	defer n.addrsMu.Unlock()
	return n.stunUDP
}

func (n *Node) stunFinished() bool {
	n.addrsMu.Lock()
	defer n.addrsMu.Unlock()
	return n.stunDone
}

func (n *Node) formatAddrs() string {
	var b strings.Builder
	b.WriteString("Address candidates:")
	fmt.Fprintf(&b, "\n  TCP listen:     %s", n.ListenAddr())
	fmt.Fprintf(&b, "\n  Local:")
	for _, a := range netutil.LocalCandidates(n.port) {
		fmt.Fprintf(&b, "\n    %s", a)
	}
	adv := n.advertisedTCP()
	if len(adv) == 0 {
		b.WriteString("\n  Advertised TCP:  (none yet; LAN IPv4 or --upnp)")
	} else {
		b.WriteString("\n  Advertised TCP (unsigned hello addrs):")
		for _, a := range adv {
			fmt.Fprintf(&b, "\n    %s", a)
		}
	}
	if m := n.stunMapped(); m != "" {
		fmt.Fprintf(&b, "\n  STUN UDP:       %s", m)
		b.WriteString("\n    This is a UDP Binding mapping, not the TCP listen port.")
	} else if n.cfg != nil && (n.cfg.NoSTUN || n.cfg.STUNServer == "") {
		b.WriteString("\n  STUN UDP:       skipped (--no-stun or empty --stun)")
	} else if !n.stunFinished() {
		b.WriteString("\n  STUN UDP:       probing")
	} else {
		b.WriteString("\n  STUN UDP:       (none / query failed)")
	}
	n.addrsMu.Lock()
	upnpExt := n.upnpExt
	n.addrsMu.Unlock()
	if upnpExt != "" {
		fmt.Fprintf(&b, "\n  UPnP TCP:       %s", upnpExt)
	} else if n.cfg != nil && n.cfg.UPnP {
		b.WriteString("\n  UPnP TCP:       failed or still probing")
	} else {
		b.WriteString("\n  UPnP TCP:       off (pass --upnp to try an IGD mapping)")
	}
	b.WriteString("\n  Inbound from the public Internet still needs a forwarded TCP port, UPnP, or a 0.5 relay.")
	return b.String()
}

func (n *Node) probeNAT() {
	n.trySTUN()
	n.tryUPnP()
}

func (n *Node) trySTUN() {
	defer func() {
		n.addrsMu.Lock()
		n.stunDone = true
		n.addrsMu.Unlock()
	}()
	if n.cfg == nil || n.cfg.NoSTUN || n.cfg.STUNServer == "" {
		return
	}
	ctx, cancel := context.WithTimeout(n.ctx, 3*time.Second)
	defer cancel()
	m, err := stun.Binding(ctx, n.cfg.STUNServer)
	if err != nil {
		n.log.Debugf("STUN %s: %v", n.cfg.STUNServer, err)
		return
	}
	mapped := m.String()
	n.addrsMu.Lock()
	n.stunUDP = mapped
	n.addrsMu.Unlock()
	n.System("STUN UDP mapped address %s (not a TCP listen port; inbound Internet still needs UPnP or a forwarded TCP port)", mapped)
}

func (n *Node) tryUPnP() {
	if n.cfg == nil || !n.cfg.UPnP {
		return
	}
	ips := netutil.PrivateIPv4()
	if len(ips) == 0 {
		n.log.Debugf("UPnP: no private IPv4 to map")
		return
	}
	local := ips[0].String()
	ctx, cancel := context.WithTimeout(n.ctx, 5*time.Second)
	defer cancel()
	m, unmap, err := upnp.MapTCP(ctx, local, n.port)
	if err != nil {
		n.log.Debugf("UPnP: %v", err)
		return
	}
	n.addrsMu.Lock()
	n.upnpExt = m.External
	n.upnpUnmap = unmap
	n.addrsMu.Unlock()
	n.refreshAdvertise()
	n.System("UPnP mapped TCP %s -> %s:%d", m.External, local, n.port)
}
