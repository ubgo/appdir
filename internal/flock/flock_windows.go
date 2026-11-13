//go:build windows

package flock

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// Flags from winbase.h; syscall does not export them.
const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errLockViolation        = syscall.Errno(33) // ERROR_LOCK_VIOLATION
)

func tryLock(f *os.File) error {
	// The DLL handle is looked up per call instead of held in a package var:
	// appdir keeps no package-level state, and locking is rare.
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
	var ol syscall.Overlapped
	r1, _, err := proc.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		0xFFFFFFFF,
		0xFFFFFFFF,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 != 0 {
		return nil
	}
	if errors.Is(err, errLockViolation) {
		return ErrLocked
	}
	return err
}
