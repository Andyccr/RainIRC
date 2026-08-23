package chat

import "testing"

func TestParseCommands(t *testing.T) {
	cases := []struct {
		in   string
		kind Kind
	}{
		{"hello", KindChat},
		{"/help", KindHelp},
		{"/connect 127.0.0.1:7777", KindConnect},
		{"/quit", KindQuit},
		{"/nope", KindUnknown},
		{"/me waves", KindMe},
		{"/whoami", KindWhoami},
		{"/id", KindWhoami},
		{"/alias", KindAlias},
		{"/known", KindKnown},
		{"/discover connect", KindDiscover},
		{"/addr", KindAddr},
		{"/version", KindVersion},
		{"/stats", KindStats},
		{"/names", KindNames},
		{"/who", KindNames},
	}
	for _, c := range cases {
		got := Parse(c.in)
		if got.Kind != c.kind {
			t.Errorf("Parse(%q) kind=%d want %d", c.in, got.Kind, c.kind)
		}
	}
	msg := Parse("/msg abcd hello there")
	if msg.Kind != KindMsg || len(msg.Args) != 2 || msg.Args[1] != "hello there" {
		t.Fatalf("msg args=%v", msg.Args)
	}
}
