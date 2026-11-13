// Command prod-db-guard shows CheckDB, the guard a tool calls before it opens
// a database or runs a migration: a dev build refuses the prod databases,
// opens them read-only with DEMO_ALLOW_PROD=1, and opens its own read-write.
// It also shows the env-leak filter that keeps a dev build out of the prod
// home when the shell still carries DEMO_HOME.
//
// Everything happens inside a temp root: the user's home is root/home (passed
// with WithHomeDir), the tool runs in a project under it (WithWorkDir), and
// the environment is a map (WithEnv).
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example.
const (
	appName    = "demo"
	homeDir    = "home"
	projectDir = "home/proj"
	sharedDir  = "shared-data" // a db: pointer target outside the project
	sharedDB   = "team.db"

	envAllowProd = "DEMO_ALLOW_PROD"
	envHome      = "DEMO_HOME"
	allowProdOn  = "1"

	localConfig = "config.local.jsonc"
	homeFolder  = "." + appName // the project home folder, inside the project
	dbExt       = ".db"         // the db kind's file is <app>.db
	fakeDBBody  = "not really sqlite\n"
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
	root, home, project string
}

func main() {
	root, err := os.MkdirTemp("", "appdir-prod-db-guard-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "prod-db-guard:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "prod-db-guard:", err)
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
		root:    root,
		home:    filepath.Join(root, filepath.FromSlash(homeDir)),
		project: filepath.Join(root, filepath.FromSlash(projectDir)),
	}
	if err := os.MkdirAll(s.project, 0o700); err != nil {
		return fmt.Errorf("create project folder: %w", err)
	}
	// Before the release tool ever ran: there is no prod home yet.
	fmt.Fprintln(w, "== fresh machine: no prod home yet")
	if err := freshHome(w, s); err != nil {
		return err
	}
	prodGlobal, prodProject, err := s.makeProdDBs()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== dev build vs release build")
	if err := guard(w, s, prodGlobal, prodProject); err != nil {
		return err
	}
	fmt.Fprintln(w, "== relative path")
	if err := relativePath(w, s); err != nil {
		return err
	}
	fmt.Fprintln(w, "== env leak: DEMO_HOME=~/.demo in a dev shell")
	if err := envLeak(w, s, prodGlobal); err != nil {
		return err
	}
	fmt.Fprintln(w, "== db: pointer in the project's config.local.jsonc")
	return pointer(w, s)
}

// open resolves the app with the sandbox seams plus the case's own options.
func (s sandbox) open(env fakeEnv, opts ...appdir.Option) (*appdir.Dirs, error) {
	base := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return s.home, nil }),
		appdir.WithWorkDir(s.project),
		appdir.WithEnv(env.lookup),
	}
	return appdir.New(appName, append(base, opts...)...)
}

// makeProdDBs plays the installed (release) tool: it creates the project
// home and both prod databases, so the dev build below has something to hit.
func (s sandbox) makeProdDBs() (string, string, error) {
	if err := os.MkdirAll(s.project, 0o700); err != nil {
		return "", "", fmt.Errorf("create project folder: %w", err)
	}
	prod, err := s.open(nil, appdir.WithRelease(true))
	if err != nil {
		return "", "", fmt.Errorf("appdir: %w", err)
	}
	if _, err := prod.InitProject(s.project); err != nil {
		return "", "", fmt.Errorf("init project: %w", err)
	}
	// Resolve again: a Dirs is a snapshot, and the project home is new.
	if prod, err = s.open(nil, appdir.WithRelease(true)); err != nil {
		return "", "", fmt.Errorf("appdir: %w", err)
	}
	var paths []string
	for _, scope := range []appdir.Scope{appdir.ScopeGlobal, appdir.ScopeProject} {
		p, err := prod.PathIn(appdir.KindDB, scope)
		if err != nil {
			return "", "", fmt.Errorf("%s db path: %w", scope, err)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return "", "", fmt.Errorf("create %s db folder: %w", scope, err)
		}
		if err := os.WriteFile(p, []byte(fakeDBBody), 0o600); err != nil {
			return "", "", fmt.Errorf("write %s db: %w", scope, err)
		}
		paths = append(paths, p)
	}
	return paths[0], paths[1], nil
}

// snippet:openDB
// openDB is what a tool runs before it opens its database: CheckDB first,
// then open with the access it allows. ErrProdDB means "do not open at all".
func openDB(d *appdir.Dirs, path string) (appdir.DBAccess, error) {
	access, err := d.CheckDB(path)
	if errors.Is(err, appdir.ErrProdDB) {
		return access, err // the tool prints it and exits; never opens, never migrates
	}
	if err != nil {
		return access, fmt.Errorf("check %s: %w", path, err)
	}
	// AccessReadOnly: open read-only and skip migrations.
	return access, nil
}

// snippet:end

// snippet:guard
// guard tries the prod databases from a dev build (refused, then read-only
// with DEMO_ALLOW_PROD=1), the dev build's own database, and a release build.
func guard(w io.Writer, s sandbox, prodGlobal, prodProject string) error {
	dev, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	allowed, err := s.open(fakeEnv{envAllowProd: allowProdOn}, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	release, err := s.open(nil, appdir.WithRelease(true))
	if err != nil {
		return err
	}
	own, err := dev.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	cases := []struct {
		label string
		d     *appdir.Dirs
		path  string
	}{
		{"dev build, prod global db", dev, prodGlobal},
		{"dev build, prod project db", dev, prodProject},
		{"dev build + DEMO_ALLOW_PROD=1, prod project db", allowed, prodProject},
		{"dev build, its own db", dev, own},
		{"release build, prod project db", release, prodProject},
	}
	for _, c := range cases {
		access, err := openDB(c.d, c.path)
		if err != nil {
			fmt.Fprintf(w, "%-48s %s -> %s\n", c.label, s.show(c.path), s.clean(err.Error()))
			continue
		}
		fmt.Fprintf(w, "%-48s %s -> %s\n", c.label, s.show(c.path), access)
	}
	return nil
}

// snippet:end

// snippet:fresh
// freshHome runs a dev build on a machine where the installed tool never ran,
// so ~/.demo does not exist yet. The prod db path has no file and no folder
// to compare by identity; CheckDB compares it by real path and still refuses,
// so a dev build cannot create and migrate the prod db in a fresh home.
func freshHome(w io.Writer, s sandbox) error {
	prodHome := filepath.Join(s.home, homeFolder)
	if _, err := os.Stat(prodHome); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("want no %s yet, stat says %v", s.show(prodHome), err)
	}
	dev, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	prodDB := filepath.Join(prodHome, appName+dbExt)
	fmt.Fprintf(w, "dev build, %s (folder missing) -> %s\n", s.show(prodDB), s.verdict(openDB(dev, prodDB)))
	return nil
}

// snippet:end

// snippet:relative
// relativePath passes CheckDB a relative name. It is joined to the folder New
// was given (WithWorkDir, here the project), not to the process's current
// folder, so the guard checks the same file the tool will open from there.
func relativePath(w io.Writer, s sandbox) error {
	dev, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	rel := filepath.Join(homeFolder, appName+dbExt) // .demo/demo.db, the prod project db
	fmt.Fprintf(w, "dev build, %s from %s -> %s\n", filepath.ToSlash(rel), s.show(s.project), s.verdict(openDB(dev, rel)))
	return nil
}

// snippet:end

// snippet:leak
// envLeak starts a dev build in a shell opened by the installed tool, which
// exported DEMO_HOME=~/.demo. The value points into the prod home, so the
// dev build ignores it (with a warning) and keeps ~/.demo-dev; and if the
// tool still tries the prod db, CheckDB refuses it.
func envLeak(w io.Writer, s sandbox, prodGlobal string) error {
	d, err := s.open(fakeEnv{envHome: "~/.demo"}, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	for _, warn := range d.Warnings() {
		fmt.Fprintf(w, "warning %s: %s\n", warn.Code, s.clean(warn.Message))
	}
	fmt.Fprintf(w, "global home: %s\n", s.show(d.Global().Base))
	if _, err := openDB(d, prodGlobal); err != nil {
		fmt.Fprintf(w, "open %s: %s\n", s.show(prodGlobal), s.clean(err.Error()))
	}
	return nil
}

// snippet:end

// snippet:pointer
// pointer moves the prod project db elsewhere with a db: key in the
// project's config.local.jsonc. The prod view CheckDB builds follows the
// pointer, so a dev build refuses the target even before the file exists.
// The dev build itself ignores db: and keeps its database under profiles/.
func pointer(w io.Writer, s sandbox) error {
	target := filepath.Join(s.root, sharedDir, sharedDB)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create db folder: %w", err)
	}
	cfg := fmt.Sprintf("{ \"db\": %q }\n", filepath.ToSlash(target))
	if err := os.WriteFile(filepath.Join(s.project, homeFolder, localConfig), []byte(cfg), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", localConfig, err)
	}
	dev, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	for _, warn := range dev.Warnings() {
		fmt.Fprintf(w, "warning %s: %s\n", warn.Code, s.clean(warn.Message))
	}
	own, err := dev.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	for _, p := range []string{target, own} {
		access, err := openDB(dev, p)
		if err != nil {
			fmt.Fprintf(w, "dev build, %s -> %s\n", s.show(p), s.clean(err.Error()))
			continue
		}
		fmt.Fprintf(w, "dev build, %s -> %s\n", s.show(p), access)
	}
	return nil
}

// snippet:end

// show prints a path relative to root, so the output is the same on every run.
func (s sandbox) show(p string) string {
	if rel, err := filepath.Rel(s.root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}

// verdict is the access openDB allowed, or its refusal without the temp root.
func (s sandbox) verdict(access appdir.DBAccess, err error) string {
	if err != nil {
		return s.clean(err.Error())
	}
	return access.String()
}

// clean drops the temp root from a message, for the same reason.
func (s sandbox) clean(msg string) string {
	return filepath.ToSlash(strings.ReplaceAll(msg, s.root+string(filepath.Separator), ""))
}
