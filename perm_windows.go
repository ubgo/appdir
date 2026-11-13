//go:build windows

package appdir

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Windows security constants (winnt.h); syscall does not export them.
const (
	daclSecurityInformation          = 0x00000004
	protectedDaclSecurityInformation = 0x80000000
	sddlRevision1                    = 1
)

// restrict gives path a protected DACL that grants full access to the current
// user and SYSTEM only, inheritance from the parent off: the Windows twin of
// 0700 / 0600 (standard platform table). The mode bits are ignored.
func restrict(path string, _ os.FileMode, isDir bool) error {
	tok, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return fmt.Errorf("acl %s: %w", path, err)
	}
	defer func() { _ = tok.Close() }() // token handle only
	tu, err := tok.GetTokenUser()
	if err != nil {
		return fmt.Errorf("acl %s: %w", path, err)
	}
	sid, err := tu.User.Sid.String()
	if err != nil {
		return fmt.Errorf("acl %s: %w", path, err)
	}
	inherit := ""
	if isDir {
		inherit = "OICI" // children of a folder get the same two entries
	}
	sddl := fmt.Sprintf("D:P(A;%s;FA;;;%s)(A;%s;FA;;;SY)", inherit, sid, inherit)

	advapi := syscall.NewLazyDLL("advapi32.dll")
	conv := advapi.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	set := advapi.NewProc("SetFileSecurityW")
	sddlPtr, err := syscall.UTF16PtrFromString(sddl)
	if err != nil {
		return fmt.Errorf("acl %s: %w", path, err)
	}
	var sd uintptr
	if r, _, e := conv.Call(uintptr(unsafe.Pointer(sddlPtr)), sddlRevision1, uintptr(unsafe.Pointer(&sd)), 0); r == 0 {
		return fmt.Errorf("acl %s: %w", path, e)
	}
	defer func() { _, _ = syscall.LocalFree(syscall.Handle(sd)) }() // frees the descriptor buffer
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("acl %s: %w", path, err)
	}
	if r, _, e := set.Call(uintptr(unsafe.Pointer(pathPtr)), daclSecurityInformation|protectedDaclSecurityInformation, sd); r == 0 {
		return fmt.Errorf("acl %s: %w", path, e)
	}
	return nil
}
