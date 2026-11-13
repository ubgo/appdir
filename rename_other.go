//go:build !windows

package appdir

import "os"

// renameReplace renames from over to.
func renameReplace(from, to string) error { return os.Rename(from, to) }
