//go:build windows

package lcu

import (
	"errors"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	recorder "github.com/wooto/lol-replay-recorder"
)

func gameProcesses() (map[int]struct{}, error) {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(snapshot)
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	result := map[int]struct{}{}
	err = syscall.Process32First(snapshot, &entry)
	for err == nil {
		if strings.EqualFold(syscall.UTF16ToString(entry.ExeFile[:]), "League of Legends.exe") {
			result[int(entry.ProcessID)] = struct{}{}
		}
		err = syscall.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return result, nil
}

type gameProcess struct {
	mu     sync.Mutex
	pid    int
	handle syscall.Handle
	closed bool
}

func openGameProcess(pid int) (recorder.ReplayProcess, error) {
	processes, err := gameProcesses()
	if err != nil {
		return nil, err
	}
	if _, ok := processes[pid]; !ok {
		return nil, errors.New("new game process disappeared")
	}
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE|syscall.PROCESS_TERMINATE|0x1000, false, uint32(pid))
	if err != nil {
		return nil, err
	}
	return &gameProcess{pid: pid, handle: h}, nil
}
func (p *gameProcess) PID() int { return p.pid }
func (p *gameProcess) Exited() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return true
	}
	result, err := syscall.WaitForSingleObject(p.handle, 0)
	return err != nil || result == syscall.WAIT_OBJECT_0
}
func (p *gameProcess) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	result, err := syscall.WaitForSingleObject(p.handle, 0)
	if err != nil {
		return err
	}
	if result != syscall.WAIT_OBJECT_0 {
		if err = syscall.TerminateProcess(p.handle, 0); err != nil {
			return err
		}
		if result, err = syscall.WaitForSingleObject(p.handle, 5000); err != nil {
			return err
		} else if result != syscall.WAIT_OBJECT_0 {
			return errors.New("game process did not exit")
		}
	}
	if err = syscall.CloseHandle(p.handle); err != nil {
		return err
	}
	p.closed = true
	return nil
}
