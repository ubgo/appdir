// Command profiles shows how appdir picks a profile (flag, env, build), how a
// build is recognised as a release, how profiles get their own folders and
// names, and how a tool passes its profile to child processes.
//
// Everything happens inside a temp root: the user's home is root/home (passed
// with WithHomeDir), the tool runs in a project under it (WithWorkDir), and
// the environment is a map (WithEnv), so the program never reads the real
// home or the process environment.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example. The app name decides the home folder (.demo),
// the env prefix (DEMO_) and the db file name (demo.db).
const (
	appName    = "demo"
	homeDir    = "home"
	projectDir = "home/proj"

	envProfile   = "DEMO_PROFILE"
	envAllowProd = "DEMO_ALLOW_PROD"
	envHome      = "DEMO_HOME"
	envDBDir     = "DEMO_DB_DIR"
	envPrefix    = "DEMO"
	allowProdOn  = "1"

	profileStaging appdir.Profile = "staging" // an author-added profile
	profileStable  appdir.Profile = "stable"  // an author-renamed release profile
	profileUnknown                = "qa"      // not in any list; untyped because flag and env values arrive as plain strings

	serviceLabel = "com.example.demo" // a launchd-style label
	workerBinary = "demo-worker"      // a child process the tool starts
)

// fakeEnv is the environment WithEnv reads instead of the process env.
type fakeEnv map[string]string

// lookup has the signature WithEnv wants; a nil map is an empty environment.
func (e fakeEnv) lookup(k string) (string, bool) {
	v, ok := e[k]
	return v, ok
}

// sandbox holds the folders every case resolves against.
type sandbox struct {
	root, home, project string
}

func main() {
	root, err := os.MkdirTemp("", "appdir-profiles-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "profiles:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "profiles:", err)
		os.Exit(1)
	}
}

// run does the whole walk-through inside root and prints paths relative to it.
func run(w io.Writer, root string) error {
	// Resolve the root once so printed paths match what appdir reports (on
	// macOS the temp folder is a symlink).
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	s := sandbox{
		root:    root,
		home:    filepath.Join(root, filepath.FromSlash(homeDir)),
		project: filepath.Join(root, filepath.FromSlash(projectDir)),
	}
	if err := os.MkdirAll(s.project, 0o700); err != nil {
		return fmt.Errorf("create project folder: %w", err)
	}
	if err := s.initProject(); err != nil {
		return err
	}
	steps := []struct {
		title string
		fn    func(io.Writer, sandbox) error
	}{
		{"profile detection", detectProfiles},
		{"release detection", releaseDetection},
		{"version line", versionLines},
		{"custom profiles", customProfiles},
		{"profile folders", profileFolders},
		{"names and banner", names},
		{"another profile's names", otherNames},
		{"env leak filter", envLeak},
		{"child processes", childProcesses},
	}
	for _, st := range steps {
		fmt.Fprintln(w, "==", st.title)
		if err := st.fn(w, s); err != nil {
			return fmt.Errorf("%s: %w", st.title, err)
		}
	}
	return nil
}

// snippet:open
// open resolves the app with the sandbox seams plus the case's own options.
// The env map replaces the process environment, so each case states exactly
// which DEMO_* variables it sets.
func (s sandbox) open(env fakeEnv, opts ...appdir.Option) (*appdir.Dirs, error) {
	base := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return s.home, nil }),
		appdir.WithWorkDir(s.project),
		appdir.WithEnv(env.lookup),
	}
	return appdir.New(appName, append(base, opts...)...)
}

// snippet:end

// initProject makes .demo/ in the project, like `demo init` would.
func (s sandbox) initProject() error {
	d, err := s.open(nil, appdir.WithRelease(true))
	if err != nil {
		return err
	}
	if _, err := d.InitProject(s.project); err != nil {
		return fmt.Errorf("init project: %w", err)
	}
	return nil
}

// snippet:detect
// detectProfiles runs the three detection rules in order: --profile
// (WithProfile), then DEMO_PROFILE with its two limits, then the build.
// WithRelease stands in for the tool's -ldflags -X release variable.
func detectProfiles(w io.Writer, s sandbox) error {
	dev, rel := appdir.WithRelease(false), appdir.WithRelease(true)
	cases := []struct {
		label string
		env   fakeEnv
		opts  []appdir.Option
	}{
		{"dev build", nil, []appdir.Option{dev}},
		{"release build", nil, []appdir.Option{rel}},
		{"dev build, --profile prod", nil, []appdir.Option{dev, appdir.WithProfile(string(appdir.ProfileProd))}},
		{"dev build, DEMO_PROFILE=prod", fakeEnv{envProfile: string(appdir.ProfileProd)}, []appdir.Option{dev}},
		{"dev build, DEMO_PROFILE=prod, DEMO_ALLOW_PROD=1", fakeEnv{envProfile: string(appdir.ProfileProd), envAllowProd: allowProdOn}, []appdir.Option{dev}},
		{"release build, DEMO_PROFILE=dev", fakeEnv{envProfile: string(appdir.ProfileDev)}, []appdir.Option{rel}},
		{"release build, --profile dev", nil, []appdir.Option{rel, appdir.WithProfile(string(appdir.ProfileDev))}},
		{"dev build, --profile qa", nil, []appdir.Option{dev, appdir.WithProfile(profileUnknown)}},
		{"release build, DEMO_PROFILE=qa", fakeEnv{envProfile: profileUnknown}, []appdir.Option{rel}},
	}
	for _, c := range cases {
		d, err := s.open(c.env, c.opts...)
		switch {
		case errors.Is(err, appdir.ErrUnknownProfile):
			// Never a fallback: a typo must not silently land on prod data.
			fmt.Fprintf(w, "%-48s -> error: %v\n", c.label, err)
			continue
		case err != nil:
			return err
		}
		fmt.Fprintf(w, "%-48s -> profile %s\n", c.label, d.Profile())
		for _, warn := range d.Warnings() {
			fmt.Fprintf(w, "%-48s    warning %s: %s\n", "", warn.Code, warn.Message)
		}
	}
	return nil
}

// snippet:end

// snippet:release
// releaseDetection feeds fake build info through WithBuildInfo, the way Go
// would report each kind of build, and prints the decision and its reason.
// Only a clean tagged version with no vcs.revision is a release: that is what
// `go install module@vX` produces. WithRelease, when given, wins.
func releaseDetection(w io.Writer, s sandbox) error {
	const tag = "v1.4.2"
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "4f1c2a9e"}
	info := func(version string, settings ...debug.BuildSetting) appdir.Option {
		return appdir.WithBuildInfo(func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: version}, Settings: settings}, true
		})
	}
	cases := []struct {
		label string
		opts  []appdir.Option
	}{
		{"go install module@v1.4.2", []appdir.Option{info(tag)}},
		{"go build in a checkout", []appdir.Option{info("(devel)", rev)}},
		{"go install ./cmd/demo on the tag", []appdir.Option{info(tag, rev)}},
		{"go install module@<commit>", []appdir.Option{info("v1.4.3-0.20261110093000-4f1c2a9e0b7d")}},
		{"modified checkout", []appdir.Option{info(tag + "+dirty")}},
		{"version without v", []appdir.Option{info("1.4.2")}},
		{"no build info", []appdir.Option{appdir.WithBuildInfo(func() (*debug.BuildInfo, bool) { return nil, false })}},
		{"-ldflags release=true", []appdir.Option{info("(devel)", rev), appdir.WithRelease(true)}},
		{"-ldflags release=false", []appdir.Option{info(tag), appdir.WithRelease(false)}},
	}
	for _, c := range cases {
		d, err := s.open(nil, c.opts...)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-34s release=%-5v reason=%-32q profile=%s\n", c.label, d.IsRelease(), d.ReleaseReason(), d.Profile())
	}
	return nil
}

// snippet:end

// snippet:report
// versionLine is what a `demo version` or `demo doctor` command prints: the
// profile, the release decision and the first rule that made it. When a user
// asks why the installed tool uses ~/.demo-dev, the reason answers it.
func versionLine(d *appdir.Dirs) string {
	return fmt.Sprintf("%s %s (release=%t, reason %q)", d.App(), d.Profile(), d.IsRelease(), d.ReleaseReason())
}

// versionLines prints the line for a user's `go install module@v1.4.2` and
// for the same tag installed from a checkout, which records vcs.revision.
func versionLines(w io.Writer, s sandbox) error {
	builds := []struct {
		label string
		info  debug.BuildInfo
	}{
		{"go install module@v1.4.2", debug.BuildInfo{Main: debug.Module{Version: "v1.4.2"}}},
		{"go install ./cmd/demo", debug.BuildInfo{
			Main:     debug.Module{Version: "v1.4.2"},
			Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "4f1c2a9e"}},
		}},
	}
	for _, b := range builds {
		d, err := s.open(nil, appdir.WithBuildInfo(func() (*debug.BuildInfo, bool) { return &b.info, true }))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-26s %s\n", b.label, versionLine(d))
	}
	return nil
}

// snippet:end

// snippet:custom
// customProfiles adds a staging profile next to prod and dev, then renames
// the release profile. The list is closed: anything outside it is refused.
func customProfiles(w io.Writer, s sandbox) error {
	list := appdir.WithProfiles(appdir.ProfileProd, appdir.ProfileDev, profileStaging)
	d, err := s.open(nil, list, appdir.WithRelease(false), appdir.WithProfile(string(profileStaging)))
	if err != nil {
		return err
	}
	db, err := d.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "--profile staging: profile %s, global %s, db %s\n", d.Profile(), s.show(d.Global().Base), s.show(db))

	// A dev build of this tool runs as staging instead of dev.
	d, err = s.open(nil, list, appdir.WithRelease(false), appdir.WithDevProfile(profileStaging))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "WithDevProfile(staging), dev build: profile %s\n", d.Profile())

	// A tool whose release profile is "stable": it keeps the plain names.
	d, err = s.open(nil, appdir.WithProfiles(profileStable, appdir.ProfileDev),
		appdir.WithReleaseProfile(profileStable), appdir.WithRelease(true))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "WithReleaseProfile(stable), release build: profile %s, global %s\n", d.Profile(), s.show(d.Global().Base))
	return nil
}

// snippet:end

// snippet:folders
// profileFolders compares where prod and dev keep their files. Data kinds
// move to ~/.demo-dev and .demo/profiles/dev/; anchored kinds (home.jsonc,
// config.jsonc, config.local.jsonc, .gitignore) stay in the shared .demo/.
func profileFolders(w io.Writer, s sandbox) error {
	for _, release := range []bool{true, false} {
		d, err := s.open(nil, appdir.WithRelease(release))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "profile %s: global %s\n", d.Profile(), s.show(d.Global().Base))
		for _, k := range []appdir.Kind{appdir.KindDB, appdir.KindCache, appdir.KindConfig} {
			p, err := d.Path(k)
			if err != nil {
				return fmt.Errorf("path %s: %w", k, err)
			}
			fmt.Fprintf(w, "  %-6s %s\n", k, s.show(p))
		}
	}
	return nil
}

// snippet:end

// snippet:names
// names labels everything a profile owns with Name/NameWith, so a dev build
// never collides with the installed one (service labels, pipe names, item
// names). Banner is the one stderr line a non-release profile prints.
func names(w io.Writer, s sandbox) error {
	for _, release := range []bool{true, false} {
		d, err := s.open(nil, appdir.WithRelease(release))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "profile %s: Name(%q)=%q NameWith(%q, \".\")=%q\n",
			d.Profile(), appName, d.Name(appName), serviceLabel, d.NameWith(serviceLabel, "."))
		fmt.Fprintf(w, "profile %s: Banner()=%q\n", d.Profile(), s.clean(d.Banner()))
	}
	return nil
}

// snippet:end

// snippet:other
// otherNames is an uninstall command in the release build that must also
// stop the dev build's service: ForProfile resolves the dev profile (paths
// only, nothing is opened) and its Name/NameWith give the dev labels.
func otherNames(w io.Writer, s sandbox) error {
	d, err := s.open(nil, appdir.WithRelease(true))
	if err != nil {
		return err
	}
	other, err := d.ForProfile(appdir.ProfileDev)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "profile %s, ForProfile(%s): Name(%q)=%q NameWith(%q, \".\")=%q\n",
		d.Profile(), other.Profile(), appName, other.Name(appName), serviceLabel, other.NameWith(serviceLabel, "."))
	return nil
}

// snippet:end

// snippet:leak
// envLeak runs a dev build in a shell that still carries variables meant for
// the installed tool. Values that point into the prod home are ignored with a
// warning; the dev build keeps its own folders.
func envLeak(w io.Writer, s sandbox) error {
	env := fakeEnv{envHome: "~/.demo", envDBDir: "~/.demo/data"}
	d, err := s.open(env, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	for _, warn := range d.Warnings() {
		fmt.Fprintf(w, "warning %s: %s\n", warn.Code, s.clean(warn.Message))
	}
	db, err := d.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "profile %s: global %s, db %s\n", d.Profile(), s.show(d.Global().Base), s.show(db))
	return nil
}

// snippet:end

// snippet:child
// childProcesses passes the profile on: ChildEnv sets DEMO_PROFILE for a
// child the tool starts, CleanEnv strips every DEMO_* variable (what a dev
// task wrapper does), and ForProfile resolves another profile's paths
// without opening its files. A real tool passes os.Environ().
func childProcesses(w io.Writer, s sandbox) error {
	parent := []string{"PATH=/usr/bin", envProfile + "=" + string(appdir.ProfileProd), envHome + "=~/.demo"}
	d, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	cmd := exec.Command(workerBinary) // built, never started here
	cmd.Env = d.ChildEnv(parent)
	fmt.Fprintf(w, "ChildEnv: %s\n", strings.Join(cmd.Env, " "))
	fmt.Fprintf(w, "CleanEnv: %s\n", strings.Join(appdir.CleanEnv(parent, envPrefix), " "))

	prod, err := d.ForProfile(appdir.ProfileProd)
	if err != nil {
		return err
	}
	db, err := prod.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "ForProfile(prod): profile %s, global %s, db %s\n", prod.Profile(), s.show(prod.Global().Base), s.show(db))
	return nil
}

// snippet:end

// show prints a path relative to root, so the output is the same on every run.
func (s sandbox) show(p string) string {
	if p == "" {
		return "(none)"
	}
	if rel, err := filepath.Rel(s.root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}

// clean drops the temp root from a message, for the same reason.
func (s sandbox) clean(msg string) string {
	return filepath.ToSlash(strings.ReplaceAll(msg, s.root+string(filepath.Separator), ""))
}
