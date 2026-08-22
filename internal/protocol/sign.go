package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrUnsigned         = errors.New("missing signature")
	ErrBadSignature     = errors.New("invalid signature")
	ErrIdentityMismatch = errors.New("sender/peer_id does not match public key")
	ErrMissingPublicKey = errors.New("missing public_key")
)

func RequiresSignature(t string) bool {
	switch t {
	case TypeHello, TypeWelcome, TypeChat, TypeJoin, TypeLeave:
		return true
	default:
		return false
	}
}

// SignBytes is the canonical payload that is signed / verified.
// The signature field itself is excluded so it can be attached after signing.
func (m *Message) SignBytes() []byte {
	if m == nil {
		return nil
	}
	action := "0"
	if m.Action {
		action = "1"
	}
	parts := []string{
		m.Type,
		m.ID,
		strconv.FormatInt(m.Timestamp, 10),
		m.PeerID,
		m.Sender,
		m.PublicKey,
		m.Nickname,
		m.Channel,
		m.To,
		m.Text,
		action,
	}
	return []byte(strings.Join(parts, "\n"))
}

// Sign attaches an Ed25519 signature using priv. PublicKey is filled from the
// private key if empty.
func (m *Message) Sign(priv ed25519.PrivateKey) error {
	if m == nil {
		return fmt.Errorf("nil message")
	}
	if len(priv) != ed25519.PrivateKeySize {
		return fmt.Errorf("bad private key")
	}
	if m.PublicKey == "" {
		m.PublicKey = hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	}
	m.Signature = hex.EncodeToString(ed25519.Sign(priv, m.SignBytes()))
	return nil
}

// VerifySignature checks public_key, Peer ID binding, and the Ed25519 signature.
func (m *Message) VerifySignature() error {
	if m == nil {
		return fmt.Errorf("nil message")
	}
	if m.PublicKey == "" {
		return ErrMissingPublicKey
	}
	if m.Signature == "" {
		return ErrUnsigned
	}
	pub, err := hex.DecodeString(m.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("bad public_key")
	}
	claimed := m.Sender
	if claimed == "" {
		claimed = m.PeerID
	}
	if claimed != "" {
		sum := sha256.Sum256(pub)
		if hex.EncodeToString(sum[:]) != strings.ToLower(claimed) {
			return ErrIdentityMismatch
		}
	}
	sig, err := hex.DecodeString(m.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), m.SignBytes(), sig) {
		return ErrBadSignature
	}
	return nil
}
