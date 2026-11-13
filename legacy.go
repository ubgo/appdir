package appdir

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/appdir/internal/flock"
)

// MoveResult is the outcome of MoveLegacy.
type MoveResult struct {
	Status MoveStatus `json:"status"`
	From   string     `json:"from,omitempty"`
	To     string     `json:"to,omitempty"`
	Note   string     `json:"note,omitempty"`
}

// defaultDaemonLock is probed in an old home when LegacyHome names none.
const defaultDaemonLock = "run/daemon.lock"

// MoveLegacy moves an old global home to the new one on first run (SPEC 9,
// standard section 8). It never deletes user data and never moves committed
// project folders. With several WithLegacyHome entries, the first that exists
// is used.
// ds:def id=dirs-movelegacy-aw98ygmb owner=@khanakia stability=stable
func (d *Dirs) MoveLegacy(ctx context.Context) (MoveResult, error) {
	if len(d.cfg.legacy) == 0 {
		return MoveResult{Status: MoveNone}, nil
	}
	if d.profile != d.cfg.releaseProfile {
		return MoveResult{Status: MoveSkipped, Note: "profile " + string(d.profile) + " never runs a legacy move"}, nil
	}
	if d.override {
		return MoveResult{Status: MoveSkipped, Note: "home set by --home or " + d.cfg.envName(envHome)}, nil
	}
	if d.globalErr != nil {
		return MoveResult{Status: MoveSkipped, Note: "no home folder"}, nil
	}
	to := d.global.Base
	for _, l := range d.cfg.legacy {
		from := filepath.Join(d.userHome, l.Dir)
		if !isDir(from) && !unfinishedMoveFrom(to, from) {
			continue
		}
		return d.moveOne(ctx, l, from, to)
	}
	return MoveResult{Status: MoveNone, To: to}, nil
}

// ds:def id=dirs-moveone-9bhmva3m owner=@khanakia stability=stable
func (d *Dirs) moveOne(ctx context.Context, l LegacyHome, from, to string) (MoveResult, error) {
	res := MoveResult{From: from, To: to}
	if st, done := legacyState(from, to); done {
		res.Status = st
		if st == MoveBothExist {
			res.Note = "both exist; using " + to
		}
		return res, nil
	}
	if err := d.checkWrite(to, nil); err != nil {
		return res, err
	}
	lockPath := filepath.Join(filepath.Dir(to), filepath.Base(to)+".moving.lock")
	lk, err := flock.TryLock(lockPath, d.cfg.filePerm)
	if errors.Is(err, flock.ErrLocked) {
		return res, newErr(ErrBusy, "another process is moving %s", from)
	}
	if err != nil {
		return res, err
	}
	defer func() { _ = lk.Release() }() // closing the fd releases the lock; the file stays by design

	// Re-check under the lock: another process may have finished.
	if st, done := legacyState(from, to); done {
		res.Status = st
		return res, nil
	}
	locks := l.DaemonLocks
	if len(locks) == 0 {
		locks = []string{defaultDaemonLock}
	}
	for _, rel := range locks {
		held, err := flock.Held(filepath.Join(from, filepath.FromSlash(rel)))
		if err != nil {
			return res, err
		}
		if held {
			return res, newErr(ErrBusy, "old home %s is in use by a running daemon; will retry next run", from)
		}
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}

	tmp := to + ".moving-tmp"
	marker := filepath.Join(to, legacyMarkerFile)
	switch {
	case exists(marker):
		// A crash after tmp -> new: finish the cross-disk steps below.
	case sameDevice(from, filepath.Dir(to)):
		if err := os.Rename(from, to); err != nil {
			return res, fmt.Errorf("move %s to %s: %w", from, to, err)
		}
		res.Status, res.Note = MoveDone, "renamed (same disk)"
		return res, d.leaveMovedNote(from, to)
	default:
		if err := os.RemoveAll(tmp); err != nil { // our own partial copy from a crashed run
			return res, err
		}
		n, size, err := copyTree(ctx, from, tmp)
		if err != nil {
			return res, fmt.Errorf("copy %s to %s: %w", from, tmp, err)
		}
		if n2, size2, err := countTree(tmp); err != nil || n2 != n || size2 != size {
			return res, fmt.Errorf("verify copy of %s: %d files/%d bytes, copy has %d/%d (%v); old folder left as is", from, n, size, n2, size2, err)
		}
		if d.cfg.legacyVerify != nil {
			if err := d.cfg.legacyVerify(from, tmp); err != nil {
				return res, fmt.Errorf("verify copy of %s: %w", from, err)
			}
		}
		if err := os.WriteFile(filepath.Join(tmp, legacyMarkerFile), []byte(from+"\n"), d.cfg.filePerm); err != nil {
			return res, err
		}
		if err := os.Rename(tmp, to); err != nil {
			return res, fmt.Errorf("move %s to %s: %w", tmp, to, err)
		}
	}
	if isDir(from) && !onlyMovedNote(from) {
		moved := from + ".moved-" + time.Now().UTC().Format("20060102")
		if err := os.Rename(from, moved); err != nil {
			return res, fmt.Errorf("rename %s to %s: %w", from, moved, err)
		}
	}
	if err := os.Remove(marker); err != nil && !isNotExist(err) {
		return res, err
	}
	res.Status, res.Note = MoveDone, "copied (other disk); old folder kept as "+from+".moved-<date>"
	return res, d.leaveMovedNote(from, to)
}

// unfinishedMoveFrom reports a cross-disk move from `from` that crashed after
// the old folder was renamed to <old>.moved-<date> but before .legacy-move
// was removed: old is gone, so without this check MoveLegacy would return
// MoveNone forever, leaving the marker in the new home and no MOVED.txt.
// The marker holds the old path, so only that move is finished.
func unfinishedMoveFrom(to, from string) bool {
	b, err := os.ReadFile(filepath.Join(to, legacyMarkerFile))
	return err == nil && strings.TrimSpace(string(b)) == from
}

// legacyState decides the cases that need no move.
func legacyState(from, to string) (MoveStatus, bool) {
	switch {
	case onlyMovedNote(from):
		return MoveAlready, true
	case exists(filepath.Join(to, legacyMarkerFile)):
		return "", false // unfinished cross-disk move
	case isDir(to):
		return MoveBothExist, true
	}
	return "", false
}

// onlyMovedNote reports an old folder that holds nothing but MOVED.txt.
func onlyMovedNote(dir string) bool {
	ents, err := os.ReadDir(dir)
	return err == nil && len(ents) == 1 && ents[0].Name() == movedFile
}

// leaveMovedNote recreates the old folder holding only MOVED.txt.
func (d *Dirs) leaveMovedNote(from, to string) error {
	if _, err := d.mkdirAll(from); err != nil {
		return fmt.Errorf("create %s: %w", from, err)
	}
	note := "moved to " + to + "\n"
	return os.WriteFile(filepath.Join(from, movedFile), []byte(note), d.cfg.filePerm)
}

// sameDevice compares device ids; unknown means "not the same" so the safe
// copy path runs.
func sameDevice(a, b string) bool {
	da, ok1 := deviceID(a)
	db, ok2 := deviceID(b)
	return ok1 && ok2 && da == db
}

// copyTree copies files, folders and symlinks (as links), keeping modes;
// sockets and pipes are live state and are skipped. Returns files and bytes.
// src itself is followed when it is a symlink (an old home linked to another
// disk): otherwise WalkDir would copy the link alone and the new home would
// point back at the old disk with nothing moved. Links inside the tree are
// still copied as links.
func copyTree(ctx context.Context, src, dst string) (int, int64, error) {
	if real, err := filepath.EvalSymlinks(src); err == nil {
		src = real
	}
	var n int
	var size int64
	err := filepath.WalkDir(src, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		info, err := de.Info()
		if err != nil {
			return err
		}
		switch {
		case de.IsDir():
			return os.MkdirAll(out, info.Mode().Perm()|0o700)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			n++
			return os.Symlink(target, out)
		case info.Mode().IsRegular():
			n++
			size += info.Size()
			return copyFile(p, out, info.Mode().Perm())
		}
		return nil // socket, pipe, device: skipped
	})
	return n, size, err
}

// countTree counts what copyTree would copy.
func countTree(root string) (int, int64, error) {
	var n int
	var size int64
	err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		info, err := de.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().IsRegular() {
			n++
			if info.Mode().IsRegular() && !strings.HasSuffix(p, legacyMarkerFile) {
				size += info.Size()
			}
		}
		return nil
	})
	return n, size, err
}

// copyFile copies one regular file and fsyncs it.
func copyFile(src, dst string, perm os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }() // read side
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
