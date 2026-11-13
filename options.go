package appdir

import (
	"io/fs"
	"os"
	"runtime/debug"
)

// Option changes one setting of New. Every choice in the standard is an
// option with the standard's default (rule zero, D17).
type Option func(*config)

// ConfigReader reads one JSONC file into a map. A missing file must return an
// error wrapping fs.ErrNotExist.
type ConfigReader func(path string) (map[string]any, error)

// LegacyVerify runs after a cross-disk copy in MoveLegacy, before the old
// folder is renamed away; a tool passes its database integrity check here.
type LegacyVerify func(src, dst string) error

// LegacyHome names an old global home folder to move from (standard section 8).
type LegacyHome struct {
	Dir         string   // folder name under the user's home, e.g. ".pm"
	DaemonLocks []string // lock files inside it to probe; default ["run/daemon.lock"]
}

// config is the full option set. It is built once in New and never changed.
type config struct {
	app            string
	dirName        string
	prefix         string
	mode           Mode
	lookup         Lookup
	kindEdits      []kindEdit
	gitCommit      []string
	gitIgnore      []string
	homeFlag       string
	profileFlag    string
	daemonFlag     string
	profiles       []Profile
	releaseProfile Profile
	devProfile     Profile
	release        *bool
	allowRoot      bool
	legacy         []LegacyHome
	legacyVerify   LegacyVerify
	readConfig     ConfigReader
	layoutVersion  int
	runFiles       map[RunFile]string
	socketMaxLen   int
	dirPerm        fs.FileMode
	filePerm       fs.FileMode
	tempFallback   bool
	cacheTag       bool
	testGuard      bool
	lookupEnv      func(string) (string, bool)
	homeDir        func() (string, error)
	workDir        func() (string, error)
	buildInfo      func() (*debug.BuildInfo, bool)
}

// ds:def id=defaultconfig-4tyrf8cd owner=@khanakia stability=stable
func defaultConfig(app string) *config {
	return &config{
		app:            app,
		dirName:        "." + app,
		prefix:         envPrefixFor(app),
		mode:           ModeProject,
		lookup:         WalkUp,
		profiles:       []Profile{ProfileProd, ProfileDev},
		releaseProfile: ProfileProd,
		devProfile:     ProfileDev,
		readConfig:     defaultConfigReader,
		layoutVersion:  defaultLayoutVersion,
		runFiles: map[RunFile]string{
			RunLock:       "lock",
			RunDaemonLock: "daemon.lock",
			RunPID:        app + ".pid",
			RunDiscovery:  "discovery.json",
			RunSocket:     app + ".sock",
			RunSocketFile: "socket",
		},
		socketMaxLen: defaultSocketMaxLen,
		dirPerm:      defaultDirPerm,
		filePerm:     defaultFilePerm,
		tempFallback: true,
		cacheTag:     true,
		testGuard:    true,
		lookupEnv:    os.LookupEnv,
		homeDir:      os.UserHomeDir,
		workDir:      os.Getwd,
		buildInfo:    debug.ReadBuildInfo,
	}
}

// WithDirName sets the home folder name. Default "." + app.
func WithDirName(name string) Option { return func(c *config) { c.dirName = name } }

// WithEnvPrefix sets the env prefix. Default: the app name upper-cased, with
// "-" and "." turned into "_".
func WithEnvPrefix(prefix string) Option { return func(c *config) { c.prefix = prefix } }

// WithMode sets the default scope of data kinds. Default ModeProject.
func WithMode(m Mode) Option { return func(c *config) { c.mode = m } }

// WithLookup sets the project lookup style. Default WalkUp.
func WithLookup(l Lookup) Option { return func(c *config) { c.lookup = l } }

// WithKind adds a kind, or changes an existing one in place (rename = change
// its path). A new kind starts as Folder(name), Ignore(), data scope.
func WithKind(name Kind, opts ...KindOption) Option {
	return func(c *config) { c.kindEdits = append(c.kindEdits, kindEdit{name: name, opts: opts}) }
}

// WithoutKind drops a kind.
func WithoutKind(name Kind) Option {
	return func(c *config) { c.kindEdits = append(c.kindEdits, kindEdit{name: name, remove: true}) }
}

// WithGitCommit adds committed gitignore patterns (relative to the home).
func WithGitCommit(patterns ...string) Option {
	return func(c *config) { c.gitCommit = append(c.gitCommit, patterns...) }
}

// WithGitIgnore forces patterns out of git; it wins over every commit rule.
func WithGitIgnore(patterns ...string) Option {
	return func(c *config) { c.gitIgnore = append(c.gitIgnore, patterns...) }
}

// WithHome passes the tool's --home flag. Empty means not given.
func WithHome(path string) Option { return func(c *config) { c.homeFlag = path } }

// WithProfile passes the tool's --profile flag. Empty means not given.
func WithProfile(name string) Option { return func(c *config) { c.profileFlag = name } }

// WithDaemonProfile passes the tool's --daemon-profile flag.
func WithDaemonProfile(name string) Option { return func(c *config) { c.daemonFlag = name } }

// WithProfiles sets the allowed profiles. Default prod, dev.
func WithProfiles(names ...Profile) Option {
	return func(c *config) { c.profiles = append([]Profile(nil), names...) }
}

// WithReleaseProfile sets the profile of a release build, the one that keeps
// base names. Default prod.
func WithReleaseProfile(p Profile) Option { return func(c *config) { c.releaseProfile = p } }

// WithDevProfile sets the profile of a non-release build. Default dev.
func WithDevProfile(p Profile) Option { return func(c *config) { c.devProfile = p } }

// WithRelease states whether this binary is a release build, from the tool's
// own -ldflags -X variable. Without it, appdir reads Go's build info.
func WithRelease(release bool) Option { return func(c *config) { c.release = &release } }

// WithAllowRoot passes the tool's --allow-root flag.
func WithAllowRoot(allow bool) Option { return func(c *config) { c.allowRoot = allow } }

// WithLegacyHome names an old global home that MoveLegacy moves from.
func WithLegacyHome(l LegacyHome) Option { return func(c *config) { c.legacy = append(c.legacy, l) } }

// WithLegacyVerify adds a check after a cross-disk legacy copy.
func WithLegacyVerify(fn LegacyVerify) Option { return func(c *config) { c.legacyVerify = fn } }

// WithConfigReader replaces the JSONC reader for home.jsonc and pointer keys.
func WithConfigReader(fn ConfigReader) Option { return func(c *config) { c.readConfig = fn } }

// WithLayoutVersion sets the layout version this binary knows. Default 1.
func WithLayoutVersion(n int) Option { return func(c *config) { c.layoutVersion = n } }

// WithRunFile renames one file in run/.
func WithRunFile(f RunFile, name string) Option {
	return func(c *config) { c.runFiles[f] = name }
}

// WithSocketMaxLen sets the longest socket path before the /tmp fallback.
// Default 100 bytes (macOS allows about 104).
func WithSocketMaxLen(n int) Option { return func(c *config) { c.socketMaxLen = n } }

// WithPerm sets the modes of folders and files appdir creates. Default 0700 / 0600.
func WithPerm(dir, file fs.FileMode) Option {
	return func(c *config) { c.dirPerm, c.filePerm = dir, file }
}

// WithTempFallback turns the temp fallback of disposable kinds on or off. Default on.
func WithTempFallback(on bool) Option { return func(c *config) { c.tempFallback = on } }

// WithCacheTag turns CACHEDIR.TAG and OS backup/index exclusions on or off. Default on.
func WithCacheTag(on bool) Option { return func(c *config) { c.cacheTag = on } }

// WithTestGuard turns the test-binary path guard on or off. Default on.
func WithTestGuard(on bool) Option { return func(c *config) { c.testGuard = on } }

// WithEnv replaces the environment lookup (seam for tests and ForProfile).
func WithEnv(fn func(string) (string, bool)) Option { return func(c *config) { c.lookupEnv = fn } }

// WithHomeDir replaces the user home lookup.
func WithHomeDir(fn func() (string, error)) Option { return func(c *config) { c.homeDir = fn } }

// WithWorkDir fixes the folder the project lookup starts from.
func WithWorkDir(dir string) Option {
	return func(c *config) { c.workDir = func() (string, error) { return dir, nil } }
}

// WithBuildInfo replaces debug.ReadBuildInfo for release detection.
func WithBuildInfo(fn func() (*debug.BuildInfo, bool)) Option {
	return func(c *config) { c.buildInfo = fn }
}
