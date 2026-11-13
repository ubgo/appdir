package appdir

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ubgo/appdir/internal/jsonclite"
)

// envPrefixFor turns an app name into its env prefix: "my-app" -> "MY_APP".
func envPrefixFor(app string) string {
	r := strings.NewReplacer("-", "_", ".", "_")
	return strings.ToUpper(r.Replace(app))
}

// defaultConfigReader parses a JSONC file with the internal reader. Replaced
// by ubgo/jsonc when it exists (SPEC 6.3).
func defaultConfigReader(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := jsonclite.Object(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// getenv returns a variable, treating set-but-empty as unset (SPEC 4.1).
func (c *config) getenv(name string) (string, bool) {
	v, ok := c.lookupEnv(name)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// envName builds "<PREFIX>_<suffix>".
func (c *config) envName(suffix string) string { return c.prefix + "_" + suffix }

// varOpen starts the only variable form a user path expands ("${VAR}");
// "$VAR" without braces stays literal.
const varOpen = "${"

// expandUserPath expands "~", "~/..." and "${VAR}" in a user-supplied path
// and makes it absolute against base. "$VAR" without braces stays literal;
// "~user" is refused; an unset or empty ${VAR} is ErrEnvVar naming it.
// ds:def id=config-expanduserpath-67kdzfc9 owner=@khanakia stability=stable
func (c *config) expandUserPath(raw, base, home string, homeErr error) (string, error) {
	s := raw
	switch {
	case s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`):
		if homeErr != nil {
			return "", newErr(ErrNoHome, "%q needs a home folder", raw)
		}
		s = home + s[1:]
	case strings.HasPrefix(s, "~"):
		return "", newErr(ErrInvalidOption, "%q: the ~user form is not supported", raw)
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return "", newErr(ErrInvalidOption, "%q: unclosed ${", raw)
			}
			name := s[i+2 : i+end]
			v, ok := c.getenv(name)
			if !ok {
				return "", newErr(ErrEnvVar, "${%s} is not set", name)
			}
			b.WriteString(v)
			i += end
			continue
		}
		b.WriteByte(s[i])
	}
	p := filepath.FromSlash(b.String())
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p), nil
}

// realPath resolves symlinks, falling back to the cleaned path when the path
// (or part of it) does not exist yet: the walk-up and comparisons want the
// real path when there is one.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	// Resolve the longest existing parent, then re-append the rest.
	dir, rest := p, ""
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Clean(p)
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(r, rest)
		}
	}
}

// sameFile compares two paths by identity (device + inode, or the Windows
// file index), never by string. Missing paths are never the same.
func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ia, ib)
}

// samePlannedPath compares two paths that may not exist yet. When both
// exist it is sameFile. When one exists and the other does not they differ.
// When neither exists it compares their parent folders by identity and their
// names (case-insensitively on macOS and Windows, whose disks usually are),
// so "~/.lore" before its first creation still matches a symlinked or
// differently spelled path to the same place.
func samePlannedPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	ea, eb := exists(a), exists(b)
	switch {
	case ea && eb:
		return sameFile(a, b)
	case ea != eb:
		return false
	}
	if !sameFile(filepath.Dir(a), filepath.Dir(b)) {
		return false
	}
	return sameName(filepath.Base(a), filepath.Base(b))
}

// sameName compares two path texts (names or whole paths) the way the
// platform's usual disk does: case-insensitively on macOS and Windows, whose
// default filesystems fold case, exactly elsewhere. It is only for paths
// that do not exist yet; existing ones are compared by identity (sameFile).
func sameName(x, y string) bool {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return strings.EqualFold(x, y)
	}
	return x == y
}

// within reports whether p is base or inside it, comparing real paths.
func within(p, base string) bool {
	rp, rb := realPath(p), realPath(base)
	rel, err := filepath.Rel(rb, rp)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// exists reports whether p exists (following links).
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// isDir reports whether p is a folder (following links).
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
