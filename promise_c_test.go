package appdir

// promise_c_test.go: promises of SPEC sections 5 (layout), 6 (config files,
// config reader seam) and 7 (run paths), and of the guides under
// docs/layout, docs/runtime/run-files-and-sockets.md and docs/choosing, that
// no other test proved. Each test names the section or page it proves. It
// reuses the tl fixture of layout_test.go; every new helper starts with "pc".

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// TestPCReservedEnvNamesNeverProduced: SPEC 5.3. No kind, whatever its
// name, produces one of the reserved names <APP>_HOME, <APP>_PROFILE,
// <APP>_ALLOW_PROD, <APP>_CEILING_DIRS, <APP>_DAEMON_PROFILE: a kind named
// like one of them only gets the same name plus _DIR.
func TestPCReservedEnvNamesNeverProduced(t *testing.T) {
	fx := tlNewFix(t)
	names := []Kind{"profile", "allow-prod", "ceiling-dirs", "daemon-profile", "home-x"}
	opts := []Option{}
	for _, n := range names {
		opts = append(opts, WithKind(n))
	}
	d := fx.tlNew(t, opts...)
	reserved := map[string]bool{}
	for _, s := range []string{"HOME", "PROFILE", "ALLOW_PROD", "CEILING_DIRS", "DAEMON_PROFILE"} {
		reserved[tlPrefix+"_"+s] = true
	}
	for _, k := range d.Kinds() {
		if env := d.KindEnv(k.Name); reserved[env] {
			t.Errorf("KindEnv(%s) = %s, a reserved name", k.Name, env)
		}
	}
	if got := d.KindEnv("profile"); got != tlPrefix+"_PROFILE_DIR" {
		t.Errorf("KindEnv(profile) = %q; want %s_PROFILE_DIR", got, tlPrefix)
	}
	if got := d.KindEnv(KindHome); got != "" {
		t.Errorf("KindEnv(home) = %q; an anchored kind has none", got)
	}
}

// TestPCAuthorKindEnvMovesIt: SPEC 5.3 example. An author kind gets its
// <APP>_<KIND>_DIR with no more code: TLAPP_TEMPLATES_DIR moves templates,
// and it is listed by Paths().
func TestPCAuthorKindEnvMovesIt(t *testing.T) {
	fx := tlNewFix(t)
	m := fx.tlProjectMarker(t)
	moved := filepath.Join(fx.root, "my-templates")
	plain := fx.tlNew(t, WithKind("templates"))
	if p, err := plain.Path("templates"); err != nil || p != filepath.Join(m, "templates") {
		t.Fatalf("Path(templates) = %q, %v", p, err)
	}
	listed := false
	for _, e := range plain.Paths() {
		listed = listed || e.Kind == "templates"
	}
	if !listed {
		t.Error("Paths() does not list the author kind")
	}
	d := fx.tlNew(t, WithKind("templates"), WithEnv(tlEnv(map[string]string{tlPrefix + "_TEMPLATES_DIR": moved})))
	if p, err := d.Path("templates"); err != nil || p != moved {
		t.Fatalf("Path(templates) with %s_TEMPLATES_DIR = %q, %v; want %s", tlPrefix, p, err, moved)
	}
}

// TestPCScopedProjectUnderGlobalMode: docs/choosing/mode-and-scopes.md. A
// Scoped(ScopeProject) kind in a ModeGlobal tool still needs a project:
// ErrNoProject outside one, the project path inside one.
func TestPCScopedProjectUnderGlobalMode(t *testing.T) {
	fx := tlNewFix(t)
	opts := []Option{WithMode(ModeGlobal), WithKind("index", Scoped(ScopeProject))}
	if _, err := fx.tlNew(t, opts...).Path("index"); !errors.Is(err, ErrNoProject) {
		t.Fatalf("Path(index) outside a project err = %v; want ErrNoProject", err)
	}
	m := fx.tlProjectMarker(t)
	if p, err := fx.tlNew(t, opts...).Path("index"); err != nil || p != filepath.Join(m, "index") {
		t.Fatalf("Path(index) in a project = %q, %v", p, err)
	}
}

// TestPCGlobalRunKindInProjectMode: docs/choosing/by-app-type.md and
// mode-and-scopes.md. A project CLI with one machine-wide daemon keeps
// ModeProject and makes run global: RunPath and Socket then work outside a
// project, in the global home. Without it they fail with ErrNoProject.
func TestPCGlobalRunKindInProjectMode(t *testing.T) {
	fx := tlNewFix(t)
	if _, err := fx.tlNew(t).RunPath(RunDiscovery); !errors.Is(err, ErrNoProject) {
		t.Fatalf("RunPath in ModeProject outside a project err = %v; want ErrNoProject", err)
	}
	d := fx.tlNew(t, WithKind(KindRun, Scoped(ScopeGlobal)), WithSocketMaxLen(4096))
	p, err := d.RunPath(RunDiscovery)
	if err != nil || p != filepath.Join(fx.tlGlobal(), "run", "discovery.json") {
		t.Fatalf("RunPath = %q, %v", p, err)
	}
	if _, err := d.Socket(); err != nil {
		t.Fatalf("Socket: %v", err)
	}
}

// TestPCAnchoredAuthorKindStaysInBase: docs/layout/kinds.md and
// mode-and-scopes.md. Anchored() keeps a kind in the home root in every
// profile, while a plain data kind moves to .<app>/profiles/<profile>/.
func TestPCAnchoredAuthorKindStaysInBase(t *testing.T) {
	fx := tlNewFix(t)
	m := fx.tlProjectMarker(t)
	d := fx.tlNew(t, WithRelease(false), WithKind("schema", File("schema.jsonc"), Anchored(), Commit()), WithKind("notes"))
	if p, err := d.Path("schema"); err != nil || p != filepath.Join(m, "schema.jsonc") {
		t.Errorf("Path(schema) in dev = %q, %v; want it in %s", p, err, m)
	}
	if p, err := d.Path("notes"); err != nil || p != filepath.Join(m, profilesDir, string(ProfileDev), "notes") {
		t.Errorf("Path(notes) in dev = %q, %v; want it under profiles/dev", p, err)
	}
}

// TestPCDroppedKindIsUnknownEverywhere: docs/layout/kinds.md Step 5 and
// docs/choosing/mode-and-scopes.md. After WithoutKind, Path, PathIn and
// Ensure return ErrUnknownKind with the message appdir: unknown kind "<name>",
// and Kinds/Paths leave it out.
func TestPCDroppedKindIsUnknownEverywhere(t *testing.T) {
	fx := tlNewFix(t)
	fx.tlProjectMarker(t)
	d := fx.tlNew(t, WithoutKind(KindBackups))
	want := `appdir: unknown kind "backups"`
	_, err1 := d.Path(KindBackups)
	_, err2 := d.PathIn(KindBackups, ScopeProject)
	_, err3 := d.Ensure(KindBackups)
	for i, err := range []error{err1, err2, err3} {
		if !errors.Is(err, ErrUnknownKind) || err.Error() != want {
			t.Errorf("call %d err = %v; want ErrUnknownKind %q", i, err, want)
		}
	}
	for _, k := range d.Kinds() {
		if k.Name == KindBackups {
			t.Error("Kinds() still lists backups")
		}
	}
	for _, e := range d.Paths() {
		if e.Kind == KindBackups {
			t.Error("Paths() still lists backups")
		}
	}
}

// TestPCWriteGitignoreAboveHome: docs/layout/gitignore.md. The work-tree
// check walks up past $HOME: a project in the user's home under a .git
// above the home folder counts as inside a git work tree.
func TestPCWriteGitignoreAboveHome(t *testing.T) {
	fx := tlNewFix(t)
	tlMkdir(t, filepath.Join(fx.root, ".git"))
	proj := filepath.Join(fx.home, "proj")
	m := filepath.Join(proj, tlMarker)
	tlMkdir(t, m)
	d := fx.tlNew(t, WithWorkDir(proj))
	ok, err := d.WriteGitignore()
	if !ok || err != nil {
		t.Fatalf("WriteGitignore = %v, %v; want true, nil", ok, err)
	}
	if got := tlRead(t, filepath.Join(m, ".gitignore")); got != tlStdBlock {
		t.Fatalf(".gitignore = %q", got)
	}
}

// TestPCWriteGitignoreMarkerLines: docs/layout/gitignore.md. Markers match
// whole lines only: indented markers count as missing (the block is
// appended when both are), a lone real marker is ErrGitignoreMarkers, and
// that error reads "<path>: # <app>:begin / # <app>:end missing, doubled or
// out of order; fix it by hand".
func TestPCWriteGitignoreMarkerLines(t *testing.T) {
	t.Run("both indented: appended", func(t *testing.T) {
		fx := tlNewFix(t)
		gi := fx.tlGitProject(t)
		old := "  # tlapp:begin\nx\n  # tlapp:end\n"
		tlWrite(t, gi, old, 0o600)
		if ok, err := fx.tlNew(t).WriteGitignore(); !ok || err != nil {
			t.Fatalf("WriteGitignore = %v, %v", ok, err)
		}
		if got := tlRead(t, gi); got != old+tlStdBlock {
			t.Fatalf(".gitignore = %q", got)
		}
	})
	t.Run("edited end marker: error with the documented message", func(t *testing.T) {
		fx := tlNewFix(t)
		gi := fx.tlGitProject(t)
		old := "# tlapp:begin\nx\n# tlapp:end (edited)\n"
		tlWrite(t, gi, old, 0o600)
		_, err := fx.tlNew(t).WriteGitignore()
		want := gi + ": # tlapp:begin / # tlapp:end missing, doubled or out of order; fix it by hand"
		if !errors.Is(err, ErrGitignoreMarkers) || err.Error() != want {
			t.Fatalf("err = %v; want %q", err, want)
		}
		if tlRead(t, gi) != old {
			t.Fatal("file changed")
		}
	})
}

// TestPCWriteGitignoreIntoBase: SPEC 5.7. WriteGitignore writes into the
// project home's Base/.gitignore, also in a non-release profile whose data
// lives under profiles/<profile>/.
func TestPCWriteGitignoreIntoBase(t *testing.T) {
	fx := tlNewFix(t)
	gi := fx.tlGitProject(t)
	ok, err := fx.tlNew(t, WithRelease(false)).WriteGitignore()
	if !ok || err != nil || tlRead(t, gi) != tlStdBlock {
		t.Fatalf("WriteGitignore = %v, %v", ok, err)
	}
	if tlExists(filepath.Join(filepath.Dir(gi), profilesDir, string(ProfileDev), ".gitignore")) {
		t.Fatal(".gitignore written into the profile's data folder")
	}
}

// TestPCGitCommitQuestionMarkStopsParents: docs/layout/gitignore.md. The
// fixed prefix of a WithGitCommit pattern ends at the first *, ? or [; a ?
// segment stops the parent re-includes like * does.
func TestPCGitCommitQuestionMarkStopsParents(t *testing.T) {
	fx := tlNewFix(t)
	got := fx.tlNew(t, WithGitCommit("a/b?/c.md")).GitignoreBlock()
	want := strings.Replace(tlStdBlock, "# tlapp:end", "!a/\n!a/b?/c.md\n# tlapp:end", 1)
	if got != want {
		t.Fatalf("block:\n%s\nwant:\n%s", got, want)
	}
}

// TestPCCacheTagOffKeepsOldTag: docs/layout/files-and-permissions.md.
// WithCacheTag(false) writes no tag and never removes one an earlier run
// wrote.
func TestPCCacheTagOffKeepsOldTag(t *testing.T) {
	fx := tlNewFix(t)
	p, err := fx.tlNew(t).Ensure(KindCache)
	if err != nil {
		t.Fatal(err)
	}
	tag := filepath.Join(p, cacheTagFile)
	if _, err := fx.tlNew(t, WithCacheTag(false)).Ensure(KindCache); err != nil {
		t.Fatal(err)
	}
	if tlRead(t, tag) != tlCacheTagBody {
		t.Fatal("WithCacheTag(false) removed or changed the existing tag")
	}
}

// TestPCInitProjectNestedHomeKind: SPEC 5.8. InitProject creates the parent
// folders of a home kind renamed to a nested path (meta/home.json), and a
// later New reads it as Home.Info.
func TestPCInitProjectNestedHomeKind(t *testing.T) {
	fx := tlNewFix(t)
	opt := WithKind(KindHome, File("meta/home.json"))
	m, err := fx.tlNew(t, opt).InitProject(fx.work)
	if err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	if !tlExists(filepath.Join(m, "meta", "home.json")) {
		t.Fatal("meta/home.json missing")
	}
	h, err := fx.tlNew(t, opt).Project()
	if err != nil || h.Info == nil || h.Info.App != tlApp || h.Info.Layout != 1 || h.Info.Scope != ScopeProject {
		t.Fatalf("Project() = %+v, %v", h, err)
	}
}

// TestPCHomeInfoOfGlobalHome: SPEC 5.8. New reads the global home's
// home.jsonc when present and exposes it as Home.Info; with none, Info is nil.
func TestPCHomeInfoOfGlobalHome(t *testing.T) {
	fx := tlNewFix(t)
	if g := fx.tlNew(t).Global(); g.Info != nil {
		t.Fatalf("Global().Info = %+v before any home.jsonc", g.Info)
	}
	if err := fx.tlNew(t).InitHome(fx.tlGlobal(), ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	g := fx.tlNew(t).Global()
	if g.Info == nil || g.Info.App != tlApp || g.Info.Layout != 1 || g.Info.Scope != ScopeGlobal {
		t.Fatalf("Global().Info = %+v", g.Info)
	}
}

// pcRecorder is a ConfigReader that records every path it is asked for and
// answers from a fixed map; any other path is fs.ErrNotExist.
type pcRecorder struct {
	mu    sync.Mutex
	seen  []string
	files map[string]map[string]any
	plain bool // answer a missing file with a plain error, not fs.ErrNotExist
}

func (r *pcRecorder) read(p string) (map[string]any, error) {
	r.mu.Lock()
	r.seen = append(r.seen, p)
	r.mu.Unlock()
	if m, ok := r.files[p]; ok {
		return m, nil
	}
	if r.plain {
		return nil, errors.New("no such thing")
	}
	return nil, fs.ErrNotExist
}

// TestPCConfigReaderSeam: SPEC 6.3 and docs/layout/config-files.md Step 5.
// WithConfigReader is what appdir reads home.jsonc and the pointer keys
// with: a home: key that only the reader returns (the file on disk is empty)
// is followed, the target is accepted from the reader's home.jsonc, and
// appdir asks the reader for nothing but home.jsonc, config.jsonc and
// config.local.jsonc.
func TestPCConfigReaderSeam(t *testing.T) {
	fx := tlNewFix(t)
	m := fx.tlProjectMarker(t)
	target := filepath.Join(fx.root, "private")
	tlMkdir(t, target) // no home.jsonc on disk: only the reader has one
	r := &pcRecorder{files: map[string]map[string]any{
		filepath.Join(m, "config.jsonc"):           {"home": filepath.ToSlash(filepath.Join("..", "..", "private"))},
		filepath.Join(target, "home.jsonc"):        {"app": tlApp, "layout": 1.0, "scope": "project"},
		filepath.Join(fx.tlGlobal(), "home.jsonc"): {"app": tlApp, "layout": 1.0, "scope": "global"},
	}}
	d := fx.tlNew(t, WithConfigReader(r.read))
	p, err := d.Project()
	if err != nil || p.Base != target || p.Rule != RulePointer {
		t.Fatalf("Project() = %+v, %v; want the reader's home: pointer followed", p, err)
	}
	if p.Info == nil || p.Info.App != tlApp {
		t.Errorf("Project().Info = %+v; want the reader's home.jsonc", p.Info)
	}
	if g := d.Global(); g.Info == nil || g.Info.Scope != ScopeGlobal {
		t.Errorf("Global().Info = %+v; want the reader's home.jsonc", g.Info)
	}
	allowed := map[string]bool{"home.jsonc": true, "config.jsonc": true, "config.local.jsonc": true}
	for _, s := range r.seen {
		if !allowed[filepath.Base(s)] {
			t.Errorf("reader asked for %s", s)
		}
	}
	if len(r.seen) == 0 {
		t.Fatal("the reader was never called")
	}
}

// TestPCConfigReaderPlainMissingError: docs/layout/config-files.md
// pitfalls. A reader that answers a missing file with a plain error (not
// wrapping fs.ErrNotExist) makes the project fail with "read <file>: ...",
// while a missing home.jsonc is taken as no home info.
func TestPCConfigReaderPlainMissingError(t *testing.T) {
	fx := tlNewFix(t)
	m := fx.tlProjectMarker(t)
	r := &pcRecorder{plain: true, files: map[string]map[string]any{}}
	d, err := New(tlApp, fx.tlOpts(WithConfigReader(r.read))...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if g := d.Global(); g.Info != nil {
		t.Errorf("Global().Info = %+v; want nil for an unreadable home.jsonc", g.Info)
	}
	_, perr := d.Project()
	if perr == nil || !strings.Contains(perr.Error(), "read "+filepath.Join(m, "config.jsonc")) {
		t.Fatalf("Project() err = %v; want read %s", perr, filepath.Join(m, "config.jsonc"))
	}
}

// TestPCDefaultReaderJSONC: SPEC 6.3 through appdir itself. The default
// reader accepts // and /* */ comments, trailing commas, // inside a string
// and escaped quotes, both for home.jsonc and for a pointer key; it refuses
// an unterminated comment or string, a backslash before a raw newline in a
// string, and a top level that is not an object (null included), which
// InitHome reports as ErrBadHomeFile and leaves untouched.
func TestPCDefaultReaderJSONC(t *testing.T) {
	good := []string{
		"// made by hand\n{\n  \"app\": \"tlapp\", /* the app */\n  \"layout\": 1,\n  \"scope\": \"global\",\n}\n",
		`{"app": "tlapp", "note": "http://x // not a comment", "q": "say \"hi\" /* no */", "layout": 1,}`,
	}
	for i, body := range good {
		fx := tlNewFix(t)
		h := filepath.Join(fx.root, "h")
		tlWrite(t, filepath.Join(h, "home.jsonc"), body, 0o600)
		if err := fx.tlNew(t).InitHome(h, ScopeGlobal); err != nil {
			t.Errorf("good %d: InitHome: %v", i, err)
		}
		if tlRead(t, filepath.Join(h, "home.jsonc")) != body {
			t.Errorf("good %d: home.jsonc rewritten", i)
		}
		d := fx.tlNew(t, WithHome(h))
		if d.Global().Info == nil || d.Global().Info.App != tlApp {
			t.Errorf("good %d: Info = %+v", i, d.Global().Info)
		}
	}
	bad := map[string]string{
		"unterminated comment":       `{"app": "tlapp" /* open`,
		"unterminated string":        `{"app": "tlapp`,
		"backslash before a newline": "{\"app\": \"tl\\\napp\"}",
		"null top level":             "null",
		"array top level":            `[{"app": "tlapp"}]`,
	}
	for name, body := range bad {
		t.Run(name, func(t *testing.T) {
			fx := tlNewFix(t)
			h := filepath.Join(fx.root, "h")
			tlWrite(t, filepath.Join(h, "home.jsonc"), body, 0o600)
			err := fx.tlNew(t).InitHome(h, ScopeGlobal)
			if !errors.Is(err, ErrBadHomeFile) {
				t.Fatalf("InitHome err = %v; want ErrBadHomeFile", err)
			}
			if tlRead(t, filepath.Join(h, "home.jsonc")) != body {
				t.Fatal("home.jsonc changed")
			}
		})
	}
	t.Run("pointer key in a commented file", func(t *testing.T) {
		fx := tlNewFix(t)
		m := fx.tlProjectMarker(t)
		target := filepath.Join(fx.root, "t")
		if err := fx.tlNew(t).InitHome(target, ScopeProject); err != nil {
			t.Fatal(err)
		}
		tlWrite(t, filepath.Join(m, "config.jsonc"), "{\n  // moved\n  \"home\": \"../../t\", /* team */\n}\n", 0o600)
		if p, err := fx.tlNew(t).Project(); err != nil || p.Base != target {
			t.Fatalf("Project() = %+v, %v; want %s", p, err, target)
		}
	})
}

// TestPCSocketDefaultMaxLen: SPEC 7 and docs/runtime/run-files-and-sockets.md.
// WithSocketMaxLen defaults to 100 bytes: a run/<app>.sock of exactly 100
// bytes stays under run/, one of 101 falls back. The behaviour part runs on
// Linux, where the fallback goes to an XDG_RUNTIME_DIR inside the test root.
func TestPCSocketDefaultMaxLen(t *testing.T) {
	fx := tlNewFix(t)
	if d := fx.tlNew(t); d.cfg.socketMaxLen != 100 {
		t.Fatalf("default socket max len = %d; want 100", d.cfg.socketMaxLen)
	}
	if runtime.GOOS != "linux" {
		return // the fallback elsewhere is the real /tmp or a pipe (darwin and windows tests)
	}
	xdg := filepath.Join(fx.root, "x")
	tlMkdir(t, xdg)
	tail := len(string(filepath.Separator) + filepath.Join(tlMarker, "run", tlApp+".sock"))
	for _, n := range []int{100, 101} {
		pad := n - tail - len(fx.root) - 1
		if pad < 1 {
			t.Fatalf("temp root %s is too long to build a %d-byte socket path", fx.root, n)
		}
		home := filepath.Join(fx.root, strings.Repeat("h", pad))
		tlMkdir(t, home)
		d, err := New(tlApp, WithHomeDir(func() (string, error) { return home, nil }), WithWorkDir(home),
			WithEnv(tlEnv(map[string]string{"XDG_RUNTIME_DIR": xdg})), WithRelease(true), WithMode(ModeGlobal))
		if err != nil {
			t.Fatal(err)
		}
		s, err := d.Socket()
		if err != nil {
			t.Fatal(err)
		}
		inRun := filepath.Join(home, tlMarker, "run", tlApp+".sock")
		if len(inRun) != n {
			t.Fatalf("built a %d-byte path, want %d", len(inRun), n)
		}
		if n == 100 && (s.Fallback || s.Path != inRun) {
			t.Errorf("100 bytes: Socket = %+v; want %s", s, inRun)
		}
		if n == 101 && (!s.Fallback || filepath.Dir(s.Path) != filepath.Join(xdg, tlApp)) {
			t.Errorf("101 bytes: Socket = %+v; want the fallback", s)
		}
	}
}

// TestPCRunFilesNotCreated: docs/runtime/run-files-and-sockets.md. appdir
// creates none of the run files: RunPath, Socket and DaemonDiscovery
// (SPEC 7: appdir only gives the path) create nothing, and
// EnsureSocketDir leaves run/ empty (no lock, pid, discovery or socket file).
func TestPCRunFilesNotCreated(t *testing.T) {
	fx := tlNewFix(t)
	d := fx.tlNew(t, WithMode(ModeGlobal), WithSocketMaxLen(4096))
	for _, f := range []RunFile{RunLock, RunDaemonLock, RunPID, RunDiscovery, RunSocket, RunSocketFile} {
		if _, err := d.RunPath(f); err != nil {
			t.Fatalf("RunPath(%s): %v", f, err)
		}
	}
	if _, err := d.Socket(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.DaemonDiscovery(); err != nil {
		t.Fatal(err)
	}
	if tlExists(fx.tlGlobal()) {
		t.Fatal("RunPath/Socket/DaemonDiscovery created the home")
	}
	if _, err := d.EnsureSocketDir(); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(filepath.Join(fx.tlGlobal(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		t.Fatalf("EnsureSocketDir created run files: %v", names)
	}
}

// TestPCNoLinkCreation: SPEC 5.6 "appdir never creates links". No
// production source file calls a link-creating API, except the one known
// spot: the cross-disk legacy copy in legacy.go recreates a symlink the
// user already had in the old home (a copy of user data, not a link of
// appdir's own). That one call is allowed exactly once; any other fails.
func TestPCNoLinkCreation(t *testing.T) {
	allowed := map[string]int{"legacy.go os.Symlink(": 1}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	more, _ := filepath.Glob(filepath.Join("internal", "*", "*.go"))
	files = append(files, more...)
	bad := []string{"os.Symlink(", "os.Link(", "syscall.Symlink(", "syscall.Link(", "CreateSymbolicLink", "CreateHardLink", "mklink"}
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, s := range bad {
			if n := strings.Count(string(b), s); n > allowed[f+" "+s] {
				t.Errorf("%s uses %s %d times; appdir never creates links", f, s, n)
			}
		}
	}
	if checked < 10 {
		t.Fatalf("only %d source files checked; wrong folder?", checked)
	}
}

// TestPCDisposableProjectKindNoTempWithoutHome: docs/choosing/mode-and-scopes.md.
// The temp fallback for a machine with no $HOME is for global kinds only: a
// disposable kind scoped to the project fails with ErrNoProject outside a
// project and uses the project home inside one, never the temp folder.
func TestPCDisposableProjectKindNoTempWithoutHome(t *testing.T) {
	fx := tlNewFix(t)
	tlSetTemp(t, fx.root)
	noHome := WithHomeDir(func() (string, error) { return "", errors.New("no home") })
	kind := WithKind("thumbs", Disposable(), Scoped(ScopeProject))
	if _, err := fx.tlNew(t, noHome, kind).Ensure("thumbs"); !errors.Is(err, ErrNoProject) {
		t.Fatalf("Ensure(thumbs) outside a project err = %v; want ErrNoProject", err)
	}
	m := fx.tlProjectMarker(t)
	p, err := fx.tlNew(t, noHome, kind).Ensure("thumbs")
	if err != nil || p != filepath.Join(m, "thumbs") {
		t.Fatalf("Ensure(thumbs) in a project = %q, %v", p, err)
	}
	if tlExists(tlTempRoot(fx.root)) {
		t.Fatal("a project disposable kind used the temp folder")
	}
}

// TestPCIdentityKindGlobalInProjectMode: SPEC 5.2. Identity and login data
// are global: a kind declared with Scoped(ScopeGlobal) lives in ~/.<app>
// even in a project tool standing in a project, and works outside one.
func TestPCIdentityKindGlobalInProjectMode(t *testing.T) {
	fx := tlNewFix(t)
	kind := WithKind("identity", File("identity.json"), Scoped(ScopeGlobal))
	want := filepath.Join(fx.tlGlobal(), "identity.json")
	if p, err := fx.tlNew(t, kind).Path("identity"); err != nil || p != want {
		t.Fatalf("Path(identity) outside a project = %q, %v; want %s", p, err, want)
	}
	fx.tlProjectMarker(t)
	if p, err := fx.tlNew(t, kind).Path("identity"); err != nil || p != want {
		t.Fatalf("Path(identity) in a project = %q, %v; want %s", p, err, want)
	}
}
