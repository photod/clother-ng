package launchers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jolehuit/clother/internal/config"
	"github.com/jolehuit/clother/internal/providers"
)

func TestSyncCreatesBinaryAndLaunchers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	execPath := filepath.Join(root, "clother-bin")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{"kimi": "moonshotai/kimi-k2.5"},
		CustomProviders: map[string]config.CustomProvider{
			"myprovider": {
				Name:        "myprovider",
				DisplayName: "myprovider",
				BaseURL:     "https://example.com/anthropic",
				APIKeyEnv:   "MYPROVIDER_API_KEY",
			},
		},
	}
	paths := config.Paths{
		ConfigDir:       filepath.Join(root, "config"),
		DataDir:         filepath.Join(root, "data"),
		CacheDir:        filepath.Join(root, "cache"),
		BinDir:          filepath.Join(root, "bin"),
		ManifestFile:    filepath.Join(root, "data", "launchers.json"),
		SessionPatchDir: filepath.Join(root, "data", "session-patches"),
	}

	if err := Sync(execPath, paths, catalog, cfg, false); err != nil {
		t.Fatal(err)
	}
	// "claude" is deliberately excluded here: Sync must never create, remove or
	// modify $BinDir/claude (see TestSyncCreatesNoClaudeWhenAbsent and friends
	// below): that slot is now owned by launchers.InstallClaudeShim, called by
	// the command layer, not by Sync itself.
	for _, name := range []string{"clother", "clother-zai", "clother-native", "clother-or-kimi", "clother-myprovider", "clother-or", "clother-custom"} {
		if _, err := os.Lstat(filepath.Join(paths.BinDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	// binary must be a regular file (copied), not a symlink
	info, err := os.Lstat(filepath.Join(paths.BinDir, "clother"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("clother should be a regular file in normal mode, not a symlink")
	}
}

func TestSyncHomebrewSkipsCopyAndUsesAbsoluteSymlinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// Simulate the Homebrew-managed binary (not in BinDir)
	homebrewBin := filepath.Join(root, "homebrew", "bin", "clother")
	if err := os.MkdirAll(filepath.Dir(homebrewBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(homebrewBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
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
	paths := config.Paths{
		ConfigDir:       filepath.Join(root, "config"),
		DataDir:         filepath.Join(root, "data"),
		CacheDir:        filepath.Join(root, "cache"),
		BinDir:          filepath.Join(root, "bin"),
		ManifestFile:    filepath.Join(root, "data", "launchers.json"),
		SessionPatchDir: filepath.Join(root, "data", "session-patches"),
	}

	if err := Sync(homebrewBin, paths, catalog, cfg, true); err != nil {
		t.Fatal(err)
	}

	// clother binary must NOT be copied into BinDir
	if _, err := os.Lstat(filepath.Join(paths.BinDir, "clother")); err == nil {
		t.Fatal("clother binary must not be copied into BinDir in Homebrew mode")
	}

	// provider symlinks must exist and point to the Homebrew binary. "claude"
	// is deliberately excluded: Sync must never create, remove or modify
	// $BinDir/claude (see TestSyncCreatesNoClaudeWhenAbsent and friends below).
	for _, name := range []string{"clother-zai", "clother-native", "clother-or", "clother-custom"} {
		link := filepath.Join(paths.BinDir, name)
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("missing symlink %s: %v", name, err)
		}
		if target != homebrewBin {
			t.Fatalf("%s symlink target = %q, want %q", name, target, homebrewBin)
		}
	}
}

func TestSyncHomebrewSkipsDynamicProviderSymlinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homebrewBin := filepath.Join(root, "homebrew", "bin", "clother")
	if err := os.MkdirAll(filepath.Dir(homebrewBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(homebrewBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{"kimi": "moonshotai/kimi-k2.5"},
		CustomProviders: map[string]config.CustomProvider{
			"myprovider": {Name: "myprovider", DisplayName: "myprovider", BaseURL: "https://example.com", APIKeyEnv: "MYPROVIDER_API_KEY"},
		},
	}
	paths := config.Paths{
		ConfigDir:       filepath.Join(root, "config"),
		DataDir:         filepath.Join(root, "data"),
		CacheDir:        filepath.Join(root, "cache"),
		BinDir:          filepath.Join(root, "bin"),
		ManifestFile:    filepath.Join(root, "data", "launchers.json"),
		SessionPatchDir: filepath.Join(root, "data", "session-patches"),
	}

	if err := Sync(homebrewBin, paths, catalog, cfg, true); err != nil {
		t.Fatal(err)
	}

	// individual dynamic symlinks must NOT be created under Homebrew
	for _, name := range []string{"clother-or-kimi", "clother-myprovider"} {
		if _, err := os.Lstat(filepath.Join(paths.BinDir, name)); err == nil {
			t.Fatalf("%s must not be created in Homebrew mode", name)
		}
	}

	// gateway symlinks must always be present
	for _, name := range []string{"clother-or", "clother-custom"} {
		if _, err := os.Lstat(filepath.Join(paths.BinDir, name)); err != nil {
			t.Fatalf("gateway symlink %s must always be created: %v", name, err)
		}
	}
}

func newSyncTestFixture(t *testing.T) (execPath string, paths config.Paths, catalog providers.Catalog, cfg *config.File) {
	t.Helper()
	root := t.TempDir()
	execPath = filepath.Join(root, "clother-bin")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var err error
	catalog, err = providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg = &config.File{
		Version:           1,
		ProviderOverrides: map[string]config.ProviderOverride{},
		OpenRouterAliases: map[string]string{},
		CustomProviders:   map[string]config.CustomProvider{},
	}
	paths = config.Paths{
		ConfigDir:       filepath.Join(root, "config"),
		DataDir:         filepath.Join(root, "data"),
		CacheDir:        filepath.Join(root, "cache"),
		BinDir:          filepath.Join(root, "bin"),
		ManifestFile:    filepath.Join(root, "data", "launchers.json"),
		SessionPatchDir: filepath.Join(root, "data", "session-patches"),
	}
	if err := os.MkdirAll(paths.BinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return execPath, paths, catalog, cfg
}

func assertGatewayLaunchersCreated(t *testing.T, paths config.Paths) {
	t.Helper()
	for _, name := range []string{"clother-or", "clother-custom"} {
		if _, err := os.Lstat(filepath.Join(paths.BinDir, name)); err != nil {
			t.Fatalf("gateway symlink %s must still be created: %v", name, err)
		}
	}
}

// Bug: Sync used to unconditionally remove and recreate $BinDir/claude, which
// deletes a real Claude Code installation living at that path. Sync must
// never create, remove or modify $BinDir/claude.
func TestSyncLeavesRegularFileClaudeUntouched(t *testing.T) {
	t.Parallel()

	execPath, paths, catalog, cfg := newSyncTestFixture(t)
	claudePath := filepath.Join(paths.BinDir, "claude")
	content := []byte("#!/bin/sh\necho real\n")
	if err := os.WriteFile(claudePath, content, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Sync(execPath, paths, catalog, cfg, false); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatalf("claude should still exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("claude should remain a regular file, not become a symlink")
	}
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("claude content changed: got %q, want %q", got, content)
	}
	assertGatewayLaunchersCreated(t, paths)
}

func TestSyncLeavesSymlinkClaudeUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	t.Parallel()

	execPath, paths, catalog, cfg := newSyncTestFixture(t)
	root := filepath.Dir(paths.BinDir)
	versionTarget := filepath.Join(root, "versions", "2.1.3")
	if err := os.MkdirAll(filepath.Dir(versionTarget), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(versionTarget, []byte("#!/bin/sh\necho real\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(paths.BinDir, "claude")
	if err := os.Symlink(versionTarget, claudePath); err != nil {
		t.Fatal(err)
	}

	if err := Sync(execPath, paths, catalog, cfg, false); err != nil {
		t.Fatal(err)
	}

	target, err := os.Readlink(claudePath)
	if err != nil {
		t.Fatalf("claude should still be a symlink: %v", err)
	}
	if target != versionTarget {
		t.Fatalf("claude symlink target changed: got %q, want %q", target, versionTarget)
	}
	assertGatewayLaunchersCreated(t, paths)
}

func TestSyncCreatesNoClaudeWhenAbsent(t *testing.T) {
	t.Parallel()

	execPath, paths, catalog, cfg := newSyncTestFixture(t)
	claudePath := filepath.Join(paths.BinDir, "claude")
	if _, err := os.Lstat(claudePath); !os.IsNotExist(err) {
		t.Fatalf("test fixture should start without claude, stat err = %v", err)
	}

	if err := Sync(execPath, paths, catalog, cfg, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(claudePath); !os.IsNotExist(err) {
		t.Fatalf("Sync must not create claude when it was absent, stat err = %v", err)
	}
	assertGatewayLaunchersCreated(t, paths)
}
