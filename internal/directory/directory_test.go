package directory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAliasPersistence(t *testing.T) {
	dir := t.TempDir()
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	d.Observe("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "pub", "Bob", "127.0.0.1:7777")
	if err := d.SetAlias("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Bobby"); err != nil {
		t.Fatal(err)
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Alias("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") != "Bobby" {
		t.Fatal("alias not persisted")
	}
	addr, err := loaded.AddrFor("Bobby")
	if err != nil || addr != "127.0.0.1:7777" {
		t.Fatalf("addr=%s err=%v", addr, err)
	}
	if loaded.DisplayName("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Bob") != "Bobby" {
		t.Fatal("display should prefer alias")
	}
}

func TestAliasUniqueAndInvalid(t *testing.T) {
	d, _ := Load(t.TempDir())
	id1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	id2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	d.Observe(id1, "", "A", "")
	d.Observe(id2, "", "B", "")
	if err := d.SetAlias(id1, "same"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetAlias(id2, "same"); err == nil {
		t.Fatal("duplicate alias should fail")
	}
	if err := d.SetAlias(id1, "host.local"); err == nil {
		t.Fatal("dotted alias should fail")
	}
	if err := d.ClearAlias("same"); err != nil {
		t.Fatal(err)
	}
	if d.Alias(id1) != "" {
		t.Fatal("alias still set")
	}
}

func TestValidAlias(t *testing.T) {
	if !ValidAlias("Laptop") || ValidAlias("") || ValidAlias("a:b") || ValidAlias("#no") {
		t.Fatal("ValidAlias mismatch")
	}
}
