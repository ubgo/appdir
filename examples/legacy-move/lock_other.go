//go:build !unix

package main

import "errors"

// errNoLockDemo: holding a lock the way the old daemon would needs LockFileEx
// on Windows, which the standard library does not export; the busy case is
// shown on Unix only.
var errNoLockDemo = errors.New("the busy case runs on Unix only")

// holdLock is the non-Unix stand-in for the Unix version: it always fails,
// and the busy step prints the error and skips itself.
func holdLock(string) (func(), error) { return nil, errNoLockDemo }
