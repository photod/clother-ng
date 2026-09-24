package commands

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jolehuit/clother/internal/config"
	"github.com/jolehuit/clother/internal/profiles"
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

// tierPromptDefault extracts the bracketed default value printed for a tier
// prompt label (e.g. "Opus model"), tolerating model IDs that themselves
// contain brackets (e.g. "glm-5.3-flash[1m]" or "k3[1m]"): the closing
// "]: " that ends the prompt is found by scanning forward from the label,
// which lands on the outermost bracket since a bracket inside a model ID is
// never itself followed by ": ".
func tierPromptDefault(t *testing.T, out, label string) string {
	t.Helper()
	marker := label + " ["
	idx := strings.Index(out, marker)
	if idx < 0 {
		if strings.Contains(out, label+": ") {
			return ""
		}
		t.Fatalf("prompt %q not found in output:\n%s", label, out)
	}
	rest := out[idx+len(marker):]
	end := strings.Index(rest, "]: ")
	if end < 0 {
		t.Fatalf("prompt %q default not terminated (no \"]: \") in output:\n%s", label, out)
	}
	return rest[:end]
}

// Tier prompt defaults must equal what launch actually produces.
//
// kimi is a catalog provider whose model_tiers include a "subagent" entry
// equal to its default model (k3-256k). Pinning the launch model to a
// different model_choice ("k3[1m]") makes profiles.Resolve drop the catalog
// subagent mapping entirely (override.Model replaces the whole tier map with
// opus/sonnet/haiku/fable = that model; see internal/profiles/resolve.go),
// so EffectiveTiers has no subagent entry once launched. The Subagent tier
// prompt must default to that same "no mapping" state, not to the catalog's
// now-irrelevant k3-256k.
//
// baselineTiers resolves the pinned model through profiles.Resolve before
// reading EffectiveTiers, so it picks up this same drop of the catalog
// subagent mapping, and the Subagent prompt never shows the now-irrelevant
// "[k3-256k]" as its default.
func TestConfigBuiltinSubagentPromptDefaultDropsWhenModelIsPinned(t *testing.T) {
	input := strings.Join([]string{"", "2", "y", "", "", "", "", ""}, "\n") + "\n"
	ctx, cfg, stdout := newTiersTestContext(t, input)

	provider, ok := ctx.Catalog.Get("kimi")
	if !ok {
		t.Fatal("kimi provider missing from catalog")
	}
	catalogSubagent := provider.ModelTiers[providers.TierSubagent]
	if catalogSubagent == "" {
		t.Fatal("kimi provider has no catalog subagent tier; pick another provider with one")
	}

	if _, err := configBuiltin(ctx, provider); err != nil {
		t.Fatal(err)
	}

	override, exists := cfg.ProviderOverrides["kimi"]
	if !exists {
		t.Fatal("expected a stored kimi override")
	}
	if override.Model != "k3[1m]" {
		t.Fatalf("override.Model = %q, want k3[1m] (the pinned model choice)", override.Model)
	}
	if override.SubagentModel != "" {
		t.Fatalf("override.SubagentModel = %q, want empty (Enter must not pin the catalog subagent model)", override.SubagentModel)
	}

	out := stdout.String()
	if strings.Contains(out, "Subagent model ["+catalogSubagent+"]") {
		t.Fatalf("Subagent prompt default still shows the catalog subagent model %q even though the launch model is pinned to %q, got:\n%s",
			catalogSubagent, override.Model, out)
	}

	target, err := profiles.Resolve("kimi", ctx.Catalog, cfg)
	if err != nil {
		t.Fatal(err)
	}
	effective := profiles.EffectiveTiers(target)
	if got := effective[providers.TierSubagent]; got != "" {
		t.Fatalf("EffectiveTiers(Resolve(\"kimi\", ...))[subagent] = %q, want empty or absent: the prompt default must match what launch resolves to", got)
	}
}

// General case: with no pin at all, the bracketed defaults printed for
// Opus, Sonnet, Haiku and Fable must equal what launch resolves those tiers
// to. zai's catalog has no "subagent" entry, so this test does not cover the
// subagent-drop case above; it only checks that the non-subagent tiers stay
// correct.
func TestConfigBuiltinPerTierPromptDefaultsMatchEffectiveTiersWithoutPin(t *testing.T) {
	input := strings.Join([]string{"", "", "y", "", "", "", "", ""}, "\n") + "\n"
	ctx, _, stdout := newTiersTestContext(t, input)

	provider, ok := ctx.Catalog.Get("zai")
	if !ok {
		t.Fatal("zai provider missing from catalog")
	}
	if _, err := configBuiltin(ctx, provider); err != nil {
		t.Fatal(err)
	}

	cfgWithoutOverride := &config.File{ProviderOverrides: map[string]config.ProviderOverride{}}
	target, err := profiles.Resolve("zai", ctx.Catalog, cfgWithoutOverride)
	if err != nil {
		t.Fatal(err)
	}
	effective := profiles.EffectiveTiers(target)

	out := stdout.String()
	for _, tier := range []struct {
		name  string
		label string
	}{
		{providers.TierOpus, "Opus model"},
		{providers.TierSonnet, "Sonnet model"},
		{providers.TierHaiku, "Haiku model"},
		{providers.TierFable, "Fable model"},
	} {
		want := effective[tier.name]
		got := tierPromptDefault(t, out, tier.label)
		if got != want {
			t.Fatalf("printed default for %q = %q, want %q (EffectiveTiers(Resolve(\"zai\", ...))[%s])", tier.label, got, want, tier.name)
		}
	}
}

// Stale tier pins must be reset by the tier prompts, not kept: a value
// already pinned to a tier (here haiku -> "glm-4.7") that the catalog no
// longer offers should not be re-stored just because Enter defaulted to it,
// and the prompt output should call out that the pin is stale.
//
// promptCatalogTiers checks each pinned tier value against
// profiles.ProviderStalePins: a stale value is shown but not offered as the
// default, so Enter resets it, and the prompt output names it as stale.
func TestConfigBuiltinPerTierPromptsResetsStaleTierPin(t *testing.T) {
	input := strings.Join([]string{"", "", "", "", "", "", "", ""}, "\n") + "\n"
	preset := config.ProviderOverride{TierModels: config.TierModels{HaikuModel: "glm-4.7"}}
	override, _, out := runZaiTierScenario(t, input, preset, true)

	if override.HaikuModel != "" {
		t.Fatalf("HaikuModel = %q, want the stale pin reset to empty", override.HaikuModel)
	}
	if !strings.Contains(out, "glm-4.7") {
		t.Fatalf("stdout does not mention the stale pin glm-4.7, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "stale") {
		t.Fatalf("stdout does not mention that the pin is stale, got:\n%s", out)
	}
}

// The "Map tiers separately?" confirm prompt must warn that declining it
// clears any tiers already pinned explicitly, when such tiers exist.
//
// promptCatalogTiers swaps in a label that says so whenever the override
// already has an explicit tier mapping, instead of always printing the fixed
// "Map tiers separately? (Opus, Sonnet, Haiku, Fable, Subagent)" string.
func TestConfigBuiltinTierConfirmMentionsClearingWhenTiersExist(t *testing.T) {
	input := strings.Join([]string{"", "", "n"}, "\n") + "\n"
	preset := config.ProviderOverride{TierModels: config.TierModels{OpusModel: "glm-5.3"}}
	_, _, out := runZaiTierScenario(t, input, preset, true)

	if !strings.Contains(out, "clears") {
		t.Fatalf("stdout does not mention that declining clears the existing tier mappings, got:\n%s", out)
	}
}

// The "Map tiers separately?" confirm label must name which tier is already
// pinned and to what (e.g. "opus=glm-5.3"), printed before the per-tier
// prompts that follow it.
func TestConfigBuiltinTierConfirmLabelNamesPinnedTiers(t *testing.T) {
	input := strings.Join([]string{"", "", "y", "", "", "", "", ""}, "\n") + "\n"
	preset := config.ProviderOverride{TierModels: config.TierModels{OpusModel: "glm-5.3"}}
	_, _, out := runZaiTierScenario(t, input, preset, true)

	tierPromptIdx := strings.Index(out, "Opus model [")
	if tierPromptIdx < 0 {
		t.Fatalf("stdout does not contain the Opus model tier prompt, got:\n%s", out)
	}
	pinnedIdx := strings.Index(out, "opus=glm-5.3")
	if pinnedIdx < 0 {
		t.Fatalf("stdout does not name the pinned tier opus=glm-5.3, got:\n%s", out)
	}
	if pinnedIdx > tierPromptIdx {
		t.Fatalf("\"opus=glm-5.3\" appears after the first tier prompt, want it named before it, got:\n%s", out)
	}
}
