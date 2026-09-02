package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConf(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ngmux.conf")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingFileIsDefault(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "nope.conf"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Prefix != DefaultPrefix || cfg.HistoryLimit != 2000 {
		t.Fatalf("expected defaults, got %+v", cfg)
	}
}

func TestParseFullConfig(t *testing.T) {
	p := writeConf(t, `
# a comment
set prefix C-a
set history-limit 9000
set default-shell /bin/bash
set mouse on
set status-fg 3
set status-bg 4
bind s split-vertical
bind v split-horizontal   # trailing comment
`)
	cfg, err := LoadFile(p)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", cfg.Warnings)
	}
	if cfg.Prefix != 0x01 {
		t.Errorf("prefix = %d, want 1 (C-a)", cfg.Prefix)
	}
	if cfg.HistoryLimit != 9000 {
		t.Errorf("history-limit = %d, want 9000", cfg.HistoryLimit)
	}
	if cfg.DefaultShell != "/bin/bash" {
		t.Errorf("default-shell = %q", cfg.DefaultShell)
	}
	if !cfg.Mouse {
		t.Errorf("mouse = false, want true")
	}
	if cfg.StatusFG != 3 || cfg.StatusBG != 4 {
		t.Errorf("status colours = %d/%d, want 3/4", cfg.StatusFG, cfg.StatusBG)
	}
	if cfg.Binds["s"] != "split-vertical" || cfg.Binds["v"] != "split-horizontal" {
		t.Errorf("binds = %v", cfg.Binds)
	}
}

func TestMalformedLinesBecomeWarnings(t *testing.T) {
	p := writeConf(t, `
set prefix
set history-limit abc
frobnicate the widget
bind toolong split-vertical
set prefix C-x
`)
	cfg, err := LoadFile(p)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(cfg.Warnings) != 4 {
		t.Fatalf("expected 4 warnings, got %d: %v", len(cfg.Warnings), cfg.Warnings)
	}
	// The last valid line still took effect.
	if cfg.Prefix != 0x18 {
		t.Errorf("prefix = %d, want 0x18 (C-x)", cfg.Prefix)
	}
}

func TestMouseDefaultsOnAndCanBeDisabled(t *testing.T) {
	def, _ := LoadFile(filepath.Join(t.TempDir(), "none.conf"))
	if !def.Mouse {
		t.Errorf("mouse should default on")
	}
	off, _ := LoadFile(writeConf(t, "set mouse off\n"))
	if off.Mouse {
		t.Errorf("`set mouse off` should disable mouse")
	}
}

func TestParseKeyForms(t *testing.T) {
	cases := map[string]byte{"C-a": 1, "c-a": 1, "^A": 1, "C-b": 2, "q": 'q'}
	for in, want := range cases {
		got, err := parseKey(in)
		if err != nil || got != want {
			t.Errorf("parseKey(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
}
