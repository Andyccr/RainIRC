package channel

import (
	"fmt"
	"sort"
	"sync"

	"github.com/Andyccr/RainIRC/internal/protocol"
)

type Channel struct {
	Name    string
	History []*protocol.Message
	Members map[string]string // peerID -> nickname
}

type Manager struct {
	mu      sync.RWMutex
	joined  map[string]*Channel
	known   map[string]struct{}
	current string
	histLim int
	localID string
}

func NewManager(localID string, historyLimit int) *Manager {
	if historyLimit <= 0 {
		historyLimit = 200
	}
	return &Manager{
		joined:  make(map[string]*Channel),
		known:   make(map[string]struct{}),
		histLim: historyLimit,
		localID: localID,
	}
}

func (m *Manager) Current() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *Manager) SetCurrent(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = name
}

func (m *Manager) Joined(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.joined[name]
	return ok
}

func (m *Manager) Join(name, localNick string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.joined[name]
	if !ok {
		ch = &Channel{Name: name, Members: make(map[string]string)}
		m.joined[name] = ch
	}
	ch.Members[m.localID] = localNick
	m.known[name] = struct{}{}
	m.current = name
	return !ok
}

func (m *Manager) Leave(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.joined[name]; !ok {
		return fmt.Errorf("not in %s", name)
	}
	delete(m.joined, name)
	if m.current == name {
		m.current = ""
		for n := range m.joined {
			m.current = n
			break
		}
	}
	return nil
}

func (m *Manager) JoinedNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.joined))
	for n := range m.joined {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) KnownNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.known))
	for n := range m.known {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) Note(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.known[name] = struct{}{}
}

func (m *Manager) AddHistory(msg *protocol.Message) {
	if msg == nil || msg.Channel == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.known[msg.Channel] = struct{}{}
	ch, ok := m.joined[msg.Channel]
	if !ok {
		return
	}
	ch.History = append(ch.History, msg)
	if len(ch.History) > m.histLim {
		ch.History = ch.History[len(ch.History)-m.histLim:]
	}
}

func (m *Manager) History(name string) []*protocol.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.joined[name]
	if !ok {
		return nil
	}
	out := make([]*protocol.Message, len(ch.History))
	copy(out, ch.History)
	return out
}

func (m *Manager) MemberJoin(channel, peerID, nick string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.known[channel] = struct{}{}
	ch, ok := m.joined[channel]
	if !ok {
		return
	}
	ch.Members[peerID] = nick
}

func (m *Manager) MemberLeave(channel, peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.joined[channel]
	if !ok {
		return
	}
	delete(ch.Members, peerID)
}

func (m *Manager) Members(name string) map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.joined[name]
	if !ok {
		return nil
	}
	out := make(map[string]string, len(ch.Members))
	for k, v := range ch.Members {
		out[k] = v
	}
	return out
}

func (m *Manager) UpdateNick(peerID, nick string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.joined {
		if _, ok := ch.Members[peerID]; ok {
			ch.Members[peerID] = nick
		}
	}
}
