// Command ngmux is a small, cross-platform terminal multiplexer: a background
// daemon owns the sessions, windows and panes, thin client processes attach.
//
// Usage:
//
//	ngmux [new] [-s name]    start the server if needed, then attach
//	ngmux attach|a [-t name] attach to an already-running server
//	ngmux ls                 list sessions
//	ngmux kill-session -t n  kill one session
//	ngmux kill-server        stop the server
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/MauricioJC3/ng_mux/internal/client"
	"github.com/MauricioJC3/ng_mux/internal/ipc"
	"github.com/MauricioJC3/ng_mux/internal/protocol"
	"github.com/MauricioJC3/ng_mux/internal/ptyx"
	"github.com/MauricioJC3/ng_mux/internal/server"
)

// version is the build version. Release builds set it via
// -ldflags "-X main.version=vX.Y.Z"; a plain `go build` leaves it "dev" and
// versionString falls back to the module version recorded by the Go toolchain.
var version = "dev"

func main() {
	args := os.Args[1:]
	cmd := "new"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	ep := ipc.DefaultEndpoint

	var err error
	switch cmd {
	case "new", "":
		err = cmdNew(ep, args)
	case "attach", "a":
		err = cmdAttach(ep, args)
	case "ls", "list-sessions":
		err = cmdList(ep)
	case "kill-session":
		err = cmdKillSession(ep, args)
	case "kill-server":
		err = cmdKillServer(ep)
	case "__server":
		// Internal: this process IS the daemon. Not for direct use.
		name := "default"
		if len(args) > 0 {
			name = args[0]
		}
		err = runDaemon(ipc.Endpoint{Name: name})
	case "help", "-h", "--help":
		usage(os.Stdout)
	case "version", "-v", "--version":
		fmt.Println("ngmux " + versionString())
	case "update", "upgrade":
		err = selfUpdate(os.Stdout, hasFlag(args, "--force"))
	case "run":
		// `ngmux run <command line...>` — explicit form.
		err = client.Exec(ep, strings.Join(args, " "))
	default:
		// Anything else is treated as a command line for the running server,
		// tmux-style: `ngmux new-window`, `ngmux send-keys -t 0 "ls" Enter`.
		err = client.Exec(ep, strings.Join(os.Args[1:], " "))
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "ngmux: %v\n", err)
		os.Exit(1)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `ngmux - cross-platform terminal multiplexer

  ngmux [new] [-s name]      start the server if needed, then attach
  ngmux attach|a [-t name]   attach to a running server
  ngmux ls                   list sessions
  ngmux kill-session -t name kill one session
  ngmux kill-server          stop the server
  ngmux update               download and install the latest release
  ngmux version              print the version
  ngmux <command> [args...]  run a command on the running server, e.g.
                             ngmux new-window
                             ngmux new-session -s logs
                             ngmux send-keys -t 0 "ls -la" Enter
                             ngmux select-layout tiled
                             ngmux rename-window build

sessions:
  ngmux new -s NAME          create a session and attach to it
  ngmux attach -t NAME       re-attach to a running session
  ngmux ls                   list sessions (name, windows, panes)
  Ctrl-b m                   show this session cheat-sheet while attached

prefix key: Ctrl-b
  "  split top/bottom      %  split left/right       x  kill pane
  o  next pane             ;  previous pane          arrows  focus pane
  H/J/K/L  resize pane     d  detach                 :  command prompt
  c  new window            n / p  next / prev window   0-9  select window
  &  kill window           ( / )  prev / next session   m  session help
  [  copy-mode             ]  paste
`)
}

// flags parses a -s/-t "session name" option (both spellings mean the same
// thing) out of args and returns the name (may be empty).
func sessionFlag(args []string) (string, error) {
	fs := flag.NewFlagSet("ngmux", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	s := fs.String("s", "", "session name")
	t := fs.String("t", "", "target session name")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if *s != "" {
		return *s, nil
	}
	return *t, nil
}

// cmdNew attaches, starting the daemon first if nothing is listening.
func cmdNew(ep ipc.Endpoint, args []string) error {
	if err := ptyx.CheckAvailable(); err != nil {
		return err
	}
	name, err := sessionFlag(args)
	if err != nil {
		return err
	}
	if _, err := ipc.Dial(ep); err != nil {
		if err := startDaemon(ep); err != nil {
			return err
		}
	}
	return client.Attach(ep, name, os.Stdin, os.Stdout)
}

func cmdAttach(ep ipc.Endpoint, args []string) error {
	if err := ptyx.CheckAvailable(); err != nil {
		return err
	}
	name, err := sessionFlag(args)
	if err != nil {
		return err
	}
	if _, err := ipc.Dial(ep); err != nil {
		return fmt.Errorf("no server running (use `ngmux new`)")
	}
	return client.Attach(ep, name, os.Stdin, os.Stdout)
}

func cmdList(ep ipc.Endpoint) error {
	conn, err := ipc.Dial(ep)
	if err != nil {
		return fmt.Errorf("no server running")
	}
	pc := protocol.NewConn(conn)
	if err := pc.Write(protocol.Message{Type: protocol.TypeListReq}); err != nil {
		return err
	}
	reply, err := pc.Read()
	if err != nil {
		return err
	}
	if len(reply.Sessions) == 0 {
		fmt.Println("(no sessions)")
		return nil
	}
	for _, s := range reply.Sessions {
		mark := ""
		if s.Attached {
			mark = " (attached)"
		}
		fmt.Printf("%s: %d window(s), %d pane(s)%s\n", s.Name, s.Windows, s.Panes, mark)
	}
	return nil
}

func cmdKillSession(ep ipc.Endpoint, args []string) error {
	name, err := sessionFlag(args)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("kill-session needs -t <name>")
	}
	conn, err := ipc.Dial(ep)
	if err != nil {
		return fmt.Errorf("no server running")
	}
	pc := protocol.NewConn(conn)
	if err := pc.Write(protocol.Message{Type: protocol.TypeKillSession, Name: name}); err != nil {
		return err
	}
	_, _ = pc.Read()
	fmt.Printf("killed session %q\n", name)
	return nil
}

func cmdKillServer(ep ipc.Endpoint) error {
	conn, err := ipc.Dial(ep)
	if err != nil {
		return fmt.Errorf("no server running")
	}
	pc := protocol.NewConn(conn)
	if err := pc.Write(protocol.Message{Type: protocol.TypeKillServer}); err != nil {
		return err
	}
	_, _ = pc.Read() // wait for Bye
	fmt.Println("server stopped")
	return nil
}

// startDaemon spawns a detached copy of this binary running `__server` and
// waits for its socket to come up.
func startDaemon(ep ipc.Endpoint) error {
	if err := spawnDaemon(ep); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := ipc.Dial(ep); err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not come up within 5s")
}

// runDaemon is the daemon's main function (the `__server` subcommand).
func runDaemon(ep ipc.Endpoint) error {
	logger := log.New(io.Discard, "", 0)
	if os.Getenv("NGMUX_DEBUG") != "" {
		logger = log.New(os.Stderr, "ngmuxd ", log.LstdFlags)
	}
	return server.Run(ep, 80, 24, logger)
}
