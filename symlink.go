package appdir

import (
	"os"
	"path/filepath"
	"strings"
)

// VerifyFile applies the symlink rule (SPEC 5.6): a file may be a symlink
// only when its real path is inside the real path of the home it belongs to.
// ds:def id=dirs-verifyfile-2nyrnmv3 owner=@khanakia stability=stable
func (d *Dirs) VerifyFile(path string) error {
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	home := d.homeOf(path)
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return newErr(ErrSymlinkEscape, "%s is a broken symlink", path)
	}
	if !within(real, home) {
		return newErr(ErrSymlinkEscape, "%s links to %s, outside the home %s", path, real, home)
	}
	return nil
}

// homeOf returns the known home folder that lexically contains path; the
// file's own folder when none does.
func (d *Dirs) homeOf(path string) string {
	best := ""
	for _, h := range []string{d.global.Base, d.global.Data, d.project.Base, d.project.Data, d.project.Marker} {
		if h == "" {
			continue
		}
		if lexicallyWithin(path, h) && len(h) > len(best) {
			best = h
		}
	}
	if best == "" {
		return filepath.Dir(path)
	}
	return best
}

// lexicallyWithin reports whether p is base or below it, by path text only.
// Only ".." itself or a "../" prefix leaves base: a child folder named
// "..x" is inside (a bare HasPrefix(rel, "..") check got that wrong).
func lexicallyWithin(p, base string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
