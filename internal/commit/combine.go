package commit

import (
    "errors"
    "fmt"
    "os"
    "strings"

    "github.com/diesi/aic/internal/cli"
    "github.com/diesi/aic/internal/config"
    "github.com/diesi/aic/internal/openai"
    "github.com/diesi/aic/internal/provider"
)

// GenerateCombinedSuggestions asks the AI to combine multiple commit messages
// into a fresh set of consolidated suggestions. It returns up to cfg.Suggestions
// items, formatted one message per choice with no numbering or bullets.
func GenerateCombinedSuggestions(cfg Config, apiKey string, selected []string) ([]string, error) {
	if len(selected) < 2 {
		return nil, errors.New("need at least two messages to combine")
	}
	if config.Bool(config.EnvAICMock) {
        fused := strings.Join(selected, "; ")
        out := []string{
            fused,
            "Refine combined suggestions for clarity",
            "Improve wording across merged changes",
            "Consolidate changes into a single subject",
            "Address edge cases from merged updates",
        }
        if cfg.Suggestions > 0 && cfg.Suggestions < len(out) {
            out = out[:cfg.Suggestions]
        }
        return out, nil
    }
    if apiKey == "" {
        switch cfg.Provider {
        case "claude":
            return nil, errors.New("missing CLAUDE_API_KEY")
        case "gemini":
            return nil, errors.New("missing GEMINI_API_KEY")
        case "custom":
            // Custom providers may not require a key (e.g., local LM Studio)
            // Proceed without error.
        default:
            return nil, errors.New("missing OPENAI_API_KEY")
        }
    }
	var p provider.Provider
    switch cfg.Provider {
    case "claude":
        p = provider.NewClaude(apiKey)
    case "gemini":
        p = provider.NewGemini(apiKey)
    case "custom":
        p = provider.NewCustom(apiKey)
    default:
        p = provider.NewOpenAI(apiKey)
    }
    systemMsg := "You synthesize multiple draft commit messages into improved, concise natural-language Git commit subjects. " +
        "Rules: output a single-line subject per choice, imperative mood, no trailing period; no type prefixes or scopes. " +
        "Prefer clarity even if longer than 92 characters. Return ONLY the subjects, with no numbering or bullets."
    if cfg.SystemAddition != "" {
        systemMsg += " Additional user instructions: " + cfg.SystemAddition
    }
    if config.Bool(config.EnvAICDebug) {
        fmt.Fprintln(os.Stderr, "[aic][debug] system prompt for combine:")
        fmt.Fprintln(os.Stderr, systemMsg)
    }
	userContent := "Combine and refine these commit messages into consolidated alternatives:\n\n" + strings.Join(selected, "\n")

    temp := float32(0.4)
    resp, err := p.Chat(openai.ChatCompletionRequest{
        Model:       cfg.Model,
        Messages:    []openai.Message{{Role: "system", Content: systemMsg}, {Role: "user", Content: userContent}},
        // Allow longer combined subjects when helpful
        MaxTokens:   512,
        N:           cfg.Suggestions,
        Temperature: &temp,
    })
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("no choices returned")
	}
	suggestions := make([]string, 0, len(resp.Choices))
	for _, msg := range resp.Choices {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			continue
		}
		lines := []string{msg}
		if strings.Contains(msg, "\n") {
			lines = []string{}
			for _, line := range strings.Split(msg, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				lines = append(lines, line)
			}
		}
		for _, ln := range lines {
			ln = cli.StripLeadingListMarker(ln)
			if ln == "" {
				continue
			}
			suggestions = append(suggestions, ln)
		}
	}
	if len(suggestions) == 0 {
		errMsg := "empty suggestions after combining"
		if config.Bool(config.EnvAICDebug) && resp != nil && resp.Raw != "" {
			errMsg = fmt.Sprintf("%s\n\nRaw Response:\n%s", errMsg, resp.Raw)
		}
		return nil, errors.New(errMsg)
	}
	if len(suggestions) > cfg.Suggestions {
		suggestions = suggestions[:cfg.Suggestions]
	}
	return suggestions, nil
}
