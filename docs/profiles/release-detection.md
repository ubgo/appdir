# Release detection

A release build runs as the release profile (`prod`) and uses the real homes; every other build runs as `dev`. appdir decides with the standard library only: your own release flag when you pass one, otherwise Go's build info, where only a clean tagged `go install module@vX` counts as a release. Read this page when you set up your release build, or when an installed binary unexpectedly uses `~/.<app>-dev`.

## Before you start

- [Profiles](profiles.md): what the release and dev profiles change.
- Import `"github.com/ubgo/appdir"` and `"runtime/debug"` (the example fakes build info with `debug.BuildInfo`).
- Your release script, if you have one, sets a variable with `-ldflags "-X main.release=true"`; [examples/demo](../../examples/demo) shows that wiring.

## Step 1 — Pass your release flag, or let Go's build info decide

The rules, in order ([detectRelease](ds:block?id=detectrelease-7fwv9uc3)):

1. `WithRelease(true)` → release, reason `flag`. `WithRelease(false)` → dev, reason `dev: flag`. The flag always wins.
2. No build info → dev (`dev: no build info`).
3. `Main.Version` is empty or `(devel)` → dev. It has no `v` prefix → dev. It contains `+dirty` → dev. It is a pseudo-version (`vX.Y.Z-0.yyyymmddhhmmss-abcdef123456` and its `-pre.0.` / `.0.` forms) → dev.
4. Any `vcs.revision` setting → dev. A build inside a checkout always records one, even on a tag.
5. Otherwise → release, reason `go-install`: only `go install module@vX` from the module cache gets here.

Everything that is not clearly a release is dev, so a forgotten flag fails safe. The example feeds one fake `debug.BuildInfo` per kind of build through `WithBuildInfo`:

<!-- snippet: examples/profiles/main.go#release -->
```go
// releaseDetection feeds fake build info through WithBuildInfo, the way Go
// would report each kind of build, and prints the decision and its reason.
// Only a clean tagged version with no vcs.revision is a release: that is what
// `go install module@vX` produces. WithRelease, when given, wins.
func releaseDetection(w io.Writer, s sandbox) error {
	const tag = "v1.4.2"
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "4f1c2a9e"}
	info := func(version string, settings ...debug.BuildSetting) appdir.Option {
		return appdir.WithBuildInfo(func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: version}, Settings: settings}, true
		})
	}
	cases := []struct {
		label string
		opts  []appdir.Option
	}{
		{"go install module@v1.4.2", []appdir.Option{info(tag)}},
		{"go build in a checkout", []appdir.Option{info("(devel)", rev)}},
		{"go install ./cmd/demo on the tag", []appdir.Option{info(tag, rev)}},
		{"go install module@<commit>", []appdir.Option{info("v1.4.3-0.20261110093000-4f1c2a9e0b7d")}},
		{"modified checkout", []appdir.Option{info(tag + "+dirty")}},
		{"version without v", []appdir.Option{info("1.4.2")}},
		{"no build info", []appdir.Option{appdir.WithBuildInfo(func() (*debug.BuildInfo, bool) { return nil, false })}},
		{"-ldflags release=true", []appdir.Option{info("(devel)", rev), appdir.WithRelease(true)}},
		{"-ldflags release=false", []appdir.Option{info(tag), appdir.WithRelease(false)}},
	}
	for _, c := range cases {
		d, err := s.open(nil, c.opts...)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-34s release=%-5v reason=%-32q profile=%s\n", c.label, d.IsRelease(), d.ReleaseReason(), d.Profile())
	}
	return nil
}
```

## Step 2 — Report the decision

`IsRelease()` and `ReleaseReason()` say what was decided and why. Put both in your `version` or `doctor` output: when a user asks "why is my installed tool using `~/.demo-dev`?", the reason answers it (`dev: vcs.revision` means it was built in a checkout). The reason is the first rule that said dev, or `flag` / `go-install` for a release; it is set on every `Dirs`, including one from `ForProfile`, which keeps the build's decision.

The example builds one line from the two calls and prints it for a user's `go install module@v1.4.2` and for the same tag installed from a checkout:

<!-- snippet: examples/profiles/main.go#report -->
```go
// versionLine is what a `demo version` or `demo doctor` command prints: the
// profile, the release decision and the first rule that made it. When a user
// asks why the installed tool uses ~/.demo-dev, the reason answers it.
func versionLine(d *appdir.Dirs) string {
	return fmt.Sprintf("%s %s (release=%t, reason %q)", d.App(), d.Profile(), d.IsRelease(), d.ReleaseReason())
}

// versionLines prints the line for a user's `go install module@v1.4.2` and
// for the same tag installed from a checkout, which records vcs.revision.
func versionLines(w io.Writer, s sandbox) error {
	builds := []struct {
		label string
		info  debug.BuildInfo
	}{
		{"go install module@v1.4.2", debug.BuildInfo{Main: debug.Module{Version: "v1.4.2"}}},
		{"go install ./cmd/demo", debug.BuildInfo{
			Main:     debug.Module{Version: "v1.4.2"},
			Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "4f1c2a9e"}},
		}},
	}
	for _, b := range builds {
		d, err := s.open(nil, appdir.WithBuildInfo(func() (*debug.BuildInfo, bool) { return &b.info, true }))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-26s %s\n", b.label, versionLine(d))
	}
	return nil
}
```

Output:

```text
== version line
go install module@v1.4.2   demo prod (release=true, reason "go-install")
go install ./cmd/demo      demo dev (release=false, reason "dev: vcs.revision")
```

Print the line on stdout for `version`. For machine-readable output put the two values in your own JSON: [examples/demo](../../examples/demo) reports them as `release` and `release_reason`.

## Step 3 — Wire your release variable only when it is set

`WithRelease` always wins, in both directions. So declare a string variable that only your release script sets, and pass `WithRelease` only when it is not empty; a user's `go install module@vX` leaves it empty and the go-install rule still makes that binary a release. [examples/demo](../../examples/demo) does exactly this: `var release string` at package level, set by `go build -ldflags "-X main.release=true"`, then, before `appdir.New` (`releaseTrue` is the constant `"true"`):

<!-- snippet: examples/demo/main.go#release -->
```go
// Pass the release flag only when the release script set it: an
// unconditional WithRelease(false) would turn a user's
// `go install module@vX` into a dev build.
if release != "" {
	opts = append(opts, appdir.WithRelease(release == releaseTrue))
}
```

| Who builds it | `release` variable | Result |
|---|---|---|
| your release script, `-X main.release=true` | `"true"` | release, reason `flag` |
| a user, `go install module@v1.4.2` | empty | release, reason `go-install` |
| a developer, `go build` or `go run` | empty | dev, reason `dev: (devel)` or `dev: vcs.revision` |
| a script that wants a dev build on purpose, `-X main.release=false` | `"false"` | dev, reason `dev: flag` |

## Try it

```sh
go run ./examples/profiles
```

The release-detection section of the output:

```text
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
```

## Pitfalls

- `go install ./cmd/<app>` in your checkout, even on a release tag, is dev: the build info has `vcs.revision`. Your teammate's `go install module@v1.4.2` is prod. Two binaries of the same version can use different homes; this is intended.
- A release built by a script (`go build` in CI, goreleaser) has `vcs.revision` too, so it is dev unless the script sets your release variable and you pass it with `WithRelease`. Do that in every packaged release.
- The checks run in the order above, so a `(devel)` build reports `dev: (devel)` even when it also has `vcs.revision`; read the reason as "the first rule that said dev".
- `appdir.WithRelease(release == "true")` passed unconditionally is `WithRelease(false)` for every build that did not set the variable, including a user's `go install module@vX`: they get `~/.<app>-dev` and reason `dev: flag`. Pass it only when the variable is set (Step 3).
- Use a string variable, not a bool: `-ldflags -X` can only set string variables.
- Tests: a test binary has no tagged version, so it is dev. Pass `WithRelease` or `WithBuildInfo` when a test needs prod ([Testing setup](../testing/for-test.md)).

## Related

- [Profiles](profiles.md): detection order and the profile folders.
- [Options reference](../reference/options.md): `WithRelease`, `WithBuildInfo`.
- Example: [examples/profiles](../../examples/profiles).
- Design: [SPEC 8.2 Release detection](../SPEC.md#82-release-detection-stdlib-only).
