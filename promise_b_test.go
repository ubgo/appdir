package appdir

// Promise tests for SPEC section 4 (resolution) and the docs pages
// homes/*.md, pointers/pointer-keys.md and start/*.md: each test below proves
// one promise that no other test pinned down. The comment on each test names
// the SPEC section or docs page it proves. Helpers carry the pb prefix; the
// tr* helpers of resolve_matrix_test.go are reused for the common seams.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// pbTree lists every path under root, so a test can prove a call created
// nothing.
func pbTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

// SPEC 4.1: flags arrive only as options; appdir never parses os.Args. A
// process started with --home and --profile on its command line resolves as
// if they were not there.
func TestPromiseB_NeverParsesOSArgs(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	alt := filepath.Join(root, "alt")
	trMkdir(t, home, alt)
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = []string{saved[0], "--home", alt, "--profile", "dev", "--allow-root"}
	d := trNew(t, trHome(home), root, nil)
	if g := d.Global(); g.Rule != RuleGlobal || g.Base != filepath.Join(home, trMarker) {
		t.Errorf("Global() = %+v; want ~/.trapp rule global (os.Args ignored)", g)
	}
	if d.Profile() != ProfileProd {
		t.Errorf("Profile() = %s; want prod (os.Args ignored)", d.Profile())
	}
}

// SPEC 4.1: a variable that is set but empty counts as unset, for every
// <APP>_* variable. A dev build with every variable set to "" resolves
// exactly like one with none set, with no warning.
func TestPromiseB_EmptyAppVarsCountAsUnset(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	marker := filepath.Join(root, "proj", trMarker)
	trMkdir(t, home, filepath.Join(root, "proj", gitEntry), marker)
	work := filepath.Join(root, "proj")
	empty := map[string]string{}
	for _, v := range []string{"HOME", "PROFILE", "ALLOW_PROD", "CEILING_DIRS", "DAEMON_PROFILE", "DB_DIR", "CACHE_DIR", "LOGS_DIR", "SHARED_DIR", "BACKUPS_DIR", "RUN_DIR"} {
		empty["TRAPP_"+v] = ""
	}
	for _, release := range []bool{true, false} {
		base := trNew(t, trHome(home), work, nil, WithRelease(release))
		d := trNew(t, trHome(home), work, empty, WithRelease(release))
		if d.Profile() != base.Profile() {
			t.Errorf("release=%v: Profile() = %s, want %s", release, d.Profile(), base.Profile())
		}
		got, want := d.Paths(), base.Paths()
		if len(got) != len(want) {
			t.Fatalf("release=%v: %d entries, want %d", release, len(got), len(want))
		}
		for i := range got {
			if got[i].Path != want[i].Path || got[i].Rule != want[i].Rule || got[i].Error != want[i].Error {
				t.Errorf("release=%v: %s = %q rule %s err %q; want %q rule %s", release, got[i].Kind, got[i].Path, got[i].Rule, got[i].Error, want[i].Path, want[i].Rule)
			}
		}
		if len(d.Warnings()) != 0 {
			t.Errorf("release=%v: warnings %v for empty variables", release, d.Warnings())
		}
	}
}

// SPEC 4.1 example and 4.3 example: an unset ${VAR} never becomes a path at
// the root. The error names the flag or variable and the missing ${VAR}, and
// paths shows it on the failing lines (the one kind for <APP>_<KIND>_DIR,
// every line for --home / <APP>_HOME).
func TestPromiseB_UnsetVarErrorNamesItsSource(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	trMkdir(t, home, filepath.Join(root, "proj", gitEntry), filepath.Join(root, "proj", trMarker))
	work := filepath.Join(root, "proj")

	t.Run("kind dir", func(t *testing.T) {
		d := trNew(t, trHome(home), work, map[string]string{"TRAPP_CACHE_DIR": "${FAST}/trapp"})
		_, err := d.Path(KindCache)
		if !errors.Is(err, ErrEnvVar) || !strings.Contains(err.Error(), "TRAPP_CACHE_DIR: ${FAST} is not set") {
			t.Fatalf("Path(cache) = %v; want TRAPP_CACHE_DIR: ${FAST} is not set", err)
		}
		for _, e := range d.Paths() {
			if e.Kind == KindCache && !strings.Contains(e.Error, "${FAST} is not set") {
				t.Errorf("paths cache line = %+v; want the error", e)
			}
			if e.Kind != KindCache && e.Error != "" {
				t.Errorf("paths %s line has error %q; only the cache should fail", e.Kind, e.Error)
			}
		}
	})
	for _, r := range []struct {
		name   string
		env    map[string]string
		opts   []Option
		prefix string
	}{
		{"flag", nil, []Option{WithHome("${SCRATCH}/trapp")}, "--home: ${SCRATCH} is not set"},
		{"env", map[string]string{"TRAPP_HOME": "${SCRATCH}/trapp"}, nil, "TRAPP_HOME: ${SCRATCH} is not set"},
	} {
		t.Run(r.name, func(t *testing.T) {
			d := trNew(t, trHome(home), work, r.env, r.opts...)
			if _, err := d.Project(); err == nil || !strings.HasPrefix(err.Error(), r.prefix) {
				t.Errorf("Project() = %v; want it to start with %q", err, r.prefix)
			}
			for _, e := range d.Paths() {
				if !strings.HasPrefix(e.Error, r.prefix) {
					t.Errorf("paths %s line = %+v; want error %q", e.Kind, e, r.prefix)
				}
			}
		})
	}
}

// SPEC 4.1 and pointer-keys.md pitfalls: a relative pointer resolves against
// the folder holding the config file (.<app>/), never against the working
// folder, also in config.local.jsonc.
func TestPromiseB_RelativeLocalPointerAgainstConfigFolder(t *testing.T) {
	w := trBuildPtrWorld(t)
	deep := filepath.Join(w.proj, "a", "b")
	trMkdir(t, deep)
	trWrite(t, filepath.Join(w.marker, "config.local.jsonc"), `{ "home": "../../private/h" }`)
	d := trNew(t, trHome(w.home), deep, nil)
	p, err := d.Project()
	if err != nil || p.Base != w.target || p.Rule != RulePointer {
		t.Fatalf("Project() = %+v, %v; want %s (relative to %s)", p, err, w.target, w.marker)
	}
}

// SPEC 4.3 rule 3 and homes/global-and-project.md: appdir does not read
// XDG_CONFIG_HOME, XDG_DATA_HOME or XDG_STATE_HOME; the global home stays
// ~/.<app>.
func TestPromiseB_IgnoresXDGBaseDirs(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	xdg := filepath.Join(root, "xdg")
	trMkdir(t, home, xdg)
	env := map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(xdg, "config"),
		"XDG_DATA_HOME":   filepath.Join(xdg, "data"),
		"XDG_STATE_HOME":  filepath.Join(xdg, "state"),
		"XDG_CACHE_HOME":  filepath.Join(xdg, "cache"),
	}
	d := trNew(t, trHome(home), root, env, WithMode(ModeGlobal))
	if g := d.Global(); g.Base != filepath.Join(home, trMarker) || g.Rule != RuleGlobal {
		t.Errorf("Global() = %+v; want ~/.trapp", g)
	}
	for _, e := range d.Paths() {
		if strings.HasPrefix(e.Path, xdg) {
			t.Errorf("%s resolved into an XDG folder: %s", e.Kind, e.Path)
		}
	}
}

// SPEC 4.4 step 1: the start is the real path of the working folder; when
// that cannot be resolved (the folder does not exist yet) the lookup still
// runs from the cleaned absolute path and finds the project above it.
func TestPromiseB_LookupFromMissingWorkDir(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	marker := filepath.Join(root, "proj", trMarker)
	trMkdir(t, home, filepath.Join(root, "proj", gitEntry), marker)
	work := filepath.Join(root, "proj", "not", "yet", "..", "made")
	d := trNew(t, trHome(home), work, nil)
	if p, err := d.Project(); err != nil || p.Marker != marker {
		t.Fatalf("Project() from a missing folder = %+v, %v; want %s", p, err, marker)
	}
}

// start/first-use.md Step 2: New fails for an unreadable working folder (and
// a missing project home is not a New error).
func TestPromiseB_NewFailsOnUnreadableWorkDir(t *testing.T) {
	root := trRoot(t)
	gone := Option(func(c *config) {
		c.workDir = func() (string, error) { return "", errors.New("getwd: no such file or directory") }
	})
	if _, err := New(trApp, trHome(root), trEnv(nil), WithRelease(true), gone); err == nil || !strings.Contains(err.Error(), "working folder") {
		t.Fatalf("New with an unreadable working folder = %v; want an error naming the working folder", err)
	}
	if _, err := New(trApp, trHome(root), WithWorkDir(root), trEnv(nil), WithRelease(true)); err != nil {
		t.Fatalf("New outside a project = %v; want success (ErrNoProject comes later)", err)
	}
}

// homes/lookup.md pitfall "Only a folder counts": a file named .<app> is not
// a project home and does not stop the walk.
func TestPromiseB_MarkerFileIsNotAHome(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	proj := filepath.Join(root, "proj")
	marker := filepath.Join(proj, trMarker)
	sub := filepath.Join(proj, "sub")
	trMkdir(t, home, filepath.Join(proj, gitEntry), marker, filepath.Join(sub, "x"))
	trWrite(t, filepath.Join(sub, trMarker), "not a folder")
	p, err := trNew(t, trHome(home), filepath.Join(sub, "x"), nil).Project()
	if err != nil || p.Marker != marker {
		t.Fatalf("Project() = %+v, %v; want %s past the .trapp file", p, err, marker)
	}
	if _, err := trNew(t, trHome(home), sub, nil, WithLookup(HereOnly)).Project(); !errors.Is(err, ErrNoProject) {
		t.Fatalf("HereOnly at a folder holding a .trapp file = %v; want ErrNoProject", err)
	}
}

// homes/lookup.md pitfall "The lookup runs once, in New": a *Dirs is a
// snapshot. A marker made after New, or a chdir after New, does not change
// its project home; a new New sees the marker.
func TestPromiseB_LookupIsASnapshot(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	proj := filepath.Join(root, "proj")
	other := filepath.Join(root, "other")
	trMkdir(t, home, filepath.Join(proj, gitEntry), filepath.Join(other, gitEntry), filepath.Join(other, trMarker))
	d := trNew(t, trHome(home), proj, nil)
	trMkdir(t, filepath.Join(proj, trMarker))
	t.Chdir(other)
	if _, err := d.Project(); !errors.Is(err, ErrNoProject) {
		t.Fatalf("Project() after mkdir and chdir = %v; want the snapshot's ErrNoProject", err)
	}
	if p, err := trNew(t, trHome(home), proj, nil).Project(); err != nil || p.Marker != filepath.Join(proj, trMarker) {
		t.Fatalf("a new New = %+v, %v; want the new marker", p, err)
	}
}

// homes/worktrees.md pitfall: a worktree whose gitdir: points to a missing
// folder (main checkout moved or on another machine) is treated as a normal
// folder: its .git file is a wall, its own home is used with rule project,
// and no worktree warning is given.
func TestPromiseB_WorktreeWithMissingMain(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	top := filepath.Join(root, "top")
	wt := filepath.Join(top, "wt")
	trMkdir(t, home, filepath.Join(top, trMarker), filepath.Join(wt, "src"))
	trWrite(t, filepath.Join(wt, gitEntry), "gitdir: "+filepath.Join(root, "gone", gitEntry, "worktrees", "wt")+"\n")
	d := trNew(t, trHome(home), filepath.Join(wt, "src"), nil)
	if _, err := d.Project(); !errors.Is(err, ErrNoProject) {
		t.Errorf("Project() = %v; want ErrNoProject (the .git file is a wall)", err)
	}
	trCheckWarnings(t, d)
	trMkdir(t, filepath.Join(wt, trMarker))
	d = trNew(t, trHome(home), filepath.Join(wt, "src"), nil)
	if p, err := d.Project(); err != nil || p.Marker != filepath.Join(wt, trMarker) || p.Rule != RuleProject {
		t.Errorf("Project() = %+v, %v; want the worktree's own home, rule project", p, err)
	}
	trCheckWarnings(t, d)
}

// homes/worktrees.md pitfall: a home made only in the worktree loses to the
// main checkout's home, because nearest-wins is judged in the main
// checkout's tree.
func TestPromiseB_WorktreeOnlyHomeLosesToMain(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	main := filepath.Join(root, "shop")
	wt := filepath.Join(root, "shop-fix")
	trMkdir(t, home, filepath.Join(main, gitEntry), filepath.Join(main, trMarker), filepath.Join(wt, "pkg", trMarker))
	trWorktree(t, main, wt)
	p, err := trNew(t, trHome(home), filepath.Join(wt, "pkg"), nil).Project()
	if err != nil || p.Marker != filepath.Join(main, trMarker) || p.Rule != RuleWorktree {
		t.Fatalf("Project() = %+v, %v; want the main checkout's home, rule worktree", p, err)
	}
}

// SPEC 4.5 and pointers/pointer-keys.md: committed-file and pointer-error
// rules no matrix row pins: `~` alone and a ${VAR} holding a relative path
// are refused in config.jsonc; a non-string db: fails even in a dev profile
// that ignores db:; a broken pointer leaves HasProject false, puts its error
// on every project-capable line of Paths and leaves the global side of both
// kinds working.
func TestPromiseB_PointerRules(t *testing.T) {
	setup := func(t *testing.T, committed, local string) trPtrWorld {
		w := trBuildPtrWorld(t)
		if committed != "" {
			trWrite(t, filepath.Join(w.marker, "config.jsonc"), committed)
		}
		if local != "" {
			trWrite(t, filepath.Join(w.marker, "config.local.jsonc"), local)
		}
		return w
	}
	t.Run("committed ~ alone is refused", func(t *testing.T) {
		w := setup(t, `{ "home": "~" }`, "")
		if _, err := trNew(t, trHome(w.home), w.proj, nil).Project(); !errors.Is(err, ErrPointerAbsolute) {
			t.Fatalf("Project() = %v; want ErrPointerAbsolute", err)
		}
	})
	t.Run("committed ${VAR} holding a relative path is refused", func(t *testing.T) {
		w := setup(t, `{ "home": "${PRIV}/h" }`, "")
		if _, err := trNew(t, trHome(w.home), w.proj, map[string]string{"PRIV": "../../private"}).Project(); !errors.Is(err, ErrPointerAbsolute) {
			t.Fatalf("Project() = %v; want ErrPointerAbsolute", err)
		}
	})
	t.Run("dev: non-string db: still fails", func(t *testing.T) {
		w := setup(t, "", `{ "db": 5 }`)
		d := trNew(t, trHome(w.home), w.proj, nil, WithRelease(false))
		if _, err := d.Project(); !errors.Is(err, ErrInvalidOption) {
			t.Fatalf("dev Project() = %v; want ErrInvalidOption", err)
		}
	})
	t.Run("broken pointer: every project-capable line fails, global side works", func(t *testing.T) {
		w := setup(t, `{ "home": "../../private/none" }`, "")
		d := trNew(t, trHome(w.home), w.proj, nil)
		if d.HasProject() {
			t.Fatal("HasProject() = true with a broken pointer")
		}
		for _, e := range d.Paths() {
			global := e.Kind == KindCache || e.Home == ScopeGlobal
			switch {
			case global && e.Error != "":
				t.Errorf("global line %s carries %q", e.Kind, e.Error)
			case !global && !strings.Contains(e.Error, "does not exist"):
				t.Errorf("project line %s = %+v; want the pointer error", e.Kind, e)
			}
		}
		for _, k := range []Kind{KindConfig, KindConfigLocal, KindHome} {
			if p, err := d.PathIn(k, ScopeGlobal); err != nil || p == "" {
				t.Errorf("PathIn(%s, global) = %q, %v; want the global file", k, p, err)
			}
			if _, err := d.PathIn(k, ScopeProject); !errors.Is(err, ErrPointerMissing) {
				t.Errorf("PathIn(%s, project) = %v; want ErrPointerMissing", k, err)
			}
		}
	})
}

// SPEC 4.7 example "--home /tmp/try": one home; a both kind is listed once
// labelled global while a project kind such as db says project, every line in
// that one folder.
func TestPromiseB_OverrideLineLabels(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	try := filepath.Join(root, "try")
	trMkdir(t, home, try)
	d := trNew(t, trHome(home), root, nil, WithHome(try))
	for _, e := range d.Paths() {
		switch e.Kind {
		case KindConfig:
			if e.Home != ScopeGlobal || e.Path != filepath.Join(try, "config.jsonc") || e.Rule != RuleFlag {
				t.Errorf("config line = %+v", e)
			}
		case KindDB:
			if e.Home != ScopeProject || e.Path != filepath.Join(try, trDBFile) || e.Rule != RuleFlag {
				t.Errorf("db line = %+v", e)
			}
		}
	}
}

// SPEC 4.3 rule 4 and 4.7 example (CI with no $HOME): New works, global
// config is code defaults, and the global db says which variable to set.
func TestPromiseB_NoHomeMessage(t *testing.T) {
	root := trRoot(t)
	d := trNew(t, trNoHome(), root, nil, WithMode(ModeGlobal))
	_, err := d.Path(KindDB)
	if !errors.Is(err, ErrNoHome) || !strings.Contains(err.Error(), "no home folder; set TRAPP_HOME") {
		t.Fatalf("Path(db) = %v; want no home folder; set TRAPP_HOME", err)
	}
	if p, err := d.Path(KindConfig); p != "" || err != nil {
		t.Errorf("Path(config) = %q, %v; want code defaults (no path, no error)", p, err)
	}
}

// SPEC 4.8 ("never in reads") and start/first-use.md ("Reads never
// create"): New and every read call create nothing, and the write checks do
// not run on reads: a home whose layout is too new still answers Path,
// PathIn, Paths and ConfigFiles; only Ensure refuses.
func TestPromiseB_ReadsNeverWriteOrCheck(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	global := filepath.Join(home, trMarker)
	trWrite(t, filepath.Join(global, "home.jsonc"), `{ "app": "trapp", "layout": 99, "scope": "global" }`)
	before := pbTree(t, root)
	d := trNew(t, trHome(home), root, nil, WithMode(ModeGlobal))
	for _, k := range []Kind{KindDB, KindCache, KindLogs, KindConfig, KindRun} {
		if p, err := d.Path(k); err != nil || p == "" {
			t.Errorf("Path(%s) = %q, %v; want a path (reads skip the layout check)", k, p, err)
		}
		if _, err := d.PathIn(k, ScopeGlobal); err != nil {
			t.Errorf("PathIn(%s, global) = %v", k, err)
		}
	}
	for _, e := range d.Paths() {
		if e.Error != "" && e.Kind != KindGitignore { // gitignore is project only
			t.Errorf("Paths %s: %s", e.Kind, e.Error)
		}
	}
	_ = d.ConfigFiles()
	_ = d.Global()
	_, _ = d.Project()
	_ = d.Warnings()
	if after := pbTree(t, root); !slices.Equal(before, after) {
		t.Errorf("reads created files:\nbefore %v\nafter  %v", before, after)
	}
	if _, err := d.Ensure(KindCache); !errors.Is(err, ErrLayoutTooNew) {
		t.Errorf("Ensure(cache) = %v; want ErrLayoutTooNew", err)
	}
}

// homes/global-and-project.md Step 4 and homes/overrides.md pitfall:
// WithDirName renames the folders only; the env names stay <APP>_*.
func TestPromiseB_DirNameKeepsEnvNames(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	alt := filepath.Join(root, "alt")
	trMkdir(t, home, alt)
	d := trNew(t, trHome(home), root, map[string]string{"TRAPP_HOME": alt}, WithDirName(".notes"))
	if d.EnvPrefix() != "TRAPP" || d.EnvName("HOME") != "TRAPP_HOME" || d.KindEnv(KindDB) != "TRAPP_DB_DIR" {
		t.Errorf("names = %s %s %s; want TRAPP, TRAPP_HOME, TRAPP_DB_DIR", d.EnvPrefix(), d.EnvName("HOME"), d.KindEnv(KindDB))
	}
	if g := d.Global(); g.Base != alt || g.Rule != RuleEnvHome {
		t.Errorf("Global() = %+v; want TRAPP_HOME honoured", g)
	}
	if g := trNew(t, trHome(home), root, nil, WithDirName(".notes")).Global(); g.Base != filepath.Join(home, ".notes") {
		t.Errorf("Global() = %s; want ~/.notes", g.Base)
	}
}

// homes/overrides.md Step 5: the default env prefix is the app name upper
// cased with - and . turned into _.
func TestPromiseB_DefaultEnvPrefix(t *testing.T) {
	root := trRoot(t)
	d, err := New("my-app.v2", trHome(root), WithWorkDir(root), trEnv(nil), WithRelease(true))
	if err != nil {
		t.Fatal(err)
	}
	if d.EnvPrefix() != "MY_APP_V2" || d.EnvName("HOME") != "MY_APP_V2_HOME" {
		t.Errorf("EnvPrefix() = %s, EnvName(HOME) = %s; want MY_APP_V2", d.EnvPrefix(), d.EnvName("HOME"))
	}
}

// homes/overrides.md "Before you start": WithHome("") means the flag was not
// given: the normal global home and project lookup apply.
func TestPromiseB_EmptyHomeFlagIsNotGiven(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	marker := filepath.Join(root, "proj", trMarker)
	trMkdir(t, home, filepath.Join(root, "proj", gitEntry), marker)
	d := trNew(t, trHome(home), filepath.Join(root, "proj"), nil, WithHome(""))
	if g := d.Global(); g.Rule != RuleGlobal || g.Base != filepath.Join(home, trMarker) {
		t.Errorf("Global() = %+v; want rule global", g)
	}
	if p, err := d.Project(); err != nil || p.Marker != marker || p.Rule != RuleProject {
		t.Errorf("Project() = %+v, %v; want %s", p, err, marker)
	}
}

// SPEC 4.6: profiles/ is ignored by the gitignore block: the block ignores
// everything and never re-includes profiles/.
func TestPromiseB_GitignoreBlockIgnoresProfiles(t *testing.T) {
	root := trRoot(t)
	b := trNew(t, trHome(root), root, nil).GitignoreBlock()
	if !strings.Contains(b, "\n*\n") || strings.Contains(b, "!"+profilesDir) {
		t.Errorf("block %q; want * and no !%s", b, profilesDir)
	}
}

// TestPromiseB_WriteGitignoreChecksWrite proves SPEC 4.8 for WriteGitignore:
// the layout check runs in every write method, so a project home whose
// home.jsonc has a newer layout refuses the .gitignore write too, and the
// file is not created.
func TestPromiseB_WriteGitignoreChecksWrite(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	proj := filepath.Join(root, "proj")
	trMkdir(t, home, filepath.Join(proj, gitEntry))
	trWrite(t, filepath.Join(proj, trMarker, "home.jsonc"), `{ "app": "trapp", "layout": 99, "scope": "project" }`)
	wrote, err := trNew(t, trHome(home), proj, nil).WriteGitignore()
	if wrote || !errors.Is(err, ErrLayoutTooNew) {
		t.Fatalf("WriteGitignore = %v, %v; want false, ErrLayoutTooNew", wrote, err)
	}
	if _, err := os.Stat(filepath.Join(proj, trMarker, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf(".gitignore written despite the layout check: %v", err)
	}
}
