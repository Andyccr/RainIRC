package node

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Andyccr/RainIRC/internal/channel"
	"github.com/Andyccr/RainIRC/internal/chat"
	"github.com/Andyccr/RainIRC/internal/config"
	"github.com/Andyccr/RainIRC/internal/directory"
	"github.com/Andyccr/RainIRC/internal/discovery"
	"github.com/Andyccr/RainIRC/internal/identity"
	"github.com/Andyccr/RainIRC/internal/limiter"
	"github.com/Andyccr/RainIRC/internal/logger"
	"github.com/Andyccr/RainIRC/internal/peer"
	"github.com/Andyccr/RainIRC/internal/protocol"
	"github.com/Andyccr/RainIRC/internal/router"
	"github.com/Andyccr/RainIRC/internal/upnp"
	"github.com/Andyccr/RainIRC/internal/version"
)

type EventKind string

const (
	EventChat     EventKind = "chat"
	EventSystem   EventKind = "system"
	EventPeerUp   EventKind = "peer_up"
	EventPeerDown EventKind = "peer_down"
	EventJoin     EventKind = "join"
	EventLeave    EventKind = "leave"
)

type Event struct {
	Kind EventKind
	Text string
	Msg  *protocol.Message
	Peer peer.Info
}

// Node is one P2P-IRC process: listener, peer table, gossip router, channels.
type Node struct {
	cfg    *config.Config
	ident  *identity.Identity
	log    *logger.Logger
	peers  *peer.Manager
	chans  *channel.Manager
	router *router.Router
	disc   *discovery.Service
	dir    *directory.Directory
	limit  *limiter.Window

	ln   net.Listener
	port int
	host string

	addrsMu   sync.Mutex
	addrs     []string
	stunUDP   string
	stunDone  bool
	upnpExt   string
	upnpUnmap func()
	upnpMap   *upnp.Mapping

	handshakeSem chan struct{}
	started      time.Time
	reconnectMu  sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	events []chan Event
	closed bool
}

func Start(parent context.Context, cfg *config.Config, ident *identity.Identity, log *logger.Logger) (*Node, error) {
	if cfg == nil {
		cfg = config.Default()
	}
	if ident == nil {
		return nil, fmt.Errorf("identity is required")
	}
	if log == nil {
		log = logger.New(io.Discard, false)
	}
	if cfg.Nickname != "" {
		if !protocol.ValidNickname(cfg.Nickname) {
			return nil, fmt.Errorf("invalid nickname")
		}
		ident.Nickname = cfg.Nickname
	}
	ctx, cancel := context.WithCancel(parent)
	n := &Node{
		cfg:     cfg,
		ident:   ident,
		log:     log,
		chans:   channel.NewManager(ident.PeerID, cfg.HistoryLimit),
		ctx:     ctx,
		cancel:  cancel,
		host:    cfg.ListenHost,
		started: time.Now(),
	}
	if n.host == "" {
		n.host = "0.0.0.0"
	}
	if cfg.MaxHandshakes > 0 {
		n.handshakeSem = make(chan struct{}, cfg.MaxHandshakes)
	}

	n.peers = peer.NewManager(ctx, ident.PeerID, log, cfg.MaxMessageSize, cfg.PingInterval, cfg.IdleTimeout)
	n.peers.SetMaxPeers(cfg.MaxPeers)
	if !cfg.Plain {
		n.peers.EnableTLS()
	}
	n.router = router.New(n.peers, n.onRouted, cfg.SeenTTL, cfg.SeenMax)
	n.peers.SetHooks(n.onPeerMessage, n.onPeerUp, n.onPeerDown)
	n.peers.SetReplaceHook(n.onPeerReplace)

	dir, err := directory.Load(cfg.DataDir)
	if err != nil {
		log.Warnf("peer directory: %v (starting empty)", err)
		dir = directory.New(cfg.DataDir)
	}
	n.dir = dir
	n.limit = limiter.New(30, time.Second)

	addr := net.JoinHostPort(n.host, fmt.Sprintf("%d", cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	n.ln = ln
	if ta, ok := ln.Addr().(*net.TCPAddr); ok {
		n.port = ta.Port
	} else {
		n.port = cfg.Port
	}

	n.peers.SetListenPort(n.port)
	n.refreshAdvertise()

	n.chans.Join("#general", n.Nick())

	if !cfg.NoDiscover {
		n.disc = discovery.New(ident, n.Nick(), n.port, n.TLS(), log)
		n.disc.SetOnPeer(n.tryAutoConnect)
		if err := n.disc.Start(); err != nil {
			log.Warnf("LAN discovery unavailable: %v", err)
			n.disc = nil
		}
	}

	go n.acceptLoop()
	go n.seenLoop()
	go n.dirSaveLoop()
	go n.probeNAT()
	if cfg.Reconnect {
		go n.reconnectLoop()
	}
	if cfg.AutoConnect {
		go func() {
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-n.ctx.Done():
					return
				case <-t.C:
					for _, a := range n.nearbySnapshot() {
						n.tryAutoConnect(a)
					}
				}
			}
		}()
	}
	mode := "tls"
	if cfg.Plain {
		mode = "plain"
	}
	n.log.Infof("listening on %s peer=%s transport=%s", n.ListenAddr(), ident.ShortID(), mode)
	return n, nil
}

func (n *Node) Ident() *identity.Identity  { return n.ident }
func (n *Node) Nick() string               { return n.ident.DefaultNickname() }
func (n *Node) Port() int                  { return n.port }
func (n *Node) PeerCount() int             { return n.peers.Len() }
func (n *Node) Channels() *channel.Manager { return n.chans }
func (n *Node) Peers() []peer.Info         { return n.peers.List() }
func (n *Node) TLS() bool                  { return n.peers.TLS() }
func (n *Node) DataDir() string            { return n.cfg.DataDir }
func (n *Node) ListenAddr() string {
	return net.JoinHostPort(n.host, fmt.Sprintf("%d", n.port))
}

func (n *Node) DialAddr() string {
	host := n.host
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", n.port))
}

func (n *Node) Subscribe() <-chan Event {
	ch := make(chan Event, 128)
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		close(ch)
		return ch
	}
	n.events = append(n.events, ch)
	n.mu.Unlock()
	return ch
}

func (n *Node) emit(ev Event) {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return
	}
	subs := append([]chan Event(nil), n.events...)
	n.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (n *Node) System(format string, args ...any) {
	n.emit(Event{Kind: EventSystem, Text: fmt.Sprintf(format, args...)})
}

func (n *Node) Close() error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil
	}
	n.closed = true
	subs := n.events
	n.events = nil
	n.addrsMu.Lock()
	unmap := n.upnpUnmap
	n.upnpUnmap = nil
	n.upnpMap = nil
	n.addrsMu.Unlock()
	n.mu.Unlock()
	n.cancel()
	for _, ch := range subs {
		close(ch)
	}
	if unmap != nil {
		unmap()
	}
	if n.ln != nil {
		_ = n.ln.Close()
	}
	n.peers.CloseAll()
	if n.disc != nil {
		n.disc.Close()
	}
	if n.dir != nil {
		if err := n.dir.Save(); err != nil {
			n.log.Warnf("save peer directory: %v", err)
		}
	}
	if err := n.ident.Save(n.cfg.DataDir); err != nil {
		n.log.Warnf("save identity: %v", err)
	}
	return nil
}

func (n *Node) acceptLoop() {
	for {
		conn, err := n.ln.Accept()
		if err != nil {
			if n.ctx.Err() != nil {
				return
			}
			n.log.Warnf("accept: %v", err)
			continue
		}
		go n.adoptInbound(conn)
	}
}

func (n *Node) adoptInbound(conn net.Conn) {
	if n.handshakeSem != nil {
		select {
		case n.handshakeSem <- struct{}{}:
			defer func() { <-n.handshakeSem }()
		default:
			_ = conn.Close()
			n.log.Debugf("reject inbound: handshake slots full")
			return
		}
	}
	n.handleInbound(conn)
}

func (n *Node) handleInbound(conn net.Conn) {
	_, err := n.peers.HandshakeAndAdopt(conn, n.ident, n.Nick(), true, n.cfg.HandshakeWait)
	if err != nil {
		if !errors.Is(err, peer.ErrDuplicate) && !errors.Is(err, peer.ErrSelfConnect) {
			n.log.Debugf("inbound handshake: %v", err)
		}
		return
	}
}

func (n *Node) Disconnect(id string) error {
	if n.dir != nil {
		if rec, err := n.dir.Lookup(id); err == nil {
			id = rec.PeerID
		}
	}
	return n.peers.Disconnect(id)
}

func (n *Node) Join(name string) error {
	ch, err := protocol.NormalizeChannel(name)
	if err != nil {
		return err
	}
	fresh := n.chans.Join(ch, n.Nick())
	n.System("joined %s", ch)
	if fresh {
		msg, err := n.sign(protocol.NewJoin(n.ident.PeerID, n.Nick(), ch))
		if err != nil {
			return err
		}
		n.router.Inject(msg)
	}
	return nil
}

func (n *Node) Leave(name string) error {
	if name == "" {
		name = n.chans.Current()
	}
	ch, err := protocol.NormalizeChannel(name)
	if err != nil {
		return err
	}
	if err := n.chans.Leave(ch); err != nil {
		return err
	}
	msg, err := n.sign(protocol.NewLeave(n.ident.PeerID, n.Nick(), ch))
	if err != nil {
		return err
	}
	n.router.Inject(msg)
	n.System("left %s", ch)
	return nil
}

func (n *Node) SendChat(text string, action bool) error {
	ch := n.chans.Current()
	if ch == "" {
		return fmt.Errorf("join a channel first (/join #general)")
	}
	text = strings.TrimRight(text, "\r\n")
	if text == "" && !action {
		return nil
	}
	msg, err := n.sign(protocol.NewChat(n.ident.PeerID, n.Nick(), ch, text, action))
	if err != nil {
		return err
	}
	n.router.Inject(msg)
	return nil
}

func (n *Node) SendDirect(to, text string) error {
	text = strings.TrimSpace(text)
	if to == "" || text == "" {
		return fmt.Errorf("usage: /msg <peer-id> <text>")
	}
	c, err := n.peers.Find(to)
	if err != nil {
		if n.dir != nil {
			if rec, lerr := n.dir.Lookup(to); lerr == nil {
				c, err = n.peers.Find(rec.PeerID)
			}
		}
		if err != nil {
			return err
		}
	}
	msg, err := n.sign(protocol.NewDirect(n.ident.PeerID, n.Nick(), c.ID(), text))
	if err != nil {
		return err
	}
	n.router.Seen().Add(msg.ID)
	if err := c.Send(msg); err != nil {
		return err
	}
	n.emit(Event{Kind: EventChat, Text: chat.FormatChat(msg, n.displayName(n.ident.PeerID, n.Nick())), Msg: msg})
	return nil
}

func (n *Node) SetNick(nick string) error {
	nick = strings.TrimSpace(nick)
	if !protocol.ValidNickname(nick) {
		return fmt.Errorf("nickname must be 1-%d visible characters", protocol.MaxNickLen)
	}
	n.ident.Nickname = nick
	n.chans.UpdateNick(n.ident.PeerID, nick)
	if n.disc != nil {
		n.disc.SetNickname(nick)
	}
	_ = n.ident.Save(n.cfg.DataDir)
	n.System("nickname is now %s", nick)
	for _, ch := range n.chans.JoinedNames() {
		msg, err := n.sign(protocol.NewJoin(n.ident.PeerID, nick, ch))
		if err != nil {
			continue
		}
		n.router.Inject(msg)
	}
	return nil
}

func (n *Node) Nearby() []discovery.Announcement {
	if n.disc == nil {
		return nil
	}
	n.disc.Announce()
	time.Sleep(150 * time.Millisecond)
	return n.disc.Nearby()
}
func (n *Node) whoami() string {
	id := n.ident
	transport := "plain TCP (insecure)"
	if n.TLS() {
		transport = "TLS 1.3 (mutual Ed25519)"
	}
	created := ""
	if !id.CreatedAt.IsZero() {
		created = "\n  Created:    " + id.CreatedAt.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf(`Local identity:
  Version:    %s
  Peer ID:    %s
  Short ID:   %s
  Fingerprint: %s
  Nickname:   %s
  Public key: %s
  Transport:  %s
  Listen:     %s
  Data dir:   %s%s

%s`,
		version.String(), id.PeerID, id.ShortID(), id.Fingerprint(), n.Nick(), id.PublicKeyHex(),
		transport, n.ListenAddr(), n.DataDir(), created, n.formatAddrs())
}

func (n *Node) formatStats() string {
	max := "unlimited"
	if n.cfg != nil && n.cfg.MaxPeers > 0 {
		max = fmt.Sprintf("%d", n.cfg.MaxPeers)
	}
	uptime := time.Since(n.started).Truncate(time.Second)
	return fmt.Sprintf(`Stats:
  version:    %s
  uptime:     %s
  peers:      %d / %s
  seen-ids:   %d
  channels:   %d
  listen:     %s
  goroutines: %d`,
		version.String(), uptime, n.peers.Len(), max, n.router.Seen().Len(),
		len(n.chans.JoinedNames()), n.ListenAddr(), runtime.NumGoroutine())
}

func shortID(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}
