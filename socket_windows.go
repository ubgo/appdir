//go:build windows

package appdir

// platformSocket returns the named pipe \\.\pipe\<name>-<user>-<hash>. Pipes
// have no length problem and no folder; the daemon still writes the name to
// run/socket so clients find it the same way on every OS.
// ds:def id=dirs-platformsocket-cj45m675 owner=@khanakia stability=stable
func (d *Dirs) platformSocket(s Socket, hash string) Socket {
	s.Path = `\\.\pipe\` + d.Name(d.cfg.app) + "-" + currentUser() + "-" + hash
	s.Fallback, s.Dir = true, ""
	return s
}

// sharedSocketDir is empty on Windows: a named pipe has no folder, so there is
// no shared fallback folder for the test guard to refuse.
func (d *Dirs) sharedSocketDir() string { return "" }
