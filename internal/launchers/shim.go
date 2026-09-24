package launchers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jolehuit/clother/internal/config"
)

// ErrClaudeNotShim reports that $BinDir/claude exists and is not a Clother
// shim, so Clother must leave it alone.
var ErrClaudeNotShim = errors.New("claude in bin dir is not a clother shim")

// SymlinkTarget returns what launcher symlinks point at: "clother" relative to
// the bin dir for copied installs, or the absolute execPath under Homebrew.
func SymlinkTarget(execPath string, skipCopy bool) string {
	if skipCopy {
		return execPath
	}
	return "clother"
}

// IsClotherShim reports whether path is a symlink whose target is a clother
// binary. Real Claude Code installs (regular files or symlinks into Claude's
// versions dir) are never shims.
func IsClotherShim(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	return filepath.Base(filepath.Clean(target)) == "clother"
}

// InstallClaudeShim points $BinDir/claude at symlinkTarget. It creates the
// shim when the slot is empty and refreshes an existing Clother shim, but
// returns ErrClaudeNotShim without touching anything else.
func InstallClaudeShim(paths config.Paths, symlinkTarget string) error {
	shim := filepath.Join(paths.BinDir, "claude")
	if _, err := os.Lstat(shim); err == nil {
		if !IsClotherShim(shim) {
			return fmt.Errorf("%w: %s", ErrClaudeNotShim, shim)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// Build the link under a temporary name and rename it over the slot, so an
	// existing shim is replaced atomically instead of remove-then-create.
	tmp := filepath.Join(paths.BinDir, fmt.Sprintf(".claude-shim-%d", os.Getpid()))
	_ = os.Remove(tmp)
	if err := os.Symlink(symlinkTarget, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, shim); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
