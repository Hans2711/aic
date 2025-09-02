package commit

import (
	"strings"

	"github.com/diesi/aic/internal/config"
)

const (
	defaultOpenAIModel = "gpt-4o-mini"
	defaultClaudeModel = "claude-3-sonnet-20240229"
	defaultSuggestions = 5
)

func defaultModelFor(providerName string) string {
	if providerName == "claude" {
		return defaultClaudeModel
	}
	return defaultOpenAIModel
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
		// Priority when both are present: OpenAI.
		hasOpenAI := strings.TrimSpace(config.Get(config.EnvOpenAIAPIKey)) != ""
		hasClaude := strings.TrimSpace(config.Get(config.EnvClaudeAPIKey)) != ""
		switch {
		case hasOpenAI && hasClaude:
			providerName = "openai"
		case hasClaude:
			providerName = "claude"
		default:
			// Fall back to OpenAI if neither key is set; error handling later will guide the user.
			providerName = "openai"
		}
	}
	cfg := Config{Provider: providerName, Model: defaultModelFor(providerName), Suggestions: defaultSuggestions, SystemAddition: systemAddition}
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
