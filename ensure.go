package appdir

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Ensure creates a kind's folder (or a file kind's parent folder) on first
// use and returns the kind's path. It is the lazy creation point: reads never
// create anything (SPEC 4.8).
// ds:def id=dirs-ensure-mxbz3fmd owner=@khanakia stability=stable
func (d *Dirs) Ensure(k Kind) (string, error) {
	spec, ok := d.kind(k)
	if !ok {
		return "", newErr(ErrUnknownKind, "appdir: unknown kind %q", k)
	}
	e, _ := d.entry(k)
	if e.err != nil {
		return "", e.err
	}
	if e.Path == "" {
		return "", newErr(ErrNoHome, "appdir: %s has no path (%s)", k, e.Note)
	}
	path, err := d.ensureEntry(spec, e)
	if err != nil && spec.Disposable && d.cfg.tempFallback && e.Rule != RuleTemp && isReadOnlyErr(err) {
		path, err = d.ensureEntry(spec, d.entryIn(spec, e.Home, true))
	}
	return path, err
}

// ensureEntry does the checks and creation for one resolved entry.
func (d *Dirs) ensureEntry(spec KindSpec, e Entry) (string, error) {
	if err := d.checkWriteIn(e.homeRoot, e.infoDir, e.info); err != nil {
		return "", err
	}
	folder := e.Path
	if spec.Type == TypeFile {
		folder = filepath.Dir(e.Path)
	}
	if e.Rule == RuleTemp {
		if err := d.ensurePrivateDir(e.homeRoot); err != nil {
			return "", err
		}
	}
	created, err := d.mkdirAll(folder)
	if err != nil {
		return "", fmt.Errorf("create %s: %w (set %s or %s)", folder, err, d.cfg.envName(envHome), d.KindEnv(spec.Name))
	}
	if spec.Disposable && spec.Type == TypeFolder && d.cfg.cacheTag && (created || !exists(filepath.Join(folder, cacheTagFile))) {
		if err := d.tagCache(folder); err != nil {
			return "", err
		}
	}
	if spec.Type == TypeFile {
		if err := d.VerifyFile(e.Path); err != nil {
			return "", err
		}
	}
	return e.Path, nil
}

// checkWrite applies the root and layout rules before any write, for a
// home whose home.jsonc (info) sits in homeRoot itself.
// ds:def id=dirs-checkwrite-n23fhubq owner=@khanakia stability=stable
func (d *Dirs) checkWrite(homeRoot string, info *HomeInfo) error {
	return d.checkWriteIn(homeRoot, homeRoot, info)
}

// checkWriteIn is checkWrite when home.jsonc was read from infoDir, which
// can differ from homeRoot (a profile's data folder, a db: pointer folder):
// the layout error names the folder that holds home.jsonc, the one the user
// has to look at (F15).
func (d *Dirs) checkWriteIn(homeRoot, infoDir string, info *HomeInfo) error {
	if info != nil && info.Layout > d.cfg.layoutVersion {
		if infoDir == "" {
			infoDir = homeRoot
		}
		return newErr(ErrLayoutTooNew, "home %s has layout %d, this binary knows %d; upgrade %s", infoDir, info.Layout, d.cfg.layoutVersion, d.cfg.app)
	}
	if !isRoot() || d.cfg.allowRoot || homeRoot == "" {
		return nil
	}
	p := homeRoot
	for !exists(p) {
		parent := filepath.Dir(p)
		if parent == p {
			return nil
		}
		p = parent
	}
	if uid, ok := ownerUID(p); ok && uid != 0 {
		return newErr(ErrRootOwned, "running as root but %s belongs to uid %d; pass --allow-root", p, uid)
	}
	return nil
}

// mkdirAll creates folder and missing parents with the dir mode, restricting
// each folder it created. It reports whether folder itself was created.
func (d *Dirs) mkdirAll(folder string) (bool, error) {
	if isDir(folder) {
		return false, nil
	}
	var missing []string
	for p := folder; !exists(p); {
		missing = append(missing, p)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	if err := os.MkdirAll(folder, d.cfg.dirPerm); err != nil {
		return false, err
	}
	for _, p := range missing {
		if err := restrict(p, d.cfg.dirPerm, true); err != nil {
			return true, err
		}
	}
	return true, nil
}

// ensurePrivateDir creates a per-user folder in a shared place (temp
// fallback, socket fallback): 0700, never a symlink, owned by us.
// ds:def id=dirs-ensureprivatedir-ars7qvkj owner=@khanakia stability=stable
func (d *Dirs) ensurePrivateDir(dir string) error {
	fi, err := os.Lstat(dir)
	if isNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dir), d.cfg.dirPerm); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(dir), err)
		}
		if err := os.Mkdir(dir, d.cfg.dirPerm); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		if err := restrict(dir, d.cfg.dirPerm, true); err != nil {
			return err
		}
		fi, err = os.Lstat(dir)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return newErr(ErrUnsafeTemp, "%s is a symlink or not a folder", dir)
	}
	if !ownedByMe(dir) {
		return newErr(ErrUnsafeTemp, "%s is not owned by you", dir)
	}
	return nil
}

// isReadOnlyErr reports a read-only or permission failure.
func isReadOnlyErr(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EROFS)
}

// InitProject creates <dir>/.<app>/ with home.jsonc and, inside a git work
// tree, the .gitignore block: the `<app> init` command. A project home is
// never created any other way. Returns the new marker folder.
// ds:def id=dirs-initproject-wpmefq6e owner=@khanakia stability=stable
func (d *Dirs) InitProject(dir string) (string, error) {
	// A relative dir is relative to the working folder New resolved from
	// (WithWorkDir), like every other user path, never to whatever the
	// process cwd happens to be when init runs.
	abs := filepath.Clean(dir)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(d.workDir, abs)
	}
	marker := filepath.Join(abs, d.cfg.dirName)
	if d.isGlobalHome(marker) {
		return "", newErr(ErrInvalidOption, "init: %s is the global home, not a project", marker)
	}
	if err := d.initHome(marker, ScopeProject); err != nil {
		return "", err
	}
	if _, err := d.writeGitignoreAt(marker); err != nil {
		return marker, err
	}
	return marker, nil
}

// InitHome creates a home at path with home.jsonc for scope: the
// `<app> init --home X` command, also used to create a pointer target.
// ds:def id=dirs-inithome-934qhf2j owner=@khanakia stability=stable
func (d *Dirs) InitHome(path string, s Scope) error {
	if s != ScopeGlobal && s != ScopeProject {
		return newErr(ErrInvalidOption, "appdir: InitHome needs global or project, got %q", s)
	}
	p, err := d.cfg.expandUserPath(path, d.workDir, d.userHome, d.userHomeErr)
	if err != nil {
		return err
	}
	return d.initHome(p, s)
}

func (d *Dirs) initHome(dir string, s Scope) error {
	spec, hasHomeFile := d.kind(KindHome)
	homeFile := filepath.Join(dir, filepath.FromSlash(spec.Path))
	// A home.jsonc that exists but cannot be read is the user's file:
	// refuse instead of treating it as absent and replacing it. The message
	// names the file once (it already contains dir) and wraps both
	// ErrBadHomeFile and the reader's error (finding D6).
	if hasHomeFile {
		if _, err := d.cfg.readConfig(homeFile); err != nil && !isNotExist(err) {
			return wrapErr(ErrBadHomeFile, err, "init: cannot read %s; fix or remove it: %s", homeFile, detailWithoutPath(err, homeFile))
		}
	}
	info := d.readHomeInfo(dir)
	if info != nil && info.App != "" && info.App != d.cfg.app {
		return newErr(ErrNotHome, "%s holds a home of %q", dir, info.App)
	}
	if err := d.checkWrite(dir, info); err != nil {
		return err
	}
	if _, err := d.mkdirAll(dir); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if info != nil || !hasHomeFile {
		return nil // already a home of this app, or a tool that has no home file
	}
	b, err := json.MarshalIndent(HomeInfo{App: d.cfg.app, Layout: d.cfg.layoutVersion, Scope: s}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", homeFile, err)
	}
	// The home kind may be renamed to a nested path (meta/home.json).
	if _, err := d.mkdirAll(filepath.Dir(homeFile)); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(homeFile), err)
	}
	return d.writeFileAtomic(homeFile, append(b, '\n'), d.cfg.filePerm)
}
