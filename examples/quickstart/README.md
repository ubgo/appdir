# quickstart

The first program a tool writes with appdir: `New` resolves the global home and the project home and creates nothing, `Path`, `PathIn` and `Paths` read paths, and `Ensure` creates a folder on first use. The last two blocks show the global home of a release and a dev build, and which home the db, the logs and the cache use outside a project in `ModeProject` and `ModeGlobal`. It runs in a temp root: the user's home is `root/home` (`WithHomeDir`), the working folder is a project under it (`WithWorkDir`) and the environment is empty (`WithEnv`).

## Run

```sh
go run ./examples/quickstart
```

Output (paths relative to the temp root):

```text
== app name rule
New: appdir: app name "My App" must match ^[a-z0-9][a-z0-9._-]*$
== New: resolve, create nothing
app:          lore
global home:  home/.lore (rule global, fallback false, info false)
has project:  true
project home: home/projects/shop/.lore (rule project, data home/projects/shop/.lore)
project info: app=lore layout=1 scope=project
== Path and PathIn
Path(db): home/projects/shop/.lore/lore.db
Path(cache): home/.lore/cache
Path(config): home/projects/shop/.lore/config.jsonc
PathIn(config, global): home/.lore/config.jsonc
PathIn(config, project): home/projects/shop/.lore/config.jsonc
== Paths (what paths --json lists)
home          global   global   home/.lore/home.jsonc
home          project  project  home/projects/shop/.lore/home.jsonc
config        global   global   home/.lore/config.jsonc
config        project  project  home/projects/shop/.lore/config.jsonc
config-local  global   global   home/.lore/config.local.jsonc
config-local  project  project  home/projects/shop/.lore/config.local.jsonc
gitignore     project  project  home/projects/shop/.lore/.gitignore
shared        project  project  home/projects/shop/.lore/shared
db            project  project  home/projects/shop/.lore/lore.db
backups       project  project  home/projects/shop/.lore/backups
cache         global   global   home/.lore/cache
run           project  project  home/projects/shop/.lore/run
logs          project  project  home/projects/shop/.lore/logs
== reads never create
global home exists: false
== Ensure: create on first use
Ensure(cache): home/.lore/cache
Ensure(logs): home/projects/shop/.lore/logs
Ensure(db): home/projects/shop/.lore/lore.db
global home exists: true
== global home per build
release=true  profile prod global home home/.lore (rule global)
release=false profile dev  global home home/.lore-dev (rule global)
== which home a kind uses, outside a project
project db     error: no .lore/ here; run 'lore init' or pass --home
project cache  global  fallback=false home/.lore/cache
project logs   global  fallback=true  home/.lore/logs
global  db     global  fallback=false home/.lore/lore.db
global  cache  global  fallback=false home/.lore/cache
global  logs   global  fallback=false home/.lore/logs
```

`go test ./examples/quickstart` runs the same program in a test folder and checks these lines.

## Guide

- [First use: resolve, read, ensure](../../docs/start/first-use.md)
- [Global and project homes](../../docs/homes/global-and-project.md)
