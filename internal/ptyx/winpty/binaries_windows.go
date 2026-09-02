//go:build windows

package winpty

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

//go:embed bin/winpty.dll bin/winpty-agent.exe
var binFS embed.FS

var embedded = []string{"winpty.dll", "winpty-agent.exe"}

var extract struct {
	sync.Once
	dir string
	err error
}

// tag is a short content hash of the embedded binaries. The unpack directory is
// keyed by it, so a newer ngmux build unpacks fresh DLLs instead of reusing a
// stale copy left by an older one.
func tag() string {
	h := sha256.New()
	for _, name := range embedded {
		b, _ := binFS.ReadFile("bin/" + name)
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Dir unpacks the embedded winpty binaries once and returns the directory that
// holds winpty.dll and winpty-agent.exe.
func Dir() (string, error) {
	extract.Do(func() { extract.dir, extract.err = extractTo(dataRoot()) })
	return extract.dir, extract.err
}

// Available reports whether the winpty fallback is usable on this machine: it
// unpacks the binaries and confirms they are in place.
func Available() error {
	_, err := Dir()
	return err
}

// dataRoot is where per-user ngmux data lives on Windows.
func dataRoot() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	if v := os.Getenv("APPDATA"); v != "" {
		return v
	}
	return os.TempDir()
}

// extractTo unpacks the binaries under base/ngmux/winpty/<tag> and returns that
// directory. It is a no-op when a same-size copy is already there.
func extractTo(base string) (string, error) {
	dir := filepath.Join(base, "ngmux", "winpty", tag())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("winpty: create %s: %w", dir, err)
	}
	for _, name := range embedded {
		if err := writeIfNeeded(dir, name); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func writeIfNeeded(dir, name string) error {
	want, err := binFS.ReadFile("bin/" + name)
	if err != nil {
		return fmt.Errorf("winpty: embedded %s missing: %w", name, err)
	}
	dst := filepath.Join(dir, name)
	// The directory name is a content hash, so a same-size file here is ours.
	if fi, err := os.Stat(dst); err == nil && fi.Size() == int64(len(want)) {
		return nil
	}
	tmp := fmt.Sprintf("%s.%d.tmp", dst, os.Getpid())
	if err := os.WriteFile(tmp, want, 0o755); err != nil {
		return fmt.Errorf("winpty: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		// A concurrent ngmux may have installed it first (and may already have
		// the DLL loaded, which blocks the replace); an identical file is fine.
		if fi, statErr := os.Stat(dst); statErr == nil && fi.Size() == int64(len(want)) {
			return nil
		}
		return fmt.Errorf("winpty: install %s: %w", dst, err)
	}
	return nil
}
