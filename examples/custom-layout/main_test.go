package main

import (
	"bytes"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// modeLine matches a line that quotes a Unix mode such as 0700.
var modeLine = regexp.MustCompile(`\b0[0-7]{3}\b`)

// TestRun runs the whole example in a test temp folder and checks the lines
// the layout guides quote.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{
		"shared       data               folder commit project             LORE_SHARED_DIR",
		"identity     identity.json      file   ignore global              LORE_IDENTITY_DIR",
		"schema       schema.jsonc       file   commit project             -                  anchored",
		"global  db=global logs=global cache=global",
		`error: appdir: kind "escape": path "../outside" has an empty, . or .. segment`,
		"!data/\n!data/**\n!templates/\n!templates/**\n!schema.jsonc\n!prompts/\n!prompts/*.md\ndata/private/\n# lore:end",
		"# hand-written lines stay\n# lore:begin",
		"!glossary.md\ndata/private/\n# lore:end\nnotes.txt",
		"shared git rule: ignore; block re-includes data/: false",
		`Path(backups): appdir: unknown kind "backups"`,
		"is ErrUnknownKind: true",
		"init $TMP/plain/.lore: folder 0700, 0 files, home.jsonc missing",
		"is ErrGitignoreMarkers: true",
		"folder 0700  CACHEDIR.TAG true",
		"home.jsonc file 0600",
		"templates after chmod 0750 and Ensure: 0750",
		"links to $TMP/outside/stolen.db, outside the home $TMP/shop/.lore",
		"is ErrSymlinkEscape: true",
		"previews  $TMP/shop/.lore/previews           folder 0750  CACHEDIR.TAG false",
		"InitHome $TMP/team-home: folder 0750, home.jsonc 0640",
		"missing note: <nil>",
		"linked note: $TMP/shop/.lore/data/note.md links to $TMP/outside/note.md, outside the home $TMP/shop/.lore",
		"home $TMP/shop/.lore has layout 2, this binary knows 1; upgrade lore",
		"Ensure(run): <nil>",
	} {
		// Windows has no mode bits (appdir sets an ACL there), so lines
		// that quote a mode are checked on Unix only.
		if runtime.GOOS == "windows" && modeLine.MatchString(want) {
			continue
		}
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
	if strings.Contains(strings.Split(got, "== Ignore()")[0], "backups") {
		t.Errorf("WithoutKind(backups) left it in the layout\n%s", got)
	}
}
