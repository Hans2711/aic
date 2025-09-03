package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Centralized environment variable keys used by the app
const (
	EnvOpenAIAPIKey = "OPENAI_API_KEY"
	EnvClaudeAPIKey = "CLAUDE_API_KEY"
	EnvGeminiAPIKey = "GEMINI_API_KEY"
	// Optional API key for custom provider (may be empty)
	EnvCustomAPIKey      = "CUSTOM_API_KEY"
	EnvAICModel          = "AIC_MODEL"
	EnvAICSuggestions    = "AIC_SUGGESTIONS"
	EnvAICMock           = "AIC_MOCK"
    EnvAICDebug          = "AIC_DEBUG"
	EnvAICNonInteractive = "AIC_NON_INTERACTIVE"
	EnvAICAutoCommit     = "AIC_AUTO_COMMIT"
    EnvAICNoColor        = "AIC_NO_COLOR"
    // Testing/advanced: disable reading repo-local .aic.json
    EnvAICDisableRepoConfig = "AIC_DISABLE_REPO_CONFIG"

	// Common terminal environment variables (non AIC-specific)
	EnvNoColor = "NO_COLOR"
	EnvTerm    = "TERM"
	EnvColumns = "COLUMNS"

	// Provider selection
	EnvAICProvider = "AIC_PROVIDER"

	// Custom provider endpoint configuration
	EnvCustomBaseURL             = "CUSTOM_BASE_URL"              // default: http://127.0.0.1:1234
	EnvCustomChatCompletionsPath = "CUSTOM_CHAT_COMPLETIONS_PATH" // default: /v1/chat/completions
	EnvCustomCompletionsPath     = "CUSTOM_COMPLETIONS_PATH"      // default: /v1/completions
	EnvCustomEmbeddingsPath      = "CUSTOM_EMBEDDINGS_PATH"       // default: /v1/embeddings
	EnvCustomModelsPath          = "CUSTOM_MODELS_PATH"           // default: /v1/models
)

// HelpEnvRowsCore returns the core environment variables and their descriptions
// for display in CLI help output. Keep descriptions concise and include
// "required" where applicable so callers can highlight them.
func HelpEnvRowsCore() [][2]string {
    return [][2]string{
		{EnvOpenAIAPIKey, "(required for provider=openai) OpenAI API key"},
		{EnvClaudeAPIKey, "(required for provider=claude) Claude API key"},
		{EnvGeminiAPIKey, "(required for provider=gemini) Gemini API key"},
		{EnvCustomAPIKey, "(optional for provider=custom) API key if your server requires it"},
		{EnvAICModel, "(optional) Model [default depends on provider]"},
		{EnvAICSuggestions, "(optional) Suggestions count 1-10 [default: 5; non-interactive: 1]"},
		{EnvAICProvider, "(optional) Provider [openai|claude|gemini|custom] (default: auto-detect from keys; priority openai>claude>gemini)"},
		{EnvAICDebug, "(optional) Set to 1 for raw response debug"},
		{EnvAICMock, "(optional) Set to 1 for mock suggestions (no API call)"},
		{EnvAICNonInteractive, "(optional) 1 to auto-select first suggestion & skip commit"},
		{EnvAICAutoCommit, "(optional) With NON_INTERACTIVE=1, also perform the commit"},
        {EnvAICNoColor, "(optional) Disable colored output (same as --no-color)"},
    }
}

// HelpEnvRowsCustom returns the custom-provider specific environment variables
// and their descriptions for CLI help output.
func HelpEnvRowsCustom() [][2]string {
	return [][2]string{
		{EnvCustomBaseURL, "(custom) Base URL [default: http://127.0.0.1:1234]"},
		{EnvCustomChatCompletionsPath, "(custom) Chat endpoint path [default: /v1/chat/completions]"},
		{EnvCustomCompletionsPath, "(custom) Completions path [default: /v1/completions]"},
		{EnvCustomEmbeddingsPath, "(custom) Embeddings path [default: /v1/embeddings]"},
		{EnvCustomModelsPath, "(custom) Models path [default: /v1/models]"},
	}
}

// Get returns the raw value for key (empty string if unset).
func Get(key string) string { return os.Getenv(key) }

// Bool parses a boolean-like environment variable.
// True values: 1, true, yes, on (case-insensitive).
// False values: 0, false, no, off. Empty is false.
func Bool(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return false
	}
	v = strings.ToLower(v)
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		// Fallback: treat any non-empty, non-falsey as true for leniency
		return true
	}
}

// IntInRange reads an int value with bounds; returns def if unset/invalid or out of range.
func IntInRange(key string, def, min, max int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < min || n > max {
		return def
	}
	return n
}

// WarnUnknownAICEnv scans the current process env and warns about unknown AIC_* vars.
// It helps catch typos or stale variables (e.g., AIC_PROVDIER, AIC_PROVIDER).
// Warnings are printed to stderr.
func WarnUnknownAICEnv() {
    known := map[string]struct{}{
        EnvAICModel: {}, EnvAICSuggestions: {}, EnvAICMock: {}, EnvAICDebug: {},
        EnvAICNonInteractive: {}, EnvAICAutoCommit: {}, EnvAICNoColor: {},
        EnvAICProvider: {}, EnvAICDisableRepoConfig: {},
        // custom provider configuration keys
        EnvCustomBaseURL: {}, EnvCustomChatCompletionsPath: {}, EnvCustomCompletionsPath: {},
        EnvCustomEmbeddingsPath: {}, EnvCustomModelsPath: {}, EnvCustomAPIKey: {},
    }
	// List of variables that exist in docs historically but are not currently used
	unused := map[string]string{}
	printedHeader := false
	printHdr := func() {
		if printedHeader {
			return
		}
		fmt.Fprintln(os.Stderr, "[aic] Notes about environment variables:")
		printedHeader = true
	}
	for _, entry := range os.Environ() {
		// entry is KEY=VALUE
		if !strings.HasPrefix(entry, "AIC_") {
			continue
		}
		k := entry
		if i := strings.IndexByte(entry, '='); i >= 0 {
			k = entry[:i]
		}
		if _, ok := known[k]; ok {
			continue
		}
		if msg, ok := unused[k]; ok {
			printHdr()
			fmt.Fprintf(os.Stderr, "  - %s is %s\n", k, msg)
			continue
		}
		// Unknown
		printHdr()
		fmt.Fprintf(os.Stderr, "  - %s is not recognized; check for typos or remove it.\n", k)
	}
}
