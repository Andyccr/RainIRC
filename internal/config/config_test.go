package config

import "testing"

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != DefaultPort {
		t.Fatalf("port=%d", cfg.Port)
	}
	if cfg.Reconnect || cfg.AutoConnect || cfg.Plain {
		t.Fatal("optional flags should be off")
	}
	if cfg.DataDir == "" {
		t.Fatal("expected default data dir")
	}
}

func TestParseFlags(t *testing.T) {
	cfg, err := Parse([]string{
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
}

func TestParseInvalidPort(t *testing.T) {
	if _, err := Parse([]string{"--port", "-1"}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Parse([]string{"--max-peers", "-2"}); err == nil {
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
