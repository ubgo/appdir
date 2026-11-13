package appdir

// Regression tests for the resolution fixes of the FINALIZE pass (findings
// F2, F3, F5, F6, F7/F18, F11, F13, F14, F15, F17, X1). Every helper, type
// and const here carries the rx prefix so it never collides with another
// test file of package appdir.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rxApp is the app name every test here resolves.
const rxApp = "rxapp"

// rxMarker is the project marker folder name of rxApp.
const rxMarker = "." + rxApp

// rxEnv turns a map into a WithEnv lookup: a test never reads the process env.
func rxEnv(m map[string]string) Option {
	return WithEnv(func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	})
}

// rxHome is a WithHomeDir seam returning dir.
func rxHome(dir string) Option {
	return WithHomeDir(func() (string, error) { return dir, nil })
}

// rxNoHome is a WithHomeDir seam for a machine with no $HOME.
func rxNoHome() Option {
	return WithHomeDir(func() (string, error) { return "", errors.New("no home") })
}

// rxRoot returns a fresh temp root with symlinks resolved, so path checks
// compare real paths.
func rxRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	r, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// rxMkdir creates folders, failing the test on error.
func rxMkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

// rxWrite writes a file, creating its folder.
func rxWrite(t *testing.T, path, body string) {
	t.Helper()
	rxMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// rxNew builds Dirs for rxApp with the seams every test must pass; extra
// options come after and win. The release profile is the default.
func rxNew(t *testing.T, home, work string, env map[string]string, extra ...Option) *Dirs {
	t.Helper()
	d, err := rxTryNew(home, work, env, extra...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

// rxTryNew is rxNew returning the error.
func rxTryNew(home, work string, env map[string]string, extra ...Option) (*Dirs, error) {
	opts := []Option{rxHome(home), WithWorkDir(work), rxEnv(env), WithRelease(true)}
	return New(rxApp, append(opts, extra...)...)
}

// rxHasWarning reports whether d has a warning with code.
func rxHasWarning(d *Dirs, code WarningCode) bool {
	for _, w := range d.Warnings() {
		if w.Code == code {
			return true
		}
	}
	return false
}

// rxWorktree makes a main checkout at main (with a .git folder) and a linked
// worktree of it at wt (a .git file pointing at main/.git/worktrees/<name>).
func rxWorktree(t *testing.T, main, wt string) {
	t.Helper()
	admin := filepath.Join(main, gitEntry, "worktrees", filepath.Base(wt))
	rxWrite(t, filepath.Join(admin, "commondir"), "../..\n")
	rxWrite(t, filepath.Join(wt, gitEntry), "gitdir: "+admin+"\n")
}

// F17: a walk that starts in $HOME itself must not climb above it.
func TestFixF17_WalkStopsAtHome(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "top", "home")
	rxMkdir(t, home, filepath.Join(root, "top", rxMarker)) // a project above $HOME
	d := rxNew(t, home, home, nil)
	if _, err := d.Project(); !errors.Is(err, ErrNoProject) {
		t.Fatalf("Project() in $HOME = %v, want ErrNoProject (walk climbed above $HOME)", err)
	}
	// Control: a folder below $HOME still finds a project inside $HOME.
	sub := filepath.Join(home, "p", "src")
	rxMkdir(t, sub, filepath.Join(home, "p", rxMarker))
	if p, err := rxNew(t, home, sub, nil).Project(); err != nil || p.Marker != filepath.Join(home, "p", rxMarker) {
		t.Fatalf("Project() below $HOME = %+v, %v", p, err)
	}
}

// F14: InitProject($HOME) before ~/.<app> exists must not make the global
// home a project home.
func TestFixF14_InitProjectRejectsGlobalHome(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "home")
	link := filepath.Join(root, "homelink")
	rxMkdir(t, home, filepath.Join(root, "work"))
	cases := []struct {
		name string
		dir  string
	}{
		{"home", home},
		{"home with trailing dot", home + string(filepath.Separator) + "."},
	}
	if err := os.Symlink(home, link); err == nil {
		cases = append(cases, struct {
			name string
			dir  string
		}{"symlink to home", link})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := rxNew(t, home, filepath.Join(root, "work"), nil)
			if _, err := d.InitProject(tc.dir); !errors.Is(err, ErrInvalidOption) {
				t.Fatalf("InitProject(%s) = %v, want ErrInvalidOption", tc.dir, err)
			}
			if exists(filepath.Join(home, rxMarker)) {
				t.Fatalf("InitProject(%s) created the global home", tc.dir)
			}
		})
	}
	// Control: any other folder is fine.
	other := filepath.Join(root, "work")
	if _, err := rxNew(t, home, other, nil).InitProject(other); err != nil {
		t.Fatalf("InitProject(other) = %v", err)
	}
}

// rxBrokenPointer makes a project whose committed config has a home: pointer
// to a folder that does not exist. Returns home, work dir and marker.
func rxBrokenPointer(t *testing.T) (string, string, string) {
	t.Helper()
	root := rxRoot(t)
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "proj")
	marker := filepath.Join(work, rxMarker)
	rxMkdir(t, home, filepath.Join(home, rxMarker))
	rxWrite(t, filepath.Join(marker, "config.jsonc"), `{"home": "../missing"}`)
	return home, work, marker
}

// F11: a broken pointer fails every project-capable kind with the pointer
// error; nothing falls back to the global home.
func TestFixF11_BrokenPointerFailsProjectCapableKinds(t *testing.T) {
	home, work, _ := rxBrokenPointer(t)
	d := rxNew(t, home, work, nil)
	for _, k := range []Kind{KindDB, KindLogs, KindConfig, KindConfigLocal, KindHome, KindShared} {
		if p, err := d.Path(k); !errors.Is(err, ErrPointerMissing) {
			t.Errorf("Path(%s) = %q, %v; want ErrPointerMissing", k, p, err)
		}
	}
	if _, err := d.Ensure(KindLogs); !errors.Is(err, ErrPointerMissing) {
		t.Errorf("Ensure(logs) = %v, want ErrPointerMissing", err)
	}
	if exists(filepath.Join(home, rxMarker, "logs")) {
		t.Errorf("Ensure(logs) wrote into the global home")
	}
	// Global-only kinds are not affected.
	if _, err := d.Path(KindCache); err != nil {
		t.Errorf("Path(cache) = %v, want the global cache", err)
	}
	// Paths shows the project line of a both kind with its error.
	found := false
	for _, e := range d.Paths() {
		if e.Kind == KindConfig && e.Home == ScopeProject {
			found = e.Error != ""
		}
		if e.Kind == KindLogs && (e.Home != ScopeProject || e.Error == "") {
			t.Errorf("Paths logs = %+v, want the project line with the pointer error", e)
		}
	}
	if !found {
		t.Errorf("Paths has no project config line carrying the pointer error")
	}
}

// F11: a broken pointer still lets ConfigFiles list the marker's own two
// files, so a tool can show where to fix the pointer.
func TestFixF11_ConfigFilesListsMarkerFiles(t *testing.T) {
	home, work, marker := rxBrokenPointer(t)
	d := rxNew(t, home, work, nil)
	want := map[string]bool{
		filepath.Join(marker, "config.jsonc"):       false,
		filepath.Join(marker, "config.local.jsonc"): false,
	}
	for _, f := range d.ConfigFiles() {
		if _, ok := want[f.Path]; ok && f.Home == ScopeProject {
			want[f.Path] = true
		}
	}
	for p, seen := range want {
		if !seen {
			t.Errorf("ConfigFiles misses %s: %+v", p, d.ConfigFiles())
		}
	}
}

// F2: a --home / <APP>_HOME with an unset ${VAR} fails every kind; the temp
// fallback and code defaults are only for a machine with no $HOME.
func TestFixF2_BrokenOverrideFailsEveryKind(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "work")
	rxMkdir(t, home, work)
	cases := []struct {
		name string
		env  map[string]string
		opts []Option
	}{
		{"flag", nil, []Option{WithHome("${RX_UNSET}/h")}},
		{"env", map[string]string{"RXAPP_HOME": "${RX_UNSET}/h"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := rxNew(t, home, work, tc.env, tc.opts...)
			for _, k := range []Kind{KindCache, KindConfig, KindHome, KindDB, KindLogs} {
				if p, err := d.Path(k); !errors.Is(err, ErrEnvVar) {
					t.Errorf("Path(%s) = %q, %v; want ErrEnvVar", k, p, err)
				}
			}
			if p, err := d.Ensure(KindCache); !errors.Is(err, ErrEnvVar) {
				t.Errorf("Ensure(cache) = %q, %v; want ErrEnvVar", p, err)
			}
		})
	}
	// Control: no $HOME and no override keeps the temp fallback and defaults.
	d, err := rxTryNew("", work, nil, rxNoHome())
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := d.entry(KindCache); e.Rule != RuleTemp || e.err != nil {
		t.Errorf("no $HOME cache = %+v, want the temp fallback", e)
	}
	if e, _ := d.entry(KindConfig); e.Rule != RuleDefaults || e.err != nil {
		t.Errorf("no $HOME config = %+v, want code defaults", e)
	}
}

// F3: WithDirName takes one plain path segment only.
func TestFixF3_DirNameMustBePlainSegment(t *testing.T) {
	root := rxRoot(t)
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b", "/abs"} {
		if _, err := rxTryNew(root, root, nil, WithDirName(name)); !errors.Is(err, ErrInvalidOption) {
			t.Errorf("WithDirName(%q) = %v, want ErrInvalidOption", name, err)
		}
	}
	for _, name := range []string{".ok", "ok", "..ok", ".a.b"} {
		if _, err := rxTryNew(root, root, nil, WithDirName(name)); err != nil {
			t.Errorf("WithDirName(%q) = %v, want nil", name, err)
		}
	}
}

// F5: worktree-main-missing is only worth saying when the worktree's own
// lookup found a home.
func TestFixF5_WorktreeWarnsOnlyWhenWorktreeHasHome(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "home")
	main := filepath.Join(root, "main")
	wt := filepath.Join(root, "wt")
	rxMkdir(t, home)
	rxWorktree(t, main, wt)

	d := rxNew(t, home, wt, nil)
	if rxHasWarning(d, WarnWorktreeMainMissing) {
		t.Errorf("warned %v with no home in the worktree either", d.Warnings())
	}
	if _, err := d.Project(); !errors.Is(err, ErrNoProject) {
		t.Errorf("Project() = %v, want ErrNoProject", err)
	}

	rxMkdir(t, filepath.Join(wt, rxMarker))
	d = rxNew(t, home, wt, nil)
	if !rxHasWarning(d, WarnWorktreeMainMissing) {
		t.Errorf("no worktree-main-missing warning when the worktree's own home is used")
	}
	if p, err := d.Project(); err != nil || p.Marker != filepath.Join(wt, rxMarker) {
		t.Errorf("Project() = %+v, %v; want the worktree's home", p, err)
	}
}

// F6: HereOnly still uses the main checkout's home in a linked worktree
// (standard section 1, D2); here-only only limits the walk.
func TestFixF6_HereOnlyTranslatesWorktree(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "home")
	main := filepath.Join(root, "main")
	wt := filepath.Join(root, "wt")
	rxMkdir(t, home, filepath.Join(main, rxMarker), filepath.Join(wt, "src"))
	rxWorktree(t, main, wt)

	p, err := rxNew(t, home, wt, nil, WithLookup(HereOnly)).Project()
	if err != nil || p.Marker != filepath.Join(main, rxMarker) || p.Rule != RuleWorktree {
		t.Fatalf("HereOnly at the worktree root = %+v, %v; want the main checkout's home", p, err)
	}
	// Here-only does not walk: a subfolder of the worktree finds nothing.
	if _, err := rxNew(t, home, filepath.Join(wt, "src"), nil, WithLookup(HereOnly)).Project(); !errors.Is(err, ErrNoProject) {
		t.Fatalf("HereOnly in wt/src = %v, want ErrNoProject", err)
	}
}

// F7/F18: a dangling .git symlink is still an entry named .git: a wall.
func TestFixF7_DanglingGitSymlinkIsWall(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "home")
	repo := filepath.Join(root, "top", "repo")
	work := filepath.Join(repo, "sub")
	rxMkdir(t, home, work, filepath.Join(root, "top", rxMarker))
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(repo, gitEntry)); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	if p, err := rxNew(t, home, work, nil).Project(); !errors.Is(err, ErrNoProject) {
		t.Fatalf("Project() = %+v, %v; want ErrNoProject (the .git link is a wall)", p, err)
	}
}

// F15: ErrLayoutTooNew names the folder home.jsonc was read from.
func TestFixF15_LayoutErrorNamesHomeJSONFolder(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "proj")
	marker := filepath.Join(work, rxMarker)
	elsewhere := filepath.Join(root, "elsewhere")
	rxMkdir(t, home, elsewhere)
	rxWrite(t, filepath.Join(marker, "home.jsonc"), `{"app": "rxapp", "layout": 99, "scope": "project"}`)

	check := func(t *testing.T, d *Dirs, bad string) {
		t.Helper()
		_, err := d.Ensure(KindDB)
		if !errors.Is(err, ErrLayoutTooNew) {
			t.Fatalf("Ensure(db) = %v, want ErrLayoutTooNew", err)
		}
		if msg := err.Error(); !strings.Contains(msg, "home "+marker+" ") || strings.Contains(msg, bad) {
			t.Fatalf("message %q should name %s, not %s", msg, marker, bad)
		}
	}
	t.Run("dev profile data folder", func(t *testing.T) {
		check(t, rxNew(t, home, work, nil, WithRelease(false)), filepath.Join(marker, profilesDir))
	})
	t.Run("db pointer folder", func(t *testing.T) {
		rxWrite(t, filepath.Join(marker, "config.local.jsonc"), `{"db": "`+filepath.ToSlash(filepath.Join(elsewhere, "x.db"))+`"}`)
		check(t, rxNew(t, home, work, nil), elsewhere)
	})
}

// F13: pointer-ignored-for-profile names the real path of the profile's db.
func TestFixF13_PointerProfileWarningNamesRealPath(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "proj")
	marker := filepath.Join(work, rxMarker)
	rxMkdir(t, home, filepath.Join(root, "elsewhere"))
	rxWrite(t, filepath.Join(marker, "config.jsonc"), `{"db": "../../elsewhere/x.db"}`)

	d := rxNew(t, home, work, nil, WithRelease(false))
	want := filepath.Join(marker, profilesDir, string(ProfileDev), rxApp+".db")
	for _, w := range d.Warnings() {
		if w.Code == WarnPointerProfile {
			if !strings.Contains(w.Message, want) {
				t.Fatalf("warning %q does not name %s", w.Message, want)
			}
			if p, _ := d.Path(KindDB); p != want {
				t.Fatalf("Path(db) = %s, want %s", p, want)
			}
			return
		}
	}
	t.Fatalf("no %s warning: %v", WarnPointerProfile, d.Warnings())
}

// X1: under --home / <APP>_HOME there is one home, and Paths lists a both
// kind once labelled global, like ConfigFiles does.
func TestFixX1_OverrideBothKindsLabelledGlobal(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "home")
	over := filepath.Join(root, "over")
	rxMkdir(t, home, over)
	for name, d := range map[string]*Dirs{
		"flag": rxNew(t, home, root, nil, WithHome(over)),
		"env":  rxNew(t, home, root, map[string]string{"RXAPP_HOME": over}),
	} {
		for _, k := range []Kind{KindHome, KindConfig, KindConfigLocal} {
			var got []Entry
			for _, e := range d.Paths() {
				if e.Kind == k {
					got = append(got, e)
				}
			}
			if len(got) != 1 || got[0].Home != ScopeGlobal {
				t.Errorf("%s: Paths %s = %+v, want one global line", name, k, got)
			}
			if p, _ := d.Path(k); len(got) == 1 && p != got[0].Path {
				t.Errorf("%s: Path(%s) = %s, Paths says %s", name, k, p, got[0].Path)
			}
		}
		for _, f := range d.ConfigFiles() {
			if f.Home != ScopeGlobal {
				t.Errorf("%s: ConfigFiles %+v, want global", name, f)
			}
		}
	}
}
