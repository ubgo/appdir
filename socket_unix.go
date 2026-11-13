//go:build unix

package appdir

import (
	"path/filepath"
	"runtime"
)

// unixSocketTmp is the literal /tmp the socket fallback uses on Unix. It is
// deliberately not os.TempDir(): macOS's $TMPDIR is long enough to defeat the
// purpose of a short socket path.
const unixSocketTmp = "/tmp"

// platformSocket keeps run/<app>.sock when it fits, else moves the socket to
// $XDG_RUNTIME_DIR/<name>/ on Linux or /tmp/<name>-<uid>/ (literally /tmp:
// macOS's $TMPDIR is long enough to defeat the purpose).
// ds:def id=dirs-platformsocket-5kfqqv9a owner=@khanakia stability=stable
func (d *Dirs) platformSocket(s Socket, hash string) Socket {
	if len(s.Path) <= d.cfg.socketMaxLen {
		return s
	}
	name := d.Name(d.cfg.app)
	dir := d.sharedSocketDir()
	if runtime.GOOS == "linux" {
		if xdg, ok := d.cfg.lookupEnv("XDG_RUNTIME_DIR"); ok && filepath.IsAbs(xdg) && isDir(xdg) {
			dir = filepath.Join(xdg, name)
		}
	}
	s.Dir, s.Fallback = dir, true
	s.Path = filepath.Join(dir, hash+".sock")
	return s
}

// sharedSocketDir is the literal /tmp/<Name(app)>-<uid> socket fallback
// folder. It is shared by every run of the app for this user (a live daemon's
// socket sits there), so Socket's test guard refuses it even when the default
// test root (os.TempDir()) is /tmp itself (finding L1).
func (d *Dirs) sharedSocketDir() string {
	return filepath.Join(unixSocketTmp, d.Name(d.cfg.app)+"-"+currentUser())
}
