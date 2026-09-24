package cli

import (
	"strings"
	"testing"
)

func TestParseClaudeShimFlag(t *testing.T) {
	parsed, err := Parse([]string{"install", "--claude-shim"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !parsed.Options.ClaudeShim {
		t.Fatal("expected Options.ClaudeShim = true")
	}
	if parsed.Options.NoClaudeShim {
		t.Fatal("expected Options.NoClaudeShim = false")
	}
	if parsed.Command != "install" {
		t.Fatalf("Command = %q, want %q", parsed.Command, "install")
	}
}

func TestParseNoClaudeShimFlag(t *testing.T) {
	parsed, err := Parse([]string{"install", "--no-claude-shim"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !parsed.Options.NoClaudeShim {
		t.Fatal("expected Options.NoClaudeShim = true")
	}
	if parsed.Options.ClaudeShim {
		t.Fatal("expected Options.ClaudeShim = false")
	}
	if parsed.Command != "install" {
		t.Fatalf("Command = %q, want %q", parsed.Command, "install")
	}
}

// --claude-shim and --no-claude-shim are global options, like --yes: they
// must parse before the command as well as after it.
func TestParseClaudeShimFlagBeforeCommand(t *testing.T) {
	parsed, err := Parse([]string{"--claude-shim", "install"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !parsed.Options.ClaudeShim {
		t.Fatal("expected Options.ClaudeShim = true")
	}
	if parsed.Command != "install" {
		t.Fatalf("Command = %q, want %q", parsed.Command, "install")
	}
}

// --claude-shim and --no-claude-shim only make sense for `install`: it is
// the only command that manages the claude shim. Using either flag with any
// other command must be rejected, with an error that names "install" so the
// user knows why.
func TestParseClaudeShimFlagsRejectedForNonInstallCommand(t *testing.T) {
	cases := [][]string{
		{"list", "--claude-shim"},
		{"--no-claude-shim", "config"},
	}
	for _, args := range cases {
		_, err := Parse(args)
		if err == nil {
			t.Fatalf("Parse(%v) error = nil, want a non-nil error", args)
		}
		if !strings.Contains(err.Error(), "install") {
			t.Fatalf("Parse(%v) error = %v, want it to mention \"install\"", args, err)
		}
	}
}

// Companion to TestParseClaudeShimFlagsRejectedForNonInstallCommand: the
// flags must still be accepted when the command is "install".
func TestParseClaudeShimFlagAcceptedForInstallCommand(t *testing.T) {
	parsed, err := Parse([]string{"install", "--claude-shim"})
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if !parsed.Options.ClaudeShim {
		t.Fatal("expected Options.ClaudeShim = true")
	}
	if parsed.Command != "install" {
		t.Fatalf("Command = %q, want %q", parsed.Command, "install")
	}
}

func TestParseBothClaudeShimFlagsIsError(t *testing.T) {
	cases := [][]string{
		{"install", "--claude-shim", "--no-claude-shim"},
		{"install", "--no-claude-shim", "--claude-shim"},
	}
	for _, args := range cases {
		_, err := Parse(args)
		if err == nil {
			t.Fatalf("Parse(%v) error = nil, want non-nil", args)
		}
		// Guard against a false pass: both flags are recognized options, so
		// the mutual-exclusion error must be the specific conflict error, not
		// the generic "unknown option" error Parse would return if either
		// flag were unrecognized.
		if strings.Contains(err.Error(), "unknown option") {
			t.Fatalf("Parse(%v) error = %v, want a claude-shim conflict error, not unknown option", args, err)
		}
	}
}
