package peer

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Andyccr/RainIRC/internal/identity"
	"github.com/Andyccr/RainIRC/internal/logger"
	"github.com/Andyccr/RainIRC/internal/protocol"
)

func testManager(t *testing.T, ident *identity.Identity) *Manager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return NewManager(ctx, ident.PeerID, logger.New(io.Discard, false), protocol.MaxMessageSize, time.Second, 5*time.Second)
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func TestPeerManager(t *testing.T) {
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	am := testManager(t, aID)
	bm := testManager(t, bID)

	ln := listen(t)
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		_, err = bm.HandshakeAndAdopt(c, bID, "B", true, time.Second)
		errCh <- err
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := am.HandshakeAndAdopt(conn, aID, "A", false, time.Second); err != nil {
		t.Fatalf("dial handshake: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("accept handshake: %v", err)
	}
	if am.Len() != 1 || bm.Len() != 1 {
		t.Fatalf("peer counts A=%d B=%d", am.Len(), bm.Len())
	}
	list := am.List()
	if list[0].ID != bID.PeerID {
		t.Fatalf("A sees %s, want %s", list[0].ID, bID.PeerID)
	}
}

func TestConnectionLifecycle(t *testing.T) {
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	am := testManager(t, aID)
	bm := testManager(t, bID)
	down := make(chan Info, 2)
	am.SetHooks(nil, nil, func(info Info) { down <- info })

	ln := listen(t)
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		_, err = bm.HandshakeAndAdopt(c, bID, "B", true, time.Second)
		errCh <- err
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := am.HandshakeAndAdopt(conn, aID, "A", false, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if err := am.Disconnect(bID.ShortID()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-down:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for disconnect")
	}
	deadline := time.Now().Add(2 * time.Second)
	for am.Len() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if am.Len() != 0 {
		t.Fatalf("A still has %d peers", am.Len())
	}
}

func TestHandshakeRejectsMismatchedPeerID(t *testing.T) {
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	bm := testManager(t, bID)
	ln := listen(t)
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		_, err = bm.HandshakeAndAdopt(c, bID, "B", true, time.Second)
		errCh <- err
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	bad := protocol.NewHello(aID.PeerID, bID.PublicKeyHex(), "evil")
	if err := protocol.Write(conn, bad); err != nil {
		t.Fatal(err)
	}
	err = <-errCh
	if err == nil {
		t.Fatal("expected handshake failure")
	}
	if !errors.Is(err, ErrHandshake) {
		t.Fatalf("got %v", err)
	}
}

func TestMalformedJSONDoesNotPanic(t *testing.T) {
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	am := testManager(t, aID)
	bm := testManager(t, bID)
	got := make(chan *protocol.Message, 4)
	bm.SetHooks(func(_ *Conn, m *protocol.Message) {
		if m.Type == protocol.TypeChat {
			got <- m
		}
	}, nil, nil)

	ln := listen(t)
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		_, err = bm.HandshakeAndAdopt(c, bID, "B", true, time.Second)
		errCh <- err
	}()
	raw, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ac, err := am.HandshakeAndAdopt(raw, aID, "A", false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	// Inject malformed JSON on A's socket via a parallel write? We send through
	// the live connection by writing to a hijacked copy — instead, send a
	// well-formed unknown payload then a chat message through Send.
	if _, err := raw.Write([]byte("{not json}\n")); err != nil {
		t.Fatal(err)
	}
	chat := protocol.NewChat(aID.PeerID, "A", "#general", "still alive", false)
	if err := ac.Send(chat); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m.Text != "still alive" {
			t.Fatalf("text=%s", m.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("peer did not survive malformed JSON")
	}
}

func TestOversizedMessageCloses(t *testing.T) {
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	am := testManager(t, aID)
	bm := testManager(t, bID)
	down := make(chan struct{}, 1)
	bm.SetHooks(nil, nil, func(Info) { down <- struct{}{} })

	ln := listen(t)
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		_, err = bm.HandshakeAndAdopt(c, bID, "B", true, time.Second)
		errCh <- err
	}()
	raw, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := am.HandshakeAndAdopt(raw, aID, "A", false, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	huge := make([]byte, protocol.MaxMessageSize+8)
	for i := range huge {
		huge[i] = 'x'
	}
	huge = append(huge, '\n')
	if _, err := raw.Write(huge); err != nil {
		t.Fatal(err)
	}
	select {
	case <-down:
	case <-time.After(2 * time.Second):
		t.Fatal("oversized message should drop the connection")
	}
}
