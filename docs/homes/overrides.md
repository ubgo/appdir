# Overrides: `--home`, `<APP>_HOME` and `<APP>_<KIND>_DIR`

Users sometimes need files somewhere else: a scratch home to try something, a cache on a fast disk, a db on a bigger one. appdir gives every tool the same three ways: the `--home` flag and the `<APP>_HOME` variable make one home for everything, and `<APP>_<KIND>_DIR` moves one kind. Paths given by users expand `~` and `${VAR}`, and a variable that is not set is an error, never a path at `/`. Read this page to wire the flag and to print the right variable names in help and doctor output.

## Before you start

- [First use](../start/first-use.md) and [Global and project homes](global-and-project.md).
- Import `"github.com/ubgo/appdir"`.
- Your tool parses its own `--home` flag and passes it as `appdir.WithHome(flagHome)`; appdir never reads `os.Args`. An empty string means not given.
- The example reads variables from a map through `WithEnv`; a real tool keeps the default (`os.LookupEnv`).

## Step 1 — Pass `--home`

`WithHome(path)` is one home for every scope, with rule `flag`: config, db and cache all go there, and the project lookup is skipped, so a command never reads one place and writes another. The path expands like every user path (Step 4); a relative path resolves against the working folder.

<!-- snippet: examples/overrides/main.go#open -->
```go
// open resolves the homes with the given fake environment and extra options
// (the flags a tool passes, such as WithHome for --home).
func (s setup) open(e env, extra ...appdir.Option) (*appdir.Dirs, error) {
	opts := append([]appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return s.home, nil }),
		appdir.WithWorkDir(s.project),
		appdir.WithEnv(func(k string) (string, bool) { v, ok := e[k]; return v, ok }),
		appdir.WithRelease(true), // prod profile: plain folder names
	}, extra...)
	d, err := appdir.New(appName, opts...)
	if err != nil {
		return nil, fmt.Errorf("appdir: %w", err)
	}
	return d, nil
}
```

## Step 2 — Honour `<APP>_HOME`

`LORE_HOME` does the same with rule `env-home`: users set it once in a shell profile or a CI job instead of passing `--home` on every command. Your tool does nothing to honour it; `New` reads it. `--home` wins over it. For a dev build, a `LORE_HOME` that points into the prod global home is ignored with warning `env-points-to-prod`, and the dev build keeps its own `~/.lore-dev` ([Child processes](../profiles/child-processes.md)). The example shows all three:

<!-- snippet: examples/overrides/main.go#envhome -->
```go
// envHome sets LORE_HOME: one home for every kind, with rule env-home. When
// --home is given too, the flag wins. A dev build ignores a LORE_HOME that
// points into the prod global home (~/.lore) and says so with a warning, so
// a variable left over from a prod shell never lets dev touch prod data.
func (s setup) envHome(w io.Writer, fast string) error {
	e := env{envHome: "${FAST}/lore", varFast: fast}
	if err := s.show(w, e, nil); err != nil {
		return err
	}
	d, err := s.open(e, appdir.WithHome(tryHome))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "with --home %s: global %s (rule %s)\n", tryHome, s.label(d.Global().Base), d.Global().Rule)
	// WithRelease(false) after open's WithRelease(true): the last option wins,
	// so this Dirs is a dev build.
	d, err = s.open(env{envHome: prodHome}, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dev build, LORE_HOME=%s: global %s (rule %s)\n", prodHome, s.label(d.Global().Base), d.Global().Rule)
	for _, wn := range d.Warnings() {
		// The message names the absolute prod home; trim the temp root so the
		// output is the same on every run.
		msg := strings.ReplaceAll(wn.Message, s.root+string(filepath.Separator), "")
		fmt.Fprintf(w, "  warning %s: %s\n", wn.Code, filepath.ToSlash(msg))
	}
	return nil
}
```

## Step 3 — Move one kind with `<APP>_<KIND>_DIR`

`LORE_CACHE_DIR=/fast/lore-cache` moves only the cache, with rule `env-kind`; everything else stays where it was. The variable names the folder: for a file kind the file keeps its name inside it, so `LORE_DB_DIR=/big/db` gives `/big/db/lore.db`. Every non-anchored kind has one, including kinds you add (`templates` gets `LORE_TEMPLATES_DIR`). Anchored kinds (`home.jsonc` and the config files) have none: moving them would split a home from its own marker; use `--home` instead ([per-kind rules](ds:block?id=dirs-entryin-zhkmadk3)).

<!-- snippet: examples/overrides/main.go#show -->
```go
// show prints where the config file, the db and the cache land, and the
// rule that picked each one.
func (s setup) show(w io.Writer, e env, extra []appdir.Option) error {
	d, err := s.open(e, extra...)
	if err != nil {
		return err
	}
	for _, k := range []appdir.Kind{appdir.KindConfig, appdir.KindDB, appdir.KindCache} {
		// Paths lists a ScopeBoth kind (config) once per home, global first;
		// the last line is the one Path(k) returns.
		var last appdir.Entry
		for _, en := range d.Paths() {
			if en.Kind == k {
				last = en
			}
		}
		fmt.Fprintf(w, "%-7s %-9s %s\n", k, last.Rule, s.label(last.Path))
	}
	return nil
}
```

## Step 4 — Know how user paths expand

Paths from `--home`, `<APP>_HOME`, `<APP>_<KIND>_DIR`, ceiling entries and local pointer keys expand the same way ([expandUserPath](ds:block?id=config-expanduserpath-67kdzfc9)):

- `~` and `~/…` use the user's home; `~user` is refused with `ErrInvalidOption`.
- `${VAR}` is replaced by the variable; `$VAR` without braces is left as literal text.
- An unset `${VAR}` is `ErrEnvVar` naming the variable (`LORE_CACHE_DIR: ${FAST} is not set`), so a missing variable never turns `${FAST}/cache` into `/cache`.
- A variable that is set but empty counts as unset, for `${VAR}` and for every `<APP>_*` variable: `LORE_HOME=` is the same as no `LORE_HOME`.
- A relative result resolves against the working folder (a relative pointer key resolves against the folder of its config file instead).

<!-- snippet: examples/overrides/main.go#bad -->
```go
// bad shows values that never turn into a path: an unset ${VAR}, a set but
// empty one, ~user. $VAR without braces is not expanded: it stays a literal,
// relative folder name.
func (s setup) bad(w io.Writer) error {
	cases := []struct {
		name string
		e    env
	}{
		{"${FAST} unset", env{envCache: "${FAST}/cache"}},
		{"${FAST} empty", env{envCache: "${FAST}/cache", varFast: ""}},
		{"~user", env{envCache: "~bob/cache"}},
		{"$FAST literal", env{envCache: "$FAST/cache", varFast: "/x"}},
		{"LORE_HOME empty", env{envHome: ""}},
	}
	for _, c := range cases {
		d, err := s.open(c.e)
		if err != nil {
			return err
		}
		p, err := d.Path(appdir.KindCache)
		switch {
		case errors.Is(err, appdir.ErrEnvVar), errors.Is(err, appdir.ErrInvalidOption):
			fmt.Fprintf(w, "%-15s error: %v\n", c.name+":", err)
		case err != nil:
			return err
		default:
			fmt.Fprintf(w, "%-15s cache: %s\n", c.name+":", s.label(p))
		}
	}
	return nil
}
```

## Step 5 — Print the variable names

A tool should name the variables in its help, its errors and `doctor`, and they must match what appdir reads. `EnvPrefix()` is the prefix (`LORE`), `EnvName(suffix)` builds `LORE_<suffix>`, and `KindEnv(kind)` is a kind's `<APP>_<KIND>_DIR` (empty for anchored kinds). The default prefix is the app name upper-cased with `-` and `.` turned into `_` (`my-app` gives `MY_APP`); `WithEnvPrefix(p)` replaces it. The `names` step in the example prints them:

<!-- snippet: examples/overrides/main.go#names -->
```go
// names prints the variable names a tool shows in help and doctor output,
// for the defaults and with WithEnvPrefix / WithDirName, and a dir name
// that is refused.
func (s setup) names(w io.Writer) error {
	d, err := s.open(nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "EnvPrefix: %s  EnvName(HOME): %s\n", d.EnvPrefix(), d.EnvName("HOME"))
	fmt.Fprintf(w, "KindEnv(cache): %s  KindEnv(config): %q (anchored)\n", d.KindEnv(appdir.KindCache), d.KindEnv(appdir.KindConfig))
	d, err = s.open(nil, appdir.WithEnvPrefix(prefix), appdir.WithDirName(dirName))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "with prefix %s: KindEnv(cache): %s\n", prefix, d.KindEnv(appdir.KindCache))
	fmt.Fprintf(w, "with dir name %s: global home %s\n", dirName, s.label(d.Global().Base))
	// The dir name must be one plain folder name: "..", "." or a path with a
	// separator would put the home outside the user's home.
	if _, err := s.open(nil, appdir.WithDirName(badDirName)); errors.Is(err, appdir.ErrInvalidOption) {
		// open adds its own "appdir:" prefix; print the library's message.
		fmt.Fprintf(w, "with dir name %s: error: %v\n", badDirName, errors.Unwrap(err))
	} else {
		return fmt.Errorf("dir name %q: want ErrInvalidOption, got %v", badDirName, err)
	}
	return nil
}
```

## Step 6 — Report a broken override

A `--home` or `<APP>_HOME` that cannot expand (an unset `${VAR}`, `~user`, or `~` on a machine with no user home) does not stop `New`. The error is stored and every kind returns it, the disposable cache included: a broken override never falls back to the global home or the temp folder, because writing anywhere other than where the user pointed would scatter their data. Check the error of the call you make and print it as is; it starts with the flag or variable name:

<!-- snippet: examples/overrides/main.go#broken -->
```go
// brokenHome shows a --home that names an unset variable: every kind fails
// with the same error, the disposable cache included. The temp fallback is
// only for a machine with no home folder and no override; a broken override
// never quietly writes somewhere else.
func (s setup) brokenHome(w io.Writer) error {
	d, err := s.open(nil, appdir.WithHome("${NOPE}/lore"))
	if err != nil {
		return err
	}
	for _, k := range []appdir.Kind{appdir.KindDB, appdir.KindCache} {
		p, err := d.Path(k)
		if err != nil {
			fmt.Fprintf(w, "%-6s error: %v\n", k, err)
			continue
		}
		fmt.Fprintf(w, "%-6s %s\n", k, s.label(p))
	}
	return nil
}
```

The last block of the output shows both kinds failing with `--home: ${NOPE} is not set`.

## Try it

```sh
go run ./examples/overrides
```

Paths are relative to the example's temp root:

```text
== no override
config  project   home/projects/shop/.lore/config.jsonc
db      project   home/projects/shop/.lore/lore.db
cache   global    home/.lore/cache
== --home ~/try
config  flag      home/try/config.jsonc
db      flag      home/try/lore.db
cache   flag      home/try/cache
== --home scratch (relative)
config  flag      home/projects/shop/scratch/config.jsonc
db      flag      home/projects/shop/scratch/lore.db
cache   flag      home/projects/shop/scratch/cache
== LORE_HOME=${FAST}/lore
config  env-home  fast/lore/config.jsonc
db      env-home  fast/lore/lore.db
cache   env-home  fast/lore/cache
with --home ~/try: global home/try (rule flag)
dev build, LORE_HOME=~/.lore/x: global home/.lore-dev (rule global)
  warning env-points-to-prod: LORE_HOME points into the prod home home/.lore; ignored for profile dev
== LORE_CACHE_DIR and LORE_DB_DIR
config  project   home/projects/shop/.lore/config.jsonc
db      env-kind  fast/db/lore.db
cache   env-kind  home/fast-cache
== env names
EnvPrefix: LORE  EnvName(HOME): LORE_HOME
KindEnv(cache): LORE_CACHE_DIR  KindEnv(config): "" (anchored)
with prefix NOTES: KindEnv(cache): NOTES_CACHE_DIR
with dir name .lore-data: global home home/.lore-data
with dir name ..: error: appdir: dir name ".." must be one plain path segment
== bad values
${FAST} unset:  error: LORE_CACHE_DIR: ${FAST} is not set
${FAST} empty:  error: LORE_CACHE_DIR: ${FAST} is not set
~user:          error: LORE_CACHE_DIR: "~bob/cache": the ~user form is not supported
$FAST literal:  cache: home/projects/shop/$FAST/cache
LORE_HOME empty: cache: home/.lore/cache
== --home with an unset ${VAR}
db     error: --home: ${NOPE} is not set
cache  error: --home: ${NOPE} is not set
```

## Pitfalls

- **A broken `--home` or `<APP>_HOME` stops cache-only commands too.** A command that only reads its cache used to work anywhere; under a broken override it fails like the rest (Step 6). `doctor` should print the error rather than skip the cache line.
- **A dev build silently keeps `~/.lore-dev` when `LORE_HOME` points into prod.** The only signal is the `env-points-to-prod` warning, so a tool that never prints `Warnings()` looks like it ignored the variable. Print warnings in `doctor` and `paths`.
- **`<APP>_<KIND>_DIR` wins even over `--home`.** The per-kind variable is checked before any home ([per-kind resolution](ds:block?id=dirs-entryin-zhkmadk3)), so a `LORE_DB_DIR` left in the shell still moves the db when the user passes `--home /tmp/try`. That is the order SPEC 4.7 sets; tell users to unset per-kind variables when they want `--home` to hold everything.
- **`Paths()` under an override lists both-homes kinds once.** With `--home` or `<APP>_HOME` there is one home, so `config`, `config-local` and `home` appear once, labelled `home: global` (as `ConfigFiles()` labels them), while project kinds such as `db` say `home: project`. Every line is the same folder; read the `rule` column (`flag` or `env-home`), not the `home` column, to tell an override.
- **`$VAR` without braces is not expanded.** `LORE_CACHE_DIR='$FAST/cache'` becomes a folder literally named `$FAST` under the working folder. Shells expand `$FAST` before your tool sees it unless it is quoted; config files never do. Write `${FAST}`.
- **A relative override follows the working folder.** `--home scratch` run from two folders gives two homes. Pass an absolute path or `~/…` in scripts.
- **Overrides are used as given for every profile.** A dev build with `--home` uses that folder, not `<folder>/profiles/dev` ([Profiles](../profiles/profiles.md)).
- **`WithDirName` and `WithEnvPrefix` are separate.** Renaming the folder does not rename the variables. `WithDirName` takes one plain folder name; `.`, `..`, an empty name or a name with a separator is `ErrInvalidOption` from `New` ([Global and project homes](global-and-project.md#pitfalls)).
- **Child processes inherit the variables.** A `LORE_HOME` set for a dev build leaks into a prod child unless you filter the environment ([Child processes](../profiles/child-processes.md)).

## Related

- [Global and project homes](global-and-project.md)
- [Fallbacks](fallbacks.md)
- [Kinds](../layout/kinds.md) (which kinds are anchored, adding kinds)
- [Options reference](../reference/options.md)
- Example: [examples/overrides](../../examples/overrides/README.md)
- SPEC: [4.1 Inputs](../SPEC.md#41-inputs), [4.3 Global home](../SPEC.md#43-global-home), [4.7 Per-kind result](../SPEC.md#47-per-kind-result), [5.3 Author-declared kinds](../SPEC.md#53-author-declared-kinds)
