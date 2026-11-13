//go:build !darwin && !windows

package appdir

// excludeFromBackup has nothing beyond CACHEDIR.TAG on Linux and the BSDs.
func excludeFromBackup(string) error { return nil }
