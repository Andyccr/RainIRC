package channel

import (
	"testing"

	"github.com/Andyccr/RainIRC/internal/protocol"
)

func TestJoinChannel(t *testing.T) {
	m := NewManager("local", 10)
	if !m.Join("#general", "Alice") {
		t.Fatal("first join should be fresh")
	}
	if m.Join("#general", "Alice") {
		t.Fatal("second join should not be fresh")
	}
	if !m.Joined("#general") {
		t.Fatal("expected membership")
	}
	if m.Current() != "#general" {
		t.Fatalf("current=%s", m.Current())
	}
}

func TestLeaveChannel(t *testing.T) {
	m := NewManager("local", 10)
	m.Join("#general", "Alice")
	m.Join("#dev", "Alice")
	if err := m.Leave("#dev"); err != nil {
		t.Fatal(err)
	}
	if m.Joined("#dev") {
		t.Fatal("still joined #dev")
	}
	if m.Current() != "#general" {
		t.Fatalf("current=%s after leaving #dev", m.Current())
	}
	if err := m.Leave("#missing"); err == nil {
		t.Fatal("expected error")
	}
}

func TestHistoryBounded(t *testing.T) {
	m := NewManager("local", 3)
	m.Join("#general", "A")
	for i := 0; i < 5; i++ {
		m.AddHistory(protocol.NewChat("local", "A", "#general", "x", false))
	}
	h := m.History("#general")
	if len(h) != 3 {
		t.Fatalf("history len %d, want 3", len(h))
	}
}

func TestMemberJoinLeave(t *testing.T) {
	m := NewManager("local", 10)
	m.Join("#general", "Alice")
	m.MemberJoin("#general", "remote", "Bob")
	members := m.Members("#general")
	if members["remote"] != "Bob" {
		t.Fatalf("members=%v", members)
	}
	m.MemberLeave("#general", "remote")
	if _, ok := m.Members("#general")["remote"]; ok {
		t.Fatal("remote still a member")
	}
}

func TestUpdateNick(t *testing.T) {
	m := NewManager("local", 10)
	m.Join("#general", "Alice")
	m.MemberJoin("#general", "remote", "Bob")
	m.UpdateNick("remote", "Robert")
	if m.Members("#general")["remote"] != "Robert" {
		t.Fatalf("nick=%q", m.Members("#general")["remote"])
	}
}

func TestMemberLeaveAll(t *testing.T) {
	m := NewManager("local", 10)
	m.Join("#general", "Alice")
	m.Join("#dev", "Alice")
	m.MemberJoin("#general", "remote", "Bob")
	m.MemberJoin("#dev", "remote", "Bob")
	m.MemberLeaveAll("remote")
	if _, ok := m.Members("#general")["remote"]; ok {
		t.Fatal("remote still in #general")
	}
	if _, ok := m.Members("#dev")["remote"]; ok {
		t.Fatal("remote still in #dev")
	}
}
