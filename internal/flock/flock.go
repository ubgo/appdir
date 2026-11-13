// Package flock is the minimal OS file lock appdir needs: flock(LOCK_EX|LOCK_NB)
// on Unix, LockFileEx with LOCKFILE_FAIL_IMMEDIATELY on Windows.
//
// Not ubgo/lock/flock: that package writes a .fence sidecar next to every lock
// and creates with 0644/0755, and appdir must be able to probe an old daemon's
// lock without creating any file in the old home (see docs/SPEC.md section 9).
package flock

import (
	"errors"
	"fmt"
	"os"
)

// ErrLocked means another process holds the lock.
var ErrLocked = errors.New("flock: locked by another process")

// Lock is a held lock. The kernel releases it when Release closes the file or
// the process dies, so a lock held by a dead process is free by itself. The
// lock file is never deleted: unlinking a lock file races with a waiter that
// already opened it.
type Lock struct {
	f *os.File
}

// TryLock creates path (mode perm) if needed and takes it without waiting.
func TryLock(path string, perm os.FileMode) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, perm)
	if err != nil {
		return nil, fmt.Errorf("flock: open %s: %w", path, err)
	}
	if err := tryLock(f); err != nil {
		_ = f.Close() // the lock error is the one worth reporting
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Held reports whether another process holds the lock at path. It never
// creates the file: a missing file means nobody holds it.
func Held(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("flock: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }() // closing also drops our probe lock
	err = tryLock(f)
	if errors.Is(err, ErrLocked) {
		return true, nil
	}
	return false, err
}

// Release drops the lock.
func (l *Lock) Release() error {
	return l.f.Close()
}
