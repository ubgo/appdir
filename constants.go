package appdir

// Rule: no bare strings for closed sets. Every kind, scope, rule, profile and
// warning code that appears in a switch, a map key or JSON output is one of the
// typed constants below.

// Kind names one entry in a home's layout: a file or a folder.
type Kind string

// Standard kinds, used when the tool's author changes nothing (SPEC 5.1).
const (
	KindHome        Kind = "home"         // home.jsonc: app, layout, scope
	KindConfig      Kind = "config"       // config.jsonc: settings, committed
	KindConfigLocal Kind = "config-local" // config.local.jsonc: this machine only
	KindGitignore   Kind = "gitignore"    // .gitignore: written between markers
	KindShared      Kind = "shared"       // shared/: team-shared files, committed
	KindDB          Kind = "db"           // <app>.db
	KindBackups     Kind = "backups"      // backups/
	KindCache       Kind = "cache"        // cache/: safe to delete
	KindRun         Kind = "run"          // run/: lock, pid, socket, discovery
	KindLogs        Kind = "logs"         // logs/
)

// Scope says which home a kind lives in.
type Scope string

// Scopes. ScopeGlobal and ScopeProject also name the two homes in results.
const (
	ScopeGlobal            Scope = "global"
	ScopeProject           Scope = "project"
	ScopeProjectElseGlobal Scope = "project-else-global"
	ScopeBoth              Scope = "both" // one copy per home (config files, home.jsonc)
)

// Mode is the kind of tool: it sets the default scope of data kinds.
type Mode string

// Modes. ModeProject is the default because it fails safe (SPEC 5.2).
const (
	ModeProject Mode = "project"
	ModeGlobal  Mode = "global"
)

// Lookup is the project lookup style.
type Lookup string

// Lookup styles.
const (
	WalkUp   Lookup = "walk-up"
	HereOnly Lookup = "here-only"
)

// EntryType says whether a kind is a file or a folder.
type EntryType string

// Entry types.
const (
	TypeFile   EntryType = "file"
	TypeFolder EntryType = "folder"
)

// GitRule says whether a kind is committed.
type GitRule string

// Git rules.
const (
	GitCommit GitRule = "commit"
	GitIgnore GitRule = "ignore"
)

// Rule names the resolution rule that picked a path, for paths --json and
// doctor.
type Rule string

// Rules.
const (
	RuleFlag     Rule = "flag"     // --home (WithHome)
	RuleEnvHome  Rule = "env-home" // <APP>_HOME
	RuleEnvKind  Rule = "env-kind" // <APP>_<KIND>_DIR
	RuleProject  Rule = "project"  // found by lookup
	RuleWorktree Rule = "worktree" // main checkout of a linked worktree
	RulePointer  Rule = "pointer"  // home: or db: key
	RuleGlobal   Rule = "global"   // ~/.<app>
	RuleTemp     Rule = "temp"     // <tmp>/<app>-<uid>/
	RuleDefaults Rule = "defaults" // no $HOME: global config is code defaults
)

// Profile names a set of homes (prod, dev, or an author-added name).
type Profile string

// Default profiles.
const (
	ProfileProd Profile = "prod"
	ProfileDev  Profile = "dev"
)

// DBAccess is the result of the prod-db guard.
type DBAccess int

// DB access levels.
const (
	AccessReadWrite DBAccess = iota
	AccessReadOnly           // dev build on the prod db with <APP>_ALLOW_PROD=1: never migrate
)

// String implements fmt.Stringer.
func (a DBAccess) String() string {
	if a == AccessReadOnly {
		return "read-only"
	}
	return "read-write"
}

// RunFile names a file in run/.
type RunFile string

// Run files (SPEC 7).
const (
	RunLock       RunFile = "lock"        // run/lock: migrations, layout changes
	RunDaemonLock RunFile = "daemon-lock" // run/daemon.lock: held for a daemon's life
	RunPID        RunFile = "pid"         // run/<app>.pid: display only
	RunDiscovery  RunFile = "discovery"   // run/discovery.json
	RunSocket     RunFile = "socket"      // run/<app>.sock
	RunSocketFile RunFile = "socket-file" // run/socket: path of a fallback socket
)

// MoveStatus is the outcome of MoveLegacy.
type MoveStatus string

// Move outcomes.
const (
	MoveNone      MoveStatus = "none"
	MoveDone      MoveStatus = "moved"
	MoveBothExist MoveStatus = "both-exist"
	MoveSkipped   MoveStatus = "skipped"
	MoveAlready   MoveStatus = "already"
)

// WarningCode identifies a warning.
type WarningCode string

// Warning codes.
const (
	WarnProfileEnvIgnored WarningCode = "profile-env-ignored"
	WarnEnvPointsToProd   WarningCode = "env-points-to-prod"
	WarnPointerChain      WarningCode = "pointer-chain-ignored"
	WarnPointerProfile    WarningCode = "pointer-ignored-for-profile"
	WarnCeilingRelative   WarningCode = "ceiling-relative"
	// WarnCeilingDropped: an <APP>_CEILING_DIRS entry that is not relative
	// but does not expand to an absolute path ($VAR without braces, an unset
	// ${VAR}, ~user); it is ignored, and without this warning a ceiling would
	// silently have no effect (finding D5).
	WarnCeilingDropped      WarningCode = "ceiling-dropped"
	WarnWorktreeMainMissing WarningCode = "worktree-main-missing"
)

// Env suffixes under <APP>_ (SPEC 5.3: reserved names).
const (
	envHome          = "HOME"
	envProfile       = "PROFILE"
	envAllowProd     = "ALLOW_PROD"
	envCeilingDirs   = "CEILING_DIRS"
	envDaemonProfile = "DAEMON_PROFILE"
	envKindSuffix    = "_DIR"
	envTestRoot      = "APPDIR_TEST_ROOT"
)

// Fixed names inside a home.
const (
	profilesDir      = "profiles"
	movedFile        = "MOVED.txt"
	legacyMarkerFile = ".legacy-move"
	cacheTagFile     = "CACHEDIR.TAG"
	gitEntry         = ".git"
	pointerKeyHome   = "home"
	pointerKeyDB     = "db"
	homeKeyApp       = "app"
	homeKeyLayout    = "layout"
	homeKeyScope     = "scope"
)

// Defaults (rule zero: each has an option).
// ds:def id=default-constants-kpphgn26 owner=@khanakia stability=stable
const (
	defaultLayoutVersion = 1
	defaultSocketMaxLen  = 100
	defaultDirPerm       = 0o700
	defaultFilePerm      = 0o600
	socketHashLen        = 12
)
