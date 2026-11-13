package appdir_test

// Runnable godoc examples. Each one works inside its own temp root: the
// user's home is root/home (WithHomeDir), the tool runs from a folder under
// it (WithWorkDir), and the environment is a fixed map (WithEnv), so nothing
// on the machine running them changes the output. Paths are printed relative
// to the root with forward slashes, so the output is the same on Linux, macOS
// and Windows.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/appdir"
)

// exampleApp is the app name every example uses: home folder .lore, env
// prefix LORE_, db file lore.db.
const exampleApp = "lore"

// sandbox is one example's temp root with a fake user home and a project
// checkout under it.
type sandbox struct {
	root    string // the temp root, real path (macOS links /var to /private/var)
	home    string // root/home: the user's home folder
	project string // root/home/src/shop: a project checkout
	env     map[string]string
}

// newSandbox creates the temp root and the two folders. The caller removes
// it with s.close.
func newSandbox() *sandbox {
	root, err := os.MkdirTemp("", "appdir-example-*")
	if err != nil {
		panic(err)
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		panic(err)
	}
	s := &sandbox{
		root:    root,
		home:    filepath.Join(root, "home"),
		project: filepath.Join(root, "home", "src", "shop"),
		env:     map[string]string{},
	}
	if err := os.MkdirAll(s.project, 0o700); err != nil {
		panic(err)
	}
	return s
}

func (s *sandbox) close() { _ = os.RemoveAll(s.root) }

// opts are the seams that keep an example off the real machine, plus
// WithRelease(true) so the prod profile keeps plain names (.lore).
func (s *sandbox) opts(workDir string, extra ...appdir.Option) []appdir.Option {
	base := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return s.home, nil }),
		appdir.WithWorkDir(workDir),
		appdir.WithEnv(func(k string) (string, bool) { v, ok := s.env[k]; return v, ok }),
		appdir.WithRelease(true),
	}
	return append(base, extra...)
}

// open resolves lore from workDir.
func (s *sandbox) open(workDir string, extra ...appdir.Option) *appdir.Dirs {
	d, err := appdir.New(exampleApp, s.opts(workDir, extra...)...)
	if err != nil {
		panic(err)
	}
	return d
}

// rel prints p relative to the root with forward slashes.
func (s *sandbox) rel(p string) string {
	if p == "" {
		return "(none)"
	}
	r, err := filepath.Rel(s.root, p)
	if err != nil || strings.HasPrefix(r, "..") {
		return p
	}
	return filepath.ToSlash(r)
}

// write creates a file under the root, with its parent folders.
func (s *sandbox) write(p, body string) {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		panic(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		panic(err)
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// The quickstart path: New resolves every home and creates nothing; Path
// reads a kind's path; Ensure creates its folder on first use.
func ExampleNew() {
	s := newSandbox()
	defer s.close()
	d, err := appdir.New(exampleApp, s.opts(s.project)...)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("global home:", s.rel(d.Global().Base), d.Global().Rule)
	fmt.Println("has project:", d.HasProject())

	_, err = d.Ensure(appdir.KindDB)
	fmt.Println("no project:", errors.Is(err, appdir.ErrNoProject))

	if _, err := d.InitProject(s.project); err != nil { // what `lore init` runs
		fmt.Println(err)
		return
	}
	d = s.open(s.project)
	db, err := d.Ensure(appdir.KindDB)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("db:", s.rel(db))
	// Output:
	// global home: home/.lore global
	// has project: false
	// no project: true
	// db: home/src/shop/.lore/lore.db
}

// The app name becomes a folder and env names, so New refuses anything but
// ^[a-z0-9][a-z0-9._-]*$.
func ExampleNew_invalidName() {
	_, err := appdir.New("My App")
	fmt.Println(errors.Is(err, appdir.ErrInvalidOption))
	// Output:
	// true
}

// Reads never create anything; Ensure is the one lazy creation point. For a
// file kind it creates the parent folder only.
func ExampleDirs_Ensure() {
	s := newSandbox()
	defer s.close()
	d := s.open(s.home, appdir.WithMode(appdir.ModeGlobal))

	logs, _ := d.Path(appdir.KindLogs)
	fmt.Println("after Path:", s.rel(logs), exists(logs))

	logs, err := d.Ensure(appdir.KindLogs)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("after Ensure:", s.rel(logs), exists(logs))

	db, _ := d.Ensure(appdir.KindDB)
	fmt.Println("db file:", s.rel(db), exists(db), "parent:", exists(filepath.Dir(db)))
	// Output:
	// after Path: home/.lore/logs false
	// after Ensure: home/.lore/logs true
	// db file: home/.lore/lore.db false parent: true
}

// ForTest points HOME, the XDG folders, <APP>_HOME and the temp folder at a
// temp root for one test, and arms the guard that panics when New resolves
// a path outside it. It needs a *testing.T, so this example only compiles.
func ExampleForTest() {
	_ = func(t *testing.T) {
		root := appdir.ForTest(t, exampleApp)
		d, err := appdir.New(exampleApp, appdir.WithMode(appdir.ModeGlobal))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(d.Global().Base, root) {
			t.Fatalf("global home %s is outside %s", d.Global().Base, root)
		}
	}
}

// The mode sets where data kinds live. ModeProject (the default) keeps the
// db in the project home and fails outside one; ModeGlobal keeps it in the
// user's global home, so the tool works anywhere.
func ExampleWithMode() {
	s := newSandbox()
	defer s.close()
	for _, m := range []appdir.Mode{appdir.ModeProject, appdir.ModeGlobal} {
		d := s.open(s.home, appdir.WithMode(m))
		db, err := d.Path(appdir.KindDB)
		if err != nil {
			fmt.Printf("%s: no project home: %v\n", m, errors.Is(err, appdir.ErrNoProject))
			continue
		}
		fmt.Printf("%s: %s\n", m, s.rel(db))
	}
	// Output:
	// project: no project home: true
	// global: home/.lore/lore.db
}

// A release build uses the prod profile and plain names; any other build
// uses the dev profile, its own global home and suffixed names, so trying a
// change never touches the installed tool's data.
func ExampleWithProfile() {
	s := newSandbox()
	defer s.close()
	for _, release := range []bool{true, false} {
		d := s.open(s.home, appdir.WithRelease(release))
		fmt.Printf("release=%v profile=%s home=%s name=%s\n", release, d.Profile(), s.rel(d.Global().Base), d.Name("lored"))
	}
	d := s.open(s.home, appdir.WithRelease(false), appdir.WithProfile("prod"))
	fmt.Println("dev build, --profile prod:", d.Profile(), s.rel(d.Global().Base))

	_, err := appdir.New(exampleApp, s.opts(s.home, appdir.WithProfile("staging"))...)
	fmt.Println("unknown profile:", errors.Is(err, appdir.ErrUnknownProfile))
	// Output:
	// release=true profile=prod home=home/.lore name=lored
	// release=false profile=dev home=home/.lore-dev name=lored-dev
	// dev build, --profile prod: prod home/.lore
	// unknown profile: true
}

// Banner is the one stderr line a non-release profile prints at start.
func ExampleDirs_Banner() {
	s := newSandbox()
	defer s.close()
	fmt.Printf("prod: %q\n", s.open(s.home).Banner())
	fmt.Println("dev:", s.open(s.home, appdir.WithRelease(false)).Banner() != "")
	// Output:
	// prod: ""
	// dev: true
}

// Paths lists every kind with the home and the rule that picked it: what a
// tool's `paths --json` and `doctor` print. Here LORE_CACHE_DIR moves the
// cache; the .gitignore kind is project-only, so the global home has none.
func ExampleDirs_Paths() {
	s := newSandbox()
	defer s.close()
	s.env["LORE_CACHE_DIR"] = filepath.Join(s.root, "fast", "cache")
	d := s.open(s.home, appdir.WithMode(appdir.ModeGlobal))
	for _, e := range d.Paths() {
		if e.Home != appdir.ScopeGlobal {
			continue
		}
		fmt.Printf("%-12s %-6s %-6s %-8s %s\n", e.Kind, e.Type, e.Git, e.Rule, s.rel(e.Path))
	}
	// Output:
	// home         file   commit global   home/.lore/home.jsonc
	// config       file   commit global   home/.lore/config.jsonc
	// config-local file   ignore global   home/.lore/config.local.jsonc
	// shared       folder commit global   home/.lore/shared
	// db           file   ignore global   home/.lore/lore.db
	// backups      folder ignore global   home/.lore/backups
	// cache        folder ignore env-kind fast/cache
	// run          folder ignore global   home/.lore/run
	// logs         folder ignore global   home/.lore/logs
}

// Warnings are diagnostics for the tool to print once to stderr; doctor
// lists them too. Here a release build ignores LORE_PROFILE, which only
// --profile can change.
func ExampleDirs_Warnings() {
	s := newSandbox()
	defer s.close()
	s.env["LORE_PROFILE"] = "dev"
	d := s.open(s.home)
	fmt.Println("profile:", d.Profile())
	for _, w := range d.Warnings() {
		fmt.Println(w.Code)
	}
	// Output:
	// profile: prod
	// profile-env-ignored
}

// A home: key in the project's config.jsonc moves the project home, for
// example into a private repo next to the checkout. The value is relative
// to the folder holding the file, and the target must be a home of this app
// (InitHome makes one).
func ExampleDirs_Project_homePointer() {
	s := newSandbox()
	defer s.close()
	d := s.open(s.project)
	marker, err := d.InitProject(s.project)
	if err != nil {
		fmt.Println(err)
		return
	}
	s.write(filepath.Join(marker, "config.jsonc"), `{ "home": "../../shop-private/lore" }`)

	_, err = s.open(s.project).Project()
	fmt.Println("target missing:", errors.Is(err, appdir.ErrPointerMissing))

	target := filepath.Join(s.home, "src", "shop-private", "lore")
	if err := d.InitHome(target, appdir.ScopeProject); err != nil {
		fmt.Println(err)
		return
	}
	p, err := s.open(s.project).Project()
	if err != nil {
		fmt.Println(err)
		return
	}
	db, _ := s.open(s.project).Path(appdir.KindDB)
	fmt.Println("marker:", s.rel(p.Marker))
	fmt.Println("data:", s.rel(p.Data), p.Rule)
	fmt.Println("db:", s.rel(db))
	// Output:
	// target missing: true
	// marker: home/src/shop/.lore
	// data: home/src/shop-private/lore pointer
	// db: home/src/shop-private/lore/lore.db
}

// A db: key moves only the database. A committed file may not hold an
// absolute path (it would break every other clone); config.local.jsonc,
// this machine's own file, may.
func ExampleDirs_Path_dbPointer() {
	s := newSandbox()
	defer s.close()
	marker, err := s.open(s.project).InitProject(s.project)
	if err != nil {
		fmt.Println(err)
		return
	}
	abs := filepath.Join(s.root, "disk2", "lore.db")
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil { // a db: target's folder must exist
		fmt.Println(err)
		return
	}
	body := fmt.Sprintf("{ %q: %q }", "db", abs)

	s.write(filepath.Join(marker, "config.jsonc"), body)
	_, err = s.open(s.project).Path(appdir.KindDB)
	fmt.Println("committed:", errors.Is(err, appdir.ErrPointerAbsolute))

	s.write(filepath.Join(marker, "config.jsonc"), "{}")
	s.write(filepath.Join(marker, "config.local.jsonc"), body)
	db, err := s.open(s.project).Path(appdir.KindDB)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("local:", s.rel(db))
	// Output:
	// committed: true
	// local: disk2/lore.db
}

// ConfigFiles lists the config files in read order; later files win per
// key. Missing files are listed too: the loader treats them as empty.
// Committed is true only for a project file inside a git work tree; this
// sandbox has no git, so nothing is committed.
func ExampleDirs_ConfigFiles() {
	s := newSandbox()
	defer s.close()
	if _, err := s.open(s.project).InitProject(s.project); err != nil {
		fmt.Println(err)
		return
	}
	for _, f := range s.open(s.project).ConfigFiles() {
		fmt.Printf("%-7s local=%-5v committed=%-5v %s\n", f.Home, f.Local, f.Committed, s.rel(f.Path))
	}
	// Output:
	// global  local=false committed=false home/.lore/config.jsonc
	// global  local=true  committed=false home/.lore/config.local.jsonc
	// project local=false committed=false home/src/shop/.lore/config.jsonc
	// project local=true  committed=false home/src/shop/.lore/config.local.jsonc
}

// WithoutKind drops a kind the tool does not use; asking for it afterwards
// is an error. WithKind adds a kind of the tool's own.
func ExampleWithoutKind() {
	s := newSandbox()
	defer s.close()
	d := s.open(s.home,
		appdir.WithMode(appdir.ModeGlobal),
		appdir.WithoutKind(appdir.KindBackups),
		appdir.WithKind("models", appdir.Folder("models"), appdir.Disposable()),
	)
	_, err := d.Path(appdir.KindBackups)
	fmt.Println("backups:", errors.Is(err, appdir.ErrUnknownKind))
	models, _ := d.Path("models")
	fmt.Println("models:", s.rel(models))
	// Output:
	// backups: true
	// models: home/.lore/models
}

// MoveLegacy moves an old global home to the new one on first run, then
// reports "already" on every later run. It never deletes user data.
func ExampleDirs_MoveLegacy() {
	s := newSandbox()
	defer s.close()
	s.write(filepath.Join(s.home, ".pm", "pm.db"), "data")
	for range 2 {
		d := s.open(s.home,
			appdir.WithMode(appdir.ModeGlobal),
			appdir.WithLegacyHome(appdir.LegacyHome{Dir: ".pm"}),
		)
		res, err := d.MoveLegacy(context.Background())
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(res.Status, s.rel(res.From), "->", s.rel(res.To))
	}
	fmt.Println("data moved:", exists(filepath.Join(s.home, ".lore", "pm.db")))
	// Output:
	// moved home/.pm -> home/.lore
	// already home/.pm -> home/.lore
	// data moved: true
}

// CheckDB is the prod-db guard: a dev build refuses the release profile's
// database unless LORE_ALLOW_PROD=1, and then gets read-only access.
func ExampleDirs_CheckDB() {
	s := newSandbox()
	defer s.close()
	prodDB := filepath.Join(s.home, ".lore", "lore.db")
	dev := s.open(s.home, appdir.WithMode(appdir.ModeGlobal), appdir.WithRelease(false))
	_, err := dev.CheckDB(prodDB)
	fmt.Println("dev build:", errors.Is(err, appdir.ErrProdDB))

	s.env["LORE_ALLOW_PROD"] = "1"
	dev = s.open(s.home, appdir.WithMode(appdir.ModeGlobal), appdir.WithRelease(false))
	access, err := dev.CheckDB(prodDB)
	fmt.Println("allowed:", access, err)
	// Output:
	// dev build: true
	// allowed: read-only <nil>
}
