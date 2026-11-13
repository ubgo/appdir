package appdir

// Input-space matrices for resolution (SPEC 4, 6.1; standard sections 1, 3,
// 4). Each row names who runs the tool, where they stand and what is set,
// then states the path, Rule and Fallback a user would see in `paths`, or
// the error they would read (checked with errors.Is). Every helper, type and
// const here carries the tr prefix so it never collides with another test
// file of package appdir.

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// trApp is the app every test here resolves.
const trApp = "trapp"

// trMarker is the project marker folder name of trApp.
const trMarker = "." + trApp

// trDBFile is the db kind's file name for trApp.
const trDBFile = trApp + ".db"

// trHomeJSON is a home.jsonc that makes a folder a home of trApp.
const trHomeJSON = `{ "app": "trapp", "layout": 1 }`

// trEnv turns a map into a WithEnv lookup: a test never reads the process env.
func trEnv(m map[string]string) Option {
	return WithEnv(func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	})
}

// trHome is a WithHomeDir seam returning dir.
func trHome(dir string) Option {
	return WithHomeDir(func() (string, error) { return dir, nil })
}

// trNoHome is a WithHomeDir seam for a machine with no $HOME.
func trNoHome() Option {
	return WithHomeDir(func() (string, error) { return "", errors.New("$HOME is not defined") })
}

// trRoot returns a fresh temp root with symlinks resolved, so expected paths
// compare equal to the real paths the lookup walks.
func trRoot(t *testing.T) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// trMkdir creates folders.
func trMkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

// trWrite writes a file, creating its folder.
func trWrite(t *testing.T, path, body string) {
	t.Helper()
	trMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// trSymlink makes link point at target, skipping the test where the OS
// refuses symlinks (Windows without developer mode).
func trSymlink(t *testing.T, target, link string) {
	t.Helper()
	trMkdir(t, filepath.Dir(link))
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// trInitHome makes dir a home of trApp (home.jsonc only).
func trInitHome(t *testing.T, dir string) {
	t.Helper()
	trWrite(t, filepath.Join(dir, "home.jsonc"), trHomeJSON)
}

// trWorktree makes wt a linked worktree of the checkout main, the way
// `git worktree add` lays it out: main/.git/worktrees/<name>/commondir holds
// "../.." and wt/.git is a file with the gitdir: line.
func trWorktree(t *testing.T, main, wt string) {
	t.Helper()
	admin := filepath.Join(main, gitEntry, "worktrees", filepath.Base(wt))
	trWrite(t, filepath.Join(admin, "commondir"), "../..\n")
	trWrite(t, filepath.Join(admin, "gitdir"), filepath.Join(wt, gitEntry)+"\n")
	trWrite(t, filepath.Join(wt, gitEntry), "gitdir: "+admin+"\n")
}

// trSubmodule makes sub a submodule of the checkout super, the way git lays
// it out: sub/.git is a file pointing at super/.git/modules/<name>, which has
// no commondir.
func trSubmodule(t *testing.T, super, sub string) {
	t.Helper()
	gitdir := filepath.Join(super, gitEntry, "modules", filepath.Base(sub))
	trWrite(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/main\n")
	rel, err := filepath.Rel(sub, gitdir)
	if err != nil {
		t.Fatal(err)
	}
	trWrite(t, filepath.Join(sub, gitEntry), "gitdir: "+filepath.ToSlash(rel)+"\n")
}

// trTryNew builds Dirs for trApp with the seams every test must pass; extra
// options come after and win. A release build (prod) unless extra says
// otherwise.
func trTryNew(home Option, work string, env map[string]string, extra ...Option) (*Dirs, error) {
	opts := []Option{home, WithWorkDir(work), trEnv(env), WithRelease(true)}
	return New(trApp, append(opts, extra...)...)
}

// trNew is trTryNew failing the test on error.
func trNew(t *testing.T, home Option, work string, env map[string]string, extra ...Option) *Dirs {
	t.Helper()
	d, err := trTryNew(home, work, env, extra...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

// trWant is what a user sees for one kind: the path, the rule that picked it
// and whether it fell back, or the error (a sentinel for errors.Is).
type trWant struct {
	path     string
	rule     Rule
	fallback bool
	err      error
}

// trCheckKind checks Path(k) and the entry behind it against w.
func trCheckKind(t *testing.T, d *Dirs, k Kind, w trWant) {
	t.Helper()
	p, err := d.Path(k)
	e, eerr := d.entry(k)
	if eerr != nil {
		t.Fatalf("%s: entry: %v", k, eerr)
	}
	if w.err != nil {
		if !errors.Is(err, w.err) {
			t.Errorf("%s: Path() = %q, %v; want error %v", k, p, err, w.err)
		}
		if e.Error == "" {
			t.Errorf("%s: entry has no Error text for paths --json", k)
		}
		if w.rule != "" && e.Rule != w.rule {
			t.Errorf("%s: failing rule = %q, want %q", k, e.Rule, w.rule)
		}
		return
	}
	if err != nil {
		t.Errorf("%s: Path() error %v, want %q", k, err, w.path)
		return
	}
	if p != w.path || e.Rule != w.rule || e.Fallback != w.fallback {
		t.Errorf("%s: got %q rule=%s fallback=%v; want %q rule=%s fallback=%v", k, p, e.Rule, e.Fallback, w.path, w.rule, w.fallback)
	}
}

// trCheckKinds runs trCheckKind for every entry of want.
func trCheckKinds(t *testing.T, d *Dirs, want map[Kind]trWant) {
	t.Helper()
	keys := make([]Kind, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		trCheckKind(t, d, k, want[k])
	}
}

// trWarnCount counts d's warnings with code.
func trWarnCount(d *Dirs, code WarningCode) int {
	n := 0
	for _, w := range d.Warnings() {
		if w.Code == code {
			n++
		}
	}
	return n
}

// trWarnMessage returns the first warning message with code, or "".
func trWarnMessage(d *Dirs, code WarningCode) string {
	for _, w := range d.Warnings() {
		if w.Code == code {
			return w.Message
		}
	}
	return ""
}

// trCheckWarnings fails unless d has exactly the warning codes in want (in
// any order, each once).
func trCheckWarnings(t *testing.T, d *Dirs, want ...WarningCode) {
	t.Helper()
	var got []WarningCode
	for _, w := range d.Warnings() {
		got = append(got, w.Code)
	}
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("warnings = %v (%v), want %v", got, d.Warnings(), want)
	}
}

// trLookupWorld is the folder tree the lookup matrix stands in. Nothing in it
// changes after it is built, so every row reads the same tree.
type trLookupWorld struct {
	root, home, global string
	shop, shopM        string // a git checkout with a project home
	innerM, sub2M      string // nested repo / submodule with their own home
	places             map[string]string
	symlinks           bool
}

// trBuildLookupWorld lays out a user's machine:
//
//	home/                          $HOME, with the global home .trapp/
//	home/plain/notes               a folder under $HOME, no git, no home
//	home/projects/shop/            git checkout (.git folder) with .trapp/
//	  src/api                      a deep subfolder
//	  vendor/lib/                  nested repo (.git folder), no home
//	  vendor/inner/                nested repo with its own .trapp/
//	  mods/sub/src                 submodule (.git file, no commondir), no home
//	  mods/sub2/                   submodule with its own .trapp/
//	home/projects/shop-fix/src/api linked worktree of shop
//	home/link -> shop/src, home/linkroot -> shop
//	outside/deep                   no git anywhere, not under $HOME
func trBuildLookupWorld(t *testing.T) trLookupWorld {
	t.Helper()
	root := trRoot(t)
	home := filepath.Join(root, "home")
	w := trLookupWorld{root: root, home: home, global: filepath.Join(home, trMarker)}
	trInitHome(t, w.global)
	trMkdir(t, filepath.Join(home, "."+trApp+"-dev"))
	w.shop = filepath.Join(home, "projects", "shop")
	w.shopM = filepath.Join(w.shop, trMarker)
	trMkdir(t, filepath.Join(w.shop, gitEntry), w.shopM, filepath.Join(w.shop, "src", "api"))
	trMkdir(t, filepath.Join(w.shop, "vendor", "lib", gitEntry))
	inner := filepath.Join(w.shop, "vendor", "inner")
	w.innerM = filepath.Join(inner, trMarker)
	trMkdir(t, filepath.Join(inner, gitEntry), w.innerM, filepath.Join(inner, "pkg"))
	sub := filepath.Join(w.shop, "mods", "sub")
	trSubmodule(t, w.shop, sub)
	trMkdir(t, filepath.Join(sub, "src"))
	sub2 := filepath.Join(w.shop, "mods", "sub2")
	trSubmodule(t, w.shop, sub2)
	w.sub2M = filepath.Join(sub2, trMarker)
	trMkdir(t, w.sub2M)
	fix := filepath.Join(home, "projects", "shop-fix")
	trWorktree(t, w.shop, fix)
	trMkdir(t, filepath.Join(fix, "src", "api"))
	trMkdir(t, filepath.Join(home, "plain", "notes"), filepath.Join(root, "outside", "deep"))
	w.places = map[string]string{
		"project root":                  w.shop,
		"deep subfolder":                filepath.Join(w.shop, "src", "api"),
		"$HOME itself":                  home,
		"plain folder under $HOME":      filepath.Join(home, "plain", "notes"),
		"outside any project":           filepath.Join(root, "outside", "deep"),
		"linked worktree root":          fix,
		"linked worktree subfolder":     filepath.Join(fix, "src", "api"),
		"nested repo without a home":    filepath.Join(w.shop, "vendor", "lib"),
		"nested repo with its own home": filepath.Join(inner, "pkg"),
		"submodule without a home":      filepath.Join(sub, "src"),
		"submodule with its own home":   sub2,
	}
	w.symlinks = os.Symlink(filepath.Join(w.shop, "src"), filepath.Join(home, "link")) == nil &&
		os.Symlink(w.shop, filepath.Join(home, "linkroot")) == nil
	if w.symlinks {
		w.places["symlink into a project subfolder"] = filepath.Join(home, "link")
		w.places["symlink to a project root"] = filepath.Join(home, "linkroot")
	}
	return w
}

// trKindsFor is what db, cache, logs and config resolve to for a user whose
// project lookup found marker (rule) or nothing (""), in mode.
func trKindsFor(mode Mode, global, marker string, rule Rule) map[Kind]trWant {
	want := map[Kind]trWant{
		KindCache: {path: filepath.Join(global, "cache"), rule: RuleGlobal},
	}
	if marker == "" {
		want[KindConfig] = trWant{path: filepath.Join(global, "config.jsonc"), rule: RuleGlobal}
		want[KindHome] = trWant{path: filepath.Join(global, "home.jsonc"), rule: RuleGlobal}
		want[KindGitignore] = trWant{err: ErrNoProject}
	} else {
		want[KindConfig] = trWant{path: filepath.Join(marker, "config.jsonc"), rule: rule}
		want[KindHome] = trWant{path: filepath.Join(marker, "home.jsonc"), rule: rule}
		want[KindGitignore] = trWant{path: filepath.Join(marker, ".gitignore"), rule: rule}
	}
	switch {
	case mode == ModeGlobal:
		want[KindDB] = trWant{path: filepath.Join(global, trDBFile), rule: RuleGlobal}
		want[KindLogs] = trWant{path: filepath.Join(global, "logs"), rule: RuleGlobal}
	case marker == "":
		want[KindDB] = trWant{err: ErrNoProject, rule: RuleProject}
		want[KindLogs] = trWant{path: filepath.Join(global, "logs"), rule: RuleGlobal, fallback: true}
	default:
		want[KindDB] = trWant{path: filepath.Join(marker, trDBFile), rule: rule}
		want[KindLogs] = trWant{path: filepath.Join(marker, "logs"), rule: rule}
	}
	return want
}

// Mode x lookup x where the user stands: the project home found (or
// ErrNoProject) and every kind's result, standard section 1 and SPEC 4.4.
func TestResolveLookupMatrix(t *testing.T) {
	w := trBuildLookupWorld(t)
	type expect struct {
		marker string // "" = ErrNoProject
		rule   Rule
	}
	rows := []struct {
		where          string
		walkUp, hereOn expect
	}{
		{"project root", expect{w.shopM, RuleProject}, expect{w.shopM, RuleProject}},
		{"deep subfolder", expect{w.shopM, RuleProject}, expect{}},
		// The global home ~/.trapp is never a project home, even in $HOME.
		{"$HOME itself", expect{}, expect{}},
		// $HOME is a wall: the walk never reaches ~/.trapp from below.
		{"plain folder under $HOME", expect{}, expect{}},
		{"outside any project", expect{}, expect{}},
		// A linked worktree uses the main checkout's home in both styles (F6).
		{"linked worktree root", expect{w.shopM, RuleWorktree}, expect{w.shopM, RuleWorktree}},
		{"linked worktree subfolder", expect{w.shopM, RuleWorktree}, expect{}},
		// A nested repo is a wall: shop/.trapp above it is not seen.
		{"nested repo without a home", expect{}, expect{}},
		{"nested repo with its own home", expect{w.innerM, RuleProject}, expect{}},
		// A submodule is its own project: no commondir, no translation.
		{"submodule without a home", expect{}, expect{}},
		{"submodule with its own home", expect{w.sub2M, RuleProject}, expect{w.sub2M, RuleProject}},
		// The walk runs on the real path, so a symlink into shop finds shop.
		{"symlink into a project subfolder", expect{w.shopM, RuleProject}, expect{}},
		{"symlink to a project root", expect{w.shopM, RuleProject}, expect{w.shopM, RuleProject}},
	}
	for _, mode := range []Mode{ModeProject, ModeGlobal} {
		for _, lookup := range []Lookup{WalkUp, HereOnly} {
			for _, r := range rows {
				name := string(mode) + "/" + string(lookup) + "/" + r.where
				t.Run(name, func(t *testing.T) {
					work, ok := w.places[r.where]
					if !ok {
						t.Skip("symlinks unavailable")
					}
					want := r.walkUp
					if lookup == HereOnly {
						want = r.hereOn
					}
					d := trNew(t, trHome(w.home), work, nil, WithMode(mode), WithLookup(lookup))
					g := d.Global()
					if g.Base != w.global || g.Data != w.global || g.Rule != RuleGlobal || g.Fallback {
						t.Errorf("Global() = %+v, want %s rule=global", g, w.global)
					}
					p, err := d.Project()
					if want.marker == "" {
						if !errors.Is(err, ErrNoProject) || d.HasProject() {
							t.Fatalf("Project() = %+v, %v; want ErrNoProject", p, err)
						}
						if !strings.Contains(err.Error(), "no "+trMarker+"/ here; run 'trapp init' or pass --home") {
							t.Errorf("ErrNoProject text = %q, want the standard's sentence", err)
						}
					} else {
						if err != nil {
							t.Fatalf("Project() error %v, want %s", err, want.marker)
						}
						if p.Marker != want.marker || p.Base != want.marker || p.Data != want.marker || p.Rule != want.rule || p.Scope != ScopeProject {
							t.Errorf("Project() = %+v; want marker/base/data %s rule %s", p, want.marker, want.rule)
						}
					}
					trCheckKinds(t, d, trKindsFor(mode, w.global, want.marker, want.rule))
					trCheckWarnings(t, d)
				})
			}
		}
	}
}

// A dev build standing in $HOME holding both ~/.trapp and ~/.trapp-dev sees
// neither as a project home: both are global homes (SPEC 4.4 step 3.1).
func TestResolveLookupHomeRejectsBothProfiles(t *testing.T) {
	w := trBuildLookupWorld(t)
	for _, release := range []bool{true, false} {
		d := trNew(t, trHome(w.home), w.home, nil, WithRelease(release))
		if _, err := d.Project(); !errors.Is(err, ErrNoProject) {
			t.Errorf("release=%v: Project() in $HOME = %v, want ErrNoProject", release, err)
		}
	}
}

// trOverrideWorld is a user's machine for the override matrix: $HOME with
// both global homes, a git project shop/.trapp, the user standing in
// shop/src, and a spare folder alt/ outside both.
type trOverrideWorld struct {
	root, home, prodGlobal, devGlobal, shopM, work, alt string
}

func trBuildOverrideWorld(t *testing.T) trOverrideWorld {
	t.Helper()
	root := trRoot(t)
	w := trOverrideWorld{
		root:       root,
		home:       filepath.Join(root, "home"),
		prodGlobal: filepath.Join(root, "home", trMarker),
		devGlobal:  filepath.Join(root, "home", trMarker+"-dev"),
		shopM:      filepath.Join(root, "shop", trMarker),
		work:       filepath.Join(root, "shop", "src"),
		alt:        filepath.Join(root, "alt"),
	}
	trMkdir(t, w.prodGlobal, w.devGlobal, filepath.Join(root, "shop", gitEntry), w.shopM, w.work, w.alt)
	return w
}

// trAllIn is every kind in one home folder h (an override), with rule.
func trAllIn(h string, rule Rule) map[Kind]trWant {
	return map[Kind]trWant{
		KindDB:     {path: filepath.Join(h, trDBFile), rule: rule},
		KindCache:  {path: filepath.Join(h, "cache"), rule: rule},
		KindLogs:   {path: filepath.Join(h, "logs"), rule: rule},
		KindConfig: {path: filepath.Join(h, "config.jsonc"), rule: rule},
		KindShared: {path: filepath.Join(h, "shared"), rule: rule},
	}
}

// trAllFail is every kind failing with err (a broken override, F2).
func trAllFail(err error, rule Rule) map[Kind]trWant {
	return map[Kind]trWant{
		KindDB: {err: err, rule: rule}, KindCache: {err: err, rule: rule}, KindLogs: {err: err, rule: rule},
		KindConfig: {err: err, rule: rule}, KindShared: {err: err, rule: rule},
	}
}

// trNoOverride is the result with nothing set: data in the project's data
// folder, cache in the profile's global home, config in the marker.
func trNoOverride(global, marker, data string) map[Kind]trWant {
	return map[Kind]trWant{
		KindDB:     {path: filepath.Join(data, trDBFile), rule: RuleProject},
		KindCache:  {path: filepath.Join(global, "cache"), rule: RuleGlobal},
		KindLogs:   {path: filepath.Join(data, "logs"), rule: RuleProject},
		KindConfig: {path: filepath.Join(marker, "config.jsonc"), rule: RuleProject},
		KindShared: {path: filepath.Join(data, "shared"), rule: RuleProject},
	}
}

// trWith returns base with the given kinds replaced.
func trWith(base map[Kind]trWant, edits map[Kind]trWant) map[Kind]trWant {
	out := make(map[Kind]trWant, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range edits {
		out[k] = v
	}
	return out
}

// Overrides: --home, <APP>_HOME, <APP>_<KIND>_DIR, expansion, set-but-empty,
// and the dev profile's leak filter (SPEC 4.1, 4.3, 4.7, 8.5).
func TestResolveOverrideMatrix(t *testing.T) {
	w := trBuildOverrideWorld(t)
	prod := trNoOverride(w.prodGlobal, w.shopM, w.shopM)
	devData := filepath.Join(w.shopM, profilesDir, string(ProfileDev))
	dev := trNoOverride(w.devGlobal, w.shopM, devData)
	ext := filepath.Join(w.root, "ext")
	rows := []struct {
		name     string
		dev      bool
		env      map[string]string
		opts     []Option
		want     map[Kind]trWant
		warnings []WarningCode
	}{
		{name: "nothing set: project home and global cache", want: prod},
		{name: "--home absolute: one home for every kind", opts: []Option{WithHome(w.alt)}, want: trAllIn(w.alt, RuleFlag)},
		{name: "--home relative resolves against the working folder", opts: []Option{WithHome("h")}, want: trAllIn(filepath.Join(w.work, "h"), RuleFlag)},
		{name: "--home ~/x expands to $HOME", opts: []Option{WithHome("~/x")}, want: trAllIn(filepath.Join(w.home, "x"), RuleFlag)},
		{name: "--home ${ALT} expands a set variable", env: map[string]string{"ALT": w.alt}, opts: []Option{WithHome("${ALT}/h")}, want: trAllIn(filepath.Join(w.alt, "h"), RuleFlag)},
		{name: "--home ${NOPE} unset fails every kind", opts: []Option{WithHome("${NOPE}/h")}, want: trAllFail(ErrEnvVar, RuleFlag)},
		{name: "--home ${ALT} set but empty fails every kind", env: map[string]string{"ALT": ""}, opts: []Option{WithHome("${ALT}/h")}, want: trAllFail(ErrEnvVar, RuleFlag)},
		{name: "--home ~bob is refused", opts: []Option{WithHome("~bob/h")}, want: trAllFail(ErrInvalidOption, RuleFlag)},
		{name: "--home ${ALT unclosed is refused", opts: []Option{WithHome("${ALT/h")}, want: trAllFail(ErrInvalidOption, RuleFlag)},
		{name: "TRAPP_HOME absolute", env: map[string]string{"TRAPP_HOME": w.alt}, want: trAllIn(w.alt, RuleEnvHome)},
		{name: "TRAPP_HOME set but empty counts as unset", env: map[string]string{"TRAPP_HOME": ""}, want: prod},
		{name: "--home beats TRAPP_HOME", env: map[string]string{"TRAPP_HOME": ext}, opts: []Option{WithHome(w.alt)}, want: trAllIn(w.alt, RuleFlag)},
		{name: "TRAPP_HOME ${NOPE} unset fails every kind", env: map[string]string{"TRAPP_HOME": "${NOPE}"}, want: trAllFail(ErrEnvVar, RuleEnvHome)},
		{name: "TRAPP_HOME ~bob is refused", env: map[string]string{"TRAPP_HOME": "~bob"}, want: trAllFail(ErrInvalidOption, RuleEnvHome)},
		{name: "TRAPP_HOME $ALT without braces stays literal", env: map[string]string{"TRAPP_HOME": "$ALT", "ALT": w.alt}, want: trAllIn(filepath.Join(w.work, "$ALT"), RuleEnvHome)},
		{name: "TRAPP_HOME ~ is $HOME itself", env: map[string]string{"TRAPP_HOME": "~"}, want: trAllIn(w.home, RuleEnvHome)},
		{name: "TRAPP_CACHE_DIR moves only the cache", env: map[string]string{"TRAPP_CACHE_DIR": ext},
			want: trWith(prod, map[Kind]trWant{KindCache: {path: ext, rule: RuleEnvKind}})},
		{name: "TRAPP_DB_DIR is a folder; the db keeps its file name", env: map[string]string{"TRAPP_DB_DIR": ext},
			want: trWith(prod, map[Kind]trWant{KindDB: {path: filepath.Join(ext, trDBFile), rule: RuleEnvKind}})},
		{name: "TRAPP_DB_DIR set but empty counts as unset", env: map[string]string{"TRAPP_DB_DIR": ""}, want: prod},
		{name: "TRAPP_DB_DIR ${NOPE} fails the db only", env: map[string]string{"TRAPP_DB_DIR": "${NOPE}/db"},
			want: trWith(prod, map[Kind]trWant{KindDB: {err: ErrEnvVar, rule: RuleEnvKind}})},
		{name: "TRAPP_DB_DIR ~/dbs expands to $HOME", env: map[string]string{"TRAPP_DB_DIR": "~/dbs"},
			want: trWith(prod, map[Kind]trWant{KindDB: {path: filepath.Join(w.home, "dbs", trDBFile), rule: RuleEnvKind}})},
		{name: "TRAPP_DB_DIR relative resolves against the working folder", env: map[string]string{"TRAPP_DB_DIR": "dbs"},
			want: trWith(prod, map[Kind]trWant{KindDB: {path: filepath.Join(w.work, "dbs", trDBFile), rule: RuleEnvKind}})},
		{name: "TRAPP_LOGS_DIR wins over --home for logs", env: map[string]string{"TRAPP_LOGS_DIR": ext}, opts: []Option{WithHome(w.alt)},
			want: trWith(trAllIn(w.alt, RuleFlag), map[Kind]trWant{KindLogs: {path: ext, rule: RuleEnvKind}})},
		{name: "TRAPP_CONFIG_DIR is ignored: config is anchored", env: map[string]string{"TRAPP_CONFIG_DIR": ext, "TRAPP_HOME_DIR": ext}, want: prod},
		{name: "TRAPP_HOME into the prod home is used by prod", env: map[string]string{"TRAPP_HOME": w.prodGlobal}, want: trAllIn(w.prodGlobal, RuleEnvHome)},

		{name: "dev: nothing set uses ~/.trapp-dev and profiles/dev", dev: true, want: dev},
		{name: "dev: TRAPP_HOME = prod home is ignored", dev: true, env: map[string]string{"TRAPP_HOME": w.prodGlobal}, want: dev, warnings: []WarningCode{WarnEnvPointsToProd}},
		{name: "dev: TRAPP_HOME inside the prod home is ignored", dev: true, env: map[string]string{"TRAPP_HOME": "~/" + trMarker + "/sub"}, want: dev, warnings: []WarningCode{WarnEnvPointsToProd}},
		{name: "dev: TRAPP_HOME = prod home is ignored even with ALLOW_PROD", dev: true, env: map[string]string{"TRAPP_HOME": w.prodGlobal, "TRAPP_ALLOW_PROD": "1"}, want: dev, warnings: []WarningCode{WarnEnvPointsToProd}},
		{name: "dev: TRAPP_HOME = ~/.trapp-dev is used", dev: true, env: map[string]string{"TRAPP_HOME": w.devGlobal}, want: trAllIn(w.devGlobal, RuleEnvHome)},
		{name: "dev: TRAPP_HOME next to the prod home is used", dev: true, env: map[string]string{"TRAPP_HOME": w.alt}, want: trAllIn(w.alt, RuleEnvHome)},
		{name: "dev: TRAPP_DB_DIR = prod home is ignored", dev: true, env: map[string]string{"TRAPP_DB_DIR": w.prodGlobal}, want: dev, warnings: []WarningCode{WarnEnvPointsToProd}},
		{name: "dev: TRAPP_CACHE_DIR inside the prod home is ignored", dev: true, env: map[string]string{"TRAPP_CACHE_DIR": filepath.Join(w.prodGlobal, "cache")}, want: dev, warnings: []WarningCode{WarnEnvPointsToProd}},
		{name: "dev: TRAPP_DB_DIR elsewhere is used", dev: true, env: map[string]string{"TRAPP_DB_DIR": ext},
			want: trWith(dev, map[Kind]trWant{KindDB: {path: filepath.Join(ext, trDBFile), rule: RuleEnvKind}})},
		{name: "dev: --home into the prod home is used (the user named it)", dev: true, opts: []Option{WithHome(w.prodGlobal)}, want: trAllIn(w.prodGlobal, RuleFlag)},
		{name: "dev: --home with TRAPP_DB_DIR into prod ignores the db dir", dev: true, env: map[string]string{"TRAPP_DB_DIR": w.prodGlobal}, opts: []Option{WithHome(w.alt)},
			want: trAllIn(w.alt, RuleFlag), warnings: []WarningCode{WarnEnvPointsToProd}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			d := trNew(t, trHome(w.home), w.work, r.env, append([]Option{WithRelease(!r.dev)}, r.opts...)...)
			trCheckKinds(t, d, r.want)
			trCheckWarnings(t, d, r.warnings...)
			if trWarnCount(d, WarnEnvPointsToProd) > 0 && !strings.Contains(trWarnMessage(d, WarnEnvPointsToProd), "ignored for profile dev") {
				t.Errorf("leak warning = %q, want it to name the profile", trWarnMessage(d, WarnEnvPointsToProd))
			}
		})
	}
}

// Under an override there is one home: Global() and Project() are the same
// folder, ScopeBoth kinds are listed once and labelled global (X1), and
// ConfigFiles lists that home's two files once.
func TestResolveOverrideOneHome(t *testing.T) {
	w := trBuildOverrideWorld(t)
	for _, r := range []struct {
		name string
		env  map[string]string
		opts []Option
		rule Rule
	}{
		{"--home", nil, []Option{WithHome(w.alt)}, RuleFlag},
		{"TRAPP_HOME", map[string]string{"TRAPP_HOME": w.alt}, nil, RuleEnvHome},
	} {
		t.Run(r.name, func(t *testing.T) {
			d := trNew(t, trHome(w.home), w.work, r.env, r.opts...)
			g := d.Global()
			p, err := d.Project()
			if err != nil || g.Base != w.alt || p.Base != w.alt || p.Data != w.alt || g.Rule != r.rule || p.Rule != r.rule || p.Marker != "" {
				t.Fatalf("Global() = %+v, Project() = %+v, %v; want both %s rule %s", g, p, err, w.alt, r.rule)
			}
			seen := map[Kind]int{}
			for _, e := range d.Paths() {
				seen[e.Kind]++
				if e.Rule != r.rule || !strings.HasPrefix(e.Path, w.alt) {
					t.Errorf("Paths() %s = %s rule %s; want inside %s rule %s", e.Kind, e.Path, e.Rule, w.alt, r.rule)
				}
				if e.Kind == KindConfig && e.Home != ScopeGlobal {
					t.Errorf("config labelled %s under an override, want global (X1)", e.Home)
				}
			}
			for k, n := range seen {
				if n != 1 {
					t.Errorf("Paths() lists %s %d times under an override, want once", k, n)
				}
			}
			want := []ConfigFile{
				{Path: filepath.Join(w.alt, "config.jsonc"), Home: ScopeGlobal},
				{Path: filepath.Join(w.alt, "config.local.jsonc"), Home: ScopeGlobal, Local: true},
			}
			if got := d.ConfigFiles(); !slices.Equal(got, want) {
				t.Errorf("ConfigFiles() = %+v, want %+v", got, want)
			}
		})
	}
}

// <APP>_CEILING_DIRS: a walk never climbs above a ceiling, and one that
// starts at or below it checks the ceiling folder itself (X2). Relative
// entries are ignored with a warning, like git; entries that do not expand to
// an absolute path are ignored with ceiling-dropped (D5).
func TestResolveCeilingMatrix(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "work")
	a := filepath.Join(work, "a")
	aM := filepath.Join(a, trMarker)
	deep := filepath.Join(a, "b", "c")
	trMkdir(t, home, aM, deep)
	sep := string(os.PathListSeparator)
	rows := []struct {
		name     string
		start    string
		ceiling  string
		env      map[string]string
		marker   string // "" = ErrNoProject
		warnings []WarningCode
	}{
		{name: "no ceiling: walk finds a/.trapp", start: deep, marker: aM},
		{name: "ceiling empty counts as unset", start: deep, ceiling: "", marker: aM},
		{name: "ceiling above the project: found", start: deep, ceiling: work, marker: aM},
		// Like git: a ceiling reached from below is never entered (SPEC 4.4 step 3.3).
		{name: "ceiling at the project folder, start below: not found", start: deep, ceiling: a},
		{name: "start in the ceiling folder holding the project: found", start: a, ceiling: a, marker: aM},
		{name: "ceiling between start and project: not found", start: deep, ceiling: filepath.Join(a, "b")},
		{name: "start at the ceiling: not found above it", start: deep, ceiling: deep},
		{name: "start above the ceiling: ceiling plays no part", start: filepath.Join(a, "b"), ceiling: deep, marker: aM},
		{name: "relative entry ignored with a warning", start: deep, ceiling: "a/b", marker: aM, warnings: []WarningCode{WarnCeilingRelative}},
		{name: "relative entry next to a real one", start: deep, ceiling: "rel" + sep + filepath.Join(a, "b"), warnings: []WarningCode{WarnCeilingRelative}},
		{name: "empty list entries are skipped", start: deep, ceiling: sep + sep + filepath.Join(a, "b") + sep, marker: ""},
		{name: "${VAR} entry expands", start: deep, ceiling: "${CEIL}/b", env: map[string]string{"CEIL": a}},
		{name: "${VAR} entry unset is skipped with a warning", start: deep, ceiling: "${NOPE}/b", marker: aM, warnings: []WarningCode{WarnCeilingDropped}},
		{name: "~bob entry is skipped with a warning", start: deep, ceiling: "~bob/x", marker: aM, warnings: []WarningCode{WarnCeilingDropped}},
		{name: "$VAR entry without braces is skipped with a warning", start: deep, ceiling: "$CEIL/b", env: map[string]string{"CEIL": a}, marker: aM, warnings: []WarningCode{WarnCeilingDropped}},
		{name: "~ entry is $HOME (not on this path)", start: deep, ceiling: "~", marker: aM},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			env := map[string]string{"TRAPP_CEILING_DIRS": r.ceiling}
			for k, v := range r.env {
				env[k] = v
			}
			d := trNew(t, trHome(home), r.start, env)
			p, err := d.Project()
			if r.marker == "" {
				if !errors.Is(err, ErrNoProject) {
					t.Errorf("Project() = %+v, %v; want ErrNoProject", p, err)
				}
			} else if err != nil || p.Marker != r.marker {
				t.Errorf("Project() = %+v, %v; want %s", p, err, r.marker)
			}
			trCheckWarnings(t, d, r.warnings...)
		})
	}
}

// expandUserPath on its own: every form a user may type in a flag, env var
// or local pointer (SPEC 4.1).
func TestResolveExpandUserPath(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	base := filepath.Join(root, "base")
	env := map[string]string{"A": filepath.Join(root, "a"), "B": "b", "EMPTY": ""}
	d := trNew(t, trHome(home), root, env)
	rows := []struct {
		raw  string
		want string
		err  error
	}{
		{"~", home, nil},
		{"~/x/y", filepath.Join(home, "x", "y"), nil},
		{"~bob/x", "", ErrInvalidOption},
		{"${A}", filepath.Join(root, "a"), nil},
		{"${A}/${B}", filepath.Join(root, "a", "b"), nil},
		{"pre-${B}-post", filepath.Join(base, "pre-b-post"), nil},
		{"${NOPE}", "", ErrEnvVar},
		{"${EMPTY}/x", "", ErrEnvVar},
		{"${A", "", ErrInvalidOption},
		{"$A/x", filepath.Join(base, "$A", "x"), nil},
		{"rel/../x", filepath.Join(base, "x"), nil},
		{"/abs//x/", filepath.Clean("/abs/x"), nil},
	}
	for _, r := range rows {
		t.Run(r.raw, func(t *testing.T) {
			got, err := d.cfg.expandUserPath(r.raw, base, d.userHome, d.userHomeErr)
			if r.err != nil {
				if !errors.Is(err, r.err) {
					t.Errorf("expand(%q) = %q, %v; want %v", r.raw, got, err, r.err)
				}
				return
			}
			if err != nil || got != r.want {
				t.Errorf("expand(%q) = %q, %v; want %q", r.raw, got, err, r.want)
			}
		})
	}
	t.Run("~ with no home folder", func(t *testing.T) {
		_, err := d.cfg.expandUserPath("~/x", base, "", errors.New("no home"))
		if !errors.Is(err, ErrNoHome) {
			t.Errorf("expand(~/x) with no home = %v, want ErrNoHome", err)
		}
	})
	t.Run("unset variable is named", func(t *testing.T) {
		_, err := d.cfg.expandUserPath("${NOPE}", base, home, nil)
		if err == nil || !strings.Contains(err.Error(), "${NOPE} is not set") {
			t.Errorf("error = %v, want it to name ${NOPE}", err)
		}
	})
}

// trPtrWorld is one project for the pointer matrix: $HOME, a git project
// proj/.trapp the user stands in, and a private home of trApp at
// private/h (what a home: pointer usually names).
type trPtrWorld struct {
	root, home, global, proj, marker, target string
}

func trBuildPtrWorld(t *testing.T) trPtrWorld {
	t.Helper()
	root := trRoot(t)
	w := trPtrWorld{
		root:   root,
		home:   filepath.Join(root, "home"),
		global: filepath.Join(root, "home", trMarker),
		proj:   filepath.Join(root, "proj"),
		marker: filepath.Join(root, "proj", trMarker),
		target: filepath.Join(root, "private", "h"),
	}
	trMkdir(t, w.home, filepath.Join(w.proj, gitEntry), w.marker)
	trInitHome(t, w.target)
	return w
}

// trProjectKinds are the kinds that live in (or may use) the project home
// with the standard layout in ModeProject: a broken pointer fails them all.
var trProjectKinds = []Kind{KindHome, KindConfig, KindConfigLocal, KindGitignore, KindShared, KindDB, KindBackups, KindRun, KindLogs}

// Pointer keys home: and db: (standard section 4, SPEC 4.5).
func TestResolvePointerMatrix(t *testing.T) {
	type expect struct {
		err      error  // a pointer error: every project-capable kind fails with it (F11)
		base     string // project Base ("" = the marker)
		data     string // project Data ("" = base)
		db       string // db path ("" = data/trapp.db)
		dbRule   Rule   // "" = pointer when base moved, else project
		warnings []WarningCode
		msg      string // a substring of the error or of the first warning
	}
	rows := []struct {
		name      string
		committed string // config.jsonc body ("" = no file)
		local     string // config.local.jsonc body ("" = no file)
		dev       bool
		env       map[string]string
		setup     func(t *testing.T, w trPtrWorld)
		want      func(w trPtrWorld) expect
	}{
		{name: "no pointer keys: the marker is the home", committed: `{ "port": 3000 }`,
			want: func(w trPtrWorld) expect { return expect{dbRule: RuleProject} }},
		{name: "committed home: relative to the marker folder", committed: `{ "home": "../../private/h" }`,
			want: func(w trPtrWorld) expect { return expect{base: w.target} }},
		{name: "committed home: ~/ path", committed: `{ "home": "~/h" }`,
			setup: func(t *testing.T, w trPtrWorld) { trInitHome(t, filepath.Join(w.home, "h")) },
			want:  func(w trPtrWorld) expect { return expect{base: filepath.Join(w.home, "h")} }},
		{name: "committed home: with JSONC comments and a trailing comma", committed: "{\n // private notes\n \"home\": \"../../private/h\",\n}",
			want: func(w trPtrWorld) expect { return expect{base: w.target} }},
		{name: "committed home: absolute is refused", committed: `{ "home": "/srv/h" }`,
			want: func(w trPtrWorld) expect {
				return expect{err: ErrPointerAbsolute, msg: "is absolute; put it in config.local.jsonc"}
			}},
		{name: "committed home: drive path is refused", committed: `{ "home": "C:/work/h" }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerAbsolute} }},
		{name: "committed home: UNC path is refused", committed: `{ "home": "\\\\server\\share\\h" }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerAbsolute} }},
		{name: "committed home: ${VAR} is refused even when set", committed: `{ "home": "${PRIV}/h" }`, env: map[string]string{"PRIV": "/srv"},
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerAbsolute} }},
		{name: "committed home: ~bob is refused", committed: `{ "home": "~bob/h" }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerAbsolute} }},
		{name: "local home: absolute is fine", local: `{ "home": "TARGET" }`,
			want: func(w trPtrWorld) expect { return expect{base: w.target} }},
		{name: "local home: ${VAR} is fine", local: `{ "home": "${PRIV}/h" }`, env: map[string]string{"PRIV": "PRIVATE"},
			want: func(w trPtrWorld) expect { return expect{base: w.target} }},
		{name: "local home: ${VAR} unset is ErrEnvVar", local: `{ "home": "${PRIV}/h" }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrEnvVar, msg: "${PRIV} is not set"} }},
		{name: "local wins over committed per key", committed: `{ "home": "/srv/absolute-is-overridden" }`, local: `{ "home": "TARGET" }`,
			want: func(w trPtrWorld) expect { return expect{base: w.target} }},
		{name: "local empty string does not clear committed", committed: `{ "home": "../../private/h" }`, local: `{ "home": "" }`,
			want: func(w trPtrWorld) expect { return expect{base: w.target} }},
		{name: "null value counts as absent", committed: `{ "home": null, "db": null }`,
			want: func(w trPtrWorld) expect { return expect{dbRule: RuleProject} }},
		{name: "missing target is ErrPointerMissing", committed: `{ "home": "../../private/none" }`,
			want: func(w trPtrWorld) expect {
				return expect{err: ErrPointerMissing, msg: "which does not exist; run 'trapp init --home "}
			}},
		{name: "target is a file: ErrPointerMissing", committed: `{ "home": "../../private/h/home.jsonc" }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerMissing} }},
		{name: "target without home.jsonc is ErrPointerNotHome", committed: `{ "home": "../../private" }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerNotHome} }},
		{name: "target is another app's home: ErrPointerNotHome", committed: `{ "home": "../../other" }`,
			setup: func(t *testing.T, w trPtrWorld) {
				trWrite(t, filepath.Join(w.root, "other", "home.jsonc"), `{ "app": "otherapp", "layout": 1 }`)
			},
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerNotHome} }},
		{name: "home: target pointer is not followed (one hop)", committed: `{ "home": "../../private/h" }`,
			setup: func(t *testing.T, w trPtrWorld) {
				trWrite(t, filepath.Join(w.target, "config.jsonc"), `{ "home": "../elsewhere" }`)
				trWrite(t, filepath.Join(w.target, "config.local.jsonc"), `{ "db": "/x/y.db" }`)
			},
			want: func(w trPtrWorld) expect {
				return expect{base: w.target, warnings: []WarningCode{WarnPointerChain, WarnPointerChain}, msg: "pointers go one hop only"}
			}},
		{name: "home: non-string value is ErrInvalidOption", committed: `{ "home": 5 }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrInvalidOption, msg: "home must be a string path"} }},
		{name: "db: non-string value is ErrInvalidOption", local: `{ "db": true }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrInvalidOption} }},
		{name: "malformed config.jsonc fails the project", committed: `{ "home": `,
			want: func(w trPtrWorld) expect { return expect{err: errTrAnyError} }},
		{name: "db: pointer in local, file need not exist", local: `{ "db": "DBDIR/notes.db" }`,
			setup: func(t *testing.T, w trPtrWorld) { trMkdir(t, filepath.Join(w.root, "dbs")) },
			want: func(w trPtrWorld) expect {
				return expect{db: filepath.Join(w.root, "dbs", "notes.db"), dbRule: RulePointer}
			}},
		{name: "db: pointer committed relative", committed: `{ "db": "../data/notes.db" }`,
			setup: func(t *testing.T, w trPtrWorld) { trMkdir(t, filepath.Join(w.proj, "data")) },
			want: func(w trPtrWorld) expect {
				return expect{db: filepath.Join(w.proj, "data", "notes.db"), dbRule: RulePointer}
			}},
		{name: "db: pointer committed absolute is refused", committed: `{ "db": "/srv/notes.db" }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerAbsolute} }},
		{name: "db: pointer folder missing is ErrPointerMissing", local: `{ "db": "DBDIR/notes.db" }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerMissing} }},
		{name: "home: and db: together", committed: `{ "home": "../../private/h" }`, local: `{ "db": "DBDIR/notes.db" }`,
			setup: func(t *testing.T, w trPtrWorld) { trMkdir(t, filepath.Join(w.root, "dbs")) },
			want: func(w trPtrWorld) expect {
				return expect{base: w.target, db: filepath.Join(w.root, "dbs", "notes.db"), dbRule: RulePointer}
			}},

		{name: "dev: home: resolves to target/profiles/dev", dev: true, committed: `{ "home": "../../private/h" }`,
			want: func(w trPtrWorld) expect {
				return expect{base: w.target, data: filepath.Join(w.target, profilesDir, string(ProfileDev))}
			}},
		{name: "dev: db: pointer is ignored with a warning naming the real path", dev: true, local: `{ "db": "DBDIR/notes.db" }`,
			setup: func(t *testing.T, w trPtrWorld) { trMkdir(t, filepath.Join(w.root, "dbs")) },
			want: func(w trPtrWorld) expect {
				data := filepath.Join(w.marker, profilesDir, string(ProfileDev))
				return expect{data: data, dbRule: RuleProject, warnings: []WarningCode{WarnPointerProfile}, msg: filepath.Join(data, trDBFile)}
			}},
		{name: "dev: broken db: pointer is ignored too", dev: true, local: `{ "db": "DBDIR/notes.db" }`,
			want: func(w trPtrWorld) expect {
				return expect{data: filepath.Join(w.marker, profilesDir, string(ProfileDev)), dbRule: RuleProject, warnings: []WarningCode{WarnPointerProfile}}
			}},
		{name: "dev: broken home: pointer still fails", dev: true, committed: `{ "home": "../../private/none" }`,
			want: func(w trPtrWorld) expect { return expect{err: ErrPointerMissing} }},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			w := trBuildPtrWorld(t)
			subst := strings.NewReplacer("TARGET", filepath.ToSlash(w.target), "PRIVATE", filepath.ToSlash(filepath.Join(w.root, "private")), "DBDIR", filepath.ToSlash(filepath.Join(w.root, "dbs")))
			if r.committed != "" {
				trWrite(t, filepath.Join(w.marker, "config.jsonc"), subst.Replace(r.committed))
			}
			if r.local != "" {
				trWrite(t, filepath.Join(w.marker, "config.local.jsonc"), subst.Replace(r.local))
			}
			if r.setup != nil {
				r.setup(t, w)
			}
			env := map[string]string{}
			for k, v := range r.env {
				env[k] = subst.Replace(v)
			}
			d := trNew(t, trHome(w.home), w.proj, env, WithRelease(!r.dev))
			x := r.want(w)
			globalHome := w.global
			if r.dev {
				globalHome += "-dev"
			}
			if x.err != nil {
				_, err := d.Project()
				if x.err == errTrAnyError {
					if err == nil || errors.Is(err, ErrNoProject) {
						t.Fatalf("Project() error = %v, want a read error", err)
					}
				} else if !errors.Is(err, x.err) {
					t.Fatalf("Project() error = %v, want %v", err, x.err)
				}
				if x.msg != "" && !strings.Contains(err.Error(), x.msg) {
					t.Errorf("error = %q, want it to contain %q", err, x.msg)
				}
				// F11: every project-capable kind fails with the pointer's
				// error; none falls back to the marker or the global home.
				for _, k := range trProjectKinds {
					p, kerr := d.Path(k)
					if kerr == nil || kerr.Error() != err.Error() {
						t.Errorf("%s: Path() = %q, %v; want the pointer error", k, p, kerr)
					}
				}
				trCheckKind(t, d, KindCache, trWant{path: filepath.Join(globalHome, "cache"), rule: RuleGlobal})
				// ConfigFiles still lists the marker's files: they hold the
				// pointer the user has to fix.
				cf := d.ConfigFiles()
				if len(cf) != 4 || cf[2].Path != filepath.Join(w.marker, "config.jsonc") || cf[3].Path != filepath.Join(w.marker, "config.local.jsonc") {
					t.Errorf("ConfigFiles() with a broken pointer = %+v, want global 2 + marker 2", cf)
				}
				return
			}
			p, err := d.Project()
			if err != nil {
				t.Fatalf("Project() error %v", err)
			}
			base := x.base
			if base == "" {
				base = w.marker
			}
			data := x.data
			if data == "" {
				data = base
			}
			rule := RuleProject
			if base != w.marker {
				rule = RulePointer
			}
			if p.Marker != w.marker || p.Base != base || p.Data != data || p.Rule != rule {
				t.Errorf("Project() = %+v; want marker %s base %s data %s rule %s", p, w.marker, base, data, rule)
			}
			if rule == RulePointer && (p.Info == nil || p.Info.App != trApp) {
				t.Errorf("Project().Info = %+v, want the target's home.jsonc", p.Info)
			}
			db, dbRule := x.db, x.dbRule
			if db == "" {
				db = filepath.Join(data, trDBFile)
			}
			if dbRule == "" {
				dbRule = rule
			}
			trCheckKinds(t, d, map[Kind]trWant{
				KindDB:      {path: db, rule: dbRule},
				KindConfig:  {path: filepath.Join(base, "config.jsonc"), rule: rule},
				KindHome:    {path: filepath.Join(base, "home.jsonc"), rule: rule},
				KindShared:  {path: filepath.Join(data, "shared"), rule: rule},
				KindLogs:    {path: filepath.Join(data, "logs"), rule: rule},
				KindBackups: {path: filepath.Join(data, "backups"), rule: rule}, // never moves with db:
				KindCache:   {path: filepath.Join(globalHome, "cache"), rule: RuleGlobal},
			})
			trCheckWarnings(t, d, x.warnings...)
			if x.msg != "" && len(x.warnings) > 0 && !strings.Contains(trWarnMessage(d, x.warnings[0]), x.msg) {
				t.Errorf("warning = %q, want it to contain %q", trWarnMessage(d, x.warnings[0]), x.msg)
			}
		})
	}
}

// errTrAnyError stands for "some error that is not ErrNoProject" in a row.
var errTrAnyError = errors.New("any error")

// ConfigFiles read order (SPEC 6.1, standard section 3).
func TestResolveConfigFilesMatrix(t *testing.T) {
	type world struct {
		root, home, proj, marker, target, alt string
	}
	build := func(t *testing.T, git bool) world {
		root := trRoot(t)
		w := world{root: root, home: filepath.Join(root, "home"), proj: filepath.Join(root, "proj"),
			marker: filepath.Join(root, "proj", trMarker), target: filepath.Join(root, "private", "h"), alt: filepath.Join(root, "alt")}
		trMkdir(t, w.home, w.marker, w.alt)
		if git {
			trMkdir(t, filepath.Join(w.proj, gitEntry))
		}
		trInitHome(t, w.target)
		return w
	}
	cf := func(dir string, home Scope, committed bool) []ConfigFile {
		return []ConfigFile{
			{Path: filepath.Join(dir, "config.jsonc"), Home: home, Committed: committed},
			{Path: filepath.Join(dir, "config.local.jsonc"), Home: home, Local: true},
		}
	}
	devFile := func(data string) ConfigFile {
		return ConfigFile{Path: filepath.Join(data, "config.local.jsonc"), Home: ScopeProject, Local: true, Profile: ProfileDev}
	}
	rows := []struct {
		name    string
		git     bool
		dev     bool
		pointer bool
		noHome  bool
		outside bool
		opts    func(w world) []Option
		env     func(w world) map[string]string
		want    func(w world) []ConfigFile
	}{
		{name: "prod in a git project", git: true, want: func(w world) []ConfigFile {
			return slices.Concat(cf(filepath.Join(w.home, trMarker), ScopeGlobal, false), cf(w.marker, ScopeProject, true))
		}},
		{name: "prod in a project outside git: not committed", want: func(w world) []ConfigFile {
			return slices.Concat(cf(filepath.Join(w.home, trMarker), ScopeGlobal, false), cf(w.marker, ScopeProject, false))
		}},
		{name: "prod outside any project: global only", git: true, outside: true, want: func(w world) []ConfigFile {
			return cf(filepath.Join(w.home, trMarker), ScopeGlobal, false)
		}},
		{name: "dev in a git project: profile file last", git: true, dev: true, want: func(w world) []ConfigFile {
			return append(slices.Concat(cf(filepath.Join(w.home, trMarker+"-dev"), ScopeGlobal, false), cf(w.marker, ScopeProject, true)),
				devFile(filepath.Join(w.marker, profilesDir, string(ProfileDev))))
		}},
		{name: "dev outside any project: no profile file", dev: true, outside: true, want: func(w world) []ConfigFile {
			return cf(filepath.Join(w.home, trMarker+"-dev"), ScopeGlobal, false)
		}},
		{name: "prod with a home: pointer: target after marker", git: true, pointer: true, want: func(w world) []ConfigFile {
			return slices.Concat(cf(filepath.Join(w.home, trMarker), ScopeGlobal, false), cf(w.marker, ScopeProject, true), cf(w.target, ScopeProject, false))
		}},
		{name: "dev with a home: pointer: profile file in the target", git: true, dev: true, pointer: true, want: func(w world) []ConfigFile {
			return append(slices.Concat(cf(filepath.Join(w.home, trMarker+"-dev"), ScopeGlobal, false), cf(w.marker, ScopeProject, true), cf(w.target, ScopeProject, false)),
				devFile(filepath.Join(w.target, profilesDir, string(ProfileDev))))
		}},
		{name: "--home: one home, two files", git: true, opts: func(w world) []Option { return []Option{WithHome(w.alt)} },
			want: func(w world) []ConfigFile { return cf(w.alt, ScopeGlobal, false) }},
		{name: "dev --home: one home, no profile file", git: true, dev: true, opts: func(w world) []Option { return []Option{WithHome(w.alt)} },
			want: func(w world) []ConfigFile { return cf(w.alt, ScopeGlobal, false) }},
		{name: "TRAPP_HOME: one home, two files", git: true, env: func(w world) map[string]string { return map[string]string{"TRAPP_HOME": w.alt} },
			want: func(w world) []ConfigFile { return cf(w.alt, ScopeGlobal, false) }},
		{name: "no $HOME: global left out", git: true, noHome: true, want: func(w world) []ConfigFile {
			return cf(w.marker, ScopeProject, true)
		}},
		{name: "no $HOME outside a project: nothing", noHome: true, outside: true, want: func(w world) []ConfigFile { return nil }},
		{name: "--home ${NOPE}: no files to read", git: true, opts: func(w world) []Option { return []Option{WithHome("${NOPE}")} },
			want: func(w world) []ConfigFile { return nil }},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			w := build(t, r.git)
			if r.pointer {
				trWrite(t, filepath.Join(w.marker, "config.jsonc"), `{ "home": "../../private/h" }`)
			}
			home := trHome(w.home)
			if r.noHome {
				home = trNoHome()
			}
			work := w.proj
			if r.outside {
				work = w.alt
			}
			var env map[string]string
			if r.env != nil {
				env = r.env(w)
			}
			opts := []Option{WithRelease(!r.dev)}
			if r.opts != nil {
				opts = append(opts, r.opts(w)...)
			}
			d := trNew(t, home, work, env, opts...)
			if got, want := d.ConfigFiles(), r.want(w); !slices.Equal(got, want) {
				t.Errorf("ConfigFiles() =\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

// Profiles x homes: the global home's name and the project's data folder
// per profile; anchored kinds stay in Base (SPEC 4.6, 8.3).
func TestResolveProfileHomes(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	marker := filepath.Join(root, "proj", trMarker)
	trMkdir(t, home, filepath.Join(root, "proj", gitEntry), marker)
	profiles := WithProfiles(ProfileProd, ProfileDev, "staging")
	rows := []struct {
		name    string
		release bool
		flag    string
		env     map[string]string
		want    Profile
		warning bool
	}{
		{name: "release build: prod", release: true, want: ProfileProd},
		{name: "dev build: dev", want: ProfileDev},
		{name: "release build --profile staging", release: true, flag: "staging", want: "staging"},
		{name: "dev build --profile prod", flag: "prod", want: ProfileProd},
		{name: "dev build TRAPP_PROFILE=staging", env: map[string]string{"TRAPP_PROFILE": "staging"}, want: "staging"},
		{name: "release build ignores TRAPP_PROFILE=staging", release: true, env: map[string]string{"TRAPP_PROFILE": "staging"}, want: ProfileProd, warning: true},
		{name: "dev build ignores TRAPP_PROFILE=prod", env: map[string]string{"TRAPP_PROFILE": string(ProfileProd)}, want: ProfileDev, warning: true},
		{name: "dev build TRAPP_PROFILE=prod with ALLOW_PROD", env: map[string]string{"TRAPP_PROFILE": string(ProfileProd), "TRAPP_ALLOW_PROD": "1"}, want: ProfileProd},
		{name: "TRAPP_PROFILE set but empty counts as unset", env: map[string]string{"TRAPP_PROFILE": ""}, want: ProfileDev},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			d := trNew(t, trHome(home), filepath.Join(root, "proj"), r.env, profiles, WithRelease(r.release), WithProfile(r.flag))
			if d.Profile() != r.want {
				t.Fatalf("Profile() = %s, want %s", d.Profile(), r.want)
			}
			global := filepath.Join(home, trMarker)
			data := marker
			if r.want != ProfileProd {
				global += "-" + string(r.want)
				data = filepath.Join(marker, profilesDir, string(r.want))
			}
			if g := d.Global(); g.Base != global || g.Data != global {
				t.Errorf("Global() = %+v, want %s", g, global)
			}
			p, err := d.Project()
			if err != nil || p.Base != marker || p.Data != data {
				t.Fatalf("Project() = %+v, %v; want base %s data %s", p, err, marker, data)
			}
			trCheckKinds(t, d, map[Kind]trWant{
				KindHome:        {path: filepath.Join(marker, "home.jsonc"), rule: RuleProject},
				KindConfig:      {path: filepath.Join(marker, "config.jsonc"), rule: RuleProject},
				KindConfigLocal: {path: filepath.Join(marker, "config.local.jsonc"), rule: RuleProject},
				KindGitignore:   {path: filepath.Join(marker, ".gitignore"), rule: RuleProject},
				KindDB:          {path: filepath.Join(data, trDBFile), rule: RuleProject},
				KindShared:      {path: filepath.Join(data, "shared"), rule: RuleProject},
				KindRun:         {path: filepath.Join(data, "run"), rule: RuleProject},
				KindLogs:        {path: filepath.Join(data, "logs"), rule: RuleProject},
				KindCache:       {path: filepath.Join(global, "cache"), rule: RuleGlobal},
			})
			if got := trWarnCount(d, WarnProfileEnvIgnored) > 0; got != r.warning {
				t.Errorf("profile-env-ignored warned = %v, want %v (%v)", got, r.warning, d.Warnings())
			}
		})
	}
	t.Run("unknown profile is refused", func(t *testing.T) {
		for _, opt := range []Option{WithProfile("qa"), trEnv(map[string]string{"TRAPP_PROFILE": "qa"})} {
			if _, err := trTryNew(trHome(home), root, nil, profiles, opt); !errors.Is(err, ErrUnknownProfile) {
				t.Errorf("New with profile qa = %v, want ErrUnknownProfile", err)
			}
		}
	})
}

// No $HOME (CI, containers): global config is code defaults, global data
// fails with ErrNoHome, disposable kinds use <tmp>/<app>-<uid> (SPEC 4.3
// step 4, 4.7 step 4).
func TestResolveNoHomeMatrix(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // keeps the temp fallback inside the test root
	root, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "proj", trMarker)
	outside := filepath.Join(root, "outside")
	alt := filepath.Join(root, "alt")
	trMkdir(t, filepath.Join(root, "proj", gitEntry), marker, outside, alt)
	tempRoot := func(name string) string { return filepath.Join(os.TempDir(), name+"-"+currentUser()) }
	defaults := trWant{rule: RuleDefaults, fallback: true}
	noHomes := map[string]Option{
		"UserHomeDir fails":       trNoHome(),
		"UserHomeDir is relative": trHome("rel/home"),
		"UserHomeDir is empty":    trHome(""),
	}
	rows := []struct {
		name string
		work string
		env  map[string]string
		opts []Option
		want map[Kind]trWant
	}{
		{name: "project mode outside a project", work: outside, want: map[Kind]trWant{
			KindConfig: defaults, KindConfigLocal: defaults, KindHome: defaults,
			KindDB:    {err: ErrNoProject},
			KindRun:   {err: ErrNoProject},
			KindLogs:  {err: ErrNoHome, rule: RuleGlobal},
			KindCache: {path: filepath.Join(tempRoot(trApp), "cache"), rule: RuleTemp, fallback: true},
		}},
		{name: "project mode inside a project", work: filepath.Join(root, "proj"), want: map[Kind]trWant{
			KindConfig: {path: filepath.Join(marker, "config.jsonc"), rule: RuleProject},
			KindDB:     {path: filepath.Join(marker, trDBFile), rule: RuleProject},
			KindLogs:   {path: filepath.Join(marker, "logs"), rule: RuleProject},
			KindCache:  {path: filepath.Join(tempRoot(trApp), "cache"), rule: RuleTemp, fallback: true},
		}},
		{name: "global mode", work: outside, opts: []Option{WithMode(ModeGlobal)}, want: map[Kind]trWant{
			KindConfig: defaults,
			KindDB:     {err: ErrNoHome, rule: RuleGlobal},
			KindRun:    {err: ErrNoHome, rule: RuleGlobal},
			KindLogs:   {err: ErrNoHome, rule: RuleGlobal},
			KindCache:  {path: filepath.Join(tempRoot(trApp), "cache"), rule: RuleTemp, fallback: true},
		}},
		{name: "dev build: temp root carries the profile", work: outside, opts: []Option{WithRelease(false)}, want: map[Kind]trWant{
			KindCache: {path: filepath.Join(tempRoot(trApp+"-dev"), "cache"), rule: RuleTemp, fallback: true},
		}},
		{name: "temp fallback off: cache fails", work: outside, opts: []Option{WithTempFallback(false)}, want: map[Kind]trWant{
			KindCache: {err: ErrNoHome, rule: RuleGlobal},
		}},
		{name: "TRAPP_HOME set: one home, no fallback", work: outside, env: map[string]string{"TRAPP_HOME": alt}, want: trAllIn(alt, RuleEnvHome)},
		{name: "TRAPP_HOME=~/x fails every kind with ErrNoHome", work: outside, env: map[string]string{"TRAPP_HOME": "~/x"}, want: trAllFail(ErrNoHome, RuleEnvHome)},
		{name: "TRAPP_CACHE_DIR still moves the cache", work: outside, env: map[string]string{"TRAPP_CACHE_DIR": alt}, want: map[Kind]trWant{
			KindCache: {path: alt, rule: RuleEnvKind},
		}},
		// With no $HOME there is no prod home to leak into: a dev build uses it.
		{name: "dev build: TRAPP_CACHE_DIR is used", work: outside, env: map[string]string{"TRAPP_CACHE_DIR": alt}, opts: []Option{WithRelease(false)}, want: map[Kind]trWant{
			KindCache: {path: alt, rule: RuleEnvKind},
		}},
	}
	for hname, home := range noHomes {
		for _, r := range rows {
			t.Run(hname+"/"+r.name, func(t *testing.T) {
				d := trNew(t, home, r.work, r.env, r.opts...)
				trCheckKinds(t, d, r.want)
				if r.env == nil {
					g := d.Global()
					if g.Rule != RuleDefaults || !g.Fallback || g.Base != "" || g.Data != "" {
						t.Errorf("Global() = %+v, want rule defaults, fallback, no folders", g)
					}
					if p, err := d.PathIn(KindConfig, ScopeGlobal); p != "" || err != nil {
						t.Errorf("PathIn(config, global) = %q, %v; want code defaults", p, err)
					}
				}
			})
		}
	}
}

// Linked worktree edge cases (SPEC 4.4 step 2).
func TestResolveWorktreeMatrix(t *testing.T) {
	rows := []struct {
		name     string
		setup    func(t *testing.T, root string) (work string)
		marker   string // relative to root; "" = ErrNoProject
		rule     Rule
		warnings []WarningCode
	}{
		{name: "main has a home: worktree uses it", rule: RuleWorktree, marker: "main/" + trMarker,
			setup: func(t *testing.T, root string) string {
				trMkdir(t, filepath.Join(root, "main", gitEntry), filepath.Join(root, "main", trMarker))
				trWorktree(t, filepath.Join(root, "main"), filepath.Join(root, "wt"))
				return filepath.Join(root, "wt")
			}},
		{name: "main has none, worktree has one: worktree with a warning", rule: RuleProject, marker: "wt/" + trMarker,
			warnings: []WarningCode{WarnWorktreeMainMissing},
			setup: func(t *testing.T, root string) string {
				trMkdir(t, filepath.Join(root, "main", gitEntry), filepath.Join(root, "wt", trMarker))
				trWorktree(t, filepath.Join(root, "main"), filepath.Join(root, "wt"))
				return filepath.Join(root, "wt")
			}},
		{name: "neither has a home: no project, no warning (F5)",
			setup: func(t *testing.T, root string) string {
				trMkdir(t, filepath.Join(root, "main", gitEntry))
				trWorktree(t, filepath.Join(root, "main"), filepath.Join(root, "wt"))
				return filepath.Join(root, "wt")
			}},
		{name: "relative gitdir line", rule: RuleWorktree, marker: "main/" + trMarker,
			setup: func(t *testing.T, root string) string {
				trMkdir(t, filepath.Join(root, "main", trMarker))
				trWrite(t, filepath.Join(root, "main", gitEntry, "worktrees", "wt", "commondir"), "../..\n")
				trWrite(t, filepath.Join(root, "wt", gitEntry), "gitdir: ../main/.git/worktrees/wt\n")
				trMkdir(t, filepath.Join(root, "wt", "src"))
				return filepath.Join(root, "wt", "src")
			}},
		{name: "absolute commondir", rule: RuleWorktree, marker: "main/" + trMarker,
			setup: func(t *testing.T, root string) string {
				trMkdir(t, filepath.Join(root, "main", trMarker))
				admin := filepath.Join(root, "main", gitEntry, "worktrees", "wt")
				trWrite(t, filepath.Join(admin, "commondir"), filepath.Join(root, "main", gitEntry)+"\n")
				trWrite(t, filepath.Join(root, "wt", gitEntry), "gitdir: "+admin+"\n")
				return filepath.Join(root, "wt")
			}},
		{name: "bare main repo: worktree is its own project", rule: RuleProject, marker: "wt/" + trMarker,
			setup: func(t *testing.T, root string) string {
				admin := filepath.Join(root, "repo.git", "worktrees", "wt")
				trWrite(t, filepath.Join(admin, "commondir"), "../..\n")
				trWrite(t, filepath.Join(root, "wt", gitEntry), "gitdir: "+admin+"\n")
				trMkdir(t, filepath.Join(root, "wt", trMarker))
				return filepath.Join(root, "wt")
			}},
		{name: ".git file without a gitdir line is a plain repo root", rule: RuleProject, marker: "wt/" + trMarker,
			setup: func(t *testing.T, root string) string {
				trWrite(t, filepath.Join(root, "wt", gitEntry), "nonsense\n")
				trMkdir(t, filepath.Join(root, "wt", trMarker), filepath.Join(root, "wt", "src"))
				return filepath.Join(root, "wt", "src")
			}},
		{name: "worktree-only subfolder: lookup climbs from the missing main path", rule: RuleWorktree, marker: "main/" + trMarker,
			setup: func(t *testing.T, root string) string {
				trMkdir(t, filepath.Join(root, "main", gitEntry), filepath.Join(root, "main", trMarker))
				trWorktree(t, filepath.Join(root, "main"), filepath.Join(root, "wt"))
				trMkdir(t, filepath.Join(root, "wt", "new", "deep"))
				return filepath.Join(root, "wt", "new", "deep")
			}},
		{name: "dangling .git symlink is a wall", setup: func(t *testing.T, root string) string {
			trMkdir(t, filepath.Join(root, "top", trMarker), filepath.Join(root, "top", "repo", "src"))
			trSymlink(t, filepath.Join(root, "gone"), filepath.Join(root, "top", "repo", gitEntry))
			return filepath.Join(root, "top", "repo", "src")
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			root := trRoot(t)
			home := filepath.Join(root, "home")
			trMkdir(t, home)
			work := r.setup(t, root)
			d := trNew(t, trHome(home), work, nil)
			p, err := d.Project()
			if r.marker == "" {
				if !errors.Is(err, ErrNoProject) {
					t.Errorf("Project() = %+v, %v; want ErrNoProject", p, err)
				}
			} else if want := filepath.Join(root, filepath.FromSlash(r.marker)); err != nil || p.Marker != want || p.Rule != r.rule {
				t.Errorf("Project() = %+v, %v; want %s rule %s", p, err, want, r.rule)
			}
			trCheckWarnings(t, d, r.warnings...)
		})
	}
}

// Paths(), PathIn() and the error paths of Path() as `paths --json` shows
// them.
func TestResolvePathsListing(t *testing.T) {
	w := trBuildPtrWorld(t)
	count := func(es []Entry) map[Kind]int {
		m := map[Kind]int{}
		for _, e := range es {
			m[e.Kind]++
		}
		return m
	}
	t.Run("in a project: both kinds once per home", func(t *testing.T) {
		d := trNew(t, trHome(w.home), w.proj, nil)
		n := count(d.Paths())
		for _, k := range []Kind{KindHome, KindConfig, KindConfigLocal} {
			if n[k] != 2 {
				t.Errorf("%s listed %d times, want 2", k, n[k])
			}
		}
		if n[KindDB] != 1 || n[KindCache] != 1 {
			t.Errorf("listing counts = %v", n)
		}
		if p, err := d.PathIn(KindDB, ScopeGlobal); err != nil || p != filepath.Join(w.global, trDBFile) {
			t.Errorf("PathIn(db, global) = %q, %v", p, err)
		}
		if p, err := d.PathIn(KindConfig, ScopeProject); err != nil || p != filepath.Join(w.marker, "config.jsonc") {
			t.Errorf("PathIn(config, project) = %q, %v", p, err)
		}
	})
	t.Run("outside a project: both kinds once", func(t *testing.T) {
		d := trNew(t, trHome(w.home), w.root, nil)
		if n := count(d.Paths()); n[KindConfig] != 1 {
			t.Errorf("config listed %d times, want 1", n[KindConfig])
		}
		if _, err := d.PathIn(KindConfig, ScopeProject); !errors.Is(err, ErrNoProject) {
			t.Errorf("PathIn(config, project) = %v, want ErrNoProject", err)
		}
	})
	t.Run("broken pointer: project line carries the error", func(t *testing.T) {
		trWrite(t, filepath.Join(w.marker, "config.local.jsonc"), `{ "home": "/no/such/place" }`)
		defer func() { _ = os.Remove(filepath.Join(w.marker, "config.local.jsonc")) }()
		d := trNew(t, trHome(w.home), w.proj, nil)
		var bad int
		for _, e := range d.Paths() {
			if e.Kind == KindConfig && e.Home == ScopeProject && strings.Contains(e.Error, "does not exist") {
				bad++
			}
		}
		if bad != 1 {
			t.Errorf("Paths() has %d broken project config lines, want 1: %+v", bad, d.Paths())
		}
	})
	t.Run("bad arguments", func(t *testing.T) {
		d := trNew(t, trHome(w.home), w.proj, nil)
		if _, err := d.Path("nope"); !errors.Is(err, ErrUnknownKind) {
			t.Errorf("Path(nope) = %v, want ErrUnknownKind", err)
		}
		if _, err := d.PathIn("nope", ScopeGlobal); !errors.Is(err, ErrUnknownKind) {
			t.Errorf("PathIn(nope) = %v, want ErrUnknownKind", err)
		}
		if _, err := d.PathIn(KindDB, ScopeBoth); !errors.Is(err, ErrInvalidOption) {
			t.Errorf("PathIn(db, both) = %v, want ErrInvalidOption", err)
		}
	})
}

// An author who renames the home folder and the env prefix gets both
// everywhere: ~/.notes, a .notes/ marker, NT_HOME and NT_<KIND>_DIR; the
// default TRAPP_* names mean nothing then (SPEC 3.3).
func TestResolveCustomNames(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	marker := filepath.Join(root, "proj", ".notes")
	ext := filepath.Join(root, "ext")
	trMkdir(t, filepath.Join(root, "proj", gitEntry), marker, filepath.Join(root, "proj", trMarker))
	trWrite(t, filepath.Join(home, ".notes", "home.jsonc"), `{ "app": "trapp", "layout": 2, "scope": "global" }`)
	custom := []Option{WithDirName(".notes"), WithEnvPrefix("NT")}
	env := map[string]string{"TRAPP_HOME": ext, "TRAPP_DB_DIR": ext, "NT_CACHE_DIR": ext}
	d := trNew(t, trHome(home), filepath.Join(root, "proj"), env, custom...)
	if d.App() != trApp || d.EnvPrefix() != "NT" || d.EnvName("HOME") != "NT_HOME" || d.Mode() != ModeProject || len(d.Kinds()) != 10 {
		t.Errorf("accessors: app %s prefix %s env %s mode %s kinds %d", d.App(), d.EnvPrefix(), d.EnvName("HOME"), d.Mode(), len(d.Kinds()))
	}
	if d.KindEnv(KindDB) != "NT_DB_DIR" || d.KindEnv(KindConfig) != "" || d.KindEnv("nope") != "" {
		t.Errorf("KindEnv: db %q config %q nope %q", d.KindEnv(KindDB), d.KindEnv(KindConfig), d.KindEnv("nope"))
	}
	g := d.Global()
	if g.Base != filepath.Join(home, ".notes") || g.Info == nil || *g.Info != (HomeInfo{App: trApp, Layout: 2, Scope: ScopeGlobal}) {
		t.Errorf("Global() = %+v info %+v", g, g.Info)
	}
	if p, err := d.Project(); err != nil || p.Marker != marker || p.Info != nil {
		t.Errorf("Project() = %+v, %v; want %s with no home.jsonc", p, err, marker)
	}
	trCheckKinds(t, d, map[Kind]trWant{
		KindDB:    {path: filepath.Join(marker, trDBFile), rule: RuleProject},
		KindCache: {path: ext, rule: RuleEnvKind},
	})
	d = trNew(t, trHome(home), filepath.Join(root, "proj"), map[string]string{"NT_HOME": ext}, custom...)
	trCheckKinds(t, d, trAllIn(ext, RuleEnvHome))
	t.Run("dev: ~/.notes-dev, leak filter on ~/.notes", func(t *testing.T) {
		d := trNew(t, trHome(home), filepath.Join(root, "proj"), map[string]string{"NT_HOME": filepath.Join(home, ".notes")}, append(custom, WithRelease(false))...)
		if g := d.Global(); g.Base != filepath.Join(home, ".notes-dev") {
			t.Errorf("dev Global() = %s", g.Base)
		}
		trCheckWarnings(t, d, WarnEnvPointsToProd)
	})
}

// New refuses a bad option set before resolving anything.
func TestResolveOptionValidation(t *testing.T) {
	root := trRoot(t)
	nilHome := WithHomeDir(nil)
	rows := []struct {
		name string
		app  string
		opts []Option
	}{
		{"app name with capitals", "TrApp", nil},
		{"app name with a slash", "tr/app", nil},
		{"dir name ..", trApp, []Option{WithDirName("..")}},
		{"dir name with a separator", trApp, []Option{WithDirName("a/b")}},
		{"empty env prefix", trApp, []Option{WithEnvPrefix("")}},
		{"unknown mode", trApp, []Option{WithMode("both")}},
		{"unknown lookup", trApp, []Option{WithLookup("down")}},
		{"layout version 0", trApp, []Option{WithLayoutVersion(0)}},
		{"socket max length 0", trApp, []Option{WithSocketMaxLen(0)}},
		{"nil seam", trApp, []Option{nilHome}},
		{"empty profile list", trApp, []Option{WithProfiles()}},
		{"release profile not in the list", trApp, []Option{WithReleaseProfile("live")}},
		{"dev profile not in the list", trApp, []Option{WithDevProfile("qa")}},
		{"run file name with a separator", trApp, []Option{WithRunFile(RunPID, "a/b.pid")}},
		{"legacy dir ..", trApp, []Option{WithLegacyHome(LegacyHome{Dir: ".."})}},
		{"kind name with capitals", trApp, []Option{WithKind("Notes")}},
		{"kind path leaving the home", trApp, []Option{WithKind("notes", Folder("../x"))}},
		{"kind with an unknown scope", trApp, []Option{WithKind("notes", Scoped("everywhere"))}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			opts := append([]Option{trHome(root), WithWorkDir(root), trEnv(nil), WithRelease(true)}, r.opts...)
			if _, err := New(r.app, opts...); !errors.Is(err, ErrInvalidOption) {
				t.Errorf("New = %v, want ErrInvalidOption", err)
			}
		})
	}
}

// TR1 (fixed): an author who drops the config kind (WithoutKind(KindConfig), or
// config-local) gets the home folder itself listed by ConfigFiles as a config
// file, so a loader would try to parse ~/.trapp and proj/.trapp as JSONC.
// applyPointers already skips a dropped kind (pointer.go "a dropped config
// kind"); ConfigFiles should leave it out the same way.
func TestResolveConfigFilesDroppedKind(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	marker := filepath.Join(root, "proj", trMarker)
	target := filepath.Join(root, "private", "h")
	trMkdir(t, home, filepath.Join(root, "proj", gitEntry))
	trInitHome(t, target)
	// The local pointer is still read when config.jsonc is dropped.
	trWrite(t, filepath.Join(marker, "config.local.jsonc"), `{ "home": "`+filepath.ToSlash(target)+`" }`)
	d := trNew(t, trHome(home), filepath.Join(root, "proj"), nil, WithoutKind(KindConfig))
	if p, err := d.Project(); err != nil || p.Base != target || p.Rule != RulePointer {
		t.Fatalf("Project() = %+v, %v; want the local home: pointer followed", p, err)
	}
	if _, err := d.Path(KindConfig); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("Path(config) = %v, want ErrUnknownKind", err)
	}
	for _, f := range d.ConfigFiles() {
		if !strings.HasSuffix(f.Path, "config.local.jsonc") {
			t.Errorf("ConfigFiles() lists %+v for a dropped config kind; want only config.local.jsonc files", f)
		}
	}
}

// The walk compares $HOME by identity: a symlinked $HOME still stops the
// walk, and its global home is still never a project home.
func TestResolveSymlinkedHome(t *testing.T) {
	root := trRoot(t)
	realHome := filepath.Join(root, "real-home")
	link := filepath.Join(root, "home-link")
	trInitHome(t, filepath.Join(realHome, trMarker))
	trMkdir(t, filepath.Join(realHome, "plain"), filepath.Join(realHome, "proj", trMarker), filepath.Join(realHome, "proj", "src"))
	trSymlink(t, realHome, link)
	for _, r := range []struct {
		name   string
		work   string
		marker string
	}{
		{"$HOME through the link", link, ""},
		{"plain folder through the link", filepath.Join(link, "plain"), ""},
		{"project through the link", filepath.Join(link, "proj", "src"), filepath.Join(realHome, "proj", trMarker)},
	} {
		t.Run(r.name, func(t *testing.T) {
			d := trNew(t, trHome(link), r.work, nil)
			p, err := d.Project()
			if r.marker == "" {
				if !errors.Is(err, ErrNoProject) {
					t.Errorf("Project() = %+v, %v; want ErrNoProject", p, err)
				}
				return
			}
			if err != nil || p.Marker != r.marker {
				t.Errorf("Project() = %+v, %v; want %s", p, err, r.marker)
			}
		})
	}
}

// The test guard (standard section 13): a test binary that resolves a home
// outside the test root panics instead of touching a real home.
func TestResolveTestGuard(t *testing.T) {
	root := trRoot(t)
	outside := string(filepath.Separator) + filepath.Join("tr-not-a-temp-folder", "home")
	t.Run("panics outside the test root", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil || !strings.Contains(r.(string), "outside the test root") {
				t.Errorf("recover() = %v, want the guard's panic", r)
			}
		}()
		_, _ = trTryNew(trHome(outside), root, nil)
	})
	t.Run("off with WithTestGuard(false)", func(t *testing.T) {
		d, err := trTryNew(trHome(outside), root, nil, WithTestGuard(false))
		if err != nil || d.Global().Base != filepath.Join(outside, trMarker) {
			t.Errorf("New = %v; global %s", err, d.Global().Base)
		}
	})
}
