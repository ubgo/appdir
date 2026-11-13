//go:build windows

package appdir

import (
	"os"
	"path/filepath"
	"strings"
)

// maxJunctionHops bounds junction resolution, like the kernel's own limit on
// nested reparse points, so a junction loop cannot spin forever.
const maxJunctionHops = 63

// evalLinks is filepath.EvalSymlinks that also follows directory junctions.
// Since Go 1.23 (winsymlink=1) EvalSymlinks treats a junction as a plain
// folder (a mount point), yet a junction leads somewhere else just like a
// symlink, so the symlink rule (SPEC 5.6) has to follow it: a junction
// inside a home that leads outside it must be seen as an escape.
func evalLinks(p string) (string, error) {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	for range maxJunctionHops {
		next, found := resolveFirstJunction(r)
		if !found {
			return r, nil
		}
		if r, err = filepath.EvalSymlinks(next); err != nil {
			return "", err
		}
	}
	return "", &os.PathError{Op: "evallinks", Path: p, Err: os.ErrInvalid}
}

// resolveFirstJunction finds the first component of p (from the volume
// root down) that is a junction and returns p with that prefix replaced by
// the junction's target.
func resolveFirstJunction(p string) (string, bool) {
	vol := filepath.VolumeName(p)
	rest := strings.TrimPrefix(p[len(vol):], string(filepath.Separator))
	if rest == "" {
		return "", false
	}
	prefix := vol + string(filepath.Separator)
	parts := strings.Split(rest, string(filepath.Separator))
	for i, part := range parts {
		prefix = filepath.Join(prefix, part)
		if target, ok := junctionTarget(prefix); ok {
			return filepath.Join(append([]string{target}, parts[i+1:]...)...), true
		}
	}
	return "", false
}

// junctionTarget returns where the junction at p leads. A path that is not
// a junction, and a volume mount point (its target is a \\?\Volume{...}
// name, not a folder path), report false.
func junctionTarget(p string) (string, bool) {
	fi, err := os.Lstat(p)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || fi.Mode()&os.ModeIrregular == 0 {
		return "", false // missing, a symlink (EvalSymlinks did it), or a plain file or folder
	}
	t, err := os.Readlink(p)
	if err != nil || t == "" || strings.HasPrefix(t, `\\?\`) || strings.HasPrefix(t, `\??\`) || !filepath.IsAbs(t) {
		return "", false
	}
	return filepath.Clean(t), true
}

// isLink reports whether fi (from os.Lstat of p) is a symlink or a
// directory junction.
func isLink(p string, fi os.FileInfo) bool {
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	_, ok := junctionTarget(p)
	return ok
}
