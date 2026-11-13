# Names and banner

A dev build must not collide with the installed tool anywhere outside its home either: not in a launchd or systemd unit, a keychain item, a named pipe or a lock name. `Name(base)` and `NameWith(base, sep)` give every such label the profile suffix (`demo-dev`, `com.example.demo.dev`) and leave it plain for the release profile. `Banner()` gives the one stderr line that tells the user they are not running prod. Use this page whenever your tool names something outside its folders.

## Before you start

- [Profiles](profiles.md): the release profile keeps plain names, every other profile gets a suffix.
- Import `"github.com/ubgo/appdir"`.

## Step 1 — Label everything with Name and NameWith

`Name(base)` returns `base` for the release profile and `base-<profile>` otherwise. `NameWith(base, sep)` does the same with your separator, for reverse-DNS labels ([NameWith](ds:block?id=dirs-namewith-uyg39uh6)). Route every label, unit, marker, pipe and item name through one of them, so `prod` and `dev` can be installed side by side.

Both are methods on the resolved `*Dirs`, so the profile they use is the one `New` picked. The example resolves each case with the same seams and adds only the options the case is about:

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

What to send through them:

| You name... | Use | Release profile | Dev profile |
|---|---|---|---|
| a launchd label, a bundle id, a D-Bus name | `NameWith("com.example.demo", ".")` | `com.example.demo` | `com.example.demo.dev` |
| a systemd unit, a Windows service, a scheduled task | `Name("demo") + ".service"` | `demo.service` | `demo-dev.service` |
| a keychain or credential-manager item | `Name("demo")` | `demo` | `demo-dev` |
| a lock, mutex or shared-memory name outside the home | `Name("demo")` | `demo` | `demo-dev` |
| a log tag or a process title | `Name("demo")` | `demo` | `demo-dev` |

appdir already does this for what it names itself: the socket fallback folder and the Windows pipe use `Name(app)`, the temp fallback folder is `<tmp>/<Name(app)>-<uid>/`, and the global home is `~/` plus `Name` of the home folder name.

## Step 2 — Print the banner once

`Banner()` returns `[<app>] profile: <profile> → <global home>` for a non-release profile, with the home shortened to `~/…` when it is under the user's home, and `""` for the release profile ([Banner](ds:block?id=dirs-banner-cwbb5293)). Print it to stderr at start-up when it is not empty; stdout stays clean for JSON.

<!-- snippet: examples/profiles/main.go#names -->
```go
// names labels everything a profile owns with Name/NameWith, so a dev build
// never collides with the installed one (service labels, pipe names, item
// names). Banner is the one stderr line a non-release profile prints.
func names(w io.Writer, s sandbox) error {
	for _, release := range []bool{true, false} {
		d, err := s.open(nil, appdir.WithRelease(release))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "profile %s: Name(%q)=%q NameWith(%q, \".\")=%q\n",
			d.Profile(), appName, d.Name(appName), serviceLabel, d.NameWith(serviceLabel, "."))
		fmt.Fprintf(w, "profile %s: Banner()=%q\n", d.Profile(), s.clean(d.Banner()))
	}
	return nil
}
```

## Step 3 — Name another profile's things

An `uninstall` or `doctor` command sometimes needs the label of a profile other than its own, for example to stop the dev service from a release build. Resolve that profile with `ForProfile` and ask it. `ForProfile` only resolves paths; it opens nothing and creates nothing ([Child processes](child-processes.md#step-2--pass-the-profile-to-child-processes)).

<!-- snippet: examples/profiles/main.go#other -->
```go
// otherNames is an uninstall command in the release build that must also
// stop the dev build's service: ForProfile resolves the dev profile (paths
// only, nothing is opened) and its Name/NameWith give the dev labels.
func otherNames(w io.Writer, s sandbox) error {
	d, err := s.open(nil, appdir.WithRelease(true))
	if err != nil {
		return err
	}
	other, err := d.ForProfile(appdir.ProfileDev)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "profile %s, ForProfile(%s): Name(%q)=%q NameWith(%q, \".\")=%q\n",
		d.Profile(), other.Profile(), appName, other.Name(appName), serviceLabel, other.NameWith(serviceLabel, "."))
	return nil
}
```

Output:

```text
== another profile's names
profile prod, ForProfile(dev): Name("demo")="demo-dev" NameWith("com.example.demo", ".")="com.example.demo.dev"
```

## Try it

```sh
go run ./examples/profiles
```

The names section of the output:

```text
== names and banner
profile prod: Name("demo")="demo" NameWith("com.example.demo", ".")="com.example.demo"
profile prod: Banner()=""
profile dev: Name("demo")="demo-dev" NameWith("com.example.demo", ".")="com.example.demo.dev"
profile dev: Banner()="[demo] profile: dev → ~/.demo-dev"
```

## Pitfalls

- The suffix depends on the profile, not on the build: a release binary run with `--profile dev` gets `demo-dev`, and a dev build run with `--profile prod` gets the plain `demo` (and with it the installed tool's labels).
- With no user home folder the banner says `(no home folder)` instead of a path. With `--home` or `<APP>_HOME`, it shows that folder, because the banner prints the global home in effect.
- A custom profile gets its own suffix: with `WithProfiles(prod, dev, "staging")`, `--profile staging` gives `demo-staging`. A custom release profile (`WithReleaseProfile`) is the one that keeps plain names, whatever it is called.
- `Name` adds a suffix; it does not check the result. A label with characters a service manager refuses stays refused; pick a base that is valid with `-<profile>` appended.
- `ForProfile` takes a profile from the list the tool allows (`WithProfiles`); any other name fails with `ErrUnknownProfile`, so an uninstall loop over profiles must use that same list.
- Print the banner to stderr, never stdout: a script reading `paths --json` must get clean JSON in every profile.
- appdir uses `Name(app)` itself for the socket fallback folder and the Windows pipe name ([Run files and sockets](../runtime/run-files-and-sockets.md)); the global home folder uses `Name(<dir name>)`, so `WithDirName("data")` gives `~/data-dev`.

## Related

- [Profiles](profiles.md) and [Child processes and env leaks](child-processes.md).
- Example: [examples/profiles](../../examples/profiles).
- Design: [SPEC 8.3 Names](../SPEC.md#83-names).
