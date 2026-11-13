// Package appdir decides where a tool keeps its things: the global home
// (~/.<app>), the project home (.<app>/ found by walking up), the folders and
// files inside them, the prod/dev profile split, pointer keys, and moving an
// old home. It implements the folder parts of the app standard; the full
// contract is docs/SPEC.md.
//
// New only reads: it stats folders and reads home.jsonc and config pointer
// keys, and never creates anything. Folders appear on the first Ensure.
// Every resolved path records the Rule that picked it, so a tool's
// `paths --json` and `doctor` can say why a path is what it is.
//
//	d, err := appdir.New("lore", appdir.WithHome(flagHome), appdir.WithProfile(flagProfile))
//	if err != nil { return err }
//	db, err := d.Ensure(appdir.KindDB) // creates .lore/ folders lazily; ErrNoProject outside a project
//
// appdir keeps no package-level mutable state; every outside input (env,
// home dir, working dir, build info) has a seam option.
package appdir
