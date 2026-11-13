package appdir

import (
	"fmt"
	"path/filepath"
	"strings"
)

// pointerValue is one home:/db: key and the file it came from.
type pointerValue struct {
	raw       string
	file      string
	committed bool
}

// applyPointers reads home:/db: from the marker's config.jsonc and
// config.local.jsonc (local wins per key) and applies them (SPEC 4.5). It
// returns the file of a db: pointer that a non-release profile ignored ("" if
// none), so the caller can warn with the real path once Data is known.
// ds:def id=dirs-applypointers-hqmm53fn owner=@khanakia stability=stable
func (d *Dirs) applyPointers() (ignoredDB string, err error) {
	marker := d.project.Marker
	cfgSpec, _ := d.kind(KindConfig)
	localSpec, _ := d.kind(KindConfigLocal)
	vals := map[string]pointerValue{}
	for _, f := range []struct {
		path      string
		committed bool
	}{
		{filepath.Join(marker, filepath.FromSlash(cfgSpec.Path)), true},
		{filepath.Join(marker, filepath.FromSlash(localSpec.Path)), false},
	} {
		if f.path == marker {
			continue // a dropped config kind
		}
		m, err := d.cfg.readConfig(f.path)
		if isNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("read %s: %w", f.path, err)
		}
		for _, key := range []string{pointerKeyHome, pointerKeyDB} {
			v, ok := m[key]
			if !ok || v == nil {
				continue
			}
			s, ok := v.(string)
			if !ok {
				return "", newErr(ErrInvalidOption, "%s: %s must be a string path", f.path, key)
			}
			if s == "" {
				continue
			}
			vals[key] = pointerValue{raw: s, file: f.path, committed: f.committed}
		}
	}

	if v, ok := vals[pointerKeyHome]; ok {
		target, err := d.pointerPath(pointerKeyHome, v)
		if err != nil {
			return "", err
		}
		if !isDir(target) {
			return "", newErr(ErrPointerMissing, "pointer points to %s, which does not exist; run '%s init --home %s'", target, d.cfg.app, target)
		}
		if err := d.checkPointerTarget(target); err != nil {
			return "", err
		}
		d.warnPointerChain(target)
		d.project.Base, d.project.Rule = target, RulePointer
		d.project.Note = "home: pointer in " + v.file
		d.targetConfig = target
	}
	if v, ok := vals[pointerKeyDB]; ok {
		if d.profile != d.cfg.releaseProfile {
			return v.file, nil
		}
		target, err := d.pointerPath(pointerKeyDB, v)
		if err != nil {
			return "", err
		}
		if !isDir(filepath.Dir(target)) {
			return "", newErr(ErrPointerMissing, "pointer points to %s, which does not exist", filepath.Dir(target))
		}
		d.dbPointer = target
	}
	return "", nil
}

// checkPointerTarget refuses a home: target that is not a home of this app
// (standard section 4: "if the target exists and has no home.jsonc for this
// app, refuse"), so a pointer can never turn a random folder into a home.
//
// With the home kind (the default) the target's home.jsonc must name this
// app. With WithoutKind(KindHome) no home.jsonc is ever written, so reading
// it would refuse every target (finding D3); the target is then recognised
// the way the lookup recognises a project home without that file, by its
// folder name: it must be named .<app> (WithDirName). The rule is
// deliberately not "any existing folder": a typo or a random folder is
// still ErrPointerNotHome, and nothing is written to mark it.
func (d *Dirs) checkPointerTarget(target string) error {
	if _, hasHomeFile := d.kind(KindHome); !hasHomeFile {
		if filepath.Base(target) != d.cfg.dirName {
			return newErr(ErrPointerNotHome, "pointer target %s is not a %s home (without home.jsonc, a home is a folder named %s)", target, d.cfg.app, d.cfg.dirName)
		}
		return nil
	}
	if info := d.readHomeInfo(target); info == nil || info.App != d.cfg.app {
		return newErr(ErrPointerNotHome, "pointer target %s is not a %s home (no home.jsonc with app: %s)", target, d.cfg.app, d.cfg.app)
	}
	return nil
}

// warnPointerProfile records pointer-ignored-for-profile naming the path the
// profile's database really resolves to (its data folder, or
// <APP>_DB_DIR), not just "under profiles" (F13). Called once Data is set.
func (d *Dirs) warnPointerProfile(file string) {
	where := d.project.Data
	if spec, ok := d.kind(KindDB); ok {
		if e := d.entryIn(spec, ScopeProject, false); e.Path != "" {
			where = e.Path
		}
	}
	d.warn(WarnPointerProfile, fmt.Sprintf("db: pointer in %s ignored for profile %s; the %s database stays at %s", file, d.profile, d.profile, where))
}

// pointerPath checks the committed-file rule and expands the value.
func (d *Dirs) pointerPath(key string, v pointerValue) (string, error) {
	if v.committed && !portablePointer(v.raw) {
		return "", newErr(ErrPointerAbsolute, "%s: %q in %s is absolute; put it in config.local.jsonc", key, v.raw, v.file)
	}
	p, err := d.cfg.expandUserPath(v.raw, filepath.Dir(v.file), d.userHome, d.userHomeErr)
	if err != nil {
		return "", fmt.Errorf("%s in %s: %w", key, v.file, err)
	}
	return p, nil
}

// portablePointer: a committed pointer is relative or starts with ~/ (and
// not absolute, a drive or UNC path, or ${VAR} that expands to one).
// ds:def id=portablepointer-cuc25d5h owner=@khanakia stability=stable
func portablePointer(raw string) bool {
	if strings.HasPrefix(raw, "~/") || strings.HasPrefix(raw, `~\`) {
		return true
	}
	if strings.HasPrefix(raw, "~") || strings.HasPrefix(raw, "$") ||
		strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, `\`) {
		return false
	}
	if len(raw) >= 2 && raw[1] == ':' { // C:\ or C:/ on any OS
		return false
	}
	return !filepath.IsAbs(raw)
}

// warnPointerChain warns when the target itself has pointer keys: one hop only.
func (d *Dirs) warnPointerChain(target string) {
	cfgSpec, _ := d.kind(KindConfig)
	localSpec, _ := d.kind(KindConfigLocal)
	for _, rel := range []string{cfgSpec.Path, localSpec.Path} {
		f := filepath.Join(target, filepath.FromSlash(rel))
		m, err := d.cfg.readConfig(f)
		if err != nil {
			continue
		}
		for _, key := range []string{pointerKeyHome, pointerKeyDB} {
			if s, ok := m[key].(string); ok && s != "" {
				d.warn(WarnPointerChain, fmt.Sprintf("%s in %s not followed: pointers go one hop only", key, f))
			}
		}
	}
}
