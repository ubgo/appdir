package appdir

// promise_d_test.go: promises of SPEC sections 8 to 14 (profiles, release
// detection, names, the prod-db guard, env leaks, another profile, the
// legacy move, the test guard, paths --json, the platform table, decisions)
// and of docs/profiles, docs/runtime/legacy-move.md and docs/testing that no
// earlier test proved. Helpers start with "pd"; the ttapp world and its
// helpers (ttWorld, ttNew, ttMustNew, ...) come from runtime_test.go.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// pdTree lists every path under root (relative, slash-separated), for
// "nothing was created" checks.
func pdTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestPDUnknownProfileNamesSource proves docs/profiles/profiles.md: the
// ErrUnknownProfile text names where the name came from (--profile or the
// <APP>_PROFILE variable) and the known profiles (SPEC 8.1).
func TestPDUnknownProfileNamesSource(t *testing.T) {
	f := ttWorld(t)
	cases := []struct {
		name string
		env  map[string]string
		opts []Option
		want string
	}{
		{"flag", nil, []Option{WithProfile("qa")}, `appdir: --profile: unknown profile "qa" (known: prod, dev)`},
		{"env", map[string]string{ttPrefix + "_PROFILE": "qa"}, nil, `appdir: TTAPP_PROFILE: unknown profile "qa" (known: prod, dev)`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ttNew(t, f, c.env, append([]Option{WithRelease(false)}, c.opts...)...)
			ttWantErr(t, err, ErrUnknownProfile)
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q lacks %q", err, c.want)
			}
		})
	}
}

// TestPDProfilesListWithoutDefaults proves docs/profiles/profiles.md:
// WithProfiles replaces the list, so WithProfiles("staging") alone fails in
// New because the release and dev profiles are no longer in it.
func TestPDProfilesListWithoutDefaults(t *testing.T) {
	f := ttWorld(t)
	_, err := ttNew(t, f, nil, WithRelease(false), WithProfiles("staging"))
	ttWantErr(t, err, ErrInvalidOption)
}

// TestPDForProfileKeepsReleaseDecision proves docs/profiles/release-detection.md:
// IsRelease and ReleaseReason are set on every Dirs, including one from
// ForProfile, which keeps the build's decision (SPEC 8.2, 8.6).
func TestPDForProfileKeepsReleaseDecision(t *testing.T) {
	f := ttWorld(t)
	rel := ttMustNew(t, f, nil, ttBI("v1.4.2"))
	o, err := rel.ForProfile(ProfileDev)
	if err != nil {
		t.Fatal(err)
	}
	if !o.IsRelease() || o.ReleaseReason() != "go-install" || o.Profile() != ProfileDev {
		t.Fatalf("release build ForProfile(dev): release %v reason %q profile %q", o.IsRelease(), o.ReleaseReason(), o.Profile())
	}
	dev := ttMustNew(t, f, nil, WithRelease(false))
	o, err = dev.ForProfile(ProfileProd)
	if err != nil {
		t.Fatal(err)
	}
	if o.IsRelease() || o.ReleaseReason() != "dev: flag" || o.Profile() != ProfileProd {
		t.Fatalf("dev build ForProfile(prod): release %v reason %q profile %q", o.IsRelease(), o.ReleaseReason(), o.Profile())
	}
}

// TestPDTestBinaryIsDev proves docs/profiles/release-detection.md and
// docs/testing/for-test.md: a test binary has no tagged version, so with no
// WithRelease and no WithBuildInfo it is a dev build with the -dev home
// (SPEC 8.2: everything not clearly a release is dev).
func TestPDTestBinaryIsDev(t *testing.T) {
	f := ttWorld(t)
	d := ttMustNew(t, f, nil, WithMode(ModeGlobal))
	if d.IsRelease() || d.Profile() != ProfileDev || !strings.HasPrefix(d.ReleaseReason(), "dev: ") {
		t.Fatalf("test binary: release %v profile %q reason %q", d.IsRelease(), d.Profile(), d.ReleaseReason())
	}
	if d.Global().Base != filepath.Join(f.home, ".ttapp-dev") {
		t.Fatalf("global home = %q", d.Global().Base)
	}
}

// TestPDStdlibOnlyModule proves SPEC 8.2 "stdlib only" (release detection
// reads runtime/debug, no buildinfo package) and SPEC 14 decision 5 (the
// module is github.com/ubgo/appdir): go.mod requires nothing, and every
// non-test file imports only the standard library or appdir's own
// internal packages.
func TestPDStdlibOnlyModule(t *testing.T) {
	fh, err := os.Open("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fh.Close() }()
	sc := bufio.NewScanner(fh)
	var module string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m, ok := strings.CutPrefix(line, "module "); ok {
			module = strings.TrimSpace(m)
		}
		if strings.HasPrefix(line, "require") {
			t.Errorf("go.mod has a dependency: %q", line)
		}
	}
	if module != "github.com/ubgo/appdir" {
		t.Fatalf("module = %q, want github.com/ubgo/appdir", module)
	}
	var files []string
	for _, pat := range []string{"*.go", "internal/*/*.go"} {
		m, _ := filepath.Glob(pat)
		files = append(files, m...)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			first, _, _ := strings.Cut(p, "/")
			if strings.Contains(first, ".") && !strings.HasPrefix(p, module+"/internal/") {
				t.Errorf("%s imports %s, not the standard library", f, p)
			}
		}
	}
	// The release rule reads Go's own build info by default.
	if _, ok := debug.ReadBuildInfo(); !ok {
		t.Log("no build info in this test binary; the default seam still compiles against runtime/debug")
	}
}

// TestPDNameDoesNotValidate proves docs/profiles/names.md: Name adds the
// profile suffix and does not check the result; the release profile keeps
// the base as it is (SPEC 8.3).
func TestPDNameDoesNotValidate(t *testing.T) {
	f := ttWorld(t)
	dev := ttMustNew(t, f, nil, WithRelease(false))
	if got := dev.Name("bad label!"); got != "bad label!-dev" {
		t.Fatalf("Name = %q", got)
	}
	if got := dev.NameWith("com.org.x", "."); got != "com.org.x.dev" {
		t.Fatalf("NameWith = %q", got)
	}
	rel := ttMustNew(t, f, nil, WithRelease(true))
	if got := rel.Name("bad label!"); got != "bad label!" {
		t.Fatalf("release Name = %q", got)
	}
}

// TestPDCheckDBByBuildNotProfile proves docs/profiles/prod-db-guard.md and
// SPEC 8.4: the guard is about the build, not the profile. A dev build with
// --profile prod is refused on the prod db; a release build with --profile
// dev never is; a dev build with --home elsewhere is still refused on
// ~/.<app>/<app>.db, because the prod view has no --home.
func TestPDCheckDBByBuildNotProfile(t *testing.T) {
	cases := []struct {
		name    string
		release bool
		opts    func(f ttFix) []Option
		err     error
	}{
		{"dev-build-profile-prod", false, func(ttFix) []Option { return []Option{WithProfile(string(ProfileProd))} }, ErrProdDB},
		{"release-build-profile-dev", true, func(ttFix) []Option { return []Option{WithProfile(string(ProfileDev))} }, nil},
		{"dev-build-home-elsewhere", false, func(f ttFix) []Option { return []Option{WithHome(filepath.Join(f.root, "x"))} }, ErrProdDB},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := ttWorld(t)
			prod := filepath.Join(f.home, "."+ttApp, ttApp+".db")
			ttWrite(t, prod, "sqlite")
			d := ttMustNew(t, f, nil, append([]Option{WithRelease(c.release)}, c.opts(f)...)...)
			acc, err := d.CheckDB(prod)
			ttWantErr(t, err, c.err)
			if acc != AccessReadWrite {
				t.Fatalf("access = %v", acc)
			}
		})
	}
}

// TestPDCheckDBProdViewUnguarded proves docs/testing/for-test.md: CheckDB
// resolves its prod view with the test guard off (the view is only
// compared, never written). Here the main view lives in the test root
// (<APP>_HOME) while the user home, which only the prod view uses, is
// outside it: CheckDB must not panic and must still recognise the prod db.
func TestPDCheckDBProdViewUnguarded(t *testing.T) {
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	t.Setenv(envTestRoot, root)
	outsideHome := filepath.VolumeName(root) + string(filepath.Separator) + filepath.Join("pd-not-a-temp-folder", "home")
	env := map[string]string{ttPrefix + "_HOME": filepath.Join(root, "apphome")}
	d, err := New(ttApp, WithHomeDir(func() (string, error) { return outsideHome, nil }), WithWorkDir(root),
		ttEnvOpt(env), WithRelease(false), WithMode(ModeGlobal))
	if err != nil {
		t.Fatal(err)
	}
	var acc DBAccess
	msg := ttPanic(func() { acc, err = d.CheckDB(filepath.Join(root, "apphome", ttApp+".db")) })
	if msg != "" || err != nil || acc != AccessReadWrite {
		t.Fatalf("own db: panic %q err %v access %v", msg, err, acc)
	}
	msg = ttPanic(func() { _, err = d.CheckDB(filepath.Join(outsideHome, "."+ttApp, ttApp+".db")) })
	if msg != "" || !errors.Is(err, ErrProdDB) {
		t.Fatalf("prod db: panic %q err %v", msg, err)
	}
}

// TestPDEnvLeakOnlyGlobalHome proves docs/profiles/child-processes.md: the
// env-leak filter compares against the prod global home only; an
// <APP>_HOME pointing at a prod project home is used, with no warning
// (SPEC 8.5), and the prod-db guard still refuses that project's prod db.
func TestPDEnvLeakOnlyGlobalHome(t *testing.T) {
	f := ttWorld(t)
	projHome := filepath.Join(f.proj, "."+ttApp)
	d := ttMustNew(t, f, map[string]string{ttPrefix + "_HOME": projHome}, WithRelease(false))
	if d.Global().Base != projHome {
		t.Fatalf("global = %q, want %q", d.Global().Base, projHome)
	}
	if slices.Contains(ttWarnCodes(d), WarnEnvPointsToProd) {
		t.Fatalf("warnings = %v", d.Warnings())
	}
	if _, err := d.CheckDB(filepath.Join(projHome, ttApp+".db")); !errors.Is(err, ErrProdDB) {
		t.Fatalf("CheckDB(prod project db) = %v, want ErrProdDB", err)
	}
}

// TestPDForProfileCreatesNothing proves SPEC 8.6 and docs/profiles/names.md:
// ForProfile only resolves paths; it opens nothing and creates nothing.
func TestPDForProfileCreatesNothing(t *testing.T) {
	f := ttWorld(t)
	before := pdTree(t, f.root)
	d := ttMustNew(t, f, nil, WithRelease(false))
	for _, p := range []Profile{ProfileProd, ProfileDev} {
		o, err := d.ForProfile(p)
		if err != nil {
			t.Fatal(err)
		}
		_ = o.Paths()
		_, _ = o.RunPath(RunDiscovery)
		_, _ = o.Path(KindDB)
		_ = o.Name("svc")
	}
	if after := pdTree(t, f.root); !slices.Equal(before, after) {
		t.Fatalf("ForProfile created files:\nbefore %v\nafter  %v", before, after)
	}
}

// TestPDLegacyOnlyGlobalHomeMoves proves SPEC 9 step 1: only the global home
// is ever moved; a project folder is never moved, even one with the legacy
// name, and a project-mode release still moves ~/<Dir> to the global home.
func TestPDLegacyOnlyGlobalHomeMoves(t *testing.T) {
	ctx := context.Background()
	f := ttWorld(t)
	projOld := filepath.Join(f.proj, ttOld)
	ttWrite(t, filepath.Join(projOld, "keep.txt"), "project data")
	opts := []Option{WithRelease(true), WithMode(ModeProject), WithLegacyHome(LegacyHome{Dir: ttOld})}
	res, err := ttMustNew(t, f, nil, opts...).MoveLegacy(ctx)
	if err != nil || res.Status != MoveNone || res.To != filepath.Join(f.home, "."+ttApp) {
		t.Fatalf("no global legacy home: %+v, %v", res, err)
	}
	ttWrite(t, filepath.Join(f.home, ttOld, "data.txt"), "user data")
	res, err = ttMustNew(t, f, nil, opts...).MoveLegacy(ctx)
	if err != nil || res.Status != MoveDone || res.To != filepath.Join(f.home, "."+ttApp) {
		t.Fatalf("global legacy home: %+v, %v", res, err)
	}
	if ttRead(t, filepath.Join(f.home, "."+ttApp, "data.txt")) != "user data" {
		t.Fatal("global data not moved to the global home")
	}
	if ttRead(t, filepath.Join(projOld, "keep.txt")) != "project data" || ttExists(filepath.Join(projOld, movedFile)) {
		t.Fatal("project folder touched")
	}
	if ttExists(filepath.Join(f.proj, "."+ttApp, "data.txt")) {
		t.Fatal("legacy data moved into the project home")
	}
}

// TestPDLegacyLockNoSidecar proves the SPEC 9 lock choice: the moving lock
// is one file, <parent>/<base(new)>.moving.lock, kept after the move, owner
// only (0600 on Unix), with no .fence sidecar or any other stray file in
// the user's home.
func TestPDLegacyLockNoSidecar(t *testing.T) {
	l := ttLegacyWorld(t)
	res, err := l.dirs(t, nil).MoveLegacy(context.Background())
	if err != nil || res.Status != MoveDone {
		t.Fatalf("res = %+v, %v", res, err)
	}
	ents, err := os.ReadDir(l.home)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	want := []string{"." + ttApp, "." + ttApp + ".moving.lock", ttOld}
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("home holds %v, want %v", names, want)
	}
	if os.PathSeparator == '/' {
		fi, err := os.Stat(filepath.Join(l.home, "."+ttApp+".moving.lock"))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("lock file mode: %v %v", fi, err)
		}
	}
}

// TestPDLegacyUnknownDeviceCopies proves docs/runtime/legacy-move.md: when
// the device ids differ or cannot be read, the move is treated as cross-disk
// (copy, never a blind rename); the same folder is the same disk.
func TestPDLegacyUnknownDeviceCopies(t *testing.T) {
	root := t.TempDir()
	if !sameDevice(root, root) {
		t.Fatal("a folder is not on its own disk")
	}
	if sameDevice(root, filepath.Join(root, "missing")) || sameDevice(filepath.Join(root, "missing"), root) {
		t.Fatal("an unreadable device id counted as the same disk")
	}
}

// TestPDPathsJSONOrder proves SPEC 11: Paths() lists entries in kind order,
// global before project for a both kind; json.Marshal gives kind, path,
// rule, fallback, home, type, git (note and error when set); under
// <APP>_HOME a both kind appears once, labelled global.
func TestPDPathsJSONOrder(t *testing.T) {
	f := ttWorld(t)
	d := ttMustNew(t, f, nil, WithRelease(true))
	order := map[Kind]int{}
	for i, k := range d.Kinds() {
		order[k.Name] = i
	}
	es := d.Paths()
	last := -1
	seen := map[Kind][]Scope{}
	for _, e := range es {
		if order[e.Kind] < last {
			t.Fatalf("%s listed after a later kind: %+v", e.Kind, es)
		}
		last = order[e.Kind]
		seen[e.Kind] = append(seen[e.Kind], e.Home)
	}
	for _, k := range d.Kinds() {
		if k.Scope == ScopeBoth && !slices.Equal(seen[k.Name], []Scope{ScopeGlobal, ScopeProject}) {
			t.Errorf("both kind %s homes = %v, want global then project", k.Name, seen[k.Name])
		}
	}
	b, err := json.Marshal(es)
	if err != nil {
		t.Fatal(err)
	}
	var objs []map[string]any
	if err := json.Unmarshal(b, &objs); err != nil {
		t.Fatal(err)
	}
	for _, o := range objs {
		for _, key := range []string{"kind", "path", "rule", "fallback", "home", "type", "git"} {
			if _, ok := o[key]; !ok {
				t.Errorf("entry %v lacks %q", o, key)
			}
		}
		for key := range o {
			if !slices.Contains([]string{"kind", "path", "rule", "fallback", "home", "type", "git", "note", "error"}, key) {
				t.Errorf("unexpected key %q in %v", key, o)
			}
		}
	}
	// A broken project line carries "error".
	ttWrite(t, filepath.Join(f.proj, "."+ttApp, "config.local.jsonc"), `{"home": "../../nope"}`)
	b, _ = json.Marshal(ttMustNew(t, f, nil, WithRelease(true)).Paths())
	if !strings.Contains(string(b), `"error":`) {
		t.Errorf("broken pointer: no error key in %s", b)
	}
	if err := os.Remove(filepath.Join(f.proj, "."+ttApp, "config.local.jsonc")); err != nil {
		t.Fatal(err)
	}
	o := ttMustNew(t, f, map[string]string{ttPrefix + "_HOME": filepath.Join(f.root, "h")}, WithRelease(true))
	n := map[Kind]int{}
	both := map[Kind]bool{}
	for _, k := range o.Kinds() {
		both[k.Name] = k.Scope == ScopeBoth
	}
	for _, e := range o.Paths() {
		n[e.Kind]++
		if both[e.Kind] && e.Home != ScopeGlobal {
			t.Errorf("override: %s labelled %s", e.Kind, e.Home)
		}
	}
	if n[KindConfig] != 1 {
		t.Errorf("override: config listed %d times", n[KindConfig])
	}
}

// TestPDAbsolutePathsOnly proves the SPEC 12 long-paths row: appdir only
// passes absolute paths (so Go's os adds \\?\ on Windows), even when the
// user gives relative ones in --home or <APP>_<KIND>_DIR.
func TestPDAbsolutePathsOnly(t *testing.T) {
	f := ttWorld(t)
	for _, c := range []struct {
		name string
		env  map[string]string
		opts []Option
	}{
		{"defaults", nil, nil},
		{"relative-kind-dir", map[string]string{ttPrefix + "_DB_DIR": "dbs", ttPrefix + "_CACHE_DIR": filepath.Join("a", "..", "c")}, nil},
		{"relative-home-flag", nil, []Option{WithHome(filepath.Join("rel", "h"))}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := ttMustNew(t, f, c.env, append([]Option{WithRelease(false)}, c.opts...)...)
			paths := []string{d.Global().Base, d.Global().Data}
			for _, e := range d.Paths() {
				paths = append(paths, e.Path)
			}
			if p, err := d.RunPath(RunDiscovery); err == nil {
				paths = append(paths, p)
			}
			for _, p := range paths {
				if p != "" && (!filepath.IsAbs(p) || filepath.Clean(p) != p) {
					t.Errorf("path %q is not absolute and clean", p)
				}
			}
		})
	}
}

// TestPDNeverMakesLinks proves the SPEC 12 symlinks row "appdir never makes
// links": no non-test file calls os.Symlink or os.Link, except copyTree in
// legacy.go, which re-creates a link the user already had when a legacy home
// is copied to another disk (SPEC 9 step 7, "symlinks as links").
func TestPDNeverMakesLinks(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range af.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "os" || (sel.Sel.Name != "Symlink" && sel.Sel.Name != "Link") {
					return true
				}
				if f != "legacy.go" || fn.Name.Name != "copyTree" {
					t.Errorf("%s: %s calls os.%s; appdir never makes links", fset.Position(sel.Pos()), fn.Name.Name, sel.Sel.Name)
				}
				return true
			})
		}
	}
}

// TestPDForTestUnsetsEveryAppVar proves SPEC 10: ForTest unsets every
// <APP>_* variable other than <APP>_HOME (here <APP>_CEILING_DIRS, a
// <APP>_<KIND>_DIR and one the library does not know), and restores them
// when the test ends.
func TestPDForTestUnsetsEveryAppVar(t *testing.T) {
	vars := []string{ttPrefix + "_CEILING_DIRS", ttPrefix + "_DB_DIR", ttPrefix + "_CACHE_DIR", ttPrefix + "_SOMETHING_NEW"}
	for _, k := range vars {
		t.Setenv(k, "/leak")
	}
	t.Run("isolated", func(t *testing.T) {
		root := ForTest(t, ttApp)
		for _, k := range vars {
			if v, ok := os.LookupEnv(k); ok {
				t.Errorf("%s still set to %q", k, v)
			}
		}
		if os.Getenv(ttPrefix+"_HOME") != filepath.Join(root, "apphome") {
			t.Errorf("%s_HOME = %q", ttPrefix, os.Getenv(ttPrefix+"_HOME"))
		}
	})
	for _, k := range vars {
		if os.Getenv(k) != "/leak" {
			t.Errorf("%s not restored", k)
		}
	}
}

// TestPDWorkflowManualOnly proves docs/testing/ci.md: the real-OS workflow
// has one trigger, workflow_dispatch (never push, pull request or a
// schedule); its proof step runs the platform tests by name and allows only
// the two documented skips; the cross-disk step fails on a skipped XDisk
// test.
func TestPDWorkflowManualOnly(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(".github", "workflows", "test.yml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	var triggers []string
	in := false
	for _, l := range lines {
		switch {
		case l == "on:":
			in = true
		case in && l != "" && !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "#"):
			in = false
		case in && strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") && !strings.HasPrefix(strings.TrimSpace(l), "#"):
			triggers = append(triggers, strings.TrimSuffix(strings.TrimSpace(l), ":"))
		}
	}
	if !slices.Equal(triggers, []string{"workflow_dispatch"}) {
		t.Fatalf("workflow triggers = %v, want only workflow_dispatch", triggers)
	}
	body := string(b)
	for _, want := range []string{
		"TestWin", "TestDarwin", "TestNoProcessStart$", "TestAtomicWriteOverExisting$", "TestForTestPointsTempIntoRoot$",
		"TestTryLockAcrossProcesses$",
		"allowed='TestWinUNCHome|TestDarwinSocketFallbackLiteralTmp'",
		"fail-fast: false",
		"-run XDisk -v",
		"an XDisk test was skipped",
		"no XDisk test passed",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("workflow lacks %q", want)
		}
	}
	for _, img := range []string{"ubuntu-latest", "macos-latest", "macos-15-intel", "windows-latest"} {
		if !strings.Contains(body, img) {
			t.Errorf("workflow matrix lacks %s", img)
		}
	}
}
