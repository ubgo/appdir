package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun runs the walk-through in a test folder and checks the lines the
// guide relies on: refusal, read-only with DEMO_ALLOW_PROD=1, read-write for
// the dev build's own db and for a release build, a fresh prod home and a
// relative path (both refused), the env-leak warning, and the db: pointer
// followed by the prod view.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	const refused = "dev build refuses the prod database "
	for _, want := range []string{
		"dev build, home/.demo/demo.db (folder missing) -> " + refused + "home/.demo/demo.db",
		"dev build, .demo/demo.db from home/proj -> " + refused + "home/proj/.demo/demo.db",
		"dev build, prod global db                        home/.demo/demo.db -> " + refused + "home/.demo/demo.db; set DEMO_ALLOW_PROD=1 to open it read-only",
		"dev build, prod project db                       home/proj/.demo/demo.db -> " + refused + "home/proj/.demo/demo.db",
		"dev build + DEMO_ALLOW_PROD=1, prod project db   home/proj/.demo/demo.db -> read-only",
		"dev build, its own db                            home/proj/.demo/profiles/dev/demo.db -> read-write",
		"release build, prod project db                   home/proj/.demo/demo.db -> read-write",
		"warning env-points-to-prod: DEMO_HOME points into the prod home home/.demo; ignored for profile dev",
		"global home: home/.demo-dev",
		"open home/.demo/demo.db: " + refused,
		"warning pointer-ignored-for-profile: db: pointer in home/proj/.demo/config.local.jsonc ignored for profile dev",
		"dev build, shared-data/team.db -> " + refused + "shared-data/team.db",
		"dev build, home/proj/.demo/profiles/dev/demo.db -> read-write",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q\n%s", want, out.String())
		}
	}
}
