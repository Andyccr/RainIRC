package directory

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

func TestExtraAddrs(t *testing.T) {
	dir := t.TempDir()
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	d.Observe(id, "", "A", "127.0.0.1:7777", "10.0.0.5:7777", "bad")
	addrs, err := d.AddrsFor(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(addrs) != 2 || addrs[0] != "127.0.0.1:7777" || addrs[1] != "10.0.0.5:7777" {
		t.Fatalf("%v", addrs)
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := loaded.AddrsFor(id)
	if err != nil || len(got) != 2 {
		t.Fatalf("persisted %v err=%v", got, err)
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

func TestConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("%064x", i)
			d.Observe(id, "", "n", "127.0.0.1:7777")
			_ = d.Save()
		}(i)
	}
	wg.Wait()
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.List()) == 0 {
		t.Fatal("expected persisted peers after concurrent saves")
	}
}
