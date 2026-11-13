package appdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Dirs is the resolved set of homes and paths for one app. It is immutable
// and safe for concurrent use.
type Dirs struct {
	cfg  *config
	opts []Option // kept so ForProfile and CheckDB can resolve again

	kinds         []KindSpec
	profile       Profile
	release       bool
	releaseReason string
	daemonProfile Profile
	allowProd     bool
	warnings      []Warning

	userHome    string
	userHomeErr error
	workDir     string

	override   bool // --home or <APP>_HOME: one home for every scope
	global     Home
	globalErr  error
	project    Home
	projectErr error

	dbPointer    string // db: pointer target (release profile only)
	targetConfig string // home: pointer target holding its own config files
}

// Home is one resolved home folder.
type Home struct {
	Scope    Scope     `json:"scope"`            // global or project
	Marker   string    `json:"marker,omitempty"` // the .<app>/ that was found (project only)
	Base     string    `json:"base"`             // holds home.jsonc and the config files
	Data     string    `json:"data"`             // holds every other kind
	Rule     Rule      `json:"rule"`
	Fallback bool      `json:"fallback"`
	Note     string    `json:"note,omitempty"`
	Info     *HomeInfo `json:"info,omitempty"` // parsed home.jsonc, nil when absent
}

// HomeInfo is the content of home.jsonc.
type HomeInfo struct {
	App    string `json:"app"`
	Layout int    `json:"layout"`
	Scope  Scope  `json:"scope"`
}

// Entry is one kind's resolved path, for paths --json and doctor.
type Entry struct {
	Kind     Kind      `json:"kind"`
	Home     Scope     `json:"home"`
	Path     string    `json:"path"`
	Type     EntryType `json:"type"`
	Git      GitRule   `json:"git"`
	Rule     Rule      `json:"rule"`
	Fallback bool      `json:"fallback"`
	Note     string    `json:"note,omitempty"`
	Error    string    `json:"error,omitempty"`

	err      error
	homeRoot string // the folder whose root/layout checks apply on write
	info     *HomeInfo
	infoDir  string // the folder info (home.jsonc) was read from, for errors
}

// Warning is something the tool should print once (to stderr).
type Warning struct {
	Code    WarningCode `json:"code"`
	Message string      `json:"message"`
}

// ConfigFile is one config file in read order (later wins per key).
type ConfigFile struct {
	Path      string  `json:"path"`
	Home      Scope   `json:"home"`
	Local     bool    `json:"local"`
	Committed bool    `json:"committed"`
	Profile   Profile `json:"profile,omitempty"` // set on the profile's own config.local.jsonc
}

// New resolves every home and path for app. It never creates anything. It
// fails only for a bad option set or an unknown profile; a missing project
// home is reported by the methods that need one.
// ds:def id=new-mes9nm7s owner=@khanakia stability=stable
func New(app string, opts ...Option) (*Dirs, error) {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`).MatchString(app) {
		return nil, newErr(ErrInvalidOption, "appdir: app name %q must match ^[a-z0-9][a-z0-9._-]*$", app)
	}
	c := defaultConfig(app)
	for _, o := range opts {
		o(c)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	kinds, err := buildKinds(app, c.mode, c.kindEdits)
	if err != nil {
		return nil, err
	}
	d := &Dirs{cfg: c, opts: append([]Option(nil), opts...), kinds: kinds}
	d.userHome, d.userHomeErr = c.homeDir()
	if d.userHomeErr == nil {
		if d.userHome == "" || !filepath.IsAbs(d.userHome) {
			d.userHomeErr = fmt.Errorf("home folder %q is not absolute", d.userHome)
		} else {
			d.userHome = filepath.Clean(d.userHome)
		}
	}
	wd, err := c.workDir()
	if err != nil {
		return nil, fmt.Errorf("appdir: working folder: %w", err)
	}
	if d.workDir, err = filepath.Abs(wd); err != nil {
		return nil, fmt.Errorf("appdir: working folder: %w", err)
	}
	d.allowProd = d.env(envAllowProd) == "1"
	d.release, d.releaseReason = detectRelease(c)
	if err := d.pickProfile(); err != nil {
		return nil, err
	}
	d.resolveHomes()
	if d.guardActive() {
		d.guardTestPaths()
	}
	return d, nil
}

func (c *config) validate() error {
	switch {
	case !plainSegment(c.dirName):
		return newErr(ErrInvalidOption, "appdir: dir name %q must be one plain path segment", c.dirName)
	case c.prefix == "":
		return newErr(ErrInvalidOption, "appdir: env prefix must not be empty")
	case c.mode != ModeProject && c.mode != ModeGlobal:
		return newErr(ErrInvalidOption, "appdir: unknown mode %q", c.mode)
	case c.lookup != WalkUp && c.lookup != HereOnly:
		return newErr(ErrInvalidOption, "appdir: unknown lookup %q", c.lookup)
	case c.layoutVersion < 1:
		return newErr(ErrInvalidOption, "appdir: layout version must be at least 1")
	case c.socketMaxLen < 1:
		return newErr(ErrInvalidOption, "appdir: socket max length must be positive")
	case c.readConfig == nil || c.lookupEnv == nil || c.homeDir == nil || c.workDir == nil || c.buildInfo == nil:
		return newErr(ErrInvalidOption, "appdir: a seam option was given nil")
	}
	if len(c.profiles) == 0 {
		return newErr(ErrInvalidOption, "appdir: the profile list is empty")
	}
	for _, p := range []Profile{c.releaseProfile, c.devProfile} {
		if !c.knownProfile(p) {
			return newErr(ErrInvalidOption, "appdir: profile %q is not in the list %v", p, c.profiles)
		}
	}
	for f, name := range c.runFiles {
		if !plainSegment(name) {
			return newErr(ErrInvalidOption, "appdir: run file %q name %q must be one plain path segment", f, name)
		}
	}
	for _, l := range c.legacy {
		if !plainSegment(l.Dir) {
			return newErr(ErrInvalidOption, "appdir: legacy dir %q must be one plain path segment", l.Dir)
		}
	}
	return nil
}

// plainSegment reports whether name is one plain path segment: not empty,
// not "." or ".." (they name the parent or the folder itself, so
// WithDirName("..") would make $HOME's parent the global home), and free of
// separators and NUL on every OS.
func plainSegment(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

// App returns the app name.
func (d *Dirs) App() string { return d.cfg.app }

// EnvPrefix returns the env prefix ("LORE").
func (d *Dirs) EnvPrefix() string { return d.cfg.prefix }

// EnvName returns "<PREFIX>_" + suffix.
func (d *Dirs) EnvName(suffix string) string { return d.cfg.envName(suffix) }

// Mode returns the tool's mode.
func (d *Dirs) Mode() Mode { return d.cfg.mode }

// Kinds returns the layout in effect, in stable order.
func (d *Dirs) Kinds() []KindSpec { return append([]KindSpec(nil), d.kinds...) }

// KindEnv returns the <APP>_<KIND>_DIR name of a kind, or "" for anchored kinds.
func (d *Dirs) KindEnv(k Kind) string {
	spec, ok := d.kind(k)
	if !ok || spec.Anchored {
		return ""
	}
	return d.cfg.envName(envPrefixFor(string(k)) + envKindSuffix)
}

// Warnings returns what the tool should print once.
func (d *Dirs) Warnings() []Warning { return append([]Warning(nil), d.warnings...) }

// Global returns the global home. With no $HOME and no override, its Rule is
// RuleDefaults and Base/Data are empty.
func (d *Dirs) Global() Home { return d.global }

// Project returns the project home, or an error wrapping ErrNoProject (or the
// pointer error that made it unusable).
func (d *Dirs) Project() (Home, error) {
	if d.projectErr != nil {
		return Home{}, d.projectErr
	}
	return d.project, nil
}

// HasProject reports whether a usable project home was found.
func (d *Dirs) HasProject() bool { return d.projectErr == nil }

// projectBroken reports a project home that was found (or demanded by an
// override) but cannot be used, as opposed to none found (ErrNoProject).
func (d *Dirs) projectBroken() bool {
	return d.projectErr != nil && !errors.Is(d.projectErr, ErrNoProject)
}

// Path returns a kind's path in its effective home (reads only). For ScopeBoth
// kinds that is the project home when there is one, else the global home.
// ds:def id=dirs-path-asygw3vm owner=@khanakia stability=stable
func (d *Dirs) Path(k Kind) (string, error) {
	e, err := d.entry(k)
	if err != nil {
		return "", err
	}
	return e.Path, e.err
}

// PathIn returns a kind's path in one home: ScopeGlobal or ScopeProject.
// ds:def id=dirs-pathin-2j8zfrym owner=@khanakia stability=stable
func (d *Dirs) PathIn(k Kind, s Scope) (string, error) {
	spec, ok := d.kind(k)
	if !ok {
		return "", newErr(ErrUnknownKind, "appdir: unknown kind %q", k)
	}
	if s != ScopeGlobal && s != ScopeProject {
		return "", newErr(ErrInvalidOption, "appdir: PathIn needs global or project, got %q", s)
	}
	e := d.entryIn(spec, s, false)
	return e.Path, e.err
}

// Paths returns every kind's path, for paths --json and doctor. ScopeBoth
// kinds appear once per home.
// ds:def id=dirs-paths-6kdzcjzg owner=@khanakia stability=stable
func (d *Dirs) Paths() []Entry {
	var out []Entry
	for _, spec := range d.kinds {
		if spec.Scope == ScopeBoth && !d.override {
			out = append(out, d.entryIn(spec, ScopeGlobal, false))
			if d.HasProject() || d.projectBroken() { // the broken line shows its error
				out = append(out, d.entryIn(spec, ScopeProject, false))
			}
			continue
		}
		e, _ := d.entry(spec.Name)
		out = append(out, e)
	}
	return out
}

// ConfigFiles returns the config files in read order (SPEC 6.1). Missing
// files are listed; the loader treats them as empty.
// ds:def id=dirs-configfiles-nzbecu5q owner=@khanakia stability=stable
func (d *Dirs) ConfigFiles() []ConfigFile {
	// A config kind the author dropped (WithoutKind) has no file: it is left
	// out, never listed as the home folder itself.
	cfgSpec, hasCfg := d.kind(KindConfig)
	localSpec, hasLocal := d.kind(KindConfigLocal)
	var out []ConfigFile
	add := func(base string, home Scope, committable bool) {
		if base == "" {
			return
		}
		if hasCfg {
			out = append(out, ConfigFile{Path: filepath.Join(base, filepath.FromSlash(cfgSpec.Path)), Home: home, Committed: committable && inGitWorkTree(base)})
		}
		if hasLocal {
			out = append(out, ConfigFile{Path: filepath.Join(base, filepath.FromSlash(localSpec.Path)), Home: home, Local: true})
		}
	}
	if d.override {
		add(d.global.Base, ScopeGlobal, false)
		return out
	}
	if d.globalErr == nil {
		add(d.global.Base, ScopeGlobal, false)
	}
	if d.projectErr != nil && d.project.Marker != "" {
		// A broken pointer: still list the marker's own two files, which
		// hold the pointer the user has to fix (F11).
		add(d.project.Marker, ScopeProject, true)
	}
	if d.projectErr == nil {
		add(d.project.Marker, ScopeProject, true)
		if d.targetConfig != "" {
			add(d.targetConfig, ScopeProject, true)
		}
		// A non-release profile may keep its own machine settings in
		// .<app>/profiles/<profile>/config.local.jsonc: gitignored with the
		// rest of profiles/, read last so it wins over both shared files
		// (owner, 2026-11-10).
		if hasLocal && d.profile != d.cfg.releaseProfile && d.project.Data != d.project.Base {
			l := filepath.Join(d.project.Data, filepath.FromSlash(localSpec.Path))
			out = append(out, ConfigFile{Path: l, Home: ScopeProject, Local: true, Profile: d.profile})
		}
	}
	return out
}

// kind returns the spec of k.
func (d *Dirs) kind(k Kind) (KindSpec, bool) {
	for _, s := range d.kinds {
		if s.Name == k {
			return s, true
		}
	}
	return KindSpec{}, false
}

// entry resolves k in its effective home.
func (d *Dirs) entry(k Kind) (Entry, error) {
	spec, ok := d.kind(k)
	if !ok {
		return Entry{}, newErr(ErrUnknownKind, "appdir: unknown kind %q", k)
	}
	switch spec.Scope {
	case ScopeGlobal:
		return d.entryIn(spec, ScopeGlobal, false), nil
	case ScopeProject:
		return d.entryIn(spec, ScopeProject, false), nil
	default: // project-else-global, both
		// Under --home / <APP>_HOME there is one home; a both kind is
		// labelled global there, like ConfigFiles does (X1).
		if spec.Scope == ScopeBoth && d.override {
			return d.entryIn(spec, ScopeGlobal, false), nil
		}
		// A project that was found but is unusable (a broken pointer) fails
		// with its own error: falling back to the global home would read or
		// write the wrong place without a word (SPEC 4.5, F11).
		if d.HasProject() || d.projectBroken() {
			return d.entryIn(spec, ScopeProject, false), nil
		}
		e := d.entryIn(spec, ScopeGlobal, false)
		if spec.Scope == ScopeProjectElseGlobal {
			e.Fallback = true
			if e.Note == "" {
				e.Note = "no project home; using the global home"
			}
		}
		return e, nil
	}
}

// entryIn resolves spec in one home (SPEC 4.7). forceTemp is used by Ensure
// when creating a disposable folder failed.
// ds:def id=dirs-entryin-zhkmadk3 owner=@khanakia stability=stable
func (d *Dirs) entryIn(spec KindSpec, home Scope, forceTemp bool) Entry {
	e := Entry{Kind: spec.Name, Home: home, Type: spec.Type, Git: spec.Git}
	rel := filepath.FromSlash(spec.Path)

	if !spec.Anchored && !forceTemp {
		if raw := d.env(envPrefixFor(string(spec.Name)) + envKindSuffix); raw != "" {
			name := d.KindEnv(spec.Name)
			p, err := d.cfg.expandUserPath(raw, d.workDir, d.userHome, d.userHomeErr)
			switch {
			case err != nil:
				e.Rule, e.err = RuleEnvKind, fmt.Errorf("%s: %w", name, err)
				e.Error = e.err.Error()
				return e
			case d.pointsToProd(p):
				// warned once in resolveHomes; fall through to the normal rules
			default:
				e.Rule, e.homeRoot = RuleEnvKind, p
				e.Path = p
				if spec.Type == TypeFile {
					e.Path = filepath.Join(p, filepath.Base(rel))
				}
				e.Note = "from " + name
				return e
			}
		}
	}

	if spec.Name == KindDB && home == ScopeProject && d.dbPointer != "" && d.projectErr == nil {
		e.Rule, e.Path, e.homeRoot, e.info, e.infoDir = RulePointer, d.dbPointer, filepath.Dir(d.dbPointer), d.project.Info, d.project.Base
		e.Note = "db: pointer"
		return e
	}

	var h Home
	var herr error
	if home == ScopeGlobal {
		h, herr = d.global, d.globalErr
	} else {
		h, herr = d.project, d.projectErr
	}
	// The temp fallback and code defaults stand in for a machine with no
	// $HOME only. A --home or <APP>_HOME that failed to expand is the user's
	// explicit choice gone wrong: every kind fails with that error rather
	// than quietly writing somewhere else (F2).
	noUserHome := herr != nil && home == ScopeGlobal && !d.override
	if forceTemp || (noUserHome && spec.Disposable && d.cfg.tempFallback) {
		root := d.tempRoot()
		e.Rule, e.Fallback, e.homeRoot = RuleTemp, true, root
		e.Path = filepath.Join(root, rel)
		e.Note = "temp fallback"
		return e
	}
	if herr != nil {
		if noUserHome && spec.Anchored {
			e.Rule, e.Fallback = RuleDefaults, true
			e.Note = "no home folder; code defaults"
			return e
		}
		// The rule that failed to give a home.
		e.Rule, e.err = h.Rule, herr
		switch {
		case home == ScopeGlobal && !d.override:
			e.Rule = RuleGlobal
		case e.Rule == "":
			e.Rule = RuleProject
		}
		e.Error = herr.Error()
		return e
	}
	root := h.Data
	if spec.Anchored {
		root = h.Base
	}
	// home.jsonc is always read from Base; Data differs for a non-release
	// profile, so the layout error must name Base (F15).
	e.Rule, e.Fallback, e.homeRoot, e.info, e.infoDir = h.Rule, h.Fallback, root, h.Info, h.Base
	e.Path = filepath.Join(root, rel)
	if h.Note != "" {
		e.Note = h.Note
	}
	return e
}

// env reads <PREFIX>_<suffix>, empty = unset.
func (d *Dirs) env(suffix string) string {
	v, _ := d.cfg.getenv(d.cfg.envName(suffix))
	return v
}

// tempRoot is <tmp>/<Name(app)>-<uid> (SPEC 4.7).
func (d *Dirs) tempRoot() string {
	return filepath.Join(os.TempDir(), d.Name(d.cfg.app)+"-"+currentUser())
}

// guardTestPaths panics when a test binary resolved a path outside the test
// root (standard section 13): a test must never write into real homes.
// ds:def id=dirs-guardtestpaths-2wuv39pv owner=@khanakia stability=stable
func (d *Dirs) guardTestPaths() {
	d.guardPath(d.global.Base)
	d.guardPath(d.global.Data)
	if d.projectErr == nil {
		d.guardPath(d.project.Base)
		d.guardPath(d.project.Data)
	}
	for _, e := range d.Paths() {
		d.guardPath(e.Path)
	}
}

// guardPath panics when p is outside the test root (APPDIR_TEST_ROOT, else
// os.TempDir()). Callers only call it in a test binary with the guard on.
func (d *Dirs) guardPath(p string) {
	root, _ := testRoot()
	if p != "" && !within(p, root) {
		d.guardPanic(p, root)
	}
}

// guardSocketDir is guardPath for the socket fallback folder, plus one more
// refusal: under the default root a folder inside the shared
// /tmp/<app>-<uid> is refused too. With TMPDIR unset os.TempDir() is /tmp,
// which contains that folder, yet it is where the real user's daemon keeps
// its socket, not the test's (finding L1). The temp fallback of disposable
// kinds can be the same folder then, and stays allowed: it only holds
// disposable files, while a socket there can reach a live daemon. An
// explicit APPDIR_TEST_ROOT is a per-test folder and never contains it.
func (d *Dirs) guardSocketDir(dir string) {
	d.guardPath(dir)
	root, defaulted := testRoot()
	if shared := d.sharedSocketDir(); defaulted && shared != "" && dir != "" && within(dir, shared) {
		d.guardPanic(dir, root)
	}
}

// testRoot returns the guard's root: APPDIR_TEST_ROOT when set, else
// os.TempDir() with defaulted true.
func testRoot() (root string, defaulted bool) {
	root, ok := os.LookupEnv(envTestRoot)
	if !ok || root == "" {
		return os.TempDir(), true
	}
	return root, false
}

// guardPanic is the one panic message of the test guard.
func (d *Dirs) guardPanic(p, root string) {
	panic(fmt.Sprintf("appdir: test binary resolved %s outside the test root %s; call appdir.ForTest(t, %q) or pass WithTestGuard(false)", p, root, d.cfg.app))
}

// guardActive reports whether the test-binary path guard applies.
func (d *Dirs) guardActive() bool { return d.cfg.testGuard && testing.Testing() }

// noProjectErr builds the standard's message.
func (d *Dirs) noProjectErr() error {
	return newErr(ErrNoProject, "no %s/ here; run '%s init' or pass --home", d.cfg.dirName, d.cfg.app)
}

// isNotExist is errors.Is(err, fs.ErrNotExist) for readability.
func isNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }
