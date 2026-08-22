// Package directory persists known peers and local aliases.
// Aliases are local labels; they are not part of cryptographic identity.
package directory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Andyccr/RainIRC/internal/netutil"
)

const fileName = "peers.json"

type Record struct {
	PeerID     string   `json:"peer_id"`
	PublicKey  string   `json:"public_key,omitempty"`
	Nickname   string   `json:"nickname,omitempty"`
	Alias      string   `json:"alias,omitempty"`
	LastAddr   string   `json:"last_addr,omitempty"`
	ExtraAddrs []string `json:"addrs,omitempty"`
	LastSeen   string   `json:"last_seen,omitempty"`
}

type fileData struct {
	Peers []Record `json:"peers"`
}

// Directory is a thread-safe, file-backed table of known peers.
type Directory struct {
	mu   sync.RWMutex
	path string
	byID map[string]*Record
}

func Path(dir string) string {
	return filepath.Join(dir, fileName)
}

func New(dataDir string) *Directory {
	return &Directory{
		path: Path(dataDir),
		byID: make(map[string]*Record),
	}
}

func Load(dataDir string) (*Directory, error) {
	d := New(dataDir)
	raw, err := os.ReadFile(d.path)
	if err != nil {
		if os.IsNotExist(err) {
			return d, nil
		}
		return nil, err
	}
	var fd fileData
	if err := json.Unmarshal(raw, &fd); err != nil {
		return nil, fmt.Errorf("parse peers.json: %w", err)
	}
	for i := range fd.Peers {
		rec := fd.Peers[i]
		if rec.PeerID == "" {
			continue
		}
		cp := rec
		d.byID[rec.PeerID] = &cp
	}
	return d, nil
}

func (d *Directory) Save() error {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	recs := make([]Record, 0, len(d.byID))
	for _, rec := range d.byID {
		recs = append(recs, *rec)
	}
	d.mu.RUnlock()
	sort.Slice(recs, func(i, j int) bool { return recs[i].PeerID < recs[j].PeerID })
	data, err := json.MarshalIndent(fileData{Peers: recs}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(d.path), 0o700); err != nil {
		return err
	}
	tmp := d.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, d.path)
}

func ValidAlias(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > 32 {
		return false
	}
	if strings.ContainsAny(s, ".:/@#") {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func (d *Directory) Observe(peerID, pubHex, nick, addr string, extra ...string) {
	if d == nil || peerID == "" {
		return
	}
	d.mu.Lock()
	rec := d.ensureLocked(peerID)
	if pubHex != "" {
		rec.PublicKey = pubHex
	}
	if nick != "" {
		rec.Nickname = nick
	}
	if addr != "" {
		rec.LastAddr = addr
	}
	if len(extra) > 0 {
		rec.ExtraAddrs = netutil.SanitizeAddrs(append(append([]string{}, rec.ExtraAddrs...), extra...), 8)
	}
	rec.LastSeen = time.Now().UTC().Format(time.RFC3339)
	d.mu.Unlock()
}

func (d *Directory) SetAlias(peerID, alias string) error {
	if d == nil {
		return fmt.Errorf("no directory")
	}
	alias = strings.TrimSpace(alias)
	if !ValidAlias(alias) {
		return fmt.Errorf("alias must be 1-32 chars without . : / @ #")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if existing := d.idByAliasLocked(alias); existing != "" && existing != peerID {
		return fmt.Errorf("alias %q already used by %s", alias, existing[:min(8, len(existing))])
	}
	rec := d.ensureLocked(peerID)
	rec.Alias = alias
	return nil
}

func (d *Directory) ClearAlias(key string) error {
	if d == nil {
		return fmt.Errorf("no directory")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, err := d.lookupLocked(key)
	if err != nil {
		return err
	}
	if rec.Alias == "" {
		return fmt.Errorf("no alias set")
	}
	rec.Alias = ""
	return nil
}

func (d *Directory) Alias(peerID string) string {
	if d == nil {
		return ""
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if rec, ok := d.byID[peerID]; ok {
		return rec.Alias
	}
	return ""
}

func (d *Directory) DisplayName(peerID, nickname string) string {
	if a := d.Alias(peerID); a != "" {
		return a
	}
	if nickname != "" {
		return nickname
	}
	if len(peerID) >= 8 {
		return peerID[:8]
	}
	return peerID
}

func (d *Directory) Lookup(key string) (*Record, error) {
	if d == nil {
		return nil, fmt.Errorf("no directory")
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	rec, err := d.lookupLocked(key)
	if err != nil {
		return nil, err
	}
	cp := *rec
	return &cp, nil
}

func (d *Directory) AddrFor(key string) (string, error) {
	addrs, err := d.AddrsFor(key)
	if err != nil {
		return "", err
	}
	return addrs[0], nil
}

func (d *Directory) AddrsFor(key string) ([]string, error) {
	rec, err := d.Lookup(key)
	if err != nil {
		return nil, err
	}
	out := netutil.SanitizeAddrs(append([]string{rec.LastAddr}, rec.ExtraAddrs...), 8)
	if len(out) == 0 {
		return nil, fmt.Errorf("no saved address for %s", d.DisplayName(rec.PeerID, rec.Nickname))
	}
	return out, nil
}

func (d *Directory) List() []Record {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Record, 0, len(d.byID))
	for _, rec := range d.byID {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := strings.ToLower(out[i].Alias), strings.ToLower(out[j].Alias)
		if ai != aj {
			if ai == "" {
				return false
			}
			if aj == "" {
				return true
			}
			return ai < aj
		}
		return out[i].PeerID < out[j].PeerID
	})
	return out
}

func (d *Directory) ReconnectTargets() []Record {
	var out []Record
	for _, rec := range d.List() {
		if rec.LastAddr != "" || len(rec.ExtraAddrs) > 0 {
			out = append(out, rec)
		}
	}
	return out
}

func (d *Directory) ensureLocked(peerID string) *Record {
	rec, ok := d.byID[peerID]
	if !ok {
		rec = &Record{PeerID: peerID}
		d.byID[peerID] = rec
	}
	return rec
}

func (d *Directory) idByAliasLocked(alias string) string {
	want := strings.EqualFold
	for id, rec := range d.byID {
		if rec.Alias != "" && want(rec.Alias, alias) {
			return id
		}
	}
	return ""
}

func (d *Directory) lookupLocked(key string) (*Record, error) {
	key = strings.TrimSpace(key)
	if rec, ok := d.byID[key]; ok {
		return rec, nil
	}
	if id := d.idByAliasLocked(key); id != "" {
		return d.byID[id], nil
	}
	var found *Record
	for id, rec := range d.byID {
		if len(key) >= 4 && strings.HasPrefix(id, key) {
			if found != nil {
				return nil, fmt.Errorf("ambiguous prefix %q", key)
			}
			found = rec
		}
	}
	if found != nil {
		return found, nil
	}
	return nil, fmt.Errorf("unknown peer %q", key)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
