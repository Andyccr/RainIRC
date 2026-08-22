package discovery

import (
	"testing"
	"time"

	"github.com/Andyccr/RainIRC/internal/identity"
)

func TestSignedAnnouncement(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	a := Announcement{
		Type:      "discovery",
		Version:   2,
		PeerID:    id.PeerID,
		PublicKey: id.PublicKeyHex(),
		Nickname:  "Alice",
		Port:      7777,
		TLS:       true,
		Timestamp: time.Now().Unix(),
	}
	if err := a.Sign(id.PrivateKey); err != nil {
		t.Fatal(err)
	}
	if err := a.Verify(); err != nil {
		t.Fatal(err)
	}
	a.Port = 9
	if err := a.Verify(); err == nil {
		t.Fatal("tampered port must fail")
	}
}

func TestUnsignedRejected(t *testing.T) {
	a := Announcement{Type: "discovery", PeerID: "x", Port: 1}
	if err := a.Verify(); err == nil {
		t.Fatal("unsigned should fail")
	}
}

func TestWrongIdentityRejected(t *testing.T) {
	a, _ := identity.Generate()
	b, _ := identity.Generate()
	ann := Announcement{
		Type:      "discovery",
		PeerID:    a.PeerID,
		PublicKey: b.PublicKeyHex(),
		Port:      7777,
		Timestamp: time.Now().Unix(),
	}
	_ = ann.Sign(b.PrivateKey)
	if err := ann.Verify(); err == nil {
		t.Fatal("mismatched peer_id must fail")
	}
}
