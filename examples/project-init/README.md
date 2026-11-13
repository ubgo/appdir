# project-init

The `init` command: `InitProject` creates `.lore/` with `home.jsonc` and the `.gitignore` block in a git work tree, `InitHome` creates homes at chosen paths (a no-op on this app's home, `ErrNotHome` on another app's), and `InitProject` refuses the global home. The last two blocks show a relative folder joined to the working folder, and `InitProject` refusing a `home.jsonc` that does not parse while leaving it untouched. It runs in a temp root with a fake user home and a project folder holding a `.git` folder (git is not run).

## Run

```sh
go run ./examples/project-init
```

Output (paths relative to the temp root):

```text
== before init
HasProject: false
Path(db): no .lore/ here; run 'lore init' or pass --home
== InitProject
created: home/projects/shop/.lore
home.jsonc:
{
  "app": "lore",
  "layout": 1,
  "scope": "project"
}
.gitignore written: true
== after init
project home: home/projects/shop/.lore (rule project)
info: app=lore layout=1 scope=project
== InitHome
InitHome(~/.lore, global): ok
InitHome(home/team/lore, project): ok
InitHome(home/team/lore, project): ok
InitHome(home/team/notes, project): home/team/notes holds a home of "notes"
== InitProject on the global home
InitProject(~): init: home/.lore is the global home, not a project
== InitProject with a relative folder
InitProject("packages/api"): home/projects/shop/packages/api/.lore
== InitProject over a broken home.jsonc
InitProject(home/projects/old): init: cannot read home/projects/old/.lore/home.jsonc; fix or remove it: jsonc: unexpected end of JSON input
home.jsonc unchanged: true
```

`go test ./examples/project-init` runs the same program in a test folder and checks these lines.

## Guide

- [Create a home with init](../../docs/start/init.md)
