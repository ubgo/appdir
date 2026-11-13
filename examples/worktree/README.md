# worktree

Git worktrees and submodules without git: the example writes the `.git` file and the `gitdir` / `commondir` files exactly as `git worktree add` and `git submodule add` do, then shows that a linked worktree uses the main checkout's `.lore/` (rule `worktree`), that a main checkout without one makes the worktree use its own with warning `worktree-main-missing` (and no warning when neither has one), that a submodule and a worktree of a bare repo are their own projects, and that `HereOnly` still follows a worktree.

## Run

```sh
go run ./examples/worktree
```

Output (paths relative to the temp root):

```text
main checkout:           from shop/src
  -> home/projects/shop/.lore (rule project)
linked worktree:         from shop-fix/src
  -> home/projects/shop/.lore (rule worktree)
     note: main checkout of a linked git worktree
main has no .lore:       from blog-fix
  -> home/projects/blog-fix/.lore (rule project)
  warning worktree-main-missing: linked worktree of home/projects/blog, but the main checkout has no .lore; using the worktree
     hint: run 'lore init' in the main checkout to share one home
neither has .lore:       from wiki-fix
  -> no .lore/ here; run 'lore init' or pass --home
submodule:               from shop/vendor/lib
  -> home/projects/shop/vendor/lib/.lore (rule project)
worktree of a bare repo: from tool-wt
  -> home/projects/tool-wt/.lore (rule project)
here-only in a worktree: from shop-fix
  -> home/projects/shop/.lore (rule worktree)
     note: main checkout of a linked git worktree
home only in worktree:   from shop-fix/pkg
  -> home/projects/shop/.lore (rule worktree)
     note: main checkout of a linked git worktree
```

`go test ./examples/worktree` runs the same program in a test folder and checks these lines.

## Guide

- [Worktrees and submodules](../../docs/homes/worktrees.md)
