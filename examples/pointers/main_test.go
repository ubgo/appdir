package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun runs the whole example in a test temp folder and checks the lines
// the pointer-keys guide quotes.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{
		"pointer points to $TMP/work/shop-private/lore, which does not exist; run 'lore init --home $TMP/work/shop-private/lore'",
		"project: $TMP/work/shop-private/lore (rule pointer)",
		`home: "$TMP/work/shop-private/lore" in $TMP/work/shop/.lore/config.jsonc is absolute; put it in config.local.jsonc`,
		"sentinel: ErrPointerAbsolute",
		"pointer points to $TMP/work/shop-db, which does not exist",
		"db:      $TMP/work/shop-db/lore.db",
		"backups: $TMP/work/shop/.lore/backups",
		"warning pointer-chain-ignored: home in $TMP/work/shop-private/lore/config.jsonc not followed: pointers go one hop only",
		"error: $TMP/work/shop/.lore/config.local.jsonc: home must be a string path",
		"sentinel: ErrInvalidOption",
		"Project: ErrPointerMissing\nglobal  $TMP/userhome/.lore/config.jsonc\nglobal  $TMP/userhome/.lore/config.local.jsonc\nproject $TMP/work/shop/.lore/config.jsonc\nproject $TMP/work/shop/.lore/config.local.jsonc\n",
		"== empty home: in config.local.jsonc\nprofile: prod\nproject: $TMP/work/shop-private/lore (rule pointer)",
		"data:    $TMP/work/shop-private/lore/profiles/dev",
		"warning pointer-ignored-for-profile: db: pointer in $TMP/work/shop/.lore/config.jsonc ignored for profile dev",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
