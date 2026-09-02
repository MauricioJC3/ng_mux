package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/inre/tmux2/internal/protocol"
)

// execCommand parses and runs one free-form command line, the shared path for
// key bindings, the ':' prompt and the `tmux2 <command>` CLI. cl is the issuing
// client, or nil for a one-shot CLI connection. It looks the command up in the
// registry, resolves the -t target, runs the handler, and forces a repaint. It
// returns text to show the caller (often empty).
func (s *Server) execCommand(cl *client, line string) (string, error) {
	args := tokenize(line)
	if len(args) == 0 {
		return "", nil
	}
	cmd := registry[args[0]]
	if cmd == nil {
		return "", fmt.Errorf("unknown command %q", args[0])
	}

	targetSpec, rest := takeFlagValue(args[1:], "-t")
	sess, paneIdx := s.resolveTarget(cl, targetSpec)
	if sess == nil {
		return "", fmt.Errorf("no such target %q", targetSpec)
	}
	if cmd.needsClient && cl == nil {
		return "", fmt.Errorf("%s needs an attached client", cmd.name)
	}

	out, err := cmd.run(&cmdCtx{srv: s, cl: cl, sess: sess, pane: paneIdx, args: rest})

	s.markDirty(sess)
	if cl != nil {
		cl.reset()
	} else {
		s.resetAllClients()
	}
	return out, err
}

// commandLine renders a legacy TypeCommand (a fixed name plus an optional int
// argument) as a registry command line. It lets an older client keep working
// against a newer server; current clients send TypeExec directly.
func commandLine(name string, n int) string {
	switch name {
	case protocol.CmdSplitHorizontal:
		return "split-window -h"
	case protocol.CmdSplitVertical:
		return "split-window -v"
	case protocol.CmdFocusNext:
		return "select-pane"
	case protocol.CmdFocusPrev:
		return "previous-pane"
	case protocol.CmdKillPane:
		return "kill-pane"
	case protocol.CmdResizeLeft:
		return "resize-pane -L"
	case protocol.CmdResizeRight:
		return "resize-pane -R"
	case protocol.CmdResizeUp:
		return "resize-pane -U"
	case protocol.CmdResizeDown:
		return "resize-pane -D"
	case protocol.CmdCopyMode:
		return "copy-mode"
	case protocol.CmdPaste:
		return "paste-buffer"
	case protocol.CmdNewWindow:
		return "new-window"
	case protocol.CmdNextWindow:
		return "next-window"
	case protocol.CmdPrevWindow:
		return "previous-window"
	case protocol.CmdSelectWindow:
		return "select-window " + strconv.Itoa(n)
	case protocol.CmdKillWindow:
		return "kill-window"
	case protocol.CmdDetach:
		return "detach-client"
	case protocol.CmdNextSession:
		return "next-session"
	case protocol.CmdPrevSession:
		return "previous-session"
	default:
		return ""
	}
}

// resolveTarget maps a "-t" spec to a session and, for send-keys/select-pane,
// a pane index (-1 == active). Spec forms: "", "sess", "sess:win", "sess.pane",
// "sess:win.pane". A ":win" part selects that window as a side effect.
func (s *Server) resolveTarget(cl *client, spec string) (*session, int) {
	paneIdx := -1
	sessName := ""
	if spec != "" {
		rest := spec
		if dot := strings.LastIndexByte(rest, '.'); dot >= 0 {
			if v, err := strconv.Atoi(rest[dot+1:]); err == nil {
				paneIdx = v
				rest = rest[:dot]
			}
		}
		winIdx := -1
		if colon := strings.IndexByte(rest, ':'); colon >= 0 {
			if v, err := strconv.Atoi(rest[colon+1:]); err == nil {
				winIdx = v
			}
			rest = rest[:colon]
		}
		sessName = rest
		defer func() {
			if winIdx >= 0 {
				if sess := s.sessionByName(sessName); sess != nil {
					sess.selectWindow(winIdx)
				}
			}
		}()
	}

	if sessName != "" {
		return s.sessionByName(sessName), paneIdx
	}
	if cl != nil {
		return s.sessionByName(cl.session()), paneIdx
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.order) > 0 {
		return s.sessions[s.order[0]], paneIdx
	}
	return nil, paneIdx
}

// renameSession changes a session's name and its entries in the server maps.
func (s *Server) renameSession(sess *session, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("rename-session needs a name")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, taken := s.sessions[newName]; taken {
		return fmt.Errorf("session %q already exists", newName)
	}
	old := sess.name
	delete(s.sessions, old)
	s.sessions[newName] = sess
	for i, n := range s.order {
		if n == old {
			s.order[i] = newName
		}
	}
	sess.mu.Lock()
	sess.name = newName
	sess.mu.Unlock()
	for c := range s.clients {
		if c.session() == old {
			c.setSession(newName)
		}
	}
	return nil
}

func (s *Server) listSessionsText() string {
	var b strings.Builder
	for _, si := range s.listSessions() {
		mark := ""
		if si.Attached {
			mark = " (attached)"
		}
		fmt.Fprintf(&b, "%s: %d windows, %d panes%s\n", si.Name, si.Windows, si.Panes, mark)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (sess *session) listWindowsText() string {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	var b strings.Builder
	for i, w := range sess.windows {
		mark := ""
		if i == sess.cur {
			mark = " (active)"
		}
		fmt.Fprintf(&b, "%d: %s  %d panes%s\n", i, w.name, len(w.panes), mark)
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- parsing helpers ---

// tokenize splits a command line on whitespace, honouring "double" and
// 'single' quotes.
func tokenize(line string) []string {
	var out []string
	var cur strings.Builder
	inWord := false
	quote := byte(0)
	flush := func() {
		if inWord {
			out = append(out, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
			inWord = true
		case c == '"' || c == '\'':
			quote = c
			inWord = true
		case c == ' ' || c == '\t':
			flush()
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	flush()
	return out
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// takeFlagValue removes "flag value" from args and returns the value.
func takeFlagValue(args []string, flag string) (string, []string) {
	for i := 0; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) {
			return args[i+1], append(append([]string(nil), args[:i]...), args[i+2:]...)
		}
	}
	return "", args
}

func firstInt(args []string) (int, bool) {
	for _, a := range args {
		if v, err := strconv.Atoi(a); err == nil {
			return v, true
		}
	}
	return 0, false
}

// encodeKeys turns send-keys arguments into a byte stream. Each argument is a
// key name (Enter, Space, Tab, Escape, BSpace, arrows, C-x) or, failing that,
// a literal string.
func encodeKeys(args []string) []byte {
	var b []byte
	for _, a := range args {
		if k, ok := namedKey(a); ok {
			b = append(b, k...)
			continue
		}
		b = append(b, a...)
	}
	return b
}

func namedKey(s string) ([]byte, bool) {
	switch s {
	case "Enter", "C-m", "Return":
		return []byte{'\r'}, true
	case "Space":
		return []byte{' '}, true
	case "Tab", "C-i":
		return []byte{'\t'}, true
	case "Escape", "Esc":
		return []byte{0x1b}, true
	case "BSpace", "BackSpace", "DC":
		return []byte{0x7f}, true
	case "Up":
		return []byte("\x1b[A"), true
	case "Down":
		return []byte("\x1b[B"), true
	case "Right":
		return []byte("\x1b[C"), true
	case "Left":
		return []byte("\x1b[D"), true
	case "Home":
		return []byte("\x1b[H"), true
	case "End":
		return []byte("\x1b[F"), true
	case "PageUp", "PPage":
		return []byte("\x1b[5~"), true
	case "PageDown", "NPage":
		return []byte("\x1b[6~"), true
	}
	if len(s) == 3 && (s[0] == 'C' || s[0] == 'c') && s[1] == '-' {
		c := s[2]
		switch {
		case c >= 'a' && c <= 'z':
			return []byte{c - 'a' + 1}, true
		case c >= 'A' && c <= 'Z':
			return []byte{c - 'A' + 1}, true
		}
	}
	return nil, false
}
