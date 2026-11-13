package appdir

// Regression tests for two fixes the coordinator made during the FINALIZE
// pass: X2 (a walk that starts inside a ceiling dir must not climb above it)
// and X3 (the test guard also covers the socket's fallback folder). Helpers
// carry the cx prefix so they never collide with other test files.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// cxApp is the app name every test here resolves.
const cxApp = "cxapp"

// TestFixX2_WalkStopsAtCeilingStart: like git's GIT_CEILING_DIRECTORIES, a
// ceiling is never climbed past, even when the walk starts in the ceiling
// folder itself; otherwise a .<app>/ above it would be picked up.
func TestFixX2_WalkStopsAtCeilingStart(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	projects := filepath.Join(root, "srv", "projects")
	ceiling := filepath.Join(projects, "notes")
	rxMkdir(t, home, ceiling, filepath.Join(projects, "."+cxApp))
	env := map[string]string{"CXAPP_CEILING_DIRS": ceiling}

	d, err := New(cxApp, rxHome(home), WithWorkDir(ceiling), rxEnv(env), WithRelease(true))
	if err != nil {
		t.Fatal(err)
	}
	if d.HasProject() {
		p, _ := d.Project()
		t.Fatalf("walk climbed past the ceiling %s and found %s", ceiling, p.Marker)
	}
	if _, err := d.Path(KindDB); !errors.Is(err, ErrNoProject) {
		t.Fatalf("Path(db) err = %v, want ErrNoProject", err)
	}

	// Control: without the ceiling the same walk finds the home above.
	d, err = New(cxApp, rxHome(home), WithWorkDir(ceiling), rxEnv(nil), WithRelease(true))
	if err != nil {
		t.Fatal(err)
	}
	if !d.HasProject() {
		t.Fatal("control: without a ceiling the walk should find the project above")
	}
}

// TestFixX3_GuardCoversSocketFallback: in a test binary a socket that falls
// back outside the test root (the literal /tmp folder) is refused with a
// panic before anything can create it, the same as every other path.
func TestFixX3_GuardCoversSocketFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows named pipe has no fallback folder to guard")
	}
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	// No XDG_RUNTIME_DIR and a tiny max length: the fallback is /tmp/...
	d, err := New(cxApp, rxHome(home), WithWorkDir(home), rxEnv(nil),
		WithRelease(true), WithMode(ModeGlobal), WithSocketMaxLen(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		name string
		fn   func() (Socket, error)
	}{{"Socket", d.Socket}, {"EnsureSocketDir", d.EnsureSocketDir}} {
		t.Run(call.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("%s: no panic for a fallback outside the test root", call.name)
				}
				if !strings.Contains(r.(string), "outside the test root") {
					t.Fatalf("%s: panic %v", call.name, r)
				}
			}()
			s, _ := call.fn()
			t.Logf("got %+v", s)
		})
	}

	// With the fallback inside the root (XDG_RUNTIME_DIR, Linux only),
	// nothing panics. macOS has no such variable: its fallback is always
	// the literal /tmp.
	if runtime.GOOS != "linux" {
		return
	}
	xdg := filepath.Join(root, "xdg")
	rxMkdir(t, xdg)
	d, err = New(cxApp, rxHome(home), WithWorkDir(home), rxEnv(map[string]string{"XDG_RUNTIME_DIR": xdg}),
		WithRelease(true), WithMode(ModeGlobal), WithSocketMaxLen(1))
	if err != nil {
		t.Fatal(err)
	}
	if s, err := d.Socket(); err != nil || (s.Fallback && !strings.HasPrefix(s.Dir, root) && s.Dir != "") {
		t.Fatalf("Socket() = %+v, %v; want a fallback inside the root", s, err)
	}
}

// TestFixTL4_InitHomeKeepsUnreadableHomeFile: init must never replace a
// home.jsonc it cannot read; that file is the user's, and replacing it would
// silently turn another app's (or a hand-edited) home into this one.
func TestFixTL4_InitHomeKeepsUnreadableHomeFile(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	h := filepath.Join(root, "h")
	rxMkdir(t, home, h)
	const broken = "{ this is not json"
	rxWrite(t, filepath.Join(h, "home.jsonc"), broken)
	d, err := New(cxApp, rxHome(home), WithWorkDir(root), rxEnv(nil), WithRelease(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.InitHome(h, ScopeProject); err == nil {
		t.Fatal("InitHome on an unreadable home.jsonc: no error, want one")
	}
	b, err := os.ReadFile(filepath.Join(h, "home.jsonc"))
	if err != nil || string(b) != broken {
		t.Fatalf("home.jsonc was changed: %q, %v", b, err)
	}
}

// TestFixTL5_InitWithoutHomeKind: a tool that drops the home kind still gets
// a folder from InitProject/InitHome, and nothing is written onto the folder
// path itself.
func TestFixTL5_InitWithoutHomeKind(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	proj := filepath.Join(root, "proj")
	rxMkdir(t, home, proj)
	d, err := New(cxApp, rxHome(home), WithWorkDir(proj), rxEnv(nil), WithRelease(true), WithoutKind(KindHome))
	if err != nil {
		t.Fatal(err)
	}
	m, err := d.InitProject(proj)
	if err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	if fi, err := os.Stat(m); err != nil || !fi.IsDir() {
		t.Fatalf("marker %s is not a folder: %v", m, err)
	}
	h := filepath.Join(root, "h")
	if err := d.InitHome(h, ScopeGlobal); err != nil {
		t.Fatalf("InitHome: %v", err)
	}
	if fi, err := os.Stat(h); err != nil || !fi.IsDir() {
		t.Fatalf("home %s is not a folder: %v", h, err)
	}
}

// TestFixT2_CheckDBRelativeUsesWorkDir: a relative db path (a tool's --db
// flag) is relative to the working folder New resolved from, like every other
// user path, not to the process cwd; otherwise `tool -C dir --db x.db`
// would guard a different file than the one the tool opens.
func TestFixT2_CheckDBRelativeUsesWorkDir(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	prodHome := filepath.Join(home, "."+cxApp)
	rxMkdir(t, prodHome)
	// Dev build standing in the prod global home: the relative name must
	// resolve there and be refused as the prod db.
	d, err := New(cxApp, rxHome(home), WithWorkDir(prodHome), rxEnv(nil), WithRelease(false), WithMode(ModeGlobal))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CheckDB(cxApp + ".db"); !errors.Is(err, ErrProdDB) {
		t.Fatalf("CheckDB(relative prod db) err = %v, want ErrProdDB", err)
	}
}
