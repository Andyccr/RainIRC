package node

import (
	"testing"

	"github.com/Andyccr/RainIRC/internal/chat"
)

func TestCommandsCoverAllKinds(t *testing.T) {
	for k := chat.KindChat; k < chat.KindUnknown; k++ {
		if _, ok := commands[k]; !ok {
			t.Errorf("no handler registered for kind %d", k)
		}
	}
}
