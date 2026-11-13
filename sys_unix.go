//go:build unix

package appdir

import (
	"os"
	"strconv"
	"syscall"
)

// deviceID returns the device id of path, for the stop-at-disk-change rule.
func deviceID(path string) (uint64, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true //nolint:unconvert // Dev is int32 on darwin, uint64 on linux
}

// ownerUID returns the owner uid of path.
func ownerUID(path string) (int, bool) {
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}

// currentUser is the numeric uid, used in temp and socket folder names.
func currentUser() string { return strconv.Itoa(os.Getuid()) }

// isRoot reports an effective uid of 0.
func isRoot() bool { return os.Geteuid() == 0 }

// ownedByMe reports whether path is owned by the effective user.
func ownedByMe(path string) bool {
	uid, ok := ownerUID(path)
	return ok && uid == os.Geteuid()
}
