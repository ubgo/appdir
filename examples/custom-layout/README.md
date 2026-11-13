# Example: custom layout

Changes the standard layout with `WithKind` (a rename, a new committed kind, a global kind, an anchored kind, a disposable kind) and `WithoutKind`, flips a kind to `Ignore()`, prints `Kinds()` with each `KindEnv` name, compares the two modes, shows the gitignore block with `WithGitCommit` / `WithGitIgnore`, `WriteGitignore` keeping hand-written lines and refusing broken markers, `ErrUnknownKind` for a dropped kind, `InitProject` under `WithoutKind(KindHome)` (folder only), `Ensure` with 0700 folders and `CACHEDIR.TAG`, `WithPerm` and `WithCacheTag(false)`, the symlink rule, `VerifyFile` on a file that is not a kind, and the layout-version guard. The project is a git work tree (a `.git` folder).

Everything runs inside a temp folder: the user home is a folder in it (via `appdir.WithHomeDir`), the environment is empty (via `appdir.WithEnv`), and the real home is never touched. The temp root is printed as `$TMP`.

## Run it

```sh
go run ./examples/custom-layout
```

## Output

```text
== kinds (mode project)
home         home.jsonc         file   commit both                -                  anchored
config       config.jsonc       file   commit both                -                  anchored
config-local config.local.jsonc file   ignore both                -                  anchored
gitignore    .gitignore         file   commit project             -                  anchored
shared       data               folder commit project             LORE_SHARED_DIR
db           lore.db            file   ignore project             LORE_DB_DIR
cache        cache              folder ignore global              LORE_CACHE_DIR     disposable
run          run                folder ignore project             LORE_RUN_DIR
logs         logs               folder ignore project-else-global LORE_LOGS_DIR
templates    templates          folder commit project             LORE_TEMPLATES_DIR
identity     identity.json      file   ignore global              LORE_IDENTITY_DIR
schema       schema.jsonc       file   commit project             -                  anchored
thumbs       thumbs             folder ignore project             LORE_THUMBS_DIR    disposable

== mode and scope
project db=project logs=project-else-global cache=global
global  db=global logs=global cache=global

== invalid kind
error: appdir: kind "escape": path "../outside" has an empty, . or .. segment
is ErrInvalidOption: true

== Ignore() and unknown kinds
shared git rule: ignore; block re-includes data/: false
Path(backups): appdir: unknown kind "backups"
Ensure(backups): appdir: unknown kind "backups"
is ErrUnknownKind: true

== WithoutKind(home) and init
init $TMP/plain/.lore: folder 0700, 0 files, home.jsonc missing

== gitignore block
# lore:begin
*
!.gitignore
!home.jsonc
!config.jsonc
!data/
!data/**
!templates/
!templates/**
!schema.jsonc
!prompts/
!prompts/*.md
data/private/
# lore:end

== WriteGitignore keeps hand-written lines
in a git work tree: true
# hand-written lines stay
# lore:begin
*
!.gitignore
!home.jsonc
!config.jsonc
!data/
!data/**
!templates/
!templates/**
!schema.jsonc
!prompts/
!prompts/*.md
!glossary.md
data/private/
# lore:end
notes.txt

== broken markers
error: $TMP/shop/.lore/.gitignore: # lore:begin / # lore:end missing, doubled or out of order; fix it by hand
is ErrGitignoreMarkers: true

== Ensure
templates $TMP/shop/.lore/templates          folder 0700  CACHEDIR.TAG false
thumbs    $TMP/shop/.lore/thumbs             folder 0700  CACHEDIR.TAG true
cache     $TMP/userhome/.lore/cache          folder 0700  CACHEDIR.TAG true
identity  $TMP/userhome/.lore/identity.json  folder 0700  CACHEDIR.TAG false
home.jsonc file 0600
templates after chmod 0750 and Ensure: 0750

== WithPerm and WithCacheTag(false)
previews  $TMP/shop/.lore/previews           folder 0750  CACHEDIR.TAG false
InitHome $TMP/team-home: folder 0750, home.jsonc 0640

== symlink rule
db -> $TMP/shop/.lore/real.db: Ensure error: <nil>
db -> $TMP/outside/stolen.db: Ensure error: $TMP/shop/.lore/lore.db links to $TMP/outside/stolen.db, outside the home $TMP/shop/.lore
is ErrSymlinkEscape: true

== VerifyFile
missing note: <nil>
linked note: $TMP/shop/.lore/data/note.md links to $TMP/outside/note.md, outside the home $TMP/shop/.lore
is ErrSymlinkEscape: true

== layout version guard
Ensure(run): home $TMP/shop/.lore has layout 2, this binary knows 1; upgrade lore
is ErrLayoutTooNew: true
Ensure(run): <nil>
```

## Read more

- Guide: [Kinds](../../docs/layout/kinds.md)
- Guide: [The .gitignore block](../../docs/layout/gitignore.md)
- Guide: [Files and permissions](../../docs/layout/files-and-permissions.md)
