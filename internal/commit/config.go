package commit

import (
	"strings"

	"github.com/diesi/aic/internal/config"
)

const (
	defaultOpenAIModel = "gpt-4o-mini"
	defaultClaudeModel = "claude-3-sonnet-20240229"
	defaultGeminiModel = "gemini-1.5-flash"
	defaultSuggestions = 5
)

func defaultModelFor(providerName string) string {
	switch providerName {
	case "claude":
		return defaultClaudeModel
	case "gemini":
		return defaultGeminiModel
	default:
		return defaultOpenAIModel
	}
}

// Config holds runtime parameters loaded from env.
type Config struct {
	Provider       string
	Model          string
	Suggestions    int
	SystemAddition string
}

func LoadConfig(systemAddition string) (Config, error) {
	providerName := strings.ToLower(config.Get(config.EnvAICProvider))
	if providerName == "" {
		// Auto-detect provider from available API keys when AIC_PROVIDER is unset.
		// Priority when multiple are present: OpenAI > Claude > Gemini.
		hasOpenAI := strings.TrimSpace(config.Get(config.EnvOpenAIAPIKey)) != ""
		hasClaude := strings.TrimSpace(config.Get(config.EnvClaudeAPIKey)) != ""
		hasGemini := strings.TrimSpace(config.Get(config.EnvGeminiAPIKey)) != ""
		switch {
		case hasOpenAI:
			providerName = "openai"
		case hasClaude:
			providerName = "claude"
		case hasGemini:
			providerName = "gemini"
		default:
			// Fall back to OpenAI if no keys are set; error handling later will guide the user.
			providerName = "openai"
		}
	}
	cfg := Config{Provider: providerName, Model: defaultModelFor(providerName), Suggestions: defaultSuggestions, SystemAddition: systemAddition}
	// In non-interactive mode, favor requesting a single suggestion by default
	// to avoid unnecessary tokens/work. Users can still override via AIC_SUGGESTIONS.
	if config.Bool(config.EnvAICNonInteractive) {
		cfg.Suggestions = 1
	}
	if v := config.Get(config.EnvAICModel); v != "" {
		cfg.Model = v
	}
	// Alias: plain gpt-5 -> specific dated release name
	if cfg.Provider == "openai" && cfg.Model == "gpt-5" {
		cfg.Model = "gpt-5-2025-08-07"
	}
	// sanity limit (max 10 for quick selection)
	cfg.Suggestions = config.IntInRange(config.EnvAICSuggestions, cfg.Suggestions, 1, 10)
	return cfg, nil
}
