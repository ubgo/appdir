// Command overrides shows every way a user moves folders: --home (WithHome)
// and <APP>_HOME make one home for everything, <APP>_<KIND>_DIR moves one
// kind, and user paths expand ~ and ${VAR}. It also shows the names a tool
// prints (KindEnv, EnvName, EnvPrefix), how WithEnvPrefix and WithDirName
// change them, and what happens with an unset or empty variable.
//
// Everything happens inside a temp root: the user's home is root/home, the
// tool runs in a project under it, and the environment is a map passed with
// WithEnv, never the process environment.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example.
const (
	appName    = "lore"
	homeDir    = "home"
	marker     = ".lore" // the project home folder
	projectDir = "home/projects/shop"
	envHome    = "LORE_HOME"
	envCache   = "LORE_CACHE_DIR"
	envDB      = "LORE_DB_DIR"
	varFast    = "FAST" // a user variable referenced as ${FAST}
	fastDir    = "fast" // what FAST points to, relative to root
	prefix     = "NOTES"
	dirName    = ".lore-data"
	badDirName = ".." // refused: not one plain folder name
	tryHome    = "~/try"
	prodHome   = "~/.lore/x" // inside the prod global home
)

// env is a fake environment for one New call.
type env map[string]string

func main() {
	root, err := os.MkdirTemp("", "appdir-overrides-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "overrides:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "overrides:", err)
		os.Exit(1)
	}
}

// run resolves the same project under different flags and variables.
func run(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	s := setup{root: root, home: filepath.Join(root, homeDir), project: filepath.Join(root, filepath.FromSlash(projectDir))}
	if err := os.MkdirAll(filepath.Join(s.project, marker), 0o700); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	fast := filepath.Join(root, fastDir)
	steps := []struct {
		title string
		do    func() error
	}{
		{"no override", func() error { return s.show(w, nil, nil) }},
		{"--home ~/try", func() error { return s.show(w, nil, []appdir.Option{appdir.WithHome(tryHome)}) }},
		{"--home scratch (relative)", func() error { return s.show(w, nil, []appdir.Option{appdir.WithHome("scratch")}) }},
		{"LORE_HOME=${FAST}/lore", func() error { return s.envHome(w, fast) }},
		{"LORE_CACHE_DIR and LORE_DB_DIR", func() error {
			return s.show(w, env{envCache: "~/fast-cache", envDB: "${FAST}/db", varFast: fast}, nil)
		}},
		{"env names", func() error { return s.names(w) }},
		{"bad values", func() error { return s.bad(w) }},
		{"--home with an unset ${VAR}", func() error { return s.brokenHome(w) }},
	}
	for _, st := range steps {
		fmt.Fprintf(w, "== %s\n", st.title)
		if err := st.do(); err != nil {
			return fmt.Errorf("%s: %w", st.title, err)
		}
	}
	return nil
}

// setup holds the folders every step uses.
type setup struct{ root, home, project string }

// snippet:open
// open resolves the homes with the given fake environment and extra options
// (the flags a tool passes, such as WithHome for --home).
func (s setup) open(e env, extra ...appdir.Option) (*appdir.Dirs, error) {
	opts := append([]appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return s.home, nil }),
		appdir.WithWorkDir(s.project),
		appdir.WithEnv(func(k string) (string, bool) { v, ok := e[k]; return v, ok }),
		appdir.WithRelease(true), // prod profile: plain folder names
	}, extra...)
	d, err := appdir.New(appName, opts...)
	if err != nil {
		return nil, fmt.Errorf("appdir: %w", err)
	}
	return d, nil
}

// snippet:end

// snippet:show
// show prints where the config file, the db and the cache land, and the
// rule that picked each one.
func (s setup) show(w io.Writer, e env, extra []appdir.Option) error {
	d, err := s.open(e, extra...)
	if err != nil {
		return err
	}
	for _, k := range []appdir.Kind{appdir.KindConfig, appdir.KindDB, appdir.KindCache} {
		// Paths lists a ScopeBoth kind (config) once per home, global first;
		// the last line is the one Path(k) returns.
		var last appdir.Entry
		for _, en := range d.Paths() {
			if en.Kind == k {
				last = en
			}
		}
		fmt.Fprintf(w, "%-7s %-9s %s\n", k, last.Rule, s.label(last.Path))
	}
	return nil
}

// snippet:end

// snippet:names
// names prints the variable names a tool shows in help and doctor output,
// for the defaults and with WithEnvPrefix / WithDirName, and a dir name
// that is refused.
func (s setup) names(w io.Writer) error {
	d, err := s.open(nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "EnvPrefix: %s  EnvName(HOME): %s\n", d.EnvPrefix(), d.EnvName("HOME"))
	fmt.Fprintf(w, "KindEnv(cache): %s  KindEnv(config): %q (anchored)\n", d.KindEnv(appdir.KindCache), d.KindEnv(appdir.KindConfig))
	d, err = s.open(nil, appdir.WithEnvPrefix(prefix), appdir.WithDirName(dirName))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "with prefix %s: KindEnv(cache): %s\n", prefix, d.KindEnv(appdir.KindCache))
	fmt.Fprintf(w, "with dir name %s: global home %s\n", dirName, s.label(d.Global().Base))
	// The dir name must be one plain folder name: "..", "." or a path with a
	// separator would put the home outside the user's home.
	if _, err := s.open(nil, appdir.WithDirName(badDirName)); errors.Is(err, appdir.ErrInvalidOption) {
		// open adds its own "appdir:" prefix; print the library's message.
		fmt.Fprintf(w, "with dir name %s: error: %v\n", badDirName, errors.Unwrap(err))
	} else {
		return fmt.Errorf("dir name %q: want ErrInvalidOption, got %v", badDirName, err)
	}
	return nil
}

// snippet:end

// snippet:envhome
// envHome sets LORE_HOME: one home for every kind, with rule env-home. When
// --home is given too, the flag wins. A dev build ignores a LORE_HOME that
// points into the prod global home (~/.lore) and says so with a warning, so
// a variable left over from a prod shell never lets dev touch prod data.
func (s setup) envHome(w io.Writer, fast string) error {
	e := env{envHome: "${FAST}/lore", varFast: fast}
	if err := s.show(w, e, nil); err != nil {
		return err
	}
	d, err := s.open(e, appdir.WithHome(tryHome))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "with --home %s: global %s (rule %s)\n", tryHome, s.label(d.Global().Base), d.Global().Rule)
	// WithRelease(false) after open's WithRelease(true): the last option wins,
	// so this Dirs is a dev build.
	d, err = s.open(env{envHome: prodHome}, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dev build, LORE_HOME=%s: global %s (rule %s)\n", prodHome, s.label(d.Global().Base), d.Global().Rule)
	for _, wn := range d.Warnings() {
		// The message names the absolute prod home; trim the temp root so the
		// output is the same on every run.
		msg := strings.ReplaceAll(wn.Message, s.root+string(filepath.Separator), "")
		fmt.Fprintf(w, "  warning %s: %s\n", wn.Code, filepath.ToSlash(msg))
	}
	return nil
}

// snippet:end

// snippet:bad
// bad shows values that never turn into a path: an unset ${VAR}, a set but
// empty one, ~user. $VAR without braces is not expanded: it stays a literal,
// relative folder name.
func (s setup) bad(w io.Writer) error {
	cases := []struct {
		name string
		e    env
	}{
		{"${FAST} unset", env{envCache: "${FAST}/cache"}},
		{"${FAST} empty", env{envCache: "${FAST}/cache", varFast: ""}},
		{"~user", env{envCache: "~bob/cache"}},
		{"$FAST literal", env{envCache: "$FAST/cache", varFast: "/x"}},
		{"LORE_HOME empty", env{envHome: ""}},
	}
	for _, c := range cases {
		d, err := s.open(c.e)
		if err != nil {
			return err
		}
		p, err := d.Path(appdir.KindCache)
		switch {
		case errors.Is(err, appdir.ErrEnvVar), errors.Is(err, appdir.ErrInvalidOption):
			fmt.Fprintf(w, "%-15s error: %v\n", c.name+":", err)
		case err != nil:
			return err
		default:
			fmt.Fprintf(w, "%-15s cache: %s\n", c.name+":", s.label(p))
		}
	}
	return nil
}

// snippet:end

// snippet:broken
// brokenHome shows a --home that names an unset variable: every kind fails
// with the same error, the disposable cache included. The temp fallback is
// only for a machine with no home folder and no override; a broken override
// never quietly writes somewhere else.
func (s setup) brokenHome(w io.Writer) error {
	d, err := s.open(nil, appdir.WithHome("${NOPE}/lore"))
	if err != nil {
		return err
	}
	for _, k := range []appdir.Kind{appdir.KindDB, appdir.KindCache} {
		p, err := d.Path(k)
		if err != nil {
			fmt.Fprintf(w, "%-6s error: %v\n", k, err)
			continue
		}
		fmt.Fprintf(w, "%-6s %s\n", k, s.label(p))
	}
	return nil
}

// snippet:end

// label prints a path relative to root, so the output is the same on every
// run. Every path in this example is under root.
func (s setup) label(p string) string {
	if p == "" {
		return "(none)"
	}
	if rel, err := filepath.Rel(s.root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}
