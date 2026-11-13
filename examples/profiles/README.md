# Example: profiles

Shows how appdir picks a profile (`--profile`, `DEMO_PROFILE` with its two limits, the build), every release reason from fake build info, a `version` line built from `IsRelease`/`ReleaseReason`, a custom `staging` profile, where each profile keeps its files, `Name`/`NameWith`/`Banner` (also for another profile through `ForProfile`), the env-leak filter, and `ChildEnv`/`CleanEnv`/`ForProfile`.

Everything runs inside a temp root: the user's home is a folder under it (`WithHomeDir`) and the environment is a map (`WithEnv`), so the program never touches your real home or reads your shell's variables.

## Run it

```sh
go run ./examples/profiles
```

## Output

Paths are relative to the temp root.

```text
== profile detection
dev build                                        -> profile dev
release build                                    -> profile prod
dev build, --profile prod                        -> profile prod
dev build, DEMO_PROFILE=prod                     -> profile dev
                                                    warning profile-env-ignored: DEMO_PROFILE=prod ignored by a dev build; set DEMO_ALLOW_PROD=1 too, or pass --profile
dev build, DEMO_PROFILE=prod, DEMO_ALLOW_PROD=1  -> profile prod
release build, DEMO_PROFILE=dev                  -> profile prod
                                                    warning profile-env-ignored: DEMO_PROFILE=dev ignored by a release build; pass --profile dev
release build, --profile dev                     -> profile dev
dev build, --profile qa                          -> error: appdir: --profile: unknown profile "qa" (known: prod, dev)
release build, DEMO_PROFILE=qa                   -> error: appdir: DEMO_PROFILE: unknown profile "qa" (known: prod, dev)
== release detection
go install module@v1.4.2           release=true  reason="go-install"                     profile=prod
go build in a checkout             release=false reason="dev: (devel)"                   profile=dev
go install ./cmd/demo on the tag   release=false reason="dev: vcs.revision"              profile=dev
go install module@<commit>         release=false reason="dev: pseudo-version"            profile=dev
modified checkout                  release=false reason="dev: +dirty"                    profile=dev
version without v                  release=false reason="dev: version without v prefix"  profile=dev
no build info                      release=false reason="dev: no build info"             profile=dev
-ldflags release=true              release=true  reason="flag"                           profile=prod
-ldflags release=false             release=false reason="dev: flag"                      profile=dev
== version line
go install module@v1.4.2   demo prod (release=true, reason "go-install")
go install ./cmd/demo      demo dev (release=false, reason "dev: vcs.revision")
== custom profiles
--profile staging: profile staging, global home/.demo-staging, db home/proj/.demo/profiles/staging/demo.db
WithDevProfile(staging), dev build: profile staging
WithReleaseProfile(stable), release build: profile stable, global home/.demo
== profile folders
profile prod: global home/.demo
  db     home/proj/.demo/demo.db
  cache  home/.demo/cache
  config home/proj/.demo/config.jsonc
profile dev: global home/.demo-dev
  db     home/proj/.demo/profiles/dev/demo.db
  cache  home/.demo-dev/cache
  config home/proj/.demo/config.jsonc
== names and banner
profile prod: Name("demo")="demo" NameWith("com.example.demo", ".")="com.example.demo"
profile prod: Banner()=""
profile dev: Name("demo")="demo-dev" NameWith("com.example.demo", ".")="com.example.demo.dev"
profile dev: Banner()="[demo] profile: dev → ~/.demo-dev"
== another profile's names
profile prod, ForProfile(dev): Name("demo")="demo-dev" NameWith("com.example.demo", ".")="com.example.demo.dev"
== env leak filter
warning env-points-to-prod: DEMO_HOME points into the prod home home/.demo; ignored for profile dev
warning env-points-to-prod: DEMO_DB_DIR points into the prod home home/.demo; ignored for profile dev
profile dev: global home/.demo-dev, db home/proj/.demo/profiles/dev/demo.db
== child processes
ChildEnv: PATH=/usr/bin DEMO_HOME=~/.demo DEMO_PROFILE=dev
CleanEnv: PATH=/usr/bin
ForProfile(prod): profile prod, global home/.demo, db home/proj/.demo/demo.db
```

## Guides

- [Profiles](../../docs/profiles/profiles.md)
- [Release detection](../../docs/profiles/release-detection.md)
- [Names and banner](../../docs/profiles/names.md)
- [Child processes and env leaks](../../docs/profiles/child-processes.md)
