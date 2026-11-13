# Profiles: prod, dev and your own

A profile is a named set of homes. The installed tool runs as `prod` and uses `~/.<app>` and `.<app>/`; the build you are working on runs as `dev` and uses `~/.<app>-dev` and `.<app>/profiles/dev/`, so it can never write into your real data by accident. appdir picks the profile from the `--profile` flag, then `<APP>_PROFILE`, then the build. Read this page when you add a `--profile` flag, add a profile of your own (such as `staging`), or wonder why a dev build ignored `<APP>_PROFILE=prod`.

## Before you start

- [First use](../start/first-use.md): `New`, `Path` and the seams.
- Import `"github.com/ubgo/appdir"`.
- How appdir decides "release build" or "dev build" is on its own page: [Release detection](release-detection.md). The examples below fix it with `WithRelease(true)` or `WithRelease(false)`, which stands in for the tool's `-ldflags -X` release variable.
- The example app is `demo`, so the env names are `DEMO_PROFILE` and `DEMO_ALLOW_PROD`. Its user home is `home/` and its project is `home/proj/` (with `.demo/` made by `InitProject`).

## Step 1 — Pass the flag and the seams to New

Every case in the example resolves through one helper. A real tool passes its `--profile` value with `WithProfile` (empty means "not given") and leaves the seams at their defaults; the example replaces the user's home, the working folder and the environment so it never touches your machine.

<!-- snippet: examples/profiles/main.go#open -->
```go
// open resolves the app with the sandbox seams plus the case's own options.
// The env map replaces the process environment, so each case states exactly
// which DEMO_* variables it sets.
func (s sandbox) open(env fakeEnv, opts ...appdir.Option) (*appdir.Dirs, error) {
	base := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return s.home, nil }),
		appdir.WithWorkDir(s.project),
		appdir.WithEnv(env.lookup),
	}
	return appdir.New(appName, append(base, opts...)...)
}
```

## Step 2 — Let appdir pick the profile

The order is fixed ([pickProfile](ds:block?id=dirs-pickprofile-g7fmfemq)): the flag wins; then `<APP>_PROFILE`; then the build (release build → the release profile, `prod`; anything else → the dev profile, `dev`). The env variable has two limits, both reported as warning `profile-env-ignored` in `Warnings()`:

- a dev build ignores `<APP>_PROFILE=prod` unless `<APP>_ALLOW_PROD=1` is set too, because a terminal that still exports the installed tool's settings must not turn your dev build into prod;
- a release build ignores any other profile from the env; use the flag to run the installed binary as `dev`.

A name outside the list, from the flag or the env, is `ErrUnknownProfile` from `New`. It is never a fallback to prod: a typo must not land on your real data.

<!-- snippet: examples/profiles/main.go#detect -->
```go
// detectProfiles runs the three detection rules in order: --profile
// (WithProfile), then DEMO_PROFILE with its two limits, then the build.
// WithRelease stands in for the tool's -ldflags -X release variable.
func detectProfiles(w io.Writer, s sandbox) error {
	dev, rel := appdir.WithRelease(false), appdir.WithRelease(true)
	cases := []struct {
		label string
		env   fakeEnv
		opts  []appdir.Option
	}{
		{"dev build", nil, []appdir.Option{dev}},
		{"release build", nil, []appdir.Option{rel}},
		{"dev build, --profile prod", nil, []appdir.Option{dev, appdir.WithProfile(string(appdir.ProfileProd))}},
		{"dev build, DEMO_PROFILE=prod", fakeEnv{envProfile: string(appdir.ProfileProd)}, []appdir.Option{dev}},
		{"dev build, DEMO_PROFILE=prod, DEMO_ALLOW_PROD=1", fakeEnv{envProfile: string(appdir.ProfileProd), envAllowProd: allowProdOn}, []appdir.Option{dev}},
		{"release build, DEMO_PROFILE=dev", fakeEnv{envProfile: string(appdir.ProfileDev)}, []appdir.Option{rel}},
		{"release build, --profile dev", nil, []appdir.Option{rel, appdir.WithProfile(string(appdir.ProfileDev))}},
		{"dev build, --profile qa", nil, []appdir.Option{dev, appdir.WithProfile(profileUnknown)}},
		{"release build, DEMO_PROFILE=qa", fakeEnv{envProfile: profileUnknown}, []appdir.Option{rel}},
	}
	for _, c := range cases {
		d, err := s.open(c.env, c.opts...)
		switch {
		case errors.Is(err, appdir.ErrUnknownProfile):
			// Never a fallback: a typo must not silently land on prod data.
			fmt.Fprintf(w, "%-48s -> error: %v\n", c.label, err)
			continue
		case err != nil:
			return err
		}
		fmt.Fprintf(w, "%-48s -> profile %s\n", c.label, d.Profile())
		for _, warn := range d.Warnings() {
			fmt.Fprintf(w, "%-48s    warning %s: %s\n", "", warn.Code, warn.Message)
		}
	}
	return nil
}
```

`Profile()` returns the profile in effect; print `Warnings()` once to stderr ([paths --json and warnings](../reference/paths-json.md)).

## Step 3 — Add your own profiles

`WithProfiles` replaces the list (default `prod, dev`). `WithReleaseProfile` names the profile a release build uses and the one that keeps the plain names; `WithDevProfile` names the profile of every other build. Both must be in the list, or `New` returns `ErrInvalidOption`.

<!-- snippet: examples/profiles/main.go#custom -->
```go
// customProfiles adds a staging profile next to prod and dev, then renames
// the release profile. The list is closed: anything outside it is refused.
func customProfiles(w io.Writer, s sandbox) error {
	list := appdir.WithProfiles(appdir.ProfileProd, appdir.ProfileDev, profileStaging)
	d, err := s.open(nil, list, appdir.WithRelease(false), appdir.WithProfile(string(profileStaging)))
	if err != nil {
		return err
	}
	db, err := d.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "--profile staging: profile %s, global %s, db %s\n", d.Profile(), s.show(d.Global().Base), s.show(db))

	// A dev build of this tool runs as staging instead of dev.
	d, err = s.open(nil, list, appdir.WithRelease(false), appdir.WithDevProfile(profileStaging))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "WithDevProfile(staging), dev build: profile %s\n", d.Profile())

	// A tool whose release profile is "stable": it keeps the plain names.
	d, err = s.open(nil, appdir.WithProfiles(profileStable, appdir.ProfileDev),
		appdir.WithReleaseProfile(profileStable), appdir.WithRelease(true))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "WithReleaseProfile(stable), release build: profile %s, global %s\n", d.Profile(), s.show(d.Global().Base))
	return nil
}
```

## Step 4 — See where each profile keeps its files

The release profile keeps `~/.<app>` and the project's `.<app>/`. Every other profile gets `~/.<app>-<profile>` as its global home and `.<app>/profiles/<profile>/` for its project data. Anchored kinds (`home.jsonc`, `config.jsonc`, `config.local.jsonc`, `.gitignore`) stay in the shared `.<app>/`: they are what makes the folder a home, so all profiles read the same project config.

<!-- snippet: examples/profiles/main.go#folders -->
```go
// profileFolders compares where prod and dev keep their files. Data kinds
// move to ~/.demo-dev and .demo/profiles/dev/; anchored kinds (home.jsonc,
// config.jsonc, config.local.jsonc, .gitignore) stay in the shared .demo/.
func profileFolders(w io.Writer, s sandbox) error {
	for _, release := range []bool{true, false} {
		d, err := s.open(nil, appdir.WithRelease(release))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "profile %s: global %s\n", d.Profile(), s.show(d.Global().Base))
		for _, k := range []appdir.Kind{appdir.KindDB, appdir.KindCache, appdir.KindConfig} {
			p, err := d.Path(k)
			if err != nil {
				return fmt.Errorf("path %s: %w", k, err)
			}
			fmt.Fprintf(w, "  %-6s %s\n", k, s.show(p))
		}
	}
	return nil
}
```

## Try it

```sh
go run ./examples/profiles
```

The sections this page covers (the rest of the output is on the other profile pages):

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
```

## Pitfalls

- `go run`, `go build` in a checkout and every test binary are dev builds. Without `WithRelease(true)` they use `~/.<app>-dev`; that is the point, but it surprises people the first time.
- A dev build with `--profile prod` resolves the prod paths, but the build is still dev: [`CheckDB`](prod-db-guard.md) refuses the prod database unless `<APP>_ALLOW_PROD=1`, and then opens it read-only. Setting `<APP>_ALLOW_PROD=1` alone also lets `<APP>_PROFILE=prod` through.
- `WithProfiles` replaces the list. `WithProfiles("staging")` alone fails in `New`, because the default release and dev profiles (`prod`, `dev`) are no longer in it.
- The error text names its source: `appdir: --profile: unknown profile "qa" (known: prod, dev)` or `appdir: DEMO_PROFILE: …`. Map `ErrUnknownProfile` to a usage exit code ([Errors](../reference/errors.md)).
- A set-but-empty `<APP>_PROFILE` counts as unset, like every appdir variable.
- `<APP>_DAEMON_PROFILE` and `--daemon-profile` are checked against the same list; an unknown name fails `New` too ([Run files and sockets](../runtime/run-files-and-sockets.md)).
- Project config is shared between profiles. A dev build that needs its own machine settings uses `.<app>/profiles/dev/config.local.jsonc`, read last ([Config files](../layout/config-files.md)).

## Related

- [Release detection](release-detection.md): how the build is decided.
- [Names and banner](names.md): `Name`, `NameWith`, `Banner`.
- [Prod-db guard](prod-db-guard.md) and [Child processes and env leaks](child-processes.md).
- [Overrides](../homes/overrides.md): `--home`, `<APP>_HOME`, `<APP>_<KIND>_DIR`.
- Example: [examples/profiles](../../examples/profiles).
- Design: [SPEC 8.1 Detection](../SPEC.md#81-detection) and [SPEC 4.6 Profile folders](../SPEC.md#46-profile-folders).
