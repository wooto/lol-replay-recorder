//go:build windows && amd64

package recorder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	enumWindows   = user32.NewProc("EnumWindows")
	getWindowPID  = user32.NewProc("GetWindowThreadProcessId")
	isVisible     = user32.NewProc("IsWindowVisible")
	showWindow    = user32.NewProc("ShowWindow")
	setForeground = user32.NewProc("SetForegroundWindow")
	getForeground = user32.NewProc("GetForegroundWindow")
	sendInput     = user32.NewProc("SendInput")
	createMutex   = kernel32.NewProc("CreateMutexW")
)

type nativeDesktop struct{}

func (nativeDesktop) acquire() (func(), error) {
	name, _ := syscall.UTF16PtrFromString(`Local\WootoLoLReplayRecorder`)
	handle, _, err := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return nil, fmt.Errorf("create replay lock: %w", err)
	}
	if errors.Is(err, syscall.Errno(183)) {
		_ = syscall.CloseHandle(syscall.Handle(handle))
		return nil, ErrBusy
	}
	var once sync.Once
	return func() { once.Do(func() { _ = syscall.CloseHandle(syscall.Handle(handle)) }) }, nil
}

type ownedProcess struct {
	command  *exec.Cmd
	done     atomic.Bool
	finished chan struct{}
}

func (p *ownedProcess) pid() int     { return p.command.Process.Pid }
func (p *ownedProcess) exited() bool { return p.done.Load() }
func (p *ownedProcess) close() error {
	if !p.done.Load() {
		if err := p.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
	}
	select {
	case <-p.finished:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("owned game process did not exit")
	}
}
func (nativeDesktop) launch(ctx context.Context, config Config, replay string) (replayProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	executable := config.GameExecutable
	if executable == "" {
		executable = `C:\Riot Games\League of Legends\Game\League of Legends.exe`
	}
	executable, err := filepath.Abs(executable)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("League game executable not found; set GameExecutable")
	}
	args := []string{replay}
	if config.ExtraLaunchArgs == nil {
		args = append(args, "-GameBaseDir="+filepath.Dir(filepath.Dir(executable)))
	} else {
		args = append(args, config.ExtraLaunchArgs...)
	}
	// Native argument escaping by os/exec; never send paths through a shell.
	command := exec.Command(executable, args...)
	command.Dir = filepath.Dir(executable)
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("launch replay: %w", err)
	}
	p := &ownedProcess{command: command, finished: make(chan struct{})}
	go func() { _ = command.Wait(); p.done.Store(true); close(p.finished) }()
	return p, nil
}

// keyboardInput matches the Windows x64 INPUT union (40 bytes).
type keyboardInput struct {
	Type         uint32
	Align        uint32
	VK           uint16
	Scan         uint16
	Flags        uint32
	Time         uint32
	Padding      uint32
	Extra        uintptr
	UnionPadding [8]byte
}

var windowLookup struct {
	sync.Mutex
	pid    int
	window uintptr
}

// Go callback trampolines cannot be freed. Reuse one for repeated recordings.
var findOwnedWindow = syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
	var candidate uint32
	getWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&candidate)))
	visible, _, _ := isVisible.Call(hwnd)
	if int(candidate) == windowLookup.pid && visible != 0 {
		windowLookup.window = hwnd
		return 0
	}
	return 1
})

func (nativeDesktop) selectPlayer(ctx context.Context, pid int, key uint16) error {
	windowLookup.Lock()
	defer windowLookup.Unlock()
	windowLookup.pid, windowLookup.window = pid, 0
	enumWindows.Call(findOwnedWindow, 0)
	window := windowLookup.window
	if window == 0 {
		return errors.New("owned game has no visible window")
	}
	showWindow.Call(window, 9)
	setForeground.Call(window)
	if err := wait(ctx, 150*time.Millisecond); err != nil {
		return err
	}
	foreground, _, _ := getForeground.Call()
	if foreground != window {
		return errors.New("Windows denied game window focus; use an unlocked interactive desktop")
	}
	for tap := 0; tap < 2; tap++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		foreground, _, _ := getForeground.Call()
		if foreground != window {
			return errors.New("game lost foreground before camera selection")
		}
		inputs := [2]keyboardInput{{Type: 1, VK: key}, {Type: 1, VK: key, Flags: 2}}
		inserted, _, err := sendInput.Call(2, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
		if inserted != 2 {
			return fmt.Errorf("Windows camera input was not delivered (%d/2): %v", inserted, err)
		}
		if err := wait(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
	return nil
}
