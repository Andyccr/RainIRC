package router

import (
	"sync"
	"time"

	"github.com/Andyccr/RainIRC/internal/protocol"
)

// Sender is the outbound path used by the gossip router.
type Sender interface {
	Broadcast(msg *protocol.Message, exceptPeerID string)
}

// Handler is invoked once per unseen routable message.
type Handler func(msg *protocol.Message, fromPeer string)

// Router implements flood/gossip with a bounded seen-message cache.
type Router struct {
	mu      sync.Mutex
	seen    *SeenStore
	sender  Sender
	handler Handler
}

func New(sender Sender, handler Handler, ttl time.Duration, max int) *Router {
	return &Router{
		seen:    NewSeenStore(ttl, max),
		sender:  sender,
		handler: handler,
	}
}

func (r *Router) Seen() *SeenStore { return r.seen }

// Handle is called for a message arriving from fromPeer (empty = local origin).
// Returns true if the message was new.
func (r *Router) Handle(msg *protocol.Message, fromPeer string) bool {
	if msg == nil || !protocol.IsRoutable(msg.Type) {
		return false
	}
	if err := msg.Validate(); err != nil {
		return false
	}
	if !r.seen.Add(msg.ID) {
		return false
	}
	if r.handler != nil {
		r.handler(msg, fromPeer)
	}
	// Direct messages stay on the single hop; they are not flooded.
	if r.sender != nil && msg.To == "" {
		r.sender.Broadcast(msg, fromPeer)
	}
	return true
}

// Inject marks a locally created message as seen, delivers it, and floods it.
func (r *Router) Inject(msg *protocol.Message) bool {
	return r.Handle(msg, "")
}

func (r *Router) Sweep(now time.Time) {
	r.seen.Sweep(now)
}
