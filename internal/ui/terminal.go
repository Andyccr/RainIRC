package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/Andyccr/RainIRC/internal/node"
	"github.com/Andyccr/RainIRC/internal/version"
)

// Terminal is a simple line-oriented chat UI. Incoming events are printed
// without a full-screen TUI so the program stays reliable over SSH and pipes.
type Terminal struct {
	node *node.Node
	in   io.Reader
	out  io.Writer
	mu   sync.Mutex
}

func New(n *node.Node) *Terminal {
	return &Terminal{node: n, in: os.Stdin, out: os.Stdout}
}

func (t *Terminal) printf(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprintf(t.out, format, args...)
}

func (t *Terminal) Run(ctx context.Context) error {
	n := t.node
	id := n.Ident()
	t.printf("%s  peer=%s  nick=%s  port=%d  tls=%s\n", version.String(), id.ShortID(), n.Nick(), n.Port(), tlsLabel(n.TLS()))
	t.printf("Listening on %s\n", n.ListenAddr())
	t.printf("Data dir: %s\n", n.DataDir())
	if hint := n.LANHint(); hint != "" {
		t.printf("LAN address: %s   (other peers: /connect %s)\n", hint, hint)
	}
	if n.AutoConnect() {
		t.printf("LAN mesh: auto-connect verified neighbors")
		if n.Reconnect() {
			t.printf("; reconnect known peers")
		}
		t.printf(".\n")
	}
	t.printf("Type /help for commands. Chat is sent to the current channel.\n")
	t.printf("------------------------------------------------\n")
	t.printf("* joined #general\n")
	t.printPrompt()

	events := n.Subscribe()
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(t.in)
		sc.Buffer(make([]byte, 0, 4096), 64*1024)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			t.printEvent(ev)
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			quit, err := n.HandleLine(line)
			if err != nil {
				t.printf("\r! %s\n", err.Error())
				t.printPrompt()
				continue
			}
			if quit {
				return nil
			}
			t.printPrompt()
		}
	}
}

func (t *Terminal) printEvent(ev node.Event) {
	switch ev.Kind {
	case node.EventChat:
		if ev.Msg != nil && ev.Msg.Channel != "" {
			t.printf("\r%s %s\n", ev.Msg.Channel, ev.Text)
		} else {
			t.printf("\r%s\n", ev.Text)
		}
	case node.EventSystem, node.EventJoin, node.EventLeave:
		if ev.Text != "" {
			t.printf("\r* %s\n", ev.Text)
		}
	}
	t.printPrompt()
}

func (t *Terminal) printPrompt() {
	cur := t.node.Channels().Current()
	if cur == "" {
		cur = "(none)"
	}
	t.printf("%s> ", cur)
}

func tlsLabel(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
