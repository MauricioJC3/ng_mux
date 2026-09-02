package vterm

import (
	"bytes"
	"testing"
)

func TestAnswersDeviceAttributeQueries(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		reply string
	}{
		{"primary DA", "\x1b[c", "\x1b[?1;2c"},
		{"primary DA explicit 0", "\x1b[0c", "\x1b[?1;2c"},
		{"secondary DA", "\x1b[>c", "\x1b[>84;0;0c"},
		{"secondary DA explicit 0", "\x1b[>0c", "\x1b[>84;0;0c"},
		{"query embedded in output", "done\x1b[cmore", "\x1b[?1;2c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reply bytes.Buffer
			term := New(20, 5, &reply)
			if _, err := term.Write([]byte(tc.in)); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if got := reply.String(); got != tc.reply {
				t.Fatalf("reply = %q, want %q", got, tc.reply)
			}
		})
	}
}

func TestPlainOutputProducesNoReply(t *testing.T) {
	var reply bytes.Buffer
	term := New(20, 5, &reply)
	if _, err := term.Write([]byte("hello world\r\nsecond line\r\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if reply.Len() != 0 {
		t.Fatalf("unexpected reply %q for plain output", reply.String())
	}
}

func TestDeviceQueryReplyDoesNotConsumeInput(t *testing.T) {
	var reply bytes.Buffer
	term := New(20, 5, &reply)
	in := []byte("AB\x1b[cCD")
	n, err := term.Write(in)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(in) {
		t.Fatalf("Write returned n=%d, want %d (query must not shorten the write)", n, len(in))
	}
}
