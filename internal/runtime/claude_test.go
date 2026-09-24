package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jolehuit/clother/internal/config"
)

func TestFindRealClaudeCanUseSameBinDir(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "clother-bin")
	realDir := filepath.Join(root, "real-bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	realClaude := filepath.Join(realDir, "claude")
	if err := os.WriteFile(realClaude, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+realDir); err != nil {
		t.Fatal(err)
	}

	got, err := FindRealClaude(config.Paths{BinDir: binDir})
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(binDir, "claude") {
		t.Fatalf("FindRealClaude() = %q, want %q", got, filepath.Join(binDir, "claude"))
	}
}

func TestFindRealClaudeSkipsSelfAndFallsBack(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(self, filepath.Join(binDir, "claude")); err != nil {
		t.Fatal(err)
	}
	realFallback := filepath.Join(binDir, "claude-real")
	if err := os.WriteFile(realFallback, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })
	if err := os.Setenv("PATH", binDir); err != nil {
		t.Fatal(err)
	}

	got, err := FindRealClaude(config.Paths{BinDir: binDir})
	if err != nil {
		t.Fatal(err)
	}
	if got != realFallback {
		t.Fatalf("FindRealClaude() = %q, want %q", got, realFallback)
	}
}

func TestPreserveRealClaudeMovesClaudeToClaudeReal(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(binDir, "claude")
	content := []byte("real-claude-binary")
	if err := os.WriteFile(claudePath, content, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := PreserveRealClaude(config.Paths{BinDir: binDir}, claudePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(claudePath); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be moved, stat err=%v", claudePath, err)
	}
	preserved := filepath.Join(binDir, "claude-real")
	got, err := os.ReadFile(preserved)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("preserved content mismatch: got %q", string(got))
	}
}

// PR #29: Claude Code's auto-updater prunes the version claude-real points to.
func TestFindRealClaudeRecoversFromDanglingClaudeReal(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(self, filepath.Join(binDir, "claude")); err != nil {
		t.Fatal(err)
	}
	versions := filepath.Join(root, ".local", "share", "claude", "versions")
	if err := os.MkdirAll(versions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(versions, "2.1.9"), filepath.Join(binDir, "claude-real")); err != nil {
		t.Fatal(err)
	}
	for name, mode := range map[string]os.FileMode{
		"2.1.10":   0o755, // numerically newest: 10 > 9
		"2.1.9.1":  0o755,
		"2.2.0":    0o644, // not executable
		"nightly":  0o755, // not a version
		"2.1.2":    0o755,
		"2.1.10.x": 0o755,
	} {
		if err := os.WriteFile(filepath.Join(versions, name), []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)

	got, err := FindRealClaude(config.Paths{BinDir: binDir})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(versions, "2.1.10"); got != want {
		t.Fatalf("FindRealClaude() = %q, want %q", got, want)
	}
}

// Bug: FindRealClaude only skipped a PATH candidate named "claude" when it
// resolved to the CURRENTLY RUNNING test executable. It must skip any
// candidate that resolves to a Clother shim (a symlink whose target's base
// name is "clother"), even when that shim is not the running binary -- e.g.
// when `clother install` runs from a freshly downloaded binary and an old
// shim is still sitting on PATH ahead of the real claude.
func TestFindRealClaudeSkipsShimOnPathEvenWhenNotSelf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	realDir := filepath.Join(root, "realdir")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "clother"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("clother", filepath.Join(binDir, "claude")); err != nil {
		t.Fatal(err)
	}
	realClaude := filepath.Join(realDir, "claude")
	if err := os.WriteFile(realClaude, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+realDir)
	t.Setenv("CLOTHER_BIN", binDir)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	paths, err := config.Detect("")
	if err != nil {
		t.Fatal(err)
	}

	got, err := FindRealClaude(paths)
	if err != nil {
		t.Fatalf("FindRealClaude() error = %v", err)
	}
	if got != realClaude {
		t.Fatalf("FindRealClaude() = %q, want %q (a Clother shim earlier on PATH must be skipped)", got, realClaude)
	}
}

// Companion to the PATH case above: the $BinDir/claude-real fallback must
// also be skipped when it resolves to a Clother shim, not just to self.
func TestFindRealClaudeSkipsShimFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "clother"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("clother", filepath.Join(binDir, "claude")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("clother", filepath.Join(binDir, "claude-real")); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir)
	t.Setenv("CLOTHER_BIN", binDir)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	// HOME has no .local/share/claude/versions, so there is nothing for the
	// newestInstalledClaude recovery path to find either.
	paths, err := config.Detect("")
	if err != nil {
		t.Fatal(err)
	}

	got, err := FindRealClaude(paths)
	if err == nil {
		t.Fatalf("FindRealClaude() = %q, want error: both claude and claude-real are Clother shims", got)
	}
}

func TestFindRealClaudeWithoutClaudeRealStillFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	versions := filepath.Join(root, ".local", "share", "claude", "versions")
	if err := os.MkdirAll(versions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versions, "2.1.10"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(root, "empty"))

	// Without a claude-real link there is no sign Clother installed over the
	// native installer, so the versions dir is not guessed at.
	if _, err := FindRealClaude(config.Paths{BinDir: filepath.Join(root, "bin")}); err == nil {
		t.Fatal("expected an error when neither PATH nor claude-real has claude")
	}
}

// Bug: PreserveRealClaude assumed $BinDir/claude, whenever it equals the
// resolved "real claude" path, is an actual Claude Code binary worth saving.
// When that slot already holds a Clother shim (an install/upgrade running
// against a stale shim left on PATH), PreserveRealClaude must leave it
// alone: it must not rename the shim over an existing claude-real, which
// would destroy the real preserved binary.
func TestPreserveRealClaudeLeavesShimAndClaudeRealUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
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
	realPath := filepath.Join(binDir, "claude-real")
	if err := os.WriteFile(realPath, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := PreserveRealClaude(config.Paths{BinDir: binDir}, claudePath); err != nil {
		t.Fatalf("PreserveRealClaude() error = %v", err)
	}

	info, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatalf("claude should still exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("claude should remain a symlink (the Clother shim), not be renamed away")
	}
	target, err := os.Readlink(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if target != "clother" {
		t.Fatalf("claude symlink target = %q, want %q", target, "clother")
	}

	realInfo, err := os.Lstat(realPath)
	if err != nil {
		t.Fatalf("claude-real should still exist: %v", err)
	}
	if realInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("claude-real should remain a regular file, not become a symlink to the shim")
	}
	got, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "real" {
		t.Fatalf("claude-real content = %q, want %q", got, "real")
	}
}
