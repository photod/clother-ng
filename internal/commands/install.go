package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/jolehuit/clother/internal/config"
	"github.com/jolehuit/clother/internal/launchers"
	"github.com/jolehuit/clother/internal/runtime"
	"github.com/jolehuit/clother/internal/update"
	"github.com/jolehuit/clother/internal/version"
)

var downloadLatestBinary = update.DownloadLatestIfNewer

func runInstall(ctx context.Context, c Context) (int, error) {
	isHomebrew := runtime.IsHomebrew()

	var execPath, installedVersion string
	var cleanup func()
	var err error

	if isHomebrew {
		// Homebrew manages the binary lifecycle; skip downloading and copying.
		// os.Executable() returns the stable /opt/homebrew/bin/clother symlink
		// path, so symlinks remain valid after `brew upgrade` without re-running
		// `clother install`.
		execPath, err = os.Executable()
		if err != nil {
			return 1, err
		}
		installedVersion = update.DisplayVersion(version.Value)
	} else {
		execPath, installedVersion, cleanup, err = resolveInstallBinary(ctx)
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			c.Output.Warn("could not fetch latest release; installing current binary instead: %v", err)
		}
	}

	if err := c.Paths.EnsureBaseDirs(); err != nil {
		return 1, err
	}
	config.NormalizeLegacySecrets(c.Secrets, c.Catalog)
	if err := config.SaveConfig(c.Paths.ConfigFile, c.Config); err != nil {
		return 1, err
	}
	if err := config.SaveSecrets(c.Paths.SecretsFile, c.Secrets); err != nil {
		return 1, err
	}
	if err := launchers.Sync(execPath, c.Paths, c.Catalog, c.Config, isHomebrew); err != nil {
		return 1, err
	}
	if err := syncClaudeShim(c, execPath, isHomebrew); err != nil {
		return 1, err
	}
	for _, legacy := range []string{
		filepath.Join(c.Paths.DataDir, "clother-full.sh"),
		filepath.Join(c.Paths.DataDir, "banner"),
	} {
		_ = os.Remove(legacy)
	}
	c.Output.Success("installed Clother %s to %s", installedVersion, c.Paths.BinDir)
	if !pathContainsDir(os.Getenv("PATH"), c.Paths.BinDir) {
		c.Output.Warn("%s is not on PATH; add `export PATH=\"%s:$PATH\"` to your shell profile and restart your shell", c.Paths.BinDir, c.Paths.BinDir)
	}
	return 0, nil
}

func resolveInstallBinary(ctx context.Context) (string, string, func(), error) {
	if path, latest, cleanup, err := downloadLatestBinary(ctx, version.Value); err == nil && path != "" {
		return path, latest, cleanup, nil
	} else if err != nil {
		current, currentErr := os.Executable()
		if currentErr != nil {
			return "", "", nil, currentErr
		}
		return current, update.DisplayVersion(version.Value), nil, err
	}

	current, err := os.Executable()
	if err != nil {
		return "", "", nil, err
	}
	return current, update.DisplayVersion(version.Value), nil, nil
}

func pathContainsDir(pathEnv, dir string) bool {
	target := normalizePathDir(dir)
	if target == "" {
		return false
	}
	for _, entry := range filepath.SplitList(pathEnv) {
		if normalizePathDir(entry) == target {
			return true
		}
	}
	return false
}

func normalizePathDir(dir string) string {
	if dir == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Clean(dir)
}

// wantClaudeShim decides whether install manages the `claude` shim: explicit
// flags win, otherwise an existing Clother shim is kept and none is created.
func wantClaudeShim(c Context) bool {
	if c.Options.ClaudeShim {
		return true
	}
	if c.Options.NoClaudeShim {
		return false
	}
	return launchers.IsClotherShim(filepath.Join(c.Paths.BinDir, "claude"))
}

// syncClaudeShim installs or refreshes the shim when wanted, moving a real
// claude out of the slot first; with --no-claude-shim it removes the shim and
// restores claude-real. A real claude is never moved unless a shim replaces it.
func syncClaudeShim(c Context, execPath string, isHomebrew bool) error {
	if !wantClaudeShim(c) {
		if c.Options.NoClaudeShim {
			return restoreRealClaude(c)
		}
		return nil
	}
	realClaude, err := runtime.FindRealClaude(c.Paths)
	if err != nil {
		if launchers.IsClotherShim(filepath.Join(c.Paths.BinDir, "claude")) {
			c.Output.Warn("claude not found; the existing `claude` shim was left as is. Run `clother install` again after installing Claude Code")
		} else {
			c.Output.Warn("claude not found; the `claude` shim was not created. Run `clother install --claude-shim` again after installing Claude Code")
		}
		return nil
	}
	if err := runtime.PreserveRealClaude(c.Paths, realClaude); err != nil {
		return err
	}
	if err := launchers.InstallClaudeShim(c.Paths, launchers.SymlinkTarget(execPath, isHomebrew)); err != nil {
		if !errors.Is(err, launchers.ErrClaudeNotShim) {
			return err
		}
		c.Output.Warn("left %s alone: it is not a Clother shim", filepath.Join(c.Paths.BinDir, "claude"))
	}
	return nil
}
