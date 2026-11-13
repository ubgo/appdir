# doctor

Builds the two reports every tool on the standard ships, from appdir's own types: `paths --json` (the standard's envelope around `Paths()`) and the appdir part of `doctor` (`ConfigFiles()`, `Warnings()`, `Kinds()`, `GitignoreBlock()`). It runs `paths` again under `--home team` (`WithHome`), where the kinds that live in both homes (`home`, `config`, `config-local`) are listed once, labelled global, so 11 lines instead of 14. Then it provokes five real errors (`ErrNoProject`, `ErrUnknownProfile`, `ErrPointerAbsolute`, `ErrUnknownKind`, `ErrLayoutTooNew`), calls `New` with five option sets it refuses (`ErrInvalidOption`), and maps each error to the standard's exit codes with `errors.Is`.

Everything runs inside a temp root: the user home, the project and the environment are handed to appdir with `WithHomeDir`, `WithWorkDir` and `WithEnv`, so the real home is never read or written. The temp root is printed as `$ROOT`.

## Run it

```sh
go run ./examples/doctor
```

## Output

```text
== paths --json
{
  "schema_version": 1,
  "kind": "paths",
  "count": 14,
  "profile": "prod",
  "data": [
    {
      "kind": "home",
      "home": "global",
      "path": "$ROOT/user/.doctor/home.jsonc",
      "type": "file",
      "git": "commit",
      "rule": "global",
      "fallback": false
    },
    {
      "kind": "home",
      "home": "project",
      "path": "$ROOT/repo/.doctor/home.jsonc",
      "type": "file",
      "git": "commit",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "config",
      "home": "global",
      "path": "$ROOT/user/.doctor/config.jsonc",
      "type": "file",
      "git": "commit",
      "rule": "global",
      "fallback": false
    },
    {
      "kind": "config",
      "home": "project",
      "path": "$ROOT/repo/.doctor/config.jsonc",
      "type": "file",
      "git": "commit",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "config-local",
      "home": "global",
      "path": "$ROOT/user/.doctor/config.local.jsonc",
      "type": "file",
      "git": "ignore",
      "rule": "global",
      "fallback": false
    },
    {
      "kind": "config-local",
      "home": "project",
      "path": "$ROOT/repo/.doctor/config.local.jsonc",
      "type": "file",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "gitignore",
      "home": "project",
      "path": "$ROOT/repo/.doctor/.gitignore",
      "type": "file",
      "git": "commit",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "shared",
      "home": "project",
      "path": "$ROOT/repo/.doctor/shared",
      "type": "folder",
      "git": "commit",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "db",
      "home": "project",
      "path": "$ROOT/repo/.doctor/doctor.db",
      "type": "file",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "backups",
      "home": "project",
      "path": "$ROOT/repo/.doctor/backups",
      "type": "folder",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "cache",
      "home": "global",
      "path": "$ROOT/user/.doctor/cache",
      "type": "folder",
      "git": "ignore",
      "rule": "global",
      "fallback": false
    },
    {
      "kind": "run",
      "home": "project",
      "path": "$ROOT/repo/.doctor/run",
      "type": "folder",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "logs",
      "home": "project",
      "path": "$ROOT/repo/.doctor/logs",
      "type": "folder",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "templates",
      "home": "project",
      "path": "$ROOT/repo/.doctor/templates",
      "type": "folder",
      "git": "commit",
      "rule": "project",
      "fallback": false
    }
  ]
}
== doctor
profile prod (release=true, flag)
config files (later wins):
  global  $ROOT/user/.doctor/config.jsonc          local=false committed=false missing
  global  $ROOT/user/.doctor/config.local.jsonc    local=true committed=false missing
  project $ROOT/repo/.doctor/config.jsonc          local=false committed=true missing
  project $ROOT/repo/.doctor/config.local.jsonc    local=true committed=false missing
warnings:
  profile-env-ignored: DOCTOR_PROFILE=dev ignored by a release build; pass --profile dev
  ceiling-relative: DOCTOR_CEILING_DIRS entry "relative/dir" is relative; ignored
layout:
  home          home.jsonc           file   commit both
  config        config.jsonc         file   commit both
  config-local  config.local.jsonc   file   ignore both
  gitignore     .gitignore           file   commit project
  shared        shared               folder commit project
  db            doctor.db            file   ignore project
  backups       backups              folder ignore project
  cache         cache                folder ignore global
  run           run                  folder ignore project
  logs          logs                 folder ignore project-else-global
  templates     templates            folder commit project
gitignore block:
  # doctor:begin
  *
  !.gitignore
  !home.jsonc
  !config.jsonc
  !shared/
  !shared/**
  !templates/
  !templates/**
  # doctor:end
== paths --home team
  home          global   flag  $ROOT/team/home.jsonc
  config        global   flag  $ROOT/team/config.jsonc
  config-local  global   flag  $ROOT/team/config.local.jsonc
  gitignore     project  flag  $ROOT/team/.gitignore
  shared        project  flag  $ROOT/team/shared
  db            project  flag  $ROOT/team/doctor.db
  backups       project  flag  $ROOT/team/backups
  cache         global   flag  $ROOT/team/cache
  run           project  flag  $ROOT/team/run
  logs          project  flag  $ROOT/team/logs
  templates     project  flag  $ROOT/team/templates
== errors
ErrNoProject       exit 3  no .doctor/ here; run 'doctor init' or pass --home
ErrUnknownProfile  exit 2  resolve: appdir: --profile: unknown profile "staging" (known: prod, dev)
ErrPointerAbsolute exit 1  home: "/srv/doctor" in $ROOT/repo-ptr/.doctor/config.jsonc is absolute; put it in config.local.jsonc
ErrUnknownKind     exit 1  appdir: unknown kind "template"
ErrLayoutTooNew    exit 5  home $ROOT/repo-new/.doctor has layout 2, this binary knows 1; upgrade doctor
== invalid options
app name         exit 2  appdir: app name "Doctor" must match ^[a-z0-9][a-z0-9._-]*$
dir name         exit 2  appdir: dir name ".." must be one plain path segment
kind path        exit 2  appdir: kind "tpl": path "../x" has an empty, . or .. segment
release profile  exit 2  appdir: profile "staging" is not in the list [prod dev]
nil seam         exit 2  appdir: a seam option was given nil
```

The two warnings come from the example's environment: `DOCTOR_PROFILE=dev` is ignored by a release build, and a relative `DOCTOR_CEILING_DIRS` entry is ignored.

## Tests

`go test ./examples/doctor/` runs the example in a test temp folder, checks the quoted lines, parses the JSON envelope, checks that the `--home` run lists each both-homes kind once labelled global, and checks the exit code of every sentinel.

## Related

- [paths --json and doctor](../../docs/reference/paths-json.md), the guide built on this example
- [Errors and exit codes](../../docs/reference/errors.md)
- [Options reference](../../docs/reference/options.md)
