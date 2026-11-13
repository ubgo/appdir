# Testing setup

A test that resolves paths the normal way reads your real `HOME` and your shell's `<APP>_HOME`, and can write into your real data. appdir gives you two ways to keep tests out: seams (`WithEnv`, `WithHomeDir`, `WithWorkDir`, `WithBuildInfo`) that replace every outside input of `New`, and `ForTest(t, app)`, which points the whole process environment at a temp root. A guard backs both up: in a test binary, `New` panics when it resolves any path outside the test root.

## Before you start

- [First use](../start/first-use.md): `New` and `Ensure`.
- Import `"github.com/ubgo/appdir"` and `"testing"`.
- The example is a tiny tool, `demo`, with one function that resolves paths (`openDirs`) and one that writes a file (`saveNote`). Its `main.go` drives it with seams; its `main_test.go` shows `ForTest` and the guard.

## Step 1 — Keep one place that resolves paths

The real binary calls `openDirs()` with no options, so it uses the process environment, the real home and Go's build info. Tests and examples pass options to replace them.

<!-- snippet: examples/testing-setup/main.go#tool -->
```go
// openDirs is the tool's one place that resolves its paths. The real binary
// calls it with no options (process env, real home, real build info); tests
// and this example pass seams. WithMode(ModeGlobal): the db lives in the
// global home, so the tool works outside a project.
func openDirs(opts ...appdir.Option) (*appdir.Dirs, error) {
	base := []appdir.Option{appdir.WithMode(appdir.ModeGlobal)}
	return appdir.New(appName, append(base, opts...)...)
}

// saveNote is the tool's real work: it creates the db folder on first use
// and writes a file there.
func saveNote(d *appdir.Dirs, text string) (string, error) {
	p, err := d.Ensure(appdir.KindDB)
	if err != nil {
		return "", fmt.Errorf("ensure db: %w", err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	return p, nil
}
```

## Step 2 — Replace every outside input with seams

`New` reads four things from the machine: the environment, the user's home folder, the working folder and Go's build info. `WithEnv(lookup)`, `WithHomeDir(fn)`, `WithWorkDir(dir)` and `WithBuildInfo(fn)` replace each one. With all four, a call is a pure function of its options: no `os.Setenv`, safe in parallel tests.

<!-- snippet: examples/testing-setup/main.go#seams -->
```go
// run drives the tool with every outside input replaced: the user's home
// (WithHomeDir), the working folder (WithWorkDir), the environment (WithEnv)
// and the build info that decides release or dev (WithBuildInfo).
func run(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	home := filepath.Join(root, homeDir)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create home: %w", err)
	}
	d, err := openDirs(
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(home),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithBuildInfo(func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: fakeVersion}}, true
		}),
	)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	fmt.Fprintf(w, "profile %s (%s)\n", d.Profile(), d.ReleaseReason())
	p, err := saveNote(d, noteText)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "saved %s\n", show(root, p))
	return nil
}
```

## Step 3 — Or isolate the whole process with ForTest

When the code under test calls `New` without seams (or starts other code that reads the environment), call `ForTest(t, app)` first ([ForTest](ds:block?id=fortest-56v4yh8s)). It makes a temp root and, with `t.Setenv`, points into it `HOME`, `USERPROFILE`, `<APP>_HOME` (`<root>/apphome`), `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`, `XDG_RUNTIME_DIR`, `TMPDIR`, and the agent config folder variables the standard lists (see [SPEC 10](../SPEC.md#10-tests)). It unsets every other `<APP>_*` variable from your shell (`<APP>_PROFILE`, `<APP>_ALLOW_PROD`, `<APP>_DAEMON_PROFILE`, `<APP>_CEILING_DIRS`, `<APP>_<KIND>_DIR`, ...), `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_COMMON_DIR`, `GIT_CEILING_DIRECTORIES` and every `GIT_CONFIG*` variable, sets `APPDIR_TEST_ROOT` to the root, and returns the root. Everything is restored when the test ends.

<!-- snippet: examples/testing-setup/main_test.go#fortest -->
```go
// TestForTest runs the tool exactly as the real binary does (no seams):
// ForTest has already pointed HOME, DEMO_HOME, the XDG folders, TMPDIR and
// the agent config folders into a temp root, and removed DEMO_PROFILE and the
// GIT_* variables, so every path lands in that root. ForTest uses t.Setenv,
// so this test cannot call t.Parallel.
func TestForTest(t *testing.T) {
	root := appdir.ForTest(t, appName)

	d, err := openDirs()
	if err != nil {
		t.Fatal(err)
	}
	p, err := saveNote(d, noteText)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "apphome", appName+".db"); p != want {
		t.Errorf("db = %s, want %s (DEMO_HOME from ForTest)", p, want)
	}
	if home := os.Getenv("HOME"); home != filepath.Join(root, "home") {
		t.Errorf("HOME = %s, want it inside the test root", home)
	}
}
```

## Step 4 — Let the guard catch a forgotten isolation

In a test binary (`testing.Testing()`), `New` panics when any resolved home or path is outside `APPDIR_TEST_ROOT`, or outside `os.TempDir()` when that variable is unset ([guardTestPaths](ds:block?id=dirs-guardtestpaths-2wuv39pv)). A panic, not an error, so a test that forgot `ForTest` stops before it writes into your notes. `Socket()` and `EnsureSocketDir()` check the socket fallback folder (`/tmp/<app>-<uid>/` or `$XDG_RUNTIME_DIR/<app>/`, decided after `New`) the same way, so a daemon test with a long project path panics instead of creating a folder in the real `/tmp`. That holds under the default root too: with `TMPDIR` unset `os.TempDir()` is `/tmp` itself, and the shared `/tmp/<app>-<uid>/` is still refused because a live daemon's socket sits there. The example hands `New` a home outside every temp folder and recovers the panic to show its message:

<!-- snippet: examples/testing-setup/main_test.go#guard -->
```go
// TestGuardPanics forgets ForTest and hands New a home outside the test
// root. In a test binary New panics before anything can be written there.
func TestGuardPanics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("outsideHome is a Unix path")
	}
	msg := func() (msg string) {
		defer func() { msg = fmt.Sprint(recover()) }()
		_, _ = openDirs(
			appdir.WithHomeDir(func() (string, error) { return outsideHome, nil }),
			appdir.WithWorkDir(t.TempDir()),
			appdir.WithEnv(func(string) (string, bool) { return "", false }),
			appdir.WithRelease(false),
		)
		return ""
	}()
	want := "appdir: test binary resolved " + outsideHome + "/.demo-dev outside the test root"
	if !strings.HasPrefix(msg, want) {
		t.Fatalf("panic = %q, want prefix %q", msg, want)
	}
	t.Log(msg)
}
```

## Step 5 — Turn the guard off where you must

`WithTestGuard(false)` turns the guard off for one `New` call: for a test that must resolve a real-looking path and never writes there.

<!-- snippet: examples/testing-setup/main_test.go#guardoff -->
```go
// TestGuardOff turns the guard off for a test that must resolve a real-looking
// path (and never writes there): WithTestGuard(false).
func TestGuardOff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("outsideHome is a Unix path")
	}
	d, err := openDirs(
		appdir.WithHomeDir(func() (string, error) { return outsideHome, nil }),
		appdir.WithWorkDir(t.TempDir()),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(false),
		appdir.WithTestGuard(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	p, err := d.Path(appdir.KindDB)
	if err != nil {
		t.Fatal(err)
	}
	if want := outsideHome + "/.demo-dev/demo.db"; p != want {
		t.Errorf("db = %s, want %s", p, want)
	}
}
```

## Try it

```sh
go run ./examples/testing-setup
go test -v ./examples/testing-setup
```

```text
profile prod (go-install)
saved home/.demo/demo.db
```

```text
=== RUN   TestRun
--- PASS: TestRun (0.00s)
=== RUN   TestForTest
--- PASS: TestForTest (0.00s)
=== RUN   TestGuardPanics
    main_test.go:83: appdir: test binary resolved /appdir-guard-example/user/.demo-dev outside the test root $TMP; call appdir.ForTest(t, "demo") or pass WithTestGuard(false)
--- PASS: TestGuardPanics (0.00s)
=== RUN   TestGuardOff
--- PASS: TestGuardOff (0.00s)
PASS
ok  	github.com/ubgo/appdir/examples/testing-setup	0.008s
```

`$TMP` stands for your temp folder (`os.TempDir()`); the test does not call `ForTest`, so that is the root the guard uses.

## Pitfalls

- `ForTest` uses `t.Setenv`, so a test that calls it cannot call `t.Parallel()`. Use seams in parallel tests.
- Under `ForTest`, `<APP>_HOME` is set, so every kind resolves into `<root>/apphome` as one home (the override rule); there is no project lookup. A test of project-home behaviour should use `WithWorkDir` and `WithHomeDir` instead, or unset `<APP>_HOME` with `t.Setenv`.
- A test binary is a dev build, so homes get the `-dev` suffix (`.demo-dev`). Pass `WithRelease(true)` or a fake `WithBuildInfo` when a test needs prod names.
- Without `ForTest`, the guard's root is `os.TempDir()`. A test that uses seams pointing into `t.TempDir()` passes the guard; one that points at a fixture folder in the repo panics, unless it sets `APPDIR_TEST_ROOT` or passes `WithTestGuard(false)`.
- The guard checks homes and `Paths()`. It does not check the socket fallback folder (`/tmp/<name>-<uid>/`); do not call `EnsureSocketDir` in tests without pointing `XDG_RUNTIME_DIR` at a temp folder (Linux) or keeping the path short.
- `CheckDB` resolves a prod view internally with the guard off; that view is only compared, never written.

## Related

- [Release detection](../profiles/release-detection.md): `WithBuildInfo`.
- [Options reference](../reference/options.md): every seam.
- Example: [examples/testing-setup](../../examples/testing-setup).
- Design: [SPEC 10 Tests](../SPEC.md#10-tests).
