package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun runs the walk-through in a test folder and checks the lines the
// guides rely on: detection order and its env limits, every release reason,
// the version line, custom profiles, the dev folders, names (own and another
// profile's), the env-leak filter and the child-process helpers.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"dev build                                        -> profile dev",
		"release build                                    -> profile prod",
		"dev build, --profile prod                        -> profile prod",
		"warning profile-env-ignored: DEMO_PROFILE=prod ignored by a dev build; set DEMO_ALLOW_PROD=1 too, or pass --profile",
		"dev build, DEMO_PROFILE=prod, DEMO_ALLOW_PROD=1  -> profile prod",
		"warning profile-env-ignored: DEMO_PROFILE=dev ignored by a release build; pass --profile dev",
		"release build, --profile dev                     -> profile dev",
		`error: appdir: --profile: unknown profile "qa" (known: prod, dev)`,
		`error: appdir: DEMO_PROFILE: unknown profile "qa" (known: prod, dev)`,
		`release=true  reason="go-install"`,
		`reason="dev: (devel)"`,
		`reason="dev: vcs.revision"`,
		`reason="dev: pseudo-version"`,
		`reason="dev: +dirty"`,
		`reason="dev: version without v prefix"`,
		`reason="dev: no build info"`,
		`release=true  reason="flag"`,
		`reason="dev: flag"`,
		`go install module@v1.4.2   demo prod (release=true, reason "go-install")`,
		`go install ./cmd/demo      demo dev (release=false, reason "dev: vcs.revision")`,
		"--profile staging: profile staging, global home/.demo-staging, db home/proj/.demo/profiles/staging/demo.db",
		"WithDevProfile(staging), dev build: profile staging",
		"WithReleaseProfile(stable), release build: profile stable, global home/.demo",
		"  db     home/proj/.demo/profiles/dev/demo.db",
		"  cache  home/.demo-dev/cache",
		"  config home/proj/.demo/config.jsonc",
		`profile dev: Name("demo")="demo-dev" NameWith("com.example.demo", ".")="com.example.demo.dev"`,
		`profile prod: Banner()=""`,
		`profile dev: Banner()="[demo] profile: dev → ~/.demo-dev"`,
		`profile prod, ForProfile(dev): Name("demo")="demo-dev" NameWith("com.example.demo", ".")="com.example.demo.dev"`,
		"warning env-points-to-prod: DEMO_HOME points into the prod home home/.demo; ignored for profile dev",
		"warning env-points-to-prod: DEMO_DB_DIR points into the prod home home/.demo; ignored for profile dev",
		"profile dev: global home/.demo-dev, db home/proj/.demo/profiles/dev/demo.db",
		"ChildEnv: PATH=/usr/bin DEMO_HOME=~/.demo DEMO_PROFILE=dev",
		"CleanEnv: PATH=/usr/bin",
		"ForProfile(prod): profile prod, global home/.demo, db home/proj/.demo/demo.db",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q\n%s", want, out.String())
		}
	}
}
