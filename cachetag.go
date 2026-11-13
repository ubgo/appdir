package appdir

import (
	"fmt"
	"os"
	"path/filepath"
)

// tagCache marks a disposable folder so backup tools and indexers skip it
// (SPEC 5.5). OS extras live in cachetag_<os>.go.
// ds:def id=dirs-tagcache-8cz6ae4w owner=@khanakia stability=stable
func (d *Dirs) tagCache(dir string) error {
	body := "Signature: 8a477f597d28d172789f06886806bc55\n" +
		"# This file is a cache directory tag created by " + d.cfg.app + ".\n" +
		"# For information about cache directory tags, see:\n" +
		"#\thttps://bford.info/cachedir/\n"
	tag := filepath.Join(dir, cacheTagFile)
	if !exists(tag) {
		if err := os.WriteFile(tag, []byte(body), d.cfg.filePerm); err != nil {
			return fmt.Errorf("cache tag %s: %w", tag, err)
		}
	}
	return excludeFromBackup(dir)
}
