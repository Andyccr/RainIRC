// Package stun implements a minimal RFC 5389 Binding client (IPv4).
// Tests speak to a local mock server and do not need Internet access.
package stun

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

const (
	magicCookie = 0x2112A442
	bindingReq  = 0x0001
	bindingOK   = 0x0101
	attrMapped  = 0x0001
	attrXOR     = 0x0020
	headerLen   = 20
)

// Mapped is the XOR-MAPPED-ADDRESS (or MAPPED-ADDRESS) from a Binding reply.
type Mapped struct {
	IP   net.IP
	Port int
}

func (m Mapped) String() string {
	if m.IP == nil {
		return ""
	}
	return net.JoinHostPort(m.IP.String(), fmt.Sprintf("%d", m.Port))
}

// Binding sends a STUN Binding request to server ("host:port") and returns
// the mapped address. Tests should pass a local mock, not a public server.
func Binding(ctx context.Context, server string) (Mapped, error) {
	if server == "" {
		return Mapped{}, fmt.Errorf("empty STUN server")
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp4", server)
	if err != nil {
		return Mapped{}, err
	}
	defer conn.Close()

	txid := make([]byte, 12)
	if _, err := rand.Read(txid); err != nil {
		return Mapped{}, err
	}
	req := make([]byte, headerLen)
	binary.BigEndian.PutUint16(req[0:2], bindingReq)
	binary.BigEndian.PutUint16(req[2:4], 0)
	binary.BigEndian.PutUint32(req[4:8], magicCookie)
	copy(req[8:20], txid)

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	}
	if _, err := conn.Write(req); err != nil {
		return Mapped{}, err
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return Mapped{}, err
	}
	return parseBinding(buf[:n], txid)
}

func parseBinding(msg, txid []byte) (Mapped, error) {
	if len(msg) < headerLen {
		return Mapped{}, fmt.Errorf("short STUN message")
	}
	typ := binary.BigEndian.Uint16(msg[0:2])
	if typ != bindingOK {
		return Mapped{}, fmt.Errorf("unexpected STUN type 0x%04x", typ)
	}
	if binary.BigEndian.Uint32(msg[4:8]) != magicCookie {
		return Mapped{}, fmt.Errorf("bad magic cookie")
	}
	if string(msg[8:20]) != string(txid) {
		return Mapped{}, fmt.Errorf("transaction id mismatch")
	}
	length := int(binary.BigEndian.Uint16(msg[2:4]))
	body := msg[headerLen:]
	if length > len(body) {
		length = len(body)
	}
	body = body[:length]
	var mapped, xor Mapped
	for len(body) >= 4 {
		at := binary.BigEndian.Uint16(body[0:2])
		al := int(binary.BigEndian.Uint16(body[2:4]))
		body = body[4:]
		if al > len(body) {
			break
		}
		val := body[:al]
		pad := (4 - al%4) % 4
		switch at {
		case attrMapped:
			m, err := parseAddr(val, false, txid)
			if err == nil {
				mapped = m
			}
		case attrXOR:
			m, err := parseAddr(val, true, txid)
			if err == nil {
				xor = m
			}
		}
		skip := al + pad
		if skip > len(body) {
			break
		}
		body = body[skip:]
	}
	if xor.IP != nil {
		return xor, nil
	}
	if mapped.IP != nil {
		return mapped, nil
	}
	return Mapped{}, fmt.Errorf("no mapped address in STUN reply")
}

func parseAddr(val []byte, xor bool, txid []byte) (Mapped, error) {
	if len(val) < 8 {
		return Mapped{}, fmt.Errorf("short address")
	}
	family := val[1]
	port := binary.BigEndian.Uint16(val[2:4])
	if xor {
		port ^= uint16(magicCookie >> 16)
	}
	switch family {
	case 0x01:
		if len(val) < 8 {
			return Mapped{}, fmt.Errorf("short IPv4")
		}
		ip := make(net.IP, 4)
		copy(ip, val[4:8])
		if xor {
			magic := []byte{0x21, 0x12, 0xa4, 0x42}
			for i := 0; i < 4; i++ {
				ip[i] ^= magic[i]
			}
		}
		return Mapped{IP: ip, Port: int(port)}, nil
	default:
		return Mapped{}, fmt.Errorf("unsupported family %d", family)
	}
}

// ServeBinding is a tiny in-process STUN server for tests.
// It reports mapped as the UDP source address of each request.
func ServeBinding(laddr string) (net.Addr, func(), error) {
	pc, err := net.ListenPacket("udp4", laddr)
	if err != nil {
		return nil, nil, err
	}
	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < headerLen {
				continue
			}
			txid := make([]byte, 12)
			copy(txid, buf[8:20])
			ua, ok := addr.(*net.UDPAddr)
			if !ok || ua.IP.To4() == nil {
				continue
			}
			ip := ua.IP.To4()
			xport := uint16(ua.Port) ^ uint16(magicCookie>>16)
			xip := make([]byte, 4)
			magic := []byte{0x21, 0x12, 0xa4, 0x42}
			for i := 0; i < 4; i++ {
				xip[i] = ip[i] ^ magic[i]
			}
			attr := make([]byte, 12)
			binary.BigEndian.PutUint16(attr[0:2], attrXOR)
			binary.BigEndian.PutUint16(attr[2:4], 8)
			attr[5] = 0x01
			binary.BigEndian.PutUint16(attr[6:8], xport)
			copy(attr[8:12], xip)
			resp := make([]byte, headerLen+len(attr))
			binary.BigEndian.PutUint16(resp[0:2], bindingOK)
			binary.BigEndian.PutUint16(resp[2:4], uint16(len(attr)))
			binary.BigEndian.PutUint32(resp[4:8], magicCookie)
			copy(resp[8:20], txid)
			copy(resp[20:], attr)
			_, _ = pc.WriteTo(resp, addr)
		}
	}()
	return pc.LocalAddr(), func() { _ = pc.Close() }, nil
}
