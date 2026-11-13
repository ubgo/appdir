package appdir

import (
	"fmt"
	"regexp"
	"strings"
)

// KindSpec describes one entry of the layout. Kinds() returns the specs in
// effect, so paths --json and doctor can show the names the author chose.
type KindSpec struct {
	Name       Kind      `json:"name"`
	Path       string    `json:"path"` // slash-separated, relative to the home
	Type       EntryType `json:"type"`
	Git        GitRule   `json:"git"`
	Scope      Scope     `json:"scope"` // "" until New fills in the mode's data scope
	Disposable bool      `json:"disposable"`
	Anchored   bool      `json:"anchored"` // lives in the home root, no env override
}

// KindOption changes one KindSpec inside WithKind.
type KindOption func(*KindSpec)

// Folder makes the kind a folder at p inside the home.
func Folder(p string) KindOption {
	return func(k *KindSpec) { k.Path, k.Type = p, TypeFolder }
}

// File makes the kind a file at p inside the home.
func File(p string) KindOption {
	return func(k *KindSpec) { k.Path, k.Type = p, TypeFile }
}

// Commit puts the kind into git.
func Commit() KindOption { return func(k *KindSpec) { k.Git = GitCommit } }

// Ignore keeps the kind out of git.
func Ignore() KindOption { return func(k *KindSpec) { k.Git = GitIgnore } }

// Scoped sets which home the kind lives in.
func Scoped(s Scope) KindOption { return func(k *KindSpec) { k.Scope = s } }

// Disposable marks the kind safe to delete (cache-like): it gets CACHEDIR.TAG
// and may fall back to the temp folder.
func Disposable() KindOption { return func(k *KindSpec) { k.Disposable = true } }

// Anchored keeps the kind in the home root of both profiles and gives it no
// <APP>_<KIND>_DIR: it is part of what makes a folder a home.
func Anchored() KindOption { return func(k *KindSpec) { k.Anchored = true } }

// standardKinds is the default layout (SPEC 5.1). Built per call: appdir keeps
// no package-level state. An empty Scope means "the mode's data scope".
// ds:def id=standardkinds-ptkt92ta owner=@khanakia stability=stable
func standardKinds(app string) []KindSpec {
	return []KindSpec{
		{Name: KindHome, Path: "home.jsonc", Type: TypeFile, Git: GitCommit, Scope: ScopeBoth, Anchored: true},
		{Name: KindConfig, Path: "config.jsonc", Type: TypeFile, Git: GitCommit, Scope: ScopeBoth, Anchored: true},
		{Name: KindConfigLocal, Path: "config.local.jsonc", Type: TypeFile, Git: GitIgnore, Scope: ScopeBoth, Anchored: true},
		{Name: KindGitignore, Path: ".gitignore", Type: TypeFile, Git: GitCommit, Scope: ScopeProject, Anchored: true},
		{Name: KindShared, Path: "shared", Type: TypeFolder, Git: GitCommit},
		{Name: KindDB, Path: app + ".db", Type: TypeFile, Git: GitIgnore},
		{Name: KindBackups, Path: "backups", Type: TypeFolder, Git: GitIgnore},
		{Name: KindCache, Path: "cache", Type: TypeFolder, Git: GitIgnore, Scope: ScopeGlobal, Disposable: true},
		{Name: KindRun, Path: "run", Type: TypeFolder, Git: GitIgnore},
		{Name: KindLogs, Path: "logs", Type: TypeFolder, Git: GitIgnore},
	}
}

// kindEdit is one WithKind / WithoutKind call, applied in order by New.
type kindEdit struct {
	name   Kind
	opts   []KindOption
	remove bool
}

// buildKinds applies the edits to the standard list and fills in data scopes.
// ds:def id=buildkinds-r6jj6d5s owner=@khanakia stability=stable
func buildKinds(app string, mode Mode, edits []kindEdit) ([]KindSpec, error) {
	kinds := standardKinds(app)
	nameRe := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	for _, e := range edits {
		if !nameRe.MatchString(string(e.name)) {
			return nil, newErr(ErrInvalidOption, "appdir: kind name %q must match %s", e.name, nameRe)
		}
		idx := -1
		for i := range kinds {
			if kinds[i].Name == e.name {
				idx = i
			}
		}
		if e.remove {
			if idx >= 0 {
				kinds = append(kinds[:idx], kinds[idx+1:]...)
			}
			continue
		}
		if idx < 0 {
			kinds = append(kinds, KindSpec{Name: e.name, Path: string(e.name), Type: TypeFolder, Git: GitIgnore})
			idx = len(kinds) - 1
		}
		for _, o := range e.opts {
			o(&kinds[idx])
		}
	}
	data := ScopeProject
	if mode == ModeGlobal {
		data = ScopeGlobal
	}
	for i := range kinds {
		k := &kinds[i]
		if err := validKindPath(k.Path); err != nil {
			return nil, fmt.Errorf("appdir: kind %q: %w", k.Name, err)
		}
		switch k.Scope {
		case "":
			k.Scope = data
			if k.Name == KindLogs && mode == ModeProject {
				k.Scope = ScopeProjectElseGlobal
			}
		case ScopeGlobal, ScopeProject, ScopeProjectElseGlobal, ScopeBoth:
		default:
			return nil, newErr(ErrInvalidOption, "appdir: kind %q: unknown scope %q", k.Name, k.Scope)
		}
	}
	return kinds, nil
}

// validKindPath rejects paths that could leave the home. The two checks
// together accept exactly the paths path.Clean leaves unchanged: no root,
// no empty segment (so no doubled or trailing slash) and no . or .. segment,
// so a separate "is it clean" test would never fire.
func validKindPath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.Contains(p, ":") {
		return newErr(ErrInvalidOption, "path %q must be relative and slash-separated", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return newErr(ErrInvalidOption, "path %q has an empty, . or .. segment", p)
		}
	}
	return nil
}
