package identity

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityGeneration(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(a.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatalf("private key size %d", len(a.PrivateKey))
	}
	if len(a.PublicKey) != ed25519.PublicKeySize {
		t.Fatalf("public key size %d", len(a.PublicKey))
	}
	if a.PeerID == b.PeerID {
		t.Fatal("two generated identities share a peer ID")
	}
	if len(a.ShortID()) != 8 {
		t.Fatalf("short id %q", a.ShortID())
	}
}

func TestPeerIDDerivation(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	got := DerivePeerID(id.PublicKey)
	if got != id.PeerID {
		t.Fatalf("DerivePeerID = %s, ident.PeerID = %s", got, id.PeerID)
	}
	if !VerifyPeerID(id.PublicKey, id.PeerID) {
		t.Fatal("VerifyPeerID rejected a valid pairing")
	}
	if VerifyPeerID(id.PublicKey, "0000000000000000000000000000000000000000000000000000000000000000") {
		t.Fatal("VerifyPeerID accepted a wrong id")
	}
	other, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if VerifyPeerID(id.PublicKey, other.PeerID) {
		t.Fatal("VerifyPeerID accepted another peer's id")
	}
}

func TestIdentityPersistence(t *testing.T) {
	dir := t.TempDir()
	id, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("first LoadOrCreate: %v", err)
	}
	id.Nickname = "Alice"
	if err := id.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("second LoadOrCreate: %v", err)
	}
	if loaded.PeerID != id.PeerID {
		t.Fatalf("peer id changed: %s -> %s", id.PeerID, loaded.PeerID)
	}
	if hex.EncodeToString(loaded.PublicKey) != hex.EncodeToString(id.PublicKey) {
		t.Fatal("public key changed across restart")
	}
	if hex.EncodeToString(loaded.PrivateKey) != hex.EncodeToString(id.PrivateKey) {
		t.Fatal("private key changed across restart")
	}
	if loaded.Nickname != "Alice" {
		t.Fatalf("nickname = %q", loaded.Nickname)
	}
	if loaded.CreatedAt.IsZero() {
		t.Fatal("created_at should survive restart")
	}
	if !loaded.CreatedAt.Equal(id.CreatedAt) && loaded.CreatedAt.Unix() != id.CreatedAt.Unix() {
		t.Fatalf("created_at changed: %v -> %v", id.CreatedAt, loaded.CreatedAt)
	}
	if len(loaded.Fingerprint()) != 16 {
		t.Fatalf("fingerprint %q", loaded.Fingerprint())
	}
	info, err := os.Stat(filepath.Join(dir, identityFileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("identity.json mode %o, want 0600", info.Mode().Perm())
	}
}

func TestCorruptIdentityRejected(t *testing.T) {
	dir := t.TempDir()
	p := Path(dir)
	if err := os.WriteFile(p, []byte(`{"peer_id":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(dir); err == nil {
		t.Fatal("expected error for corrupt identity")
	}
}
