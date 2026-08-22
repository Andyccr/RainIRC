package protocol

import (
	"testing"

	"github.com/Andyccr/RainIRC/internal/identity"
)

func TestSignAndVerify(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	m := NewChat(id.PeerID, "Alice", "#general", "hello", false)
	if err := m.Sign(id.PrivateKey); err != nil {
		t.Fatal(err)
	}
	if m.Signature == "" || m.PublicKey == "" {
		t.Fatal("signature or public key missing")
	}
	if err := m.VerifySignature(); err != nil {
		t.Fatal(err)
	}
}

func TestTamperedMessageRejected(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	m := NewChat(id.PeerID, "Alice", "#general", "hello", false)
	if err := m.Sign(id.PrivateKey); err != nil {
		t.Fatal(err)
	}
	m.Text = "evil"
	if err := m.VerifySignature(); err == nil {
		t.Fatal("tampered text must fail")
	}
}

func TestWrongKeyRejected(t *testing.T) {
	a, _ := identity.Generate()
	b, _ := identity.Generate()
	m := NewChat(a.PeerID, "Alice", "#general", "hello", false)
	if err := m.Sign(b.PrivateKey); err != nil {
		t.Fatal(err)
	}
	// Sign fills/keeps public key: already set from NewChat? NewChat doesn't set PublicKey.
	// Sign with B sets PublicKey to B if empty, then sender is still A.
	if err := m.VerifySignature(); err == nil {
		t.Fatal("sender A with key B must fail")
	}
}

func TestUnsignedRejected(t *testing.T) {
	m := NewChat("aa", "A", "#general", "hello", false)
	if err := m.VerifySignature(); err == nil {
		t.Fatal("unsigned must fail")
	}
}

func TestHelloSignature(t *testing.T) {
	id, _ := identity.Generate()
	m := NewHello(id.PeerID, id.PublicKeyHex(), "Alice")
	if err := m.Sign(id.PrivateKey); err != nil {
		t.Fatal(err)
	}
	if err := m.VerifySignature(); err != nil {
		t.Fatal(err)
	}
}
