// Command doctor builds the two reports every tool on the standard ships:
// `paths --json` (every kind, its path and the rule that picked it) and
// `doctor` (config files, warnings, layout, gitignore block). It runs
// `paths` once more under --home, where a kind that lives in both homes is
// listed once. It also maps appdir's sentinel errors to the standard's exit
// codes with errors.Is, and shows option sets New refuses.
//
// Everything happens inside a temp root: the user home, the project and the
// environment are all handed to appdir through its seams, so running it
// never touches the real home.
//
//	go run ./examples/doctor
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/appdir"
)

// appName is the example tool's name; its homes are .doctor/ and ~/.doctor.
const appName = "doctor"

// Folder names inside the temp root.
const (
	homeFolder      = "user"        // stands in for the user's home
	repoFolder      = "repo"        // a git repo with a project home
	elsewhere       = "elsewhere"   // a folder with no project home above it
	pointerRepo     = "repo-ptr"    // project whose committed config has an absolute pointer
	tooNewRepo      = "repo-new"    // project whose home.jsonc says layout 2
	teamFolder      = "team"        // the folder --home names in the override run
	gitFolder       = ".git"        // makes a folder a git work tree for the gitignore block
	markerFolder    = "." + appName // the project home inside a repo
	configFile      = "config.jsonc"
	homeInfoFile    = "home.jsonc"
	rootPlaceholder = "$ROOT" // printed instead of the temp root, so output is stable
)

// Kind added by this tool on top of the standard layout.
const kindTemplates appdir.Kind = "templates"

// Envelope fields of the standard's --json output (section 11).
const (
	schemaVersion = 1
	envelopeKind  = "paths"
)

// Exit codes of the standard (section 11): a small fixed set.
const (
	exitOK       = 0
	exitError    = 1
	exitUsage    = 2
	exitNotFound = 3
	exitBusy     = 4
	exitTooNew   = 5
	exitConflict = 6
)

// exampleEnv is the environment the example hands to appdir instead of the
// process env. A release build ignores DOCTOR_PROFILE=dev and a relative
// ceiling entry is ignored: both show up as warnings in the report. A
// function, not a variable: the example keeps no package state.
func exampleEnv() map[string]string {
	return map[string]string{
		"DOCTOR_PROFILE":      string(appdir.ProfileDev),
		"DOCTOR_CEILING_DIRS": "relative/dir",
	}
}

func main() {
	root, err := os.MkdirTemp("", "appdir-doctor-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "doctor:", err)
		os.Exit(exitError)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // best effort: the root is a temp folder
	if err != nil {
		fmt.Fprintln(os.Stderr, "doctor:", err)
		os.Exit(exitCode(err))
	}
}

// run builds both reports and the error table inside root.
func run(w io.Writer, root string) error {
	// Resolve the root once so hideRoot matches what appdir reports: the
	// lookup walks real paths (on macOS the temp folder is a symlink).
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	repo := filepath.Join(root, repoFolder)
	if err := os.MkdirAll(filepath.Join(repo, gitFolder), 0o700); err != nil {
		return fmt.Errorf("make repo: %w", err)
	}
	// `doctor init` once, so there is a project home to report on.
	d, err := resolve(root, repo)
	if err != nil {
		return err
	}
	if _, err := d.InitProject(repo); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	// New never creates anything and reads the disk once: resolve again to
	// see the new home.
	if d, err = resolve(root, repo); err != nil {
		return err
	}

	fmt.Fprintln(w, "== paths --json")
	if err := writePathsJSON(w, d, root); err != nil {
		return err
	}
	fmt.Fprintln(w, "== doctor")
	writeDoctor(w, d, root)
	fmt.Fprintln(w, "== paths --home team")
	if err := writePathsOverride(w, root, repo); err != nil {
		return err
	}
	fmt.Fprintln(w, "== errors")
	if err := writeErrors(w, root); err != nil {
		return err
	}
	fmt.Fprintln(w, "== invalid options")
	return writeInvalidOptions(w, root)
}

// snippet:resolve
// resolve builds the Dirs of the example tool. Every outside input goes
// through a seam: the user home, the start folder of the lookup, the
// environment and the release flag.
func resolve(root, work string, extra ...appdir.Option) (*appdir.Dirs, error) {
	opts := append(seams(root, work),
		appdir.WithRelease(true), // the tool's -ldflags -X variable in a real build
		appdir.WithKind(kindTemplates, appdir.Folder(string(kindTemplates)), appdir.Commit()),
	)
	d, err := appdir.New(appName, append(opts, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}
	return d, nil
}

// seams hands appdir the temp user home, the start folder and the example's
// environment, so nothing is read from the real machine.
func seams(root, work string) []appdir.Option {
	home := filepath.Join(root, homeFolder)
	env := exampleEnv()
	return []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(work),
		appdir.WithEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }),
	}
}

// snippet:end

// snippet:paths-json
// pathsEnvelope is the standard's --json envelope around Paths().
type pathsEnvelope struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	Count         int            `json:"count"`
	Profile       appdir.Profile `json:"profile"`
	Data          []appdir.Entry `json:"data"`
}

// writePathsJSON prints `paths --json`. Entry already carries JSON tags, so
// the tool only wraps the slice in its envelope.
func writePathsJSON(w io.Writer, d *appdir.Dirs, root string) error {
	entries := d.Paths()
	for i := range entries {
		// Only for this example's stable output; a real tool prints the path.
		entries[i].Path = hideRoot(entries[i].Path, root)
		entries[i].Error = hideRoot(entries[i].Error, root)
	}
	env := pathsEnvelope{SchemaVersion: schemaVersion, Kind: envelopeKind, Count: len(entries), Profile: d.Profile(), Data: entries}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(env); err != nil {
		return fmt.Errorf("paths --json: %w", err)
	}
	return nil
}

// snippet:end

// snippet:paths-override
// writePathsOverride prints `paths --home team` from inside the repo. Under
// --home (or <APP>_HOME) there is one home, so a kind that lives in both
// homes (home, config, config-local) is listed once, labelled global, and
// every kind gets the same folder and rule.
func writePathsOverride(w io.Writer, root, repo string) error {
	d, err := resolve(root, repo, appdir.WithHome(filepath.Join(root, teamFolder)))
	if err != nil {
		return err
	}
	for _, e := range d.Paths() {
		fmt.Fprintf(w, "  %-13s %-8s %-5s %s\n", e.Kind, e.Home, e.Rule, hideRoot(e.Path, root))
	}
	return nil
}

// snippet:end

// snippet:doctor
// writeDoctor prints the parts of `doctor` that come from appdir: config
// files in read order, warnings, the layout in effect and the gitignore block.
func writeDoctor(w io.Writer, d *appdir.Dirs, root string) {
	fmt.Fprintf(w, "profile %s (release=%t, %s)\n", d.Profile(), d.IsRelease(), d.ReleaseReason())
	fmt.Fprintln(w, "config files (later wins):")
	for _, c := range d.ConfigFiles() {
		state := "missing"
		if _, err := os.Stat(c.Path); err == nil {
			state = "present"
		}
		fmt.Fprintf(w, "  %-7s %-40s local=%t committed=%t %s\n", c.Home, hideRoot(c.Path, root), c.Local, c.Committed, state)
	}
	fmt.Fprintln(w, "warnings:")
	for _, wn := range d.Warnings() {
		fmt.Fprintf(w, "  %s: %s\n", wn.Code, hideRoot(wn.Message, root))
	}
	fmt.Fprintln(w, "layout:")
	for _, k := range d.Kinds() {
		fmt.Fprintf(w, "  %-13s %-20s %-6s %-6s %s\n", k.Name, k.Path, k.Type, k.Git, k.Scope)
	}
	fmt.Fprintln(w, "gitignore block:")
	for _, line := range strings.Split(strings.TrimSuffix(d.GitignoreBlock(), "\n"), "\n") {
		fmt.Fprintln(w, "  "+line)
	}
}

// snippet:end

// snippet:exit-code
// exitCode maps an error to the standard's exit codes. appdir leaves this to
// the tool; errors.Is works through every wrap.
func exitCode(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, appdir.ErrInvalidOption), errors.Is(err, appdir.ErrUnknownProfile), errors.Is(err, appdir.ErrEnvVar):
		return exitUsage
	case errors.Is(err, appdir.ErrNoProject), errors.Is(err, appdir.ErrNoHome),
		errors.Is(err, appdir.ErrPointerMissing), errors.Is(err, appdir.ErrPointerNotHome):
		return exitNotFound
	case errors.Is(err, appdir.ErrBusy):
		return exitBusy
	case errors.Is(err, appdir.ErrLayoutTooNew):
		return exitTooNew
	case errors.Is(err, appdir.ErrNotHome), errors.Is(err, appdir.ErrGitignoreMarkers), errors.Is(err, appdir.ErrBadHomeFile):
		return exitConflict
	default:
		return exitError
	}
}

// snippet:end

// failure is one provoked error and the sentinel it should match.
type failure struct {
	name     string
	sentinel error
	provoke  func(root string) error
}

// snippet:errors
// failures provokes five real errors, each the way a user would hit it. The
// errors are returned unwrapped on purpose: writeErrors prints them as the
// user would see them.
func failures() []failure {
	return []failure{
		{"ErrNoProject", appdir.ErrNoProject, func(root string) error {
			// A project kind (db) asked for outside any project.
			d, err := resolve(root, filepath.Join(root, elsewhere))
			if err != nil {
				return err
			}
			_, err = d.Path(appdir.KindDB)
			return err
		}},
		{"ErrUnknownProfile", appdir.ErrUnknownProfile, func(root string) error {
			// --profile staging, but the list is prod, dev.
			_, err := resolve(root, root, appdir.WithProfile("staging"))
			return err
		}},
		{"ErrPointerAbsolute", appdir.ErrPointerAbsolute, func(root string) error {
			// An absolute home: pointer in the committed config.jsonc.
			repo, err := initRepo(root, pointerRepo)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(repo, markerFolder, configFile), []byte(`{"home": "/srv/doctor"}`), 0o600); err != nil {
				return err
			}
			d, err := resolve(root, repo)
			if err != nil {
				return err
			}
			_, err = d.Project()
			return err
		}},
		{"ErrUnknownKind", appdir.ErrUnknownKind, func(root string) error {
			// A kind the layout never declared: a typo of "templates".
			d, err := resolve(root, filepath.Join(root, repoFolder))
			if err != nil {
				return err
			}
			_, err = d.Path("template")
			return err
		}},
		{"ErrLayoutTooNew", appdir.ErrLayoutTooNew, func(root string) error {
			// A newer binary wrote layout 2; this one knows 1 and refuses to write.
			repo, err := initRepo(root, tooNewRepo)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(repo, markerFolder, homeInfoFile), []byte(`{"app": "doctor", "layout": 2, "scope": "project"}`), 0o600); err != nil {
				return err
			}
			d, err := resolve(root, repo)
			if err != nil {
				return err
			}
			_, err = d.Ensure(appdir.KindDB)
			return err
		}},
	}
}

// writeErrors provokes each failure and prints its exit code and message.
func writeErrors(w io.Writer, root string) error {
	for _, f := range failures() {
		err := f.provoke(root)
		if !errors.Is(err, f.sentinel) {
			return fmt.Errorf("%s: got %v", f.name, err)
		}
		fmt.Fprintf(w, "%-18s exit %d  %s\n", f.name, exitCode(err), hideRoot(err.Error(), root))
	}
	return nil
}

// snippet:end

// invalidOption is one option set New refuses with ErrInvalidOption.
type invalidOption struct {
	name string
	app  string
	opts []appdir.Option
}

// snippet:invalid-options
// invalidOptions lists option sets New refuses. Each is an author bug or a
// bad value passed through from a flag; New checks them before it reads
// anything, and the error wraps ErrInvalidOption.
func invalidOptions() []invalidOption {
	return []invalidOption{
		{"app name", "Doctor", nil}, // upper case is not allowed
		{"dir name", appName, []appdir.Option{appdir.WithDirName("..")}},
		{"kind path", appName, []appdir.Option{appdir.WithKind("tpl", appdir.Folder("../x"))}},
		{"release profile", appName, []appdir.Option{appdir.WithReleaseProfile("staging")}},
		{"nil seam", appName, []appdir.Option{appdir.WithEnv(nil)}},
	}
}

// writeInvalidOptions calls New with each bad set and prints its exit code
// and message. The seams come first so a set that New wrongly accepted
// would still read only the temp root.
func writeInvalidOptions(w io.Writer, root string) error {
	for _, c := range invalidOptions() {
		_, err := appdir.New(c.app, append(seams(root, root), c.opts...)...)
		if !errors.Is(err, appdir.ErrInvalidOption) {
			return fmt.Errorf("%s: got %v", c.name, err)
		}
		fmt.Fprintf(w, "%-16s exit %d  %s\n", c.name, exitCode(err), err)
	}
	return nil
}

// snippet:end

// initRepo makes root/name a project with a home (no git: the gitignore
// block is not needed for these cases).
func initRepo(root, name string) (string, error) {
	repo := filepath.Join(root, name)
	d, err := resolve(root, repo)
	if err != nil {
		return "", err
	}
	if _, err := d.InitProject(repo); err != nil {
		return "", fmt.Errorf("init %s: %w", name, err)
	}
	return repo, nil
}

// hideRoot replaces the temp root with $ROOT so the output can be pasted
// into the docs unchanged.
func hideRoot(s, root string) string {
	if s == "" {
		return s
	}
	return strings.ReplaceAll(filepath.ToSlash(s), filepath.ToSlash(root), rootPlaceholder)
}
