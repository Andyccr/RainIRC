package transport

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Andyccr/RainIRC/internal/identity"
)

var errMismatch = errors.New("tls public key mismatch")

func TestTLSHandshake(t *testing.T) {
	a, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	b, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	c1, c2 := net.Pipe()
	t.Cleanup(func() { _ = c1.Close(); _ = c2.Close() })
	errc := make(chan error, 2)
	go func() {
		conn, err := Handshake(c1, a, true, 3*time.Second)
		if err == nil {
			pub, perr := PeerPublicKey(conn)
			if perr != nil {
				err = perr
			} else if !pub.Equal(b.PublicKey) {
				err = errMismatch
			}
		}
		errc <- err
	}()
	go func() {
		conn, err := Handshake(c2, b, false, 3*time.Second)
		if err == nil {
			pub, perr := PeerPublicKey(conn)
			if perr != nil {
				err = perr
			} else if !pub.Equal(a.PublicKey) {
				err = errMismatch
			}
		}
		errc <- err
	}()
	for i := 0; i < 2; i++ {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}
}

func TestCertificateIsEd25519(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := TLSConfig(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Certificates) != 1 || len(cfg.Certificates[0].Certificate) == 0 {
		t.Fatal("expected a certificate")
	}
}
