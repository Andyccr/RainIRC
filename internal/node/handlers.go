package node

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Andyccr/RainIRC/internal/chat"
	"github.com/Andyccr/RainIRC/internal/peer"
	"github.com/Andyccr/RainIRC/internal/protocol"
)

func (n *Node) onPeerMessage(c *peer.Conn, msg *protocol.Message) {
	switch msg.Type {
	case protocol.TypePing:
		if c.AllowPong(time.Second) {
			_ = c.Send(protocol.NewPong(n.ident.PeerID))
		}
		return
	case protocol.TypePong:
		return
	case protocol.TypeHello, protocol.TypeWelcome:
		n.log.Debugf("unexpected %s after handshake from %s", msg.Type, c.Info().ShortID())
		return
	}
	if !protocol.IsRoutable(msg.Type) {
		return
	}
	if err := msg.VerifySignature(); err != nil {
		n.log.Warnf("drop %s from %s: %v", msg.Type, c.Info().ShortID(), err)
		return
	}
	if err := msg.Fresh(time.Now()); err != nil {
		n.log.Debugf("drop %s from %s: %v", msg.Type, c.Info().ShortID(), err)
		return
	}
	sender := msg.Sender
	if sender == "" {
		sender = msg.PeerID
	}
	if n.limit != nil && !n.limit.Allow(sender) {
		n.log.Debugf("rate-limit %s from %s", msg.Type, shortID(sender))
		return
	}
	n.router.Handle(msg, c.ID())
}

func (n *Node) applyNick(peerID, nick string) {
	if peerID == "" || nick == "" {
		return
	}
	n.chans.UpdateNick(peerID, nick)
	n.peers.UpdateNick(peerID, nick)
	if n.dir != nil {
		n.dir.Observe(peerID, "", nick, "")
	}
}

func (n *Node) onRouted(msg *protocol.Message, fromPeer string) {
	switch msg.Type {
	case protocol.TypeChat:
		n.applyNick(msg.Sender, msg.Nickname)
		if msg.To != "" && msg.To != n.ident.PeerID && msg.Sender != n.ident.PeerID {
			return
		}
		if msg.To == "" {
			n.chans.AddHistory(msg)
			if !n.chans.Joined(msg.Channel) {
				n.chans.Note(msg.Channel)
				return
			}
		}
		n.emit(Event{Kind: EventChat, Text: chat.FormatChat(msg, n.displayName(msg.Sender, msg.Nickname)), Msg: msg})
	case protocol.TypeJoin:
		ch, err := protocol.NormalizeChannel(msg.Channel)
		if err != nil {
			return
		}
		n.chans.Note(ch)
		members := n.chans.Members(ch)
		prev, existed := "", false
		if members != nil {
			prev, existed = members[msg.Sender]
		}
		n.chans.MemberJoin(ch, msg.Sender, msg.Nickname)
		n.applyNick(msg.Sender, msg.Nickname)
		if !n.chans.Joined(ch) || msg.Sender == n.ident.PeerID {
			return
		}
		if !existed {
			n.emit(Event{Kind: EventJoin, Text: fmt.Sprintf("%s joined %s", n.displayName(msg.Sender, msg.Nickname), ch), Msg: msg})
			return
		}
		if msg.Nickname != "" && prev != msg.Nickname {
			n.emit(Event{Kind: EventSystem, Text: fmt.Sprintf("%s is now %s", n.displayName(msg.Sender, prev), msg.Nickname), Msg: msg})
		}
	case protocol.TypeLeave:
		ch, err := protocol.NormalizeChannel(msg.Channel)
		if err != nil {
			return
		}
		n.chans.MemberLeave(ch, msg.Sender)
		if n.chans.Joined(ch) && msg.Sender != n.ident.PeerID {
			n.emit(Event{Kind: EventLeave, Text: fmt.Sprintf("%s left %s", n.displayName(msg.Sender, msg.Nickname), ch), Msg: msg})
		}
	}
}

func (n *Node) onPeerUp(info peer.Info) {
	n.observePeer(info)
	n.System("connected to %s (%s)", info.ShortID(), n.displayName(info.ID, info.Nickname))
	n.emit(Event{Kind: EventPeerUp, Text: info.ShortID(), Peer: info})
	n.resyncJoins(info.ID)
}

func (n *Node) onPeerReplace(info peer.Info) {
	n.observePeer(info)
	n.resyncJoins(info.ID)
}

func (n *Node) resyncJoins(peerID string) {
	for _, ch := range n.chans.JoinedNames() {
		msg, err := n.sign(protocol.NewJoin(n.ident.PeerID, n.Nick(), ch))
		if err != nil {
			continue
		}
		_ = n.peers.SendTo(peerID, msg)
		n.router.Seen().Add(msg.ID)
	}
}

func (n *Node) onPeerDown(info peer.Info) {
	n.chans.MemberLeaveAll(info.ID)
	n.System("disconnected from %s (%s)", info.ShortID(), n.displayName(info.ID, info.Nickname))
	n.emit(Event{Kind: EventPeerDown, Text: info.ShortID(), Peer: info})
}

func (n *Node) dirSaveLoop() {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-t.C:
			if n.dir != nil {
				if err := n.dir.Save(); err != nil {
					n.log.Debugf("save peer directory: %v", err)
				}
			}
		}
	}
}

func (n *Node) seenLoop() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-t.C:
			n.router.Sweep(time.Now())
		}
	}
}

func (n *Node) sign(msg *protocol.Message) (*protocol.Message, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}
	if err := msg.Sign(n.ident.PrivateKey); err != nil {
		return nil, err
	}
	return msg, nil
}

func (n *Node) formatNames(channel string) string {
	if channel == "" {
		channel = n.chans.Current()
	}
	ch, err := protocol.NormalizeChannel(channel)
	if err != nil {
		return err.Error()
	}
	members := n.chans.Members(ch)
	if members == nil {
		return fmt.Sprintf("not in %s", ch)
	}
	ids := make([]string, 0, len(members))
	for id := range members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	fmt.Fprintf(&b, "Names %s (%d):", ch, len(members))
	for _, id := range ids {
		fmt.Fprintf(&b, "\n  %s  %s", shortID(id), n.displayName(id, members[id]))
	}
	return b.String()
}
