// Package transport wraps a TCP connection with TLS 1.3.
//
// Certificates are self-signed Ed25519 certs derived from the peer identity.
// There is no WebPKI / CA. After the TLS handshake, the application still
// verifies that the certificate public key matches the NDJSON hello/welcome
// identity (see peer.Manager).
package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/Andyccr/RainIRC/internal/identity"
)

const ALPN = "p2pirc/2"

var (
	certMu   sync.Mutex
	certByID = map[string]tls.Certificate{}
)

// Handshake upgrades raw to a mutually authenticated TLS 1.3 connection.
func Handshake(raw net.Conn, ident *identity.Identity, inbound bool, wait time.Duration) (net.Conn, error) {
	if wait <= 0 {
		wait = 5 * time.Second
	}
	cfg, err := TLSConfig(ident)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	var conn *tls.Conn
	if inbound {
		conn = tls.Server(raw, cfg)
	} else {
		conn = tls.Client(raw, cfg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// TLSConfig returns a TLS 1.3 config bound to ident's Ed25519 key.
func TLSConfig(ident *identity.Identity) (*tls.Config, error) {
	cert, err := certificate(ident)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		ClientAuth:         tls.RequireAnyClientCert,
		InsecureSkipVerify: true, // no CA; VerifyPeerCertificate checks Ed25519
		NextProtos:         []string{ALPN},
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &cert, nil
		},
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("missing peer certificate")
			}
			parsed, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("parse peer certificate: %w", err)
			}
			if _, ok := parsed.PublicKey.(ed25519.PublicKey); !ok {
				return fmt.Errorf("peer certificate is not Ed25519")
			}
			return nil
		},
	}, nil
}

// PeerPublicKey returns the Ed25519 key from the TLS peer certificate.
// On a plaintext connection it returns (nil, nil).
func PeerPublicKey(conn net.Conn) (ed25519.PublicKey, error) {
	tc, ok := conn.(*tls.Conn)
	if !ok {
		return nil, nil
	}
	certs := tc.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no peer certificate")
	}
	pub, ok := certs[0].PublicKey.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("peer certificate is not Ed25519")
	}
	return pub, nil
}

func certificate(ident *identity.Identity) (tls.Certificate, error) {
	if ident == nil {
		return tls.Certificate{}, fmt.Errorf("nil identity")
	}
	certMu.Lock()
	if c, ok := certByID[ident.PeerID]; ok {
		certMu.Unlock()
		return c, nil
	}
	certMu.Unlock()

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: ident.ShortID(), Organization: []string{"P2P-IRC"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, ident.PublicKey, ident.PrivateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  ident.PrivateKey,
	}
	certMu.Lock()
	certByID[ident.PeerID] = cert
	certMu.Unlock()
	return cert, nil
}
