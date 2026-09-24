package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jolehuit/clother/internal/cli"
	"github.com/jolehuit/clother/internal/config"
	"github.com/jolehuit/clother/internal/providers"
	"github.com/jolehuit/clother/internal/ui"
)

// newStaleTestContext builds a Context with a bytes.Buffer stdout, so the
// printed text and JSON output can both be inspected.
func newStaleTestContext(t *testing.T, format string) (Context, *config.File, *bytes.Buffer) {
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
	var stdout bytes.Buffer
	uiFormat := ui.FormatHuman
	if format == "json" {
		uiFormat = ui.FormatJSON
	}
	ctx := Context{
		Paths:   paths,
		Config:  cfg,
		Secrets: config.Secrets{},
		Catalog: catalog,
		Output:  &ui.Output{Stdout: &stdout, Stderr: io.Discard, Format: uiFormat},
		Options: cli.Options{Format: format},
	}
	return ctx, cfg, &stdout
}

func TestInfoHumanFormatShowsStaleLine(t *testing.T) {
	ctx, cfg, stdout := newStaleTestContext(t, "")
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.1"}

	code, err := runInfo(context.Background(), ctx, []string{"zai"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("runInfo() code = %d, want 0", code)
	}

	var staleLine string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "Stale:") {
			staleLine = line
			break
		}
	}
	if staleLine == "" {
		t.Fatalf("stdout has no Stale: line, got:\n%s", stdout.String())
	}
	if !strings.Contains(staleLine, "glm-5.1") {
		t.Fatalf("Stale: line = %q, want it to mention glm-5.1", staleLine)
	}
	if !strings.Contains(staleLine, "clother config zai") {
		t.Fatalf("Stale: line = %q, want it to point at clother config zai", staleLine)
	}
}

// The Stale: line must call the reference catalog "bundled" (the one
// shipped with Clother), not some other, unspecified "current" catalog.
func TestInfoHumanFormatStaleLineMentionsBundledCatalog(t *testing.T) {
	ctx, cfg, stdout := newStaleTestContext(t, "")
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.1"}

	code, err := runInfo(context.Background(), ctx, []string{"zai"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("runInfo() code = %d, want 0", code)
	}

	var staleLine string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "Stale:") {
			staleLine = line
			break
		}
	}
	if staleLine == "" {
		t.Fatalf("stdout has no Stale: line, got:\n%s", stdout.String())
	}
	if !strings.Contains(staleLine, "bundled catalog") {
		t.Fatalf("Stale: line = %q, want it to mention \"bundled catalog\"", staleLine)
	}
}

func TestInfoHumanFormatNoStaleLineWhenClean(t *testing.T) {
	ctx, _, stdout := newStaleTestContext(t, "")

	code, err := runInfo(context.Background(), ctx, []string{"zai"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("runInfo() code = %d, want 0", code)
	}
	if strings.Contains(stdout.String(), "Stale:") {
		t.Fatalf("stdout has an unexpected Stale: line, got:\n%s", stdout.String())
	}
}

func TestInfoJSONFormatIncludesStalePins(t *testing.T) {
	ctx, cfg, stdout := newStaleTestContext(t, "json")
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.1"}

	code, err := runInfo(context.Background(), ctx, []string{"zai"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("runInfo() code = %d, want 0", code)
	}

	var decoded struct {
		StalePins []struct {
			Field string `json:"field"`
			Model string `json:"model"`
		} `json:"StalePins"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode JSON: %v\noutput:\n%s", err, stdout.String())
	}
	if len(decoded.StalePins) != 1 {
		t.Fatalf("StalePins = %+v, want exactly one entry", decoded.StalePins)
	}
	if decoded.StalePins[0].Field != "model" || decoded.StalePins[0].Model != "glm-5.1" {
		t.Fatalf("StalePins[0] = %+v, want {field:model model:glm-5.1}", decoded.StalePins[0])
	}
}

func TestInfoJSONFormatOmitsStalePinsWhenClean(t *testing.T) {
	ctx, _, stdout := newStaleTestContext(t, "json")

	code, err := runInfo(context.Background(), ctx, []string{"zai"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("runInfo() code = %d, want 0", code)
	}

	var decoded struct {
		StalePins []struct {
			Field string `json:"field"`
			Model string `json:"model"`
		} `json:"StalePins"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode JSON: %v\noutput:\n%s", err, stdout.String())
	}
	if len(decoded.StalePins) != 0 {
		t.Fatalf("StalePins = %+v, want none when the pin is not stale", decoded.StalePins)
	}
}

func TestListHumanFormatMarksStaleRow(t *testing.T) {
	ctx, cfg, stdout := newStaleTestContext(t, "")
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.1"}

	code, err := runList(context.Background(), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("runList() code = %d, want 0", code)
	}

	var zaiLine string
	otherStale := false
	for _, line := range strings.Split(stdout.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "zai":
			zaiLine = line
		default:
			if strings.Contains(strings.ToLower(line), "stale") {
				otherStale = true
			}
		}
	}
	if zaiLine == "" {
		t.Fatalf("no zai row found, got:\n%s", stdout.String())
	}
	if !strings.Contains(strings.ToLower(zaiLine), "stale") {
		t.Fatalf("zai row = %q, want it to mention stale", zaiLine)
	}
	if otherStale {
		t.Fatalf("a non-stale row mentions stale, got:\n%s", stdout.String())
	}
}

func TestListJSONFormatMarksStaleItem(t *testing.T) {
	ctx, cfg, stdout := newStaleTestContext(t, "json")
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.1"}

	code, err := runList(context.Background(), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("runList() code = %d, want 0", code)
	}

	var decoded struct {
		Profiles []struct {
			Name  string `json:"name"`
			Stale bool   `json:"stale"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode JSON: %v\noutput:\n%s", err, stdout.String())
	}

	var zaiStale, zaiFound bool
	var otherFound, otherStale bool
	for _, item := range decoded.Profiles {
		if item.Name == "zai" {
			zaiFound = true
			zaiStale = item.Stale
			continue
		}
		otherFound = true
		if item.Stale {
			otherStale = true
		}
	}
	if !zaiFound {
		t.Fatal("no zai item found in profiles JSON")
	}
	if !zaiStale {
		t.Fatal("zai item stale = false, want true")
	}
	if !otherFound {
		t.Fatal("expected at least one non-zai profile in the JSON output")
	}
	if otherStale {
		t.Fatal("a non-stale provider was marked stale")
	}
}

func TestConfigZaiResetsStalePinOnEmptyAnswer(t *testing.T) {
	ctx, cfg := newConfigTestContext(t, "\n\n")
	ctx.Secrets["ZAI_API_KEY"] = "sk-test"
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.1"}

	var stdout bytes.Buffer
	ctx.Output = &ui.Output{Stdout: &stdout, Stderr: io.Discard, Format: ui.FormatHuman}

	provider, ok := ctx.Catalog.Get("zai")
	if !ok {
		t.Fatal("zai provider missing from catalog")
	}
	if _, err := configBuiltin(ctx, provider); err != nil {
		t.Fatal(err)
	}

	override, exists := cfg.ProviderOverrides["zai"]
	if exists && override.Model != "" {
		t.Fatalf("override = %+v, want the stale pin reset to the catalog default", override)
	}

	out := stdout.String()
	if !strings.Contains(out, "glm-5.1") {
		t.Fatalf("stdout = %q, want it to mention the stale pin glm-5.1", out)
	}
	if !strings.Contains(strings.ToLower(out), "stale") {
		t.Fatalf("stdout = %q, want it to mention that the pin is stale", out)
	}
	if !strings.Contains(out, "glm-5.3[1m]") {
		t.Fatalf("stdout = %q, want it to mention the catalog default glm-5.3[1m]", out)
	}
}

func TestConfigZaiKeepsValidPinOnEmptyAnswer(t *testing.T) {
	ctx, cfg := newConfigTestContext(t, "\n\n")
	ctx.Secrets["ZAI_API_KEY"] = "sk-test"
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.3"}

	provider, ok := ctx.Catalog.Get("zai")
	if !ok {
		t.Fatal("zai provider missing from catalog")
	}
	if _, err := configBuiltin(ctx, provider); err != nil {
		t.Fatal(err)
	}

	if got := cfg.ProviderOverrides["zai"].Model; got != "glm-5.3" {
		t.Fatalf("override model = %q, want glm-5.3 kept", got)
	}
}

func TestConfigZaiDashClearsValidPin(t *testing.T) {
	ctx, cfg := newConfigTestContext(t, "\n-\n")
	ctx.Secrets["ZAI_API_KEY"] = "sk-test"
	cfg.ProviderOverrides["zai"] = config.ProviderOverride{Model: "glm-5.3"}

	provider, ok := ctx.Catalog.Get("zai")
	if !ok {
		t.Fatal("zai provider missing from catalog")
	}
	if _, err := configBuiltin(ctx, provider); err != nil {
		t.Fatal(err)
	}

	override, exists := cfg.ProviderOverrides["zai"]
	if exists && override.Model != "" {
		t.Fatalf("override = %+v, want the pin cleared by \"-\"", override)
	}
}
