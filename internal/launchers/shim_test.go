package launchers

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jolehuit/clother/internal/config"
)

func TestSymlinkTarget(t *testing.T) {
	t.Parallel()

	if got := SymlinkTarget("/opt/homebrew/bin/clother", false); got != "clother" {
		t.Fatalf("SymlinkTarget(skipCopy=false) = %q, want %q", got, "clother")
	}
	if got := SymlinkTarget("/opt/homebrew/bin/clother", true); got != "/opt/homebrew/bin/clother" {
		t.Fatalf("SymlinkTarget(skipCopy=true) = %q, want %q", got, "/opt/homebrew/bin/clother")
	}
}

func TestIsClotherShim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	t.Parallel()

	dir := t.TempDir()

	cases := []struct {
		name  string
		setup func(t *testing.T) string
		want  bool
	}{
		{
			name: "relative link text clother",
			setup: func(t *testing.T) string {
				path := filepath.Join(dir, "relative-clother")
				if err := os.Symlink("clother", path); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: true,
		},
		{
			name: "absolute homebrew bin clother",
			setup: func(t *testing.T) string {
				path := filepath.Join(dir, "homebrew-bin-clother")
				if err := os.Symlink("/opt/homebrew/bin/clother", path); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: true,
		},
		{
			name: "absolute homebrew cellar clother",
			setup: func(t *testing.T) string {
				path := filepath.Join(dir, "homebrew-cellar-clother")
				if err := os.Symlink("/opt/homebrew/Cellar/clother/3.0.11/bin/clother", path); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: true,
		},
		{
			name: "claude versions dir is not a shim",
			setup: func(t *testing.T) string {
				path := filepath.Join(dir, "versions-2-1-3")
				if err := os.Symlink("/home/u/.local/share/claude/versions/2.1.3", path); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: false,
		},
		{
			name: "regular file is not a shim",
			setup: func(t *testing.T) string {
				path := filepath.Join(dir, "regular-file")
				if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: false,
		},
		{
			name: "missing path is not a shim",
			setup: func(t *testing.T) string {
				return filepath.Join(dir, "does-not-exist")
			},
			want: false,
		},
		{
			name: "symlink to clother-zai is not a shim",
			setup: func(t *testing.T) string {
				path := filepath.Join(dir, "clother-zai-link")
				if err := os.Symlink("clother-zai", path); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := tc.setup(t)
			if got := IsClotherShim(path); got != tc.want {
				t.Fatalf("IsClotherShim(%q) = %v, want %v", path, got, tc.want)
			}
		})
	}
}

func TestInstallClaudeShimCreatesWhenAbsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	t.Parallel()

	binDir := t.TempDir()
	paths := config.Paths{BinDir: binDir}
	claudePath := filepath.Join(binDir, "claude")

	if err := InstallClaudeShim(paths, "clother"); err != nil {
		t.Fatalf("InstallClaudeShim() error = %v", err)
	}

	target, err := os.Readlink(claudePath)
	if err != nil {
		t.Fatalf("claude should be a symlink: %v", err)
	}
	if target != "clother" {
		t.Fatalf("claude symlink target = %q, want %q", target, "clother")
	}
}

func TestInstallClaudeShimRefreshesExistingShim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	t.Parallel()

	binDir := t.TempDir()
	paths := config.Paths{BinDir: binDir}
	claudePath := filepath.Join(binDir, "claude")
	if err := os.Symlink("clother", claudePath); err != nil {
		t.Fatal(err)
	}

	newTarget := "/abs/path/clother"
	if err := InstallClaudeShim(paths, newTarget); err != nil {
		t.Fatalf("InstallClaudeShim() error = %v", err)
	}

	target, err := os.Readlink(claudePath)
	if err != nil {
		t.Fatalf("claude should still be a symlink: %v", err)
	}
	if target != newTarget {
		t.Fatalf("claude symlink target = %q, want %q", target, newTarget)
	}
}

func TestInstallClaudeShimLeavesRegularFileUntouched(t *testing.T) {
	t.Parallel()

	binDir := t.TempDir()
	paths := config.Paths{BinDir: binDir}
	claudePath := filepath.Join(binDir, "claude")
	content := []byte("#!/bin/sh\necho real\n")
	if err := os.WriteFile(claudePath, content, 0o755); err != nil {
		t.Fatal(err)
	}

	err := InstallClaudeShim(paths, "clother")
	if !errors.Is(err, ErrClaudeNotShim) {
		t.Fatalf("InstallClaudeShim() error = %v, want ErrClaudeNotShim", err)
	}

	info, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatal(err)
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
}

func TestInstallClaudeShimLeavesForeignSymlinkUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	t.Parallel()

	binDir := t.TempDir()
	paths := config.Paths{BinDir: binDir}
	claudePath := filepath.Join(binDir, "claude")
	foreignTarget := "/home/u/.local/share/claude/versions/2.1.3"
	if err := os.Symlink(foreignTarget, claudePath); err != nil {
		t.Fatal(err)
	}

	err := InstallClaudeShim(paths, "clother")
	if !errors.Is(err, ErrClaudeNotShim) {
		t.Fatalf("InstallClaudeShim() error = %v, want ErrClaudeNotShim", err)
	}

	target, err := os.Readlink(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if target != foreignTarget {
		t.Fatalf("foreign symlink target changed: got %q, want %q", target, foreignTarget)
	}
}
