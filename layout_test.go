package appdir

// layout_test.go: what a tool author and its users see from the layout half
// of appdir: option validation in New, the kind list and its paths per mode,
// the .gitignore block and its writer, InitProject/InitHome, Ensure (modes,
// cache tags, temp fallback, layout guard), the symlink rule, atomic writes
// and path expansion. Every helper, type and const here starts with "tl".

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// tlApp is the app name every test here uses; tlPrefix is its env prefix.
const (
	tlApp    = "tlapp"
	tlPrefix = "TLAPP"
	tlMarker = "." + tlApp
)

// tlStdBlock is the standard block of SPEC 5.7 for tlApp, byte for byte.
const tlStdBlock = "# tlapp:begin\n*\n!.gitignore\n!home.jsonc\n!config.jsonc\n!shared/\n!shared/**\n# tlapp:end\n"

// tlCacheTagBody is the CACHEDIR.TAG content a user's backup tool reads.
const tlCacheTagBody = "Signature: 8a477f597d28d172789f06886806bc55\n# This file is a cache directory tag created by tlapp.\n# For information about cache directory tags, see:\n#\thttps://bford.info/cachedir/\n"

// tlFix is one isolated machine: a user home and a working folder inside a
// fresh temp root.
type tlFix struct {
	root string // real path of the temp root
	home string // the user's home ($HOME seam)
	work string // the working folder (project lookup start)
}

// tlNewFix builds a fixture with empty home and work folders.
func tlNewFix(t *testing.T) tlFix {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fx := tlFix{root: root, home: filepath.Join(root, "home"), work: filepath.Join(root, "work")}
	tlMkdir(t, fx.home)
	tlMkdir(t, fx.work)
	return fx
}

// tlEnv is an env seam over a fixed map (never the process env).
func tlEnv(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// tlOpts are the seams every New here gets; later options win.
func (fx tlFix) tlOpts(extra ...Option) []Option {
	home := fx.home
	base := []Option{
		WithHomeDir(func() (string, error) { return home, nil }),
		WithWorkDir(fx.work),
		WithEnv(tlEnv(nil)),
		WithRelease(true),
	}
	return append(base, extra...)
}

// tlNew builds Dirs for tlApp on the fixture and fails the test on error.
func (fx tlFix) tlNew(t *testing.T, extra ...Option) *Dirs {
	t.Helper()
	d, err := New(tlApp, fx.tlOpts(extra...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

// tlGlobal is the global home of tlApp on the fixture.
func (fx tlFix) tlGlobal() string { return filepath.Join(fx.home, tlMarker) }

// tlProjectMarker makes work/.tlapp (a found project) and returns it.
func (fx tlFix) tlProjectMarker(t *testing.T) string {
	t.Helper()
	m := filepath.Join(fx.work, tlMarker)
	tlMkdir(t, m)
	return m
}

func tlMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
}

func tlWrite(t *testing.T, p, s string, perm fs.FileMode) {
	t.Helper()
	tlMkdir(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte(s), perm); err != nil {
		t.Fatal(err)
	}
}

func tlRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tlMode returns the permission bits of p (following links).
func tlMode(t *testing.T, p string) fs.FileMode {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func tlExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// tlNoTemps fails when an atomic-write temp file was left in dir.
func tlNoTemps(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", filepath.Join(dir, e.Name()))
		}
	}
}

// tlUnix skips tests that rely on Unix modes or symlinks.
func tlUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits / symlinks")
	}
}

// tlHomeJSON parses home.jsonc in dir.
func tlHomeJSON(t *testing.T, dir string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(tlRead(t, filepath.Join(dir, "home.jsonc"))), &m); err != nil {
		t.Fatalf("home.jsonc in %s: %v", dir, err)
	}
	return m
}

// ---------------------------------------------------------------------------
// New: option validation

// TestLayoutNewValidation: New with a bad option (app name, dir name, prefix, mode, lookup, profiles, run files, legacy dirs, nil seams) fails with ErrInvalidOption; each valid edge case is accepted.
func TestLayoutNewValidation(t *testing.T) {
	fx := tlNewFix(t)
	cases := []struct {
		name string
		app  string
		opts []Option
	}{
		{"app empty", "", nil},
		{"app upper case", "Lore", nil},
		{"app starts with dash", "-x", nil},
		{"app starts with dot", ".x", nil},
		{"app with space", "a b", nil},
		{"app with slash", "a/b", nil},
		{"app unicode", "lóre", nil},
		{"dir name empty", tlApp, []Option{WithDirName("")}},
		{"dir name dot", tlApp, []Option{WithDirName(".")}},
		{"dir name dotdot", tlApp, []Option{WithDirName("..")}},
		{"dir name slash", tlApp, []Option{WithDirName("a/b")}},
		{"dir name backslash", tlApp, []Option{WithDirName(`a\b`)}},
		{"dir name NUL", tlApp, []Option{WithDirName("a\x00b")}},
		{"run file slash", tlApp, []Option{WithRunFile(RunPID, "x/y.pid")}},
		{"run file dotdot", tlApp, []Option{WithRunFile(RunSocket, "..")}},
		{"run file empty", tlApp, []Option{WithRunFile(RunLock, "")}},
		{"legacy dir slash", tlApp, []Option{WithLegacyHome(LegacyHome{Dir: "a/.pm"})}},
		{"legacy dir dotdot", tlApp, []Option{WithLegacyHome(LegacyHome{Dir: ".."})}},
		{"legacy dir empty", tlApp, []Option{WithLegacyHome(LegacyHome{})}},
		{"env prefix empty", tlApp, []Option{WithEnvPrefix("")}},
		{"bad mode", tlApp, []Option{WithMode("both")}},
		{"bad lookup", tlApp, []Option{WithLookup("down")}},
		{"layout version zero", tlApp, []Option{WithLayoutVersion(0)}},
		{"layout version negative", tlApp, []Option{WithLayoutVersion(-1)}},
		{"socket max len zero", tlApp, []Option{WithSocketMaxLen(0)}},
		{"empty profile list", tlApp, []Option{WithProfiles()}},
		{"release profile not in list", tlApp, []Option{WithReleaseProfile("staging")}},
		{"dev profile not in list", tlApp, []Option{WithDevProfile("beta")}},
		{"profile list drops prod", tlApp, []Option{WithProfiles(ProfileDev)}},
		{"nil config reader", tlApp, []Option{WithConfigReader(nil)}},
		{"nil env", tlApp, []Option{WithEnv(nil)}},
		{"nil home dir", tlApp, []Option{WithHomeDir(nil)}},
		{"nil build info", tlApp, []Option{WithBuildInfo(nil)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := New(tc.app, fx.tlOpts(tc.opts...)...)
			if !errors.Is(err, ErrInvalidOption) {
				t.Fatalf("New = %v, %v; want ErrInvalidOption", d, err)
			}
			if err.Error() == ErrInvalidOption.Error() {
				t.Errorf("message %q does not name the bad value", err)
			}
		})
	}

	good := []struct {
		name string
		app  string
		opts []Option
	}{
		{"app digits first", "0x", nil},
		{"app dots dashes underscores", "my-app.v2_x", nil},
		{"dir name custom dotted", tlApp, []Option{WithDirName(".tl-home")}},
		{"dir name with ..x", tlApp, []Option{WithDirName("..x")}},
		{"run file renamed", tlApp, []Option{WithRunFile(RunPID, "daemon.pid")}},
		{"legacy dir plain", tlApp, []Option{WithLegacyHome(LegacyHome{Dir: ".pm"})}},
		{"custom profiles", tlApp, []Option{WithProfiles("live", "beta"), WithReleaseProfile("live"), WithDevProfile("beta")}},
		{"global mode here-only", tlApp, []Option{WithMode(ModeGlobal), WithLookup(HereOnly)}},
		{"socket max len 1", tlApp, []Option{WithSocketMaxLen(1)}},
	}
	for _, tc := range good {
		t.Run("ok "+tc.name, func(t *testing.T) {
			if _, err := New(tc.app, fx.tlOpts(tc.opts...)...); err != nil {
				t.Fatalf("New: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Kinds

// tlStdKinds is SPEC 5.1 with the mode's data scope filled in.
func tlStdKinds(mode Mode) []KindSpec {
	data, logs := ScopeProject, ScopeProjectElseGlobal
	if mode == ModeGlobal {
		data, logs = ScopeGlobal, ScopeGlobal
	}
	return []KindSpec{
		{Name: KindHome, Path: "home.jsonc", Type: TypeFile, Git: GitCommit, Scope: ScopeBoth, Anchored: true},
		{Name: KindConfig, Path: "config.jsonc", Type: TypeFile, Git: GitCommit, Scope: ScopeBoth, Anchored: true},
		{Name: KindConfigLocal, Path: "config.local.jsonc", Type: TypeFile, Git: GitIgnore, Scope: ScopeBoth, Anchored: true},
		{Name: KindGitignore, Path: ".gitignore", Type: TypeFile, Git: GitCommit, Scope: ScopeProject, Anchored: true},
		{Name: KindShared, Path: "shared", Type: TypeFolder, Git: GitCommit, Scope: data},
		{Name: KindDB, Path: tlApp + ".db", Type: TypeFile, Git: GitIgnore, Scope: data},
		{Name: KindBackups, Path: "backups", Type: TypeFolder, Git: GitIgnore, Scope: data},
		{Name: KindCache, Path: "cache", Type: TypeFolder, Git: GitIgnore, Scope: ScopeGlobal, Disposable: true},
		{Name: KindRun, Path: "run", Type: TypeFolder, Git: GitIgnore, Scope: data},
		{Name: KindLogs, Path: "logs", Type: TypeFolder, Git: GitIgnore, Scope: logs},
	}
}

// TestLayoutStandardKinds: with no author changes, Kinds() returns the SPEC 5.1 table for each mode, and hands out a copy the caller cannot use to edit the layout.
func TestLayoutStandardKinds(t *testing.T) {
	for _, mode := range []Mode{ModeProject, ModeGlobal} {
		t.Run(string(mode), func(t *testing.T) {
			fx := tlNewFix(t)
			d := fx.tlNew(t, WithMode(mode))
			if got, want := d.Kinds(), tlStdKinds(mode); !reflect.DeepEqual(got, want) {
				t.Fatalf("Kinds():\n got %+v\nwant %+v", got, want)
			}
			// Kinds() hands out a copy: a caller editing it changes nothing.
			ks := d.Kinds()
			ks[0].Path = "hacked"
			if d.Kinds()[0].Path != "home.jsonc" {
				t.Fatal("Kinds() shares its backing array")
			}
		})
	}
}

// TestLayoutKindPaths: where each standard kind lands for a user, per mode,
// with and without a project home.
func TestLayoutKindPaths(t *testing.T) {
	type want struct {
		path     string // "" when an error is expected
		err      error
		fallback bool
	}
	run := func(t *testing.T, mode Mode, withProject bool, exp func(fx tlFix, g, p string) map[Kind]want) {
		fx := tlNewFix(t)
		p := ""
		if withProject {
			p = fx.tlProjectMarker(t)
		}
		d := fx.tlNew(t, WithMode(mode))
		g := fx.tlGlobal()
		wants := exp(fx, g, p)
		entries := map[Kind]Entry{}
		for _, e := range d.Paths() {
			if _, seen := entries[e.Kind]; !seen {
				entries[e.Kind] = e
			}
		}
		for k, w := range wants {
			got, err := d.Path(k)
			if w.err != nil {
				if !errors.Is(err, w.err) {
					t.Errorf("Path(%s) = %q, %v; want %v", k, got, err, w.err)
				}
				continue
			}
			if err != nil || got != w.path {
				t.Errorf("Path(%s) = %q, %v; want %q", k, got, err, w.path)
			}
			if k == KindLogs && entries[k].Fallback != w.fallback {
				t.Errorf("Paths() logs Fallback = %v; want %v", entries[k].Fallback, w.fallback)
			}
		}
	}
	t.Run("project mode with project", func(t *testing.T) {
		run(t, ModeProject, true, func(fx tlFix, g, p string) map[Kind]want {
			return map[Kind]want{
				KindHome:        {path: filepath.Join(p, "home.jsonc")},
				KindConfig:      {path: filepath.Join(p, "config.jsonc")},
				KindConfigLocal: {path: filepath.Join(p, "config.local.jsonc")},
				KindGitignore:   {path: filepath.Join(p, ".gitignore")},
				KindShared:      {path: filepath.Join(p, "shared")},
				KindDB:          {path: filepath.Join(p, tlApp+".db")},
				KindBackups:     {path: filepath.Join(p, "backups")},
				KindCache:       {path: filepath.Join(g, "cache")},
				KindRun:         {path: filepath.Join(p, "run")},
				KindLogs:        {path: filepath.Join(p, "logs")},
			}
		})
	})
	t.Run("project mode outside a project", func(t *testing.T) {
		run(t, ModeProject, false, func(fx tlFix, g, p string) map[Kind]want {
			return map[Kind]want{
				KindHome:      {path: filepath.Join(g, "home.jsonc")},
				KindConfig:    {path: filepath.Join(g, "config.jsonc")},
				KindGitignore: {err: ErrNoProject},
				KindShared:    {err: ErrNoProject},
				KindDB:        {err: ErrNoProject},
				KindBackups:   {err: ErrNoProject},
				KindRun:       {err: ErrNoProject},
				KindCache:     {path: filepath.Join(g, "cache")},
				KindLogs:      {path: filepath.Join(g, "logs"), fallback: true},
			}
		})
	})
	t.Run("global mode inside a project", func(t *testing.T) {
		run(t, ModeGlobal, true, func(fx tlFix, g, p string) map[Kind]want {
			return map[Kind]want{
				KindConfig:    {path: filepath.Join(p, "config.jsonc")}, // both: project wins
				KindGitignore: {path: filepath.Join(p, ".gitignore")},
				KindShared:    {path: filepath.Join(g, "shared")},
				KindDB:        {path: filepath.Join(g, tlApp+".db")},
				KindRun:       {path: filepath.Join(g, "run")},
				KindCache:     {path: filepath.Join(g, "cache")},
				KindLogs:      {path: filepath.Join(g, "logs")},
			}
		})
	})
	t.Run("global mode outside a project", func(t *testing.T) {
		run(t, ModeGlobal, false, func(fx tlFix, g, p string) map[Kind]want {
			return map[Kind]want{
				KindConfig:    {path: filepath.Join(g, "config.jsonc")},
				KindGitignore: {err: ErrNoProject},
				KindDB:        {path: filepath.Join(g, tlApp+".db")},
				KindLogs:      {path: filepath.Join(g, "logs")},
			}
		})
	})
	t.Run("unknown kind", func(t *testing.T) {
		fx := tlNewFix(t)
		d := fx.tlNew(t)
		if _, err := d.Path("nope"); !errors.Is(err, ErrUnknownKind) {
			t.Fatalf("Path(nope) err = %v; want ErrUnknownKind", err)
		}
	})
}

// TestLayoutWithKind: an author adding or renaming a kind (WithKind, Folder, File, Anchored...) gets the expected KindSpec; a rename keeps the git rule, scope and flags of the standard kind.
func TestLayoutWithKind(t *testing.T) {
	cases := []struct {
		name string
		mode Mode
		opts []Option
		kind Kind
		want KindSpec
	}{
		{"new kind defaults project mode", ModeProject, []Option{WithKind("templates")}, "templates",
			KindSpec{Name: "templates", Path: "templates", Type: TypeFolder, Git: GitIgnore, Scope: ScopeProject}},
		{"new kind defaults global mode", ModeGlobal, []Option{WithKind("templates")}, "templates",
			KindSpec{Name: "templates", Path: "templates", Type: TypeFolder, Git: GitIgnore, Scope: ScopeGlobal}},
		{"new kind named logs-x is not logs", ModeProject, []Option{WithKind("logs-x")}, "logs-x",
			KindSpec{Name: "logs-x", Path: "logs-x", Type: TypeFolder, Git: GitIgnore, Scope: ScopeProject}},
		{"rename shared keeps git and scope", ModeProject, []Option{WithKind(KindShared, Folder("data"))}, KindShared,
			KindSpec{Name: KindShared, Path: "data", Type: TypeFolder, Git: GitCommit, Scope: ScopeProject}},
		{"rename cache keeps disposable and global", ModeProject, []Option{WithKind(KindCache, Folder("tmp/c"))}, KindCache,
			KindSpec{Name: KindCache, Path: "tmp/c", Type: TypeFolder, Git: GitIgnore, Scope: ScopeGlobal, Disposable: true}},
		{"rename config keeps anchored both", ModeProject, []Option{WithKind(KindConfig, File("settings.jsonc"))}, KindConfig,
			KindSpec{Name: KindConfig, Path: "settings.jsonc", Type: TypeFile, Git: GitCommit, Scope: ScopeBoth, Anchored: true}},
		{"Folder on a file kind", ModeProject, []Option{WithKind(KindDB, Folder("db"))}, KindDB,
			KindSpec{Name: KindDB, Path: "db", Type: TypeFolder, Git: GitIgnore, Scope: ScopeProject}},
		{"File nested", ModeProject, []Option{WithKind("identity", File("auth/identity.json"), Scoped(ScopeGlobal))}, "identity",
			KindSpec{Name: "identity", Path: "auth/identity.json", Type: TypeFile, Git: GitIgnore, Scope: ScopeGlobal}},
		{"Commit", ModeProject, []Option{WithKind("tpl", Commit())}, "tpl",
			KindSpec{Name: "tpl", Path: "tpl", Type: TypeFolder, Git: GitCommit, Scope: ScopeProject}},
		{"Ignore on committed shared", ModeProject, []Option{WithKind(KindShared, Ignore())}, KindShared,
			KindSpec{Name: KindShared, Path: "shared", Type: TypeFolder, Git: GitIgnore, Scope: ScopeProject}},
		{"Scoped project-else-global", ModeProject, []Option{WithKind("x", Scoped(ScopeProjectElseGlobal))}, "x",
			KindSpec{Name: "x", Path: "x", Type: TypeFolder, Git: GitIgnore, Scope: ScopeProjectElseGlobal}},
		{"Scoped both", ModeGlobal, []Option{WithKind("x", Scoped(ScopeBoth))}, "x",
			KindSpec{Name: "x", Path: "x", Type: TypeFolder, Git: GitIgnore, Scope: ScopeBoth}},
		{"Scoped logs global in project mode", ModeProject, []Option{WithKind(KindLogs, Scoped(ScopeGlobal))}, KindLogs,
			KindSpec{Name: KindLogs, Path: "logs", Type: TypeFolder, Git: GitIgnore, Scope: ScopeGlobal}},
		{"Disposable", ModeProject, []Option{WithKind("thumbs", Disposable())}, "thumbs",
			KindSpec{Name: "thumbs", Path: "thumbs", Type: TypeFolder, Git: GitIgnore, Scope: ScopeProject, Disposable: true}},
		{"Anchored", ModeProject, []Option{WithKind("lockfile", File("lock.jsonc"), Anchored(), Commit())}, "lockfile",
			KindSpec{Name: "lockfile", Path: "lock.jsonc", Type: TypeFile, Git: GitCommit, Scope: ScopeProject, Anchored: true}},
		{"every option at once", ModeProject, []Option{WithKind("all", File("a/b.txt"), Commit(), Scoped(ScopeGlobal), Disposable(), Anchored())}, "all",
			KindSpec{Name: "all", Path: "a/b.txt", Type: TypeFile, Git: GitCommit, Scope: ScopeGlobal, Disposable: true, Anchored: true}},
		{"later option wins", ModeProject, []Option{WithKind("x", Commit(), Ignore(), Folder("a"), Folder("b"))}, "x",
			KindSpec{Name: "x", Path: "b", Type: TypeFolder, Git: GitIgnore, Scope: ScopeProject}},
		{"two WithKind calls stack", ModeProject, []Option{WithKind("x", Commit()), WithKind("x", Folder("y"))}, "x",
			KindSpec{Name: "x", Path: "y", Type: TypeFolder, Git: GitCommit, Scope: ScopeProject}},
		{"WithKind after WithoutKind starts fresh", ModeProject, []Option{WithoutKind(KindShared), WithKind(KindShared)}, KindShared,
			KindSpec{Name: KindShared, Path: "shared", Type: TypeFolder, Git: GitIgnore, Scope: ScopeProject}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := tlNewFix(t)
			d := fx.tlNew(t, append([]Option{WithMode(tc.mode)}, tc.opts...)...)
			var got *KindSpec
			for _, k := range d.Kinds() {
				if k.Name == tc.kind {
					k := k
					got = &k
				}
			}
			if got == nil {
				t.Fatalf("kind %s missing from Kinds()", tc.kind)
			}
			if *got != tc.want {
				t.Fatalf("kind %s:\n got %+v\nwant %+v", tc.kind, *got, tc.want)
			}
		})
	}

	t.Run("order: standard first, new kinds appended in call order", func(t *testing.T) {
		fx := tlNewFix(t)
		d := fx.tlNew(t, WithKind("zeta"), WithKind("alpha"), WithKind(KindShared, Folder("data")), WithoutKind(KindBackups), WithoutKind(KindRun), WithKind(KindRun))
		var names []Kind
		for _, k := range d.Kinds() {
			names = append(names, k.Name)
		}
		want := []Kind{KindHome, KindConfig, KindConfigLocal, KindGitignore, KindShared, KindDB, KindCache, KindLogs, "zeta", "alpha", KindRun}
		if !reflect.DeepEqual(names, want) {
			t.Fatalf("order %v; want %v", names, want)
		}
		again := fx.tlNew(t, WithKind("zeta"), WithKind("alpha"), WithKind(KindShared, Folder("data")), WithoutKind(KindBackups), WithoutKind(KindRun), WithKind(KindRun))
		if !reflect.DeepEqual(d.Kinds(), again.Kinds()) {
			t.Fatal("Kinds() differs between two identical New calls")
		}
	})

	t.Run("WithoutKind drops from Paths, block and Ensure", func(t *testing.T) {
		fx := tlNewFix(t)
		fx.tlProjectMarker(t)
		d := fx.tlNew(t, WithoutKind(KindShared), WithoutKind("not-there"))
		for _, e := range d.Paths() {
			if e.Kind == KindShared {
				t.Fatal("shared still in Paths()")
			}
		}
		if strings.Contains(d.GitignoreBlock(), "shared") {
			t.Fatalf("shared still in block:\n%s", d.GitignoreBlock())
		}
		if _, err := d.Ensure(KindShared); !errors.Is(err, ErrUnknownKind) {
			t.Fatalf("Ensure(shared) err = %v; want ErrUnknownKind", err)
		}
		if _, err := d.Path(KindShared); !errors.Is(err, ErrUnknownKind) {
			t.Fatalf("Path(shared) err = %v; want ErrUnknownKind", err)
		}
	})

	t.Run("renamed kind resolves to its new path", func(t *testing.T) {
		fx := tlNewFix(t)
		p := fx.tlProjectMarker(t)
		d := fx.tlNew(t, WithKind(KindShared, Folder("team/data")))
		got, err := d.Ensure(KindShared)
		if err != nil || got != filepath.Join(p, "team", "data") {
			t.Fatalf("Ensure(shared) = %q, %v", got, err)
		}
	})
}

// TestLayoutInvalidKinds: a kind with a bad name or a path that could leave the home makes New fail with ErrInvalidOption.
func TestLayoutInvalidKinds(t *testing.T) {
	cases := []struct {
		name string
		opt  Option
	}{
		{"name empty", WithKind("")},
		{"name upper", WithKind("Templates")},
		{"name digit first", WithKind("1x")},
		{"name dash first", WithKind("-x")},
		{"name underscore", WithKind("a_b")},
		{"name dot", WithKind("a.b")},
		{"name slash", WithKind("a/b")},
		{"WithoutKind bad name", WithoutKind("Bad")},
		{"path empty", WithKind("x", Folder(""))},
		{"path absolute", WithKind("x", Folder("/etc"))},
		{"path backslash", WithKind("x", Folder(`a\b`))},
		{"path colon", WithKind("x", File("c:x"))},
		{"path drive letter", WithKind("x", File(`C:/x`))},
		{"path dotdot", WithKind("x", Folder(".."))},
		{"path dotdot inside", WithKind("x", Folder("a/../b"))},
		{"path dotdot escape", WithKind("x", Folder("../x"))},
		{"path dot", WithKind("x", Folder("."))},
		{"path dot prefix", WithKind("x", Folder("./a"))},
		{"path empty segment", WithKind("x", Folder("a//b"))},
		{"path trailing slash", WithKind("x", Folder("a/"))},
		{"existing kind to bad path", WithKind(KindDB, File("../db"))},
		{"unknown scope", WithKind("x", Scoped("everywhere"))},
		{"unknown scope on existing", WithKind(KindRun, Scoped("nowhere"))},
	}
	fx := tlNewFix(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tlApp, fx.tlOpts(tc.opt)...); !errors.Is(err, ErrInvalidOption) {
				t.Fatalf("New err = %v; want ErrInvalidOption", err)
			}
		})
	}
	t.Run("ok dotted names are fine", func(t *testing.T) {
		if _, err := New(tlApp, fx.tlOpts(WithKind("x", Folder("..x/.y/z..")))...); err != nil {
			t.Fatalf("New: %v", err)
		}
	})
}

// TestLayoutKindEnv: each movable kind, standard or author-added, names its <APP>_<KIND>_DIR variable (upper case, dashes become underscores, custom prefix honoured); anchored kinds (home, config files, gitignore, an Anchored() kind) and unknown kinds name none.
func TestLayoutKindEnv(t *testing.T) {
	fx := tlNewFix(t)
	d := fx.tlNew(t, WithKind("templates"), WithKind("my-kind"), WithKind("pinned", Anchored()))
	cases := []struct {
		kind Kind
		want string
	}{
		{KindShared, tlPrefix + "_SHARED_DIR"},
		{KindDB, tlPrefix + "_DB_DIR"},
		{KindCache, tlPrefix + "_CACHE_DIR"},
		{KindLogs, tlPrefix + "_LOGS_DIR"},
		{"templates", tlPrefix + "_TEMPLATES_DIR"},
		{"my-kind", tlPrefix + "_MY_KIND_DIR"},
		{KindHome, ""},
		{KindConfig, ""},
		{KindConfigLocal, ""},
		{KindGitignore, ""},
		{"pinned", ""},
		{"unknown", ""},
	}
	for _, tc := range cases {
		if got := d.KindEnv(tc.kind); got != tc.want {
			t.Errorf("KindEnv(%s) = %q; want %q", tc.kind, got, tc.want)
		}
	}
	// A custom prefix flows into every name.
	dp := fx.tlNew(t, WithEnvPrefix("TL2"), WithKind("my-kind"))
	if got := dp.KindEnv("my-kind"); got != "TL2_MY_KIND_DIR" {
		t.Errorf("KindEnv with prefix TL2 = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Gitignore block

// TestLayoutGitignoreBlock: GitignoreBlock writes the marker-delimited block (ignore all, re-include committed kinds and their parents) and honours WithGitCommit / WithGitIgnore as SPEC 5.7 says.
func TestLayoutGitignoreBlock(t *testing.T) {
	block := func(lines ...string) string {
		return "# tlapp:begin\n*\n" + strings.Join(lines, "\n") + "\n# tlapp:end\n"
	}
	std := []string{"!.gitignore", "!home.jsonc", "!config.jsonc", "!shared/", "!shared/**"}
	with := func(extra ...string) string { return block(append(append([]string(nil), std...), extra...)...) }
	cases := []struct {
		name string
		opts []Option
		want string
	}{
		{"default layout", nil, tlStdBlock},
		{"global mode same block", []Option{WithMode(ModeGlobal)}, tlStdBlock},
		{"author committed folder", []Option{WithKind("templates", Commit())}, with("!templates/", "!templates/**")},
		{"author nested committed folder gets parents", []Option{WithKind("tpl", Folder("team/tpl/x"), Commit())},
			with("!team/", "!team/tpl/", "!team/tpl/x/", "!team/tpl/x/**")},
		{"author nested committed file gets parents", []Option{WithKind("notes", File("docs/notes.md"), Commit())},
			with("!docs/", "!docs/notes.md")},
		{"author ignored kind adds nothing", []Option{WithKind("tmp")}, tlStdBlock},
		{"renamed shared", []Option{WithKind(KindShared, Folder("data"))},
			block("!.gitignore", "!home.jsonc", "!config.jsonc", "!data/", "!data/**")},
		{"shared made ignored", []Option{WithKind(KindShared, Ignore())},
			block("!.gitignore", "!home.jsonc", "!config.jsonc")},
		{"gitignore kind first even when renamed later", []Option{WithKind(KindGitignore, File("sub/.gitignore"))},
			block("!sub/", "!sub/.gitignore", "!home.jsonc", "!config.jsonc", "!shared/", "!shared/**")},
		{"WithGitCommit file", []Option{WithGitCommit("README.md")}, with("!README.md")},
		{"WithGitCommit nested pattern gets parents", []Option{WithGitCommit("notes/a/*.md")},
			with("!notes/", "!notes/a/", "!notes/a/*.md")},
		{"WithGitCommit glob segment stops parents", []Option{WithGitCommit("x/*/y/z.md")},
			with("!x/", "!x/*/y/z.md")},
		{"WithGitCommit glob first segment", []Option{WithGitCommit("*.md")}, with("!*.md")},
		{"WithGitCommit bracket glob", []Option{WithGitCommit("a/[bc]/d")}, with("!a/", "!a/[bc]/d")},
		{"WithGitCommit under shared dedups parents", []Option{WithGitCommit("shared/x/y")}, with("!shared/x/", "!shared/x/y")},
		{"WithGitCommit duplicate written once", []Option{WithGitCommit("a.txt", "a.txt")}, with("!a.txt")},
		{"WithGitIgnore removes committed folder", []Option{WithGitIgnore("shared")},
			block("!.gitignore", "!home.jsonc", "!config.jsonc")},
		{"WithGitIgnore removes committed folder with slash", []Option{WithGitIgnore("shared/")},
			block("!.gitignore", "!home.jsonc", "!config.jsonc")},
		{"WithGitIgnore removes committed file", []Option{WithGitIgnore("config.jsonc")},
			block("!.gitignore", "!home.jsonc", "!shared/", "!shared/**")},
		{"WithGitIgnore removes WithGitCommit pattern", []Option{WithGitCommit("notes/"), WithGitIgnore("notes")}, tlStdBlock},
		{"WithGitIgnore removed nested kind keeps no parents", []Option{WithKind("t", Folder("a/b"), Commit()), WithGitIgnore("a/b")}, tlStdBlock},
		{"WithGitIgnore inside committed folder is a plain line", []Option{WithGitIgnore("shared/secret/")}, with("shared/secret/")},
		{"WithGitIgnore unrelated is a plain line", []Option{WithGitIgnore("*.bak", "big/")}, with("*.bak", "big/")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := tlNewFix(t)
			got := fx.tlNew(t, tc.opts...).GitignoreBlock()
			if got != tc.want {
				t.Fatalf("block:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}

	// A WithGitIgnore pattern is "forced out of git; wins over every commit
	// rule" (options.go, SPEC 5.7). A pattern that matches a generated
	// re-include line but not a commit entry must still win.
	bugs := []struct {
		name string
		opts []Option
		pat  string // the ignore pattern
		line string // the re-include it must cancel or follow
	}{
		{"ignore shared/** keeps shared contents out", []Option{WithGitIgnore("shared/**")}, "shared/**", "!shared/**"},
		{"ignore parent of a committed nested kind", []Option{WithKind("t", Folder("a/b"), Commit()), WithGitIgnore("a/")}, "a/", "!a/"},
	}
	for _, tc := range bugs {
		t.Run("ignore wins "+tc.name, func(t *testing.T) {
			fx := tlNewFix(t)
			lines := strings.Split(fx.tlNew(t, tc.opts...).GitignoreBlock(), "\n")
			reinc, after := -1, -1
			for i, l := range lines {
				switch l {
				case tc.line:
					reinc = i
				case tc.pat:
					after = i
				}
			}
			// Either the re-include is gone, or the plain pattern follows it.
			if reinc >= 0 && after < reinc {
				t.Fatalf("TL1: WithGitIgnore(%q) equals a generated re-include line (%s) but neither removes it nor follows it; git would still commit the path", tc.pat, tc.line)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// WriteGitignore

// tlGitProject makes work/.git and work/.tlapp and returns the .gitignore path.
func (fx tlFix) tlGitProject(t *testing.T) string {
	t.Helper()
	tlMkdir(t, filepath.Join(fx.work, ".git"))
	return filepath.Join(fx.tlProjectMarker(t), ".gitignore")
}

// TestLayoutWriteGitignore: WriteGitignore writes or refreshes the block only in a git work tree (a .git file counts), keeps the user's own lines outside the markers and the file's mode, and fails with ErrGitignoreMarkers on broken markers and ErrNoProject without a project.
func TestLayoutWriteGitignore(t *testing.T) {
	t.Run("not a git work tree writes nothing", func(t *testing.T) {
		fx := tlNewFix(t)
		m := fx.tlProjectMarker(t)
		ok, err := fx.tlNew(t).WriteGitignore()
		if ok || err != nil {
			t.Fatalf("WriteGitignore = %v, %v; want false, nil", ok, err)
		}
		if tlExists(filepath.Join(m, ".gitignore")) {
			t.Fatal(".gitignore created outside a git work tree")
		}
	})
	t.Run(".git file (worktree/submodule) counts", func(t *testing.T) {
		fx := tlNewFix(t)
		m := fx.tlProjectMarker(t)
		tlWrite(t, filepath.Join(fx.root, ".git"), "gitdir: elsewhere\n", 0o600)
		ok, err := fx.tlNew(t).WriteGitignore()
		if !ok || err != nil || tlRead(t, filepath.Join(m, ".gitignore")) != tlStdBlock {
			t.Fatalf("WriteGitignore = %v, %v", ok, err)
		}
	})
	t.Run("no project is ErrNoProject", func(t *testing.T) {
		fx := tlNewFix(t)
		if _, err := fx.tlNew(t).WriteGitignore(); !errors.Is(err, ErrNoProject) {
			t.Fatalf("err = %v; want ErrNoProject", err)
		}
	})
	t.Run("gitignore kind removed writes nothing", func(t *testing.T) {
		fx := tlNewFix(t)
		gi := fx.tlGitProject(t)
		ok, err := fx.tlNew(t, WithoutKind(KindGitignore)).WriteGitignore()
		if ok || err != nil || tlExists(gi) {
			t.Fatalf("WriteGitignore = %v, %v; exists=%v", ok, err, tlExists(gi))
		}
	})

	edits := []struct {
		name string
		old  string
		want string
	}{
		{"new file", "", tlStdBlock},
		{"existing without markers appends", "# mine\n*.log\n", "# mine\n*.log\n" + tlStdBlock},
		{"existing without trailing newline", "# mine", "# mine\n" + tlStdBlock},
		{"replaces between markers keeping outside lines",
			"top\n# tlapp:begin\nold\nstuff\n# tlapp:end\nbottom\n", "top\n" + tlStdBlock + "bottom\n"},
		{"CRLF markers are recognised",
			"top\r\n# tlapp:begin\r\nold\r\n# tlapp:end\r\nbottom\r\n", "top\r\n" + tlStdBlock + "bottom\r\n"},
		{"another app's markers are hand lines",
			"# other:begin\nx\n# other:end\n", "# other:begin\nx\n# other:end\n" + tlStdBlock},
	}
	for _, tc := range edits {
		t.Run(tc.name, func(t *testing.T) {
			fx := tlNewFix(t)
			gi := fx.tlGitProject(t)
			if tc.old != "" {
				tlWrite(t, gi, tc.old, 0o644)
			}
			d := fx.tlNew(t)
			ok, err := d.WriteGitignore()
			if !ok || err != nil {
				t.Fatalf("WriteGitignore = %v, %v", ok, err)
			}
			if got := tlRead(t, gi); got != tc.want {
				t.Fatalf("file:\n%q\nwant:\n%q", got, tc.want)
			}
			// Idempotent: a second write changes nothing (not even mtime).
			fi1, _ := os.Stat(gi)
			if ok, err := d.WriteGitignore(); !ok || err != nil {
				t.Fatalf("second WriteGitignore = %v, %v", ok, err)
			}
			fi2, _ := os.Stat(gi)
			if got := tlRead(t, gi); got != tc.want || !fi1.ModTime().Equal(fi2.ModTime()) || !os.SameFile(fi1, fi2) {
				t.Fatalf("second write changed the file:\n%q", got)
			}
			tlNoTemps(t, filepath.Dir(gi))
		})
	}

	t.Run("block follows option changes", func(t *testing.T) {
		fx := tlNewFix(t)
		gi := fx.tlGitProject(t)
		if _, err := fx.tlNew(t).WriteGitignore(); err != nil {
			t.Fatal(err)
		}
		tlWrite(t, gi, "hand\n"+tlRead(t, gi)+"tail\n", 0o600)
		if _, err := fx.tlNew(t, WithGitCommit("README.md")).WriteGitignore(); err != nil {
			t.Fatal(err)
		}
		want := "hand\n" + strings.Replace(tlStdBlock, "# tlapp:end", "!README.md\n# tlapp:end", 1) + "tail\n"
		if got := tlRead(t, gi); got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	broken := []struct {
		name string
		old  string
	}{
		{"begin only", "# tlapp:begin\n*\n"},
		{"end only", "*\n# tlapp:end\n"},
		{"doubled begin", "# tlapp:begin\n# tlapp:begin\n*\n# tlapp:end\n"},
		{"doubled block", tlStdBlock + tlStdBlock},
		{"out of order", "# tlapp:end\n*\n# tlapp:begin\n"},
	}
	for _, tc := range broken {
		t.Run("markers "+tc.name, func(t *testing.T) {
			fx := tlNewFix(t)
			gi := fx.tlGitProject(t)
			tlWrite(t, gi, tc.old, 0o644)
			ok, err := fx.tlNew(t).WriteGitignore()
			if !ok || !errors.Is(err, ErrGitignoreMarkers) {
				t.Fatalf("WriteGitignore = %v, %v; want true, ErrGitignoreMarkers", ok, err)
			}
			if !strings.Contains(err.Error(), gi) {
				t.Errorf("error %q does not name %s", err, gi)
			}
			if got := tlRead(t, gi); got != tc.old {
				t.Fatalf("file changed to %q", got)
			}
		})
	}

	t.Run("symlinked .gitignore is written through", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		gi := fx.tlGitProject(t)
		target := filepath.Join(fx.work, "real.gitignore")
		tlWrite(t, target, "keep\n", 0o640)
		if err := os.Symlink(target, gi); err != nil {
			t.Fatal(err)
		}
		if ok, err := fx.tlNew(t).WriteGitignore(); !ok || err != nil {
			t.Fatalf("WriteGitignore = %v, %v", ok, err)
		}
		fi, err := os.Lstat(gi)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf(".gitignore is no longer a symlink: %v", err)
		}
		if got := tlRead(t, target); got != "keep\n"+tlStdBlock {
			t.Fatalf("target = %q", got)
		}
		if m := tlMode(t, target); m != 0o640 {
			t.Errorf("target mode %o; want 640 kept", m)
		}
		tlNoTemps(t, fx.work)
		tlNoTemps(t, filepath.Dir(gi))
	})

	t.Run("existing file keeps its mode", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		gi := fx.tlGitProject(t)
		tlWrite(t, gi, "x\n", 0o644)
		if err := os.Chmod(gi, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.tlNew(t).WriteGitignore(); err != nil {
			t.Fatal(err)
		}
		if m := tlMode(t, gi); m != 0o644 {
			t.Fatalf("mode %o; want 644 kept", m)
		}
	})

	t.Run("new file gets the file mode", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		gi := fx.tlGitProject(t)
		if _, err := fx.tlNew(t, WithPerm(0o750, 0o640)).WriteGitignore(); err != nil {
			t.Fatal(err)
		}
		if m := tlMode(t, gi); m != 0o640 {
			t.Fatalf("mode %o; want 640", m)
		}
	})

	t.Run("unreadable .gitignore is an error", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		gi := fx.tlGitProject(t)
		tlMkdir(t, gi) // a folder named .gitignore cannot be read as a file
		if ok, err := fx.tlNew(t).WriteGitignore(); !ok || err == nil {
			t.Fatalf("WriteGitignore = %v, %v; want an error", ok, err)
		}
	})
}

// ---------------------------------------------------------------------------
// InitProject / InitHome

// TestLayoutInitProject: InitProject creates the marker folder with home.jsonc (relative dirs against the working folder), refuses the global home under any spelling, another app's home and a too-new layout, and keeps an existing home of this app.
func TestLayoutInitProject(t *testing.T) {
	t.Run("creates marker and home.jsonc", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		proj := filepath.Join(fx.work, "shop")
		tlMkdir(t, proj)
		d := fx.tlNew(t)
		m, err := d.InitProject(proj)
		if err != nil || m != filepath.Join(proj, tlMarker) {
			t.Fatalf("InitProject = %q, %v", m, err)
		}
		if got := tlMode(t, m); got != 0o700 {
			t.Errorf("marker mode %o; want 700", got)
		}
		if got := tlMode(t, filepath.Join(m, "home.jsonc")); got != 0o600 {
			t.Errorf("home.jsonc mode %o; want 600", got)
		}
		want := map[string]any{"app": tlApp, "layout": float64(1), "scope": "project"}
		if got := tlHomeJSON(t, m); !reflect.DeepEqual(got, want) {
			t.Errorf("home.jsonc %v; want %v", got, want)
		}
		if tlExists(filepath.Join(m, ".gitignore")) {
			t.Error(".gitignore written outside a git work tree")
		}
		tlNoTemps(t, m)
		// A later run finds the project.
		d2, err := New(tlApp, fx.tlOpts(WithWorkDir(proj))...)
		if err != nil || !d2.HasProject() {
			t.Fatalf("project not found after init: %v", err)
		}
		if h, _ := d2.Project(); h.Info == nil || h.Info.App != tlApp || h.Info.Scope != ScopeProject {
			t.Fatalf("Project().Info = %+v", h.Info)
		}
	})
	t.Run("relative dir resolves against the working folder", func(t *testing.T) {
		fx := tlNewFix(t)
		// The process cwd is moved to a decoy folder so a relative path that
		// ignores WithWorkDir lands there, never in the repo.
		decoy := filepath.Join(fx.root, "decoy")
		tlMkdir(t, decoy)
		t.Chdir(decoy)
		m, err := fx.tlNew(t).InitProject("a/b")
		want := filepath.Join(fx.work, "a", "b", tlMarker)
		if err != nil {
			t.Fatalf("InitProject: %v", err)
		}
		if m != want {
			t.Fatalf("TL2: InitProject(relative) = %s, want %s (resolved against WithWorkDir, like InitHome)", m, want)
		}
	})
	t.Run("missing parents are created", func(t *testing.T) {
		fx := tlNewFix(t)
		dir := filepath.Join(fx.work, "a", "b")
		m, err := fx.tlNew(t).InitProject(dir)
		if err != nil || m != filepath.Join(dir, tlMarker) || !tlExists(filepath.Join(m, "home.jsonc")) {
			t.Fatalf("InitProject = %q, %v", m, err)
		}
	})
	t.Run("custom layout version and dir name", func(t *testing.T) {
		fx := tlNewFix(t)
		m, err := fx.tlNew(t, WithLayoutVersion(3), WithDirName(".tl-x")).InitProject(fx.work)
		if err != nil || filepath.Base(m) != ".tl-x" {
			t.Fatalf("InitProject = %q, %v", m, err)
		}
		if got := tlHomeJSON(t, m)["layout"]; got != float64(3) {
			t.Fatalf("layout %v; want 3", got)
		}
	})
	t.Run("inside a git work tree writes .gitignore", func(t *testing.T) {
		fx := tlNewFix(t)
		tlMkdir(t, filepath.Join(fx.root, ".git"))
		m, err := fx.tlNew(t).InitProject(fx.work)
		if err != nil {
			t.Fatal(err)
		}
		if got := tlRead(t, filepath.Join(m, ".gitignore")); got != tlStdBlock {
			t.Fatalf(".gitignore = %q", got)
		}
	})
	t.Run("broken markers fail after home.jsonc", func(t *testing.T) {
		fx := tlNewFix(t)
		tlMkdir(t, filepath.Join(fx.root, ".git"))
		gi := filepath.Join(fx.work, tlMarker, ".gitignore")
		tlWrite(t, gi, "# tlapp:begin\n", 0o600)
		m, err := fx.tlNew(t).InitProject(fx.work)
		if !errors.Is(err, ErrGitignoreMarkers) || m == "" {
			t.Fatalf("InitProject = %q, %v; want marker + ErrGitignoreMarkers", m, err)
		}
	})
	t.Run("existing home of this app is kept", func(t *testing.T) {
		fx := tlNewFix(t)
		m := fx.tlProjectMarker(t)
		orig := "// mine\n{\"app\": \"tlapp\", \"layout\": 1, \"scope\": \"project\", \"extra\": true}\n"
		hj := filepath.Join(m, "home.jsonc")
		tlWrite(t, hj, orig, 0o644)
		fi1, _ := os.Stat(hj)
		got, err := fx.tlNew(t).InitProject(fx.work)
		if err != nil || got != m {
			t.Fatalf("InitProject = %q, %v", got, err)
		}
		fi2, _ := os.Stat(hj)
		if tlRead(t, hj) != orig || !os.SameFile(fi1, fi2) {
			t.Fatal("existing home.jsonc was rewritten")
		}
	})
	t.Run("another app's home is ErrNotHome", func(t *testing.T) {
		fx := tlNewFix(t)
		m := fx.tlProjectMarker(t)
		orig := `{"app": "other", "layout": 1, "scope": "project"}`
		tlWrite(t, filepath.Join(m, "home.jsonc"), orig, 0o600)
		if _, err := fx.tlNew(t).InitProject(fx.work); !errors.Is(err, ErrNotHome) {
			t.Fatalf("err = %v; want ErrNotHome", err)
		}
		if tlRead(t, filepath.Join(m, "home.jsonc")) != orig {
			t.Fatal("other app's home.jsonc changed")
		}
	})
	t.Run("layout too new is refused", func(t *testing.T) {
		fx := tlNewFix(t)
		m := fx.tlProjectMarker(t)
		tlWrite(t, filepath.Join(m, "home.jsonc"), `{"app": "tlapp", "layout": 2, "scope": "project"}`, 0o600)
		if _, err := fx.tlNew(t).InitProject(fx.work); !errors.Is(err, ErrLayoutTooNew) {
			t.Fatalf("err = %v; want ErrLayoutTooNew", err)
		}
	})
	for _, exists := range []bool{true, false} {
		t.Run("on the global home exists="+strconv.FormatBool(exists), func(t *testing.T) {
			fx := tlNewFix(t)
			if exists {
				tlMkdir(t, fx.tlGlobal())
			}
			d := fx.tlNew(t)
			if _, err := d.InitProject(fx.home); !errors.Is(err, ErrInvalidOption) {
				t.Fatalf("InitProject($HOME) err = %v; want ErrInvalidOption", err)
			}
			if tlExists(filepath.Join(fx.tlGlobal(), "home.jsonc")) {
				t.Fatal("global home got a project home.jsonc")
			}
		})
	}
	t.Run("on the global home through a symlinked path", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		link := filepath.Join(fx.root, "homelink")
		if err := os.Symlink(fx.home, link); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.tlNew(t).InitProject(link); !errors.Is(err, ErrInvalidOption) {
			t.Fatalf("err = %v; want ErrInvalidOption", err)
		}
	})
	t.Run("on the prod global home from a dev build", func(t *testing.T) {
		fx := tlNewFix(t)
		d := fx.tlNew(t, WithRelease(false))
		if _, err := d.InitProject(fx.home); !errors.Is(err, ErrInvalidOption) {
			t.Fatalf("err = %v; want ErrInvalidOption", err)
		}
	})
}

// TestLayoutInitHome: InitHome expands ~ and ${VAR} in the path (ErrEnvVar / ErrNoHome when it cannot), accepts only the global and project scopes, writes home.jsonc, and refuses another app's home or an unparsable home.jsonc.
func TestLayoutInitHome(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		env   map[string]string
		scope Scope
		want  func(fx tlFix) string
	}{
		{"absolute global", "", nil, ScopeGlobal, func(fx tlFix) string { return filepath.Join(fx.root, "abs") }},
		{"tilde slash", "~/elsewhere/h", nil, ScopeGlobal, func(fx tlFix) string { return filepath.Join(fx.home, "elsewhere", "h") }},
		{"tilde alone", "~", nil, ScopeGlobal, func(fx tlFix) string { return fx.home }},
		{"var", "${TLX}/h", map[string]string{"TLX": "@root"}, ScopeProject, func(fx tlFix) string { return filepath.Join(fx.root, "h") }},
		{"two vars", "${TLA}/${TLB}", map[string]string{"TLA": "@root", "TLB": "deep"}, ScopeProject, func(fx tlFix) string { return filepath.Join(fx.root, "deep") }},
		{"tilde then var", "~/${TLB}", map[string]string{"TLB": "v"}, ScopeGlobal, func(fx tlFix) string { return filepath.Join(fx.home, "v") }},
		{"dollar without braces is literal", "$TLX/h", map[string]string{"TLX": "@root"}, ScopeGlobal, func(fx tlFix) string { return filepath.Join(fx.work, "$TLX", "h") }},
		{"relative joined to work dir", "rel/h", nil, ScopeGlobal, func(fx tlFix) string { return filepath.Join(fx.work, "rel", "h") }},
		{"unclean path cleaned", "rel/../h2/./x", nil, ScopeGlobal, func(fx tlFix) string { return filepath.Join(fx.work, "h2", "x") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := tlNewFix(t)
			env := map[string]string{}
			for k, v := range tc.env {
				env[k] = strings.ReplaceAll(v, "@root", fx.root)
			}
			raw := tc.raw
			if raw == "" {
				raw = filepath.Join(fx.root, "abs")
			}
			d := fx.tlNew(t, WithEnv(tlEnv(env)))
			if err := d.InitHome(raw, tc.scope); err != nil {
				t.Fatalf("InitHome(%q): %v", raw, err)
			}
			dir := tc.want(fx)
			want := map[string]any{"app": tlApp, "layout": float64(1), "scope": string(tc.scope)}
			if got := tlHomeJSON(t, dir); !reflect.DeepEqual(got, want) {
				t.Fatalf("home.jsonc in %s = %v; want %v", dir, got, want)
			}
			if runtime.GOOS != "windows" {
				if m := tlMode(t, filepath.Join(dir, "home.jsonc")); m != 0o600 {
					t.Errorf("home.jsonc mode %o", m)
				}
			}
			tlNoTemps(t, dir)
		})
	}

	errCases := []struct {
		name    string
		raw     string
		env     map[string]string
		scope   Scope
		noHome  bool
		want    error
		mention string
	}{
		{"scope both", "x", nil, ScopeBoth, false, ErrInvalidOption, "both"},
		{"scope project-else-global", "x", nil, ScopeProjectElseGlobal, false, ErrInvalidOption, ""},
		{"scope empty", "x", nil, "", false, ErrInvalidOption, ""},
		{"unset var", "${TLMISSING}/h", nil, ScopeGlobal, false, ErrEnvVar, "TLMISSING"},
		{"empty var", "${TLEMPTY}/h", map[string]string{"TLEMPTY": ""}, ScopeGlobal, false, ErrEnvVar, "TLEMPTY"},
		{"second var unset", "${TLA}/${TLB}", map[string]string{"TLA": "x"}, ScopeGlobal, false, ErrEnvVar, "TLB"},
		{"unclosed brace", "${TLX/h", map[string]string{"TLX": "x"}, ScopeGlobal, false, ErrInvalidOption, "${"},
		{"tilde user", "~bob/h", nil, ScopeGlobal, false, ErrInvalidOption, "~bob"},
		{"tilde without a home", "~/h", nil, ScopeGlobal, true, ErrNoHome, ""},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			fx := tlNewFix(t)
			opts := []Option{WithEnv(tlEnv(tc.env))}
			if tc.noHome {
				opts = append(opts, WithHomeDir(func() (string, error) { return "", errors.New("no home") }))
			}
			before, _ := os.ReadDir(fx.work)
			err := fx.tlNew(t, opts...).InitHome(tc.raw, tc.scope)
			if !errors.Is(err, tc.want) {
				t.Fatalf("InitHome(%q, %q) err = %v; want %v", tc.raw, tc.scope, err, tc.want)
			}
			if tc.mention != "" && !strings.Contains(err.Error(), tc.mention) {
				t.Errorf("error %q does not mention %q", err, tc.mention)
			}
			if after, _ := os.ReadDir(fx.work); len(after) != len(before) {
				t.Errorf("a failed InitHome created %v", after)
			}
		})
	}

	t.Run("existing home of this app is kept", func(t *testing.T) {
		fx := tlNewFix(t)
		h := filepath.Join(fx.root, "h")
		orig := `{"app": "tlapp", "layout": 1, "scope": "global"}`
		tlWrite(t, filepath.Join(h, "home.jsonc"), orig, 0o600)
		if err := fx.tlNew(t).InitHome(h, ScopeProject); err != nil {
			t.Fatal(err)
		}
		if tlRead(t, filepath.Join(h, "home.jsonc")) != orig {
			t.Fatal("rewritten")
		}
	})
	t.Run("another app's home is ErrNotHome", func(t *testing.T) {
		fx := tlNewFix(t)
		h := filepath.Join(fx.root, "h")
		tlWrite(t, filepath.Join(h, "home.jsonc"), `{"app": "pm"}`, 0o600)
		if err := fx.tlNew(t).InitHome(h, ScopeGlobal); !errors.Is(err, ErrNotHome) {
			t.Fatalf("err = %v; want ErrNotHome", err)
		}
	})
	t.Run("unparsable home.jsonc is refused, never replaced", func(t *testing.T) {
		// A home.jsonc appdir cannot read is the user's file (hand-edited,
		// another tool's): init refuses and leaves it exactly as it was.
		fx := tlNewFix(t)
		h := filepath.Join(fx.root, "h")
		tlWrite(t, filepath.Join(h, "home.jsonc"), "{nope", 0o644)
		if err := fx.tlNew(t).InitHome(h, ScopeGlobal); err == nil {
			t.Fatal("InitHome on an unreadable home.jsonc: no error")
		}
		if b, err := os.ReadFile(filepath.Join(h, "home.jsonc")); err != nil || string(b) != "{nope" {
			t.Fatalf("home.jsonc changed: %q, %v", b, err)
		}
		if runtime.GOOS != "windows" {
			if m := tlMode(t, filepath.Join(h, "home.jsonc")); m != 0o644 {
				t.Errorf("mode %o; want the existing 644 kept", m)
			}
		}
	})
	t.Run("home.jsonc kind renamed", func(t *testing.T) {
		fx := tlNewFix(t)
		h := filepath.Join(fx.root, "h")
		if err := fx.tlNew(t, WithKind(KindHome, File("meta/home.json"))).InitHome(h, ScopeGlobal); err != nil {
			t.Fatalf("TL3: InitHome with home kind File(\"meta/home.json\"): %v", err)
		}
		if !tlExists(filepath.Join(h, "meta", "home.json")) {
			t.Fatal("renamed home file missing")
		}
	})
}

// ---------------------------------------------------------------------------
// Ensure

// TestLayoutEnsure: Ensure creates a folder kind (or a file kind's parent) with the private mode, creates missing parents restricted, and never changes the mode of folders that already exist.
func TestLayoutEnsure(t *testing.T) {
	t.Run("folder kind 0700 with created parents", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		d := fx.tlNew(t, WithMode(ModeGlobal), WithKind("deep", Folder("a/b/c")))
		p, err := d.Ensure("deep")
		if err != nil || p != filepath.Join(fx.tlGlobal(), "a", "b", "c") {
			t.Fatalf("Ensure = %q, %v", p, err)
		}
		for _, dir := range []string{fx.tlGlobal(), filepath.Join(fx.tlGlobal(), "a"), filepath.Join(fx.tlGlobal(), "a", "b"), p} {
			if m := tlMode(t, dir); m != 0o700 {
				t.Errorf("%s mode %o; want 700", dir, m)
			}
		}
		if m := tlMode(t, fx.home); m != 0o700 {
			t.Errorf("user home mode changed to %o", m)
		}
	})
	t.Run("standard folder kinds in a project", func(t *testing.T) {
		fx := tlNewFix(t)
		m := fx.tlProjectMarker(t)
		d := fx.tlNew(t)
		for _, k := range []Kind{KindShared, KindBackups, KindRun, KindLogs} {
			p, err := d.Ensure(k)
			if err != nil || p != filepath.Join(m, string(k)) || !isDir(p) {
				t.Errorf("Ensure(%s) = %q, %v", k, p, err)
			}
		}
	})
	t.Run("file kind creates the parent only", func(t *testing.T) {
		fx := tlNewFix(t)
		d := fx.tlNew(t, WithMode(ModeGlobal), WithKind("ident", File("auth/id.json")))
		for _, k := range []Kind{KindDB, "ident"} {
			p, err := d.Ensure(k)
			if err != nil {
				t.Fatalf("Ensure(%s): %v", k, err)
			}
			if tlExists(p) {
				t.Errorf("Ensure(%s) created the file %s", k, p)
			}
			if !isDir(filepath.Dir(p)) {
				t.Errorf("Ensure(%s) did not create %s", k, filepath.Dir(p))
			}
		}
	})
	t.Run("existing folder perms untouched", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		run := filepath.Join(fx.tlGlobal(), "run")
		tlMkdir(t, run)
		for _, p := range []string{fx.tlGlobal(), run} {
			if err := os.Chmod(p, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := fx.tlNew(t, WithMode(ModeGlobal)).Ensure(KindRun); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{fx.tlGlobal(), run} {
			if m := tlMode(t, p); m != 0o755 {
				t.Errorf("%s mode %o; want 755 kept", p, m)
			}
		}
	})
	t.Run("existing parent kept, new child restricted", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		tlMkdir(t, fx.tlGlobal())
		if err := os.Chmod(fx.tlGlobal(), 0o755); err != nil {
			t.Fatal(err)
		}
		p, err := fx.tlNew(t, WithMode(ModeGlobal)).Ensure(KindBackups)
		if err != nil {
			t.Fatal(err)
		}
		if tlMode(t, fx.tlGlobal()) != 0o755 || tlMode(t, p) != 0o700 {
			t.Fatalf("modes parent %o child %o", tlMode(t, fx.tlGlobal()), tlMode(t, p))
		}
	})
	t.Run("WithPerm custom modes", func(t *testing.T) {
		tlUnix(t)
		fx := tlNewFix(t)
		d := fx.tlNew(t, WithMode(ModeGlobal), WithPerm(0o750, 0o640))
		p, err := d.Ensure(KindCache)
		if err != nil {
			t.Fatal(err)
		}
		if m := tlMode(t, p); m != 0o750 {
			t.Errorf("cache mode %o; want 750", m)
		}
		if m := tlMode(t, fx.tlGlobal()); m != 0o750 {
			t.Errorf("home mode %o; want 750", m)
		}
		if m := tlMode(t, filepath.Join(p, "CACHEDIR.TAG")); m != 0o640 {
			t.Errorf("CACHEDIR.TAG mode %o; want 640", m)
		}
		h := filepath.Join(fx.root, "h")
		if err := d.InitHome(h, ScopeGlobal); err != nil {
			t.Fatal(err)
		}
		if m := tlMode(t, filepath.Join(h, "home.jsonc")); m != 0o640 {
			t.Errorf("home.jsonc mode %o; want 640", m)
		}
	})
	t.Run("unknown kind", func(t *testing.T) {
		fx := tlNewFix(t)
		if _, err := fx.tlNew(t).Ensure("nope"); !errors.Is(err, ErrUnknownKind) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("project kind outside a project", func(t *testing.T) {
		fx := tlNewFix(t)
		if _, err := fx.tlNew(t).Ensure(KindDB); !errors.Is(err, ErrNoProject) {
			t.Fatalf("err = %v; want ErrNoProject", err)
		}
		if tlExists(fx.tlGlobal()) {
			t.Fatal("a failed Ensure created the global home")
		}
	})
	t.Run("anchored kind with no user home has no path", func(t *testing.T) {
		fx := tlNewFix(t)
		d := fx.tlNew(t, WithHomeDir(func() (string, error) { return "", errors.New("none") }))
		if _, err := d.Ensure(KindConfig); !errors.Is(err, ErrNoHome) {
			t.Fatalf("err = %v; want ErrNoHome", err)
		}
	})
}

// TestLayoutCacheTag: Ensure writes CACHEDIR.TAG once into every disposable folder kind (standard or author-added) and never into other kinds, unless WithCacheTag(false).
func TestLayoutCacheTag(t *testing.T) {
	t.Run("disposable kind tagged once with the exact signature", func(t *testing.T) {
		fx := tlNewFix(t)
		d := fx.tlNew(t, WithMode(ModeGlobal))
		p, err := d.Ensure(KindCache)
		if err != nil {
			t.Fatal(err)
		}
		tag := filepath.Join(p, "CACHEDIR.TAG")
		got := tlRead(t, tag)
		if got != tlCacheTagBody || !strings.HasPrefix(got, "Signature: 8a477f597d28d172789f06886806bc55\n") {
			t.Fatalf("tag = %q", got)
		}
		// Not re-tagged: a user edit survives a second Ensure.
		tlWrite(t, tag, "Signature: 8a477f597d28d172789f06886806bc55\n# edited\n", 0o600)
		if _, err := d.Ensure(KindCache); err != nil {
			t.Fatal(err)
		}
		if got := tlRead(t, tag); !strings.Contains(got, "# edited") {
			t.Fatalf("tag rewritten: %q", got)
		}
	})
	t.Run("existing untagged cache folder gets the tag", func(t *testing.T) {
		fx := tlNewFix(t)
		c := filepath.Join(fx.tlGlobal(), "cache")
		tlMkdir(t, c)
		if _, err := fx.tlNew(t).Ensure(KindCache); err != nil {
			t.Fatal(err)
		}
		if tlRead(t, filepath.Join(c, "CACHEDIR.TAG")) != tlCacheTagBody {
			t.Fatal("no tag on an existing cache folder")
		}
	})
	t.Run("author disposable kind tagged", func(t *testing.T) {
		fx := tlNewFix(t)
		fx.tlProjectMarker(t)
		p, err := fx.tlNew(t, WithKind("thumbs", Folder("t/thumbs"), Disposable())).Ensure("thumbs")
		if err != nil || !tlExists(filepath.Join(p, "CACHEDIR.TAG")) {
			t.Fatalf("Ensure = %q, %v; tag missing", p, err)
		}
		if tlExists(filepath.Join(filepath.Dir(p), "CACHEDIR.TAG")) {
			t.Fatal("parent tagged too")
		}
	})
	t.Run("non-disposable kinds are not tagged", func(t *testing.T) {
		fx := tlNewFix(t)
		p, err := fx.tlNew(t, WithMode(ModeGlobal)).Ensure(KindRun)
		if err != nil || tlExists(filepath.Join(p, "CACHEDIR.TAG")) {
			t.Fatalf("Ensure(run) = %q, %v; tagged=%v", p, err, tlExists(filepath.Join(p, "CACHEDIR.TAG")))
		}
	})
	t.Run("disposable file kind is not tagged", func(t *testing.T) {
		fx := tlNewFix(t)
		p, err := fx.tlNew(t, WithMode(ModeGlobal), WithKind("blob", File("blobs/b.bin"), Disposable())).Ensure("blob")
		if err != nil || tlExists(filepath.Join(filepath.Dir(p), "CACHEDIR.TAG")) {
			t.Fatalf("Ensure(blob) = %q, %v", p, err)
		}
	})
	t.Run("WithCacheTag(false)", func(t *testing.T) {
		fx := tlNewFix(t)
		p, err := fx.tlNew(t, WithCacheTag(false)).Ensure(KindCache)
		if err != nil || tlExists(filepath.Join(p, "CACHEDIR.TAG")) {
			t.Fatalf("Ensure = %q, %v; tag present", p, err)
		}
	})
}

// tlTempRoot is the temp fallback folder for tlApp under tmp.
func tlTempRoot(tmp string) string {
	return filepath.Join(tmp, tlApp+"-"+strconv.Itoa(os.Getuid()))
}

// tlReadOnlyHome makes the user home read-only and restores it at cleanup.
func tlReadOnlyHome(t *testing.T, fx tlFix) {
	t.Helper()
	tlUnix(t)
	if os.Geteuid() == 0 {
		t.Skip("root ignores folder modes")
	}
	if err := os.Chmod(fx.home, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(fx.home, 0o700) })
}

// TestLayoutTempFallback: a disposable kind whose home is unusable falls back to the private temp folder; with the fallback off the error names the folder and the env vars; an unsafe temp folder is ErrUnsafeTemp.
func TestLayoutTempFallback(t *testing.T) {
	t.Run("read-only home: disposable kind falls back to temp", func(t *testing.T) {
		fx := tlNewFix(t)
		t.Setenv("TMPDIR", fx.root)
		tlReadOnlyHome(t, fx)
		d := fx.tlNew(t, WithMode(ModeGlobal))
		p, err := d.Ensure(KindCache)
		want := filepath.Join(tlTempRoot(fx.root), "cache")
		if err != nil || p != want {
			t.Fatalf("Ensure(cache) = %q, %v; want %q", p, err, want)
		}
		if m := tlMode(t, tlTempRoot(fx.root)); m != 0o700 {
			t.Errorf("temp root mode %o", m)
		}
		if tlRead(t, filepath.Join(p, "CACHEDIR.TAG")) != tlCacheTagBody {
			t.Error("fallback cache not tagged")
		}
		// A non-disposable kind does not fall back: it fails naming the fix.
		_, err = d.Ensure(KindRun)
		if !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), tlPrefix+"_HOME") || !strings.Contains(err.Error(), tlPrefix+"_RUN_DIR") {
			t.Fatalf("Ensure(run) err = %v", err)
		}
		// A second call reuses the fallback folder.
		if p2, err := d.Ensure(KindCache); err != nil || p2 != want {
			t.Fatalf("second Ensure = %q, %v", p2, err)
		}
	})
	t.Run("WithTempFallback(false) names the folder and the env vars", func(t *testing.T) {
		fx := tlNewFix(t)
		t.Setenv("TMPDIR", fx.root)
		tlReadOnlyHome(t, fx)
		_, err := fx.tlNew(t, WithMode(ModeGlobal), WithTempFallback(false)).Ensure(KindCache)
		if err == nil {
			t.Fatal("no error")
		}
		for _, s := range []string{filepath.Join(fx.tlGlobal(), "cache"), tlPrefix + "_HOME", tlPrefix + "_CACHE_DIR"} {
			if !strings.Contains(err.Error(), s) {
				t.Errorf("error %q does not name %s", err, s)
			}
		}
		if tlExists(tlTempRoot(fx.root)) {
			t.Error("temp folder created with the fallback off")
		}
	})
	t.Run("no user home: disposable kind uses temp", func(t *testing.T) {
		fx := tlNewFix(t)
		t.Setenv("TMPDIR", fx.root)
		d := fx.tlNew(t, WithHomeDir(func() (string, error) { return "", errors.New("no home") }))
		p, err := d.Ensure(KindCache)
		if err != nil || p != filepath.Join(tlTempRoot(fx.root), "cache") {
			t.Fatalf("Ensure = %q, %v", p, err)
		}
	})
	t.Run("temp fallback folder that is a symlink is ErrUnsafeTemp", func(t *testing.T) {
		fx := tlNewFix(t)
		t.Setenv("TMPDIR", fx.root)
		tlReadOnlyHome(t, fx)
		evil := filepath.Join(fx.root, "evil")
		tlMkdir(t, evil)
		if err := os.Symlink(evil, tlTempRoot(fx.root)); err != nil {
			t.Fatal(err)
		}
		_, err := fx.tlNew(t, WithMode(ModeGlobal)).Ensure(KindCache)
		if !errors.Is(err, ErrUnsafeTemp) {
			t.Fatalf("err = %v; want ErrUnsafeTemp", err)
		}
		if tlExists(filepath.Join(evil, "cache")) {
			t.Fatal("wrote through the planted symlink")
		}
	})
	t.Run("temp fallback path that is a file is ErrUnsafeTemp", func(t *testing.T) {
		fx := tlNewFix(t)
		t.Setenv("TMPDIR", fx.root)
		tlWrite(t, tlTempRoot(fx.root), "x", 0o600)
		d := fx.tlNew(t, WithHomeDir(func() (string, error) { return "", errors.New("no home") }))
		if _, err := d.Ensure(KindCache); !errors.Is(err, ErrUnsafeTemp) {
			t.Fatalf("err = %v; want ErrUnsafeTemp", err)
		}
	})
}

// TestLayoutWriteFailures: a folder the user cannot write into gives an
// error naming the path, never a half-written file or a stray temp file.
func TestLayoutWriteFailures(t *testing.T) {
	ro := func(t *testing.T, p string) {
		t.Helper()
		tlUnix(t)
		if os.Geteuid() == 0 {
			t.Skip("root ignores folder modes")
		}
		if err := os.Chmod(p, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(p, 0o700) })
	}
	t.Run("WriteGitignore in a read-only marker", func(t *testing.T) {
		fx := tlNewFix(t)
		gi := fx.tlGitProject(t)
		tlWrite(t, gi, "hand\n", 0o600)
		ro(t, filepath.Dir(gi))
		ok, err := fx.tlNew(t).WriteGitignore()
		if !ok || !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), gi) {
			t.Fatalf("WriteGitignore = %v, %v", ok, err)
		}
		if tlRead(t, gi) != "hand\n" {
			t.Fatal("file changed")
		}
		tlNoTemps(t, filepath.Dir(gi))
	})
	t.Run("InitHome under a read-only folder", func(t *testing.T) {
		fx := tlNewFix(t)
		parent := filepath.Join(fx.root, "locked")
		tlMkdir(t, parent)
		ro(t, parent)
		err := fx.tlNew(t).InitHome(filepath.Join(parent, "h"), ScopeGlobal)
		if !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), parent) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("InitHome in a read-only existing folder", func(t *testing.T) {
		fx := tlNewFix(t)
		h := filepath.Join(fx.root, "h")
		tlMkdir(t, h)
		ro(t, h)
		err := fx.tlNew(t).InitHome(h, ScopeGlobal)
		if !errors.Is(err, os.ErrPermission) || tlExists(filepath.Join(h, "home.jsonc")) {
			t.Fatalf("err = %v", err)
		}
		tlNoTemps(t, h)
	})
	t.Run("read-only cache folder cannot be tagged", func(t *testing.T) {
		fx := tlNewFix(t)
		c := filepath.Join(fx.tlGlobal(), "cache")
		tlMkdir(t, c)
		ro(t, c)
		_, err := fx.tlNew(t, WithTempFallback(false)).Ensure(KindCache)
		if !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), "CACHEDIR.TAG") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("temp fallback parent cannot be created", func(t *testing.T) {
		fx := tlNewFix(t)
		locked := filepath.Join(fx.root, "locked")
		tlMkdir(t, locked)
		t.Setenv("TMPDIR", filepath.Join(locked, "tmp"))
		ro(t, locked)
		// No user home: the cache goes straight to the temp fallback. New's
		// test guard root is TMPDIR, which no path here is inside, so the
		// guard is off for this one New.
		d := fx.tlNew(t, WithHomeDir(func() (string, error) { return "", errors.New("no home") }), WithTestGuard(false))
		if _, err := d.Ensure(KindCache); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("err = %v", err)
		}
	})
}

// TestLayoutLayoutVersion: a home whose home.jsonc layout is newer than the binary knows refuses writes with ErrLayoutTooNew naming that home; older or equal layouts are written normally.
func TestLayoutLayoutVersion(t *testing.T) {
	cases := []struct {
		name    string
		layout  int
		known   int
		wantErr bool
	}{
		{"same", 1, 1, false},
		{"older home", 1, 2, false},
		{"newer home", 2, 1, true},
		{"newer home, binary knows it", 2, 2, false},
		{"much newer", 9, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := tlNewFix(t)
			tlWrite(t, filepath.Join(fx.tlGlobal(), "home.jsonc"), `{"app": "tlapp", "layout": `+strconv.Itoa(tc.layout)+`, "scope": "global"}`, 0o600)
			d := fx.tlNew(t, WithMode(ModeGlobal), WithLayoutVersion(tc.known))
			for _, k := range []Kind{KindRun, KindCache, KindDB} {
				_, err := d.Ensure(k)
				if tc.wantErr {
					if !errors.Is(err, ErrLayoutTooNew) || !strings.Contains(err.Error(), fx.tlGlobal()) {
						t.Fatalf("Ensure(%s) err = %v; want ErrLayoutTooNew naming %s", k, err, fx.tlGlobal())
					}
					if tlExists(filepath.Join(fx.tlGlobal(), string(k))) {
						t.Fatalf("Ensure(%s) created a folder in a too-new home", k)
					}
				} else if err != nil {
					t.Fatalf("Ensure(%s): %v", k, err)
				}
			}
		})
	}
	t.Run("project home layout too new", func(t *testing.T) {
		fx := tlNewFix(t)
		m := fx.tlProjectMarker(t)
		tlWrite(t, filepath.Join(m, "home.jsonc"), `{"app": "tlapp", "layout": 5, "scope": "project"}`, 0o600)
		if _, err := fx.tlNew(t).Ensure(KindShared); !errors.Is(err, ErrLayoutTooNew) {
			t.Fatalf("err = %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Symlinks

// TestLayoutVerifyFile: VerifyFile accepts missing and plain files and symlinks that stay inside the home, and rejects (ErrSymlinkEscape) links that leave it, including a sibling whose name only starts like the home.
func TestLayoutVerifyFile(t *testing.T) {
	tlUnix(t)
	cases := []struct {
		name string
		// setup makes the db path (inside home h) and returns it.
		setup func(t *testing.T, fx tlFix, h string) string
		want  error
	}{
		{"missing file", func(t *testing.T, fx tlFix, h string) string { return filepath.Join(h, "tlapp.db") }, nil},
		{"plain file", func(t *testing.T, fx tlFix, h string) string {
			p := filepath.Join(h, "tlapp.db")
			tlWrite(t, p, "", 0o600)
			return p
		}, nil},
		{"link inside the home", func(t *testing.T, fx tlFix, h string) string {
			tlWrite(t, filepath.Join(h, "data", "real.db"), "", 0o600)
			p := filepath.Join(h, "tlapp.db")
			tlSymlink(t, filepath.Join("data", "real.db"), p)
			return p
		}, nil},
		{"link outside the home", func(t *testing.T, fx tlFix, h string) string {
			tlWrite(t, filepath.Join(fx.root, "outside.db"), "", 0o600)
			p := filepath.Join(h, "tlapp.db")
			tlSymlink(t, filepath.Join(fx.root, "outside.db"), p)
			return p
		}, ErrSymlinkEscape},
		{"relative link climbing out", func(t *testing.T, fx tlFix, h string) string {
			tlWrite(t, filepath.Join(fx.home, "x.db"), "", 0o600)
			p := filepath.Join(h, "tlapp.db")
			tlSymlink(t, "../x.db", p)
			return p
		}, ErrSymlinkEscape},
		{"link into a sibling named like the home", func(t *testing.T, fx tlFix, h string) string {
			tlWrite(t, h+"-evil/x.db", "", 0o600)
			p := filepath.Join(h, "tlapp.db")
			tlSymlink(t, h+"-evil/x.db", p)
			return p
		}, ErrSymlinkEscape},
		{"broken link", func(t *testing.T, fx tlFix, h string) string {
			p := filepath.Join(h, "tlapp.db")
			tlSymlink(t, filepath.Join(h, "gone.db"), p)
			return p
		}, ErrSymlinkEscape},
		{"link to a link inside", func(t *testing.T, fx tlFix, h string) string {
			tlWrite(t, filepath.Join(h, "a.db"), "", 0o600)
			tlSymlink(t, filepath.Join(h, "a.db"), filepath.Join(h, "b.db"))
			p := filepath.Join(h, "tlapp.db")
			tlSymlink(t, filepath.Join(h, "b.db"), p)
			return p
		}, nil},
	}
	for _, homeLinked := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(tc.name+" home-symlinked="+strconv.FormatBool(homeLinked), func(t *testing.T) {
				fx := tlNewFix(t)
				h := fx.tlGlobal()
				if homeLinked {
					// repolink-style: ~/.tlapp is a link to a private folder.
					real := filepath.Join(fx.root, "private", "tl-home")
					tlMkdir(t, real)
					tlSymlink(t, real, h)
				} else {
					tlMkdir(t, h)
				}
				p := tc.setup(t, fx, h)
				d := fx.tlNew(t, WithMode(ModeGlobal))
				err := d.VerifyFile(p)
				if tc.want == nil && err != nil {
					t.Fatalf("VerifyFile: %v", err)
				}
				if tc.want != nil && !errors.Is(err, tc.want) {
					t.Fatalf("VerifyFile err = %v; want %v", err, tc.want)
				}
				// Ensure applies the same rule to file kinds.
				_, eerr := d.Ensure(KindDB)
				if (tc.want == nil) != (eerr == nil) || (tc.want != nil && !errors.Is(eerr, tc.want)) {
					t.Fatalf("Ensure(db) err = %v; want %v", eerr, tc.want)
				}
			})
		}
	}
	t.Run("file in a project home linking into the global home escapes", func(t *testing.T) {
		fx := tlNewFix(t)
		m := fx.tlProjectMarker(t)
		tlWrite(t, filepath.Join(fx.tlGlobal(), "tlapp.db"), "", 0o600)
		p := filepath.Join(m, "tlapp.db")
		tlSymlink(t, filepath.Join(fx.tlGlobal(), "tlapp.db"), p)
		if err := fx.tlNew(t).VerifyFile(p); !errors.Is(err, ErrSymlinkEscape) {
			t.Fatalf("err = %v; want ErrSymlinkEscape", err)
		}
	})
	t.Run("file outside every home checks against its own folder", func(t *testing.T) {
		fx := tlNewFix(t)
		dir := filepath.Join(fx.root, "loose")
		tlWrite(t, filepath.Join(dir, "a"), "", 0o600)
		tlSymlink(t, filepath.Join(dir, "a"), filepath.Join(dir, "b"))
		tlSymlink(t, filepath.Join(fx.root, "x"), filepath.Join(dir, "c"))
		tlWrite(t, filepath.Join(fx.root, "x"), "", 0o600)
		d := fx.tlNew(t)
		if err := d.VerifyFile(filepath.Join(dir, "b")); err != nil {
			t.Fatalf("b: %v", err)
		}
		if err := d.VerifyFile(filepath.Join(dir, "c")); !errors.Is(err, ErrSymlinkEscape) {
			t.Fatalf("c: %v", err)
		}
	})
}

func tlSymlink(t *testing.T, target, link string) {
	t.Helper()
	tlMkdir(t, filepath.Dir(link))
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
