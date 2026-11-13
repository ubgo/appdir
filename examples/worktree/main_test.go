package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun checks each worktree case resolves to the home, rule and warning
// the worktrees guide shows.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"main checkout:           from shop/src\n  -> home/projects/shop/.lore (rule project)",
		"linked worktree:         from shop-fix/src\n  -> home/projects/shop/.lore (rule worktree)\n     note: main checkout of a linked git worktree",
		"main has no .lore:       from blog-fix\n  -> home/projects/blog-fix/.lore (rule project)\n  warning worktree-main-missing: linked worktree of home/projects/blog, but the main checkout has no .lore; using the worktree",
		// No home in either place: no warning, because there is nothing to fall
		// back to; the next case starts right after the error line.
		"neither has .lore:       from wiki-fix\n  -> no .lore/ here; run 'lore init' or pass --home\nsubmodule:",
		"submodule:               from shop/vendor/lib\n  -> home/projects/shop/vendor/lib/.lore (rule project)",
		"worktree of a bare repo: from tool-wt\n  -> home/projects/tool-wt/.lore (rule project)",
		"here-only in a worktree: from shop-fix\n  -> home/projects/shop/.lore (rule worktree)",
		"using the worktree\n     hint: run 'lore init' in the main checkout to share one home\n",
		"home only in worktree:   from shop-fix/pkg\n  -> home/projects/shop/.lore (rule worktree)",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q\n%s", want, out.String())
		}
	}
}
