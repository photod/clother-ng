package commands

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jolehuit/clother/internal/config"
	"github.com/jolehuit/clother/internal/providers"
	"github.com/jolehuit/clother/internal/ui"
)

// newTiersTestContext builds a Context wired to the real catalog, with the
// zai API key pre-set so its key prompt takes an empty line (as
// TestConfigZai* in stale_test.go do), and a bytes.Buffer stdout so prompts
// and messages can be inspected.
func newTiersTestContext(t *testing.T, input string) (Context, *config.File, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, ".cache"))
	t.Setenv("CLOTHER_BIN", filepath.Join(root, "bin"))

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
	// The prompter writes every prompt label (including the per-tier ones) to
	// its own Out, not to c.Output.Stdout: both point at the same buffer here
	// so TestConfigBuiltinPerTierPromptsStdoutMentionsAllTiers can see them,
	// matching how NewPrompter wires both to os.Stdout in production.
	var stdout bytes.Buffer
	ctx := Context{
		Paths:   paths,
		Config:  cfg,
		Secrets: config.Secrets{"ZAI_API_KEY": "sk-test"},
		Catalog: catalog,
		Output:  &ui.Output{Stdout: &stdout, Stderr: io.Discard, Format: ui.FormatHuman},
		Prompt:  &ui.Prompter{In: strings.NewReader(input), Out: &stdout},
	}
	return ctx, cfg, &stdout
}

// runZaiTierScenario runs configBuiltin against the real zai provider with
// the given input and returns the resulting override plus everything
// printed to stdout.
func runZaiTierScenario(t *testing.T, input string, preset config.ProviderOverride, presetExists bool) (config.ProviderOverride, bool, string) {
	t.Helper()
	ctx, cfg, stdout := newTiersTestContext(t, input)
	if presetExists {
		cfg.ProviderOverrides["zai"] = preset
	}
	provider, ok := ctx.Catalog.Get("zai")
	if !ok {
		t.Fatal("zai provider missing from catalog")
	}
	if _, err := configBuiltin(ctx, provider); err != nil {
		t.Fatal(err)
	}
	override, exists := cfg.ProviderOverrides["zai"]
	return override, exists, stdout.String()
}

// Scenario (a): confirming "y" maps every tier; only answers that differ from
// the no-mapping default (the chosen model, or the catalog tier) are stored.
func TestConfigBuiltinPerTierPromptsStoreExplicitOverridesOnly(t *testing.T) {
	input := strings.Join([]string{"", "", "y", "2", "", "4", "", ""}, "\n") + "\n"
	override, _, _ := runZaiTierScenario(t, input, config.ProviderOverride{}, false)

	want := config.ProviderOverride{
		TierModels: config.TierModels{
			OpusModel:  "glm-5.3",
			HaikuModel: "glm-5.3-flash",
		},
	}
	if override != want {
		t.Fatalf("override = %+v, want %+v", override, want)
	}
}

// Scenario (f): the tier prompts label each tier by name.
func TestConfigBuiltinPerTierPromptsStdoutMentionsAllTiers(t *testing.T) {
	input := strings.Join([]string{"", "", "y", "2", "", "4", "", ""}, "\n") + "\n"
	_, _, out := runZaiTierScenario(t, input, config.ProviderOverride{}, false)

	for _, want := range []string{"Opus", "Sonnet", "Haiku", "Fable", "Subagent"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout does not mention %q, got:\n%s", want, out)
		}
	}
}

// Scenario (b): declining the "Map tiers separately?" question leaves no
// override at all when there was none to begin with.
func TestConfigBuiltinDecliningTierMapLeavesNoOverride(t *testing.T) {
	input := strings.Join([]string{"", "", "n"}, "\n") + "\n"
	override, exists, _ := runZaiTierScenario(t, input, config.ProviderOverride{}, false)

	if exists && override != (config.ProviderOverride{}) {
		t.Fatalf("override = %+v, want none (or the zero value)", override)
	}
}

// Scenario (c): declining the question clears any tiers that were already
// pinned explicitly.
func TestConfigBuiltinDecliningTierMapClearsExistingExplicitTiers(t *testing.T) {
	input := strings.Join([]string{"", "", "n"}, "\n") + "\n"
	preset := config.ProviderOverride{TierModels: config.TierModels{OpusModel: "glm-5.3"}}
	override, _, _ := runZaiTierScenario(t, input, preset, true)

	if override.OpusModel != "" {
		t.Fatalf("OpusModel = %q, want cleared", override.OpusModel)
	}
}

// Scenario (d): with explicit tiers already pinned, Enter at the confirm
// question defaults to yes, and "-" clears a single tier.
func TestConfigBuiltinConfirmDefaultsYesWhenTiersExistAndDashClears(t *testing.T) {
	input := strings.Join([]string{"", "", "", "-", "", "", "", ""}, "\n") + "\n"
	preset := config.ProviderOverride{TierModels: config.TierModels{OpusModel: "glm-5.3"}}
	override, _, _ := runZaiTierScenario(t, input, preset, true)

	if override.OpusModel != "" {
		t.Fatalf("OpusModel = %q, want cleared via \"-\"", override.OpusModel)
	}
}

// The Enter default at the "Map tiers separately?" question is yes when the
// override already has an explicit tier: an explicit answer ("4") to the
// first tier prompt that follows must be read as the opus tier, not
// discarded because the confirm defaulted to no.
func TestConfigBuiltinConfirmEnterDefaultsToYesWithExistingTiers(t *testing.T) {
	input := strings.Join([]string{"", "", "", "4", "", "", "", ""}, "\n") + "\n"
	preset := config.ProviderOverride{TierModels: config.TierModels{OpusModel: "glm-5.3"}}
	override, _, _ := runZaiTierScenario(t, input, preset, true)

	if override.OpusModel != "glm-5.3-flash" {
		t.Fatalf("OpusModel = %q, want glm-5.3-flash (the confirm must default to yes)", override.OpusModel)
	}
}

// The Enter default at the "Map tiers separately?" question is no without an
// existing explicit tier: "4" must never reach the opus prompt, since the
// tier-mapping block is skipped entirely.
func TestConfigBuiltinConfirmEnterDefaultsToNoWithoutExistingTiers(t *testing.T) {
	input := strings.Join([]string{"", "", "", "4"}, "\n") + "\n"
	override, _, _ := runZaiTierScenario(t, input, config.ProviderOverride{}, false)

	if override.OpusModel != "" {
		t.Fatalf("OpusModel = %q, want empty (the confirm must default to no and skip the tier prompts)", override.OpusModel)
	}
}

// Scenario (e): the subagent tier has an empty no-mapping default, so any
// explicit value is stored.
func TestConfigBuiltinSubagentPromptStoresExplicitValue(t *testing.T) {
	input := strings.Join([]string{"", "", "y", "", "", "", "", "glm-5.3-flash"}, "\n") + "\n"
	override, _, _ := runZaiTierScenario(t, input, config.ProviderOverride{}, false)

	if override.SubagentModel != "glm-5.3-flash" {
		t.Fatalf("SubagentModel = %q, want glm-5.3-flash", override.SubagentModel)
	}
}
