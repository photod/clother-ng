package commands

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jolehuit/clother/internal/cli"
	"github.com/jolehuit/clother/internal/config"
	"github.com/jolehuit/clother/internal/launchers"
	"github.com/jolehuit/clother/internal/providers"
	"github.com/jolehuit/clother/internal/ui"
)

// Bug: persistConfig calls launchers.Sync on every `clother config` save, and
// Sync used to unconditionally delete and recreate $BinDir/claude, wiping out
// a real Claude Code installation living there.
func TestPersistConfigLeavesRealClaudeUntouched(t *testing.T) {
	ctx, _ := newConfigTestContext(t, "")
	binDir := ctx.Paths.BinDir
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	content := []byte("#!/bin/sh\necho real\n")
	if err := os.WriteFile(claudePath, content, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := persistConfig(ctx); err != nil {
		t.Fatalf("persistConfig() error = %v", err)
	}

	info, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatalf("claude should still exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("claude should remain a regular file after persistConfig")
	}
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("claude content changed: got %q, want %q", got, content)
	}
}

// Bug: runUninstall deleted $BinDir/claude unconditionally without checking
// whether it was a Clother shim, destroying a real Claude Code install.
func TestRunUninstallLeavesRealClaudeFileUntouched(t *testing.T) {
	ctx, _ := newConfigTestContext(t, "")
	ctx.Options.Yes = true
	binDir := ctx.Paths.BinDir
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	content := []byte("real")
	if err := os.WriteFile(claudePath, content, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := runUninstall(context.Background(), ctx); err != nil {
		t.Fatalf("runUninstall() error = %v", err)
	}

	info, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatalf("claude should survive uninstall: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("claude should remain a regular file after uninstall")
	}
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("claude content changed: got %q, want %q", got, content)
	}
}

func TestRunUninstallRestoresRealClaudeFromShim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	ctx, _ := newConfigTestContext(t, "")
	ctx.Options.Yes = true
	binDir := ctx.Paths.BinDir
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	realPath := filepath.Join(binDir, "claude-real")
	if err := os.WriteFile(realPath, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("clother", claudePath); err != nil {
		t.Fatal(err)
	}

	if _, err := runUninstall(context.Background(), ctx); err != nil {
		t.Fatalf("runUninstall() error = %v", err)
	}

	info, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatalf("claude should be restored after uninstall: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("restored claude should be a regular file, not a symlink")
	}
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "real" {
		t.Fatalf("restored claude content = %q, want %q", got, "real")
	}
	if _, err := os.Lstat(realPath); !os.IsNotExist(err) {
		t.Fatalf("claude-real should be gone after restore, stat err = %v", err)
	}
}

func TestRunUninstallRemovesShimWithoutRealClaude(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	ctx, _ := newConfigTestContext(t, "")
	ctx.Options.Yes = true
	binDir := ctx.Paths.BinDir
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	if err := os.Symlink("clother", claudePath); err != nil {
		t.Fatal(err)
	}

	if _, err := runUninstall(context.Background(), ctx); err != nil {
		t.Fatalf("runUninstall() error = %v", err)
	}

	if _, err := os.Lstat(claudePath); !os.IsNotExist(err) {
		t.Fatalf("shim claude should be removed, stat err = %v", err)
	}
}

// Bug companion: when no real claude can be found anywhere, runInstall must
// not create a claude shim pointing at nothing.
func TestRunInstallSkipsShimWhenNoRealClaudeFound(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	binDir := filepath.Join(root, "bin")
	emptyPathDir := filepath.Join(root, "empty-path")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLOTHER_BIN", binDir)
	t.Setenv("CLOTHER_SKIP_SELF_UPDATE", "1")

	if err := os.MkdirAll(emptyPathDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", emptyPathDir)

	paths, err := config.Detect("")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{},
		CustomProviders:   map[string]config.CustomProvider{},
	}
	output := &ui.Output{Stdout: io.Discard, Stderr: io.Discard, Format: ui.FormatHuman}

	code, err := runInstall(context.Background(), Context{
		Paths:   paths,
		Config:  cfg,
		Secrets: config.Secrets{},
		Catalog: catalog,
		Output:  output,
	})
	if err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0", code)
	}

	if _, err := os.Lstat(filepath.Join(binDir, "claude")); !os.IsNotExist(err) {
		t.Fatalf("claude shim should be skipped when no real claude is found, stat err = %v", err)
	}
}

// Companion to TestRunInstallPreservesSameBinClaude (install_test.go): when a
// real claude IS found, the shim installed at $BinDir/claude must be a
// genuine Clother shim, and the original binary must be preserved intact.
func TestRunInstallCreatesClotherShimWhenRealClaudeFound(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	binDir := filepath.Join(root, "bin")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLOTHER_BIN", binDir)
	t.Setenv("CLOTHER_SKIP_SELF_UPDATE", "1")

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realClaude := filepath.Join(binDir, "claude")
	realContent := []byte("#!/bin/sh\necho real\n")
	if err := os.WriteFile(realClaude, realContent, 0o755); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	paths, err := config.Detect("")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{},
		CustomProviders:   map[string]config.CustomProvider{},
	}
	output := &ui.Output{Stdout: io.Discard, Stderr: io.Discard, Format: ui.FormatHuman}

	code, err := runInstall(context.Background(), Context{
		Paths:   paths,
		Config:  cfg,
		Secrets: config.Secrets{},
		Catalog: catalog,
		Output:  output,
		Options: cli.Options{ClaudeShim: true},
	})
	if err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0", code)
	}

	if !launchers.IsClotherShim(filepath.Join(binDir, "claude")) {
		t.Fatal("expected $BinDir/claude to be a Clother shim after install")
	}
	got, err := os.ReadFile(filepath.Join(binDir, "claude-real"))
	if err != nil {
		t.Fatalf("expected preserved real claude: %v", err)
	}
	if string(got) != string(realContent) {
		t.Fatalf("claude-real content = %q, want %q", got, realContent)
	}
	// A non-Homebrew install must use a relative link ("clother"), not an
	// absolute path, so the shim keeps working if BinDir is ever moved.
	linkTarget, err := os.Readlink(filepath.Join(binDir, "claude"))
	if err != nil {
		t.Fatalf("expected claude to be a symlink: %v", err)
	}
	if linkTarget != "clother" {
		t.Fatalf("claude symlink target = %q, want %q", linkTarget, "clother")
	}
}

// Bug (data-loss on upgrade): when $BinDir/claude is already a Clother shim
// left on PATH from a previous install, and the test/downloaded binary
// running `clother install` is not that shim, FindRealClaude used to treat
// the shim itself as "the real claude". PreserveRealClaude then renamed the
// shim onto claude-real, destroying the actual preserved Claude Code binary
// that was sitting there. runInstall must recognize the existing shim,
// leave claude-real alone, and install a fresh shim over it.
func TestRunInstallUpgradeOverExistingShimKeepsRealClaude(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	binDir := filepath.Join(root, "bin")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLOTHER_BIN", binDir)
	t.Setenv("CLOTHER_SKIP_SELF_UPDATE", "1")

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "clother"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("clother", filepath.Join(binDir, "claude")); err != nil {
		t.Fatal(err)
	}
	realContent := []byte("#!/bin/sh\necho real\n")
	if err := os.WriteFile(filepath.Join(binDir, "claude-real"), realContent, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir)

	paths, err := config.Detect("")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{},
		CustomProviders:   map[string]config.CustomProvider{},
	}
	output := &ui.Output{Stdout: io.Discard, Stderr: io.Discard, Format: ui.FormatHuman}

	code, err := runInstall(context.Background(), Context{
		Paths:   paths,
		Config:  cfg,
		Secrets: config.Secrets{},
		Catalog: catalog,
		Output:  output,
		Options: cli.Options{ClaudeShim: true},
	})
	if err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0", code)
	}

	realPath := filepath.Join(binDir, "claude-real")
	realInfo, err := os.Lstat(realPath)
	if err != nil {
		t.Fatalf("claude-real should still exist: %v", err)
	}
	if realInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("claude-real should remain a regular file, not become a symlink to the old shim")
	}
	got, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(realContent) {
		t.Fatalf("claude-real content changed: got %q, want %q", got, realContent)
	}

	if !launchers.IsClotherShim(filepath.Join(binDir, "claude")) {
		t.Fatal("expected $BinDir/claude to be a Clother shim after install")
	}
}

// Bug: restoreRealClaude blindly restored claude-real even when it was a
// dangling symlink (Claude Code's auto-updater had pruned the version it
// pointed at), leaving claude as a broken link instead of removing the
// dangling claude-real outright.
func TestRunUninstallRemovesDanglingClaudeReal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	ctx, _ := newConfigTestContext(t, "")
	ctx.Options.Yes = true
	var out bytes.Buffer
	ctx.Output = &ui.Output{Stdout: &out, Stderr: &out, Format: ui.FormatHuman}

	binDir := ctx.Paths.BinDir
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	if err := os.Symlink("clother", claudePath); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(binDir)
	dangling := filepath.Join(root, "versions", "9.9.9")
	realPath := filepath.Join(binDir, "claude-real")
	if err := os.Symlink(dangling, realPath); err != nil {
		t.Fatal(err)
	}

	if _, err := runUninstall(context.Background(), ctx); err != nil {
		t.Fatalf("runUninstall() error = %v", err)
	}

	if _, err := os.Lstat(claudePath); !os.IsNotExist(err) {
		t.Fatalf("claude should not exist after uninstall (not restored as a broken link), stat err = %v", err)
	}
	if _, err := os.Lstat(realPath); !os.IsNotExist(err) {
		t.Fatalf("dangling claude-real should be removed, not kept, stat err = %v", err)
	}
	if !strings.Contains(out.String(), "claude-real") {
		t.Fatalf("uninstall output should mention claude-real, got %q", out.String())
	}
}

// Regression guard: a foreign symlink at $BinDir/claude (not a Clother shim)
// must survive uninstall untouched, dangling claude-real handling included.
func TestRunUninstallLeavesForeignSymlinkClaudeUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	ctx, _ := newConfigTestContext(t, "")
	ctx.Options.Yes = true
	binDir := ctx.Paths.BinDir
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	versionsRoot := t.TempDir()
	target := filepath.Join(versionsRoot, "versions", "2.1.3")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho real\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	if err := os.Symlink(target, claudePath); err != nil {
		t.Fatal(err)
	}

	if _, err := runUninstall(context.Background(), ctx); err != nil {
		t.Fatalf("runUninstall() error = %v", err)
	}

	got, err := os.Readlink(claudePath)
	if err != nil {
		t.Fatalf("foreign claude symlink should survive uninstall: %v", err)
	}
	if got != target {
		t.Fatalf("claude symlink target changed: got %q, want %q", got, target)
	}
}

// New default behavior: the claude shim is opt-in. With no flags and a
// regular-file real claude already sitting in BinDir, runInstall must leave
// it alone entirely -- no shim, no claude-real. This is the key behavior
// change from the old "shim by default" install.
func TestRunInstallDefaultLeavesRealClaudeUntouched(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	binDir := filepath.Join(root, "bin")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLOTHER_BIN", binDir)
	t.Setenv("CLOTHER_SKIP_SELF_UPDATE", "1")

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	realContent := []byte("#!/bin/sh\necho real\n")
	if err := os.WriteFile(claudePath, realContent, 0o755); err != nil {
		t.Fatal(err)
	}

	// binDir/claude is the only claude on PATH, so it is unambiguously "the"
	// real claude regardless of what else is installed on the host running
	// this test.
	t.Setenv("PATH", binDir)

	paths, err := config.Detect("")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{},
		CustomProviders:   map[string]config.CustomProvider{},
	}
	output := &ui.Output{Stdout: io.Discard, Stderr: io.Discard, Format: ui.FormatHuman}

	code, err := runInstall(context.Background(), Context{
		Paths:   paths,
		Config:  cfg,
		Secrets: config.Secrets{},
		Catalog: catalog,
		Output:  output,
	})
	if err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0", code)
	}

	info, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatalf("claude should still exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("claude should remain a regular file, not become a shim, with no flags")
	}
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(realContent) {
		t.Fatalf("claude content changed: got %q, want %q", got, realContent)
	}
	if _, err := os.Lstat(filepath.Join(binDir, "claude-real")); !os.IsNotExist(err) {
		t.Fatalf("claude-real should not be created with no flags, stat err = %v", err)
	}
}

// Regression guard: with no flags, an existing Clother shim from a previous
// install is kept as is, and the real claude it preserved is untouched.
func TestRunInstallDefaultKeepsExistingClotherShim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	binDir := filepath.Join(root, "bin")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLOTHER_BIN", binDir)
	t.Setenv("CLOTHER_SKIP_SELF_UPDATE", "1")

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "clother"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("clother", filepath.Join(binDir, "claude")); err != nil {
		t.Fatal(err)
	}
	realContent := []byte("#!/bin/sh\necho real\n")
	realPath := filepath.Join(binDir, "claude-real")
	if err := os.WriteFile(realPath, realContent, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir)

	paths, err := config.Detect("")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{},
		CustomProviders:   map[string]config.CustomProvider{},
	}
	output := &ui.Output{Stdout: io.Discard, Stderr: io.Discard, Format: ui.FormatHuman}

	code, err := runInstall(context.Background(), Context{
		Paths:   paths,
		Config:  cfg,
		Secrets: config.Secrets{},
		Catalog: catalog,
		Output:  output,
	})
	if err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0", code)
	}

	if !launchers.IsClotherShim(filepath.Join(binDir, "claude")) {
		t.Fatal("expected $BinDir/claude to remain a Clother shim with no flags")
	}
	got, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("expected claude-real to still exist: %v", err)
	}
	if string(got) != string(realContent) {
		t.Fatalf("claude-real content changed: got %q, want %q", got, realContent)
	}
}

// New behavior: --no-claude-shim removes an existing Clother shim and
// restores the preserved real claude back into BinDir/claude, the same
// outcome as `clother uninstall`'s restore step.
func TestRunInstallNoClaudeShimRestoresRealClaude(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	binDir := filepath.Join(root, "bin")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLOTHER_BIN", binDir)
	t.Setenv("CLOTHER_SKIP_SELF_UPDATE", "1")

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "clother"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	if err := os.Symlink("clother", claudePath); err != nil {
		t.Fatal(err)
	}
	realContent := []byte("#!/bin/sh\necho real\n")
	realPath := filepath.Join(binDir, "claude-real")
	if err := os.WriteFile(realPath, realContent, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir)

	paths, err := config.Detect("")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{},
		CustomProviders:   map[string]config.CustomProvider{},
	}
	output := &ui.Output{Stdout: io.Discard, Stderr: io.Discard, Format: ui.FormatHuman}

	code, err := runInstall(context.Background(), Context{
		Paths:   paths,
		Config:  cfg,
		Secrets: config.Secrets{},
		Catalog: catalog,
		Output:  output,
		Options: cli.Options{NoClaudeShim: true},
	})
	if err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0", code)
	}

	info, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatalf("claude should be restored: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("restored claude should be a regular file, not a symlink")
	}
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(realContent) {
		t.Fatalf("restored claude content = %q, want %q", got, realContent)
	}
	if _, err := os.Lstat(realPath); !os.IsNotExist(err) {
		t.Fatalf("claude-real should be gone after restore, stat err = %v", err)
	}
}

// Bug: syncClaudeShim's --no-claude-shim path calls restoreRealClaude, which
// used to only warn on failure instead of returning an error. When the
// restore step cannot actually remove the shim (e.g. a read-only BinDir),
// syncClaudeShim must surface that failure to the caller instead of
// reporting success and silently leaving the shim in place.
//
// This calls the unexported syncClaudeShim helper directly (not runInstall)
// so the test cannot pass vacuously: runInstall also writes into BinDir
// before reaching the shim-restore step (launchers.Sync copies the clother
// binary), so a read-only BinDir could make runInstall fail for an unrelated
// reason and the assertion would pass without ever exercising the restore
// failure this test targets.
//
// syncClaudeShim returns a non-nil error here: the read-only BinDir makes
// os.Remove(shim) inside restoreRealClaude fail, and that failure must
// propagate.
func TestSyncClaudeShimReturnsErrorWhenRestoreFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits do not block removal the same way on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory write permission bits")
	}

	ctx, _ := newConfigTestContext(t, "")
	ctx.Options.NoClaudeShim = true

	binDir := ctx.Paths.BinDir
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	if err := os.Symlink("clother", claudePath); err != nil {
		t.Fatal(err)
	}
	realPath := filepath.Join(binDir, "claude-real")
	if err := os.WriteFile(realPath, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(binDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(binDir, 0o755)
	})

	execPath := filepath.Join(binDir, "clother")
	if err := syncClaudeShim(ctx, execPath, false); err == nil {
		t.Fatal("syncClaudeShim() error = nil, want a non-nil error: the read-only BinDir must make the shim restore fail visibly")
	}
}

// New behavior: an explicit --claude-shim, unlike the warn-and-continue
// default, must fail loudly when no real claude can be found at all, so a
// user who asked for the shim learns immediately instead of getting no shim
// and only a warning.
func TestSyncClaudeShimExplicitFlagErrorsWhenNoRealClaudeFound(t *testing.T) {
	home := t.TempDir()
	emptyPathDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", emptyPathDir)

	var out bytes.Buffer
	ctx := Context{
		Paths:   config.Paths{BinDir: ""},
		Output:  &ui.Output{Stdout: &out, Stderr: &out, Format: ui.FormatHuman},
		Options: cli.Options{ClaudeShim: true},
	}

	if err := syncClaudeShim(ctx, filepath.Join(emptyPathDir, "clother"), false); err == nil {
		t.Fatal("syncClaudeShim() error = nil, want a non-nil error: an explicit --claude-shim with no real claude found must fail, not warn-and-continue")
	}
}

// Companion to TestSyncClaudeShimExplicitFlagErrorsWhenNoRealClaudeFound:
// without an explicit flag, an existing Clother shim in BinDir is kept as
// is, warning rather than erroring, when no real claude can be found.
func TestSyncClaudeShimWithoutFlagWarnsWhenNoRealClaudeFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	binDir := filepath.Join(root, "bin")
	emptyPathDir := filepath.Join(root, "empty-path")
	for _, dir := range []string{home, binDir, emptyPathDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, "clother"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("clother", filepath.Join(binDir, "claude")); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("PATH", emptyPathDir)

	var out bytes.Buffer
	ctx := Context{
		Paths:  config.Paths{BinDir: binDir},
		Output: &ui.Output{Stdout: &out, Stderr: &out, Format: ui.FormatHuman},
	}

	if err := syncClaudeShim(ctx, filepath.Join(binDir, "clother"), false); err != nil {
		t.Fatalf("syncClaudeShim() error = %v, want nil (warn-and-continue without an explicit flag)", err)
	}
}

// Companion to TestRunInstallSkipsShimWhenNoRealClaudeFound: with no flags
// and no claude anywhere on PATH or preserved as claude-real, runInstall
// must not create a claude shim pointing at nothing.
func TestRunInstallDefaultNoClaudeAnywhereSkipsShim(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	binDir := filepath.Join(root, "bin")
	emptyPathDir := filepath.Join(root, "empty-path")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLOTHER_BIN", binDir)
	t.Setenv("CLOTHER_SKIP_SELF_UPDATE", "1")

	if err := os.MkdirAll(emptyPathDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", emptyPathDir)

	paths, err := config.Detect("")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{},
		CustomProviders:   map[string]config.CustomProvider{},
	}
	output := &ui.Output{Stdout: io.Discard, Stderr: io.Discard, Format: ui.FormatHuman}

	code, err := runInstall(context.Background(), Context{
		Paths:   paths,
		Config:  cfg,
		Secrets: config.Secrets{},
		Catalog: catalog,
		Output:  output,
	})
	if err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0", code)
	}

	if _, err := os.Lstat(filepath.Join(binDir, "claude")); !os.IsNotExist(err) {
		t.Fatalf("claude shim should be skipped when no real claude is found, stat err = %v", err)
	}
}
