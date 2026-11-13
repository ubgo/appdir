//go:build windows

package appdir

import (
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
)

// deviceID returns the volume serial number of path, for the
// stop-at-disk-change rule (the Windows twin of a device id).
func deviceID(path string) (uint64, bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	// FILE_FLAG_BACKUP_SEMANTICS is required to open a folder handle.
	h, err := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		vol := filepath.VolumeName(path)
		if vol == "" {
			return 0, false
		}
		var sum uint64
		for _, r := range strings.ToUpper(vol) {
			sum = sum*31 + uint64(r)
		}
		return sum, true
	}
	defer func() { _ = syscall.CloseHandle(h) }() // read-only handle
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return 0, false
	}
	return uint64(info.VolumeSerialNumber), true
}

// ownerUID has no Windows meaning: there is no sudo-to-another-user case.
func ownerUID(string) (int, bool) { return 0, false }

// currentUser is the user name without the domain, safe for file and pipe names.
func currentUser() string {
	u, err := user.Current()
	if err != nil {
		return "user"
	}
	name := u.Username
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		if r < 128 && (r == '.' || r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "user"
	}
	return b.String()
}

// isRoot is false: an elevated Windows process is the same user, so the
// root-in-another-user's-home check does not apply (SPEC 4.8).
func isRoot() bool { return false }

// ownedByMe is true: %TEMP% is per user on Windows, so the shared-/tmp
// attack the owner check guards against does not exist there.
func ownedByMe(string) bool { return true }
