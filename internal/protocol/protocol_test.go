package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestMessageEncoding(t *testing.T) {
	m := NewChat("abcd", "Alice", "#general", "hello", false)
	data, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Fatal("encoded message missing trailing newline")
	}
	if bytes.Count(data, []byte("\n")) != 1 {
		t.Fatal("expected exactly one newline")
	}
	var round Message
	if err := json.Unmarshal(bytes.TrimSpace(data), &round); err != nil {
		t.Fatal(err)
	}
	if round.Type != TypeChat || round.Text != "hello" || round.Channel != "#general" {
		t.Fatalf("round trip mismatch: %+v", round)
	}
}

func TestMessageDecoding(t *testing.T) {
	raw := `{"type":"chat","id":"abc123","channel":"#general","sender":"7f3a91c2","nickname":"Alice","timestamp":1787390200,"text":"hello"}` + "\n"
	m, err := Read(bufio.NewReader(strings.NewReader(raw)), MaxMessageSize)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if m.Text != "hello" || m.Sender != "7f3a91c2" {
		t.Fatalf("decoded %+v", m)
	}
}

func TestMalformedMessage(t *testing.T) {
	cases := []string{
		"{not json}\n",
		"[]\n",
		"{\"type\":\"chat\"}\n", // missing id/timestamp
		"\n",
	}
	for _, c := range cases {
		_, err := Read(bufio.NewReader(strings.NewReader(c)), MaxMessageSize)
		if err == nil {
			t.Fatalf("expected error for %q", c)
		}
		if !errors.Is(err, ErrMalformed) && !errors.Is(err, io.EOF) {
			// envelope errors wrap ErrMalformed
			if !strings.Contains(err.Error(), "malformed") && !strings.Contains(err.Error(), "invalid") && !strings.Contains(err.Error(), "missing") {
				t.Fatalf("unexpected error for %q: %v", c, err)
			}
		}
	}
	m := &Message{Type: TypeChat, ID: "x", Timestamp: 1, Sender: "a", Channel: "#bad channel", Text: "hi"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected invalid channel to fail")
	}
}

func TestMessageSizeLimit(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), MaxMessageSize+16)
	huge = append([]byte(`{"type":"chat","id":"1","timestamp":1,"text":"`), huge...)
	huge = append(huge, []byte(`"}`+"\n")...)
	_, err := Read(bufio.NewReader(bytes.NewReader(huge)), MaxMessageSize)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
}

func TestHelloValidateVersion(t *testing.T) {
	m := NewHello(strings.Repeat("a", 64), strings.Repeat("b", 64), "n")
	m.Version = 99
	if err := m.Validate(); err == nil {
		t.Fatal("expected version rejection")
	}
}

func TestNormalizeChannel(t *testing.T) {
	got, err := NormalizeChannel("General")
	if err != nil || got != "#general" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := NormalizeChannel("#bad channel"); err == nil {
		t.Fatal("expected error")
	}
}
