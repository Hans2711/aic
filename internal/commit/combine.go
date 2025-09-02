package commit

import (
	"errors"
	"fmt"
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
			"refactor: combined mock suggestions",
			"chore: refine combined wording",
			"feat: consolidate scope across changes",
			"fix: address edge cases from combined",
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
	default:
		p = provider.NewOpenAI(apiKey)
	}
	systemMsg := "You are a helpful assistant that synthesizes multiple draft commit messages into improved conventional commit suggestions. " +
		"Given several commit messages that may overlap, produce distinct, concise, high-quality alternatives (max 30 tokens each). " +
		"No line breaks; return ONLY the commit messages, one per choice, with no numbering or bullets."
	userContent := "Combine and refine these commit messages into consolidated alternatives:\n\n" + strings.Join(selected, "\n")

	temp := float32(0.4)
	resp, err := p.Chat(openai.ChatCompletionRequest{
		Model:       cfg.Model,
		Messages:    []openai.Message{{Role: "system", Content: systemMsg}, {Role: "user", Content: userContent}},
		MaxTokens:   256,
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
