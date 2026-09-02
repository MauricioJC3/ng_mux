// Package config loads tmux2's optional configuration file. The format is one
// directive per line, '#' starts a comment:
//
//	set prefix C-a
//	set history-limit 5000
//	set default-shell /bin/bash
//	set mouse on
//	set status-bg 4
//	set status-fg 7
//	bind s split-vertical
//	bind v split-horizontal
//
// Unknown directives are collected as warnings rather than failing the load, so
// a newer config never bricks an older binary.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// DefaultPrefix is Ctrl-b, matching tmux.
const DefaultPrefix = 0x02

// Config is the resolved configuration. Zero value is the built-in default.
type Config struct {
	Prefix       byte              // control byte that begins a command sequence
	HistoryLimit int               // scrollback lines kept per pane
	DefaultShell string            // overrides $SHELL / platform default when set
	Mouse        bool              // reserved for a later phase
	StatusFG     int               // status bar foreground colour index (0..255)
	StatusBG     int               // status bar background colour index (0..255)
	Binds        map[string]string // key (single rune) -> command name

	// Warnings holds human-readable notes about lines that were ignored.
	Warnings []string
}

// Default returns the built-in configuration. Mouse support is on by default;
// disable it with `set mouse off` if you want native terminal selection.
func Default() Config {
	return Config{
		Prefix:       DefaultPrefix,
		HistoryLimit: 2000,
		Mouse:        true,
		StatusFG:     0,
		StatusBG:     7,
		Binds:        map[string]string{},
	}
}

// Path returns the location config is read from, honouring TMUX2_CONFIG then
// the per-OS user config directory.
func Path() string {
	if p := os.Getenv("TMUX2_CONFIG"); p != "" {
		return p
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "tmux2", "tmux2.conf")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "tmux2", "tmux2.conf")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "tmux2", "tmux2.conf")
}

// Load reads and parses Path(). A missing file is not an error: it returns the
// default config. A malformed line is recorded in Warnings and skipped.
func Load() (Config, error) {
	return LoadFile(Path())
}

// LoadFile is Load with an explicit path (used by tests).
func LoadFile(path string) (Config, error) {
	cfg := Default()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
		fields := strings.Fields(text)
		if err := cfg.apply(fields); err != nil {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("line %d: %v", line, err))
		}
	}
	return cfg, sc.Err()
}

func (c *Config) apply(fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "set":
		if len(fields) < 3 {
			return fmt.Errorf("set needs a name and a value")
		}
		return c.set(fields[1], strings.Join(fields[2:], " "))
	case "bind":
		if len(fields) < 3 {
			return fmt.Errorf("bind needs a key and a command")
		}
		key := fields[1]
		if len([]rune(key)) != 1 {
			return fmt.Errorf("bind key must be a single character, got %q", key)
		}
		c.Binds[key] = fields[2]
		return nil
	default:
		return fmt.Errorf("unknown directive %q", fields[0])
	}
}

func (c *Config) set(name, value string) error {
	switch name {
	case "prefix":
		b, err := parseKey(value)
		if err != nil {
			return err
		}
		c.Prefix = b
	case "history-limit":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("history-limit must be a non-negative integer")
		}
		c.HistoryLimit = n
	case "default-shell":
		c.DefaultShell = value
	case "mouse":
		on, err := parseBool(value)
		if err != nil {
			return err
		}
		c.Mouse = on
	case "status-fg":
		n, err := parseColor(value)
		if err != nil {
			return err
		}
		c.StatusFG = n
	case "status-bg":
		n, err := parseColor(value)
		if err != nil {
			return err
		}
		c.StatusBG = n
	default:
		return fmt.Errorf("unknown setting %q", name)
	}
	return nil
}

// parseKey turns "C-a", "C-b", "^A" or a bare single char into its control byte.
func parseKey(s string) (byte, error) {
	switch {
	case len(s) == 3 && (strings.HasPrefix(s, "C-") || strings.HasPrefix(s, "c-")):
		r := s[2]
		return ctrl(r)
	case len(s) == 2 && s[0] == '^':
		return ctrl(s[1])
	case len(s) == 1:
		return s[0], nil
	default:
		return 0, fmt.Errorf("cannot parse key %q (use C-a form)", s)
	}
}

func ctrl(r byte) (byte, error) {
	switch {
	case r >= 'a' && r <= 'z':
		return r - 'a' + 1, nil
	case r >= 'A' && r <= 'Z':
		return r - 'A' + 1, nil
	default:
		return 0, fmt.Errorf("C- must be followed by a letter, got %q", string(r))
	}
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "on", "true", "yes", "1":
		return true, nil
	case "off", "false", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("expected on/off, got %q", s)
	}
}

func parseColor(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 255 {
		return 0, fmt.Errorf("colour must be 0..255, got %q", s)
	}
	return n, nil
}
