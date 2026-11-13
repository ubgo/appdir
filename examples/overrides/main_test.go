package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun checks the lines the overrides guide quotes for each flag,
// variable and bad value.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"db      flag      home/try/lore.db",
		"cache   flag      home/projects/shop/scratch/cache",
		"db      env-home  fast/lore/lore.db",
		"with --home ~/try: global home/try (rule flag)",
		"dev build, LORE_HOME=~/.lore/x: global home/.lore-dev (rule global)",
		"  warning env-points-to-prod: LORE_HOME points into the prod home home/.lore; ignored for profile dev",
		"db      env-kind  fast/db/lore.db",
		"cache   env-kind  home/fast-cache",
		"config  project   home/projects/shop/.lore/config.jsonc",
		`KindEnv(cache): LORE_CACHE_DIR  KindEnv(config): "" (anchored)`,
		"with prefix NOTES: KindEnv(cache): NOTES_CACHE_DIR",
		"with dir name .lore-data: global home home/.lore-data",
		"${FAST} unset:  error: LORE_CACHE_DIR: ${FAST} is not set",
		"${FAST} empty:  error: LORE_CACHE_DIR: ${FAST} is not set",
		`~user:          error: LORE_CACHE_DIR: "~bob/cache": the ~user form is not supported`,
		"$FAST literal:  cache: home/projects/shop/$FAST/cache",
		"LORE_HOME empty: cache: home/.lore/cache",
		"db     error: --home: ${NOPE} is not set",
		"cache  error: --home: ${NOPE} is not set",
		`with dir name ..: error: appdir: dir name ".." must be one plain path segment`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q\n%s", want, out.String())
		}
	}
}
