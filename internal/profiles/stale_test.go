package profiles

import (
	"strings"
	"testing"

	"github.com/jolehuit/clother/internal/config"
	"github.com/jolehuit/clother/internal/providers"
)

// A pinned model the catalog no longer offers ("stale pin") must be
// surfaced, so the user is not left launching a provider with a model ID
// the backend silently reroutes or rejects.

func TestStalePinsFlagsUnknownModel(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.1"}

	pins := StalePins("zai", catalog, cfg)
	if len(pins) != 1 {
		t.Fatalf("StalePins() = %+v, want exactly one stale pin", pins)
	}
	if pins[0].Field != "model" || pins[0].Model != "glm-5.1" {
		t.Fatalf("StalePins()[0] = %+v, want {model glm-5.1}", pins[0])
	}
}

func TestStalePinsAcceptsKnownModelChoice(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.3"}

	if pins := StalePins("zai", catalog, cfg); len(pins) != 0 {
		t.Fatalf("StalePins() = %+v, want none for a known model_choices entry", pins)
	}
}

func TestStalePinsFlagsStaleTierOnly(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{
		TierModels: config.TierModels{HaikuModel: "glm-4.7"},
	}

	pins := StalePins("zai", catalog, cfg)
	if len(pins) != 1 {
		t.Fatalf("StalePins() = %+v, want exactly one stale pin", pins)
	}
	if pins[0].Field != "haiku" || pins[0].Model != "glm-4.7" {
		t.Fatalf("StalePins()[0] = %+v, want {haiku glm-4.7}", pins[0])
	}
}

func TestStalePinsOrdersModelThenTiers(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{
		Model:      "glm-5.1",
		TierModels: config.TierModels{OpusModel: "glm-4.7"},
	}

	pins := StalePins("zai", catalog, cfg)
	want := []StalePin{
		{Field: "model", Model: "glm-5.1"},
		{Field: "opus", Model: "glm-4.7"},
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

func TestStalePinsEmptyWithoutOverride(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()

	if pins := StalePins("zai", catalog, cfg); len(pins) != 0 {
		t.Fatalf("StalePins() = %+v, want none without an override", pins)
	}
}

func TestStalePinsEmptyForProviderWithoutModelChoices(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := catalog.Get("ollama")
	if !ok {
		t.Fatal("ollama provider missing from catalog")
	}
	if len(provider.ModelChoices) != 0 {
		t.Fatalf("ollama has model_choices %+v, need a provider with none for this test", provider.ModelChoices)
	}

	cfg := newTestConfig()
	cfg.ProviderOverrides["ollama"] = config.ProviderOverride{Model: "whatever-arbitrary-model"}

	if pins := StalePins("ollama", catalog, cfg); len(pins) != 0 {
		t.Fatalf("StalePins() = %+v, want none for a provider without model_choices", pins)
	}
}

func TestStalePinsEmptyForOpenRouterAlias(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.OpenRouterAliases["x"] = "some/model"

	if pins := StalePins("or-x", catalog, cfg); len(pins) != 0 {
		t.Fatalf("StalePins() = %+v, want none for an OpenRouter alias", pins)
	}
}

func TestStalePinsEmptyForCustomProvider(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()
	cfg.CustomProviders["gateway"] = config.CustomProvider{
		Name: "gateway", DisplayName: "gateway", BaseURL: "https://gateway.example.com",
		APIKeyEnv: "GATEWAY_API_KEY", DefaultModel: "model-a",
	}

	if pins := StalePins("gateway", catalog, cfg); len(pins) != 0 {
		t.Fatalf("StalePins() = %+v, want none for a custom provider", pins)
	}
}

func TestStalePinsEmptyForUnknownProfile(t *testing.T) {
	t.Parallel()

	catalog, err := providers.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig()

	if pins := StalePins("does-not-exist", catalog, cfg); len(pins) != 0 {
		t.Fatalf("StalePins() = %+v, want none for an unknown profile", pins)
	}
}

// ProviderStalePins must not bail out early just because ModelChoices is
// empty: a provider that still declares a DefaultModel and/or ModelTiers has
// known models to check a pin against, even without an explicit choices
// list.
func TestProviderStalePinsFlagsPinOutsideDefaultAndTiersWithoutModelChoices(t *testing.T) {
	t.Parallel()

	provider := providers.Provider{
		ID:           "x",
		DefaultModel: "m1",
		ModelTiers:   map[string]string{"opus": "m2"},
	}
	override := config.ProviderOverride{Model: "zzz"}

	pins := ProviderStalePins(provider, override)
	want := []StalePin{{Field: "model", Model: "zzz"}}
	if len(pins) != len(want) {
		t.Fatalf("ProviderStalePins() = %+v, want %+v", pins, want)
	}
	for i := range want {
		if pins[i] != want[i] {
			t.Fatalf("ProviderStalePins()[%d] = %+v, want %+v (full: %+v)", i, pins[i], want[i], pins)
		}
	}
}

// Companion: a provider with no DefaultModel, no ModelTiers and no
// ModelChoices at all has nothing to check a pin against, so it must never
// flag one.
func TestProviderStalePinsEmptyForProviderWithNoCatalogModelInfo(t *testing.T) {
	t.Parallel()

	provider := providers.Provider{ID: "y"}
	override := config.ProviderOverride{
		Model:      "anything",
		TierModels: config.TierModels{OpusModel: "whatever"},
	}

	if pins := ProviderStalePins(provider, override); len(pins) != 0 {
		t.Fatalf("ProviderStalePins() = %+v, want none for a provider with no DefaultModel, ModelTiers or ModelChoices", pins)
	}
}

func TestStaleWarningEmptyWithoutPins(t *testing.T) {
	t.Parallel()

	if got := StaleWarning("zai", nil); got != "" {
		t.Fatalf("StaleWarning(nil) = %q, want empty", got)
	}
	if got := StaleWarning("zai", []StalePin{}); got != "" {
		t.Fatalf("StaleWarning(empty) = %q, want empty", got)
	}
}

func TestStaleWarningMentionsPinCommandAndCatalog(t *testing.T) {
	t.Parallel()

	pins := []StalePin{{Field: "model", Model: "glm-5.1"}}
	msg := StaleWarning("zai", pins)

	if strings.Contains(msg, "\n") {
		t.Fatalf("StaleWarning() = %q, want a single line", msg)
	}
	if !strings.Contains(msg, "glm-5.1") {
		t.Fatalf("StaleWarning() = %q, want it to mention the stale model", msg)
	}
	if !strings.Contains(msg, "clother config zai") {
		t.Fatalf("StaleWarning() = %q, want it to point at clother config zai", msg)
	}
	if !strings.Contains(msg, "catalog") {
		t.Fatalf("StaleWarning() = %q, want it to mention the catalog", msg)
	}
}

// The catalog referenced here is the one Clother ships (bundled with the
// binary), not some other, unspecified "current" catalog; the wording must
// say so.
func TestStaleWarningMentionsBundledCatalog(t *testing.T) {
	t.Parallel()

	pins := []StalePin{{Field: "model", Model: "glm-5.1"}}
	msg := StaleWarning("zai", pins)

	if !strings.Contains(msg, "bundled catalog") {
		t.Fatalf("StaleWarning() = %q, want it to mention \"bundled catalog\"", msg)
	}
}
