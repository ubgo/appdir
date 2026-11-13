package appdir

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// Socket is where a daemon listens (SPEC 7).
type Socket struct {
	Path        string `json:"path"`
	Fallback    bool   `json:"fallback"`     // not under run/; the daemon writes Path to PointerFile
	PointerFile string `json:"pointer_file"` // run/socket
	Dir         string `json:"dir"`          // folder to create; "" for a Windows pipe
}

// RunPath returns a file in run/ (reads only).
func (d *Dirs) RunPath(f RunFile) (string, error) {
	name, ok := d.cfg.runFiles[f]
	if !ok {
		return "", newErr(ErrInvalidOption, "appdir: unknown run file %q", f)
	}
	run, err := d.Path(KindRun)
	if err != nil {
		return "", err
	}
	return filepath.Join(run, name), nil
}

// Socket returns the socket path: run/<app>.sock when short enough, else a
// per-user fallback (platform rules in socket_<os>.go).
// ds:def id=dirs-socket-brsjjca9 owner=@khanakia stability=stable
func (d *Dirs) Socket() (Socket, error) {
	run, err := d.Path(KindRun)
	if err != nil {
		return Socket{}, err
	}
	s := Socket{
		Path:        filepath.Join(run, d.cfg.runFiles[RunSocket]),
		PointerFile: filepath.Join(run, d.cfg.runFiles[RunSocketFile]),
		Dir:         run,
	}
	sum := sha256.Sum256([]byte(realPath(run)))
	s = d.platformSocket(s, hex.EncodeToString(sum[:])[:socketHashLen])
	// The fallback folder is resolved here, after New, so New's test guard
	// never saw it: check it the same way before anyone can create it.
	if d.guardActive() {
		d.guardSocketDir(s.Dir)
	}
	return s, nil
}

// EnsureSocketDir creates what a daemon needs before it listens: run/
// through Ensure in every case, plus the private fallback folder (0700, with
// the symlink and owner checks) when the socket is not under run/.
// run/ is created even for a fallback or a Windows pipe because the daemon
// writes run/socket (the pointer file), the pid and discovery.json there;
// one code path for every OS keeps the Windows pipe case covered by the
// Unix tests.
// ds:def id=dirs-ensuresocketdir-r9rr28ym owner=@khanakia stability=stable
func (d *Dirs) EnsureSocketDir() (Socket, error) {
	s, err := d.Socket()
	if err != nil {
		return s, err
	}
	if _, err := d.Ensure(KindRun); err != nil {
		return s, err
	}
	if s.Fallback && s.Dir != "" {
		return s, d.ensurePrivateDir(s.Dir)
	}
	return s, nil
}

// DaemonProfile returns the profile whose daemon a client talks to:
// --daemon-profile, then <APP>_DAEMON_PROFILE, then its own profile.
func (d *Dirs) DaemonProfile() Profile { return d.daemonProfile }

// DaemonDiscovery returns the discovery file of the daemon profile. A client
// reads only this file and the socket it names, never the other profile's db.
// ds:def id=dirs-daemondiscovery-wb9urrua owner=@khanakia stability=stable
func (d *Dirs) DaemonDiscovery() (string, Profile, error) {
	p := d.daemonProfile
	if p == d.profile {
		path, err := d.RunPath(RunDiscovery)
		return path, p, err
	}
	other, err := d.ForProfile(p)
	if err != nil {
		return "", p, err
	}
	path, err := other.RunPath(RunDiscovery)
	return path, p, err
}
