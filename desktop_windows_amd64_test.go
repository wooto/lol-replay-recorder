//go:build windows && amd64

package recorder

import (
	"errors"
	"testing"
	"unsafe"
)

func TestWindowsInputABI(t *testing.T) {
	if size := unsafe.Sizeof(keyboardInput{}); size != 40 {
		t.Fatalf("INPUT ABI size is %d", size)
	}
}
func TestWindowsReplayLease(t *testing.T) {
	desktop := nativeDesktop{}
	release, err := desktop.acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	second, err := desktop.acquire()
	if second != nil {
		second()
	}
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("second process lease should be busy, got %v", err)
	}
	release()
	again, err := desktop.acquire()
	if err != nil {
		t.Fatal(err)
	}
	again()
}
