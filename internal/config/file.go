package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const FileName = "config"

// FilePath is <data-dir>/config.
func FilePath(dataDir string) string {
	return filepath.Join(dataDir, FileName)
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", v)
	}
}

func splitKV(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}
	if i := strings.IndexByte(line, '='); i >= 0 {
		return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
	}
	fields := strings.Fields(line)
	if len(fields) == 1 {
		return fields[0], "true", true
	}
	return fields[0], strings.Join(fields[1:], " "), true
}

func applyFile(cfg *Config, path string, set map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, ";") {
			continue
		}
		key, value, ok := splitKV(raw)
		if !ok || key == "" {
			return fmt.Errorf("%s:%d: expected key=value", path, lineNo)
		}
		key = strings.ToLower(key)
		if set[key] {
			continue
		}
		if err := applyKey(cfg, key, value); err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	return sc.Err()
}

func applyKey(cfg *Config, key, value string) error {
	switch key {
	case "port":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("port: %w", err)
		}
		cfg.Port = n
	case "nickname":
		cfg.Nickname = value
	case "debug":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Debug = b
	case "no-discover":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.NoDiscover = b
	case "plain":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Plain = b
	case "auto-connect":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.AutoConnect = b
	case "reconnect":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Reconnect = b
	case "lan":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Lan = b
	case "stun":
		cfg.STUNServer = value
	case "no-stun":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.NoSTUN = b
	case "upnp":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.UPnP = b
	case "max-peers":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("max-peers: %w", err)
		}
		cfg.MaxPeers = n
	case "data-dir":
		return fmt.Errorf("data-dir belongs on the command line, not in the config file")
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}

func applyLan(cfg *Config) {
	if cfg.Lan {
		cfg.AutoConnect = true
		cfg.Reconnect = true
	}
}
