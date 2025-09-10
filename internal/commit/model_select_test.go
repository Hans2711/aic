package commit

import (
	"strings"
	"testing"

	"github.com/diesi/aic/internal/config"
)

func TestSmallLargeModelForDefaultsAndEnv(t *testing.T) {
	t.Setenv("AIC_MODEL_SMALL", "")
	t.Setenv("AIC_MODEL_LARGE", "")
	cases := []struct {
		provider string
		small    string
		large    string
	}{
		{"openai", openAISmallModel, openAILargeModel},
		{"claude", claudeSmallModel, claudeLargeModel},
		{"gemini", geminiSmallModel, geminiLargeModel},
		{"custom", openAISmallModel, openAILargeModel},
	}
	for _, c := range cases {
		if got := smallModelFor(c.provider); got != c.small {
			t.Fatalf("smallModelFor %s = %s want %s", c.provider, got, c.small)
		}
		if got := largeModelFor(c.provider); got != c.large {
			t.Fatalf("largeModelFor %s = %s want %s", c.provider, got, c.large)
		}
	}
	t.Setenv("AIC_MODEL_SMALL", "tiny")
	t.Setenv("AIC_MODEL_LARGE", "huge")
	if got := smallModelFor("openai"); got != "tiny" {
		t.Fatalf("env small override failed, got %s", got)
	}
	if got := largeModelFor("openai"); got != "huge" {
		t.Fatalf("env large override failed, got %s", got)
	}
}

func TestLoadConfig_ModelEnvOverrides(t *testing.T) {
	t.Setenv("AIC_DISABLE_REPO_CONFIG", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AIC_PROVIDER", "openai")
	t.Setenv("AIC_MODEL", "")
	t.Setenv("AIC_MODEL_LARGE", "huge")
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Model != "huge" {
		t.Fatalf("expected AIC_MODEL_LARGE to override, got %s", cfg.Model)
	}
	t.Setenv("AIC_MODEL", "user")
	cfg, err = LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Model != "user" {
		t.Fatalf("expected AIC_MODEL to override, got %s", cfg.Model)
	}

	t.Setenv("AIC_MODEL", "")
	t.Setenv("AIC_MODEL_SMALL", "tiny")
	cfg, err = LoadCombineConfig("")
	if err != nil {
		t.Fatalf("LoadCombineConfig error: %v", err)
	}
	if cfg.Model != "tiny" {
		t.Fatalf("expected AIC_MODEL_SMALL to override combine, got %s", cfg.Model)
	}
}

func TestModelForTokens(t *testing.T) {
	t.Setenv("AIC_MODEL_SMALL", "")
	t.Setenv("AIC_MODEL_LARGE", "")
    if got := ModelForTokens("openai", 1999); got != openAISmallModel {
		t.Fatalf("want small model, got %s", got)
	}
    if got := ModelForTokens("openai", 2000); got != openAILargeModel {
		t.Fatalf("want large model, got %s", got)
	}
	t.Setenv("AIC_MODEL_SMALL", "tiny")
	t.Setenv("AIC_MODEL_LARGE", "huge")
    if got := ModelForTokens("openai", 10); got != "tiny" {
		t.Fatalf("env small override, got %s", got)
	}
    if got := ModelForTokens("openai", 5000); got != "huge" {
		t.Fatalf("env large override, got %s", got)
	}
}

func TestModelSelectionRespectsAICModel(t *testing.T) {
	t.Setenv("AIC_MODEL", "explicit")
	cfg := Config{Provider: "openai", Model: "explicit"}
	diff := strings.Repeat("a", 100)
	tokens := len([]rune(diff)) / 4
    if config.Get(config.EnvAICModel) == "" {
        cfg.Model = ModelForTokens(cfg.Provider, tokens)
    }
	if cfg.Model != "explicit" {
		t.Fatalf("expected explicit model to be preserved, got %s", cfg.Model)
	}
}
