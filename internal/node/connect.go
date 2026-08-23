package node

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Andyccr/RainIRC/internal/config"
	"github.com/Andyccr/RainIRC/internal/discovery"
	"github.com/Andyccr/RainIRC/internal/peer"
)

var errAlreadyDialing = errors.New("already connecting")

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
	n.dir.Observe(info.ID, pub, info.Nickname, reconnectAddr(info))
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

func (n *Node) Connect(address string) error {
	addrs, err := n.resolveConnectTargets(address)
	if err != nil {
		return err
	}
	if len(addrs) == 1 {
		return n.dialOne(n.ctx, addrs[0])
	}
	return n.dialFirst(addrs)
}

func (n *Node) dialFirst(addrs []string) error {
	ctx, cancel := context.WithCancel(n.ctx)
	defer cancel()
	type result struct{ err error }
	ch := make(chan result, len(addrs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, a := range addrs {
		a := a
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				ch <- result{ctx.Err()}
				return
			}
			err := n.dialOne(ctx, a)
			if err == nil {
				cancel()
			}
			ch <- result{err}
		}()
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	var last error
	ok := false
	for r := range ch {
		if r.err == nil {
			ok = true
			continue
		}
		if errors.Is(r.err, context.Canceled) {
			continue
		}
		if errors.Is(r.err, errAlreadyDialing) {
			if last == nil {
				last = r.err
			}
			continue
		}
		last = r.err
		n.log.Debugf("dial: %v", r.err)
	}
	if ok {
		return nil
	}
	if last == nil {
		return fmt.Errorf("no addresses to try")
	}
	return last
}

func (n *Node) beginDial(address string) bool {
	n.dialMu.Lock()
	defer n.dialMu.Unlock()
	if n.dialing == nil {
		n.dialing = make(map[string]struct{})
	}
	if _, ok := n.dialing[address]; ok {
		return false
	}
	n.dialing[address] = struct{}{}
	return true
}

func (n *Node) endDial(address string) {
	n.dialMu.Lock()
	delete(n.dialing, address)
	n.dialMu.Unlock()
}

func (n *Node) acquireDialSlot(ctx context.Context) error {
	if n.dialSem == nil {
		return nil
	}
	select {
	case n.dialSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *Node) dialOne(ctx context.Context, address string) error {
	if ctx == nil {
		ctx = n.ctx
	}
	if !n.beginDial(address) {
		return fmt.Errorf("%w to %s", errAlreadyDialing, address)
	}
	defer n.endDial(address)
	if err := n.acquireDialSlot(ctx); err != nil {
		return err
	}
	if n.dialSem != nil {
		defer func() { <-n.dialSem }()
	}
	d := net.Dialer{Timeout: n.cfg.DialTimeout}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("dial %s: %w", address, err)
	}
	if ctx.Err() != nil {
		_ = conn.Close()
		return ctx.Err()
	}
	c, err := n.peers.HandshakeAndAdopt(conn, n.ident, n.Nick(), false, n.cfg.HandshakeWait)
	if err != nil {
		if errors.Is(err, peer.ErrDuplicate) {
			return nil
		}
		return err
	}
	if c != nil {
		info := c.Info()
		info.Inbound = false
		info.Addr = address
		n.observePeer(info)
	}
	return nil
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
	if n.backoff != nil && !n.backoff.Allow(a.PeerID) {
		return
	}
	go n.autoDial(a.PeerID, a.Addr)
}

func (n *Node) autoDial(peerID, addr string) {
	if n.peers.Connected(peerID) {
		return
	}
	if err := n.Connect(addr); err != nil {
		if errors.Is(err, errAlreadyDialing) {
			return
		}
		if n.backoff != nil {
			n.backoff.Fail(peerID)
		}
		n.log.Debugf("auto-connect %s: %v", shortID(peerID), err)
		return
	}
	if n.backoff != nil {
		n.backoff.Reset(peerID)
	}
}

func (n *Node) reconnectLoop() {
	if n.dir == nil {
		return
	}
	time.Sleep(250 * time.Millisecond)
	n.reconnectOnce()
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-t.C:
			n.reconnectOnce()
		}
	}
}

func (n *Node) reconnectOnce() {
	if n.dir == nil {
		return
	}
	if !n.reconnectMu.TryLock() {
		return
	}
	defer n.reconnectMu.Unlock()
	for _, rec := range n.dir.ReconnectTargets() {
		if rec.PeerID == n.ident.PeerID || n.peers.Connected(rec.PeerID) {
			continue
		}
		if n.backoff != nil && !n.backoff.Allow(rec.PeerID) {
			continue
		}
		if err := n.Connect(rec.PeerID); err != nil {
			if errors.Is(err, errAlreadyDialing) {
				continue
			}
			if n.backoff != nil {
				n.backoff.Fail(rec.PeerID)
			}
			n.log.Debugf("reconnect %s: %v", shortID(rec.PeerID), err)
			continue
		}
		if n.backoff != nil {
			n.backoff.Reset(rec.PeerID)
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
