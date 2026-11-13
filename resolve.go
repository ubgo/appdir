package appdir

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveHomes fills global and project homes (SPEC 4.3 to 4.6).
// ds:def id=dirs-resolvehomes-2zu5rbrz owner=@khanakia stability=stable
func (d *Dirs) resolveHomes() {
	c := d.cfg
	d.global = Home{Scope: ScopeGlobal}

	// 1-2. --home, then <APP>_HOME: one home for every scope.
	if c.homeFlag != "" {
		d.setOverride(c.homeFlag, RuleFlag, "from --home")
		return
	}
	if raw := d.env(envHome); raw != "" {
		p, err := c.expandUserPath(raw, d.workDir, d.userHome, d.userHomeErr)
		switch {
		case err != nil:
			err = fmt.Errorf("%s: %w", c.envName(envHome), err)
			d.global.Rule, d.project.Rule = RuleEnvHome, RuleEnvHome
			d.globalErr, d.projectErr, d.override = err, err, true
			return
		case d.pointsToProd(p):
			d.warn(WarnEnvPointsToProd, fmt.Sprintf("%s points into the %s home %s; ignored for profile %s", c.envName(envHome), c.releaseProfile, d.prodGlobal(), d.profile))
		default:
			d.setOverride(p, RuleEnvHome, "from "+c.envName(envHome))
			return
		}
	}
	d.warnKindEnvLeaks()

	// 3-4. Global home.
	if d.userHomeErr != nil {
		d.global.Rule, d.global.Fallback = RuleDefaults, true
		d.global.Note = "no home folder"
		d.globalErr = newErr(ErrNoHome, "no home folder; set %s", c.envName(envHome))
	} else {
		dir := filepath.Join(d.userHome, d.Name(c.dirName))
		d.global = Home{Scope: ScopeGlobal, Base: dir, Data: dir, Rule: RuleGlobal}
		d.global.Info = d.readHomeInfo(dir)
	}

	// Project home.
	d.resolveProject()
}

// setOverride makes one home for both scopes.
func (d *Dirs) setOverride(raw string, rule Rule, note string) {
	d.override = true
	p := raw
	if rule == RuleFlag {
		var err error
		if p, err = d.cfg.expandUserPath(raw, d.workDir, d.userHome, d.userHomeErr); err != nil {
			err = fmt.Errorf("--home: %w", err)
			d.global.Rule, d.project.Rule = rule, rule
			d.globalErr, d.projectErr = err, err
			return
		}
	}
	h := Home{Scope: ScopeGlobal, Base: p, Data: p, Rule: rule, Note: note, Info: d.readHomeInfo(p)}
	d.global = h
	h.Scope = ScopeProject
	d.project = h
	d.warnKindEnvLeaks()
}

// resolveProject runs the lookup and the pointer keys.
func (d *Dirs) resolveProject() {
	start := realPath(d.workDir)
	marker, rule := d.lookupProject(start)
	if marker == "" {
		d.projectErr = d.noProjectErr()
		return
	}
	d.project = Home{Scope: ScopeProject, Marker: marker, Base: marker, Rule: rule}
	if rule == RuleWorktree {
		d.project.Note = "main checkout of a linked git worktree"
	}
	ignoredDB, err := d.applyPointers()
	if err != nil {
		d.projectErr = err
		return
	}
	d.project.Data = d.project.Base
	if d.profile != d.cfg.releaseProfile {
		d.project.Data = filepath.Join(d.project.Base, profilesDir, string(d.profile))
	}
	d.project.Info = d.readHomeInfo(d.project.Base)
	if ignoredDB != "" {
		d.warnPointerProfile(ignoredDB)
	}
}

// lookupProject finds the project marker (SPEC 4.4). Linked worktrees are
// checked first so they use the main checkout's home.
// ds:def id=dirs-lookupproject-cbn3hgm5 owner=@khanakia stability=stable
func (d *Dirs) lookupProject(start string) (string, Rule) {
	if root := d.repoRoot(start); root != "" {
		if main := linkedWorktreeMain(root); main != "" {
			rel, err := filepath.Rel(root, start)
			if err == nil {
				if m := d.findMarker(filepath.Join(main, rel)); m != "" {
					return m, RuleWorktree
				}
				// Warn only when the worktree's own home is what gets used:
				// with no home in either place the warning names nothing real.
				m := d.findMarker(start)
				if m != "" {
					d.warn(WarnWorktreeMainMissing, fmt.Sprintf("linked worktree of %s, but the main checkout has no %s; using the worktree", main, d.cfg.dirName))
				}
				return m, RuleProject
			}
		}
	}
	return d.findMarker(start), RuleProject
}

// findMarker applies the lookup style from start.
// ds:def id=dirs-findmarker-s5yjcjcd owner=@khanakia stability=stable
func (d *Dirs) findMarker(start string) string {
	found := ""
	d.walk(start, func(dir string) bool {
		cand := filepath.Join(dir, d.cfg.dirName)
		if isDir(cand) && !d.isGlobalHome(cand) {
			found = cand
			return true
		}
		if d.cfg.lookup == HereOnly {
			return true
		}
		// A repo root is a wall. Lstat, not Stat: any entry named .git counts,
		// even a dangling symlink, like repoRoot and inGitWorkTree.
		_, err := os.Lstat(filepath.Join(dir, gitEntry))
		return err == nil
	})
	return found
}

// repoRoot returns the first folder at or above start holding an entry named
// .git, within the stop rules; "" when none.
func (d *Dirs) repoRoot(start string) string {
	root := ""
	d.walk(start, func(dir string) bool {
		if _, err := os.Lstat(filepath.Join(dir, gitEntry)); err == nil {
			root = dir
			return true
		}
		return false
	})
	return root
}

// walk calls visit from start upward until visit returns true or a stop rule
// hits: filesystem root, $HOME, a ceiling dir, or a change of disk. The start
// folder itself is always visited.
// ds:def id=dirs-walk-5dgdy46j owner=@khanakia stability=stable
func (d *Dirs) walk(start string, visit func(dir string) bool) {
	ceilings := d.ceilings()
	dir := start
	dev, devOK := deviceID(dir)
	for {
		if visit(dir) {
			return
		}
		// $HOME is a wall in both directions: the parent check below stops
		// before entering it, and this one stops after visiting it when the
		// walk started there, so a folder above $HOME is never a project.
		if d.userHomeErr == nil && sameFile(dir, d.userHome) {
			return
		}
		// A ceiling is a wall the same way (git's GIT_CEILING_DIRECTORIES):
		// a walk that starts inside one checks it, then never climbs above.
		for _, c := range ceilings {
			if sameFile(dir, c) {
				return
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		if d.userHomeErr == nil && sameFile(parent, d.userHome) {
			return
		}
		for _, c := range ceilings {
			if sameFile(parent, c) {
				return
			}
		}
		if pdev, ok := deviceID(parent); ok && devOK && pdev != dev {
			return
		}
		dir = parent
	}
}

// ceilings parses <APP>_CEILING_DIRS.
// ds:def id=dirs-ceilings-7ypck9dn owner=@khanakia stability=stable
func (d *Dirs) ceilings() []string {
	var out []string
	for _, part := range filepath.SplitList(d.env(envCeilingDirs)) {
		if p, _, _ := d.ceilingEntry(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ceilingEntry classifies one <APP>_CEILING_DIRS entry. It is the single
// rule both ceilings() (which keeps the paths) and warnCeilings() (which
// records why an entry was ignored) use, so the two never disagree.
//
//   - "" (empty list slot): ignored, no warning.
//   - relative (no /, ~ or $ in front): ignored with ceiling-relative, as git.
//   - expands to an absolute path: kept.
//   - anything else ($VAR without braces stays literal, an unset ${VAR},
//     ~user, an unknown home for ~): ignored with ceiling-dropped and the
//     reason, so a ceiling never silently has no effect (finding D5).
func (d *Dirs) ceilingEntry(part string) (path string, code WarningCode, reason string) {
	if part == "" {
		return "", "", ""
	}
	if isRelativeEntry(part) {
		return "", WarnCeilingRelative, "is relative"
	}
	p, err := d.cfg.expandUserPath(part, "", d.userHome, d.userHomeErr)
	switch {
	case err != nil:
		return "", WarnCeilingDropped, "does not expand (" + detailWithoutPath(err, part) + ")"
	case !filepath.IsAbs(p) && !strings.HasPrefix(part, varOpen):
		return "", WarnCeilingDropped, "is not an absolute path (only ${VAR} expands, not $VAR)"
	case !filepath.IsAbs(p):
		return "", WarnCeilingDropped, fmt.Sprintf("expands to %q, not an absolute path", p)
	}
	return p, "", ""
}

// isRelativeEntry reports a ceiling entry that is neither absolute nor
// starts with ~ or ${VAR}.
func isRelativeEntry(part string) bool {
	return !filepath.IsAbs(part) && !strings.HasPrefix(part, "~") && !strings.HasPrefix(part, "$")
}

// warnCeilings records one warning per ignored ceiling entry:
// ceiling-relative (git's rule) or ceiling-dropped (see ceilingEntry).
func (d *Dirs) warnCeilings() {
	for _, part := range filepath.SplitList(d.env(envCeilingDirs)) {
		if _, code, reason := d.ceilingEntry(part); code != "" {
			d.warn(code, fmt.Sprintf("%s entry %q %s; ignored", d.cfg.envName(envCeilingDirs), part, reason))
		}
	}
}

// isGlobalHome rejects the global home of the release profile and of the
// current profile as a project home (by identity, never by string).
func (d *Dirs) isGlobalHome(cand string) bool {
	if d.userHomeErr != nil {
		return false
	}
	// samePlannedPath, not sameFile: InitProject asks before ~/.<app> exists,
	// and two missing paths are never "the same file" (F14).
	return samePlannedPath(cand, d.prodGlobal()) || samePlannedPath(cand, filepath.Join(d.userHome, d.Name(d.cfg.dirName)))
}

// prodGlobal is the release profile's global home, computed with an empty env.
func (d *Dirs) prodGlobal() string {
	if d.userHomeErr != nil {
		return ""
	}
	return filepath.Join(d.userHome, d.cfg.dirName)
}

// pointsToProd reports whether a non-release profile was handed a path inside
// the release profile's global home (SPEC 8.5).
// ds:def id=dirs-pointstoprod-9gdwvqpz owner=@khanakia stability=stable
func (d *Dirs) pointsToProd(p string) bool {
	if d.profile == d.cfg.releaseProfile {
		return false
	}
	g := d.prodGlobal()
	return g != "" && within(p, g)
}

// warnKindEnvLeaks warns once per <APP>_<KIND>_DIR that points into prod.
func (d *Dirs) warnKindEnvLeaks() {
	d.warnCeilings()
	for _, k := range d.kinds {
		if k.Anchored {
			continue
		}
		name := d.KindEnv(k.Name)
		raw := d.env(envPrefixFor(string(k.Name)) + envKindSuffix)
		if raw == "" {
			continue
		}
		if p, err := d.cfg.expandUserPath(raw, d.workDir, d.userHome, d.userHomeErr); err == nil && d.pointsToProd(p) {
			d.warn(WarnEnvPointsToProd, fmt.Sprintf("%s points into the %s home %s; ignored for profile %s", name, d.cfg.releaseProfile, d.prodGlobal(), d.profile))
		}
	}
}

// linkedWorktreeMain returns the main checkout of a linked worktree whose
// root is root, or "". It reads files only; git is never run (D2).
// ds:def id=linkedworktreemain-xwtqp3tk owner=@khanakia stability=stable
func linkedWorktreeMain(root string) string {
	gitPath := filepath.Join(root, gitEntry)
	fi, err := os.Lstat(gitPath)
	if err != nil || fi.IsDir() {
		return ""
	}
	gitdir := readGitdirLine(gitPath)
	if gitdir == "" {
		return ""
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(root, gitdir)
	}
	b, err := os.ReadFile(filepath.Join(gitdir, "commondir"))
	if err != nil {
		return "" // a submodule: its gitdir has no commondir, so it is its own project
	}
	common := strings.TrimSpace(string(b))
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitdir, common)
	}
	common = filepath.Clean(common)
	if filepath.Base(common) != gitEntry {
		return "" // bare main repo: no checkout to share
	}
	return filepath.Dir(common)
}

// readGitdirLine returns the value of the "gitdir:" line of a .git file.
func readGitdirLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }() // read-only file
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "gitdir:"); ok {
			return filepath.FromSlash(strings.TrimSpace(v))
		}
	}
	return ""
}

// inGitWorkTree reports whether dir is inside a git work tree: an entry named
// .git in it or any parent.
func inGitWorkTree(dir string) bool {
	for d := dir; ; {
		if _, err := os.Lstat(filepath.Join(d, gitEntry)); err == nil {
			return true
		}
		p := filepath.Dir(d)
		if p == d {
			return false
		}
		d = p
	}
}

// readHomeInfo reads home.jsonc in dir; nil when absent or unreadable.
func (d *Dirs) readHomeInfo(dir string) *HomeInfo {
	spec, ok := d.kind(KindHome)
	if !ok || dir == "" {
		return nil
	}
	m, err := d.cfg.readConfig(filepath.Join(dir, filepath.FromSlash(spec.Path)))
	if err != nil {
		return nil
	}
	info := &HomeInfo{}
	info.App, _ = m[homeKeyApp].(string)
	if n, ok := m[homeKeyLayout].(float64); ok {
		info.Layout = int(n)
	}
	if s, ok := m[homeKeyScope].(string); ok {
		info.Scope = Scope(s)
	}
	return info
}

// warn records a warning.
func (d *Dirs) warn(code WarningCode, msg string) {
	d.warnings = append(d.warnings, Warning{Code: code, Message: msg})
}
