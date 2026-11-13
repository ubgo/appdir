package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun runs the walk-through in a test folder and checks the lines the
// guide relies on: the name rule, the two homes, the effective-home rule of
// Path, and that only Ensure creates the global home.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`New: appdir: app name "My App" must match ^[a-z0-9][a-z0-9._-]*$`,
		"app:          lore\n",
		"global home:  home/.lore (rule global, fallback false, info false)",
		"project home: home/projects/shop/.lore (rule project, data home/projects/shop/.lore)",
		"project info: app=lore layout=1 scope=project",
		"Path(db): home/projects/shop/.lore/lore.db",
		"Path(cache): home/.lore/cache",
		"PathIn(config, global): home/.lore/config.jsonc",
		"cache         global   global   home/.lore/cache",
		"global home exists: false",
		"Ensure(db): home/projects/shop/.lore/lore.db",
		"global home exists: true",
		"release=true  profile prod global home home/.lore (rule global)",
		"release=false profile dev  global home home/.lore-dev (rule global)",
		"project db     error: no .lore/ here; run 'lore init' or pass --home",
		"project logs   global  fallback=true  home/.lore/logs",
		"global  db     global  fallback=false home/.lore/lore.db",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q\n%s", want, out.String())
		}
	}
}
