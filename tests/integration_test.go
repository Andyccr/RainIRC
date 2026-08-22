package tests

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Andyccr/RainIRC/internal/config"
	"github.com/Andyccr/RainIRC/internal/identity"
	"github.com/Andyccr/RainIRC/internal/logger"
	"github.com/Andyccr/RainIRC/internal/node"
	"github.com/Andyccr/RainIRC/internal/stun"
)

func startNode(t *testing.T, nick string) *node.Node {
	t.Helper()
	cfg := config.Default()
	cfg.Port = 0
	cfg.ListenHost = "127.0.0.1"
	cfg.DataDir = t.TempDir()
	cfg.Nickname = nick
	cfg.NoDiscover = true
	ident, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	ident.Nickname = nick
	n, err := node.Start(context.Background(), cfg, ident, logger.New(io.Discard, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })
	return n
}

func waitPeers(t *testing.T, n *node.Node, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if n.PeerCount() >= count {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d peers, have %d", count, n.PeerCount())
}

func waitChat(t *testing.T, n *node.Node, channel, text string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range n.Channels().History(channel) {
			if m.Text == text {
				return
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	var got []string
	for _, m := range n.Channels().History(channel) {
		got = append(got, m.Text)
	}
	t.Fatalf("timeout waiting for %q in %s, have %v", text, channel, got)
}

func TestTwoPeerConnection(t *testing.T) {
	a := startNode(t, "Alice")
	b := startNode(t, "Bob")
	if err := a.Connect(b.DialAddr()); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, a, 1)
	waitPeers(t, b, 1)
}

func TestChatBetweenTwoPeers(t *testing.T) {
	a := startNode(t, "Alice")
	b := startNode(t, "Bob")
	if err := a.Connect(b.DialAddr()); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, a, 1)
	waitPeers(t, b, 1)
	if err := a.SendChat("hello", false); err != nil {
		t.Fatal(err)
	}
	waitChat(t, b, "#general", "hello")
	if err := b.SendChat("hi back", false); err != nil {
		t.Fatal(err)
	}
	waitChat(t, a, "#general", "hi back")
}

func TestThreePeerMessagePropagation(t *testing.T) {
	a := startNode(t, "A")
	b := startNode(t, "B")
	c := startNode(t, "C")
	if err := a.Connect(b.DialAddr()); err != nil {
		t.Fatal(err)
	}
	if err := c.Connect(b.DialAddr()); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, a, 1)
	waitPeers(t, c, 1)
	waitPeers(t, b, 2)
	if err := a.SendChat("hello from A", false); err != nil {
		t.Fatal(err)
	}
	waitChat(t, b, "#general", "hello from A")
	waitChat(t, c, "#general", "hello from A")
}

func TestThreePeerLoopNoAmplification(t *testing.T) {
	a := startNode(t, "A")
	b := startNode(t, "B")
	c := startNode(t, "C")
	if err := a.Connect(b.DialAddr()); err != nil {
		t.Fatal(err)
	}
	if err := b.Connect(c.DialAddr()); err != nil {
		t.Fatal(err)
	}
	if err := c.Connect(a.DialAddr()); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, a, 2)
	waitPeers(t, b, 2)
	waitPeers(t, c, 2)

	if err := a.SendChat("loop test", false); err != nil {
		t.Fatal(err)
	}
	waitChat(t, b, "#general", "loop test")
	waitChat(t, c, "#general", "loop test")
	time.Sleep(200 * time.Millisecond)

	count := func(n *node.Node) int {
		ncount := 0
		for _, m := range n.Channels().History("#general") {
			if m.Text == "loop test" {
				ncount++
			}
		}
		return ncount
	}
	if count(a) != 1 || count(b) != 1 || count(c) != 1 {
		t.Fatalf("message stored more than once: A=%d B=%d C=%d", count(a), count(b), count(c))
	}
}

func TestDuplicateConnections(t *testing.T) {
	a := startNode(t, "A")
	b := startNode(t, "B")
	if err := a.Connect(b.DialAddr()); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, a, 1)
	waitPeers(t, b, 1)
	_ = b.Connect(a.DialAddr())
	time.Sleep(300 * time.Millisecond)
	if a.PeerCount() != 1 || b.PeerCount() != 1 {
		t.Fatalf("duplicate not collapsed: A=%d B=%d", a.PeerCount(), b.PeerCount())
	}
}

func TestJoinLeaveAndIdentityRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Port = 0
	cfg.ListenHost = "127.0.0.1"
	cfg.DataDir = dir
	cfg.Nickname = "Alice"
	cfg.NoDiscover = true
	ident, err := identity.LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	n, err := node.Start(context.Background(), cfg, ident, logger.New(io.Discard, false))
	if err != nil {
		t.Fatal(err)
	}
	id := n.Ident().PeerID
	if err := n.Join("#dev"); err != nil {
		t.Fatal(err)
	}
	if !n.Channels().Joined("#dev") {
		t.Fatal("not joined")
	}
	if err := n.Leave("#dev"); err != nil {
		t.Fatal(err)
	}
	_ = n.Close()

	ident2, err := identity.LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ident2.PeerID != id {
		t.Fatalf("identity changed across restart: %s -> %s", id, ident2.PeerID)
	}
}

func TestHandleLineUnknownCommand(t *testing.T) {
	n := startNode(t, "A")
	_, err := n.HandleLine("/nope")
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("got %v", err)
	}
}

func TestTLSEnabledByDefault(t *testing.T) {
	n := startNode(t, "A")
	if !n.TLS() {
		t.Fatal("TLS should be enabled by default")
	}
}

func TestWhoami(t *testing.T) {
	n := startNode(t, "Alice")
	ev := n.Subscribe()
	if _, err := n.HandleLine("/whoami"); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ev:
		if !strings.Contains(e.Text, n.Ident().PeerID) {
			t.Fatalf("whoami missing peer id: %s", e.Text)
		}
		if !strings.Contains(e.Text, "TLS 1.3") {
			t.Fatalf("whoami missing TLS: %s", e.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for /whoami")
	}
}

func TestPlainDisablesTLS(t *testing.T) {
	cfg := config.Default()
	cfg.Port = 0
	cfg.ListenHost = "127.0.0.1"
	cfg.DataDir = t.TempDir()
	cfg.Nickname = "P"
	cfg.NoDiscover = true
	cfg.Plain = true
	ident, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	n, err := node.Start(context.Background(), cfg, ident, logger.New(io.Discard, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })
	if n.TLS() {
		t.Fatal("--plain should disable TLS")
	}
}

func TestConnectByAlias(t *testing.T) {
	a := startNode(t, "Alice")
	b := startNode(t, "Bob")
	if err := a.Connect(b.DialAddr()); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, a, 1)
	if _, err := a.HandleLine("/alias " + b.Ident().ShortID() + " laptop"); err != nil {
		t.Fatal(err)
	}
	if err := a.Disconnect(b.Ident().ShortID()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for a.PeerCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(15 * time.Millisecond)
	}
	if a.PeerCount() != 0 {
		t.Fatal("still connected after disconnect")
	}
	if err := a.Connect("laptop"); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, a, 1)
}

func TestReconnectKnown(t *testing.T) {
	b := startNode(t, "Bob")
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Port = 0
	cfg.ListenHost = "127.0.0.1"
	cfg.DataDir = dir
	cfg.Nickname = "Alice"
	cfg.NoDiscover = true
	ident, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	ident.Nickname = "Alice"
	a, err := node.Start(context.Background(), cfg, ident, logger.New(io.Discard, false))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Connect(b.DialAddr()); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, a, 1)
	waitPeers(t, b, 1)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}

	ident2, err := identity.LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Reconnect = true
	a2, err := node.Start(context.Background(), cfg, ident2, logger.New(io.Discard, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a2.Close() })
	waitPeers(t, a2, 1)
	waitPeers(t, b, 1)
}

func TestAddrCommandListsLoopback(t *testing.T) {
	n := startNode(t, "Alice")
	ev := n.Subscribe()
	if _, err := n.HandleLine("/addr"); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ev:
		if !strings.Contains(e.Text, "127.0.0.1") {
			t.Fatalf("/addr missing loopback: %s", e.Text)
		}
		if !strings.Contains(e.Text, "STUN") {
			t.Fatalf("/addr missing STUN section: %s", e.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for /addr")
	}
}

func TestVersionCommand(t *testing.T) {
	n := startNode(t, "Alice")
	ev := n.Subscribe()
	if _, err := n.HandleLine("/version"); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ev:
		if !strings.Contains(e.Text, "0.4.0") {
			t.Fatalf("/version: %s", e.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for /version")
	}
}

func TestSTUNMappedListed(t *testing.T) {
	addr, stop, err := stun.ServeBinding("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)

	cfg := config.Default()
	cfg.Port = 0
	cfg.ListenHost = "127.0.0.1"
	cfg.DataDir = t.TempDir()
	cfg.Nickname = "S"
	cfg.NoDiscover = true
	cfg.STUNServer = addr.String()
	ident, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	ident.Nickname = "S"
	n, err := node.Start(context.Background(), cfg, ident, logger.New(io.Discard, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ev := n.Subscribe()
		if _, err := n.HandleLine("/addr"); err != nil {
			t.Fatal(err)
		}
		select {
		case e := <-ev:
			if strings.Contains(e.Text, "This is a UDP Binding mapping") && strings.Contains(e.Text, "127.0.0.1") {
				return
			}
		case <-time.After(80 * time.Millisecond):
		}
	}
	t.Fatal("STUN mapping never appeared in /addr")
}
