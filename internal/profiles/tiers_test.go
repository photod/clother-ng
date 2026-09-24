package profiles

import (
	"testing"

	"github.com/jolehuit/clother/internal/config"
	"github.com/jolehuit/clother/internal/providers"
)

// Issue: fable and subagent tier overrides are silently ignored by Resolve
// and StalePins, even though config.TierModels carries FableModel and
// SubagentModel.

func TestResolveFableOverrideAppliesOnlyFableTier(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := catalog.Get("zai")
	if !ok {
		t.Fatal("zai provider missing from catalog")
	}

	cfg := newTestConfig()
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{
		TierModels: config.TierModels{FableModel: "glm-5.3"},
	}

	target, err := Resolve("zai", catalog, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target.ModelTiers[providers.TierFable] != "glm-5.3" {
		t.Fatalf("fable tier = %q, want glm-5.3", target.ModelTiers[providers.TierFable])
	}
	for tier, want := range provider.ModelTiers {
		if tier == providers.TierFable {
			continue
		}
		if target.ModelTiers[tier] != want {
			t.Fatalf("tier %s = %q, want the untouched catalog value %q", tier, target.ModelTiers[tier], want)
		}
	}
}

func TestResolveSubagentOverrideSetsEffectiveTier(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{
		TierModels: config.TierModels{SubagentModel: "glm-5.3-flash"},
	}

	target, err := Resolve("zai", catalog, cfg)
	if err != nil {
		t.Fatal(err)
	}
	tiers := EffectiveTiers(target)
	if tiers[providers.TierSubagent] != "glm-5.3-flash" {
		t.Fatalf("EffectiveTiers()[subagent] = %q, want glm-5.3-flash (full tiers %+v)", tiers[providers.TierSubagent], tiers)
	}
}

func TestStalePinsFlagsStaleFableTier(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{
		TierModels: config.TierModels{FableModel: "glm-4.7"},
	}

	pins := StalePins("zai", catalog, cfg)
	if len(pins) != 1 {
		t.Fatalf("StalePins() = %+v, want exactly one stale pin", pins)
	}
	if pins[0].Field != providers.TierFable || pins[0].Model != "glm-4.7" {
		t.Fatalf("StalePins()[0] = %+v, want {fable glm-4.7}", pins[0])
	}
}

func TestStalePinsOrdersModelOpusSonnetHaikuFableSubagent(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{
		Model: "glm-5.1",
		TierModels: config.TierModels{
			OpusModel:     "glm-4.7",
			SonnetModel:   "glm-4.7",
			HaikuModel:    "glm-4.7",
			FableModel:    "glm-4.7",
			SubagentModel: "glm-4.7",
		},
	}

	pins := StalePins("zai", catalog, cfg)
	want := []StalePin{
		{Field: "model", Model: "glm-5.1"},
		{Field: providers.TierOpus, Model: "glm-4.7"},
		{Field: providers.TierSonnet, Model: "glm-4.7"},
		{Field: providers.TierHaiku, Model: "glm-4.7"},
		{Field: providers.TierFable, Model: "glm-4.7"},
		{Field: providers.TierSubagent, Model: "glm-4.7"},
	}
	if len(pins) != len(want) {
		t.Fatalf("StalePins() = %+v, want %+v", pins, want)
	}
	for i := range want {
		if pins[i] != want[i] {
			t.Fatalf("StalePins()[%d] = %+v, want %+v (full: %+v)", i, pins[i], want[i], pins)
		}
	}
}
