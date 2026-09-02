//go:build windows

package winpty

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtractToUnpacksBothBinaries runs on the Windows CI job: it exercises the
// //go:embed payload and the on-disk unpack that the winpty backend depends on.
func TestExtractToUnpacksBothBinaries(t *testing.T) {
	dir, err := extractTo(t.TempDir())
	if err != nil {
		t.Fatalf("extractTo: %v", err)
	}
	for _, name := range embedded {
		want, err := binFS.ReadFile("bin/" + name)
		if err != nil {
			t.Fatalf("embedded %s missing: %v", name, err)
		}
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read unpacked %s: %v", name, err)
		}
		if len(got) != len(want) {
			t.Errorf("%s: unpacked %d bytes, embedded %d", name, len(got), len(want))
		}
	}
}

// TestExtractToIsIdempotent: a second call with the same base neither errors nor
// rewrites (the dir is content-hashed, so a same-size file is left alone).
func TestExtractToIsIdempotent(t *testing.T) {
	base := t.TempDir()
	first, err := extractTo(base)
	if err != nil {
		t.Fatalf("first extractTo: %v", err)
	}
	fi, err := os.Stat(filepath.Join(first, "winpty.dll"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := extractTo(base)
	if err != nil {
		t.Fatalf("second extractTo: %v", err)
	}
	if first != second {
		t.Fatalf("dir changed between calls: %q then %q", first, second)
	}
	fi2, _ := os.Stat(filepath.Join(second, "winpty.dll"))
	if !fi2.ModTime().Equal(fi.ModTime()) {
		t.Errorf("winpty.dll was rewritten on the second call")
	}
}

func TestTagIsStableAndShort(t *testing.T) {
	a, b := tag(), tag()
	if a != b {
		t.Fatalf("tag() not stable: %q vs %q", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("tag() = %q, want 16 hex chars", a)
	}
}
