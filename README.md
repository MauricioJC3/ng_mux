# tmux2

A small, cross-platform terminal multiplexer written in pure Go. Runs on Linux,
macOS and Windows 10+ from the same codebase.

Like tmux, it is **client/server**: a background daemon owns every session,
pane, pseudo-terminal and terminal emulator; thin client processes attach to it,
forward keystrokes, and paint the ANSI frames the daemon streams back. Detaching
just kills the client — the daemon and your shells keep running.

## Status: Phase 4

Working now:

- background daemon, auto-spawned on first `tmux2`
- **multiple sessions**, each with **multiple windows**, each a tree of panes
- split a pane left/right or top/bottom; **resize panes** with the keyboard
- move focus between panes; cycle / select / kill windows
- switch between sessions while attached
- named sessions: `tmux2 new -s work`, `tmux2 attach -t work`
- **scrollback + copy-mode**: scroll history, select text, yank to a paste
  buffer, paste it back
- **config file** (`~/.config/tmux2/tmux2.conf`): custom prefix key, key
  bindings, history limit, default shell, status-bar colours
- **command layer**: `tmux2 <command> [args]` scripts the running server
  (`new-window`, `split-window -h`, `send-keys`, `select-layout`, `rename-*`, …),
  and `Ctrl-b :` opens the same commands as an in-terminal prompt
- **preset layouts**: `select-layout even-horizontal | even-vertical | tiled |
  main-vertical | main-horizontal`
- **rename** windows and sessions
- **mouse** (on by default; `set mouse off` to disable): click a pane to focus
  it, drag a border to resize, wheel to scroll history, click a window name or
  the `[+]` button in the status bar
- status bar: session name, window list with the active marker, clock
- kill a pane; detach and re-attach
- `tmux2 ls`, `tmux2 kill-session -t name`, `tmux2 kill-server`
- cross-platform PTY (ConPTY on Windows) and IPC (Unix socket / named pipe)

Deliberately out of scope: hooks, session groups, `choose-tree`,
system-clipboard integration.

### Scrollback caveat

vt10x keeps no history of its own, so tmux2 reconstructs scrollback by feeding
the emulator line by line and detecting when the screen scrolls. This is
reliable for shell output; capture is skipped entirely while a full-screen app
(vim, less, htop) holds the alternate screen, matching tmux.

## Install

**Linux / macOS** — one line, no toolchain needed:

```sh
curl -fsSL https://raw.githubusercontent.com/MauricioJC3/ng_mux/main/install.sh | sh
```

It drops the `tmux2` binary in `~/.local/bin` (override with `TMUX2_INSTALL_DIR`)
and adds that directory to your shell's `PATH` if it is missing.

**Windows** (PowerShell):

```powershell
irm https://raw.githubusercontent.com/MauricioJC3/ng_mux/main/install.ps1 | iex
```

**Update** to the latest release at any time:

```sh
tmux2 update
```

`tmux2 version` prints the installed version.

### Build from source

```sh
go install github.com/MauricioJC3/ng_mux/cmd/tmux2@latest   # needs Go 1.27+
# or, from a clone:
go build -o tmux2 ./cmd/tmux2       # native
GOOS=windows go build ./cmd/tmux2   # Windows binary
```

## Use

```
tmux2 [new] [-s name]      start the server if needed, then attach
tmux2 attach | a [-t name] attach to a running server
tmux2 ls                   list sessions
tmux2 kill-session -t name kill one session
tmux2 kill-server          stop the server
tmux2 update               install the latest release
tmux2 version              print the installed version
tmux2 <command> [args]     run a command on the running server:
  tmux2 new-window
  tmux2 split-window -h
  tmux2 send-keys -t work "make test" Enter
  tmux2 select-layout tiled
  tmux2 rename-window build
  tmux2 rename-session prod
  tmux2 list-windows
```

Targets: `-t <session>`, `-t <session>:<window>`, `-t <session>:<window>.<pane>`
(pane and window are indices). `send-keys` arguments are key names (`Enter`,
`Space`, `C-c`, `Up`, …) or literal strings.

### Key bindings

Prefix is **Ctrl-b** (same as tmux).

| Keys            | Action                     |
|-----------------|----------------------------|
| `Ctrl-b "`      | split top / bottom         |
| `Ctrl-b %`      | split left / right         |
| `Ctrl-b o`      | focus next pane            |
| `Ctrl-b ;`      | focus previous pane        |
| `Ctrl-b →/↓/←/↑`| focus next / previous pane |
| `Ctrl-b H/J/K/L`| resize pane left/down/up/right |
| `Ctrl-b x`      | kill focused pane          |
| `Ctrl-b c`      | new window                 |
| `Ctrl-b n` / `p`| next / previous window     |
| `Ctrl-b 0`–`9`  | select window by index     |
| `Ctrl-b &`      | kill window                |
| `Ctrl-b (` / `)`| previous / next session    |
| `Ctrl-b [`      | enter copy-mode (scrollback) |
| `Ctrl-b ]`      | paste the paste buffer     |
| `Ctrl-b :`      | command prompt             |
| `Ctrl-b d`      | detach                     |
| `Ctrl-b Ctrl-b` | send a literal Ctrl-b      |

In copy-mode: arrows / PageUp / PageDown move and scroll, `g` / `G` jump to the
oldest / newest line, `Space` starts a selection, `Enter` or `y` copies it and
exits, `Esc` or `q` exits.

### Config file

`~/.config/tmux2/tmux2.conf` (or `%APPDATA%\tmux2\tmux2.conf`; override with
`TMUX2_CONFIG`). One directive per line:

```
set prefix C-a
set history-limit 5000
set default-shell /bin/bash
set status-fg 0
set status-bg 4
bind s split-vertical
bind v split-horizontal
```

Unknown lines are logged and skipped, never fatal.

## Architecture

```
cmd/tmux2            CLI + daemon spawn (build-tagged per OS)
internal/
  protocol          length-prefixed JSON framing, client<->server messages
  ipc               transport: Unix domain socket / Windows named pipe
  config            tmux2.conf parser (prefix, binds, limits, colours)
  ptyx              pseudo-terminal wrapper (aymanbagabas/go-pty)
  vterm             VT100/xterm emulator wrapper (hinshun/vt10x) + scrollback
  layout            binary split tree -> pane rectangles; split / remove / resize
  render            composite panes + borders + status bar, ANSI frame diffing
  termio            client's real terminal: raw mode, size, resize events
  server            the daemon: sessions -> windows -> panes, broadcast loop
    session.go        one named session: ordered windows, current window, status
    window.go         one window: the pane split tree and focus
    copymode.go       per-pane scrollback / selection state machine
    command.go        free-form command line parser + dispatcher
    mouse.go          click-to-focus, border drag-resize, wheel-scroll
  client            the thin half: connect, raw mode, forward input, blit frames
                     (also parses mouse sequences and the ':' prompt)
  e2e               daemon + client driven through a real pty
```

## Test

```sh
go test ./...
```

`internal/e2e` starts an in-process daemon, attaches a client over a real pty,
and checks that shell output reaches the screen, that splitting adds a pane, and
that detach stops the client.
