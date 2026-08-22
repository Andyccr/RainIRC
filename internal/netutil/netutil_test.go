package netutil

import (
	"net"
	"testing"
)

func TestLocalCandidates(t *testing.T) {
	got := LocalCandidates(7777)
	if len(got) == 0 {
		t.Fatal("expected at least loopback")
	}
	found := false
	for _, a := range got {
		if a == "127.0.0.1:7777" {
			found = true
		}
		if _, _, err := net.SplitHostPort(a); err != nil {
			t.Fatalf("bad candidate %q: %v", a, err)
		}
	}
	if !found {
		t.Fatalf("missing loopback in %v", got)
	}
}

func TestUnique(t *testing.T) {
	got := Unique([]string{"a", "", "b", "a"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("%v", got)
	}
}

func TestSanitizeAddrs(t *testing.T) {
	got := SanitizeAddrs([]string{
		" 10.0.0.1:7777 ",
		"nope",
		"10.0.0.1:7777",
		"10.0.0.2:99999",
		"",
	}, 8)
	if len(got) != 1 || got[0] != "10.0.0.1:7777" {
		t.Fatalf("%v", got)
	}
}

func TestAdvertiseCandidatesSkipLoopback(t *testing.T) {
	for _, a := range AdvertiseCandidates(7777) {
		host, _, err := net.SplitHostPort(a)
		if err != nil {
			t.Fatal(err)
		}
		if host == "127.0.0.1" {
			t.Fatalf("loopback leaked: %v", a)
		}
	}
}
