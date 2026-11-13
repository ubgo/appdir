// Command daemon-paths shows the paths a tool with a background daemon needs:
// the files in run/, the socket (with its fallback when the path is too long
// for a Unix socket), and how a client finds the daemon of another profile.
//
// Everything happens inside a temp root: the user's home is root/home (passed
// with WithHomeDir) and the environment is a map (WithEnv). The socket
// fallback is pointed at root/xdg-run through XDG_RUNTIME_DIR, so the
// example never creates anything in the real /tmp.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example.
const (
	appName    = "demo"
	homeDir    = "home"
	workDir    = "home/work"
	runtimeDir = "xdg-run" // stands in for /run/user/<uid>

	envRuntimeDir    = "XDG_RUNTIME_DIR"
	envDaemonProfile = "DEMO_DAEMON_PROFILE"

	pidFileName   = "demo-daemon.pid" // WithRunFile renames run/demo.pid
	shortMaxLen   = 20                // forces the fallback: every real path is longer
	unknownDaemon = "qa"
)

// fakeEnv is the environment WithEnv reads instead of the process env.
type fakeEnv map[string]string

// lookup has the signature WithEnv wants; a nil map is an empty environment.
func (e fakeEnv) lookup(k string) (string, bool) {
	v, ok := e[k]
	return v, ok
}

// sandbox holds the folders every case resolves against.
type sandbox struct {
	root, home, work string
}

func main() {
	root, err := os.MkdirTemp("", "appdir-daemon-paths-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "daemon-paths:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "daemon-paths:", err)
		os.Exit(1)
	}
}

// run does the whole walk-through inside root and prints paths relative to it.
func run(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	s := sandbox{
		root: root,
		home: filepath.Join(root, filepath.FromSlash(homeDir)),
		work: filepath.Join(root, filepath.FromSlash(workDir)),
	}
	if err := os.MkdirAll(s.work, 0o700); err != nil {
		return fmt.Errorf("create work folder: %w", err)
	}
	steps := []struct {
		title string
		fn    func(io.Writer, sandbox) error
	}{
		{"run files", runFiles},
		{"socket in run/", socketInRun},
		{"socket fallback (path too long)", socketFallback},
		{"daemon profile", daemonProfile},
	}
	for _, st := range steps {
		fmt.Fprintln(w, "==", st.title)
		if err := st.fn(w, s); err != nil {
			return fmt.Errorf("%s: %w", st.title, err)
		}
	}
	return nil
}

// snippet:open
// open resolves a daemon tool. WithMode(ModeGlobal) puts run/ in the global
// home: run/ follows the mode's data scope, and a daemon serves the whole
// machine, not one project.
func (s sandbox) open(env fakeEnv, opts ...appdir.Option) (*appdir.Dirs, error) {
	base := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return s.home, nil }),
		appdir.WithWorkDir(s.work),
		appdir.WithEnv(env.lookup),
		appdir.WithMode(appdir.ModeGlobal),
	}
	return appdir.New(appName, append(base, opts...)...)
}

// snippet:end

// snippet:runfiles
// runFiles lists every file in run/. They are names only: appdir creates
// none of them; the daemon takes the locks and writes the files.
func runFiles(w io.Writer, s sandbox) error {
	d, err := s.open(nil, appdir.WithRelease(true), appdir.WithRunFile(appdir.RunPID, pidFileName))
	if err != nil {
		return err
	}
	for _, f := range []appdir.RunFile{
		appdir.RunLock, appdir.RunDaemonLock, appdir.RunPID,
		appdir.RunDiscovery, appdir.RunSocket, appdir.RunSocketFile,
	} {
		p, err := d.RunPath(f)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "RunPath(%s): %s\n", f, s.show(p))
	}
	return nil
}

// snippet:end

// snippet:socket
// socketInRun asks for the socket with the default limit (100 bytes): the
// path fits, so it stays in run/. EnsureSocketDir creates run/ 0700, on
// every OS: a Windows named pipe has no folder (sock.Dir is ""), but the
// daemon still writes run/socket, its pid and discovery.json in run/.
func socketInRun(w io.Writer, s sandbox) error {
	d, err := s.open(nil, appdir.WithRelease(true))
	if err != nil {
		return err
	}
	sock, err := d.EnsureSocketDir()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Socket: %s fallback=%v\n", s.show(sock.Path), sock.Fallback)
	run, err := d.Path(appdir.KindRun)
	if err != nil {
		return err
	}
	return printMode(w, s, run)
}

// snippet:end

// snippet:fallback
// socketFallback sets a tiny WithSocketMaxLen so the socket path is "too
// long", as it is in a deep project folder. On Linux the socket moves to
// $XDG_RUNTIME_DIR/<name>/<hash>.sock (else /tmp/<name>-<uid>/); the daemon
// writes that path to run/socket and clients read it from there.
func socketFallback(w io.Writer, s sandbox) error {
	xdg := filepath.Join(s.root, runtimeDir)
	if err := os.MkdirAll(xdg, 0o700); err != nil {
		return err
	}
	// WithTestGuard(false): on macOS the fallback is the real /tmp, outside
	// the test root, and merely resolving it would panic in a test binary.
	// This function never creates a folder outside its root (see below).
	d, err := s.open(fakeEnv{envRuntimeDir: xdg}, appdir.WithRelease(true), appdir.WithSocketMaxLen(shortMaxLen),
		appdir.WithTestGuard(false))
	if err != nil {
		return err
	}
	sock, err := d.Socket()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Socket: %s fallback=%v\n", s.show(sock.Path), sock.Fallback)
	fmt.Fprintf(w, "PointerFile: %s\n", s.show(sock.PointerFile))
	if sock.Dir == "" || !within(sock.Dir, s.root) {
		// A Windows pipe has no folder, and on macOS the fallback is the
		// real /tmp: this example creates nothing outside its root.
		fmt.Fprintln(w, "fallback folder is outside the example root; not created")
		return nil
	}
	// The fallback folder (0700, owner and symlink checks) and run/, where
	// the pointer file goes.
	if _, err := d.EnsureSocketDir(); err != nil {
		return err
	}
	if err := printMode(w, s, sock.Dir); err != nil {
		return err
	}
	// The daemon's side: record where it listens.
	if err := os.WriteFile(sock.PointerFile, []byte(sock.Path+"\n"), 0o600); err != nil {
		return err
	}
	// The client's side: read run/socket, connect there.
	b, err := os.ReadFile(sock.PointerFile)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "client reads run/socket: %s\n", s.show(strings.TrimSpace(string(b))))
	return nil
}

// snippet:end

// snippet:daemon
// daemonProfile shows how a dev client finds a daemon: its own by default,
// another profile's with --daemon-profile (WithDaemonProfile) or
// DEMO_DAEMON_PROFILE. DaemonDiscovery returns that profile's discovery
// file; the client reads only that file and the socket it names.
func daemonProfile(w io.Writer, s sandbox) error {
	dev := appdir.WithRelease(false)
	cases := []struct {
		label string
		env   fakeEnv
		opts  []appdir.Option
	}{
		{"dev client", nil, []appdir.Option{dev}},
		{"dev client, --daemon-profile prod", nil, []appdir.Option{dev, appdir.WithDaemonProfile(string(appdir.ProfileProd))}},
		{"dev client, DEMO_DAEMON_PROFILE=prod", fakeEnv{envDaemonProfile: string(appdir.ProfileProd)}, []appdir.Option{dev}},
		{"dev client, --daemon-profile qa", nil, []appdir.Option{dev, appdir.WithDaemonProfile(unknownDaemon)}},
	}
	for _, c := range cases {
		d, err := s.open(c.env, c.opts...)
		if errors.Is(err, appdir.ErrUnknownProfile) {
			fmt.Fprintf(w, "%-38s -> error: %v\n", c.label, err)
			continue
		}
		if err != nil {
			return err
		}
		path, p, err := d.DaemonDiscovery()
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-38s -> DaemonProfile %s, discovery %s\n", c.label, d.DaemonProfile(), s.show(path))
		if p != d.Profile() {
			// The standard asks both sides to print one line when a client
			// talks to another profile's daemon (after the protocol
			// handshake, which is the tool's job, not appdir's).
			fmt.Fprintf(w, "%-38s    [%s] %s client → %s daemon\n", "", appName, d.Profile(), p)
		}
	}
	return nil
}

// snippet:end

// printMode prints a folder's permission bits (0700 on Unix).
func printMode(w io.Writer, s sandbox, dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "created %s mode %04o\n", s.show(dir), fi.Mode().Perm())
	return nil
}

// within reports whether p is base or inside it.
func within(p, base string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// show prints a path relative to root, so the output is the same on every run.
func (s sandbox) show(p string) string {
	if rel, err := filepath.Rel(s.root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}
