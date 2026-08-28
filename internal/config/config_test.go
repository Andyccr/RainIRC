package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Parse([]string{"--data-dir", dir})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != DefaultPort {
		t.Fatalf("port=%d", cfg.Port)
	}
	if cfg.Reconnect || cfg.AutoConnect || cfg.Plain || cfg.Lan {
		t.Fatal("optional flags should be off")
	}
	if cfg.DataDir != dir {
		t.Fatalf("data-dir=%s", cfg.DataDir)
	}
}

func TestParseFlags(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Parse([]string{
		"--data-dir", dir,
		"--port", "0",
		"--nickname", "Alice",
		"--reconnect",
		"--no-stun",
		"--max-peers", "8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 0 || cfg.Nickname != "Alice" || !cfg.Reconnect || cfg.MaxPeers != 8 {
		t.Fatalf("%+v", cfg)
	}
	if !cfg.NoSTUN {
		t.Fatal("expected --no-stun")
	}
	if cfg.AutoConnect {
		t.Fatal("--reconnect must not imply auto-connect")
	}
}

func TestParseLan(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Parse([]string{"--data-dir", dir, "--lan"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Lan || !cfg.AutoConnect || !cfg.Reconnect {
		t.Fatalf("lan preset: %+v", cfg)
	}
}

func TestParseConfigFile(t *testing.T) {
	dir := t.TempDir()
	body := "# local mesh\n" +
		"nickname = Alice\n" +
		"lan true\n" +
		"port 9000\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse([]string{"--data-dir", dir})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Nickname != "Alice" || cfg.Port != 9000 || !cfg.Lan || !cfg.AutoConnect || !cfg.Reconnect {
		t.Fatalf("%+v", cfg)
	}
}

func TestParseCLIOverridesFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("nickname=File\nport=1111\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse([]string{"--data-dir", dir, "--nickname", "CLI", "--port", "2222"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Nickname != "CLI" || cfg.Port != 2222 {
		t.Fatalf("%+v", cfg)
	}
}

func TestParseConfigBoolWords(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("debug=yes\nupnp=on\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse([]string{"--data-dir", dir})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Debug || !cfg.UPnP {
		t.Fatalf("%+v", cfg)
	}
}

func TestParseUnknownConfigKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("relay=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]string{"--data-dir", dir}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseInvalidPort(t *testing.T) {
	dir := t.TempDir()
	if _, err := Parse([]string{"--data-dir", dir, "--port", "-1"}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Parse([]string{"--data-dir", dir, "--max-peers", "-2"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseVersion(t *testing.T) {
	cfg, err := Parse([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ShowVersion {
		t.Fatal("expected --version")
	}
}
