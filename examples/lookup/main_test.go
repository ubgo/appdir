package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun checks each lookup case resolves to the home (or the error) the
// lookup guide shows.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	const none = "  -> no .lore/ here; run 'lore init' or pass --home"
	for _, want := range []string{
		"walk-up:                        from home/projects/shop/src/api\n  -> home/projects/shop/.lore (rule project)",
		"repo root is a wall:            from home/projects/mono/vendor/lib/src\n" + none,
		"stops below $HOME:              from home/projects/notes/sub\n" + none,
		"global home is never a project: from home\n" + none,
		"no ceiling:                     from work/a/b\n  -> work/.lore (rule project)",
		"ceiling dirs:                   from work/a/b\n" + none,
		`warning ceiling-relative: LORE_CEILING_DIRS entry "projects" is relative; ignored`,
		"real path of a symlink:         from home/projects/api-link\n  -> home/projects/shop/.lore (rule project)",
		"nearest wins:                   from home/projects/mono/pkg/web/src\n  -> home/projects/mono/pkg/web/.lore (rule project)",
		"here-only, deep folder:         from home/projects/shop/src/api\n" + none,
		"here-only, project root:        from home/projects/shop\n  -> home/projects/shop/.lore (rule project)",
		"dangling .git is a wall:        from work/repo/src\n" + none,
		"start at a ceiling:             from work/a/b\n" + none,
		"$VAR ceiling is ignored:        from work/a/b\n  -> work/.lore (rule project)\n  warning ceiling-dropped: LORE_CEILING_DIRS entry \"$ROOT/work/a\" is not an absolute path (only ${VAR} expands, not $VAR); ignored",
		none + "\n     cache still works: home/.lore/cache\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q\n%s", want, out.String())
		}
	}
}
