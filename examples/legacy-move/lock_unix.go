//go:build unix

package main

import (
	"fmt"
	"os"
	"syscall"
)

// holdLock plays the old daemon that is still running: it takes the same
// kind of OS lock (flock, exclusive) on its daemon.lock and keeps it until
// release is called. MoveLegacy probes the file and must see it as held.
func holdLock(path string) (release func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close() // the lock error is the one worth reporting
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return func() { _ = f.Close() }, nil // closing the file drops the lock
}
