// Package protocol defines the newline-delimited JSON messages used on the wire.
//
// Protocol version 2: TLS 1.3 transport (default) plus Ed25519 signatures on
// hello/welcome/chat/join/leave. `--plain` disables TLS for debugging only.
package protocol

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	Version        = 2
	MaxMessageSize = 64 * 1024
	MaxTextLen     = 4096
	MaxNickLen     = 32
	MaxChannelLen  = 32
	MaxIDLen       = 128
	MaxPeerIDLen   = 64
)

const (
	TypeHello   = "hello"
	TypeWelcome = "welcome"
	TypeChat    = "chat"
	TypeJoin    = "join"
	TypeLeave   = "leave"
	TypePing    = "ping"
	TypePong    = "pong"
)

// Message is one NDJSON object. Unknown fields are ignored by encoding/json.
type Message struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
	Version   int    `json:"version,omitempty"`

	PeerID    string `json:"peer_id,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Text      string `json:"text,omitempty"`
	Action    bool   `json:"action,omitempty"`
	To        string `json:"to,omitempty"`
	Signature string `json:"signature,omitempty"`
}

func Now() int64 { return time.Now().Unix() }

// NewID returns a 64-char hex SHA-256 of sender + timestamp + nonce + payload.
func NewID(sender string, timestamp int64, payload string) string {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	h := sha256.New()
	h.Write([]byte(sender))
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(timestamp))
	h.Write(ts[:])
	h.Write(nonce)
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

func NewHello(peerID, pubHex, nick string) *Message {
	ts := Now()
	m := &Message{
		Type:      TypeHello,
		Timestamp: ts,
		Version:   Version,
		PeerID:    peerID,
		PublicKey: pubHex,
		Nickname:  nick,
	}
	m.ID = NewID(peerID, ts, TypeHello)
	return m
}

func NewWelcome(peerID, pubHex, nick string) *Message {
	ts := Now()
	m := &Message{
		Type:      TypeWelcome,
		Timestamp: ts,
		Version:   Version,
		PeerID:    peerID,
		PublicKey: pubHex,
		Nickname:  nick,
	}
	m.ID = NewID(peerID, ts, TypeWelcome)
	return m
}

func NewChat(sender, nick, channel, text string, action bool) *Message {
	ts := Now()
	m := &Message{
		Type:      TypeChat,
		Timestamp: ts,
		Sender:    sender,
		Nickname:  nick,
		Channel:   channel,
		Text:      text,
		Action:    action,
	}
	m.ID = NewID(sender, ts, channel+"\n"+text)
	return m
}

func NewDirect(sender, nick, to, text string) *Message {
	ts := Now()
	m := &Message{
		Type:      TypeChat,
		Timestamp: ts,
		Sender:    sender,
		Nickname:  nick,
		To:        to,
		Text:      text,
	}
	m.ID = NewID(sender, ts, to+"\n"+text)
	return m
}

func NewJoin(sender, nick, channel string) *Message {
	ts := Now()
	m := &Message{
		Type:      TypeJoin,
		Timestamp: ts,
		Sender:    sender,
		Nickname:  nick,
		Channel:   channel,
	}
	m.ID = NewID(sender, ts, "join:"+channel)
	return m
}

func NewLeave(sender, nick, channel string) *Message {
	ts := Now()
	m := &Message{
		Type:      TypeLeave,
		Timestamp: ts,
		Sender:    sender,
		Nickname:  nick,
		Channel:   channel,
	}
	m.ID = NewID(sender, ts, "leave:"+channel)
	return m
}

func NewPing(sender string) *Message {
	ts := Now()
	m := &Message{Type: TypePing, Timestamp: ts}
	m.ID = NewID(sender, ts, TypePing)
	return m
}

func NewPong(sender string) *Message {
	ts := Now()
	m := &Message{Type: TypePong, Timestamp: ts}
	m.ID = NewID(sender, ts, TypePong)
	return m
}

func IsRoutable(t string) bool {
	switch t {
	case TypeChat, TypeJoin, TypeLeave:
		return true
	default:
		return false
	}
}

func ValidNickname(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > MaxNickLen {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func NormalizeChannel(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty channel name")
	}
	if !strings.HasPrefix(s, "#") {
		s = "#" + s
	}
	s = strings.ToLower(s)
	if len(s) < 2 || len(s) > MaxChannelLen {
		return "", fmt.Errorf("channel name length")
	}
	for i, r := range s {
		if i == 0 {
			continue
		}
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return "", fmt.Errorf("invalid channel character")
		}
	}
	return s, nil
}

func (m *Message) ValidateEnvelope() error {
	if m == nil {
		return fmt.Errorf("nil message")
	}
	if m.Type == "" {
		return fmt.Errorf("missing type")
	}
	if m.ID == "" || len(m.ID) > MaxIDLen {
		return fmt.Errorf("invalid id")
	}
	if m.Timestamp == 0 {
		return fmt.Errorf("missing timestamp")
	}
	return nil
}

func (m *Message) Validate() error {
	if err := m.ValidateEnvelope(); err != nil {
		return err
	}
	switch m.Type {
	case TypeHello, TypeWelcome:
		if m.Version != Version {
			return fmt.Errorf("unsupported protocol version %d", m.Version)
		}
		if len(m.PeerID) != MaxPeerIDLen {
			return fmt.Errorf("invalid peer_id")
		}
		if m.PublicKey == "" {
			return fmt.Errorf("missing public_key")
		}
	case TypeChat:
		if m.Sender == "" {
			return fmt.Errorf("missing sender")
		}
		if m.To == "" {
			if m.Channel == "" {
				return fmt.Errorf("missing channel")
			}
			ch, err := NormalizeChannel(m.Channel)
			if err != nil {
				return err
			}
			m.Channel = ch
		}
		if m.Text == "" {
			return fmt.Errorf("missing text")
		}
		if utf8.RuneCountInString(m.Text) > MaxTextLen {
			return fmt.Errorf("text too long")
		}
	case TypeJoin, TypeLeave:
		if m.Sender == "" {
			return fmt.Errorf("missing sender")
		}
		ch, err := NormalizeChannel(m.Channel)
		if err != nil {
			return err
		}
		m.Channel = ch
	case TypePing, TypePong:
		return nil
	default:
		return fmt.Errorf("unknown type %q", m.Type)
	}
	return nil
}
