package commit

import (
	"os"
	"testing"
)

// NOTE: This test only validates configuration parsing without calling the API.
func TestLoadConfig(t *testing.T) {
	os.Setenv("AIC_MODEL", "test-model")
	os.Setenv("AIC_SUGGESTIONS", "10")
	cfg, _ := LoadConfig("extra")
	if cfg.Model != "test-model" { t.Fatalf("expected model override, got %s", cfg.Model) }
	if cfg.Suggestions != 10 { t.Fatalf("expected suggestions=10 got %d", cfg.Suggestions) }
	if cfg.SystemAddition != "extra" { t.Fatalf("system addition mismatch") }
}

func TestLoadConfigBounds(t *testing.T) {
	os.Setenv("AIC_MODEL", "")
	os.Setenv("AIC_SUGGESTIONS", "999") // out of range, should fallback
	cfg, _ := LoadConfig("")
	if cfg.Suggestions != defaultSuggestions { t.Fatalf("expected default suggestions, got %d", cfg.Suggestions) }
	if cfg.Model != defaultModel { t.Fatalf("expected default model, got %s", cfg.Model) }
}
