package commit

import (
	"testing"
)

// NOTE: This test only validates configuration parsing without calling the API.
func TestLoadConfig(t *testing.T) {
	t.Setenv("AIC_DISABLE_REPO_CONFIG", "1")
	// Isolate from any real ~/.aic.json to keep expectations stable
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AIC_MODEL", "test-model")
	t.Setenv("AIC_SUGGESTIONS", "10")
	cfg, _ := LoadConfig("extra")
	if cfg.Model != "test-model" {
		t.Fatalf("expected model override, got %s", cfg.Model)
	}
	if cfg.Suggestions != 10 {
		t.Fatalf("expected suggestions=10 got %d", cfg.Suggestions)
	}
	if cfg.SystemAddition != "extra" {
		t.Fatalf("system addition mismatch")
	}
}

func TestLoadConfigBounds(t *testing.T) {
	t.Setenv("AIC_DISABLE_REPO_CONFIG", "1")
	t.Setenv("AIC_MODEL", "")
	t.Setenv("AIC_SUGGESTIONS", "999") // out of range, should fallback
	cfg, _ := LoadConfig("")
	if cfg.Suggestions != defaultSuggestions {
		t.Fatalf("expected default suggestions, got %d", cfg.Suggestions)
	}
	if cfg.Model != openAILargeModel {
		t.Fatalf("expected default model, got %s", cfg.Model)
	}
}

func TestLoadCombineConfigOverrides(t *testing.T) {
	t.Setenv("AIC_DISABLE_REPO_CONFIG", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AIC_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "x")
	t.Setenv("AIC_MODEL", "gpt-4o-mini")
	t.Setenv("AIC_COMBINE_PROVIDER", "claude")
	t.Setenv("CLAUDE_API_KEY", "y")
	t.Setenv("AIC_COMBINE_MODEL", "claude-3-sonnet-20240229")
	t.Setenv("AIC_COMBINE_SUGGESTIONS", "3")
	cfg, err := LoadCombineConfig("")
	if err != nil {
		t.Fatalf("LoadCombineConfig error: %v", err)
	}
	if cfg.Provider != "claude" {
		t.Fatalf("expected provider claude, got %s", cfg.Provider)
	}
	if cfg.Model != "claude-3-sonnet-20240229" {
		t.Fatalf("expected model override, got %s", cfg.Model)
	}
	if cfg.Suggestions != 3 {
		t.Fatalf("expected suggestions=3 got %d", cfg.Suggestions)
	}
}
