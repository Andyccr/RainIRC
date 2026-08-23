package node

import (
	"fmt"
	"strings"

	"github.com/Andyccr/RainIRC/internal/chat"
	"github.com/Andyccr/RainIRC/internal/config"
	"github.com/Andyccr/RainIRC/internal/version"
)

type cmdHandler func(*Node, chat.Command) (bool, error)

var commands = map[chat.Kind]cmdHandler{
	chat.KindChat:       cmdChat,
	chat.KindHelp:       cmdHelp,
	chat.KindConnect:    cmdConnect,
	chat.KindDisconnect: cmdDisconnect,
	chat.KindPeers:      cmdPeers,
	chat.KindJoin:       cmdJoin,
	chat.KindLeave:      cmdLeave,
	chat.KindChannels:   cmdChannels,
	chat.KindNick:       cmdNick,
	chat.KindMe:         cmdMe,
	chat.KindMsg:        cmdMsg,
	chat.KindDiscover:   cmdDiscover,
	chat.KindWhoami:     cmdWhoami,
	chat.KindAlias:      cmdAlias,
	chat.KindUnalias:    cmdUnalias,
	chat.KindKnown:      cmdKnown,
	chat.KindAddr:       cmdAddr,
	chat.KindVersion:    cmdVersion,
	chat.KindStats:      cmdStats,
	chat.KindNames:      cmdNames,
	chat.KindQuit:       cmdQuit,
}

func (n *Node) HandleLine(line string) (quit bool, err error) {
	cmd := chat.Parse(line)
	if cmd.Kind == chat.KindUnknown {
		return false, fmt.Errorf("unknown command %s (try /help)", cmd.Raw)
	}
	fn, ok := commands[cmd.Kind]
	if !ok {
		return false, fmt.Errorf("unknown command %s (try /help)", cmd.Raw)
	}
	return fn(n, cmd)
}

func cmdChat(n *Node, cmd chat.Command) (bool, error) {
	if cmd.Raw == "" || len(cmd.Args) < 1 {
		return false, nil
	}
	return false, n.SendChat(cmd.Args[0], false)
}

func cmdHelp(n *Node, _ chat.Command) (bool, error) {
	n.System("%s", chat.HelpText())
	return false, nil
}

func cmdConnect(n *Node, cmd chat.Command) (bool, error) {
	if len(cmd.Args) < 1 {
		return false, fmt.Errorf("usage: /connect <host:port|alias>")
	}
	return false, n.Connect(cmd.Args[0])
}

func cmdDisconnect(n *Node, cmd chat.Command) (bool, error) {
	if len(cmd.Args) < 1 {
		return false, fmt.Errorf("usage: /disconnect <peer-id>")
	}
	return false, n.Disconnect(cmd.Args[0])
}

func cmdPeers(n *Node, _ chat.Command) (bool, error) {
	list := n.peers.List()
	if len(list) == 0 {
		n.System("no connected peers")
		return false, nil
	}
	var b strings.Builder
	b.WriteString("Connected peers:")
	for _, p := range list {
		dir := "out"
		if p.Inbound {
			dir = "in"
		}
		fmt.Fprintf(&b, "\n  %s  %s  %s  (%s)", p.ShortID(), n.displayName(p.ID, p.Nickname), p.Addr, dir)
	}
	n.System("%s", b.String())
	return false, nil
}

func cmdJoin(n *Node, cmd chat.Command) (bool, error) {
	if len(cmd.Args) < 1 {
		return false, fmt.Errorf("usage: /join #channel")
	}
	return false, n.Join(cmd.Args[0])
}

func cmdLeave(n *Node, cmd chat.Command) (bool, error) {
	name := n.chans.Current()
	if len(cmd.Args) >= 1 {
		name = cmd.Args[0]
	}
	return false, n.Leave(name)
}

func cmdChannels(n *Node, _ chat.Command) (bool, error) {
	var b strings.Builder
	cur := n.chans.Current()
	b.WriteString("Joined:")
	for _, name := range n.chans.JoinedNames() {
		mark := ""
		if name == cur {
			mark = "  (current)"
		}
		fmt.Fprintf(&b, "\n  %s%s", name, mark)
	}
	b.WriteString("\nKnown:")
	for _, name := range n.chans.KnownNames() {
		fmt.Fprintf(&b, "\n  %s", name)
	}
	n.System("%s", b.String())
	return false, nil
}

func cmdNick(n *Node, cmd chat.Command) (bool, error) {
	if len(cmd.Args) < 1 {
		return false, fmt.Errorf("usage: /nick <name>")
	}
	return false, n.SetNick(cmd.Args[0])
}

func cmdMe(n *Node, cmd chat.Command) (bool, error) {
	if len(cmd.Args) < 1 || strings.TrimSpace(cmd.Args[0]) == "" {
		return false, fmt.Errorf("usage: /me <action>")
	}
	return false, n.SendChat(cmd.Args[0], true)
}

func cmdMsg(n *Node, cmd chat.Command) (bool, error) {
	if len(cmd.Args) < 2 {
		return false, fmt.Errorf("usage: /msg <peer-id> <text>")
	}
	return false, n.SendDirect(cmd.Args[0], cmd.Args[1])
}

func cmdDiscover(n *Node, cmd chat.Command) (bool, error) {
	if len(cmd.Args) >= 1 && strings.EqualFold(cmd.Args[0], "connect") {
		count, err := n.connectNearbyVerified()
		if err != nil {
			return false, err
		}
		n.System("connected to %d nearby verified peer(s)", count)
		return false, nil
	}
	list := n.Nearby()
	if len(list) == 0 {
		n.System("no nearby peers (LAN multicast %s:%d)", config.MulticastGroup, config.MulticastPort)
		return false, nil
	}
	var b strings.Builder
	b.WriteString("Nearby peers:")
	for _, a := range list {
		mark := "unverified"
		if a.Verified {
			mark = "verified"
		}
		if n.peers.Connected(a.PeerID) {
			mark += ", connected"
		}
		alias := ""
		if n.dir != nil && n.dir.Alias(a.PeerID) != "" {
			alias = "  alias=" + n.dir.Alias(a.PeerID)
		}
		tls := ""
		if a.TLS {
			tls = " tls"
		}
		fmt.Fprintf(&b, "\n  %s  %s%s  %s  (%s%s)", shortID(a.PeerID), a.Nickname, alias, a.Addr, mark, tls)
	}
	n.System("%s", b.String())
	return false, nil
}

func cmdWhoami(n *Node, _ chat.Command) (bool, error) {
	n.System("%s", n.whoami())
	return false, nil
}

func cmdAlias(n *Node, cmd chat.Command) (bool, error) {
	if len(cmd.Args) == 0 {
		n.System("%s", n.formatAliases())
		return false, nil
	}
	if len(cmd.Args) < 2 {
		return false, fmt.Errorf("usage: /alias <peer-id> <name>")
	}
	return false, n.setAlias(cmd.Args[0], cmd.Args[1])
}

func cmdUnalias(n *Node, cmd chat.Command) (bool, error) {
	if len(cmd.Args) < 1 {
		return false, fmt.Errorf("usage: /unalias <name|peer-id>")
	}
	return false, n.clearAlias(cmd.Args[0])
}

func cmdKnown(n *Node, _ chat.Command) (bool, error) {
	n.System("%s", n.formatKnown())
	return false, nil
}

func cmdAddr(n *Node, _ chat.Command) (bool, error) {
	n.System("%s", n.formatAddrs())
	return false, nil
}

func cmdVersion(n *Node, _ chat.Command) (bool, error) {
	n.System("%s", version.String())
	return false, nil
}

func cmdStats(n *Node, _ chat.Command) (bool, error) {
	n.System("%s", n.formatStats())
	return false, nil
}

func cmdNames(n *Node, cmd chat.Command) (bool, error) {
	ch := n.chans.Current()
	if len(cmd.Args) >= 1 {
		ch = cmd.Args[0]
	}
	n.System("%s", n.formatNames(ch))
	return false, nil
}

func cmdQuit(*Node, chat.Command) (bool, error) {
	return true, nil
}
