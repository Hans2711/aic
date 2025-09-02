package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Centralized environment variable keys used by the app
const (
	EnvOpenAIAPIKey      = "OPENAI_API_KEY"
	EnvClaudeAPIKey      = "CLAUDE_API_KEY"
	EnvGeminiAPIKey      = "GEMINI_API_KEY"
	EnvAICModel          = "AIC_MODEL"
	EnvAICSuggestions    = "AIC_SUGGESTIONS"
	EnvAICMock           = "AIC_MOCK"
	EnvAICDebug          = "AIC_DEBUG"
	EnvAICDebugSummary   = "AIC_DEBUG_SUMMARY"
	EnvAICNonInteractive = "AIC_NON_INTERACTIVE"
	EnvAICAutoCommit     = "AIC_AUTO_COMMIT"
	EnvAICNoColor        = "AIC_NO_COLOR"

	// Common terminal environment variables (non AIC-specific)
	EnvNoColor = "NO_COLOR"
	EnvTerm    = "TERM"
	EnvColumns = "COLUMNS"

	// Provider selection
	EnvAICProvider = "AIC_PROVIDER"
)

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
		EnvAICDebugSummary: {}, EnvAICNonInteractive: {}, EnvAICAutoCommit: {}, EnvAICNoColor: {},
		EnvAICProvider: {},
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
