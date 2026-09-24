package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jolehuit/clother/internal/launchers"
)

func runUninstall(_ context.Context, c Context) (int, error) {
	if !c.Options.Yes {
		ok, err := c.Prompt.Confirm("Remove all Clother files?", false)
		if err != nil {
			return 1, err
		}
		if !ok {
			return 0, nil
		}
	}
	manifest, _ := launchers.LoadManifest(c.Paths.ManifestFile)
	for _, name := range manifest.Launchers {
		_ = os.Remove(filepath.Join(c.Paths.BinDir, name))
	}
	restoreRealClaude(c)
	_ = os.Remove(filepath.Join(c.Paths.BinDir, "clother"))
	_ = os.RemoveAll(c.Paths.ConfigDir)
	_ = os.RemoveAll(c.Paths.DataDir)
	_ = os.RemoveAll(c.Paths.CacheDir)
	fmt.Fprintln(c.Output.Stdout, "Clother uninstalled")
	return 0, nil
}

// restoreRealClaude removes the claude shim only if Clother owns it, then moves
// a preserved claude-real back into the freed slot. A real claude is never
// touched, and a dangling claude-real (its version pruned by Claude's updater)
// is dropped with a warning instead of being restored as a broken claude.
func restoreRealClaude(c Context) {
	binDir := c.Paths.BinDir
	shim := filepath.Join(binDir, "claude")
	preserved := filepath.Join(binDir, "claude-real")
	if launchers.IsClotherShim(shim) {
		if err := os.Remove(shim); err != nil {
			c.Output.Warn("could not remove the claude shim %s: %v", shim, err)
			return
		}
	}
	if _, err := os.Lstat(preserved); err != nil {
		return
	}
	if _, err := os.Stat(preserved); err != nil {
		_ = os.Remove(preserved)
		c.Output.Warn("%s pointed at a removed Claude Code version; reinstall Claude Code to restore `claude`", preserved)
		return
	}
	if _, err := os.Lstat(shim); !os.IsNotExist(err) {
		return
	}
	if err := os.Rename(preserved, shim); err != nil {
		c.Output.Warn("could not restore %s to %s: %v", preserved, shim, err)
	}
}
