package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun runs the whole example in a test temp folder and checks the lines
// the config-files guide quotes.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{
		"3. project $TMP/shop/.lore/config.jsonc committed\n4. project $TMP/shop/.lore/config.local.jsonc local\nport = 4000",
		"5. project $TMP/shop/.lore/profiles/dev/config.local.jsonc local profile=dev\nport = 4100",
		"5. project $TMP/shop-private/lore/config.jsonc committed\n6. project $TMP/shop-private/lore/config.local.jsonc local missing\nport = 5000",
		"7. project $TMP/shop-private/lore/profiles/dev/config.local.jsonc local profile=dev missing",
		"== --home (profile prod)\n1. global  $TMP/elsewhere/config.jsonc missing\n2. global  $TMP/elsewhere/config.local.jsonc local missing\nport unset (code default)",
		"== no $HOME (profile prod)\n1. project $TMP/shop/.lore/config.jsonc committed",
		"== WithoutKind(config-local) (profile prod)\n1. global  $TMP/userhome/.lore/config.jsonc\n2. project $TMP/shop/.lore/config.jsonc committed\nport = 3000",
		"read $TMP/shop/.lore/config.local.jsonc\nread $TMP/shop-private/lore/home.jsonc",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
