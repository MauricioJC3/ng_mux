//go:build windows

package winpty

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// winpty agent / spawn flags (winpty_constants.h).
const (
	flagColorEscapes      = 0x4 // emit ANSI colour escapes, not attribute records
	spawnFlagAutoShutdown = 0x1 // close the agent when the spawned process exits
)

// dll is the set of winpty.dll entry points ngmux uses.
type dll struct {
	errorFree, errorMsg             *windows.LazyProc
	configNew, configFree           *windows.LazyProc
	configSetInitialSize            *windows.LazyProc
	open                            *windows.LazyProc
	coninName, conoutName           *windows.LazyProc
	spawnConfigNew, spawnConfigFree *windows.LazyProc
	spawn                           *windows.LazyProc
	setSize, free                   *windows.LazyProc
}

var loaded struct {
	sync.Once
	d   *dll
	err error
}

func load() (*dll, error) {
	loaded.Do(func() {
		dir, err := Dir()
		if err != nil {
			loaded.err = err
			return
		}
		lib := windows.NewLazyDLL(filepath.Join(dir, "winpty.dll"))
		if err := lib.Load(); err != nil {
			loaded.err = fmt.Errorf("winpty: load winpty.dll: %w", err)
			return
		}
		proc := func(name string) *windows.LazyProc {
			p := lib.NewProc(name)
			if e := p.Find(); e != nil && loaded.err == nil {
				loaded.err = fmt.Errorf("winpty: missing %s: %w", name, e)
			}
			return p
		}
		loaded.d = &dll{
			errorFree:            proc("winpty_error_free"),
			errorMsg:             proc("winpty_error_msg"),
			configNew:            proc("winpty_config_new"),
			configFree:           proc("winpty_config_free"),
			configSetInitialSize: proc("winpty_config_set_initial_size"),
			open:                 proc("winpty_open"),
			coninName:            proc("winpty_conin_name"),
			conoutName:           proc("winpty_conout_name"),
			spawnConfigNew:       proc("winpty_spawn_config_new"),
			spawnConfigFree:      proc("winpty_spawn_config_free"),
			spawn:                proc("winpty_spawn"),
			setSize:              proc("winpty_set_size"),
			free:                 proc("winpty_free"),
		}
	})
	return loaded.d, loaded.err
}

// PTY is an open winpty session with a child process running inside it.
type PTY struct {
	d       *dll
	wp      uintptr // winpty_t*
	conin   *os.File
	conout  *os.File
	process windows.Handle // spawned child

	closeOnce sync.Once
}

// Open starts prog+args on a new winpty sized cols x rows. dir and env are
// optional (empty dir / nil env inherit from this process).
func Open(prog string, args []string, dir string, env []string, cols, rows int) (*PTY, error) {
	d, err := load()
	if err != nil {
		return nil, err
	}

	cfg, _, _ := d.configNew.Call(uintptr(flagColorEscapes), 0)
	if cfg == 0 {
		return nil, errors.New("winpty: winpty_config_new failed")
	}
	defer d.configFree.Call(cfg)
	d.configSetInitialSize.Call(cfg, uintptr(int32(cols)), uintptr(int32(rows)))

	var openErr uintptr
	wp, _, _ := d.open.Call(cfg, uintptr(unsafe.Pointer(&openErr)))
	if wp == 0 {
		return nil, d.errMsg("winpty_open", openErr)
	}
	ok := false
	defer func() {
		if !ok {
			d.free.Call(wp)
		}
	}()

	conout, err := openPipe(lpcwstr(d.conoutName.Call(wp)), windows.GENERIC_READ)
	if err != nil {
		return nil, err
	}
	conin, err := openPipe(lpcwstr(d.coninName.Call(wp)), windows.GENERIC_WRITE)
	if err != nil {
		conout.Close()
		return nil, err
	}

	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{prog}, args...)))
	if err != nil {
		conin.Close()
		conout.Close()
		return nil, err
	}
	var cwdPtr, envPtr *uint16
	if dir != "" {
		if cwdPtr, err = windows.UTF16PtrFromString(dir); err != nil {
			conin.Close()
			conout.Close()
			return nil, err
		}
	}
	if len(env) > 0 {
		block, err := envBlock(env)
		if err != nil {
			conin.Close()
			conout.Close()
			return nil, err
		}
		envPtr = &block[0]
	}

	var spawnErr uintptr
	spawnCfg, _, _ := d.spawnConfigNew.Call(
		uintptr(spawnFlagAutoShutdown),
		0, // appname: NULL, take the program from cmdline
		uintptr(unsafe.Pointer(cmdline)),
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(unsafe.Pointer(envPtr)),
		uintptr(unsafe.Pointer(&spawnErr)),
	)
	if spawnCfg == 0 {
		conin.Close()
		conout.Close()
		return nil, d.errMsg("winpty_spawn_config_new", spawnErr)
	}
	defer d.spawnConfigFree.Call(spawnCfg)

	var (
		proc      windows.Handle
		createErr uint32
		spawnErr2 uintptr
	)
	ret, _, _ := d.spawn.Call(
		wp, spawnCfg,
		uintptr(unsafe.Pointer(&proc)),
		0, // thread handle: not needed
		uintptr(unsafe.Pointer(&createErr)),
		uintptr(unsafe.Pointer(&spawnErr2)),
	)
	if ret == 0 {
		conin.Close()
		conout.Close()
		if createErr != 0 {
			return nil, fmt.Errorf("winpty: CreateProcess %q: %w", prog, syscall.Errno(createErr))
		}
		return nil, d.errMsg("winpty_spawn", spawnErr2)
	}

	ok = true
	return &PTY{d: d, wp: wp, conin: conin, conout: conout, process: proc}, nil
}

func (p *PTY) Read(b []byte) (int, error)  { return p.conout.Read(b) }
func (p *PTY) Write(b []byte) (int, error) { return p.conin.Write(b) }

// SetSize resizes the winpty console.
func (p *PTY) SetSize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	var werr uintptr
	ret, _, _ := p.d.setSize.Call(p.wp, uintptr(int32(cols)), uintptr(int32(rows)), uintptr(unsafe.Pointer(&werr)))
	if ret == 0 {
		return p.d.errMsg("winpty_set_size", werr)
	}
	return nil
}

// Wait blocks until the child exits and returns its exit error (nil on 0).
func (p *PTY) Wait() error {
	if _, err := windows.WaitForSingleObject(p.process, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.process, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("winpty: shell exited with code %d", code)
	}
	return nil
}

// Kill best-effort terminates the child. Close then tears down the session.
func (p *PTY) Kill() { _ = windows.TerminateProcess(p.process, 1) }

// Close releases the pipes (unblocking any pending Read), shuts the agent down
// and drops the child handle. Safe to call more than once.
func (p *PTY) Close() error {
	var err error
	p.closeOnce.Do(func() {
		e1 := p.conout.Close()
		e2 := p.conin.Close()
		p.d.free.Call(p.wp)
		_ = windows.CloseHandle(p.process)
		err = errors.Join(e1, e2)
	})
	return err
}

// errMsg turns a winpty_error_ptr_t into a Go error and frees it.
func (d *dll) errMsg(where string, werr uintptr) error {
	if werr == 0 {
		return fmt.Errorf("winpty: %s failed", where)
	}
	msgPtr, _, _ := d.errorMsg.Call(werr)
	msg := goString(msgPtr)
	d.errorFree.Call(werr)
	if msg == "" {
		return fmt.Errorf("winpty: %s failed", where)
	}
	return fmt.Errorf("winpty: %s: %s", where, msg)
}

// lpcwstr reads the LPCWSTR returned by a LazyProc.Call (its r1) into a string.
func lpcwstr(r1, _ uintptr, _ error) string { return goString(r1) }

var procLstrcpynW = windows.NewLazySystemDLL("kernel32.dll").NewProc("lstrcpynW")

// goString copies a C wide string at the address ptr into a Go string. It hands
// ptr straight back to Win32 (lstrcpynW) rather than dereferencing a
// uintptr-derived pointer, so `go vet` stays happy; winpty's names and error
// messages are short, and a >1023-char string is simply truncated.
func goString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var buf [1024]uint16
	procLstrcpynW.Call(uintptr(unsafe.Pointer(&buf[0])), ptr, uintptr(len(buf)))
	return windows.UTF16ToString(buf[:])
}

func openPipe(name string, access uint32) (*os.File, error) {
	if name == "" {
		return nil, errors.New("winpty: empty pipe name")
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, access, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("winpty: open %s: %w", name, err)
	}
	return os.NewFile(uintptr(h), name), nil
}

// envBlock builds a CreateProcess-style environment block: KEY=VALUE entries
// each NUL-terminated, the whole thing terminated by an extra NUL, in UTF-16.
func envBlock(env []string) ([]uint16, error) {
	var buf []uint16
	for _, e := range env {
		u, err := windows.UTF16FromString(e)
		if err != nil {
			return nil, fmt.Errorf("winpty: bad env entry %q: %w", e, err)
		}
		buf = append(buf, u...) // UTF16FromString includes the trailing NUL
	}
	buf = append(buf, 0)
	return buf, nil
}
