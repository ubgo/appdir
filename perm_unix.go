//go:build unix

package appdir

import "os"

// restrict sets owner-only permissions on something appdir created.
func restrict(path string, perm os.FileMode, _ bool) error {
	return os.Chmod(path, perm)
}
