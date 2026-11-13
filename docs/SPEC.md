# appdir v2 — specification

<DocStatus state="review" owner="khanakia" updated="2026-11-10"></DocStatus>

Module `github.com/ubgo/appdir`. This is the spec a developer codes from. It implements the folder parts of `app-standard.md` (draft v2, 2026-11-08): sections 1, 2, 3, 4, 8, 10, the platform table, and the path parts of section 9. Every rule below has a short example (who, what happens, what they see), in the same style as the standard.

In this page `<app>` is the tool's short name (`lore`) and `<APP>` its env prefix (`LORE`). Code snippets use `lore` and `demo` as sample apps.

## 1. Purpose and scope

appdir answers one question for a tool: **where do my things live?** It finds the global home and the project home, knows which folders and files sit inside them, picks the profile (prod or dev), and reports which rule picked each path so `paths --json` and `doctor` can show it.

<Callout type="info" title="In scope">

- Finding the global home (`~/.<app>`) and the project home (`.<app>/`), with flags, env, walk-up and pointer keys (standard sections 1, 4).
- The layout: standard kinds, author-declared kinds, `<APP>_<KIND>_DIR`, permissions, symlink rules, `CACHEDIR.TAG`, the `.gitignore` block (section 2).
- Two scopes and the list of config files to read, in order (section 3). Merging them is cfgkit's job.
- Profiles: detection, release detection, `Name(base)`, the prod-db guard, `CleanEnv`, the env-leak filter, `--daemon-profile` path lookup (section 10, section 9 paths).
- Run paths: `run/` files, the socket path with its fallback, the discovery file path (section 9, paths only).
- Moving an old global home (section 8).
- Test isolation: `ForTest(t)` and the test-binary guard (section 13, review G1).

</Callout>

<Callout type="warning" title="Out of scope (other packages)">

- Config loading, merging, unknown keys, secrets, global-only settings: `ubgo/cfgkit` plus its JSONC source. appdir only lists the files and says which are committed.
- JSONC parsing and editing: `ubgo/jsonc` (not built yet). appdir has a tiny internal reader until then (section 6.3).
- Database open, pragmas, migrations, backups, integrity checks: `sqlitedb`. appdir gives it the paths and the prod-db guard.
- Agent install: `agentinstall`. It uses `Name(base)` and the global home from appdir.
- Daemon protocol, handshake, discovery file content, lock hand-off: the tool. appdir gives the paths.
- Log rotation: the tool's logger. appdir gives `logs/`.

</Callout>

## 2. Design rules

- **Rule zero.** Every choice is a package option with the standard's default. Nothing here is hard-wired; a case the standard did not foresee gets an option, not a workaround. Renamed or replaced names are the author's call: no warning, no doctor note (D17).
- **No hidden state.** No package-level mutable variables, no caches, no `init()`. Every outside input (env, home dir, working dir, build info) goes through a seam that has a default and an option.
- **Reads never write.** `New`, `Paths`, `Path`, `Socket`, `ConfigFiles` and every other read only stat and read files. Folders are created by `Ensure`, `InitProject`, `InitHome`, `EnsureSocketDir`, `WriteGitignore` and `MoveLegacy`, which are the only methods that write.
- **Errors are deferred.** `New` fails only for a bad option set, an unknown profile, or an unreadable working folder. A missing project home is stored and returned by the methods that need it, so a command that only needs the cache still runs outside a project.
- **Stdlib only.** No third-party module. OS-specific code sits behind build tags (section 12).

## 3. Public API

### 3.1 Constructor

```go
// New resolves every folder for app. It never creates anything.
func New(app string, opts ...Option) (*Dirs, error)
```

`app` must match `^[a-z0-9][a-z0-9._-]*$`. `*Dirs` is immutable and safe for concurrent use.

**Example:** lore's `main` calls `appdir.New("lore", appdir.WithHome(flagHome), appdir.WithProfile(flagProfile), appdir.WithLookup(appdir.HereOnly), appdir.WithKind(appdir.KindShared, appdir.Folder("data")))`. The call reads the env and the disk but creates nothing, so `lore version` run in a read-only container still works.

### 3.2 Types and constants

```go
type Kind string      // a named entry in the layout
type Scope string     // where a kind lives
type Mode string      // project tool or global tool
type Lookup string    // project lookup style
type EntryType string // file or folder
type GitRule string   // commit or ignore
type Rule string      // which rule picked a path
type Profile string   // prod, dev, or an author-added name
type DBAccess int     // result of the prod-db guard
type RunFile string   // a named file in run/
type MoveStatus string
type WarningCode string

const (
    KindHome        Kind = "home"         // home.jsonc
    KindConfig      Kind = "config"       // config.jsonc
    KindConfigLocal Kind = "config-local" // config.local.jsonc
    KindGitignore   Kind = "gitignore"    // .gitignore
    KindShared      Kind = "shared"       // shared/
    KindDB          Kind = "db"           // <app>.db
    KindBackups     Kind = "backups"      // backups/
    KindCache       Kind = "cache"        // cache/
    KindRun         Kind = "run"          // run/
    KindLogs        Kind = "logs"         // logs/
)

const (
    ScopeGlobal            Scope = "global"
    ScopeProject           Scope = "project"
    ScopeProjectElseGlobal Scope = "project-else-global"
    ScopeBoth              Scope = "both" // one per home: config files, home.jsonc
)

const (
    ModeProject Mode = "project" // default
    ModeGlobal  Mode = "global"
)

const (
    WalkUp   Lookup = "walk-up" // default
    HereOnly Lookup = "here-only"
)

const (
    TypeFile   EntryType = "file"
    TypeFolder EntryType = "folder"
)

const (
    GitCommit GitRule = "commit"
    GitIgnore GitRule = "ignore"
)

const (
    RuleFlag     Rule = "flag"      // --home (WithHome)
    RuleEnvHome  Rule = "env-home"  // <APP>_HOME
    RuleEnvKind  Rule = "env-kind"  // <APP>_<KIND>_DIR
    RuleProject  Rule = "project"   // found by lookup
    RuleWorktree Rule = "worktree"  // main checkout of a linked worktree
    RulePointer  Rule = "pointer"   // home: or db: key
    RuleGlobal   Rule = "global"    // ~/.<app>
    RuleTemp     Rule = "temp"      // <tmp>/<app>-<uid>/
    RuleDefaults Rule = "defaults"  // no $HOME: global config uses code defaults
)

const (
    ProfileProd Profile = "prod"
    ProfileDev  Profile = "dev"
)

const (
    AccessReadWrite DBAccess = iota // normal
    AccessReadOnly                  // dev build on the prod db with <APP>_ALLOW_PROD=1
)

const (
    RunLock       RunFile = "lock"        // run/lock
    RunDaemonLock RunFile = "daemon-lock" // run/daemon.lock
    RunPID        RunFile = "pid"         // run/<app>.pid
    RunDiscovery  RunFile = "discovery"   // run/discovery.json
    RunSocket     RunFile = "socket"      // run/<app>.sock
    RunSocketFile RunFile = "socket-file" // run/socket (holds the fallback socket path)
)

const (
    MoveNone      MoveStatus = "none"       // no old folder
    MoveDone      MoveStatus = "moved"      // moved now
    MoveBothExist MoveStatus = "both-exist" // new one used, old left alone
    MoveSkipped   MoveStatus = "skipped"    // dev profile, or --home/<APP>_HOME in use
    MoveAlready   MoveStatus = "already"    // old place holds only MOVED.txt
)
```

### 3.3 Options (every one, with its default)

| Option | Default | What it sets |
|---|---|---|
| `WithDirName(name)` | `"." + app` | home folder name (global `~/<name>`, project `<name>/`); one plain folder name (rules below) |
| `WithEnvPrefix(p)` | upper-cased app, `-` and `.` become `_` | env prefix (`LORE`) |
| `WithMode(m)` | `ModeProject` | default scope of data kinds (section 5.2) |
| `WithLookup(l)` | `WalkUp` | project lookup style |
| `WithKind(name, ...KindOption)` | standard kinds | add a kind or change one |
| `WithoutKind(name)` | — | drop a kind |
| `WithGitCommit(patterns...)` | from kinds | extra committed patterns |
| `WithGitIgnore(patterns...)` | from kinds | patterns forced ignored; wins over commit |
| `WithHome(path)` | `""` | the tool's `--home` flag |
| `WithProfile(name)` | `""` | the tool's `--profile` flag |
| `WithDaemonProfile(name)` | `""` | the tool's `--daemon-profile` flag |
| `WithProfiles(names...)` | `prod`, `dev` | allowed profiles (open list) |
| `WithReleaseProfile(name)` | `prod` | profile of a release build; the one that keeps base names |
| `WithDevProfile(name)` | `dev` | profile of a non-release build |
| `WithRelease(bool)` | from build info | explicit release flag, fed from the tool's `-ldflags -X` var |
| `WithAllowRoot(bool)` | `false` | the tool's `--allow-root` flag |
| `WithLegacyHome(LegacyHome)` | none | an old global home to move from; `Dir` is one plain folder name |
| `WithLegacyVerify(fn)` | `nil` (none) | extra check after a cross-disk copy (db integrity); the file-count and byte check always runs first, with or without it |
| `WithConfigReader(fn)` | internal JSONC reader | how `home.jsonc` and pointer keys are read |
| `WithLayoutVersion(n)` | `1` | layout version this binary knows |
| `WithRunFile(RunFile, name)` | table in 3.2 | rename a file in `run/`; one plain file name |
| `WithSocketMaxLen(n)` | `100` | longest socket path before the fallback |
| `WithPerm(dir, file)` | `0700`, `0600` | modes for what appdir creates |
| `WithTempFallback(bool)` | `true` | disposable kinds fall back to `<tmp>/<app>-<uid>/` |
| `WithCacheTag(bool)` | `true` | `CACHEDIR.TAG` and OS backup/index exclusions |
| `WithTestGuard(bool)` | `true` | panic in a test binary on a path outside the test root |
| `WithEnv(fn)` | `os.LookupEnv` | env seam |
| `WithHomeDir(fn)` | `os.UserHomeDir` | home seam |
| `WithWorkDir(dir)` | `os.Getwd()` | working-dir seam |
| `WithBuildInfo(fn)` | `debug.ReadBuildInfo` | build-info seam |

```go
type Option func(*config)
type KindOption func(*KindSpec)

func Folder(path string) KindOption    // a folder at path inside the home
func File(path string) KindOption      // a file at path inside the home
func Commit() KindOption               // goes to git
func Ignore() KindOption               // stays out of git
func Scoped(s Scope) KindOption        // global, project, project-else-global, both
func Disposable() KindOption           // safe to delete (cache-like)
func Anchored() KindOption             // lives in the home root, no env override

type KindSpec struct {
    Name       Kind
    Path       string    // slash-separated, relative to the home
    Type       EntryType
    Git        GitRule
    Scope      Scope     // "" means: the mode's data scope
    Disposable bool
    Anchored   bool
}

type LegacyHome struct {
    Dir         string   // old folder name under the user's home, e.g. ".pm"
    DaemonLocks []string // lock files inside it to probe, default ["run/daemon.lock"]
}

type ConfigReader func(path string) (map[string]any, error) // missing file: error wrapping fs.ErrNotExist
type LegacyVerify func(src, dst string) error
```

**Option rules.** `New` checks the option set and returns `ErrInvalidOption` (message `appdir: <what>`, section 3.5) when:

- the app name does not match `^[a-z0-9][a-z0-9._-]*$`;
- the dir name (`WithDirName`), a run file name (`WithRunFile`) or a legacy folder (`LegacyHome.Dir`) is not one plain path segment: empty, `.`, `..`, or containing `/`, `\` or NUL. `WithDirName("..")` would make the parent of `$HOME` the global home, so it is refused;
- the env prefix is empty, the mode or lookup style is unknown, the layout version is below 1, or the socket length limit is below 1;
- a seam option (`WithEnv`, `WithHomeDir`, `WithWorkDir`, `WithBuildInfo`, `WithConfigReader`) was given `nil`;
- the profile list is empty, or the release or dev profile is not in it;
- a kind name does not match `^[a-z][a-z0-9-]*$`, a kind path is absolute, not slash-separated, not clean, or has an empty, `.` or `..` segment, or a kind's scope is unknown.

**Example:** A tool passes its `--dir-name` flag straight to `WithDirName` and a user types `..`. `New` returns `appdir: dir name ".." must be one plain path segment`; nothing is resolved above `$HOME`.

**Example:** the lore author keeps `data/` as the team folder and adds a `templates` kind: `appdir.WithKind(appdir.KindShared, appdir.Folder("data"))`, `appdir.WithKind("templates", appdir.Folder("templates"), appdir.Commit())`. `WithKind` on an existing name starts from that kind and changes only what the options say, so the rename keeps `shared`'s commit rule and scope.

### 3.4 Methods

```go
// Identity and names
func (d *Dirs) App() string
func (d *Dirs) EnvPrefix() string
func (d *Dirs) EnvName(suffix string) string       // "LORE_" + suffix
func (d *Dirs) KindEnv(k Kind) string              // "LORE_CACHE_DIR"; "" for anchored kinds
func (d *Dirs) Kinds() []KindSpec                  // names in effect, stable order
func (d *Dirs) Mode() Mode

// Homes
func (d *Dirs) Global() Home
func (d *Dirs) Project() (Home, error)             // ErrNoProject when none
func (d *Dirs) HasProject() bool

// Paths (reads only)
func (d *Dirs) Path(k Kind) (string, error)        // the kind in its effective home
func (d *Dirs) PathIn(k Kind, s Scope) (string, error) // ScopeGlobal or ScopeProject
func (d *Dirs) Paths() []Entry                     // every kind, for paths --json / doctor
func (d *Dirs) ConfigFiles() []ConfigFile          // read order, section 7.1

// Writes (lazy creation)
func (d *Dirs) Ensure(k Kind) (string, error)      // creates the folder (or the file's folder)
func (d *Dirs) InitProject(dir string) (string, error) // <app> init; refuses the global home
func (d *Dirs) InitHome(path string, s Scope) error    // <app> init --home X
func (d *Dirs) GitignoreBlock() string
func (d *Dirs) WriteGitignore() (bool, error)      // false when the project home is not in a git work tree
func (d *Dirs) VerifyFile(path string) error       // symlink rule (section 5.6)

// Profiles
func (d *Dirs) Profile() Profile
func (d *Dirs) IsRelease() bool
func (d *Dirs) ReleaseReason() string              // "flag", "go-install", "dev: (devel)", ...
func (d *Dirs) Name(base string) string            // base for the release profile, base-dev for dev
func (d *Dirs) NameWith(base, sep string) string   // com.org.app + "." -> com.org.app.dev
func (d *Dirs) Banner() string                     // "[lore] profile: dev → ~/.lore-dev"; "" for the release profile
func (d *Dirs) Warnings() []Warning
func (d *Dirs) CheckDB(path string) (DBAccess, error)
func (d *Dirs) ForProfile(p Profile) (*Dirs, error)
func (d *Dirs) ChildEnv(environ []string) []string // sets <APP>_PROFILE explicitly
func CleanEnv(environ []string, prefix string) []string

// Run paths (section 9)
func (d *Dirs) RunPath(f RunFile) (string, error)
func (d *Dirs) Socket() (Socket, error)
func (d *Dirs) EnsureSocketDir() (Socket, error)
func (d *Dirs) DaemonProfile() Profile
func (d *Dirs) DaemonDiscovery() (string, Profile, error)

// Legacy move
func (d *Dirs) MoveLegacy(ctx context.Context) (MoveResult, error)

// Tests
func ForTest(t testing.TB, app string) string // returns the test root
```

Result types:

```go
type Home struct {
    Scope    Scope     `json:"scope"`            // global or project
    Marker   string    `json:"marker,omitempty"` // the .<app>/ that was found (project only)
    Base     string    `json:"base"`             // folder holding home.jsonc and config files
    Data     string    `json:"data"`             // folder holding every other kind (Base, or Base/profiles/<p>)
    Rule     Rule      `json:"rule"`
    Fallback bool      `json:"fallback"`
    Note     string    `json:"note,omitempty"`
    Info     *HomeInfo `json:"info,omitempty"`   // parsed home.jsonc, nil when absent
}

type HomeInfo struct {
    App    string `json:"app"`
    Layout int    `json:"layout"`
    Scope  Scope  `json:"scope"`
}

type Entry struct {
    Kind     Kind      `json:"kind"`
    Home     Scope     `json:"home"` // global or project
    Path     string    `json:"path"`
    Type     EntryType `json:"type"`
    Git      GitRule   `json:"git"`
    Rule     Rule      `json:"rule"`
    Fallback bool      `json:"fallback"`
    Note     string    `json:"note,omitempty"`
    Error    string    `json:"error,omitempty"`
}

type ConfigFile struct {
    Path      string  `json:"path"`
    Home      Scope   `json:"home"`
    Local     bool    `json:"local"`             // config.local.jsonc
    Committed bool    `json:"committed"`         // project config.jsonc inside a git work tree
    Profile   Profile `json:"profile,omitempty"` // set on profiles/<profile>/config.local.jsonc
}

type Socket struct {
    Path        string `json:"path"`         // where the daemon listens
    Fallback    bool   `json:"fallback"`     // not under run/
    PointerFile string `json:"pointer_file"` // run/socket, which the daemon writes when Fallback
    Dir         string `json:"dir"`          // run/, or the fallback folder (0700, owner checked); "" for a Windows pipe
}

type Warning struct {
    Code    WarningCode `json:"code"`
    Message string      `json:"message"`
}

type MoveResult struct {
    Status MoveStatus `json:"status"`
    From   string     `json:"from,omitempty"` // empty when there was nothing to move from
    To     string     `json:"to,omitempty"`
    Note   string     `json:"note,omitempty"`
}
```

Warning codes (`type WarningCode string`): `profile-env-ignored`, `env-points-to-prod`, `pointer-chain-ignored`, `pointer-ignored-for-profile`, `ceiling-relative`, `ceiling-dropped`, `worktree-main-missing`.

### 3.5 Errors

Every error appdir returns wraps one of these sentinels; check it with `errors.Is`. The text is a full sentence that names the path, key or variable it is about and does not repeat the sentinel's own text. An error about a user path carries a prefix naming where the value came from: `--home: `, `LORE_HOME: `, `LORE_CACHE_DIR: ` (any `<APP>_<KIND>_DIR`), or `home in <file>: ` / `db in <file>: ` for a pointer key. Messages below use the sample paths `/home/you/work/shop` (a project) and `/home/you/.lore` (the global home).

| Error | When | Message shape |
|---|---|---|
| `ErrInvalidOption` | a bad option set in `New` (section 3.3 rules); a bad user path; a pointer value that is not a string; a bad argument to `PathIn`, `InitHome`, `RunPath`; `InitProject` on the global home (list below) | `appdir: dir name ".." must be one plain path segment` |
| `ErrUnknownProfile` | `--profile`, `<APP>_PROFILE`, `--daemon-profile` or `<APP>_DAEMON_PROFILE` names a profile not in the list | `appdir: --profile: unknown profile "devv" (known: prod, dev)` (the source is named: `LORE_PROFILE`, `--daemon-profile`, ...) |
| `ErrUnknownKind` | `Path`, `PathIn`, `Ensure` with a kind not in the layout | `appdir: unknown kind "nope"` |
| `ErrNoProject` | a project kind and no project home | `no .lore/ here; run 'lore init' or pass --home` |
| `ErrNoHome` | no user home and a global data kind; a `~` path with no user home; `Ensure` on a kind that has no path (code defaults) | `no home folder; set LORE_HOME`; `--home: "~/x" needs a home folder`; `appdir: config has no path (no home folder; code defaults)` |
| `ErrEnvVar` | `${VAR}` unset or empty in a user path | `LORE_CACHE_DIR: ${FAST} is not set`; `--home: ${NOPE} is not set`; `home in /home/you/work/shop/.lore/config.local.jsonc: ${DATA} is not set` |
| `ErrPointerAbsolute` | absolute or `${VAR}` pointer in committed `config.jsonc` | `home: "/srv/work/x" in /home/you/work/shop/.lore/config.jsonc is absolute; put it in config.local.jsonc` |
| `ErrPointerMissing` | pointer target missing (`home:`), or the folder of a `db:` target missing | `pointer points to /home/you/work/x, which does not exist; run 'lore init --home /home/you/work/x'` (`home:`); `pointer points to /home/you/work/x, which does not exist` (`db:`, naming the folder) |
| `ErrPointerNotHome` | `home:` target has no `home.jsonc` for this app; with `WithoutKind(KindHome)`, a target not named `.<app>` | `pointer target /home/you/work/x is not a lore home (no home.jsonc with app: lore)`; `pointer target /home/you/work/x is not a lore home (without home.jsonc, a home is a folder named .lore)` |
| `ErrLayoutTooNew` | `home.jsonc` layout above `WithLayoutVersion`, on a write | `home /home/you/work/shop/.lore has layout 2, this binary knows 1; upgrade lore` (names the folder holding `home.jsonc`) |
| `ErrRootOwned` | root writing into another user's home | `running as root but /home/a/.lore belongs to uid 1000; pass --allow-root` |
| `ErrSymlinkEscape` | db/config symlink pointing outside the real home, or broken | `/home/you/work/shop/.lore/lore.db links to /etc/x, outside the home /home/you/work/shop/.lore`; `/home/you/work/shop/.lore/lore.db is a broken symlink` |
| `ErrUnsafeTemp` | temp or socket fallback folder is a symlink, not a folder, or owned by someone else | `/tmp/lore-1000 is not owned by you`; `/tmp/lore-1000 is a symlink or not a folder` |
| `ErrProdDB` | dev build about to open a prod db | `dev build refuses the prod database /home/you/.lore/lore.db; set LORE_ALLOW_PROD=1 to open it read-only` |
| `ErrBusy` | legacy move lock or old daemon lock held | `old home /home/you/.pm is in use by a running daemon; will retry next run`; `another process is moving /home/you/.pm` |
| `ErrGitignoreMarkers` | `# lore:begin`/`end` missing one side, doubled or out of order | `/home/you/work/shop/.lore/.gitignore: # lore:begin / # lore:end missing, doubled or out of order; fix it by hand` |
| `ErrNotHome` | `InitHome` or `InitProject` on a folder holding another app's `home.jsonc` | `/home/you/work/x holds a home of "agx"` |
| `ErrBadHomeFile` | `InitHome` or `InitProject` on a folder whose `home.jsonc` exists but cannot be read or parsed; the reader's error is wrapped too | `init: cannot read /home/you/work/shop/.lore/home.jsonc; fix or remove it: jsonc: unexpected end of JSON input` |

`ErrInvalidOption` cases outside `New` (the `New` cases are the option rules in 3.3):

| Case | Where it shows | Message |
|---|---|---|
| `~user` in a user path | the kind or home that uses the path | `LORE_CACHE_DIR: "~bob/x": the ~user form is not supported` |
| unclosed `${` in a user path | same | `LORE_CACHE_DIR: "${FAST/x": unclosed ${` |
| a `home:` or `db:` value that is not a string | every project-capable kind (4.5) | `/home/you/work/shop/.lore/config.jsonc: home must be a string path` |
| `PathIn` scope other than global or project | `PathIn` | `appdir: PathIn needs global or project, got "both"` |
| `InitHome` scope other than global or project | `InitHome` | `appdir: InitHome needs global or project, got "both"` |
| an unknown run file | `RunPath` | `appdir: unknown run file "nope"` |
| `InitProject` where `dir/.<app>` is the global home | `InitProject` | `init: /home/you/.lore is the global home, not a project` |

**Example:** A user's shell has `LORE_CACHE_DIR='~bob/cache'`. `Path(cache)` returns `LORE_CACHE_DIR: "~bob/cache": the ~user form is not supported` and `errors.Is(err, appdir.ErrInvalidOption)` is true, so the tool reports a bad value with a line the user can act on.

Exit codes stay the tool's job (standard section 11); the sentinels map cleanly: `ErrNoProject` → 3, `ErrBusy` → 4, `ErrLayoutTooNew` → 5.

## 4. Resolution

### 4.1 Inputs

- Flags arrive as options (`WithHome`, `WithProfile`, `WithDaemonProfile`, `WithAllowRoot`). appdir never parses `os.Args`.
- Env is read through `WithEnv`. **A variable that is set but empty counts as unset**, for every `<APP>_*` variable and every `${VAR}`.
- User paths (flag, env, pointer keys in `config.local.jsonc`) may use `~`, `~/…` and `${VAR}`. `$VAR` without braces is literal. `~user` and an unclosed `${` are `ErrInvalidOption`. An unset or empty `${VAR}` is `ErrEnvVar` naming the variable. Relative paths from flag or env resolve against the working directory; relative pointers resolve against the folder holding the config file.

**Example:** You set `LORE_CACHE_DIR='${FAST}/lore'` but `FAST` is not set. lore does not write to `/lore`; `Path(cache)` returns `LORE_CACHE_DIR: ${FAST} is not set`, and `paths` shows that error on the cache line.

### 4.2 Profile first

The profile is picked before any home (section 8), because it changes both homes.

### 4.3 Global home

First match wins:

1. `WithHome(path)` (`--home`) → rule `flag`.
2. `<APP>_HOME` → rule `env-home`. In a non-release profile, a value that points into the release profile's global home is ignored with warning `env-points-to-prod` (section 8.5).
3. `~/<Name(dirName)>`: `~/.lore` for prod, `~/.lore-dev` for dev → rule `global`. We do not read `XDG_CONFIG_HOME` / `XDG_DATA_HOME` / `XDG_STATE_HOME`.
4. No `$HOME` (`os.UserHomeDir` fails) and no rule 1 or 2: the global home is unset. Global config kinds resolve to nothing (rule `defaults`: the tool uses code defaults); global data kinds fail with `ErrNoHome`; disposable kinds use the temp fallback (section 4.7).

A `--home` or `<APP>_HOME` that cannot be expanded (an unset `${VAR}`, `~user`, `~` with no user home) fails **every** kind with that error, the disposable ones included: the user named a place, so appdir never quietly writes somewhere else (no temp fallback, no code defaults).

With 1 or 2, there is **one home for everything**: the project home is the same folder, so a command never reads one place and writes another.

**Example:** `lore --home '${SCRATCH}/lore' task add x` with `SCRATCH` unset stops with `--home: ${SCRATCH} is not set`, and `lore paths` shows that error on every line, the cache too.

**Example:** `LORE_HOME=/tmp/try lore task list` in `~/projects/shop/src/api` uses `/tmp/try` for config, db and cache, and ignores `shop/.lore/`. `lore paths` shows every line with rule `env-home`.

### 4.4 Project home: lookup

Skipped when 4.3 rule 1 or 2 matched.

1. Start = `EvalSymlinks(workdir)`; if that fails, the cleaned absolute workdir. Walk the **real** path.
2. **Linked worktree check first.** Walk up from start with the stop rules below, looking only for the first folder `R` holding an entry named `.git`. If `R/.git` is a **file** whose `gitdir:` folder holds a `commondir` file, this is a linked worktree: `common = gitdir/commondir content` (relative to `gitdir`); if `base(common) == ".git"`, the main checkout is `M = dir(common)`. Translate start to `M/rel(R, start)` and run the lookup there (stop rules apply from that path). If the main checkout has no `.<app>/` on that path, run the lookup in the worktree itself; when that finds a home, add warning `worktree-main-missing` (when neither has one there is no project home and no warning). A bare main repo (common dir not named `.git`) has no checkout: use the worktree. `git` is never run. A submodule's `.git` file points to `.git/modules/x`, which has no `commondir`, so a submodule is its own project.
3. **Lookup** (style `WalkUp`): for each folder `D` from start upward:
   1. If `D/<dirName>` is a folder, and it is not the same folder (`os.SameFile`) as the global home of the release profile or of the current profile, it is the project home. Stop.
   2. If `D` holds an entry named `.git` of any kind (file, folder or symlink, even a dangling one: checked with `Lstat`), stop: a repo root is a wall.
   3. If `D` is `$HOME` or is in `<APP>_CEILING_DIRS` (`os.SameFile`), stop: a walk that starts there checks it and never climbs above it.
   4. Let `P = parent(D)`. Stop if `P == D` (filesystem root), if `P` is `$HOME` (`os.SameFile`), if `P` is in `<APP>_CEILING_DIRS` (`os.SameFile`), or if `P` is on another disk than `D` (device id; Windows: volume serial). Otherwise `D = P`.
4. Style `HereOnly`: step 2 still runs, so a linked worktree is translated to its main checkout first (standard section 1 and D2: a linked worktree uses the main checkout's home; the standard makes no exception for here-only). Then only step 3.1, for `D` = the start (the translated start in a linked worktree).
5. Nothing found: no project home. Project kinds fail with `ErrNoProject`; `project-else-global` kinds use the global home with `Fallback: true`.

Notes on the stop rules:

- The start folder is always checked, even when it is `$HOME` or a ceiling; step 3.1 then rejects the global home by `os.SameFile`, so `$HOME/.lore` is never a project home. Step 3.3 then ends the walk there, so a folder above `$HOME` or above a ceiling is never a project home either (git treats `GIT_CEILING_DIRECTORIES` the same way).
- `<APP>_CEILING_DIRS` uses the OS list separator (`:`; Windows `;`), entries get `~`/`${VAR}` expansion, and relative entries are ignored with warning `ceiling-relative` (same as git). An entry that starts with `~` or `$` but does not expand to an absolute path (`$VAR` without braces stays literal, an unset `${VAR}`, `~user`, a `${VAR}` holding a relative path) is ignored with warning `ceiling-dropped`, which names the entry and the reason, so a ceiling never silently has no effect.
- Folders are compared with `os.SameFile`, never by string, so case-insensitive disks and symlinked homes work.
- **Nearest wins:** `packages/x/.lore/` beats one at the monorepo root, because the walk stops at the first hit. A nested repo is a wall because 3.2 stops at its `.git`.
- Step 2 (the worktree check) walks with the same stop rules, so it never finds a `.git` above `$HOME` or a ceiling either.

**Example:** You run lore in `~/projects/shop/src/api`. lore checks `src/api`, then `src`, then `shop`, finds `shop/.lore/` and uses it (rule `project`). If `shop` had no `.lore/`, lore would stop there anyway, because `shop` holds `.git`; it never climbs into `~/projects`.

**Example:** On a server your projects live in `/srv/work`, which is not under `$HOME`, and `/srv` is its own disk. You run lore in `/srv/work/notes` (no git). The walk stops at the disk change below `/srv`, or at `LORE_CEILING_DIRS=/srv/work` if you set it, instead of reaching `/`.

**Example:** You run `lore` in `/home/you` itself, and someone left a `/home/.lore` folder. The walk checks `/home/you` (its `.lore` is your global home, so it is skipped) and stops there; `/home/.lore` is never used, and `lore task add` says `no .lore/ here; run 'lore init' or pass --home`.

**Example:** A symlink `.git` in `vendor/tool` points at a folder that was deleted. It still marks `vendor/tool` as a repo root, so running lore in `vendor/tool/src` stops at `vendor/tool` instead of climbing into the parent project.

**Example:** You run `git worktree add ../shop-fix` and run lore in `~/projects/shop-fix/src`. lore reads `shop-fix/.git` (`gitdir: /…/shop/.git/worktrees/shop-fix`), then its `commondir` (`../..`), so the main checkout is `~/projects/shop`. It uses `~/projects/shop/.lore/` (rule `worktree`). Your tasks are the same in both folders, also for a tool that uses `HereOnly`.

**Example:** You add a worktree `../wiki-fix` of a repo `wiki` that has no `.lore/` anywhere. lore finds no project home and prints only `no .lore/ here; run 'lore init' or pass --home`, with no `worktree-main-missing` warning, because there is no worktree home to fall back to.

### 4.5 Project home: pointer keys

After a project home `.<app>/` (the marker) is found, appdir reads `home` and `db` from the marker's `config.jsonc`, then `config.local.jsonc` (local wins per key), through the config reader. A key that is absent, `null` or `""` counts as not set, so an empty local value does not clear a committed one.

**Example:** The committed `config.jsonc` says `"home": "../../shop-private/lore"` and you write `"home": ""` in your `config.local.jsonc` to turn it off. It does not: the empty value counts as not set and the committed pointer still applies. Pass `--home` to work elsewhere on this machine.

- In `config.jsonc` (committed), a value must be relative (to the marker folder) or start with `~/`. An absolute path, a drive or UNC path, or a value starting with `$` is `ErrPointerAbsolute`. In `config.local.jsonc`, any user path is fine (`~`, `${VAR}`, absolute, relative).
- `home: X`: X must exist (`ErrPointerMissing`) and hold a `home.jsonc` whose `app` is this app (`ErrPointerNotHome`). When the author dropped the home kind (`WithoutKind(KindHome)`), no `home.jsonc` is ever written, so that check would refuse every target; X must then be a folder named `.<app>` (the `WithDirName` name), the same thing the lookup accepts as a project home without `home.jsonc`, else `ErrPointerNotHome`. Why this rule: it is the safest one that still refuses a random folder or a typo. Accepting any existing folder would let a pointer turn an arbitrary folder into a home; writing `home.jsonc` anyway as a hidden marker would contradict the author's choice and, being outside the commit list, would not reach teammates through git; recognising other layout files (`config.jsonc`) is too weak, since unrelated folders hold such files. The name is the one marker every home of this app has without `home.jsonc`. X becomes the project home's `Base` (rule `pointer`). One hop only: X's own `home`/`db` keys are not followed (warning `pointer-chain-ignored`).
- `db: F`: the folder of F must exist (`ErrPointerMissing`); F itself may not exist yet (first run creates it). Only the `db` kind moves (rule `pointer`). `backups/` stays in the home that owns the marker (standard section 7).
- A pointer error (including a `home:`/`db:` value that is not a string, `ErrInvalidOption`) makes every project-capable kind fail with that error: project kinds, `project-else-global` kinds (`logs`) and the project side of `both` kinds (`config`, `config-local`, `home`). It never falls back to the marker itself or to the global home. `Project()` returns the error, `HasProject()` is false, `Paths` shows it on each of those lines, and `ConfigFiles` still lists the marker's own `config.jsonc` and `config.local.jsonc`, which hold the key the user has to fix. Global kinds (the cache) and the global side of `both` kinds are unaffected.
- In a non-release profile, `home:` resolves to `X/profiles/<profile>` and `db:` is ignored (warning `pointer-ignored-for-profile`, naming the path the profile's database really uses), so dev data never lands next to the prod db the pointer names.

**Example:** `shop/.lore/config.jsonc` says `{ "home": "../../shop-private/lore" }`. On your laptop (`/home/you/work/shop`) and on the server (`/srv/work/shop`) the private repo sits next to `shop`, so lore uses `shop-private/lore/` on both; nothing new appears in `shop`. `lore paths` shows rule `pointer`.

**Example:** You write `{ "home": "/home/you/work/shop-private/lore" }` in the committed file. lore stops with `home: "/home/you/work/shop-private/lore" in /home/you/work/shop/.lore/config.jsonc is absolute; put it in config.local.jsonc`. You move it to `config.local.jsonc` and it works on your laptop only, as intended; the server keeps using the relative pointer.

**Example:** The private repo was not cloned yet, so `home: ../../shop-private/lore` points nowhere. `lore paths` shows `pointer points to /home/you/work/shop-private/lore, which does not exist; run 'lore init --home /home/you/work/shop-private/lore'` on the db, logs and project config lines; the cache line is fine. Nothing is written to `~/.lore` in the meantime.

**Example:** Your dev build runs in `shop`, whose `config.local.jsonc` has a `db:` pointer. It warns `db: pointer in /home/you/work/shop/.lore/config.local.jsonc ignored for profile dev; the dev database stays at /home/you/work/shop/.lore/profiles/dev/lore.db`.

### 4.6 Profile folders

- Global home of a non-release profile: `~/<Name(dirName)>` (`~/.lore-dev`).
- Project home of a non-release profile: `Base` stays `.<app>/` (so `home.jsonc`, the committed `config.jsonc`, `config.local.jsonc` and `.gitignore` are shared and a dev build reads the team settings like prod), `Data` is `.<app>/profiles/<profile>/`. Every non-anchored kind lives under `Data`. `profiles/` is ignored by the gitignore block (owner, 2026-11-10).
- That profile may add its own `.<app>/profiles/<profile>/config.local.jsonc`, read after both shared files so it wins (section 6.1, owner, 2026-11-10).
- `--home` / `<APP>_HOME`: used as given for every profile. The user named the folder.

**Example:** You run your dev build of lore in `shop`. It reads the team settings from `shop/.lore/config.jsonc` like prod, but its database is `shop/.lore/profiles/dev/lore.db`. The prod `shop/.lore/lore.db` is never opened.

### 4.7 Per-kind result

For kind `k` with scope `s` (section 5.2), in this order:

1. `<APP>_<KIND>_DIR` (not for anchored kinds) → the folder for `k`; a file kind keeps its file name inside it. Rule `env-kind`. Leak filter as in 4.3.
2. `db` only: a `db:` pointer → rule `pointer`.
3. The home picked by `s`: `project` → project `Data` (or `ErrNoProject`); `global` → global `Data` (or `ErrNoHome`); `project-else-global` → project, else global with `Fallback: true`; `both` → for `Path`, the project home if there is one, else global; `Paths` lists both. A project that was found but is broken (a pointer error, 4.5) is not "no project": `project-else-global` and `both` kinds fail with its error. Under `--home`/`<APP>_HOME` there is one home: `Paths` lists a `both` kind once, labelled `home: global` (like `ConfigFiles`).
4. Disposable kinds with no `$HOME` and no `--home`/`<APP>_HOME`: `<tmp>/<Name(app)>-<uid>/<path>` (rule `temp`, `Fallback: true`). A broken override never gets here (4.3). With `WithTempFallback(true)`, `Ensure` also falls back there when creating the folder fails with a read-only or permission error.

`<uid>` is the numeric uid on Unix and the user name on Windows (`%TEMP%` is per user there).

**Example:** Your laptop disk is nearly full, so you set `LORE_CACHE_DIR=/mnt/fast/lore-cache`. `lore paths` shows `cache  /mnt/fast/lore-cache  env-kind`; everything else stays in `~/.lore`.

**Example:** You run `lore --home /tmp/try paths`. The config line reads `config  global  /tmp/try/config.jsonc  flag`, once; the db line reads `db  project  /tmp/try/lore.db  flag`. Both are the same folder.

**Example:** A CI container has no `$HOME`. `lore version` works. `lore paths` shows global config as `defaults`, cache as `/tmp/lore-1001/cache (temp, fallback)`, and the global db line says `no home folder; set LORE_HOME`.

### 4.8 Writing: sudo, root, read-only

These checks run in every write method, never in reads:

- **Root.** If the effective uid is 0 and the nearest existing folder of the target home is owned by another uid, refuse with `ErrRootOwned` unless `WithAllowRoot(true)`. Windows has no equivalent (an elevated process is the same user); the check is a no-op there.
- **Layout.** If the home's `home.jsonc` has `layout` above `WithLayoutVersion`, refuse with `ErrLayoutTooNew`. The message names the folder `home.jsonc` was read from (the home's `Base`), also when the write goes to a profile's data folder or a `db:` pointer.
- **Read-only home.** A failing `mkdir` returns the OS error wrapped with the path and a hint (`set <APP>_HOME or <APP>_<KIND>_DIR`). Disposable kinds fall back to temp first (4.7).

**Example:** You run `sudo lore task add x` on Linux where sudo keeps `HOME=/home/you`. lore sees euid 0 and `/home/you/.lore` owned by uid 1000, and prints `running as root but /home/you/.lore belongs to uid 1000; pass --allow-root`. Your next normal run does not hit root-owned files.

**Example:** A newer lore wrote `layout: 2` into `shop/.lore/home.jsonc`. Your older dev build calls `Ensure(db)` for `shop/.lore/profiles/dev/` and stops with `home /home/you/work/shop/.lore has layout 2, this binary knows 1; upgrade lore`, naming the folder that holds `home.jsonc`.

## 5. Layout

### 5.1 Standard kinds

| Kind | Path | Type | Git | Scope | Flags |
|---|---|---|---|---|---|
| `home` | `home.jsonc` | file | commit | both | anchored |
| `config` | `config.jsonc` | file | commit | both | anchored |
| `config-local` | `config.local.jsonc` | file | ignore | both | anchored |
| `gitignore` | `.gitignore` | file | commit | project | anchored |
| `shared` | `shared/` | folder | commit | data scope | |
| `db` | `<app>.db` | file | ignore | data scope | |
| `backups` | `backups/` | folder | ignore | data scope | |
| `cache` | `cache/` | folder | ignore | global | disposable |
| `run` | `run/` | folder | ignore | data scope | |
| `logs` | `logs/` | folder | ignore | project-else-global (project mode), global (global mode) | |

```filetree
.lore/
  home.jsonc            app, layout, scope (committed)
  config.jsonc          settings (committed)
  config.local.jsonc    this machine only
  .gitignore            written by appdir between markers
  shared/               team-shared files (committed)
  lore.db               the database
  backups/
  run/                  lock, daemon.lock, lore.pid, discovery.json, socket, lore.sock
  logs/
  profiles/
    dev/                the dev profile's data, same layout minus anchored files
```

### 5.2 Mode and scope

- **Data scope** = `project` for `ModeProject` (default), `global` for `ModeGlobal`. Kinds with an empty scope use it.
- `ModeProject` is the default because it fails safe: a global tool that forgets `WithMode(ModeGlobal)` gets a loud `ErrNoProject` outside projects, while the opposite mistake would silently write project data into `~/.<app>` (review A3).
- Identity and login data are always global: an author adds them as `WithKind("identity", appdir.File("identity.json"), appdir.Scoped(appdir.ScopeGlobal))`.

**Example:** agx is a global tool: `appdir.New("agx", appdir.WithMode(appdir.ModeGlobal))`. Its db is `~/.agx/agx.db` wherever you run it. lore keeps the default, so `lore task add` in `~/Downloads` stops with `no .lore/ here; run 'lore init' or pass --home`.

### 5.3 Author-declared kinds

- `WithKind(name, opts...)`: a new name starts as `Folder(name)`, `Ignore()`, data scope. An existing name is changed in place. Kind names match `^[a-z][a-z0-9-]*$`; paths are slash-separated, relative, and may not contain `..`.
- `WithoutKind(name)` drops it from `Paths`, the gitignore block and `Ensure`.
- Every non-anchored kind gets `<APP>_<KIND>_DIR` (kind name upper-cased, `-` → `_`): `templates` → `LORE_TEMPLATES_DIR`. Anchored kinds get none: moving `config.jsonc` away from `home.jsonc` would split a home from its own marker; `--home` moves them.
- Reserved env names (no kind may produce them): `<APP>_HOME`, `<APP>_PROFILE`, `<APP>_ALLOW_PROD`, `<APP>_CEILING_DIRS`, `<APP>_DAEMON_PROFILE`. A kind named `home`, `profile`, … would only produce `<APP>_HOME_DIR`, which does not clash, so no extra check is needed.

**Example:** The lore author adds `templates`. With no more code, `lore paths` lists `shop/.lore/templates`, `LORE_TEMPLATES_DIR=~/my-templates` moves it, and it is committed like `shared/`.

### 5.4 Permissions

- `Ensure` and `Init*` create folders with `0700` and files with `0600` (`WithPerm`). Existing permissions are left alone.
- Windows: the same calls set a protected DACL granting full access to the current user's SID and SYSTEM only, inheritance off (`D:P(A;OICI;FA;;;<user-sid>)(A;OICI;FA;;;SY)`).

### 5.5 Cache tagging

On creating a disposable folder (`WithCacheTag(true)`):

- All OSes: `CACHEDIR.TAG` with the standard signature line `Signature: 8a477f597d28d172789f06886806bc55`.
- macOS: an empty `.metadata_never_index`, and the Time Machine exclusion via `tmutil addexclusion <dir>` (best effort; the stdlib has no `setxattr` on darwin, so this is the one place appdir runs a command).
- Windows: the `FILE_ATTRIBUTE_NOT_CONTENT_INDEXED` attribute.

**Example:** lore creates `~/.lore/cache/` on first use. restic and borg skip it because of `CACHEDIR.TAG`, Spotlight stops indexing it, and Time Machine leaves it out of your backup.

### 5.6 Symlinks

- A home folder may be a symlink (repolink makes these).
- A file kind (the db, the config files) may be a symlink only if its real path is inside the real path of the folder it belongs to. `VerifyFile(path)` checks it; `Ensure` calls it for file kinds that already exist, and `CheckDB` calls it. Otherwise `ErrSymlinkEscape`.
- Windows: junctions count as links (Go's `EvalSymlinks` follows them). appdir never creates links.

**Example:** repolink links `shop/.lore` to `~/private/shop-lore`. lore follows it fine. Someone replaces `lore.db` with a link to `/etc/x`: `Ensure(db)` fails with `shop/.lore/lore.db links outside the home`.

### 5.7 `.gitignore` block

`GitignoreBlock()` builds, from the kinds and `WithGitCommit`/`WithGitIgnore`:

```
# lore:begin
*
!.gitignore
!home.jsonc
!config.jsonc
!shared/
!shared/**
# lore:end
```

- Start with `*` (anything not listed is ignored, so a new file never reaches git by accident).
- For each committed file kind `p`: re-include every parent folder (`!a/`, `!a/b/`), then `!p`.
- For each committed folder kind `p`: parents, then `!p/` and `!p/**`.
- For each `WithGitCommit` pattern: parents of its fixed prefix, then the pattern.
- `WithGitIgnore` patterns win: one that equals a commit entry (a committed kind or a `WithGitCommit` pattern) removes that entry and is not written again, because `*` already ignores it; any other is written after the re-includes as a plain line, so it wins in git (it re-excludes something inside a committed folder, or a folder that is only re-included as the parent of a committed path).
- `WriteGitignore` writes the block into the project home's `Base/.gitignore` only when the home is inside a git work tree (an entry named `.git` in it or above it). It replaces the text between the markers and never touches other lines; with no markers it appends the block. A missing, doubled or out-of-order marker is `ErrGitignoreMarkers`; appdir never guesses.

**Example:** Next month lore starts writing `stats.json` in `.lore/`. It is not on the commit list, so `*` keeps it out of git and Erman never gets your stats in a pull.

**Example:** The lore author passes `WithGitIgnore("shared")` to keep `shared/` private. The block loses `!shared/` and `!shared/**` and gains no `shared` line: `*` already ignores it.

### 5.8 `home.jsonc`

`InitProject` and `InitHome` write `{"app": "<app>", "layout": <n>, "scope": "project|global"}` (atomic: temp file in the same folder, fsync, rename; Windows retries a sharing violation a few times). `New` reads it when present and exposes it as `Home.Info`.

A relative `dir` in `InitProject` resolves against the working folder `New` used (`WithWorkDir`), like every other user path. `Init*` refuses a `home.jsonc` that exists but cannot be read with `ErrBadHomeFile` ("fix or remove it", naming the file once) instead of replacing the user's file, creates the parent folders of a home kind renamed to a nested path (`meta/home.json`), and, when the author dropped the home kind (`WithoutKind(KindHome)`), only creates the folder.

`InitProject(dir)` refuses (`ErrInvalidOption`) when `dir/.<app>` is the global home of the release profile or of the current profile. It compares by identity, and before `~/.<app>` exists by its parent folder's identity and its name (case-insensitive on macOS and Windows), so `lore init` in `$HOME` never turns the future global home into a project.

**Example:** On a new machine you run `lore init` in `/home/you` before lore ever ran. lore stops with `init: /home/you/.lore is the global home, not a project`; `~/.lore` is created later by the first global write, with `scope: global`.

## 6. Scopes, config files, pointer reading

### 6.1 Config files

`ConfigFiles()` returns the read order of section 3 of the standard, later wins per key:

1. global `config.jsonc`
2. global `config.local.jsonc`
3. project marker `config.jsonc` (Committed when inside a git work tree)
4. project marker `config.local.jsonc`
5. with a `home:` pointer: the target's `config.jsonc`, then `config.local.jsonc`
6. non-release profile only: `.<app>/profiles/<profile>/config.local.jsonc` (the project's `Data` folder, so with a `home:` pointer it is `<target>/profiles/<profile>/config.local.jsonc`), marked with `ConfigFile.Profile`. It is gitignored with the rest of `profiles/` and wins over both shared files (owner, 2026-11-10).

Missing files are listed (the loader treats them as empty). With no `$HOME`, 1–2 are left out. With `--home`/`<APP>_HOME`, the one home's two files are listed once, with `Home: global`. With a broken pointer (4.5), 3–4 are still listed (they hold the key to fix) and 5–6 are not. A config kind the author dropped (`WithoutKind(KindConfig)` or `KindConfigLocal`) is left out of every home's pair. Env and flags come after, in cfgkit.

**Example:** `shop/.lore/config.jsonc` says port 3000, your `config.local.jsonc` says 4000. `ConfigFiles()` lists both in that order; cfgkit merges them and you get 4000 while Erman keeps 3000.

**Example:** Your dev build of lore must not take port 4000 from the installed one. You write `{ "port": 4100 }` in `shop/.lore/profiles/dev/config.local.jsonc`. The dev build reads `config.jsonc` (3000), `config.local.jsonc` (4000), then the dev file (4100) and uses 4100; the release build never lists the dev file and keeps 4000. Git ignores the dev file like everything under `profiles/`.

### 6.2 What appdir does not decide

Global-only settings, secret policy and unknown keys are per setting, so they belong to the config loader. `ConfigFile.Home` and `ConfigFile.Committed` give it what it needs to apply them.

### 6.3 Config reader seam

`WithConfigReader(fn)` reads `home.jsonc` and pointer keys. The default parses JSONC with `internal/jsonclite`: it drops `//` and `/* */` comments and trailing commas, then calls `encoding/json`. It walks strings correctly (a `//` inside a string is not a comment; escapes are honoured) and fails on an unterminated comment or string, on a backslash followed by a raw newline inside a string, and on a top level that is not an object (`null` included). **Replaced by `ubgo/jsonc` when it exists.**

## 7. Run paths (section 9, paths only)

- `RunPath(f)`: `run/lock`, `run/daemon.lock`, `run/<app>.pid`, `run/discovery.json`, `run/<app>.sock`, `run/socket` inside the data home of `run`'s scope. Names change with `WithRunFile`.
- `Socket()`: Unix: `run/<app>.sock` when its length is at most `WithSocketMaxLen` (100 bytes; macOS allows about 104). Longer: `<dir>/<hash>.sock` with `<dir>` = `$XDG_RUNTIME_DIR/<Name(app)>/` on Linux when set, else `/tmp/<Name(app)>-<uid>/`, and `<hash>` = first 12 hex of SHA-256 of the real path of `run/` (unique per home and profile). `Fallback: true`, and the daemon writes the path to `run/socket`. Windows: always `\\.\pipe\<Name(app)>-<user>-<hash>`, `Fallback: true` (it is not under `run/`).
- `EnsureSocketDir()` creates `run/` in every case (through `Ensure`, with its checks), because the daemon writes `run/socket`, the pid and `discovery.json` there even when the socket itself lives elsewhere. When the socket falls back on Unix it also creates the fallback folder `0700` and refuses it (`ErrUnsafeTemp`) if it is a symlink, not a folder, or owned by another uid. A Windows pipe has no folder, so only `run/` is created.
- `DaemonProfile()`: `WithDaemonProfile` (flag), then `<APP>_DAEMON_PROFILE`, then the own profile. Unknown name: `ErrUnknownProfile` from `New`.
- `DaemonDiscovery()`: the discovery file of the daemon profile, computed with `ForProfile`. The client still only talks to the daemon; appdir only gives the path.

**Example:** Your project is `/home/you/work/clients/acme-industries/platform-services/dev-tools/agent-context` (80 bytes), so `.runbay/run/runbay.sock` inside it is 104 bytes. appdir returns `/tmp/runbay-1000/3f9a1c0b2d4e.sock` and `run/socket` as the pointer file; `EnsureSocketDir()` creates both `/tmp/runbay-1000/` and the project's `.runbay/run/`, the daemon writes the socket path to `run/socket`, and clients read it and connect.

**Example:** You start your dev runbay with `--daemon-profile prod`. `DaemonDiscovery()` returns `~/.runbay/run/discovery.json` and profile `prod`; your dev app connects through it and prints the one line the standard asks for.

## 8. Profiles

### 8.1 Detection

1. `WithProfile` (`--profile`): must be in the list, else `ErrUnknownProfile`.
2. `<APP>_PROFILE`: must be in the list, else `ErrUnknownProfile` (never a fallback to prod). Two limits: a **dev build** ignores the release profile from env (warning `profile-env-ignored`) unless `<APP>_ALLOW_PROD=1`; a **release build** ignores any other profile from env (use the flag).
3. The build: release → `WithReleaseProfile` (`prod`); anything else → `WithDevProfile` (`dev`).

### 8.2 Release detection (stdlib only)

A build is a release when:

- `WithRelease(true)` was passed (the tool feeds it from its own `-ldflags -X main.release=true` var), or
- `debug.ReadBuildInfo()` shows a `Main.Version` that starts with `v`, is not `(devel)`, is not a pseudo-version (`vX.Y.Z-0.yyyymmddhhmmss-abcdef123456` and the `-pre.0.` / `.0.` forms), has no `+dirty`, **and** there is no `vcs.revision` setting. That is what `go install module@vX` produces from the module cache; a build in a checkout always has `vcs.revision`.

`WithRelease(false)` forces dev. Everything else is dev, so a forgotten flag fails safe. `ReleaseReason()` says which rule decided (`flag`, `go-install`, `dev: flag`, `dev: no build info`, `dev: (devel)`, `dev: version without v prefix`, `dev: vcs.revision`, `dev: pseudo-version`, `dev: +dirty`). When `ubgo/buildinfo` gains `IsRelease()`, tools may pass it through `WithRelease`.

**Example:** A teammate runs `go install github.com/ubgo/lore@latest` and gets `v0.13.1` with no vcs lines: prod, `~/.lore`. You run `go install ./cmd/lore` on the `v0.13.1` tag in your checkout: build info has `vcs.revision`, so it is dev with `~/.lore-dev`.

### 8.3 Names

`Name(base)` returns `base` for the release profile and `base-<profile>` otherwise; `NameWith(base, ".")` gives `com.org.app.dev`. Use it for every label, unit, marker, pipe and item name. `Banner()` returns `[<app>] profile: dev → <global home>` for non-release profiles and `""` for prod; the tool prints it to stderr.

### 8.4 Prod-db guard

`CheckDB(path)` before opening and before any migration. A relative `path` resolves against the working folder `New` used (`WithWorkDir`), so the guard checks the file the tool opens:

- Release build: `AccessReadWrite`.
- Dev build: compute the prod db paths for this machine as an installed release build would see them: the same options, the release profile, no `--home`, and the env with **every `<APP>_*` variable removed** (the overrides: `<APP>_HOME`, `<APP>_<KIND>_DIR`, `<APP>_PROFILE`, ...). Other variables are kept, because a prod `db:` pointer in `config.local.jsonc` may use `${VAR}`. Both candidates are checked: the global home db and, from the same working dir, the project db including a `db:`/`home:` pointer; a project view that fails (for example a broken pointer) does not skip the global one. Compare with `path` by `os.SameFile`; when a file does not exist yet, compare its real parent folder (`os.SameFile`) and file name (case-insensitive on macOS and Windows); when the parent folder does not exist yet either (the prod home before the installed build ever ran), compare the real paths as far as they exist. `VerifyFile(path)` runs first, for every build.
- Match and no `<APP>_ALLOW_PROD=1`: `ErrProdDB`. Match with it: `AccessReadOnly` (the caller opens read-only and never migrates). No match: `AccessReadWrite`.

**Example:** You run your dev lore with `--home ~/.lore` to check something (a flag is explicit, so the env-leak filter of 8.5 does not apply; `LORE_HOME=~/.lore` would simply be ignored for the dev profile). The dev build's db is now `~/.lore/lore.db`, and `CheckDB(~/.lore/lore.db)` matches the prod db and returns `dev build refuses the prod database /home/you/.lore/lore.db; set LORE_ALLOW_PROD=1 to open it read-only`. That is the runbay 2026-10-11 incident, closed.

**Example:** Your `shop/.lore/config.local.jsonc` says `{ "db": "${DATA}/shop.db" }` and your shell has `DATA=/srv/data`. Your dev build calls `CheckDB("/srv/data/shop.db")`: the prod view keeps `DATA`, follows the pointer, matches, and refuses with `ErrProdDB`.

**Example:** On a fresh machine the installed lore never ran, so `~/.lore` does not exist. Your dev build calls `CheckDB("/home/you/.lore/lore.db")`. Neither the file nor its folder exists, so the real paths are compared; they match and the dev build is refused instead of creating the prod db.

### 8.5 Env leak

- In a non-release profile, `<APP>_HOME` and `<APP>_<KIND>_DIR` values inside the release profile's global home (empty-env resolution) are ignored with warning `env-points-to-prod`.
- `ChildEnv(environ)` returns `environ` with `<APP>_PROFILE=<profile>` set (replacing any value), for every child process the tool starts.
- `CleanEnv(environ, prefix)` returns `environ` without any `<PREFIX>_*` variable: the Go twin of runbay's `dev-env.sh` for dev task wrappers. It is a pure function.

**Example:** A terminal opened by the installed runbay carries `RUNBAY_HOME=~/.runbay`. You start your dev build there. It prints `warning: RUNBAY_HOME points into the prod home /home/you/.runbay; ignored for profile dev` and uses `~/.runbay-dev`.

### 8.6 Another profile

`ForProfile(p)` resolves the same app with the same options and env, as if `--profile p` were given. It is how a tool computes another profile's paths (discovery file, `doctor` comparisons) without opening that profile's files.

## 9. Legacy move (section 8)

`WithLegacyHome(LegacyHome{Dir: ".pm"})` and a call to `MoveLegacy(ctx)` at startup. Steps:

1. Skip (`MoveSkipped`) when the profile is not the release profile, or the global home came from `--home`/`<APP>_HOME`. Only the global home is ever moved; committed project folders never are (they change through an explicit `<app> migrate-layout`).
2. `old = ~/<Dir>`, `new` = the global home. If old does not exist: `MoveNone`. If old holds only `MOVED.txt`: `MoveAlready`. If both exist: `MoveBothExist`, new is used, nothing moves, and `MoveResult.Note` says so for `doctor`.
3. Take `<parent>/<base(new)>.moving.lock` with the OS lock (non-blocking). Held → `ErrBusy`. The lock file is never deleted.
4. Re-check step 2 under the lock (another process may have finished).
5. Probe each `DaemonLocks` file in old that exists (never create one). Held → `ErrBusy` ("try again next run").
6. Same disk (device id): `rename(old, new)`.
7. Other disk: copy old into `<new>.moving-tmp` (old itself followed when it is a symlink, so a `~/.<old>` linked to another disk moves its contents, not the link; inside it files, folders, symlinks as links, modes kept; sockets and pipes skipped), write `.legacy-move` (old path) into it, verify file count and bytes plus `WithLegacyVerify`, rename it to new, rename old to `<old>.moved-<YYYYMMDD>`, then remove `.legacy-move`.
8. Recreate old as a folder holding only `MOVED.txt` ("moved to <new>"). Never delete user data.

Crash safety: a same-disk rename is atomic. A crash during a cross-disk copy leaves `<new>.moving-tmp`, which the next run deletes (it is our copy; old is intact) before starting again. A crash after `tmp → new` leaves `.legacy-move` in new; the next run sees it and finishes steps 7–8 instead of reporting both-exist. A crash after old was renamed to `<old>.moved-<date>` leaves no old folder and `.legacy-move` (holding the old path) still in new; the next run matches that path, removes the marker and writes `MOVED.txt` (`MoveDone`) instead of reporting `MoveNone` forever.

**Lock choice:** appdir uses a minimal internal lock (`internal/flock`: `flock(LOCK_EX|LOCK_NB)` on Unix, `LockFileEx` with `LOCKFILE_FAIL_IMMEDIATELY` on Windows) and not `ubgo/lock/flock` v0.1.0. That package fits the kernel semantics, but it writes a `.fence` sidecar next to every lock (a second stray file in `$HOME`), creates with `0644`/`0755` instead of `0600`/`0700`, and opens with `O_CREATE`, while probing an old daemon's lock must never create a file in the old home.

**Example:** runbay used to live in `~/.pm`. The new runbay starts, finds `~/.pm` and no `~/.runbay`, takes `~/.runbay.moving.lock`, sees the old daemon lock is free, renames `~/.pm` to `~/.runbay`, and leaves `~/.pm/MOVED.txt`. If the old daemon were still running, `MoveLegacy` would return `ErrBusy`; runbay prints `old home /home/you/.pm is in use by a running daemon; will retry next run`, moves nothing, and tries again on the next start.

**Example:** The machine loses power during a cross-disk move, right after `~/.pm` became `~/.pm.moved-20261110`. On the next start `MoveLegacy` finds `~/.runbay/.legacy-move` naming `~/.pm`, removes it, writes `~/.pm/MOVED.txt` and returns `MoveDone`.

## 10. Tests

`ForTest(t, app)` makes a temp root and, with `t.Setenv`, points `HOME`, `USERPROFILE`, `<APP>_HOME`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`, `XDG_RUNTIME_DIR`, `CLAUDE_CONFIG_DIR`, `CODEX_HOME` and `TMPDIR` into it, unsets every other `<APP>_*` variable (`<APP>_PROFILE`, `<APP>_ALLOW_PROD`, `<APP>_DAEMON_PROFILE`, `<APP>_CEILING_DIRS`, `<APP>_<KIND>_DIR`, ...), `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_COMMON_DIR`, `GIT_CEILING_DIRECTORIES` and every `GIT_CONFIG*`, and sets `APPDIR_TEST_ROOT`.

In a test binary (`testing.Testing()`), `New` panics when any resolved path is outside `APPDIR_TEST_ROOT` (or `os.TempDir()` when unset). `Socket()` and `EnsureSocketDir()` check the socket fallback folder (`/tmp/<app>-<uid>/` or `$XDG_RUNTIME_DIR/<app>/`, resolved after `New`) the same way, before anything can create it. When the root is the default `os.TempDir()` (which is `/tmp` itself with `TMPDIR` unset), they also refuse the shared `/tmp/<app>-<uid>/` folder even though it is inside that root: a live daemon's socket sits there, so it is the real user's, not the test's. The temp fallback of disposable kinds may be the same folder then and stays allowed, since it holds only disposable files. `WithTestGuard(false)` turns it off.

**Example:** lore's test forgets to isolate and runs in a shell with `LORE_HOME=~/.lore`. `New` panics with `appdir: test binary resolved /home/you/.lore outside the test root /tmp; call appdir.ForTest(t, "lore") or pass WithTestGuard(false)`, instead of the test writing into your real notes.

**Example:** A daemon test under `ForTest` uses a deep temp project on macOS, so its socket falls back to `/tmp/lore-1000/`, outside the test root. `Socket()` panics before `EnsureSocketDir()` can create that folder in the real `/tmp`; the test raises `WithSocketMaxLen` or uses a shorter project path.

## 11. JSON for `paths --json`

`Paths()` returns entries in kind order, global before project for `both` kinds (under `--home`/`<APP>_HOME`, a `both` kind appears once, with `home: global`); `json.Marshal` gives the standard's `kind, path, rule, fallback` plus `home, type, git, note, error`. The tool wraps it in its envelope (`voltkit/output`).

## 12. Platform table mapped to code

| Standard rule | macOS / Linux | Windows | File / build tag |
|---|---|---|---|
| Global home | `~/.<app>` | `%USERPROFILE%\.<app>` | `resolve.go` (`os.UserHomeDir` covers both) |
| Owner-only 0700/0600 | mode bits | protected DACL (user SID + SYSTEM) | `perm_unix.go` (`unix`), `perm_windows.go` (`windows`) |
| Stop at a disk change | `Stat_t.Dev` | volume serial (`GetFileInformationByHandle`) | `sys_unix.go`, `sys_windows.go` |
| Locks | `flock` | `LockFileEx` | `internal/flock/flock_unix.go`, `flock_windows.go` |
| Atomic write (`home.jsonc`) | temp, fsync, rename | temp, flush, rename with retry on sharing violation | `atomic.go`, `rename_windows.go`, `rename_other.go` |
| Socket | `run/` or `/tmp/<app>-<uid>/`, `$XDG_RUNTIME_DIR` on Linux | `\\.\pipe\<app>-<user>-<hash>` | `run.go`, `socket_unix.go`, `socket_windows.go` |
| Symlinks | normal | junctions followed by `EvalSymlinks`; appdir never makes links | `symlink.go` |
| Long paths | n/a | Go's `os` adds `\\?\` to absolute paths; appdir only passes absolute paths | `resolve.go` |
| Cache exclusions | `CACHEDIR.TAG`; macOS `.metadata_never_index` + `tmutil` | `CACHEDIR.TAG` + not-content-indexed | `cachetag.go`, `cachetag_darwin.go`, `cachetag_windows.go`, `cachetag_other.go` |
| Root / sudo | euid 0 vs owner uid | no-op | `sys_unix.go`, `sys_windows.go` |
| Temp fallback owner check | uid + `Lstat` | `%TEMP%` is per user; `Lstat` only | `sys_unix.go`, `sys_windows.go` |
| Network/synced folder check | — | — | sqlitedb (section 6), not appdir |
| Service install | — | — | the tool (`Name`/`NameWith` give the labels) |

## 13. Traceability

Every appdir rule of `app-standard.md` sections 1–4, 8, 10 (plus the path parts of 9, the platform table, section 13) and every appdir review item → where this spec covers it.

| Source | Rule | Spec |
|---|---|---|
| Platform table | global home per OS | 4.3, 12 |
| Platform table | owner-only folders/files, ACL on Windows | 5.4, 12 |
| Platform table | stop at disk change, volume serial | 4.4, 12 |
| Platform table | locks flock / LockFileEx | 9, 12 |
| Platform table | atomic write with Windows retry | 5.8, 12 |
| Platform table | socket / named pipe | 7, 12 |
| Platform table | symlinks, junctions | 5.6, 12 |
| Platform table | long paths | 12 |
| Platform table | cache exclusions | 5.5, 12 |
| Platform table | network folder check | out: sqlitedb (section 6 of the standard) |
| Platform table | service install | out: the tool; names via 8.3 |
| Rule zero | every default is an option, two levels | 2, 3.3 |
| Rule zero | renamed names, no warning; `paths --json` shows names | 2, 3.4 `Kinds`, 11 |
| §1 | order flag → env → project (+pointer) → global | 4.3, 4.4, 4.5 |
| §1 | no XDG | 4.3 |
| §1 | lookup style option walk-up / here-only | 3.3, 4.4 |
| §1 | real path, walk the real path | 4.4 step 1 |
| §1 | stop rules `.git` of any kind, `$HOME`, disk, root, ceiling dirs; never run git | 4.4 step 3 |
| §1 | never the global home, `os.SameFile` | 4.4 step 3.1 |
| §1 | nearest wins, nested repo wall, submodule own project | 4.4 notes, step 2 |
| §1 | linked worktree via `gitdir:` + `commondir` | 4.4 step 2 |
| §1 | per-kind scope global / project / project-else-global | 3.2, 4.7, 5.2 |
| §1 | default scopes (db by mode, cache global, config both, identity global) | 5.1, 5.2 |
| §1 | no project + project kind → error text | 3.5 `ErrNoProject`, 4.4 step 5 |
| §1 | project home created only by init | 2, 3.4 `InitProject` |
| §1 | `<APP>_<KIND>_DIR` | 4.7, 5.3 |
| §1 | set-but-empty = unset | 4.1 |
| §1 | `~` and `${VAR}`, unset is an error | 4.1 |
| §1 | no `$HOME` rules | 4.3 rule 4, 4.7 |
| §1 | lazy creation, reads never create | 2, 4.8 |
| §1 | root vs another user's home, `--allow-root` | 4.8 |
| §1 | doctor shows rule + fallback | 3.4 `Paths`, 11 |
| §2 | layout is a setting: kinds with name, path, type, git, scope, disposable | 3.3, 5.1, 5.3 |
| §2 | `WithKind` / `WithoutKind`, new kinds get env var, paths, gitignore | 5.3 |
| §2 | standard kinds and tree | 5.1 |
| §2 | `home.jsonc` app/layout/scope; higher layout refuses to write | 5.8, 4.8 |
| §2 | only create used folders | 2 (`Ensure` per kind) |
| §2 | 0700 / 0600 | 5.4 |
| §2 | home may be a symlink; db/config only inside real home | 5.6 |
| §2 | `CACHEDIR.TAG`, Time Machine, `.metadata_never_index` | 5.5 |
| §2 | logs rotate 10 MB × 3, 30 days, crash dumps | out: the tool's logger; appdir gives `logs/` |
| §2 | per-file git rules, default list, ignore everything else | 5.7 |
| §2 | `WithGitCommit` / `WithGitIgnore` win over defaults | 5.7 |
| §2 | marker block, only in a git work tree, re-include folders first | 5.7 |
| §2 | doctor lists committed and ignored | 5.7 (`GitignoreBlock`), the tool prints it |
| §3 | two homes at once | 3.4 `Global`/`Project` |
| §3 | config read order | 6.1 |
| §3 | other kinds from their scope | 4.7 |
| §3 | global-only settings | out: config loader (6.2); appdir supplies `ConfigFile.Home`/`Committed` |
| §4 | `home:` / `db:` keys in config.jsonc / config.local.jsonc only | 4.5 |
| §4 | committed: relative or `~/`; absolute only in local/env | 4.5 |
| §4 | one hop | 4.5 |
| §4 | missing target error; init --home creates | 4.5, 3.4 `InitHome` |
| §4 | target without this app's `home.jsonc` refused | 4.5 |
| §8 | lock in parent folder `.<new>.moving.lock` | 9 step 3 |
| §8 | refuse while old daemon lock held | 9 step 5 |
| §8 | same disk rename; other disk copy, verify, rename old `.moved-<date>` | 9 steps 6–7 |
| §8 | `MOVED.txt`, never delete | 9 step 8 |
| §8 | both exist: use new, say so | 9 step 2 |
| §8 | dev profile never moves | 9 step 1 |
| §8 | committed folders never moved automatically | 9 step 1 (`migrate-layout` is the tool's command) |
| §9 | `run/lock`, `run/daemon.lock`, pid, discovery paths | 7 |
| §9 | socket over 100 bytes → `/tmp/<app>-<uid>/<hash>.sock`, XDG_RUNTIME_DIR, `run/socket` | 7 |
| §9 | profile in discovery/handshake | out: the tool's protocol |
| §9 | `--daemon-profile` / `<APP>_DAEMON_PROFILE` path lookup | 7, 8.6 |
| §10 | profile changes both homes | 4.6 |
| §10 | open profile list, unknown name is an error | 3.3, 8.1 |
| §10 | detection order flag → env (two limits) → build | 8.1 |
| §10 | release = explicit flag or go-install rule | 8.2 |
| §10 | `Name(base)`, no literal labels | 8.3 |
| §10 | dev global home `~/.<app>-dev`, project `.<app>/profiles/dev/` | 4.6 |
| §10 | socket fallback name per profile | 7 (`Name(app)` in the folder) |
| §10 | TCP ports, launchd/systemd labels, skills, blocks, hooks, manifests, binary names, keychain, bundle ids | out: each tool / agentinstall, through `Name`/`NameWith` (8.3) |
| §10 | prod-db guard, `os.SameFile`, empty env, read-only with ALLOW_PROD | 8.4 |
| §10 | `<APP>_PROFILE` on children; leak filter; `CleanEnv` | 8.5 |
| §10 | prod/dev install entries never cross | out: agentinstall; names via 8.3 |
| §10 | one stderr line when not prod | 8.3 `Banner` |
| §13 | `ForTest(t)`, panic outside temp root | 10 |
| A1 | `.git` can be a file | 4.4 step 3.2 |
| A2 | worktrees use the main checkout home | 4.4 step 2 |
| A3 | project data never falls into the global home | 4.4 step 5, 5.2 |
| A4 | stop at disk change, ceiling dirs | 4.4 steps 3.3–3.4 |
| A5 | real path, `os.SameFile` | 4.4 |
| A6 | nesting rules | 4.4 notes |
| A7 | pointer read after the project is found | 4.5 |
| A8 | sudo/root | 4.8 |
| A9 | no HOME / read-only HOME, lazy creation | 4.3 rule 4, 4.7, 4.8 |
| A10 | temp fallback `<tmp>/<app>-<uid>/`, 0700, owner/symlink checked | 4.7, 7 (`EnsureSocketDir`), `Ensure` |
| A11 | profiles cover both homes | 4.6 |
| A12 | `~` and `${VAR}` only, empty = unset | 4.1 |
| B1 | exact gitignore block | 5.7 |
| B2 | symlink rule | 5.6 |
| B3 | cache exclusions | 5.5 |
| B4 | cache scope global | 5.1 |
| B5 | `home.jsonc` layout marker | 5.8, 4.8 |
| B6 | log rotation | out: the tool's logger (see §2 row) |
| C2 | pointer safety and portability | 4.5 |
| C3 | one pointer location | 4.5 |
| C6 | reserved env names | 5.3 |
| D9 | socket path too long | 7 |
| D11 | safe legacy move | 9 |
| D12 | committed folders not auto-moved | 9 step 1 |
| G1 | test isolation | 10 |

## 14. Decided

The open questions of the first draft are closed. Decisions 1 and 5 are the owner's (2026-11-10); 2 to 4 were chosen by the coordinator and are to be reviewed with the owner.

1. **Dev profile and project config (owner).** A dev build reads `.<app>/config.jsonc` and `.<app>/config.local.jsonc` like prod; only data moves to `.<app>/profiles/dev/`. In addition, an optional `.<app>/profiles/dev/config.local.jsonc` (gitignored under `profiles/`) is read last and wins over both, dev profile only (sections 4.6, 6.1).
2. **`db:` pointer in the dev profile (chosen by coordinator, to review with the owner).** Ignored with warning `pointer-ignored-for-profile`; the dev db stays at `.<app>/profiles/dev/<app>.db`, so dev data never lands in the private repo the pointer names (section 4.5).
3. **Read-only home and disposable kinds (chosen by coordinator, to review with the owner).** Besides the no-`$HOME` case, `Ensure` falls back to `<tmp>/<app>-<uid>/` for a disposable kind when creating its folder fails with a read-only or permission error; `WithTempFallback(false)` turns it off (section 4.7).
4. **Default mode (chosen by coordinator, to review with the owner).** `ModeProject`, because it fails safe; global tools such as agx and runbay_pty pass `WithMode(ModeGlobal)` (section 5.2).
5. **Name (owner).** The module is `github.com/ubgo/appdir`. Closed.

No open questions remain.
