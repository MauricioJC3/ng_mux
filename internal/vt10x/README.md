# internal/vt10x

An in-tree fork of [github.com/hinshun/vt10x](https://github.com/hinshun/vt10x)
(commit `5011da428d02`), the headless VT100/xterm emulator ngmux renders panes
from. Vendored so ngmux can extend it — the upstream has no notion of
double-width characters, which ngmux needs.

Changes from upstream:

- Dropped `ioctl_posix.go` / `ioctl_other.go` (`ResizePty`): unused by ngmux,
  and the two build-tagged copies had mismatched signatures.
- `gofmt`ed (added `//go:build` lines).

Everything else is upstream. Original MIT licence in `LICENSE`.
