package appdir

// Promise tests for SPEC sections 1, 2 and 3 (purpose, design rules, public
// API) and the reference pages docs/reference/*.md. Each test names the
// promise and the section it proves. Helpers are prefixed pa.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ubgo/appdir/internal/flock"
)

const (
	paApp    = "paapp"
	paPrefix = "PAAPP"
	paMarker = "." + paApp
)

// paWorld is one isolated machine: a user home, a project folder with a git
// repo and a .paapp marker, and a folder outside any project.
type paWorld struct {
	root, home, proj, marker, out string
}

func paNewWorld(t *testing.T) paWorld {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := paWorld{
		root:   root,
		home:   filepath.Join(root, "home"),
		proj:   filepath.Join(root, "proj"),
		marker: filepath.Join(root, "proj", paMarker),
		out:    filepath.Join(root, "elsewhere"),
	}
	for _, d := range []string{w.home, w.marker, filepath.Join(w.proj, gitEntry), w.out} {
		paMkdir(t, d)
	}
	return w
}

func paMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func paWrite(t *testing.T, path, body string) {
	t.Helper()
	paMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// paEnv is an env seam over a fixed map; the process env is never read.
func paEnv(m map[string]string) Option {
	return WithEnv(func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	})
}

// paNoHome is a home seam with no user home folder.
func paNoHome() Option {
	return WithHomeDir(func() (string, error) { return "", errors.New("no home") })
}

// opts are the seams for this world: home, work dir, env, a release build,
// and a socket limit the deep temp root fits in (later options win).
func (w paWorld) opts(work string, env map[string]string, extra ...Option) []Option {
	home := w.home
	base := []Option{
		WithHomeDir(func() (string, error) { return home, nil }),
		WithWorkDir(work),
		paEnv(env),
		WithRelease(true),
		WithSocketMaxLen(4096),
	}
	return append(base, extra...)
}

func (w paWorld) new(t *testing.T, work string, env map[string]string, extra ...Option) *Dirs {
	t.Helper()
	d, err := New(paApp, w.opts(work, env, extra...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

// paCheckErr proves the SPEC 3.5 contract for one returned error: it wraps
// want (also after the tool wraps it again with %w), its text is a sentence
// that names what it is about (frag), and the sentinel's own text is not
// repeated in it (an appError prints only its message, never "msg: sentinel").
func paCheckErr(t *testing.T, err, want error, frag string) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want %v", want)
	}
	if wrapped := fmt.Errorf("tool: %w", err); !errors.Is(wrapped, want) {
		t.Fatalf("errors.Is(%q, %v) = false", err, want)
	}
	msg := err.Error()
	if frag != "" && !strings.Contains(msg, frag) {
		t.Errorf("message %q does not contain %q", msg, frag)
	}
	if n := strings.Count(msg, want.Error()); n > 1 || strings.HasSuffix(msg, ": "+want.Error()) {
		t.Errorf("message %q repeats the sentinel text %q", msg, want.Error())
	}
}

// SPEC 3.5: every sentinel is returned in its trigger case, wraps through a
// caller's %w, and its message names the path, key or variable. (ErrRootOwned
// needs euid 0: TestPromiseA_RootOwned in promise_a_linux_test.go.)
func TestPromiseA_SentinelCatalog(t *testing.T) {
	t.Run("ErrInvalidOption", func(t *testing.T) {
		w := paNewWorld(t)
		_, err := New(paApp, w.opts(w.proj, nil, WithDirName(".."))...)
		paCheckErr(t, err, ErrInvalidOption, `appdir: dir name ".." must be one plain path segment`)
	})
	t.Run("ErrUnknownProfile", func(t *testing.T) {
		w := paNewWorld(t)
		_, err := New(paApp, w.opts(w.proj, nil, WithProfile("devv"))...)
		paCheckErr(t, err, ErrUnknownProfile, `appdir: --profile: unknown profile "devv" (known: prod, dev)`)
		_, err = New(paApp, w.opts(w.proj, map[string]string{paPrefix + "_PROFILE": "devv"}, WithRelease(false))...)
		paCheckErr(t, err, ErrUnknownProfile, paPrefix+`_PROFILE: unknown profile "devv"`)
		_, err = New(paApp, w.opts(w.proj, nil, WithDaemonProfile("devv"))...)
		paCheckErr(t, err, ErrUnknownProfile, `--daemon-profile: unknown profile "devv"`)
		_, err = New(paApp, w.opts(w.proj, map[string]string{paPrefix + "_DAEMON_PROFILE": "devv"})...)
		paCheckErr(t, err, ErrUnknownProfile, paPrefix+`_DAEMON_PROFILE: unknown profile "devv"`)
	})
	t.Run("ErrUnknownKind", func(t *testing.T) {
		w := paNewWorld(t)
		d := w.new(t, w.proj, nil)
		_, err := d.Path("nope")
		paCheckErr(t, err, ErrUnknownKind, `appdir: unknown kind "nope"`)
		_, err = d.PathIn("nope", ScopeGlobal)
		paCheckErr(t, err, ErrUnknownKind, `"nope"`)
		_, err = d.Ensure("nope")
		paCheckErr(t, err, ErrUnknownKind, `"nope"`)
	})
	t.Run("ErrNoProject", func(t *testing.T) {
		w := paNewWorld(t)
		d := w.new(t, w.out, nil)
		_, err := d.Path(KindDB)
		paCheckErr(t, err, ErrNoProject, "no .paapp/ here; run 'paapp init' or pass --home")
		_, err = d.Project()
		paCheckErr(t, err, ErrNoProject, "no .paapp/ here")
	})
	t.Run("ErrNoHome", func(t *testing.T) {
		w := paNewWorld(t)
		d, err := New(paApp, append(w.opts(w.out, nil), paNoHome(), WithMode(ModeGlobal))...)
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.Path(KindDB)
		paCheckErr(t, err, ErrNoHome, "no home folder; set PAAPP_HOME")
		_, err = d.Ensure(KindConfig)
		paCheckErr(t, err, ErrNoHome, "config has no path")
		d, err = New(paApp, append(w.opts(w.out, nil), paNoHome(), WithHome("~/x"))...)
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.Path(KindDB)
		paCheckErr(t, err, ErrNoHome, `--home: "~/x" needs a home folder`)
	})
	t.Run("ErrEnvVar", func(t *testing.T) {
		w := paNewWorld(t)
		d := w.new(t, w.proj, map[string]string{paPrefix + "_CACHE_DIR": "${PA_FAST}/c"})
		_, err := d.Path(KindCache)
		paCheckErr(t, err, ErrEnvVar, "PAAPP_CACHE_DIR: ${PA_FAST} is not set")
	})
	t.Run("ErrPointerAbsolute", func(t *testing.T) {
		w := paNewWorld(t)
		target := filepath.ToSlash(filepath.Join(w.root, "target"))
		paWrite(t, filepath.Join(w.marker, "config.jsonc"), `{ "home": "`+target+`" }`)
		d := w.new(t, w.proj, nil)
		_, err := d.Project()
		paCheckErr(t, err, ErrPointerAbsolute, `home: "`+target+`" in `+filepath.Join(w.marker, "config.jsonc")+` is absolute; put it in config.local.jsonc`)
	})
	t.Run("ErrPointerMissing", func(t *testing.T) {
		w := paNewWorld(t)
		missing := filepath.Join(w.root, "missing")
		paWrite(t, filepath.Join(w.marker, "config.local.jsonc"), `{ "home": "`+filepath.ToSlash(missing)+`" }`)
		_, err := w.new(t, w.proj, nil).Project()
		paCheckErr(t, err, ErrPointerMissing, "pointer points to "+missing+", which does not exist; run 'paapp init --home "+missing+"'")
	})
	t.Run("ErrPointerNotHome", func(t *testing.T) {
		w := paNewWorld(t)
		target := filepath.Join(w.root, "nothome")
		paMkdir(t, target)
		paWrite(t, filepath.Join(w.marker, "config.local.jsonc"), `{ "home": "`+filepath.ToSlash(target)+`" }`)
		_, err := w.new(t, w.proj, nil).Project()
		paCheckErr(t, err, ErrPointerNotHome, "pointer target "+target+" is not a paapp home (no home.jsonc with app: paapp)")
	})
	t.Run("ErrLayoutTooNew", func(t *testing.T) {
		w := paNewWorld(t)
		paWrite(t, filepath.Join(w.marker, "home.jsonc"), `{ "app": "paapp", "layout": 2, "scope": "project" }`)
		_, err := w.new(t, w.proj, nil).Ensure(KindDB)
		paCheckErr(t, err, ErrLayoutTooNew, "home "+w.marker+" has layout 2, this binary knows 1; upgrade paapp")
	})
	t.Run("ErrSymlinkEscape", func(t *testing.T) {
		w := paNewWorld(t)
		outside := filepath.Join(w.root, "outside.db")
		paWrite(t, outside, "x")
		link := filepath.Join(w.marker, "config.local.jsonc")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		err := w.new(t, w.proj, nil).VerifyFile(link)
		paCheckErr(t, err, ErrSymlinkEscape, link+" links to "+outside+", outside the home "+w.marker)
	})
	t.Run("ErrUnsafeTemp", func(t *testing.T) {
		w := paNewWorld(t)
		for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
			t.Setenv(k, w.root)
		}
		tmp := filepath.Join(w.root, paApp+"-"+currentUser())
		paWrite(t, tmp, "a file in the way")
		d, err := New(paApp, append(w.opts(w.out, nil), paNoHome())...)
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.Ensure(KindCache)
		paCheckErr(t, err, ErrUnsafeTemp, tmp+" is a symlink or not a folder")
	})
	t.Run("ErrProdDB", func(t *testing.T) {
		w := paNewWorld(t)
		d := w.new(t, w.proj, nil, WithRelease(false))
		prod := filepath.Join(w.marker, paApp+".db")
		_, err := d.CheckDB(prod)
		paCheckErr(t, err, ErrProdDB, "dev build refuses the prod database "+prod+"; set PAAPP_ALLOW_PROD=1 to open it read-only")
	})
	t.Run("ErrBusy", func(t *testing.T) {
		w := paNewWorld(t)
		old := filepath.Join(w.home, ".paold")
		paMkdir(t, filepath.Join(old, "run"))
		lk, err := flock.TryLock(filepath.Join(old, "run", "daemon.lock"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lk.Release() }()
		d := w.new(t, w.out, nil, WithMode(ModeGlobal), WithLegacyHome(LegacyHome{Dir: ".paold"}))
		_, err = d.MoveLegacy(context.Background())
		paCheckErr(t, err, ErrBusy, "old home "+old+" is in use by a running daemon; will retry next run")
	})
	t.Run("ErrGitignoreMarkers", func(t *testing.T) {
		w := paNewWorld(t)
		gi := filepath.Join(w.marker, ".gitignore")
		paWrite(t, gi, "# paapp:begin\n*\n")
		_, err := w.new(t, w.proj, nil).WriteGitignore()
		paCheckErr(t, err, ErrGitignoreMarkers, gi+": # paapp:begin / # paapp:end missing, doubled or out of order; fix it by hand")
	})
	t.Run("ErrNotHome", func(t *testing.T) {
		w := paNewWorld(t)
		other := filepath.Join(w.root, "other")
		paWrite(t, filepath.Join(other, "home.jsonc"), `{ "app": "agx", "layout": 1, "scope": "project" }`)
		err := w.new(t, w.proj, nil).InitHome(other, ScopeProject)
		paCheckErr(t, err, ErrNotHome, other+` holds a home of "agx"`)
	})
	t.Run("ErrBadHomeFile", func(t *testing.T) {
		w := paNewWorld(t)
		bad := filepath.Join(w.root, "bad")
		paWrite(t, filepath.Join(bad, "home.jsonc"), `{`)
		err := w.new(t, w.proj, nil).InitHome(bad, ScopeProject)
		paCheckErr(t, err, ErrBadHomeFile, "init: cannot read "+filepath.Join(bad, "home.jsonc")+"; fix or remove it: ")
		if b, _ := os.ReadFile(filepath.Join(bad, "home.jsonc")); string(b) != "{" {
			t.Errorf("home.jsonc changed to %q", b)
		}
	})
}

// SPEC 3.5: an error about a user path carries a prefix naming where the
// value came from: --home, <APP>_HOME, <APP>_<KIND>_DIR, home in <file>,
// db in <file>.
func TestPromiseA_ErrorSourcePrefix(t *testing.T) {
	w := paNewWorld(t)
	cases := []struct {
		name   string
		env    map[string]string
		extra  []Option
		local  string // config.local.jsonc body
		kind   Kind
		prefix string
	}{
		{name: "flag", extra: []Option{WithHome("${PA_NOPE}/h")}, kind: KindDB, prefix: "--home: ${PA_NOPE} is not set"},
		{name: "app home env", env: map[string]string{paPrefix + "_HOME": "${PA_NOPE}"}, kind: KindDB, prefix: "PAAPP_HOME: ${PA_NOPE} is not set"},
		{name: "kind env", env: map[string]string{paPrefix + "_CACHE_DIR": "${PA_NOPE}"}, kind: KindCache, prefix: "PAAPP_CACHE_DIR: ${PA_NOPE} is not set"},
		{name: "home pointer", local: `{ "home": "${PA_NOPE}/h" }`, kind: KindDB, prefix: "home in " + filepath.Join(w.marker, "config.local.jsonc") + ": ${PA_NOPE} is not set"},
		{name: "db pointer", local: `{ "db": "${PA_NOPE}/x.db" }`, kind: KindDB, prefix: "db in " + filepath.Join(w.marker, "config.local.jsonc") + ": ${PA_NOPE} is not set"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			local := filepath.Join(w.marker, "config.local.jsonc")
			_ = os.Remove(local)
			if c.local != "" {
				paWrite(t, local, c.local)
				defer func() { _ = os.Remove(local) }()
			}
			_, err := w.new(t, w.proj, c.env, c.extra...).Path(c.kind)
			paCheckErr(t, err, ErrEnvVar, "")
			if !strings.HasPrefix(err.Error(), c.prefix) {
				t.Errorf("message %q, want prefix %q", err, c.prefix)
			}
		})
	}
}

// SPEC 3.5, the ErrInvalidOption cases outside New, each with its message.
func TestPromiseA_InvalidOptionOutsideNew(t *testing.T) {
	w := paNewWorld(t)
	t.Run("~user", func(t *testing.T) {
		_, err := w.new(t, w.proj, map[string]string{paPrefix + "_CACHE_DIR": "~bob/cache"}).Path(KindCache)
		paCheckErr(t, err, ErrInvalidOption, `PAAPP_CACHE_DIR: "~bob/cache": the ~user form is not supported`)
	})
	t.Run("unclosed ${", func(t *testing.T) {
		_, err := w.new(t, w.proj, map[string]string{paPrefix + "_CACHE_DIR": "${FAST/x"}).Path(KindCache)
		paCheckErr(t, err, ErrInvalidOption, `PAAPP_CACHE_DIR: "${FAST/x": unclosed ${`)
	})
	t.Run("pointer not a string", func(t *testing.T) {
		cfg := filepath.Join(w.marker, "config.jsonc")
		paWrite(t, cfg, `{ "home": 5 }`)
		defer func() { _ = os.Remove(cfg) }()
		d := w.new(t, w.proj, nil)
		_, err := d.Project()
		paCheckErr(t, err, ErrInvalidOption, cfg+": home must be a string path")
		for _, k := range []Kind{KindDB, KindLogs, KindConfig} {
			if _, err := d.Path(k); !errors.Is(err, ErrInvalidOption) {
				t.Errorf("Path(%s) = %v, want ErrInvalidOption", k, err)
			}
		}
	})
	d := w.new(t, w.proj, nil)
	t.Run("PathIn both", func(t *testing.T) {
		_, err := d.PathIn(KindConfig, ScopeBoth)
		paCheckErr(t, err, ErrInvalidOption, `appdir: PathIn needs global or project, got "both"`)
	})
	t.Run("InitHome both", func(t *testing.T) {
		target := filepath.Join(w.root, "ih")
		err := d.InitHome(target, ScopeBoth)
		paCheckErr(t, err, ErrInvalidOption, `appdir: InitHome needs global or project, got "both"`)
		if exists(target) {
			t.Errorf("InitHome created %s", target)
		}
	})
	t.Run("RunPath unknown", func(t *testing.T) {
		_, err := d.RunPath("nope")
		paCheckErr(t, err, ErrInvalidOption, `appdir: unknown run file "nope"`)
	})
	t.Run("InitProject on the global home", func(t *testing.T) {
		_, err := d.InitProject(w.home)
		paCheckErr(t, err, ErrInvalidOption, "init: "+filepath.Join(w.home, paMarker)+" is the global home, not a project")
		if exists(filepath.Join(w.home, paMarker)) {
			t.Error("InitProject created the global home")
		}
	})
}

// paGoFiles parses the non-test Go files of dir.
func paGoFiles(t *testing.T, dir string) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return fset, files
}

// SPEC 2 "No hidden state": no package-level mutable variables (the sentinel
// errors in errors.go are the only package-level vars) and no init().
func TestPromiseA_NoPackageState(t *testing.T) {
	fset, files := paGoFiles(t, ".")
	if len(files) == 0 {
		t.Fatal("no Go files found")
	}
	for _, f := range files {
		name := filepath.Base(fset.Position(f.Pos()).Filename)
		for _, decl := range f.Decls {
			switch dd := decl.(type) {
			case *ast.FuncDecl:
				if dd.Recv == nil && dd.Name.Name == "init" {
					t.Errorf("%s: func init() found", name)
				}
			case *ast.GenDecl:
				if dd.Tok != token.VAR {
					continue
				}
				for _, s := range dd.Specs {
					for _, id := range s.(*ast.ValueSpec).Names {
						if name != "errors.go" || !strings.HasPrefix(id.Name, "Err") {
							t.Errorf("%s: package-level var %s", name, id.Name)
						}
					}
				}
			}
		}
	}
}

// SPEC 2 "Stdlib only" and platforms.md "go.mod has no requirements": no
// require in go.mod and every non-test import is the standard library or
// appdir's own internal packages.
func TestPromiseA_StdlibOnly(t *testing.T) {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "require") {
			t.Errorf("go.mod has a requirement: %q", line)
		}
	}
	dirs := []string{"."}
	ents, err := os.ReadDir("internal")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join("internal", e.Name()))
		}
	}
	for _, dir := range dirs {
		fset, files := paGoFiles(t, dir)
		for _, f := range files {
			for _, imp := range f.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				first, _, _ := strings.Cut(p, "/")
				if strings.Contains(first, ".") && !strings.HasPrefix(p, "github.com/ubgo/appdir/internal/") {
					t.Errorf("%s imports %s, not the standard library", fset.Position(f.Pos()).Filename, p)
				}
			}
		}
	}
}

// paSnapshot lists every entry under root with its mode, size and mtime.
func paSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		out[p] = fmt.Sprintf("%v %d %d", fi.Mode(), fi.Size(), fi.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// paReadAll calls every read method of d (everything except Ensure,
// InitProject, InitHome, EnsureSocketDir, WriteGitignore and MoveLegacy).
func paReadAll(d *Dirs, w paWorld) {
	_, _, _, _ = d.App(), d.EnvPrefix(), d.EnvName("HOME"), d.Mode()
	_, _, _ = d.Kinds(), d.Global(), d.HasProject()
	_, _ = d.Project()
	for _, k := range d.Kinds() {
		_ = d.KindEnv(k.Name)
		_, _ = d.Path(k.Name)
		_, _ = d.PathIn(k.Name, ScopeGlobal)
		_, _ = d.PathIn(k.Name, ScopeProject)
	}
	_, _, _ = d.Paths(), d.ConfigFiles(), d.GitignoreBlock()
	_ = d.VerifyFile(filepath.Join(w.marker, "config.jsonc"))
	_, _, _, _ = d.Profile(), d.IsRelease(), d.ReleaseReason(), d.Name("x")
	_, _, _ = d.NameWith("x", "."), d.Banner(), d.Warnings()
	_, _ = d.CheckDB(filepath.Join(w.marker, paApp+".db"))
	_, _ = d.CheckDB(filepath.Join(w.home, paMarker, paApp+".db"))
	_, _ = d.ForProfile(ProfileProd)
	_, _ = d.ForProfile(ProfileDev)
	_ = d.ChildEnv([]string{"A=1"})
	for _, f := range []RunFile{RunLock, RunDaemonLock, RunPID, RunDiscovery, RunSocket, RunSocketFile} {
		_, _ = d.RunPath(f)
	}
	_, _ = d.Socket()
	_ = d.DaemonProfile()
	_, _, _ = d.DaemonDiscovery()
}

// SPEC 2 "Reads never write", SPEC 3.1 "New never creates anything", and
// features.md "Reads never create": New and every read method leave the disk
// exactly as they found it, in a project, outside one, with pointers, with
// no home folder, in dev and release builds, with the socket falling back.
func TestPromiseA_ReadsNeverWrite(t *testing.T) {
	w := paNewWorld(t)
	target := filepath.Join(w.root, "target")
	paWrite(t, filepath.Join(target, "home.jsonc"), `{ "app": "paapp", "layout": 1, "scope": "project" }`)
	paWrite(t, filepath.Join(w.marker, "config.local.jsonc"), `{ "home": "`+filepath.ToSlash(target)+`", "db": "`+filepath.ToSlash(filepath.Join(w.root, "dbs", "x.db"))+`" }`)
	paMkdir(t, filepath.Join(w.root, "dbs"))
	xdg := filepath.Join(w.root, "xdg")
	paMkdir(t, xdg)
	tempFallback := filepath.Join(os.TempDir(), paApp+"-"+currentUser())
	tempBefore := exists(tempFallback)

	variants := []struct {
		name string
		work string
		env  map[string]string
		opts []Option
	}{
		{"release in project", w.proj, nil, nil},
		{"dev in project", w.proj, nil, []Option{WithRelease(false)}},
		{"outside a project", w.out, nil, []Option{WithRelease(false)}},
		{"global mode", w.out, nil, []Option{WithMode(ModeGlobal)}},
		{"no home folder", w.out, nil, []Option{paNoHome(), WithRelease(false)}},
		{"env overrides", w.proj, map[string]string{paPrefix + "_CACHE_DIR": filepath.Join(w.root, "c"), paPrefix + "_DB_DIR": filepath.Join(w.root, "d")}, nil},
		{"home flag", w.proj, nil, []Option{WithHome(filepath.Join(w.root, "flaghome"))}},
		{"legacy configured", w.proj, nil, []Option{WithLegacyHome(LegacyHome{Dir: ".paold"})}},
	}
	if runtime.GOOS == "linux" {
		variants = append(variants, struct {
			name string
			work string
			env  map[string]string
			opts []Option
		}{"socket fallback", w.proj, map[string]string{"XDG_RUNTIME_DIR": xdg}, []Option{WithSocketMaxLen(10)}})
	}
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			before := paSnapshot(t, w.root)
			d := w.new(t, v.work, v.env, v.opts...)
			paReadAll(d, w)
			after := paSnapshot(t, w.root)
			if !reflect.DeepEqual(before, after) {
				for p, s := range after {
					if before[p] != s {
						t.Errorf("changed or created: %s (%s)", p, s)
					}
				}
				for p := range before {
					if _, ok := after[p]; !ok {
						t.Errorf("removed: %s", p)
					}
				}
			}
			if exists(tempFallback) != tempBefore {
				t.Errorf("reads changed the temp fallback folder %s", tempFallback)
			}
		})
	}
}

// SPEC 2 "Errors are deferred": New succeeds outside a project, with a broken
// pointer, a non-string pointer, a too-new layout and no home folder; the
// methods that need the home return the error, and a command that only needs
// the cache still runs outside a project. New fails for a bad option set and
// an unknown profile.
func TestPromiseA_ErrorsDeferred(t *testing.T) {
	t.Run("outside a project", func(t *testing.T) {
		w := paNewWorld(t)
		d := w.new(t, w.out, nil)
		if p, err := d.Path(KindCache); err != nil || p != filepath.Join(w.home, paMarker, "cache") {
			t.Errorf("Path(cache) = %q, %v", p, err)
		}
		if _, err := d.Path(KindDB); !errors.Is(err, ErrNoProject) {
			t.Errorf("Path(db) = %v, want ErrNoProject", err)
		}
	})
	broken := map[string]struct {
		file, body string
		want       error
	}{
		"missing pointer":    {"config.local.jsonc", `{ "home": "../nowhere" }`, ErrPointerMissing},
		"absolute pointer":   {"config.jsonc", `{ "db": "/abs/x.db" }`, ErrPointerAbsolute},
		"non-string pointer": {"config.jsonc", `{ "home": true }`, ErrInvalidOption},
	}
	for name, b := range broken {
		t.Run(name, func(t *testing.T) {
			w := paNewWorld(t)
			paWrite(t, filepath.Join(w.marker, b.file), b.body)
			d := w.new(t, w.proj, nil)
			if _, err := d.Project(); !errors.Is(err, b.want) {
				t.Errorf("Project() = %v, want %v", err, b.want)
			}
			if p, err := d.Path(KindCache); err != nil || p == "" {
				t.Errorf("Path(cache) = %q, %v; a global kind still works", p, err)
			}
		})
	}
	t.Run("layout too new", func(t *testing.T) {
		w := paNewWorld(t)
		paWrite(t, filepath.Join(w.marker, "home.jsonc"), `{ "app": "paapp", "layout": 9, "scope": "project" }`)
		d := w.new(t, w.proj, nil)
		if p, err := d.Path(KindDB); err != nil || p != filepath.Join(w.marker, paApp+".db") {
			t.Errorf("Path(db) = %q, %v; reads work on a too-new home", p, err)
		}
		if _, err := d.Ensure(KindDB); !errors.Is(err, ErrLayoutTooNew) {
			t.Errorf("Ensure(db) = %v", err)
		}
	})
	t.Run("no home folder", func(t *testing.T) {
		w := paNewWorld(t)
		d, err := New(paApp, append(w.opts(w.proj, nil), paNoHome())...)
		if err != nil {
			t.Fatalf("New = %v", err)
		}
		if _, err := d.PathIn(KindDB, ScopeGlobal); !errors.Is(err, ErrNoHome) {
			t.Errorf("PathIn(db, global) = %v", err)
		}
		if p, err := d.Path(KindDB); err != nil || p != filepath.Join(w.marker, paApp+".db") {
			t.Errorf("Path(db) = %q, %v", p, err)
		}
	})
	t.Run("New fails only for options and profiles", func(t *testing.T) {
		w := paNewWorld(t)
		if _, err := New(paApp, w.opts(w.proj, nil, WithMode("x"))...); !errors.Is(err, ErrInvalidOption) {
			t.Errorf("bad option: %v", err)
		}
		if _, err := New(paApp, w.opts(w.proj, nil, WithProfile("x"))...); !errors.Is(err, ErrUnknownProfile) {
			t.Errorf("unknown profile: %v", err)
		}
	})
}

// errors.md: "New checks the app name and the option set before it reads the
// home, the working folder or the environment, so a refused set has no side
// effects". The seams record any call.
func TestPromiseA_NewChecksOptionsFirst(t *testing.T) {
	called := ""
	spy := []Option{
		WithHomeDir(func() (string, error) { called += " home"; return "", errors.New("x") }),
		WithEnv(func(string) (string, bool) { called += " env"; return "", false }),
		WithBuildInfo(func() (*debug.BuildInfo, bool) { called += " buildinfo"; return nil, false }),
		WithConfigReader(func(string) (map[string]any, error) { called += " reader"; return nil, fs.ErrNotExist }),
	}
	bad := []struct {
		app string
		opt Option
	}{
		{"Bad", WithMode(ModeProject)},
		{paApp, WithDirName("..")},
		{paApp, WithKind("tpl", Folder("../x"))},
		{paApp, WithKind("x", Scoped("nope"))},
		{paApp, WithReleaseProfile("staging")},
		{paApp, WithLayoutVersion(0)},
		{paApp, WithSocketMaxLen(0)},
		{paApp, WithRunFile(RunPID, "a/b")},
	}
	for i, b := range bad {
		called = ""
		if _, err := New(b.app, append(append([]Option(nil), spy...), b.opt)...); !errors.Is(err, ErrInvalidOption) {
			t.Errorf("case %d: err = %v", i, err)
		}
		if called != "" {
			t.Errorf("case %d: New read%s before refusing the option set", i, called)
		}
	}
}

// SPEC 3.1 "*Dirs is immutable and safe for concurrent use" and options.md
// "methods that return slices return a fresh copy".
func TestPromiseA_DirsImmutableConcurrent(t *testing.T) {
	w := paNewWorld(t)
	d := w.new(t, w.proj, map[string]string{paPrefix + "_CEILING_DIRS": "rel"}, WithRelease(false))
	type view struct {
		Paths   []Entry
		Kinds   []KindSpec
		Config  []ConfigFile
		Warn    []Warning
		Global  Home
		Project Home
		Socket  Socket
	}
	look := func() view {
		p, _ := d.Project()
		s, _ := d.Socket()
		return view{d.Paths(), d.Kinds(), d.ConfigFiles(), d.Warnings(), d.Global(), p, s}
	}
	want := look()
	if len(want.Warn) == 0 || len(want.Paths) == 0 || len(want.Config) == 0 {
		t.Fatalf("fixture too thin: %+v", want)
	}
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if got := look(); !reflect.DeepEqual(got, want) {
					errs <- fmt.Sprintf("concurrent read differs: %+v", got)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	// Changing a returned slice does not change the Dirs.
	p, k, c, wr := d.Paths(), d.Kinds(), d.ConfigFiles(), d.Warnings()
	p[0].Path, k[0].Name, c[0].Path, wr[0].Message = "x", "x", "x", "x"
	if got := look(); !reflect.DeepEqual(got, want) {
		t.Errorf("mutating a returned slice changed the Dirs: %+v", got)
	}
}

// SPEC 3.3 option table: the default of each option, observed through the
// public API (no option given vs the documented default).
func TestPromiseA_OptionDefaults(t *testing.T) {
	w := paNewWorld(t)
	t.Run("WithDirName default .app", func(t *testing.T) {
		d := w.new(t, w.proj, nil)
		if g := d.Global(); g.Base != filepath.Join(w.home, paMarker) {
			t.Errorf("Global().Base = %s", g.Base)
		}
		if p, _ := d.Project(); p.Marker != w.marker {
			t.Errorf("Project().Marker = %s", p.Marker)
		}
	})
	t.Run("WithEnvPrefix default upper-cased, - and . to _", func(t *testing.T) {
		home := w.home
		d, err := New("my-app.x2", WithHomeDir(func() (string, error) { return home, nil }), WithWorkDir(w.proj),
			paEnv(map[string]string{"MY_APP_X2_CACHE_DIR": filepath.Join(w.root, "c")}), WithRelease(true))
		if err != nil {
			t.Fatal(err)
		}
		if d.EnvPrefix() != "MY_APP_X2" || d.KindEnv(KindCache) != "MY_APP_X2_CACHE_DIR" || d.EnvName("HOME") != "MY_APP_X2_HOME" {
			t.Errorf("prefix %q kind env %q", d.EnvPrefix(), d.KindEnv(KindCache))
		}
		if p, _ := d.Path(KindCache); p != filepath.Join(w.root, "c") {
			t.Errorf("Path(cache) = %s, want the MY_APP_X2_CACHE_DIR value", p)
		}
	})
	t.Run("WithMode default ModeProject", func(t *testing.T) {
		d := w.new(t, w.proj, nil)
		if d.Mode() != ModeProject {
			t.Errorf("Mode() = %s", d.Mode())
		}
		if p, _ := d.Path(KindDB); p != filepath.Join(w.marker, paApp+".db") {
			t.Errorf("Path(db) = %s, want the project home", p)
		}
	})
	t.Run("WithLookup default WalkUp", func(t *testing.T) {
		sub := filepath.Join(w.proj, "a", "b")
		paMkdir(t, sub)
		if p, err := w.new(t, sub, nil).Project(); err != nil || p.Marker != w.marker {
			t.Errorf("Project() from a subfolder = %+v, %v", p, err)
		}
	})
	t.Run("WithKind default the standard kinds", func(t *testing.T) {
		var names []Kind
		for _, k := range w.new(t, w.proj, nil).Kinds() {
			names = append(names, k.Name)
		}
		want := []Kind{KindHome, KindConfig, KindConfigLocal, KindGitignore, KindShared, KindDB, KindBackups, KindCache, KindRun, KindLogs}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("Kinds() = %v", names)
		}
	})
	t.Run("WithHome WithProfile WithDaemonProfile default unset", func(t *testing.T) {
		d := w.new(t, w.proj, nil, WithRelease(false))
		if d.Global().Rule != RuleGlobal || d.Profile() != ProfileDev || d.DaemonProfile() != ProfileDev {
			t.Errorf("rule %s profile %s daemon %s", d.Global().Rule, d.Profile(), d.DaemonProfile())
		}
	})
	t.Run("WithProfiles default prod and dev", func(t *testing.T) {
		for _, p := range []Profile{ProfileProd, ProfileDev} {
			if d := w.new(t, w.proj, nil, WithProfile(string(p))); d.Profile() != p {
				t.Errorf("Profile() = %s", d.Profile())
			}
		}
		if _, err := New(paApp, w.opts(w.proj, nil, WithProfile("staging"))...); !errors.Is(err, ErrUnknownProfile) {
			t.Errorf("staging: %v", err)
		}
	})
	t.Run("WithReleaseProfile prod keeps base names, WithDevProfile dev", func(t *testing.T) {
		rel := w.new(t, w.proj, nil)
		if rel.Profile() != ProfileProd || rel.Name("x") != "x" || rel.Global().Base != filepath.Join(w.home, paMarker) {
			t.Errorf("release: %s %s %s", rel.Profile(), rel.Name("x"), rel.Global().Base)
		}
		dev := w.new(t, w.proj, nil, WithRelease(false))
		if dev.Profile() != ProfileDev || dev.Name("x") != "x-dev" || dev.Global().Base != filepath.Join(w.home, paMarker+"-dev") {
			t.Errorf("dev: %s %s %s", dev.Profile(), dev.Name("x"), dev.Global().Base)
		}
	})
	t.Run("WithRelease default from build info", func(t *testing.T) {
		home := w.home
		base := []Option{WithHomeDir(func() (string, error) { return home, nil }), WithWorkDir(w.proj), paEnv(nil)}
		bi := func(v string) Option {
			return WithBuildInfo(func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{Main: debug.Module{Path: "example.com/paapp", Version: v}}, true
			})
		}
		d, err := New(paApp, append(base, bi("v1.2.3"))...)
		if err != nil || !d.IsRelease() || d.ReleaseReason() != "go-install" {
			t.Errorf("tagged: %v %v %q", err, d != nil && d.IsRelease(), d.ReleaseReason())
		}
		d, err = New(paApp, append(base, bi("(devel)"))...)
		if err != nil || d.IsRelease() || d.ReleaseReason() != "dev: (devel)" {
			t.Errorf("devel: %v %q", err, d.ReleaseReason())
		}
		if d, err := New(paApp, append(base, bi("v1.2.3"), WithRelease(false))...); err != nil || d.IsRelease() || d.ReleaseReason() != "dev: flag" {
			t.Errorf("flag over build info: %v", err)
		}
	})
	t.Run("WithAllowRoot default false", func(t *testing.T) {
		// Proven as root in TestPromiseA_RootOwned (promise_a_linux_test.go).
		if w.new(t, w.proj, nil).cfg.allowRoot {
			t.Error("allowRoot on by default")
		}
	})
	t.Run("WithLegacyHome default none", func(t *testing.T) {
		paMkdir(t, filepath.Join(w.home, ".paold"))
		res, err := w.new(t, w.proj, nil).MoveLegacy(context.Background())
		if err != nil || res != (MoveResult{Status: MoveNone}) {
			t.Errorf("MoveLegacy = %+v, %v", res, err)
		}
		if !isDir(filepath.Join(w.home, ".paold")) {
			t.Error("an unconfigured old folder was moved")
		}
	})
	t.Run("WithConfigReader default internal JSONC reader", func(t *testing.T) {
		g := filepath.Join(w.home, paMarker)
		paWrite(t, filepath.Join(g, "home.jsonc"), "// global home\n{ \"app\": \"paapp\", /* layout */ \"layout\": 1, \"scope\": \"global\", }\n")
		defer func() { _ = os.RemoveAll(g) }()
		info := w.new(t, w.proj, nil).Global().Info
		if info == nil || *info != (HomeInfo{App: paApp, Layout: 1, Scope: ScopeGlobal}) {
			t.Errorf("Global().Info = %+v", info)
		}
	})
	t.Run("WithLayoutVersion default 1", func(t *testing.T) {
		w := paNewWorld(t)
		h := filepath.Join(w.root, "h")
		if err := w.new(t, w.proj, nil).InitHome(h, ScopeProject); err != nil {
			t.Fatal(err)
		}
		d := w.new(t, w.proj, nil, WithHome(h))
		if info := d.Global().Info; info == nil || info.Layout != 1 {
			t.Errorf("InitHome wrote %+v", info)
		}
		if _, err := d.Ensure(KindDB); err != nil {
			t.Errorf("layout 1 refused: %v", err)
		}
		paWrite(t, filepath.Join(h, "home.jsonc"), `{ "app": "paapp", "layout": 2, "scope": "project" }`)
		if _, err := w.new(t, w.proj, nil, WithHome(h)).Ensure(KindLogs); !errors.Is(err, ErrLayoutTooNew) {
			t.Errorf("layout 2: %v", err)
		}
	})
	t.Run("WithTempFallback default on", func(t *testing.T) {
		d, err := New(paApp, append(w.opts(w.out, nil), paNoHome())...)
		if err != nil {
			t.Fatal(err)
		}
		if e, _ := d.entry(KindCache); e.Rule != RuleTemp || !e.Fallback || e.Path != filepath.Join(os.TempDir(), paApp+"-"+currentUser(), "cache") {
			t.Errorf("cache with no home = %+v", e)
		}
	})
	t.Run("WithCacheTag default on", func(t *testing.T) {
		w := paNewWorld(t)
		p, err := w.new(t, w.proj, nil).Ensure(KindCache)
		if err != nil || !exists(filepath.Join(p, cacheTagFile)) {
			t.Errorf("Ensure(cache) = %q, %v; no CACHEDIR.TAG", p, err)
		}
	})
	t.Run("WithTestGuard default on", func(t *testing.T) {
		outside := filepath.Join(string(filepath.Separator), "pa-not-a-test-root")
		if runtime.GOOS == "windows" {
			outside = filepath.VolumeName(w.root) + outside
		}
		msg := func() (m string) {
			defer func() { m = fmt.Sprint(recover()) }()
			_, _ = New(paApp, w.opts(w.proj, nil, WithHome(outside))...)
			return ""
		}()
		if !strings.Contains(msg, "outside the test root") {
			t.Errorf("New with a home outside the test root did not panic: %q", msg)
		}
	})
}

// SPEC 3.3 seam defaults: os.LookupEnv, os.UserHomeDir, os.Getwd. Under
// ForTest and with no seam option, New reads the process env, $HOME and the
// current folder. Also SPEC 4.1 / options.md: a set-but-empty variable is
// unset (ForTest sets PAAPP_HOME; emptying it removes the override).
func TestPromiseA_DefaultSeams(t *testing.T) {
	root := ForTest(t, paApp)
	t.Setenv(paPrefix+"_HOME", "")
	cache := filepath.Join(root, "mycache")
	t.Setenv(paPrefix+"_CACHE_DIR", cache)
	proj := filepath.Join(root, "proj")
	paMkdir(t, filepath.Join(proj, paMarker))
	paMkdir(t, filepath.Join(proj, gitEntry))
	t.Chdir(proj)
	d, err := New(paApp, WithRelease(true))
	if err != nil {
		t.Fatal(err)
	}
	if g := d.Global(); g.Base != filepath.Join(root, "home", paMarker) || g.Rule != RuleGlobal {
		t.Errorf("Global() = %+v, want $HOME/.paapp", g)
	}
	if p, err := d.Path(KindCache); err != nil || p != cache {
		t.Errorf("Path(cache) = %q, %v; want the process env value", p, err)
	}
	p, err := d.Project()
	if err != nil {
		t.Fatal(err)
	}
	if real, _ := filepath.EvalSymlinks(p.Marker); real != filepath.Join(proj, paMarker) {
		t.Errorf("Project().Marker = %s, want the marker in the current folder", p.Marker)
	}
	// debug.ReadBuildInfo of a test binary is not a tagged go install.
	if d, err := New(paApp); err != nil || d.IsRelease() || !strings.HasPrefix(d.ReleaseReason(), "dev: ") {
		t.Errorf("default build info: %v", err)
	}
}

// options.md: option order and list semantics, kind defaults, anchored kinds,
// and the home/env seam rules; SPEC 2 rule zero (D17): renamed names give no
// warning.
func TestPromiseA_OptionSemantics(t *testing.T) {
	w := paNewWorld(t)
	t.Run("a setter given twice keeps the last value", func(t *testing.T) {
		d := w.new(t, w.proj, nil, WithDirName(".a"), WithDirName(".b"))
		if d.Global().Base != filepath.Join(w.home, ".b") {
			t.Errorf("Global().Base = %s", d.Global().Base)
		}
	})
	t.Run("WithProfiles replaces the list", func(t *testing.T) {
		_, err := New(paApp, w.opts(w.proj, nil, WithProfiles("live", "beta"), WithReleaseProfile("live"), WithDevProfile("beta"), WithProfile("prod"))...)
		if !errors.Is(err, ErrUnknownProfile) {
			t.Errorf("prod after WithProfiles(live, beta): %v", err)
		}
	})
	t.Run("WithoutKind of an unknown name does nothing", func(t *testing.T) {
		if a, b := w.new(t, w.proj, nil).Kinds(), w.new(t, w.proj, nil, WithoutKind("nothing")).Kinds(); !reflect.DeepEqual(a, b) {
			t.Errorf("Kinds changed: %v", b)
		}
	})
	t.Run("kind edits apply in order", func(t *testing.T) {
		d := w.new(t, w.proj, nil, WithKind("tpl", Folder("a")), WithKind("tpl", Folder("b")))
		if p, _ := d.Path("tpl"); p != filepath.Join(w.marker, "b") {
			t.Errorf("Path(tpl) = %s", p)
		}
		d = w.new(t, w.proj, nil, WithKind("tpl"), WithoutKind("tpl"))
		if _, err := d.Path("tpl"); !errors.Is(err, ErrUnknownKind) {
			t.Errorf("dropped after add: %v", err)
		}
	})
	t.Run("a new kind starts as Folder(name), Ignore(), the data scope", func(t *testing.T) {
		for mode, scope := range map[Mode]Scope{ModeProject: ScopeProject, ModeGlobal: ScopeGlobal} {
			k := w.new(t, w.proj, nil, WithMode(mode), WithKind("tpl")).Kinds()
			got := k[len(k)-1]
			want := KindSpec{Name: "tpl", Path: "tpl", Type: TypeFolder, Git: GitIgnore, Scope: scope}
			if got != want {
				t.Errorf("%s: %+v, want %+v", mode, got, want)
			}
		}
	})
	t.Run("WithKind on an existing kind keeps its git rule and scope", func(t *testing.T) {
		for _, k := range w.new(t, w.proj, nil, WithKind(KindShared, Folder("data"))).Kinds() {
			if k.Name == KindShared && (k.Path != "data" || k.Git != GitCommit || k.Scope != ScopeProject) {
				t.Errorf("shared = %+v", k)
			}
		}
	})
	t.Run("Ignore turns a committed kind back to ignored", func(t *testing.T) {
		d := w.new(t, w.proj, nil, WithKind(KindShared, Ignore()))
		if strings.Contains(d.GitignoreBlock(), "!shared/") {
			t.Errorf("block still re-includes shared: %q", d.GitignoreBlock())
		}
	})
	t.Run("an anchored kind has no env variable", func(t *testing.T) {
		d := w.new(t, w.proj, map[string]string{paPrefix + "_CONFIG_DIR": filepath.Join(w.root, "x"), paPrefix + "_TPL_DIR": filepath.Join(w.root, "y")},
			WithKind("tpl", Anchored()))
		if p, _ := d.Path(KindConfig); p != filepath.Join(w.marker, "config.jsonc") || d.KindEnv(KindConfig) != "" {
			t.Errorf("Path(config) = %s", p)
		}
		if p, _ := d.Path("tpl"); p != filepath.Join(w.marker, "tpl") || d.KindEnv("tpl") != "" {
			t.Errorf("Path(tpl) = %s", p)
		}
	})
	t.Run("WithHomeDir error, empty or relative means no home folder", func(t *testing.T) {
		for _, h := range []string{"", "relative/home"} {
			h := h
			d, err := New(paApp, append(w.opts(w.out, nil), WithHomeDir(func() (string, error) { return h, nil }), WithMode(ModeGlobal))...)
			if err != nil {
				t.Fatal(err)
			}
			if g := d.Global(); g.Rule != RuleDefaults || !g.Fallback || g.Base != "" {
				t.Errorf("home %q: Global() = %+v", h, g)
			}
			if _, err := d.Path(KindDB); !errors.Is(err, ErrNoHome) {
				t.Errorf("home %q: Path(db) = %v", h, err)
			}
		}
	})
	t.Run("a set but empty variable counts as unset", func(t *testing.T) {
		d := w.new(t, w.proj, map[string]string{paPrefix + "_HOME": "", paPrefix + "_CACHE_DIR": "", paPrefix + "_PROFILE": ""}, WithRelease(false))
		if d.Global().Rule != RuleGlobal || d.Profile() != ProfileDev {
			t.Errorf("rule %s profile %s", d.Global().Rule, d.Profile())
		}
		if e, _ := d.entry(KindCache); e.Rule != RuleGlobal {
			t.Errorf("cache rule %s", e.Rule)
		}
	})
	t.Run("renamed names give no warning (D17)", func(t *testing.T) {
		d := w.new(t, w.proj, nil, WithDirName(".notes"), WithEnvPrefix("NT"), WithKind(KindShared, Folder("data")),
			WithKind(KindDB, File("store.db")), WithRunFile(RunPID, "d.pid"), WithProfiles("live", "beta"), WithReleaseProfile("live"), WithDevProfile("beta"))
		if ws := d.Warnings(); len(ws) != 0 {
			t.Errorf("Warnings() = %+v", ws)
		}
	})
}

// SPEC 3.2: the string values of the public constants are the API (they
// reach paths --json, home.jsonc and env values).
func TestPromiseA_ConstantValues(t *testing.T) {
	got := map[string]string{
		"home": string(KindHome), "config": string(KindConfig), "config-local": string(KindConfigLocal), "gitignore": string(KindGitignore),
		"shared": string(KindShared), "db": string(KindDB), "backups": string(KindBackups), "cache": string(KindCache), "run": string(KindRun), "logs": string(KindLogs),
		"global": string(ScopeGlobal), "project": string(ScopeProject), "project-else-global": string(ScopeProjectElseGlobal), "both": string(ScopeBoth),
		"mode project": string(ModeProject), "mode global": string(ModeGlobal), "walk-up": string(WalkUp), "here-only": string(HereOnly),
		"file": string(TypeFile), "folder": string(TypeFolder), "commit": string(GitCommit), "ignore": string(GitIgnore),
		"flag": string(RuleFlag), "env-home": string(RuleEnvHome), "env-kind": string(RuleEnvKind), "rule project": string(RuleProject),
		"worktree": string(RuleWorktree), "pointer": string(RulePointer), "rule global": string(RuleGlobal), "temp": string(RuleTemp), "defaults": string(RuleDefaults),
		"prod": string(ProfileProd), "dev": string(ProfileDev),
		"lock": string(RunLock), "daemon-lock": string(RunDaemonLock), "pid": string(RunPID), "discovery": string(RunDiscovery), "socket": string(RunSocket), "socket-file": string(RunSocketFile),
		"none": string(MoveNone), "moved": string(MoveDone), "both-exist": string(MoveBothExist), "skipped": string(MoveSkipped), "already": string(MoveAlready),
		"profile-env-ignored": string(WarnProfileEnvIgnored), "env-points-to-prod": string(WarnEnvPointsToProd), "pointer-chain-ignored": string(WarnPointerChain),
		"pointer-ignored-for-profile": string(WarnPointerProfile), "ceiling-relative": string(WarnCeilingRelative), "ceiling-dropped": string(WarnCeilingDropped),
		"worktree-main-missing": string(WarnWorktreeMainMissing),
	}
	for k, v := range got {
		want := k
		if i := strings.LastIndex(k, " "); i >= 0 {
			want = k[i+1:]
		}
		if v != want {
			t.Errorf("%s = %q, want %q", k, v, want)
		}
	}
	if AccessReadWrite != 0 || AccessReadOnly != 1 {
		t.Errorf("DBAccess values %d %d", AccessReadWrite, AccessReadOnly)
	}
}

// paKeys marshals v and returns its sorted JSON keys.
func paKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SPEC 3.4 result types and paths-json.md "Result types": the JSON names are
// the documented ones and omitempty fields are dropped when empty.
func TestPromiseA_JSONTags(t *testing.T) {
	cases := []struct {
		name       string
		full, zero any
		fullKeys   string
		zeroKeys   string
	}{
		{"Home", Home{Marker: "m", Note: "n", Info: &HomeInfo{}}, Home{},
			"base data fallback info marker note rule scope", "base data fallback rule scope"},
		{"HomeInfo", HomeInfo{}, HomeInfo{}, "app layout scope", "app layout scope"},
		{"Entry", Entry{Note: "n", Error: "e"}, Entry{},
			"error fallback git home kind note path rule type", "fallback git home kind path rule type"},
		{"ConfigFile", ConfigFile{Profile: "dev"}, ConfigFile{}, "committed home local path profile", "committed home local path"},
		{"Socket", Socket{}, Socket{}, "dir fallback path pointer_file", "dir fallback path pointer_file"},
		{"Warning", Warning{}, Warning{}, "code message", "code message"},
		{"MoveResult", MoveResult{From: "f", To: "t", Note: "n"}, MoveResult{}, "from note status to", "status"},
		{"KindSpec", KindSpec{}, KindSpec{}, "anchored disposable git name path scope type", "anchored disposable git name path scope type"},
	}
	for _, c := range cases {
		if got := strings.Join(paKeys(t, c.full), " "); got != c.fullKeys {
			t.Errorf("%s keys = %q, want %q", c.name, got, c.fullKeys)
		}
		if got := strings.Join(paKeys(t, c.zero), " "); got != c.zeroKeys {
			t.Errorf("%s empty keys = %q, want %q", c.name, got, c.zeroKeys)
		}
	}
}

// SPEC 3.4 method contracts not pinned elsewhere, and paths-json.md: Paths
// lists kinds in layout order with both kinds global first then project;
// ConfigFiles lists files that do not exist; Path of a both kind is the
// project copy in a project, else the global one; Kinds is stable.
func TestPromiseA_Methods(t *testing.T) {
	w := paNewWorld(t)
	d := w.new(t, w.proj, nil)
	if d.App() != paApp || d.EnvName("X") != paPrefix+"_X" || !d.HasProject() {
		t.Errorf("App %q EnvName %q HasProject %v", d.App(), d.EnvName("X"), d.HasProject())
	}
	if !reflect.DeepEqual(d.Kinds(), d.Kinds()) {
		t.Error("Kinds() not stable")
	}
	var order []string
	for _, e := range d.Paths() {
		order = append(order, string(e.Kind)+"@"+string(e.Home))
	}
	want := "home@global home@project config@global config@project config-local@global config-local@project gitignore@project shared@project db@project backups@project cache@global run@project logs@project"
	if strings.Join(order, " ") != want {
		t.Errorf("Paths order = %v", order)
	}
	files := d.ConfigFiles()
	if len(files) != 4 {
		t.Fatalf("ConfigFiles = %+v", files)
	}
	for _, f := range files {
		if exists(f.Path) {
			t.Errorf("fixture file %s exists", f.Path)
		}
	}
	if p, _ := d.Path(KindConfig); p != filepath.Join(w.marker, "config.jsonc") {
		t.Errorf("Path(config) in a project = %s", p)
	}
	if p, _ := d.PathIn(KindConfig, ScopeGlobal); p != filepath.Join(w.home, paMarker, "config.jsonc") {
		t.Errorf("PathIn(config, global) = %s", p)
	}
	out := w.new(t, w.out, nil)
	if out.HasProject() {
		t.Error("HasProject outside a project")
	}
	if p, _ := out.Path(KindConfig); p != filepath.Join(w.home, paMarker, "config.jsonc") {
		t.Errorf("Path(config) outside a project = %s", p)
	}
	if b := d.Banner(); b != "" {
		t.Errorf("release Banner = %q", b)
	}
	if !strings.HasPrefix(w.new(t, w.proj, nil, WithRelease(false)).Banner(), "[paapp] profile: dev → ") {
		t.Error("dev Banner shape")
	}
	if d.NameWith("com.org.app", ".") != "com.org.app" || w.new(t, w.proj, nil, WithRelease(false)).NameWith("com.org.app", ".") != "com.org.app.dev" {
		t.Error("NameWith")
	}
	if p, err := d.InitProject(w.out); err != nil || p != filepath.Join(w.out, paMarker) || !exists(filepath.Join(p, "home.jsonc")) {
		t.Errorf("InitProject = %q, %v", p, err)
	}
	if ok, err := out.WriteGitignore(); !errors.Is(err, ErrNoProject) || ok {
		t.Errorf("WriteGitignore without a project = %v, %v", ok, err)
	}
	if ok, err := w.new(t, w.out, nil).WriteGitignore(); ok || err != nil {
		t.Errorf("WriteGitignore outside git = %v, %v; want false, nil", ok, err)
	}
}

// TestPromiseA_TempDirThroughSeam proves SPEC 2 ("every outside input goes
// through a seam") and SPEC 4.7 rule 4 for the temp folder: with no home
// folder, a disposable kind falls back under the TMPDIR (TMP on Windows)
// that WithEnv gives, not the process's own temp folder.
func TestPromiseA_TempDirThroughSeam(t *testing.T) {
	root := trRoot(t)
	tmp := filepath.Join(root, "seam-tmp")
	d := trNew(t, trNoHome(), root, map[string]string{tempEnvNames()[0]: tmp}, WithMode(ModeGlobal))
	p, err := d.Path(KindCache)
	if err != nil {
		t.Fatal(err)
	}
	if !lexicallyWithin(p, tmp) {
		t.Fatalf("Path(cache) = %s, want under the seam's temp folder %s", p, tmp)
	}
	// A relative value is not a temp folder: the OS default is used.
	d = trNew(t, trNoHome(), root, map[string]string{tempEnvNames()[0]: "rel"}, WithMode(ModeGlobal))
	if p, _ := d.Path(KindCache); !lexicallyWithin(p, os.TempDir()) {
		t.Fatalf("Path(cache) with a relative temp variable = %s, want under %s", p, os.TempDir())
	}
}

// TestPromiseA_OSErrorsWrapCause proves the second half of SPEC 3.5: an
// error from the OS or a parser wraps no sentinel but names the path and
// keeps the underlying error. A config.jsonc that does not parse fails the
// project home with the file named; a file sitting where the home folder
// goes fails Ensure with that path named and the OS error wrapped.
func TestPromiseA_OSErrorsWrapCause(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	proj := filepath.Join(root, "proj")
	cfg := filepath.Join(proj, trMarker, "config.jsonc")
	trInitHome(t, filepath.Join(proj, trMarker))
	trWrite(t, cfg, "{")
	_, err := trNew(t, trHome(home), proj, nil).Project()
	if err == nil || !strings.Contains(err.Error(), cfg) {
		t.Fatalf("Project() with a broken config.jsonc = %v, want an error naming %s", err, cfg)
	}

	trWrite(t, filepath.Join(home, trMarker), "a file, not a folder")
	_, err = trNew(t, trHome(home), root, nil, WithMode(ModeGlobal)).Ensure(KindLogs)
	if err == nil || !strings.Contains(err.Error(), filepath.Join(home, trMarker)) {
		t.Fatalf("Ensure(logs) under a file = %v, want an error naming the home", err)
	}
	if errors.Unwrap(err) == nil {
		t.Fatalf("Ensure error %v wraps nothing; want the OS error kept", err)
	}
}
