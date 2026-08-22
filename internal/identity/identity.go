// Package identity provides a stable Ed25519 peer identity.
//
// SECURITY LIMITATION (MVP): the private key is stored on disk as hex in
// identity.json with file mode 0600. There is no OS keychain integration and
// no passphrase. Anyone who can read that file can impersonate this peer.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	identityFileName = "identity.json"
	shortIDLen       = 8
)

// Identity is a long-lived peer keypair and derived identifiers.
type Identity struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	PeerID     string // full SHA-256(public key) hex
	Nickname   string // cosmetic; not an identity
	CreatedAt  time.Time
}

func (id *Identity) ShortID() string {
	if len(id.PeerID) < shortIDLen {
		return id.PeerID
	}
	return id.PeerID[:shortIDLen]
}

func (id *Identity) PublicKeyHex() string {
	return hex.EncodeToString(id.PublicKey)
}

func (id *Identity) DefaultNickname() string {
	if id.Nickname != "" {
		return id.Nickname
	}
	return id.ShortID()
}

// Fingerprint is the first 16 hex characters of the Peer ID.
func (id *Identity) Fingerprint() string {
	if len(id.PeerID) < 16 {
		return id.PeerID
	}
	return id.PeerID[:16]
}

// DerivePeerID returns the full hex SHA-256 of the raw public key bytes.
func DerivePeerID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

// VerifyPeerID reports whether claimedID is SHA-256(pub).
func VerifyPeerID(pub ed25519.PublicKey, claimedID string) bool {
	return DerivePeerID(pub) == strings.ToLower(claimedID)
}

// Generate creates a fresh Ed25519 identity.
func Generate() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return &Identity{
		PrivateKey: priv,
		PublicKey:  pub,
		PeerID:     DerivePeerID(pub),
		CreatedAt:  time.Now().UTC(),
	}, nil
}

type fileRecord struct {
	PeerID     string `json:"peer_id"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
	Nickname   string `json:"nickname,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

func (id *Identity) toRecord() fileRecord {
	created := id.CreatedAt.UTC()
	if created.IsZero() {
		created = time.Now().UTC()
		id.CreatedAt = created
	}
	return fileRecord{
		PeerID:     id.PeerID,
		PublicKey:  hex.EncodeToString(id.PublicKey),
		PrivateKey: hex.EncodeToString(id.PrivateKey),
		Nickname:   id.Nickname,
		CreatedAt:  created.Format(time.RFC3339),
	}
}

func fromRecord(rec fileRecord) (*Identity, error) {
	privRaw, err := hex.DecodeString(rec.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("private_key hex: %w", err)
	}
	pubRaw, err := hex.DecodeString(rec.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("public_key hex: %w", err)
	}
	if len(privRaw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("private_key length %d, want %d", len(privRaw), ed25519.PrivateKeySize)
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public_key length %d, want %d", len(pubRaw), ed25519.PublicKeySize)
	}
	priv := ed25519.PrivateKey(privRaw)
	pub := ed25519.PublicKey(pubRaw)
	derivedPub := priv.Public().(ed25519.PublicKey)
	if !pub.Equal(derivedPub) {
		return nil, fmt.Errorf("public key does not match private key")
	}
	peerID := DerivePeerID(pub)
	if rec.PeerID != "" && rec.PeerID != peerID {
		return nil, fmt.Errorf("stored peer_id does not match public key")
	}
	id := &Identity{
		PrivateKey: priv,
		PublicKey:  pub,
		PeerID:     peerID,
		Nickname:   rec.Nickname,
	}
	if rec.CreatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, rec.CreatedAt); err == nil {
			id.CreatedAt = ts
		}
	}
	return id, nil
}

func Path(dir string) string {
	return filepath.Join(dir, identityFileName)
}

// LoadOrCreate returns the identity in dir, generating one if missing.
func LoadOrCreate(dir string) (*Identity, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	p := Path(dir)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			id, err := Generate()
			if err != nil {
				return nil, err
			}
			if err := id.Save(dir); err != nil {
				return nil, err
			}
			return id, nil
		}
		return nil, fmt.Errorf("read identity: %w", err)
	}
	var rec fileRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parse identity.json: %w", err)
	}
	return fromRecord(rec)
}

// Save writes identity.json using a temp file + rename. Mode 0600.
func (id *Identity) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	rec := id.toRecord()
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	p := Path(dir)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace identity: %w", err)
	}
	return nil
}
