//go:build windows

package appdir

import "syscall"

// fileAttributeNotContentIndexed is FILE_ATTRIBUTE_NOT_CONTENT_INDEXED.
const fileAttributeNotContentIndexed = 0x00002000

// excludeFromBackup sets the "not content indexed" attribute.
func excludeFromBackup(dir string) error {
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return err
	}
	return syscall.SetFileAttributes(p, attrs|fileAttributeNotContentIndexed)
}
