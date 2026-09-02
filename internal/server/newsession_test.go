package server

import (
	"strings"
	"testing"
)

func TestNewSessionCreatesNamed(t *testing.T) {
	for _, line := range []string{"new-session -s work", "new -s work", "new-session work"} {
		t.Run(line, func(t *testing.T) {
			srv, _, _ := setupSession(t) // starts with session "0"
			exec(t, srv, line)
			if !srv.hasSession("work") {
				t.Fatalf("%q did not create session %q", line, "work")
			}
		})
	}
}

func TestNewSessionAutoNamesNextFreeInteger(t *testing.T) {
	srv, _, _ := setupSession(t) // "0" is taken
	exec(t, srv, "new-session")
	if !srv.hasSession("1") {
		t.Fatalf("unnamed new-session should have created %q", "1")
	}
	exec(t, srv, "new-session")
	if !srv.hasSession("2") {
		t.Fatalf("second unnamed new-session should have created %q", "2")
	}
}

func TestNewSessionRejectsExistingName(t *testing.T) {
	srv, _, _ := setupSession(t) // "0" exists
	if _, err := srv.execCommand(nil, "new-session -s 0"); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want an 'already exists' error", err)
	}
}

func TestNewSessionRunsFromCLIWithoutAClient(t *testing.T) {
	srv, _, _ := setupSession(t)
	// needsClient is false: this must not be rejected the way detach is.
	if _, err := srv.execCommand(nil, "new-session -s scripted"); err != nil {
		t.Fatalf("new-session from CLI: %v", err)
	}
	if !srv.hasSession("scripted") {
		t.Fatal("session was not created")
	}
}
