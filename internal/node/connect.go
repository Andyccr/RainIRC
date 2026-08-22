package node

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Andyccr/RainIRC/internal/config"
	"github.com/Andyccr/RainIRC/internal/discovery"
	"github.com/Andyccr/RainIRC/internal/peer"
)

func reconnectAddr(info peer.Info) string {
	if !info.Inbound {
		return info.Addr
	}
	host, _, err := net.SplitHostPort(info.Addr)
	if err != nil || info.ListenPort <= 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(info.ListenPort))
}

func (n *Node) observePeer(info peer.Info) {
	if n.dir == nil {
		return
	}
	pub := ""
	if len(info.PublicKey) > 0 {
		pub = hex.EncodeToString(info.PublicKey)
	}
	n.dir.Observe(info.ID, pub, info.Nickname, reconnectAddr(info), info.Addrs...)
	_ = n.dir.Save()
}

func (n *Node) displayName(peerID, nick string) string {
	if n.dir != nil {
		return n.dir.DisplayName(peerID, nick)
	}
	if nick != "" {
		return nick
	}
	return shortID(peerID)
}

func (n *Node) resolveConnectTargets(target string) ([]string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("usage: /connect <host:port|alias>")
	}
	if _, _, err := net.SplitHostPort(target); err == nil {
		return []string{target}, nil
	}
	if ip := net.ParseIP(target); ip != nil {
		return []string{net.JoinHostPort(target, strconv.Itoa(config.DefaultPort))}, nil
	}
	if n.dir != nil {
		if addrs, err := n.dir.AddrsFor(target); err == nil && len(addrs) > 0 {
			return addrs, nil
		}
	}
	for _, a := range n.nearbySnapshot() {
		if strings.EqualFold(a.Nickname, target) || a.PeerID == target ||
			(len(target) >= 4 && strings.HasPrefix(a.PeerID, target)) {
			return []string{a.Addr}, nil
		}
		if n.dir != nil && strings.EqualFold(n.dir.Alias(a.PeerID), target) {
			return []string{a.Addr}, nil
		}
	}
	return nil, fmt.Errorf("cannot resolve %q (try host:port, alias, or /discover)", target)
}

func (n *Node) nearbySnapshot() []discovery.Announcement {
	if n.disc == nil {
		return nil
	}
	return n.disc.Nearby()
}

func (n *Node) tryAutoConnect(a discovery.Announcement) {
	if n.cfg == nil || !n.cfg.AutoConnect || !a.Verified || a.Addr == "" {
		return
	}
	if n.peers.Connected(a.PeerID) {
		return
	}
	if err := n.Connect(a.Addr); err != nil {
		n.log.Debugf("auto-connect %s: %v", shortID(a.PeerID), err)
		return
	}
}

func (n *Node) reconnectKnown() {
	if n.dir == nil {
		return
	}
	time.Sleep(250 * time.Millisecond)
	for _, rec := range n.dir.ReconnectTargets() {
		if rec.PeerID == n.ident.PeerID || n.peers.Connected(rec.PeerID) {
			continue
		}
		if err := n.Connect(rec.PeerID); err != nil {
			n.log.Debugf("reconnect %s: %v", shortID(rec.PeerID), err)
		}
	}
}

func (n *Node) connectNearbyVerified() (int, error) {
	list := n.Nearby()
	count := 0
	var last error
	for _, a := range list {
		if !a.Verified || n.peers.Connected(a.PeerID) {
			continue
		}
		if err := n.Connect(a.Addr); err != nil {
			last = err
			n.log.Debugf("discover connect %s: %v", shortID(a.PeerID), err)
			continue
		}
		count++
	}
	if count == 0 && last != nil {
		return 0, last
	}
	return count, nil
}

func (n *Node) formatKnown() string {
	if n.dir == nil {
		return "no peer directory"
	}
	list := n.dir.List()
	if len(list) == 0 {
		return "no known peers yet"
	}
	var b strings.Builder
	b.WriteString("Known peers:")
	for _, rec := range list {
		alias := rec.Alias
		if alias == "" {
			alias = "-"
		}
		addr := rec.LastAddr
		if addr == "" {
			addr = "-"
		}
		extra := ""
		if len(rec.ExtraAddrs) > 0 {
			extra = "  +" + strings.Join(rec.ExtraAddrs, ",")
		}
		fmt.Fprintf(&b, "\n  %s  alias=%s  nick=%s  %s%s", shortID(rec.PeerID), alias, rec.Nickname, addr, extra)
	}
	return b.String()
}

func (n *Node) formatAliases() string {
	if n.dir == nil {
		return "no peer directory"
	}
	var b strings.Builder
	b.WriteString("Aliases:")
	found := false
	for _, rec := range n.dir.List() {
		if rec.Alias == "" {
			continue
		}
		found = true
		fmt.Fprintf(&b, "\n  %s  %s  %s", rec.Alias, shortID(rec.PeerID), rec.LastAddr)
	}
	if !found {
		return "no aliases (set one with /alias <peer-id> <name>)"
	}
	return b.String()
}

func (n *Node) setAlias(peerKey, name string) error {
	if n.dir == nil {
		return fmt.Errorf("peer directory unavailable")
	}
	id := peerKey
	if rec, err := n.dir.Lookup(peerKey); err == nil {
		id = rec.PeerID
	} else if c, err := n.peers.Find(peerKey); err == nil {
		id = c.ID()
		n.observePeer(c.Info())
	} else {
		return fmt.Errorf("unknown peer %q (connect first or use a known id)", peerKey)
	}
	if err := n.dir.SetAlias(id, name); err != nil {
		return err
	}
	_ = n.dir.Save()
	n.System("alias %s -> %s", name, shortID(id))
	return nil
}

func (n *Node) clearAlias(key string) error {
	if n.dir == nil {
		return fmt.Errorf("peer directory unavailable")
	}
	if err := n.dir.ClearAlias(key); err != nil {
		return err
	}
	_ = n.dir.Save()
	n.System("removed alias for %s", key)
	return nil
}
