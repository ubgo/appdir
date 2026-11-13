//go:build darwin

package appdir

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// excludeFromBackup adds an empty .metadata_never_index (Spotlight) and the
// Time Machine exclusion. The stdlib has no setxattr on darwin, so the
// exclusion goes through tmutil, best effort: a missing tmutil or a failure
// never blocks creating the cache.
func excludeFromBackup(dir string) error {
	f := filepath.Join(dir, ".metadata_never_index")
	if !exists(f) {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "tmutil", "addexclusion", dir).Run() // best effort by design
	return nil
}
