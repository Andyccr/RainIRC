package peer

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Andyccr/RainIRC/internal/identity"
	"github.com/Andyccr/RainIRC/internal/logger"
	"github.com/Andyccr/RainIRC/internal/netutil"
	"github.com/Andyccr/RainIRC/internal/protocol"
	"github.com/Andyccr/RainIRC/internal/transport"
)

var (
	ErrHandshake   = errors.New("handshake failed")
	ErrSelfConnect = errors.New("refusing connection to self")
	ErrDuplicate   = errors.New("duplicate connection dropped")
	ErrUnknownPeer = errors.New("unknown peer")
	ErrAmbiguousID = errors.New("peer id prefix matches more than one peer")
	ErrSendQueue   = errors.New("send queue full")
	ErrTooMany     = errors.New("too many peers")
)

// Info is metadata about a remote peer after a successful handshake.
type Info struct {
	ID         string
	PublicKey  ed25519.PublicKey
	Nickname   string
	Addr       string
	Inbound    bool
	Connected  time.Time
	ListenPort int
	Addrs      []string
}

func (i Info) ShortID() string {
	if len(i.ID) < 8 {
		return i.ID
	}
	return i.ID[:8]
}

// Conn is one TCP session with a remote peer.
// A single bufio.Reader is used from handshake through the read loop so
// buffered bytes are never discarded. Writes are serialized on sendCh.
type Conn struct {
	nc        net.Conn
	reader    *bufio.Reader
	info      Info
	inbound   bool
	sendCh    chan *protocol.Message
	ctx       context.Context
	cancel    context.CancelFunc
	lastRecv  atomic.Int64
	maxMsg    int
	pingEvery time.Duration
	idleAfter time.Duration
	onMsg     func(*Conn, *protocol.Message)
	onClose   func(*Conn)
	log       *logger.Logger

	closeOnce  sync.Once
	notifyOnce sync.Once
	malformed  int
	lastPong   atomic.Int64
}

func (c *Conn) AllowPong(minInterval time.Duration) bool {
	if minInterval <= 0 {
		minInterval = time.Second
	}
	now := time.Now().UnixNano()
	last := c.lastPong.Load()
	if last != 0 && now-last < int64(minInterval) {
		return false
	}
	return c.lastPong.CompareAndSwap(last, now)
}

func (c *Conn) Info() Info               { return c.info }
func (c *Conn) ID() string               { return c.info.ID }
func (c *Conn) Inbound() bool            { return c.inbound }
func (c *Conn) Context() context.Context { return c.ctx }

func (c *Conn) touch() {
	c.lastRecv.Store(time.Now().UnixNano())
}

func (c *Conn) Send(m *protocol.Message) error {
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	case c.sendCh <- m:
		return nil
	default:
		return ErrSendQueue
	}
}

func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.nc.Close()
	})
}

func (c *Conn) start() {
	c.touch()
	go c.readLoop()
	go c.writeLoop()
}

func (c *Conn) readLoop() {
	defer c.shutdown()
	for {
		if c.idleAfter > 0 {
			_ = c.nc.SetReadDeadline(time.Now().Add(c.idleAfter))
		}
		msg, err := protocol.Read(c.reader, c.maxMsg)
		if err != nil {
			if errors.Is(err, protocol.ErrMalformed) {
				c.malformed++
				c.log.Warnf("malformed message from %s: %v", c.info.ShortID(), err)
				if c.malformed >= 5 {
					c.log.Warnf("disconnecting %s after repeated malformed frames", c.info.ShortID())
					return
				}
				continue
			}
			c.log.Debugf("read from %s: %v", c.info.ShortID(), err)
			return
		}
		c.malformed = 0
		c.touch()
		if c.onMsg != nil {
			c.onMsg(c, msg)
		}
	}
}

func (c *Conn) writeLoop() {
	ticker := time.NewTicker(c.pingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			last := time.Unix(0, c.lastRecv.Load())
			if c.idleAfter > 0 && time.Since(last) > c.idleAfter {
				c.log.Warnf("idle timeout for %s", c.info.ShortID())
				c.Close()
				return
			}
			if time.Since(last) < c.pingEvery {
				continue
			}
			_ = c.writeMsg(protocol.NewPing(c.info.ID))
		case msg := <-c.sendCh:
			if err := c.writeMsg(msg); err != nil {
				c.log.Debugf("write to %s: %v", c.info.ShortID(), err)
				c.Close()
				return
			}
		}
	}
}

func (c *Conn) writeMsg(m *protocol.Message) error {
	_ = c.nc.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return protocol.Write(c.nc, m)
}

func (c *Conn) shutdown() {
	c.Close()
	c.notifyOnce.Do(func() {
		if c.onClose != nil {
			c.onClose(c)
		}
	})
}

// Manager is a thread-safe table of live peer connections.
type Manager struct {
	mu        sync.RWMutex
	conns     map[string]*Conn
	localID   string
	log       *logger.Logger
	maxMsg    int
	pingEvery time.Duration
	idleAfter time.Duration
	parent    context.Context

	onMsg      func(*Conn, *protocol.Message)
	onUp       func(Info)
	onDown     func(Info)
	onReplace  func(Info)
	tls        bool
	listenPort int
	advertise  []string
	maxPeers   int
}

func NewManager(parent context.Context, localID string, log *logger.Logger, maxMsg int, pingEvery, idleAfter time.Duration) *Manager {
	if log == nil {
		log = logger.New(nil, false)
	}
	if pingEvery <= 0 {
		pingEvery = 20 * time.Second
	}
	if idleAfter <= 0 {
		idleAfter = 60 * time.Second
	}
	return &Manager{
		conns:     make(map[string]*Conn),
		localID:   localID,
		log:       log,
		maxMsg:    maxMsg,
		pingEvery: pingEvery,
		idleAfter: idleAfter,
		parent:    parent,
	}
}

func (m *Manager) SetHooks(onMsg func(*Conn, *protocol.Message), onUp, onDown func(Info)) {
	m.onMsg = onMsg
	m.onUp = onUp
	m.onDown = onDown
}

func (m *Manager) SetReplaceHook(fn func(Info)) { m.onReplace = fn }

func (m *Manager) SetMaxPeers(n int) { m.maxPeers = n }

// EnableTLS wraps accepted/dialed sockets with mutually authenticated TLS 1.3
// before the NDJSON handshake.
func (m *Manager) EnableTLS() { m.tls = true }

func (m *Manager) TLS() bool { return m.tls }

func (m *Manager) SetListenPort(p int) { m.listenPort = p }

// SetAdvertiseAddrs stores unsigned TCP address hints for hello/welcome.
func (m *Manager) SetAdvertiseAddrs(addrs []string) {
	m.mu.Lock()
	m.advertise = netutil.SanitizeAddrs(addrs, 8)
	m.mu.Unlock()
}

func (m *Manager) advertiseSnapshot() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.advertise) == 0 {
		return nil
	}
	out := make([]string, len(m.advertise))
	copy(out, m.advertise)
	return out
}

func (m *Manager) Connected(peerID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.conns[peerID]
	return ok
}

func (m *Manager) LocalID() string { return m.localID }

func (m *Manager) wrap(nc net.Conn, inbound bool) *Conn {
	ctx, cancel := context.WithCancel(m.parent)
	if tc, ok := nc.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
	}
	c := &Conn{
		nc:        nc,
		reader:    bufio.NewReaderSize(nc, 4096),
		inbound:   inbound,
		sendCh:    make(chan *protocol.Message, 64),
		ctx:       ctx,
		cancel:    cancel,
		maxMsg:    m.maxMsg,
		pingEvery: m.pingEvery,
		idleAfter: m.idleAfter,
		log:       m.log,
		onMsg:     m.onMsg,
		onClose:   m.unregister,
	}
	return c
}

// HandshakeAndAdopt performs the version-2 handshake then registers the peer.
// Duplicate-connection rule: keep the TCP session initiated by the
// lexicographically smaller Peer ID. See docs/architecture.md.
func (m *Manager) HandshakeAndAdopt(nc net.Conn, ident *identity.Identity, nick string, inbound bool, wait time.Duration) (*Conn, error) {
	if tc, ok := nc.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
	}
	if m.tls {
		secured, err := transport.Handshake(nc, ident, inbound, wait)
		if err != nil {
			return nil, fmt.Errorf("%w: tls: %v", ErrHandshake, err)
		}
		nc = secured
	}
	c := m.wrap(nc, inbound)
	info, err := m.handshake(c, ident, nick, inbound, wait)
	if err != nil {
		c.Close()
		return nil, err
	}
	if info.ID == m.localID {
		c.Close()
		return nil, ErrSelfConnect
	}
	if pub, err := transport.PeerPublicKey(nc); err != nil {
		c.Close()
		return nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	} else if pub != nil && !pub.Equal(info.PublicKey) {
		c.Close()
		return nil, fmt.Errorf("%w: TLS certificate does not match hello public key", ErrHandshake)
	}
	info.Inbound = inbound
	info.Connected = time.Now()
	info.Addr = nc.RemoteAddr().String()
	c.info = info
	return m.adopt(c)
}

func (m *Manager) handshake(c *Conn, ident *identity.Identity, nick string, inbound bool, wait time.Duration) (Info, error) {
	if wait <= 0 {
		wait = 5 * time.Second
	}
	_ = c.nc.SetDeadline(time.Now().Add(wait))
	defer c.nc.SetDeadline(time.Time{})

	if inbound {
		msg, err := protocol.Read(c.reader, c.maxMsg)
		if err != nil {
			return Info{}, fmt.Errorf("%w: read hello: %v", ErrHandshake, err)
		}
		if msg.Type != protocol.TypeHello {
			return Info{}, fmt.Errorf("%w: expected hello, got %s", ErrHandshake, msg.Type)
		}
		info, err := identityFromHandshake(msg)
		if err != nil {
			return Info{}, err
		}
		welcome := protocol.NewWelcome(ident.PeerID, ident.PublicKeyHex(), nick)
		welcome.Port = m.listenPort
		welcome.Addrs = m.advertiseSnapshot()
		if err := welcome.Sign(ident.PrivateKey); err != nil {
			return Info{}, fmt.Errorf("%w: sign welcome: %v", ErrHandshake, err)
		}
		if err := protocol.Write(c.nc, welcome); err != nil {
			return Info{}, fmt.Errorf("%w: write welcome: %v", ErrHandshake, err)
		}
		return info, nil
	}

	hello := protocol.NewHello(ident.PeerID, ident.PublicKeyHex(), nick)
	hello.Port = m.listenPort
	hello.Addrs = m.advertiseSnapshot()
	if err := hello.Sign(ident.PrivateKey); err != nil {
		return Info{}, fmt.Errorf("%w: sign hello: %v", ErrHandshake, err)
	}
	if err := protocol.Write(c.nc, hello); err != nil {
		return Info{}, fmt.Errorf("%w: write hello: %v", ErrHandshake, err)
	}
	msg, err := protocol.Read(c.reader, c.maxMsg)
	if err != nil {
		return Info{}, fmt.Errorf("%w: read welcome: %v", ErrHandshake, err)
	}
	if msg.Type != protocol.TypeWelcome {
		return Info{}, fmt.Errorf("%w: expected welcome, got %s", ErrHandshake, msg.Type)
	}
	return identityFromHandshake(msg)
}

func (m *Manager) adopt(c *Conn) (*Conn, error) {
	m.mu.Lock()
	existing, ok := m.conns[c.info.ID]
	var drop *Conn
	kept := true
	replaced := false
	if !ok {
		if m.maxPeers > 0 && len(m.conns) >= m.maxPeers {
			m.mu.Unlock()
			c.Close()
			return nil, ErrTooMany
		}
		m.conns[c.info.ID] = c
	} else {
		wantOutbound := m.localID < c.info.ID
		newOutbound := !c.inbound
		oldOutbound := !existing.inbound
		if newOutbound == oldOutbound {
			drop = c
			kept = false
		} else if newOutbound == wantOutbound {
			drop = existing
			m.conns[c.info.ID] = c
			replaced = true
		} else {
			drop = c
			kept = false
		}
	}
	m.mu.Unlock()

	if drop != nil {
		m.log.Infof("dropping duplicate connection to %s", c.info.ShortID())
		drop.Close()
		if !kept {
			return nil, ErrDuplicate
		}
	}
	c.start()
	if replaced {
		if m.onReplace != nil {
			m.onReplace(c.info)
		}
	} else if m.onUp != nil && drop == nil {
		m.onUp(c.info)
	}
	return c, nil
}

func (m *Manager) unregister(c *Conn) {
	m.mu.Lock()
	cur, ok := m.conns[c.info.ID]
	if ok && cur == c {
		delete(m.conns, c.info.ID)
	}
	m.mu.Unlock()
	if ok && cur == c && m.onDown != nil {
		m.onDown(c.info)
	}
}

func (m *Manager) Broadcast(msg *protocol.Message, exceptPeerID string) {
	m.mu.RLock()
	conns := make([]*Conn, 0, len(m.conns))
	for id, c := range m.conns {
		if id == exceptPeerID {
			continue
		}
		conns = append(conns, c)
	}
	m.mu.RUnlock()
	for _, c := range conns {
		if err := c.Send(msg); err != nil {
			m.log.Debugf("broadcast to %s: %v", c.info.ShortID(), err)
			if errors.Is(err, ErrSendQueue) {
				c.Close()
			}
		}
	}
}

func (m *Manager) SendTo(peerID string, msg *protocol.Message) error {
	c, err := m.Find(peerID)
	if err != nil {
		return err
	}
	return c.Send(msg)
}

func (m *Manager) Find(idOrPrefix string) (*Conn, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if c, ok := m.conns[idOrPrefix]; ok {
		return c, nil
	}
	var found *Conn
	for id, c := range m.conns {
		if id == idOrPrefix || (len(idOrPrefix) >= 4 && len(id) >= len(idOrPrefix) && id[:len(idOrPrefix)] == idOrPrefix) {
			if found != nil {
				return nil, ErrAmbiguousID
			}
			found = c
		}
	}
	if found == nil {
		return nil, ErrUnknownPeer
	}
	return found, nil
}

func (m *Manager) Disconnect(idOrPrefix string) error {
	c, err := m.Find(idOrPrefix)
	if err != nil {
		return err
	}
	c.Close()
	return nil
}

func (m *Manager) List() []Info {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Info, 0, len(m.conns))
	for _, c := range m.conns {
		out = append(out, c.info)
	}
	return out
}

func (m *Manager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.conns)
}

func (m *Manager) CloseAll() {
	m.mu.RLock()
	conns := make([]*Conn, 0, len(m.conns))
	for _, c := range m.conns {
		conns = append(conns, c)
	}
	m.mu.RUnlock()
	for _, c := range conns {
		c.Close()
	}
}

func (m *Manager) UpdateNick(peerID, nick string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conns[peerID]; ok {
		c.info.Nickname = nick
	}
}

func identityFromHandshake(msg *protocol.Message) (Info, error) {
	if err := msg.Validate(); err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	if err := msg.VerifySignature(); err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	if err := msg.Fresh(time.Now()); err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	raw, err := hex.DecodeString(msg.PublicKey)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return Info{}, fmt.Errorf("%w: bad public_key", ErrHandshake)
	}
	pub := ed25519.PublicKey(raw)
	if !identity.VerifyPeerID(pub, msg.PeerID) {
		return Info{}, fmt.Errorf("%w: peer_id does not match public key", ErrHandshake)
	}
	nick := msg.Nickname
	if nick == "" {
		if len(msg.PeerID) >= 8 {
			nick = msg.PeerID[:8]
		} else {
			nick = msg.PeerID
		}
	}
	return Info{
		ID:         msg.PeerID,
		PublicKey:  pub,
		Nickname:   nick,
		ListenPort: msg.Port,
		Addrs:      netutil.SanitizeAddrs(msg.Addrs, 8),
	}, nil
}
