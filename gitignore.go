package appdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitignoreBlock returns the marker block for .<app>/.gitignore (SPEC 5.7):
// ignore everything, then re-include the committed files, folders first
// because git cannot re-include a file under an excluded folder.
// ds:def id=dirs-gitignoreblock-8bujrmug owner=@khanakia stability=stable
func (d *Dirs) GitignoreBlock() string {
	ignored := map[string]bool{}
	for _, p := range d.cfg.gitIgnore {
		ignored[strings.TrimSuffix(p, "/")] = true
	}
	// removed holds the ignore patterns that cancelled a commit entry: the
	// leading `*` already ignores those, so they are not written again as a
	// plain line (SPEC 5.7: only "any other" pattern is written).
	removed := map[string]bool{}
	var lines []string
	seen := map[string]bool{}
	add := func(l string) {
		if !seen[l] {
			seen[l] = true
			lines = append(lines, l)
		}
	}
	addParents := func(p string) {
		segs := strings.Split(p, "/")
		for i := 0; i < len(segs)-1; i++ {
			if strings.ContainsAny(segs[i], "*?[") {
				return
			}
			add("!" + strings.Join(segs[:i+1], "/") + "/")
		}
	}
	// The .gitignore itself first, as in the standard's block.
	ordered := make([]KindSpec, 0, len(d.kinds))
	for _, k := range d.kinds {
		if k.Name == KindGitignore {
			ordered = append([]KindSpec{k}, ordered...)
		} else {
			ordered = append(ordered, k)
		}
	}
	for _, k := range ordered {
		if k.Git != GitCommit {
			continue
		}
		if ignored[k.Path] {
			removed[k.Path] = true
			continue
		}
		addParents(k.Path)
		if k.Type == TypeFolder {
			add("!" + k.Path + "/")
			add("!" + k.Path + "/**")
		} else {
			add("!" + k.Path)
		}
	}
	for _, p := range d.cfg.gitCommit {
		if key := strings.TrimSuffix(p, "/"); ignored[key] {
			removed[key] = true
			continue
		}
		addParents(p)
		add("!" + p)
	}
	var extra []string
	for _, p := range d.cfg.gitIgnore {
		// A pattern that only matched a generated re-include line must still
		// be written: placed after the re-includes it wins in git, which is
		// what "ignore wins over every commit rule" means. Only a pattern
		// that already removed a whole commit entry is left out.
		if !removed[strings.TrimSuffix(p, "/")] {
			extra = append(extra, p)
		}
	}
	var b strings.Builder
	b.WriteString(d.beginMarker() + "\n*\n")
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	for _, l := range extra {
		b.WriteString(l + "\n")
	}
	b.WriteString(d.endMarker() + "\n")
	return b.String()
}

func (d *Dirs) beginMarker() string { return "# " + d.cfg.app + ":begin" }
func (d *Dirs) endMarker() string   { return "# " + d.cfg.app + ":end" }

// WriteGitignore writes the block into the project home's .gitignore. It
// returns false (and writes nothing) when the home is not in a git work tree.
func (d *Dirs) WriteGitignore() (bool, error) {
	if d.projectErr != nil {
		return false, d.projectErr
	}
	// A write method like every other (SPEC 4.8): refuse a home whose
	// layout is newer than this binary, and root in another user's home.
	base := d.project.Base
	if err := d.checkWrite(base, d.readHomeInfo(base)); err != nil {
		return false, err
	}
	return d.writeGitignoreAt(base)
}

// writeGitignoreAt replaces the text between the markers in base/.gitignore,
// never touching other lines; appends the block when there are no markers.
// ds:def id=dirs-writegitignoreat-6hp578mz owner=@khanakia stability=stable
func (d *Dirs) writeGitignoreAt(base string) (bool, error) {
	spec, ok := d.kind(KindGitignore)
	if !ok || !inGitWorkTree(base) {
		return false, nil
	}
	path := filepath.Join(base, filepath.FromSlash(spec.Path))
	old, err := os.ReadFile(path)
	if err != nil && !isNotExist(err) {
		return true, fmt.Errorf("read %s: %w", path, err)
	}
	block := d.GitignoreBlock()
	lines := strings.SplitAfter(string(old), "\n")
	begin, end, nb, ne := -1, -1, 0, 0
	for i, l := range lines {
		switch strings.TrimRight(l, "\r\n") {
		case d.beginMarker():
			begin, nb = i, nb+1
		case d.endMarker():
			end, ne = i, ne+1
		}
	}
	var out string
	switch {
	case nb == 0 && ne == 0:
		out = string(old)
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += block
	case nb == 1 && ne == 1 && begin < end:
		out = strings.Join(lines[:begin], "") + block + strings.Join(lines[end+1:], "")
	default:
		return true, newErr(ErrGitignoreMarkers, "%s: %s / %s missing, doubled or out of order; fix it by hand", path, d.beginMarker(), d.endMarker())
	}
	if out == string(old) {
		return true, nil
	}
	return true, d.writeFileAtomic(path, []byte(out), d.cfg.filePerm)
}
