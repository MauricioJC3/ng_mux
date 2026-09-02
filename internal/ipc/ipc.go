// Package ipc gives the client and server a single connection abstraction that
// works on both platforms: a Unix domain socket on Linux/macOS and a named
// pipe on Windows. Callers deal only with net.Conn and net.Listener.
package ipc

import "errors"

// ErrNoServer is returned by Dial when no server is listening at the endpoint.
var ErrNoServer = errors.New("ipc: no server running")

// Endpoint identifies one server instance. Name is usually "default"; a
// different name gives an isolated second server (the equivalent of tmux -L).
type Endpoint struct {
	Name string
}

// DefaultEndpoint is the server everyone attaches to unless told otherwise.
var DefaultEndpoint = Endpoint{Name: "default"}
