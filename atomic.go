package appdir

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic writes a temp file in the same folder, fsyncs and renames
// it over path. An existing file keeps its permissions; a symlink target is
// written through (the link stays).
func (d *Dirs) writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	newFile := true
	if fi, err := os.Stat(path); err == nil {
		perm, newFile = fi.Mode().Perm(), false
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	name := tmp.Name()
	cleanup := func(err error) error {
		_ = tmp.Close()     // already failed; the write error matters
		_ = os.Remove(name) // our own temp file
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		return cleanup(err)
	}
	if err := tmp.Sync(); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		return cleanup(err)
	}
	if err := os.Chmod(name, perm); err != nil {
		return cleanup(err)
	}
	if err := renameReplace(name, path); err != nil {
		_ = os.Remove(name) // our own temp file
		return fmt.Errorf("write %s: %w", path, err)
	}
	if newFile {
		return restrict(path, perm, false)
	}
	return nil
}
