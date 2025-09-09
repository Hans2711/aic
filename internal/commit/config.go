package commit

import (
    "fmt"
    "os"
    "strings"

    "github.com/diesi/aic/internal/config"
)

const (
    // Defaults for initial suggestions
    // OpenAI: favor higher quality by default
    defaultOpenAIModel = "gpt-4o"
    defaultClaudeModel = "claude-sonnet-4-20250514"
    defaultGeminiModel = "gemini-2.5-flash"
    defaultSuggestions = 5
)

func defaultModelFor(providerName string) string {
	switch providerName {
	case "claude":
		return defaultClaudeModel
	case "gemini":
		return defaultGeminiModel
	case "custom":
		// For custom provider, default to OpenAI-compatible model name; users can override via AIC_MODEL.
		return defaultOpenAIModel
	default:
		return defaultOpenAIModel
	}
}

// defaultCombineModelFor returns the default model to use during the
// combine step. This can differ from the initial suggestion default.
func defaultCombineModelFor(providerName string) string {
    switch providerName {
    case "openai":
        // For combine, default to a faster model unless overridden
        return "gpt-4o-mini"
    case "claude":
        return defaultClaudeModel
    case "gemini":
        return defaultGeminiModel
    case "custom":
        // Let custom follow the same default as initial; provider may auto-pick
        return defaultOpenAIModel
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
	// Load optional repo and user instructions and merge with CLI-provided additions.
    // Merge order (lowest -> highest precedence): repo-style-memory, repo, home, CLI.
    // The final string concatenates non-empty parts with spaces.
    parts := []string{}
    rs := config.LoadRepoStyle()
    rc := config.LoadRepoConfig()
    uc := config.LoadUserConfig()
    if rs.Instructions != "" {
        parts = append(parts, rs.Instructions)
    }
    if rc.Instructions != "" {
        parts = append(parts, rc.Instructions)
    }
	if uc.Instructions != "" {
		parts = append(parts, uc.Instructions)
	}
	if strings.TrimSpace(systemAddition) != "" {
		parts = append(parts, strings.TrimSpace(systemAddition))
	}
	systemAddition = strings.TrimSpace(strings.Join(parts, " "))

    if config.Bool(config.EnvAICDebug) {
        if config.Bool(config.EnvAICDisableRepoConfig) {
            fmt.Fprintln(os.Stderr, "[aic][debug] repo config disabled via AIC_DISABLE_REPO_CONFIG=1")
        }
        fmt.Fprintf(os.Stderr, "[aic][debug] repo style memory instructions: %q\n", rs.Instructions)
        fmt.Fprintf(os.Stderr, "[aic][debug] repo .aic.json instructions: %q\n", rc.Instructions)
        fmt.Fprintf(os.Stderr, "[aic][debug] home ~/.aic.json instructions: %q\n", uc.Instructions)
        fmt.Fprintf(os.Stderr, "[aic][debug] merged instructions: %q\n", systemAddition)
    }
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
	// For custom provider, if AIC_MODEL isn't explicitly set, leave model empty and let the provider pick from /v1/models.
	if cfg.Provider == "custom" && config.Get(config.EnvAICModel) == "" {
		cfg.Model = ""
	}
	// Alias: plain gpt-5 -> specific dated release name
	if cfg.Provider == "openai" && cfg.Model == "gpt-5" {
		cfg.Model = "gpt-5-2025-08-07"
	}
	// sanity limit (max 10 for quick selection)
	cfg.Suggestions = config.IntInRange(config.EnvAICSuggestions, cfg.Suggestions, 1, 10)
	return cfg, nil
}

// LoadCombineConfig loads configuration for the combine step. It starts with the
// base config and then applies any AIC_COMBINE_* overrides.
func LoadCombineConfig(systemAddition string) (Config, error) {
    cfg, err := LoadConfig(systemAddition)
    if err != nil {
        return Config{}, err
    }
    // Override provider for combine if explicitly set
    if v := strings.ToLower(config.Get(config.EnvAICCombineProvider)); v != "" {
        cfg.Provider = v
    }

    // Determine default combine model. If AIC_COMBINE_MODEL is set, use it.
    // Otherwise, if the user explicitly set AIC_MODEL, keep it unless a
    // combine provider override was given. If neither were set, use the
    // combine default (OpenAI: gpt-4o-mini).
    combineModel := strings.TrimSpace(config.Get(config.EnvAICCombineModel))
    userModel := strings.TrimSpace(config.Get(config.EnvAICModel))
    if combineModel != "" {
        cfg.Model = combineModel
    } else {
        // If provider was overridden for combine, pick its combine default.
        // If not overridden, only switch to combine default when user did not set AIC_MODEL.
        if strings.ToLower(config.Get(config.EnvAICCombineProvider)) != "" || userModel == "" {
            cfg.Model = defaultCombineModelFor(cfg.Provider)
        }
        // Special case: custom provider without explicit model -> let provider pick from /v1/models
        if cfg.Provider == "custom" && userModel == "" && combineModel == "" {
            cfg.Model = ""
        }
    }
    if cfg.Provider == "openai" && cfg.Model == "gpt-5" {
        cfg.Model = "gpt-5-2025-08-07"
    }
    cfg.Suggestions = config.IntInRange(config.EnvAICCombineSuggestions, cfg.Suggestions, 1, 10)
    return cfg, nil
}
