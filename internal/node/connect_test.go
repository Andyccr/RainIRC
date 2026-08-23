package node

import "testing"

func TestBeginDialDedupe(t *testing.T) {
	n := &Node{}
	if !n.beginDial("127.0.0.1:1") {
		t.Fatal("first dial")
	}
	if n.beginDial("127.0.0.1:1") {
		t.Fatal("same address must be in-flight")
	}
	if !n.beginDial("127.0.0.1:2") {
		t.Fatal("other address")
	}
	n.endDial("127.0.0.1:1")
	if !n.beginDial("127.0.0.1:1") {
		t.Fatal("after end")
	}
}
