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
		// Guard against a false pass: today neither flag is recognized at all,
		// so Parse already errors, but with "unknown option ...". Once the
		// flags are implemented, the error must be the specific conflict
		// error, not a generic "unknown option" one.
		if strings.Contains(err.Error(), "unknown option") {
			t.Fatalf("Parse(%v) error = %v, want a claude-shim conflict error, not unknown option", args, err)
		}
	}
}
