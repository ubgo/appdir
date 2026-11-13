// Command demo prints what appdir resolves for an app named "demo", as text
// or as JSON. It is the runnable example for the package.
//
//	demo [--json] [--home DIR] [--profile NAME] [--daemon-profile NAME] [--init] [--init-home DIR] [--ensure KIND]
//
// Build a release binary with: go build -ldflags "-X main.release=true" ./examples/demo
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/appdir"
)

// Names used by the demo.
const (
	appName     = "demo"
	legacyHome  = ".demo-old" // where an older version kept its global home
	releaseTrue = "true"      // the -ldflags value that marks a release build
)

// release is set by a release script: -ldflags "-X main.release=true".
// Empty means "let appdir read Go's build info". It is the one package
// variable on purpose: -X can only set a package-level string variable.
var release string

// report is the JSON shape of one run.
type report struct {
	App           string              `json:"app"`
	Profile       appdir.Profile      `json:"profile"`
	Release       bool                `json:"release"`
	ReleaseReason string              `json:"release_reason"`
	Banner        string              `json:"banner,omitempty"`
	Global        appdir.Home         `json:"global"`
	Project       *appdir.Home        `json:"project,omitempty"`
	ProjectError  string              `json:"project_error,omitempty"`
	Paths         []appdir.Entry      `json:"paths"`
	ConfigFiles   []appdir.ConfigFile `json:"config_files"`
	Socket        *appdir.Socket      `json:"socket,omitempty"`
	Discovery     string              `json:"discovery,omitempty"`
	DaemonProfile appdir.Profile      `json:"daemon_profile"`
	Warnings      []appdir.Warning    `json:"warnings,omitempty"`
	Init          string              `json:"init,omitempty"`
	Ensured       string              `json:"ensured,omitempty"`
	CheckDB       string              `json:"check_db,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
}

// run parses args, resolves the demo app from the real process (env, home,
// working folder: it is a command line tool, not a sandboxed example), runs
// the write flags, and prints the report to stdout. The banner goes to stderr
// so --json output stays parseable.
func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	home := fs.String("home", "", "use this home for everything")
	profile := fs.String("profile", "", "profile (prod, dev)")
	daemonProfile := fs.String("daemon-profile", "", "talk to this profile's daemon")
	doInit := fs.Bool("init", false, "create .demo/ in the current folder (demo init)")
	ensure := fs.String("ensure", "", "create this kind's folder (e.g. db, cache)")
	initHome := fs.String("init-home", "", "create a project home at this path (demo init --home X)")
	short := fs.Bool("short", false, "print only the key lines")
	checkDB := fs.String("check-db", "", "run the prod-db guard on this path")
	if err := fs.Parse(args); err != nil {
		return err // the flag package already printed the usage
	}

	opts := []appdir.Option{
		appdir.WithHome(*home),
		appdir.WithProfile(*profile),
		appdir.WithDaemonProfile(*daemonProfile),
		appdir.WithLegacyHome(appdir.LegacyHome{Dir: legacyHome}),
	}
	// snippet:release
	// Pass the release flag only when the release script set it: an
	// unconditional WithRelease(false) would turn a user's
	// `go install module@vX` into a dev build.
	if release != "" {
		opts = append(opts, appdir.WithRelease(release == releaseTrue))
	}
	// snippet:end
	d, err := appdir.New(appName, opts...)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	// init commands write, then the tool resolves again to see the result.
	var initDone string
	if *doInit {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("working folder: %w", err)
		}
		if initDone, err = d.InitProject(cwd); err != nil {
			return fmt.Errorf("init: %w", err)
		}
	}
	if *initHome != "" {
		if err := d.InitHome(*initHome, appdir.ScopeProject); err != nil {
			return fmt.Errorf("init home: %w", err)
		}
		initDone = *initHome
	}
	if initDone != "" {
		if d, err = appdir.New(appName, opts...); err != nil {
			return fmt.Errorf("appdir: %w", err)
		}
	}
	if b := d.Banner(); b != "" {
		fmt.Fprintln(stderr, b)
	}

	r := report{
		App: d.App(), Profile: d.Profile(), Release: d.IsRelease(), ReleaseReason: d.ReleaseReason(),
		Banner: d.Banner(), Global: d.Global(), Paths: d.Paths(), ConfigFiles: d.ConfigFiles(),
		DaemonProfile: d.DaemonProfile(), Warnings: d.Warnings(), Init: initDone,
	}
	if p, err := d.Project(); err == nil {
		r.Project = &p
	} else {
		r.ProjectError = err.Error()
	}
	if s, err := d.Socket(); err == nil {
		r.Socket = &s
	}
	if path, _, err := d.DaemonDiscovery(); err == nil {
		r.Discovery = path
	}
	if *ensure != "" {
		if r.Ensured, err = d.Ensure(appdir.Kind(*ensure)); err != nil {
			r.Ensured = "error: " + err.Error()
		}
	}

	if *checkDB != "" {
		access, err := d.CheckDB(*checkDB)
		r.CheckDB = access.String()
		if err != nil {
			r.CheckDB = "refused: " + err.Error()
		}
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	printText(stdout, r, *short)
	return nil
}

// shortKind reports whether --short prints kind k.
func shortKind(k appdir.Kind) bool {
	switch k {
	case appdir.KindDB, appdir.KindCache, appdir.KindShared:
		return true
	default:
		return false
	}
}

// printText prints the report as aligned text. Errors that only repeat the
// project error are collapsed to one pointer line, so the output stays short.
func printText(w io.Writer, r report, short bool) {
	fmt.Fprintf(w, "profile   %s (release=%t, %s)\n", r.Profile, r.Release, r.ReleaseReason)
	fmt.Fprintf(w, "global    %s  [%s]\n", orNone(r.Global.Base), r.Global.Rule)
	if r.Project != nil {
		fmt.Fprintf(w, "project   %s  [%s]", r.Project.Base, r.Project.Rule)
		if r.Project.Data != r.Project.Base {
			fmt.Fprintf(w, "  data=%s", r.Project.Data)
		}
		if r.Project.Marker != "" && r.Project.Marker != r.Project.Base {
			fmt.Fprintf(w, "  marker=%s", r.Project.Marker)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "project   none: %s\n", r.ProjectError)
	}
	fmt.Fprintln(w, "paths:")
	for _, e := range r.Paths {
		if short && !shortKind(e.Kind) {
			continue
		}
		val := e.Path
		switch {
		case e.Error != "" && e.Error == r.ProjectError:
			val = "error: no usable project home (above)"
		case e.Error != "":
			val = "error: " + e.Error
		}
		fb := ""
		if e.Fallback {
			fb = " fallback"
		}
		fmt.Fprintf(w, "  %-13s %-8s %-60s [%s%s]\n", e.Kind, e.Home, orNone(val), e.Rule, fb)
	}
	if !short {
		fmt.Fprintln(w, "config files (read order):")
	}
	for _, c := range r.ConfigFiles {
		if short {
			break
		}
		prof := ""
		if c.Profile != "" {
			prof = " profile=" + string(c.Profile)
		}
		fmt.Fprintf(w, "  %s (local=%t committed=%t%s)\n", c.Path, c.Local, c.Committed, prof)
	}
	if r.Socket != nil {
		fmt.Fprintf(w, "socket    %s (fallback=%t)\n", r.Socket.Path, r.Socket.Fallback)
	}
	if r.Discovery != "" {
		fmt.Fprintf(w, "discovery %s (daemon profile %s)\n", r.Discovery, r.DaemonProfile)
	}
	for _, wn := range r.Warnings {
		fmt.Fprintf(w, "warning   %s: %s\n", wn.Code, wn.Message)
	}
	if r.Init != "" {
		fmt.Fprintf(w, "init      %s\n", r.Init)
	}
	if r.CheckDB != "" {
		fmt.Fprintf(w, "check-db  %s\n", r.CheckDB)
	}
	if r.Ensured != "" {
		fmt.Fprintf(w, "ensured   %s\n", r.Ensured)
	}
}

// orNone shows an empty value as "-" so the columns stay aligned.
func orNone(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
