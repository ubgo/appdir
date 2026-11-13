//go:build windows

package appdir

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// renameReplace renames from over to, retrying a few times when antivirus or
// the indexer holds the target open (standard platform table). os.Rename uses
// MoveFileEx with MOVEFILE_REPLACE_EXISTING.
func renameReplace(from, to string) error {
	const (
		errSharingViolation = syscall.Errno(32)
		errAccessDenied     = syscall.ERROR_ACCESS_DENIED
		attempts            = 5
	)
	var err error
	for i := 0; i < attempts; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		if !errors.Is(err, errSharingViolation) && !errors.Is(err, errAccessDenied) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 50 * time.Millisecond)
	}
	return err
}
