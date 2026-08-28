package version

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	if Version != "0.5.1" {
		t.Fatalf("Version=%s", Version)
	}
	if String() != "p2pirc 0.5.1" {
		t.Fatalf("String=%s", String())
	}
	old := Commit
	Commit = "abc1234"
	t.Cleanup(func() { Commit = old })
	if !strings.Contains(String(), "abc1234") {
		t.Fatalf("String=%s", String())
	}
}
