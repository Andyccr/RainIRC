// Package config holds process-level settings parsed from flags and defaults.
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultPort          = 7777
	DefaultMaxMessage    = 64 * 1024
	DefaultPingInterval  = 20 * time.Second
	DefaultIdleTimeout   = 60 * time.Second
	DefaultSeenTTL       = 24 * time.Hour
	DefaultSeenMax       = 50000
	DefaultHistoryPerCh  = 200
	DefaultHandshakeWait = 5 * time.Second
	DefaultDialTimeout   = 10 * time.Second
	DefaultMaxPeers      = 64
	DefaultMaxHandshakes = 32
	MulticastGroup       = "239.255.77.77"
	MulticastPort        = 7776
)

// Config is the runtime configuration for one peer process.
type Config struct {
	Port        int
	Nickname    string
	DataDir     string
	Debug       bool
	ListenHost  string
	NoDiscover  bool
	Plain       bool // disable TLS (insecure)
	AutoConnect bool // connect to verified LAN discoveries
	Reconnect   bool // keep retrying last-known peer addresses from peers.json
	STUNServer  string
	NoSTUN      bool
	UPnP        bool
	ShowVersion bool
	MaxPeers    int

	MaxMessageSize int
	MaxHandshakes  int
	PingInterval   time.Duration
	IdleTimeout    time.Duration
	SeenTTL        time.Duration
	SeenMax        int
	HistoryLimit   int
	HandshakeWait  time.Duration
	DialTimeout    time.Duration
}

func Default() *Config {
	return &Config{
		Port:           DefaultPort,
		ListenHost:     "0.0.0.0",
		MaxMessageSize: DefaultMaxMessage,
		PingInterval:   DefaultPingInterval,
		IdleTimeout:    DefaultIdleTimeout,
		SeenTTL:        DefaultSeenTTL,
		SeenMax:        DefaultSeenMax,
		HistoryLimit:   DefaultHistoryPerCh,
		HandshakeWait:  DefaultHandshakeWait,
		DialTimeout:    DefaultDialTimeout,
		MaxPeers:       DefaultMaxPeers,
		MaxHandshakes:  DefaultMaxHandshakes,
	}
}

// Parse reads command-line flags. args should typically be os.Args[1:].
func Parse(args []string) (*Config, error) {
	cfg := Default()
	fs := flag.NewFlagSet("p2pirc", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.IntVar(&cfg.Port, "port", DefaultPort, "TCP listen port (0 = ephemeral)")
	fs.StringVar(&cfg.Nickname, "nickname", "", "cosmetic nickname (default: first 8 chars of peer ID)")
	fs.StringVar(&cfg.DataDir, "data-dir", "", "directory for identity (default: ~/.p2pirc)")
	fs.BoolVar(&cfg.Debug, "debug", false, "enable debug logging on stderr")
	fs.BoolVar(&cfg.NoDiscover, "no-discover", false, "disable LAN UDP multicast discovery")
	fs.BoolVar(&cfg.Plain, "plain", false, "disable TLS (insecure; debug only)")
	fs.BoolVar(&cfg.AutoConnect, "auto-connect", false, "automatically connect to verified LAN peers")
	fs.BoolVar(&cfg.Reconnect, "reconnect", false, "keep retrying last-known peer addresses from peers.json every 5s")
	fs.StringVar(&cfg.STUNServer, "stun", "stun.l.google.com:19302", "STUN server host:port (UDP Binding; not a TCP hole punch)")
	fs.BoolVar(&cfg.NoSTUN, "no-stun", false, "do not query STUN")
	fs.BoolVar(&cfg.UPnP, "upnp", false, "try IGD AddPortMapping for the TCP listen port (opt-in)")
	fs.BoolVar(&cfg.ShowVersion, "version", false, "print version and exit")
	fs.IntVar(&cfg.MaxPeers, "max-peers", DefaultMaxPeers, "maximum live TCP/TLS peer sessions (0 = unlimited)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid port %d", cfg.Port)
	}
	if cfg.DataDir == "" {
		dir, err := DefaultDataDir()
		if err != nil {
			return nil, err
		}
		cfg.DataDir = dir
	}
	return cfg, nil
}

func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".p2pirc"), nil
}
