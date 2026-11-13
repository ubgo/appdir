package appdir

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Release reasons reported by ReleaseReason.
const (
	reasonFlag      = "flag"
	reasonFlagDev   = "dev: flag"
	reasonGoInstall = "go-install"
	reasonNoInfo    = "dev: no build info"
	reasonDevel     = "dev: (devel)"
	reasonPseudo    = "dev: pseudo-version"
	reasonDirty     = "dev: +dirty"
	reasonVCS       = "dev: vcs.revision"
	reasonNotSemver = "dev: version without v prefix"
	settingVCSRev   = "vcs.revision"
	develVersion    = "(devel)"
)

// detectRelease decides release vs dev with the stdlib only (SPEC 8.2): the
// explicit flag first, else a clean tagged module version with no
// vcs.revision, which only `go install module@vX` produces.
// ds:def id=detectrelease-7fwv9uc3 owner=@khanakia stability=stable
func detectRelease(c *config) (bool, string) {
	if c.release != nil {
		if *c.release {
			return true, reasonFlag
		}
		return false, reasonFlagDev
	}
	bi, ok := c.buildInfo()
	if !ok || bi == nil {
		return false, reasonNoInfo
	}
	v := bi.Main.Version
	switch {
	case v == "" || v == develVersion:
		return false, reasonDevel
	case !strings.HasPrefix(v, "v"):
		return false, reasonNotSemver
	case strings.Contains(v, "+dirty"):
		return false, reasonDirty
	case isPseudoVersion(v):
		return false, reasonPseudo
	}
	for _, s := range bi.Settings {
		if s.Key == settingVCSRev {
			return false, reasonVCS
		}
	}
	return true, reasonGoInstall
}

// isPseudoVersion matches Go module pseudo-versions (same pattern as
// golang.org/x/mod/module.IsPseudoVersion), compiled per call: it runs once.
func isPseudoVersion(v string) bool {
	re := regexp.MustCompile(`^v[0-9]+\.(0\.0-|\d+\.\d+-([^+]*\.)?0\.)\d{14}-[A-Za-z0-9]+(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)
	return re.MatchString(v)
}

// knownProfile reports whether p is in the list.
func (c *config) knownProfile(p Profile) bool {
	for _, q := range c.profiles {
		if q == p {
			return true
		}
	}
	return false
}

// pickProfile applies SPEC 8.1: flag, env (two limits), build.
// ds:def id=dirs-pickprofile-g7fmfemq owner=@khanakia stability=stable
func (d *Dirs) pickProfile() error {
	c := d.cfg
	check := func(p Profile, from string) error {
		if !c.knownProfile(p) {
			return newErr(ErrUnknownProfile, "appdir: %s: unknown profile %q (known: %s)", from, p, joinProfiles(c.profiles))
		}
		return nil
	}
	byBuild := c.devProfile
	if d.release {
		byBuild = c.releaseProfile
	}
	switch {
	case c.profileFlag != "":
		p := Profile(c.profileFlag)
		if err := check(p, "--profile"); err != nil {
			return err
		}
		d.profile = p
	case d.env(envProfile) != "":
		name := c.envName(envProfile)
		p := Profile(d.env(envProfile))
		if err := check(p, name); err != nil {
			return err
		}
		switch {
		case !d.release && p == c.releaseProfile && !d.allowProd:
			d.warn(WarnProfileEnvIgnored, fmt.Sprintf("%s=%s ignored by a dev build; set %s=1 too, or pass --profile", name, p, c.envName(envAllowProd)))
			d.profile = byBuild
		case d.release && p != c.releaseProfile:
			d.warn(WarnProfileEnvIgnored, fmt.Sprintf("%s=%s ignored by a release build; pass --profile %s", name, p, p))
			d.profile = byBuild
		default:
			d.profile = p
		}
	default:
		d.profile = byBuild
	}

	d.daemonProfile = d.profile
	if c.daemonFlag != "" {
		d.daemonProfile = Profile(c.daemonFlag)
		if err := check(d.daemonProfile, "--daemon-profile"); err != nil {
			return err
		}
	} else if v := d.env(envDaemonProfile); v != "" {
		d.daemonProfile = Profile(v)
		if err := check(d.daemonProfile, c.envName(envDaemonProfile)); err != nil {
			return err
		}
	}
	return nil
}

func joinProfiles(ps []Profile) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = string(p)
	}
	return strings.Join(s, ", ")
}

// Profile returns the profile in effect.
func (d *Dirs) Profile() Profile { return d.profile }

// IsRelease reports whether this binary is a release build.
func (d *Dirs) IsRelease() bool { return d.release }

// ReleaseReason says which rule decided release or dev.
func (d *Dirs) ReleaseReason() string { return d.releaseReason }

// Name returns base for the release profile and base-<profile> otherwise.
// Every label, unit, marker, pipe and item name goes through it.
func (d *Dirs) Name(base string) string { return d.NameWith(base, "-") }

// NameWith is Name with a custom separator: NameWith("com.org.app", ".")
// gives "com.org.app.dev".
// ds:def id=dirs-namewith-uyg39uh6 owner=@khanakia stability=stable
func (d *Dirs) NameWith(base, sep string) string {
	if d.profile == d.cfg.releaseProfile {
		return base
	}
	return base + sep + string(d.profile)
}

// Banner returns the one stderr line for a non-release profile
// ("[lore] profile: dev → ~/.lore-dev"), or "" for the release profile.
// ds:def id=dirs-banner-cwbb5293 owner=@khanakia stability=stable
func (d *Dirs) Banner() string {
	if d.profile == d.cfg.releaseProfile {
		return ""
	}
	where := d.global.Base
	if where == "" {
		where = "(no home folder)"
	} else if d.userHomeErr == nil {
		if lexicallyWithin(where, d.userHome) {
			rel, _ := filepath.Rel(d.userHome, where) // cannot fail: lexicallyWithin checked it
			where = filepath.Join("~", rel)
		}
	}
	return fmt.Sprintf("[%s] profile: %s → %s", d.cfg.app, d.profile, where)
}

// ForProfile resolves the same app with the same options as if --profile p
// were given. It reads only; it never opens the other profile's files.
// ds:def id=dirs-forprofile-g2gegg5p owner=@khanakia stability=stable
func (d *Dirs) ForProfile(p Profile) (*Dirs, error) {
	opts := append(append([]Option(nil), d.opts...), WithProfile(string(p)))
	return New(d.cfg.app, opts...)
}

// ChildEnv returns environ with <APP>_PROFILE set to this profile, for every
// child process the tool starts (SPEC 8.5).
// ds:def id=dirs-childenv-myszxsrp owner=@khanakia stability=stable
func (d *Dirs) ChildEnv(environ []string) []string {
	key := d.cfg.envName(envProfile) + "="
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if !strings.HasPrefix(kv, key) {
			out = append(out, kv)
		}
	}
	return append(out, key+string(d.profile))
}

// CleanEnv returns environ without any <prefix>_* variable: the Go twin of a
// dev-env.sh wrapper. Pure function.
// ds:def id=cleanenv-276nsr8w owner=@khanakia stability=stable
func CleanEnv(environ []string, prefix string) []string {
	p := prefix + "_"
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if !strings.HasPrefix(kv, p) {
			out = append(out, kv)
		}
	}
	return out
}

// CheckDB is the prod-db guard (SPEC 8.4). Call it before opening a database
// and before any migration.
// ds:def id=dirs-checkdb-2r8b9xmq owner=@khanakia stability=stable
func (d *Dirs) CheckDB(path string) (DBAccess, error) {
	// A relative path is relative to the working folder New resolved from
	// (WithWorkDir), so the guard checks the same file the tool opens.
	abs := filepath.Clean(path)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(d.workDir, abs)
	}
	if err := d.VerifyFile(abs); err != nil {
		return AccessReadWrite, err
	}
	if d.release {
		return AccessReadWrite, nil
	}
	prod, err := d.prodView()
	if err != nil {
		return AccessReadWrite, err
	}
	for _, s := range []Scope{ScopeGlobal, ScopeProject} {
		cand, err := prod.PathIn(KindDB, s)
		if err != nil || cand == "" || !sameDBPath(abs, cand) {
			continue
		}
		if d.allowProd {
			return AccessReadOnly, nil
		}
		return AccessReadWrite, newErr(ErrProdDB, "dev build refuses the prod database %s; set %s=1 to open it read-only", abs, d.cfg.envName(envAllowProd))
	}
	return AccessReadWrite, nil
}

// prodView resolves the release profile's paths for this machine with no
// --home and no <APP>_* variable: what an installed binary would open here.
// Only the app's own variables are stripped (the overrides the standard's
// "empty env" means: <APP>_HOME, <APP>_<KIND>_DIR, <APP>_PROFILE...). Every
// other variable stays, because a prod db: pointer in config.local.jsonc may
// be written with ${VAR} (and XDG_* shapes the global home); an empty env
// made such a pointer fail with ErrEnvVar and the prod db went unrecognised.
func (d *Dirs) prodView() (*Dirs, error) {
	lookup, appVars := d.cfg.lookupEnv, d.cfg.prefix+"_"
	opts := append(append([]Option(nil), d.opts...),
		WithEnv(func(name string) (string, bool) {
			if strings.HasPrefix(name, appVars) {
				return "", false
			}
			return lookup(name)
		}),
		WithHome(""),
		WithProfile(string(d.cfg.releaseProfile)),
		WithTestGuard(false))
	return New(d.cfg.app, opts...)
}

// sameDBPath compares by identity; for a file that does not exist yet it
// compares the real parent folder and the file name. A parent folder that
// does not exist yet (the prod home before the installed build ever ran) has
// no identity, so it is compared by real path (symlinks resolved as far as
// the path exists): otherwise a dev build would create and migrate the
// prod db in a fresh ~/.<app>.
//
// It is deliberately not samePlannedPath (resolve.go's global-home check):
// that one says "different" as soon as one path exists and the other does
// not, and needs both parents to exist. The prod-db guard must instead lean
// towards "same" for a db whose folder is still to be created, because a
// false "different" lets a dev build migrate the prod database.
func sameDBPath(a, b string) bool {
	if exists(a) && exists(b) {
		return sameFile(a, b)
	}
	pa, pb := filepath.Dir(a), filepath.Dir(b)
	if !sameFile(pa, pb) && (exists(pa) && exists(pb) || !sameName(realPath(pa), realPath(pb))) {
		return false
	}
	return sameName(filepath.Base(a), filepath.Base(b))
}
