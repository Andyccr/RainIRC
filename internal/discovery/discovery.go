package discovery

import (
	"encoding/json"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Andyccr/RainIRC/internal/config"
	"github.com/Andyccr/RainIRC/internal/logger"
)

// Announcement is the UDP multicast payload.
type Announcement struct {
	Type     string    `json:"type"`
	PeerID   string    `json:"peer_id"`
	Nickname string    `json:"nickname"`
	Port     int       `json:"port"`
	Addr     string    `json:"-"`
	Seen     time.Time `json:"-"`
}

type Service struct {
	mu     sync.Mutex
	local  Announcement
	seen   map[string]Announcement
	log    *logger.Logger
	conn   *net.UDPConn
	group  *net.UDPAddr
	stop   chan struct{}
	closed bool
}

func New(peerID, nick string, tcpPort int, log *logger.Logger) *Service {
	if log == nil {
		log = logger.New(nil, false)
	}
	return &Service{
		local: Announcement{
			Type:     "discovery",
			PeerID:   peerID,
			Nickname: nick,
			Port:     tcpPort,
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
	closed := s.closed
	s.mu.Unlock()
	if closed || conn == nil || group == nil {
		return
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
	t := time.NewTicker(15 * time.Second)
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

func (s *Service) readLoop() {
	buf := make([]byte, 2048)
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
			return
		}
		var a Announcement
		if err := json.Unmarshal(buf[:n], &a); err != nil {
			continue
		}
		if a.Type != "discovery" || a.PeerID == "" || a.Port <= 0 {
			continue
		}
		s.mu.Lock()
		if a.PeerID == s.local.PeerID {
			s.mu.Unlock()
			continue
		}
		host := src.IP.String()
		a.Addr = net.JoinHostPort(host, strconv.Itoa(a.Port))
		a.Seen = time.Now()
		s.seen[a.PeerID] = a
		s.mu.Unlock()
	}
}
