package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun checks the lines the init guide quotes: the ErrNoProject message,
// the home.jsonc content, the .gitignore block in a git work tree, the
// ErrNotHome refusal and the refusal to init the global home.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Path(db): no .lore/ here; run 'lore init' or pass --home",
		"created: home/projects/shop/.lore",
		`"scope": "project"`,
		".gitignore written: true",
		"project home: home/projects/shop/.lore (rule project)",
		"info: app=lore layout=1 scope=project",
		"InitHome(~/.lore, global): ok",
		`InitHome(home/team/notes, project): home/team/notes holds a home of "notes"`,
		"InitProject(~): init: home/.lore is the global home, not a project",
		`InitProject("packages/api"): home/projects/shop/packages/api/.lore`,
		"InitProject(home/projects/old): init: cannot read home/projects/old/.lore/home.jsonc; fix or remove it: jsonc: unexpected end of JSON input",
		"home.jsonc unchanged: true",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q\n%s", want, out.String())
		}
	}
}
