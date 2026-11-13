# Run files and sockets

A tool with a background daemon needs a few well-known files: a lock, the daemon's lock, a pid file, a discovery file, and the socket clients connect to. appdir gives their paths in `run/`, moves the socket somewhere shorter when the path is too long for a Unix socket, and tells a client which profile's daemon to talk to. It only computes paths: your daemon takes the locks, writes the files and listens on the socket.

## Before you start

- [First use](../start/first-use.md): `New`, `Path`, `Ensure`.
- [Profiles](../profiles/profiles.md): each profile has its own `run/` and its own daemon.
- Import `"github.com/ubgo/appdir"`.
- `run` is a data kind, so it follows the mode: in a project tool it is in the project home, in a global tool (`WithMode(appdir.ModeGlobal)`) in `~/.<app>/run/`. The example is a global tool.

## Step 1 — Resolve a daemon tool

The example's helper adds `WithMode(appdir.ModeGlobal)` to the usual seams: one daemon serves the whole machine.

<!-- snippet: examples/daemon-paths/main.go#open -->
```go
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
```

## Step 2 — Get the run files

`RunPath(f)` returns a file in `run/`: `RunLock` (`lock`, for migrations and layout changes), `RunDaemonLock` (`daemon.lock`, held for the daemon's life), `RunPID` (`<app>.pid`, display only), `RunDiscovery` (`discovery.json`), `RunSocket` (`<app>.sock`) and `RunSocketFile` (`socket`, the path of a fallback socket). `WithRunFile(f, name)` renames one; the name must be one path segment.

<!-- snippet: examples/daemon-paths/main.go#runfiles -->
```go
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
```

## Step 3 — Get the socket and create its folder

`Socket()` returns where the daemon listens ([Socket](ds:block?id=dirs-socket-brsjjca9)). When `run/<app>.sock` is at most `WithSocketMaxLen` bytes (default 100; macOS allows about 104), that is the path and `Fallback` is false. `EnsureSocketDir()` creates `run/` through `Ensure` (0700) in every case, because the daemon writes `run/socket`, its pid and `discovery.json` there, and, when the socket falls back, the fallback folder too ([EnsureSocketDir](ds:block?id=dirs-ensuresocketdir-r9rr28ym)).

<!-- snippet: examples/daemon-paths/main.go#socket -->
```go
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
```

## Step 4 — Handle the fallback for long paths

A deep project folder makes `run/<app>.sock` too long. Then the socket moves to `<dir>/<hash>.sock`, where `<dir>` is `$XDG_RUNTIME_DIR/<Name(app)>/` on Linux when that variable names an existing absolute folder, else `/tmp/<Name(app)>-<uid>/` (literally `/tmp`: the macOS `$TMPDIR` is too long to help), and `<hash>` is the first 12 hex characters of the SHA-256 of the real path of `run/`, so each home and profile gets its own socket ([platformSocket](ds:block?id=dirs-platformsocket-5kfqqv9a)). `Fallback` is true; the daemon writes the path to `PointerFile` (`run/socket`) and clients read it from there. `EnsureSocketDir` creates the fallback folder 0700 and refuses it with `ErrUnsafeTemp` when it is a symlink or owned by another user.

On Windows the socket is always a named pipe, `\\.\pipe\<Name(app)>-<user>-<hash>`, with `Fallback` true and an empty `Dir` ([platformSocket on Windows](ds:block?id=dirs-platformsocket-cj45m675)). The daemon still writes the pipe name to `run/socket`, so clients find it the same way on every OS.

The example forces the fallback with a tiny `WithSocketMaxLen` and points `XDG_RUNTIME_DIR` at a folder inside its temp root through `WithEnv`:

<!-- snippet: examples/daemon-paths/main.go#fallback -->
```go
// socketFallback sets a tiny WithSocketMaxLen so the socket path is "too
// long", as it is in a deep project folder. On Linux the socket moves to
// $XDG_RUNTIME_DIR/<name>/<hash>.sock (else /tmp/<name>-<uid>/); the daemon
// writes that path to run/socket and clients read it from there.
func socketFallback(w io.Writer, s sandbox) error {
	xdg := filepath.Join(s.root, runtimeDir)
	if err := os.MkdirAll(xdg, 0o700); err != nil {
		return err
	}
	d, err := s.open(fakeEnv{envRuntimeDir: xdg}, appdir.WithRelease(true), appdir.WithSocketMaxLen(shortMaxLen))
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
```

## Step 5 — Find the daemon of another profile

`DaemonProfile()` is the profile whose daemon a client talks to: `WithDaemonProfile` (your `--daemon-profile` flag), then `<APP>_DAEMON_PROFILE`, then the client's own profile. An unknown name is `ErrUnknownProfile` from `New`. `DaemonDiscovery()` returns that profile's discovery file, computed with `ForProfile` ([DaemonDiscovery](ds:block?id=dirs-daemondiscovery-wb9urrua)). The client reads only that file and the socket it names, never the other profile's database. The protocol handshake and the one line both sides print are your tool's job.

<!-- snippet: examples/daemon-paths/main.go#daemon -->
```go
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
```

## Try it

```sh
go run ./examples/daemon-paths
```

On Linux (the hash differs on every run, because the temp root differs):

```text
== run files
RunPath(lock): home/.demo/run/lock
RunPath(daemon-lock): home/.demo/run/daemon.lock
RunPath(pid): home/.demo/run/demo-daemon.pid
RunPath(discovery): home/.demo/run/discovery.json
RunPath(socket): home/.demo/run/demo.sock
RunPath(socket-file): home/.demo/run/socket
== socket in run/
Socket: home/.demo/run/demo.sock fallback=false
created home/.demo/run mode 0700
== socket fallback (path too long)
Socket: xdg-run/demo/23b257995ba6.sock fallback=true
PointerFile: home/.demo/run/socket
created xdg-run/demo mode 0700
client reads run/socket: xdg-run/demo/23b257995ba6.sock
== daemon profile
dev client                             -> DaemonProfile dev, discovery home/.demo-dev/run/discovery.json
dev client, --daemon-profile prod      -> DaemonProfile prod, discovery home/.demo/run/discovery.json
                                          [demo] dev client → prod daemon
dev client, DEMO_DAEMON_PROFILE=prod   -> DaemonProfile prod, discovery home/.demo/run/discovery.json
                                          [demo] dev client → prod daemon
dev client, --daemon-profile qa        -> error: appdir: --daemon-profile: unknown profile "qa" (known: prod, dev)
```

On macOS the fallback is `/tmp/demo-<uid>/<hash>.sock` and the example does not create it; on Windows the socket is a pipe and the example prints `fallback folder is outside the example root; not created`.

## Pitfalls

- Windows: a pipe has no folder, so `Socket().Dir` is empty; do not create or check it. `EnsureSocketDir` still creates `run/`, where the daemon writes `PointerFile`.
- Always write `PointerFile` when `Fallback` is true, and make clients read it: the fallback path depends on the real path of `run/` and on the profile, so a client cannot guess it.
- `/tmp/<name>-<uid>/` is shared space. Create it only through `EnsureSocketDir`, which checks the owner and refuses a symlink; never `os.MkdirAll` it yourself.
- `run/` lives where the mode puts data. A project-mode tool outside a project gets `ErrNoProject` from `RunPath` and `Socket`; give daemons `WithMode(appdir.ModeGlobal)` or keep `run` global with `WithKind(appdir.KindRun, appdir.Scoped(appdir.ScopeGlobal))` ([Kinds](../layout/kinds.md)).
- `Name(app)` is in the fallback folder and the pipe name, so the dev daemon and the prod daemon never share a socket.
- appdir creates none of the run files. The locks are yours to take (and never delete: unlinking a lock file races with a waiter).

## Related

- [Legacy move](legacy-move.md): probes the old daemon's `run/daemon.lock`.
- [Child processes and env leaks](../profiles/child-processes.md): `ForProfile`.
- [paths --json](../reference/paths-json.md): the `Socket` fields in JSON.
- Example: [examples/daemon-paths](../../examples/daemon-paths).
- Design: [SPEC 7 Run paths](../SPEC.md#7-run-paths-section-9-paths-only).
