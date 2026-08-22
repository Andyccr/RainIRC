package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/Andyccr/RainIRC/internal/node"
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
	t.printf("P2PIRC  peer=%s  nick=%s  port=%d\n", id.ShortID(), n.Nick(), n.Port())
	t.printf("Listening on %s\n", n.ListenAddr())
	if hint := lanAddress(n.Port()); hint != "" {
		t.printf("LAN address: %s   (other peers: /connect %s)\n", hint, hint)
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

func lanAddress(port int) string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.IsLoopback() {
			continue
		}
		ip := ipn.IP.To4()
		if ip == nil {
			continue
		}
		return net.JoinHostPort(ip.String(), strconv.Itoa(port))
	}
	return ""
}
