//go:build !windows

package appdir

import (
	"os"
	"path/filepath"
)

// evalLinks is filepath.EvalSymlinks: on Unix every link is a symlink.
func evalLinks(p string) (string, error) { return filepath.EvalSymlinks(p) }

// isLink reports whether fi (from os.Lstat of p) is a link.
func isLink(_ string, fi os.FileInfo) bool { return fi.Mode()&os.ModeSymlink != 0 }
