package client

import (
	"bufio"
	"strings"
	"testing"
)

func TestResolveKeyDefaults(t *testing.T) {
	km := keymap{prefix: DefaultPrefix, binds: map[string]string{}}
	cases := []struct {
		key  byte
		want string
	}{
		{'"', "split-window -v"},
		{'%', "split-window -h"},
		{'o', "select-pane"},
		{';', "previous-pane"},
		{'x', "kill-pane"},
		{'c', "new-window"},
		{'n', "next-window"},
		{'p', "previous-window"},
		{'&', "kill-window"},
		{'[', "copy-mode"},
		{']', "paste-buffer"},
		{'z', "resize-pane -Z"},
		{'H', "resize-pane -L"},
		{'L', "resize-pane -R"},
		{'d', "detach-client"},
		{'0', "select-window 0"},
		{'7', "select-window 7"},
	}
	for _, tc := range cases {
		t.Run(string(rune(tc.key)), func(t *testing.T) {
			got, ok := km.resolveKey(tc.key)
			if !ok || got != tc.want {
				t.Fatalf("resolveKey(%q) = %q,%v; want %q,true", tc.key, got, ok, tc.want)
			}
		})
	}
}

func TestResolveKeyUnbound(t *testing.T) {
	km := keymap{prefix: DefaultPrefix, binds: map[string]string{}}
	if got, ok := km.resolveKey('Z'); ok {
		t.Fatalf("resolveKey('Z') = %q,true; want unbound", got)
	}
}

func TestResolveKeyConfigBindOverridesDefault(t *testing.T) {
	km := keymap{prefix: DefaultPrefix, binds: map[string]string{
		"x": "detach",              // legacy short name -> translated
		"S": "split-window -v",     // full command line -> passed through
		"%": "select-layout tiled", // overrides the built-in split
	}}
	if got, _ := km.resolveKey('x'); got != "detach-client" {
		t.Errorf("legacy bind 'x'=detach resolved to %q, want detach-client", got)
	}
	if got, _ := km.resolveKey('S'); got != "split-window -v" {
		t.Errorf("passthrough bind 'S' resolved to %q", got)
	}
	if got, _ := km.resolveKey('%'); got != "select-layout tiled" {
		t.Errorf("override bind '%%' resolved to %q, want the override", got)
	}
}

func TestArrowCommand(t *testing.T) {
	cases := map[string]string{
		"[A": "previous-pane",
		"[D": "previous-pane",
		"[B": "select-pane",
		"[C": "select-pane",
	}
	for body, want := range cases {
		t.Run(body, func(t *testing.T) {
			br := bufio.NewReader(strings.NewReader(body))
			got, ok := arrowCommand(br)
			if !ok || got != want {
				t.Fatalf("arrowCommand(%q) = %q,%v; want %q,true", body, got, ok, want)
			}
		})
	}
}

func TestArrowCommandRejectsNonArrow(t *testing.T) {
	br := bufio.NewReader(strings.NewReader("[Z"))
	if got, ok := arrowCommand(br); ok {
		t.Fatalf("arrowCommand(\"[Z\") = %q,true; want false", got)
	}
}
