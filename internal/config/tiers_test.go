package config

import (
	"path/filepath"
	"testing"

	"github.com/jolehuit/clother/internal/providers"
)

// Issue: TierModels grew FableModel and SubagentModel, but Map() and
// trimmed() still only know about opus/sonnet/haiku, so the fable alias and
// CLAUDE_CODE_SUBAGENT_MODEL can never be wired through a config file.

func TestTierModelsMapIncludesFableAndSubagent(t *testing.T) {
	t.Parallel()

	tm := TierModels{
		OpusModel:     "opus-x",
		SonnetModel:   "sonnet-x",
		HaikuModel:    "haiku-x",
		FableModel:    "fable-x",
		SubagentModel: "subagent-x",
	}
	got := tm.Map()
	want := map[string]string{
		providers.TierOpus:     "opus-x",
		providers.TierSonnet:   "sonnet-x",
		providers.TierHaiku:    "haiku-x",
		providers.TierFable:    "fable-x",
		providers.TierSubagent: "subagent-x",
	}
	if len(got) != len(want) {
		t.Fatalf("Map() = %+v, want %+v", got, want)
	}
	for tier, model := range want {
		if got[tier] != model {
			t.Fatalf("Map()[%q] = %q, want %q (full map %+v)", tier, got[tier], model, got)
		}
	}
}

func TestTierModelsMapOmitsWhitespaceOnlyFableAndSubagent(t *testing.T) {
	t.Parallel()

	tm := TierModels{FableModel: "   ", SubagentModel: "\t\t"}
	got := tm.Map()
	if _, ok := got[providers.TierFable]; ok {
		t.Fatalf("Map() includes a whitespace-only fable model: %+v", got)
	}
	if _, ok := got[providers.TierSubagent]; ok {
		t.Fatalf("Map() includes a whitespace-only subagent model: %+v", got)
	}
}

func TestTierModelsTrimmedTrimsFableAndSubagent(t *testing.T) {
	t.Parallel()

	tm := TierModels{
		OpusModel:     " opus ",
		SonnetModel:   " sonnet ",
		HaikuModel:    " haiku ",
		FableModel:    " fable ",
		SubagentModel: " subagent ",
	}
	got := tm.trimmed()
	want := TierModels{
		OpusModel:     "opus",
		SonnetModel:   "sonnet",
		HaikuModel:    "haiku",
		FableModel:    "fable",
		SubagentModel: "subagent",
	}
	if got != want {
		t.Fatalf("trimmed() = %+v, want %+v", got, want)
	}
}

// TestConfigRoundTripsAndTrimsFableAndSubagentModel saves a config with
// padded fable_model/subagent_model, reloads it from disk the way the app
// does (LoadConfig followed by Normalize), and checks the values survive the
// round trip and get whitespace-trimmed like the other tier fields.
func TestConfigRoundTripsAndTrimsFableAndSubagentModel(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &File{
		Version: 1,
		ProviderOverrides: map[string]ProviderOverride{
			"zai": {
				Model: "glm-5.3",
				TierModels: TierModels{
					OpusModel:     " glm-5.3 ",
					FableModel:    " glm-5.3 ",
					SubagentModel: " glm-5.3-flash ",
				},
			},
		},
		OpenRouterAliases: map[string]string{},
		CustomProviders:   map[string]CustomProvider{},
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Normalize(catalog)

	override := loaded.ProviderOverrides["zai"]
	if override.FableModel != "glm-5.3" {
		t.Fatalf("FableModel after round trip = %q, want trimmed glm-5.3", override.FableModel)
	}
	if override.SubagentModel != "glm-5.3-flash" {
		t.Fatalf("SubagentModel after round trip = %q, want trimmed glm-5.3-flash", override.SubagentModel)
	}
}
