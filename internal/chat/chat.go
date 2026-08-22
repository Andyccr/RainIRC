package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/Andyccr/RainIRC/internal/protocol"
)

type Kind int

const (
	KindChat Kind = iota
	KindHelp
	KindConnect
	KindDisconnect
	KindPeers
	KindJoin
	KindLeave
	KindChannels
	KindNick
	KindMe
	KindMsg
	KindDiscover
	KindQuit
	KindUnknown
)

type Command struct {
	Kind Kind
	Args []string
	Raw  string
}

func Parse(line string) Command {
	line = strings.TrimRight(line, "\r\n")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return Command{Kind: KindChat, Raw: ""}
	}
	if !strings.HasPrefix(trimmed, "/") {
		return Command{Kind: KindChat, Args: []string{line}, Raw: line}
	}
	parts := strings.Fields(trimmed)
	name := strings.ToLower(parts[0])
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, parts[0]))
	cmd := Command{Raw: trimmed, Args: parts[1:]}
	switch name {
	case "/help":
		cmd.Kind = KindHelp
	case "/connect":
		cmd.Kind = KindConnect
	case "/disconnect":
		cmd.Kind = KindDisconnect
	case "/peers":
		cmd.Kind = KindPeers
	case "/join":
		cmd.Kind = KindJoin
	case "/leave":
		cmd.Kind = KindLeave
	case "/channels":
		cmd.Kind = KindChannels
	case "/nick":
		cmd.Kind = KindNick
		if rest != "" {
			cmd.Args = []string{rest}
		}
	case "/me":
		cmd.Kind = KindMe
		cmd.Args = []string{rest}
	case "/msg":
		cmd.Kind = KindMsg
		if len(parts) >= 3 {
			cmd.Args = []string{parts[1], strings.TrimSpace(strings.TrimPrefix(rest, parts[1]))}
		}
	case "/discover":
		cmd.Kind = KindDiscover
	case "/quit", "/exit":
		cmd.Kind = KindQuit
	default:
		cmd.Kind = KindUnknown
	}
	return cmd
}

func HelpText() string {
	return strings.TrimSpace(`
Commands:
  /help                      Show this help
  /connect <host:port>       Connect to a peer
  /disconnect <peer-id>      Disconnect a peer
  /peers                     List connected peers
  /join <#channel>           Join a channel
  /leave <#channel>          Leave a channel
  /channels                  List channels
  /nick <name>               Set nickname (cosmetic only)
  /me <action>               Send an action to the current channel
  /msg <peer-id> <text>      Direct message a connected peer
  /discover                  Show nearby LAN peers (UDP multicast)
  /quit                      Exit
`)
}

func FormatChat(msg *protocol.Message) string {
	nick := msg.Nickname
	if nick == "" {
		nick = short(msg.Sender)
	}
	sid := short(msg.Sender)
	if msg.Action {
		return fmt.Sprintf("* %s %s", nick, msg.Text)
	}
	if msg.To != "" {
		return fmt.Sprintf("[dm %s] %s: %s", sid, nick, msg.Text)
	}
	return fmt.Sprintf("[%s] %s: %s", sid, nick, msg.Text)
}

func FormatTime(ts int64) string {
	if ts <= 0 {
		return time.Now().Format("15:04:05")
	}
	return time.Unix(ts, 0).Format("15:04:05")
}

func short(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}
