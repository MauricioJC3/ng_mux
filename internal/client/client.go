// Package client is the thin half of ngmux. It connects to the daemon, puts
// the real terminal into raw mode, forwards keystrokes (intercepting the
// prefix key for multiplexer commands), and writes the ANSI frames the server
// sends straight to stdout.
package client

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"

	"github.com/MauricioJC3/ng_mux/internal/config"
	"github.com/MauricioJC3/ng_mux/internal/ipc"
	"github.com/MauricioJC3/ng_mux/internal/protocol"
	"github.com/MauricioJC3/ng_mux/internal/termio"
)

// lockedWriter serializes writes to the terminal so the frame stream and the
// local command prompt never interleave mid-sequence.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (l *lockedWriter) WriteString(s string) { _, _ = l.Write([]byte(s)) }

// mouseOn / mouseOff toggle SGR mouse reporting (button + drag + wheel).
const (
	mouseOn  = "\x1b[?1000h\x1b[?1002h\x1b[?1006h"
	mouseOff = "\x1b[?1006l\x1b[?1002l\x1b[?1000l"
)

// DefaultPrefix is Ctrl-b, matching tmux. The config file can override it.
const DefaultPrefix = config.DefaultPrefix

// keymap is the resolved prefix key plus any user bindings from the config.
type keymap struct {
	prefix byte
	binds  map[string]string // single-rune key -> command name
}

// enterAlt / exitAlt switch the host terminal to its alternate screen so the
// user's shell scrollback is untouched while attached.
const (
	enterAlt = "\x1b[?1049h"
	exitAlt  = "\x1b[?1049l\x1b[0m"
)

// Attach connects to the daemon at ep, attaches to the named session (empty
// means the server's default), and runs until the session detaches, the server
// exits, or stdin closes.
func Attach(ep ipc.Endpoint, session string, in, out *os.File) error {
	conn, err := ipc.Dial(ep)
	if err != nil {
		return err
	}
	pc := protocol.NewConn(conn)

	size, err := termio.GetSize(out)
	if err != nil || size.Cols == 0 {
		size = termio.Size{Cols: 80, Rows: 24}
	}
	if err := pc.Write(protocol.Message{
		Type: protocol.TypeAttach,
		Name: session,
		Cols: size.Cols,
		Rows: size.Rows,
	}); err != nil {
		return err
	}

	cfg, _ := config.Load()
	km := keymap{prefix: cfg.Prefix, binds: cfg.Binds}
	if km.prefix == 0 {
		km.prefix = DefaultPrefix
	}

	sess, err := termio.Enter(in, out)
	if err != nil {
		return err
	}
	defer sess.Restore()

	w := &lockedWriter{w: out}
	w.WriteString(enterAlt)
	defer w.WriteString(exitAlt)
	if cfg.Mouse {
		w.WriteString(mouseOn)
		defer w.WriteString(mouseOff)
	}

	stopResize := make(chan struct{})
	defer close(stopResize)
	go termio.WatchResize(out, func(s termio.Size) {
		pc.Write(protocol.Message{Type: protocol.TypeResize, Cols: s.Cols, Rows: s.Rows})
	}, stopResize)

	// Input goroutine: stdin -> server. Ends when stdin errors.
	inputErr := make(chan error, 1)
	go func() { inputErr <- forwardInput(bufio.NewReaderSize(in, 4096), w, out, pc, km) }()

	// Main goroutine: server -> stdout, until Bye or disconnect.
	readErr := readFrames(pc, w)

	select {
	case <-inputErr:
	default:
	}
	if errors.Is(readErr, errDetached) || errors.Is(readErr, io.EOF) {
		return nil
	}
	return readErr
}

var errDetached = errors.New("detached")

// Exec sends a one-shot command line to the daemon and prints its reply. It
// does not attach. Used by the `ngmux <command>` CLI form.
func Exec(ep ipc.Endpoint, line string) error {
	conn, err := ipc.Dial(ep)
	if err != nil {
		return err
	}
	defer conn.Close()
	pc := protocol.NewConn(conn)
	if err := pc.Write(protocol.Message{Type: protocol.TypeExec, Name: line}); err != nil {
		return err
	}
	reply, err := pc.Read()
	if err != nil {
		return err
	}
	switch reply.Type {
	case protocol.TypeExecReply:
		if reply.Name != "" {
			io.WriteString(os.Stdout, reply.Name+"\n")
		}
		return nil
	case protocol.TypeError:
		return errors.New(reply.Name)
	default:
		return nil
	}
}

// readFrames pumps server messages to the terminal until the session ends.
func readFrames(pc *protocol.Conn, out io.Writer) error {
	for {
		msg, err := pc.Read()
		if err != nil {
			return err
		}
		switch msg.Type {
		case protocol.TypeFrame:
			if _, err := out.Write(msg.Data); err != nil {
				return err
			}
		case protocol.TypeExecReply:
			if msg.Name != "" {
				out.Write([]byte("\x1b[999;1H\x1b[2K" + firstLine(msg.Name)))
			}
		case protocol.TypeBye:
			return errDetached
		case protocol.TypeError:
			return errors.New("server: " + msg.Name)
		}
	}
}

func firstLine(s string) string {
	if i := bytes.IndexByte([]byte(s), '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// forwardInput reads raw keystrokes and routes them: escape sequences (arrows,
// mouse) via handleEscape, the prefix key into command handling (including the
// ':' command prompt), everything else straight to the focused pane. term is
// the real terminal, used only to size the prefix cheat-sheet popup.
func forwardInput(br *bufio.Reader, out *lockedWriter, term *os.File, pc *protocol.Conn, km keymap) error {
	prefix := km.prefix
	for {
		b, err := br.ReadByte()
		if err != nil {
			return err
		}

		switch {
		case b == 0x1b:
			if err := handleEscape(br, pc); err != nil {
				return err
			}

		case b == prefix:
			// Show the cheat-sheet while we block for the next key, unless the
			// user already typed it (a buffered byte) — then skip the flash.
			hide, shown := func() {}, false
			if br.Buffered() == 0 {
				hide, shown = showWhichKey(out, term, km), true
			}
			cmd, err := br.ReadByte()
			if err != nil {
				hide()
				return err
			}
			hide()
			// Repaint over where the panel was. The ':' prompt draws itself and
			// owns the bottom row, so let it refresh on its own instead.
			if shown && cmd != ':' {
				pc.Write(protocol.Message{Type: protocol.TypeRefresh})
			}
			switch {
			case cmd == ':':
				commandPrompt(br, out, pc)
			case cmd == prefix:
				// prefix twice: send one literal prefix byte to the pane.
				pc.Write(protocol.Message{Type: protocol.TypeInput, Data: []byte{prefix}})
			case cmd == 0x1b:
				if line, ok := arrowCommand(br); ok {
					pc.Write(protocol.Message{Type: protocol.TypeExec, Name: line})
				}
			default:
				line, ok := km.resolveKey(cmd)
				if !ok {
					continue
				}
				pc.Write(protocol.Message{Type: protocol.TypeExec, Name: line})
				if line == cmdDetach {
					return errDetached
				}
			}

		default:
			buf := []byte{b}
			for br.Buffered() > 0 {
				nb, e := br.ReadByte()
				if e != nil {
					break
				}
				if nb == prefix || nb == 0x1b {
					_ = br.UnreadByte()
					break
				}
				buf = append(buf, nb)
			}
			if err := pc.Write(protocol.Message{Type: protocol.TypeInput, Data: buf}); err != nil {
				return err
			}
		}
	}
}

// handleEscape consumes an escape sequence (0x1b already read). SGR mouse
// sequences become TypeMouse; anything else is forwarded verbatim as input.
func handleEscape(br *bufio.Reader, pc *protocol.Conn) error {
	b1, err := br.ReadByte()
	if err != nil {
		return pc.Write(protocol.Message{Type: protocol.TypeInput, Data: []byte{0x1b}})
	}
	if b1 != '[' {
		return pc.Write(protocol.Message{Type: protocol.TypeInput, Data: []byte{0x1b, b1}})
	}
	b2, err := br.ReadByte()
	if err != nil {
		return pc.Write(protocol.Message{Type: protocol.TypeInput, Data: []byte{0x1b, '['}})
	}
	if b2 == '<' { // SGR mouse: ESC [ < Cb ; Cx ; Cy (M|m)
		var body []byte
		for {
			c, e := br.ReadByte()
			if e != nil {
				return nil
			}
			if c == 'M' || c == 'm' {
				body = append(body, c)
				break
			}
			body = append(body, c)
		}
		if msg, ok := parseSGRMouse(body); ok {
			return pc.Write(msg)
		}
		return nil
	}
	// Any other CSI: forward ESC [ b2 … up to and including the final byte.
	seq := []byte{0x1b, '[', b2}
	for !isCSIFinal(b2) {
		c, e := br.ReadByte()
		if e != nil {
			break
		}
		seq = append(seq, c)
		b2 = c
	}
	return pc.Write(protocol.Message{Type: protocol.TypeInput, Data: seq})
}

func isCSIFinal(b byte) bool { return b >= 0x40 && b <= 0x7e }

// parseSGRMouse decodes the "Cb;Cx;Cy(M|m)" body of an SGR mouse report.
func parseSGRMouse(body []byte) (protocol.Message, bool) {
	if len(body) < 2 {
		return protocol.Message{}, false
	}
	final := body[len(body)-1]
	fields := bytes.Split(body[:len(body)-1], []byte{';'})
	if len(fields) != 3 {
		return protocol.Message{}, false
	}
	cb, err1 := strconv.Atoi(string(fields[0]))
	cx, err2 := strconv.Atoi(string(fields[1]))
	cy, err3 := strconv.Atoi(string(fields[2]))
	if err1 != nil || err2 != nil || err3 != nil {
		return protocol.Message{}, false
	}
	var kind string
	switch {
	case cb&64 != 0:
		if cb&1 != 0 {
			kind = protocol.MouseWheelDown
		} else {
			kind = protocol.MouseWheelUp
		}
	case cb&32 != 0:
		kind = protocol.MouseDrag
	case final == 'm':
		kind = protocol.MouseRelease
	default:
		kind = protocol.MousePress
	}
	return protocol.Message{
		Type: protocol.TypeMouse, Name: kind,
		MX: cx - 1, MY: cy - 1, MB: cb & 3,
	}, true
}

// commandPrompt runs the ':' line editor locally, drawing on the bottom row.
func commandPrompt(br *bufio.Reader, out *lockedWriter, pc *protocol.Conn) {
	var buf []byte
	draw := func() { out.WriteString("\x1b[?25h\x1b[999;1H\x1b[2K:" + string(buf)) }
	draw()
	for {
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		switch b {
		case '\r', '\n':
			if len(buf) > 0 {
				pc.Write(protocol.Message{Type: protocol.TypeExec, Name: string(buf)})
			}
			pc.Write(protocol.Message{Type: protocol.TypeRefresh})
			return
		case 0x1b: // Esc: cancel (swallow a following arrow-key body if any)
			for br.Buffered() > 0 {
				if nb, _ := br.ReadByte(); isCSIFinal(nb) {
					break
				}
			}
			pc.Write(protocol.Message{Type: protocol.TypeRefresh})
			return
		case 0x7f, 0x08:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
			draw()
		default:
			if b >= 0x20 && b < 0x7f {
				buf = append(buf, b)
				draw()
			}
		}
	}
}

// cmdDetach is the one command line the input loop reacts to locally: after
// sending it the client stops reading input and lets Attach return.
const cmdDetach = "detach-client"

// defaultKeyCommands maps the key pressed after the prefix to the command line
// sent to the server. Digits 0-9 (select-window) and the arrow keys are handled
// separately. Config `bind` directives override entries here.
var defaultKeyCommands = map[byte]string{
	'"': "split-window -v", // top / bottom
	'%': "split-window -h", // left / right
	'o': "select-pane",     // next pane
	';': "previous-pane",
	'x': "kill-pane",
	'c': "new-window",
	'n': "next-window",
	'p': "previous-window",
	'&': "kill-window",
	'(': "previous-session",
	')': "next-session",
	'[': "copy-mode",
	']': "paste-buffer",
	'H': "resize-pane -L",
	'J': "resize-pane -D",
	'K': "resize-pane -U",
	'L': "resize-pane -R",
	'd': cmdDetach,
}

// legacyBindCommands translates the short command names older ngmux.conf files
// use in `bind` directives into current command lines. A bind value that is not
// listed here is passed through unchanged, so a new config can bind a full
// command line directly (e.g. `bind S split-window -v`).
var legacyBindCommands = map[string]string{
	"split-vertical":   "split-window -v",
	"split-horizontal": "split-window -h",
	"focus-next":       "select-pane",
	"focus-prev":       "previous-pane",
	"focus-previous":   "previous-pane",
	"paste":            "paste-buffer",
	"detach":           cmdDetach,
	"prev-window":      "previous-window",
	"prev-session":     "previous-session",
}

// resolveKey maps the byte after the prefix to a server command line. ok is
// false when the key is unbound.
func (km keymap) resolveKey(key byte) (line string, ok bool) {
	if b, bound := km.binds[string(rune(key))]; bound {
		if mapped, isLegacy := legacyBindCommands[b]; isLegacy {
			return mapped, true
		}
		return b, true
	}
	if key >= '0' && key <= '9' {
		return "select-window " + string(rune(key)), true
	}
	if line, has := defaultKeyCommands[key]; has {
		return line, true
	}
	return "", false
}

// arrowCommand reads the "[A|B|C|D" body of an arrow key pressed after the
// prefix (the leading ESC is already consumed) and maps it to a focus move.
func arrowCommand(br *bufio.Reader) (string, bool) {
	if b1, err := br.ReadByte(); err != nil || b1 != '[' {
		return "", false
	}
	b2, err := br.ReadByte()
	if err != nil {
		return "", false
	}
	switch b2 {
	case 'A', 'D':
		return "previous-pane", true
	case 'B', 'C':
		return "select-pane", true
	}
	return "", false
}
