package discovery

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Andyccr/RainIRC/internal/config"
	"github.com/Andyccr/RainIRC/internal/identity"
	"github.com/Andyccr/RainIRC/internal/logger"
)

// Announcement is the UDP multicast payload.
type Announcement struct {
	Type      string `json:"type"`
	Version   int    `json:"version,omitempty"`
	PeerID    string `json:"peer_id"`
	PublicKey string `json:"public_key,omitempty"`
	Nickname  string `json:"nickname"`
	Port      int    `json:"port"`
	TLS       bool   `json:"tls,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Signature string `json:"signature,omitempty"`

	Addr     string    `json:"-"`
	Seen     time.Time `json:"-"`
	Verified bool      `json:"-"`
}

type Service struct {
	mu     sync.Mutex
	ident  *identity.Identity
	local  Announcement
	seen   map[string]Announcement
	log    *logger.Logger
	conn   *net.UDPConn
	group  *net.UDPAddr
	stop   chan struct{}
	closed bool
	onPeer func(Announcement)
}

func New(ident *identity.Identity, nick string, tcpPort int, tls bool, log *logger.Logger) *Service {
	if log == nil {
		log = logger.New(nil, false)
	}
	peerID := ""
	pub := ""
	if ident != nil {
		peerID = ident.PeerID
		pub = ident.PublicKeyHex()
	}
	return &Service{
		ident: ident,
		local: Announcement{
			Type:      "discovery",
			Version:   2,
			PeerID:    peerID,
			PublicKey: pub,
			Nickname:  nick,
			Port:      tcpPort,
			TLS:       tls,
		},
		seen: make(map[string]Announcement),
		log:  log,
		stop: make(chan struct{}),
	}
}

func (s *Service) SetNickname(n string) {
	s.mu.Lock()
	s.local.Nickname = n
	s.mu.Unlock()
}

func (s *Service) SetOnPeer(fn func(Announcement)) {
	s.mu.Lock()
	s.onPeer = fn
	s.mu.Unlock()
}

func (s *Service) Start() error {
	group := &net.UDPAddr{
		IP:   net.ParseIP(config.MulticastGroup),
		Port: config.MulticastPort,
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return err
	}
	_ = conn.SetReadBuffer(4096)
	s.mu.Lock()
	s.conn = conn
	s.group = group
	s.mu.Unlock()
	go s.readLoop()
	go s.announceLoop()
	return nil
}

func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.stop)
	c := s.conn
	s.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

func (s *Service) Announce() {
	s.mu.Lock()
	conn := s.conn
	group := s.group
	payload := s.local
	ident := s.ident
	closed := s.closed
	s.mu.Unlock()
	if closed || conn == nil || group == nil {
		return
	}
	payload.Timestamp = time.Now().Unix()
	if ident != nil {
		_ = payload.Sign(ident.PrivateKey)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = conn.WriteToUDP(data, group)
}

func (s *Service) Nearby() []Announcement {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-45 * time.Second)
	out := make([]Announcement, 0, len(s.seen))
	for id, a := range s.seen {
		if a.Seen.Before(cutoff) {
			delete(s.seen, id)
			continue
		}
		out = append(out, a)
	}
	return out
}

func (s *Service) announceLoop() {
	s.Announce()
	t := time.NewTicker(8 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.Announce()
		}
	}
}

const maxSeen = 256

func (s *Service) readLoop() {
	buf := make([]byte, 4096)
	for {
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn == nil {
			return
		}
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			s.log.Debugf("discovery read: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var a Announcement
		if err := json.Unmarshal(buf[:n], &a); err != nil {
			continue
		}
		if a.Type != "discovery" || a.PeerID == "" || a.Port <= 0 || a.Port > 65535 {
			continue
		}
		a.Verified = a.Verify() == nil
		s.mu.Lock()
		if a.PeerID == s.local.PeerID {
			s.mu.Unlock()
			continue
		}
		host := src.IP.String()
		a.Addr = net.JoinHostPort(host, strconv.Itoa(a.Port))
		a.Seen = time.Now()
		_, existed := s.seen[a.PeerID]
		s.seen[a.PeerID] = a
		s.pruneSeenLocked(time.Now())
		cb := s.onPeer
		s.mu.Unlock()
		if cb != nil && !existed {
			go cb(a)
		}
	}
}

func (s *Service) pruneSeenLocked(now time.Time) {
	cutoff := now.Add(-45 * time.Second)
	for id, a := range s.seen {
		if a.Seen.Before(cutoff) {
			delete(s.seen, id)
		}
	}
	for len(s.seen) > maxSeen {
		var oldest string
		var t time.Time
		first := true
		for id, a := range s.seen {
			if first || a.Seen.Before(t) {
				oldest, t, first = id, a.Seen, false
			}
		}
		if oldest == "" {
			break
		}
		delete(s.seen, oldest)
	}
}

func (a *Announcement) signBytes() []byte {
	tls := "0"
	if a.TLS {
		tls = "1"
	}
	return []byte(strings.Join([]string{
		a.Type,
		strconv.Itoa(a.Version),
		a.PeerID,
		a.PublicKey,
		a.Nickname,
		strconv.Itoa(a.Port),
		tls,
		strconv.FormatInt(a.Timestamp, 10),
	}, "\n"))
}

func (a *Announcement) Sign(priv ed25519.PrivateKey) error {
	if a.PublicKey == "" {
		a.PublicKey = hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	}
	a.Signature = hex.EncodeToString(ed25519.Sign(priv, a.signBytes()))
	return nil
}

func (a *Announcement) Verify() error {
	if a.Signature == "" || a.PublicKey == "" {
		return fmt.Errorf("unsigned")
	}
	pub, err := hex.DecodeString(a.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("bad public_key")
	}
	sum := sha256.Sum256(pub)
	if hex.EncodeToString(sum[:]) != strings.ToLower(a.PeerID) {
		return fmt.Errorf("peer_id mismatch")
	}
	sig, err := hex.DecodeString(a.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("bad signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), a.signBytes(), sig) {
		return fmt.Errorf("invalid signature")
	}
	if a.Timestamp > 0 {
		now := time.Now().Unix()
		if a.Timestamp > now+120 || a.Timestamp < now-300 {
			return fmt.Errorf("timestamp out of window")
		}
	}
	return nil
}
