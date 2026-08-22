package version

import "testing"

func TestString(t *testing.T) {
	if Version != "0.4.0" {
		t.Fatalf("Version=%s", Version)
	}
	if String() != "p2pirc 0.4.0" {
		t.Fatalf("String=%s", String())
	}
}
