package profiles

import (
	"fmt"
	"strings"

	"github.com/jolehuit/clother/internal/config"
	"github.com/jolehuit/clother/internal/providers"
)

// StalePin is a model pinned in the user's config that the provider's catalog
// no longer offers. Field is "model", "opus", "sonnet" or "haiku".
type StalePin struct {
	Field string `json:"field"`
	Model string `json:"model"`
}

// StalePins lists the pinned models of a catalog provider that are not among
// its catalog model IDs (default model, tier models and model choices).
// Providers without a model list (local, OpenRouter aliases, custom) never
// have stale pins.
func StalePins(profile string, catalog providers.Catalog, cfg *config.File) []StalePin {
	provider, ok := catalog.Get(profile)
	if !ok || cfg == nil {
		return nil
	}
	override, ok := cfg.ProviderOverrides[profile]
	if !ok {
		return nil
	}
	return ProviderStalePins(provider, override)
}

// ProviderStalePins is StalePins for one catalog provider and its override.
func ProviderStalePins(provider providers.Provider, override config.ProviderOverride) []StalePin {
	if len(provider.ModelChoices) == 0 {
		return nil
	}
	known := map[string]bool{provider.DefaultModel: true}
	for _, model := range provider.ModelTiers {
		known[model] = true
	}
	for _, choice := range provider.ModelChoices {
		known[choice.ID] = true
	}
	var pins []StalePin
	for _, pin := range []StalePin{
		{Field: "model", Model: override.Model},
		{Field: providers.TierOpus, Model: override.OpusModel},
		{Field: providers.TierSonnet, Model: override.SonnetModel},
		{Field: providers.TierHaiku, Model: override.HaikuModel},
	} {
		pin.Model = strings.TrimSpace(pin.Model)
		if pin.Model != "" && !known[pin.Model] {
			pins = append(pins, pin)
		}
	}
	return pins
}

// StaleWarning is the one-line notice printed when launching a profile with
// stale pins; empty when there are none.
func StaleWarning(profile string, pins []StalePin) string {
	if len(pins) == 0 {
		return ""
	}
	return fmt.Sprintf("clother: %s pins %s, not in the current catalog; run `clother config %s` to reset", profile, FormatStalePins(pins), profile)
}

// FormatStalePins renders pins as "model=glm-5.1, haiku=glm-4.7".
func FormatStalePins(pins []StalePin) string {
	parts := make([]string, 0, len(pins))
	for _, pin := range pins {
		parts = append(parts, pin.Field+"="+pin.Model)
	}
	return strings.Join(parts, ", ")
}
