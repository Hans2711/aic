package commit

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/diesi/aic/internal/cli"
	"github.com/diesi/aic/internal/config"
	"github.com/diesi/aic/internal/git"
	"github.com/diesi/aic/internal/openai"
	"github.com/diesi/aic/internal/provider"
)

// GenerateSuggestions creates commit message suggestions based on staged diff.
func GenerateSuggestions(cfg Config, apiKey string) ([]string, error) {
	if config.Bool(config.EnvAICMock) {
		mock := []string{"feat: mock change", "fix: mock issue", "chore: update dependencies"}
		if cfg.Suggestions > 0 && cfg.Suggestions < len(mock) {
			mock = mock[:cfg.Suggestions]
		}
		return mock, nil
	}
    if apiKey == "" {
        switch cfg.Provider {
        case "claude":
            return nil, errors.New("missing CLAUDE_API_KEY")
        case "gemini":
            return nil, errors.New("missing GEMINI_API_KEY")
        case "custom":
            // Custom provider may not require an API key (e.g., local LM Studio)
            // Proceed without error.
        default:
            return nil, errors.New("missing OPENAI_API_KEY")
        }
    }
	gitDiff, err := git.StagedDiff()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(gitDiff) == "" {
		return nil, errors.New("no staged changes")
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

	originalDiff := gitDiff
	const hardLimit = 16000
	var summary string
	if len(originalDiff) > hardLimit {
		if s, sumErr := summarizeDiff(p, cfg.Provider, originalDiff); sumErr == nil && strings.TrimSpace(s) != "" {
			summary = s
		} else {
			summary = ""
		}
		if len(gitDiff) > hardLimit {
			if !utf8.ValidString(gitDiff[:hardLimit]) {
				cut := hardLimit
				for cut > 0 && (gitDiff[cut]&0xC0) == 0x80 {
					cut--
				}
				gitDiff = gitDiff[:cut]
			} else {
				gitDiff = gitDiff[:hardLimit]
			}
		}
		if summary != "" && config.Bool(config.EnvAICDebugSummary) {
			fmt.Fprintf(os.Stderr, "%s\n[debug] diff summarized (orig=%d chars, shown=%d)\n%s\n", cli.ColorDim, len(originalDiff), len(gitDiff), cli.ColorReset)
			fmt.Fprintf(os.Stderr, "===== DIFF SUMMARY DEBUG START =====\n%s\n===== DIFF SUMMARY DEBUG END =====\n", summary)
		}
	}

    userContent := composeUserContent(originalDiff, gitDiff, summary)
    systemMsg := "You generate single-line Conventional Commit messages. " +
        "Rules: one line per message (<=72 chars), imperative mood, no trailing period; " +
        "start with a type (feat|fix|refactor|docs|chore|test|perf|build|ci|style) and optional scope; " +
        "do NOT mention the diff/user/files or explain. No numbering, bullets, quotes, emojis, or reasoning. " +
        "Output: return ONLY the messages, one per choice. " +
        "Produce exactly " + strconv.Itoa(cfg.Suggestions) + " distinct options prioritizing the most impactful changes."
	if cfg.SystemAddition != "" {
		systemMsg += " Additional user instructions: " + cfg.SystemAddition
	}

    temp := float32(0.25)
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
		errMsg := "empty suggestions"
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

// summarizeDiff creates a concise structured summary of a very large diff.
// It ALWAYS uses the providers default model (defaultModel constant) regardless of user override.
// The output is intentionally compact: bullet-style high level file change descriptions + notable additions/removals.
func summarizeDiff(p provider.Provider, providerName, diff string) (string, error) {
	// Light temperature for determinism
	temp := float32(0.2)
	req := openai.ChatCompletionRequest{
		Model: defaultModelFor(providerName),
		Messages: []openai.Message{
			{
				Role:    "system",
				Content: "You summarize git diffs. Produce a concise overview: list each file (max 1 line) with nature of change (add/remove/modify/rename) and highlight any: API signature changes, new public functions, deleted functions, dependency/version changes, security related changes, configuration changes. After the list, include a short 'Key Impacts:' section (<=3 bullet lines). No commit messages, no speculation.",
			},
			{
				Role:    "user",
				Content: firstNRunes(diff, 48000),
			},
		},
		MaxTokens:   384,
		Temperature: &temp,
	}
	resp, err := p.Chat(req)
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("empty summary response")
	}
	out := strings.TrimSpace(resp.Choices[0])
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// firstNRunes returns at most n runes from the input string.
// It ensures any truncation occurs on rune boundaries so the result is valid UTF-8.
func firstNRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// composeUserContent builds the final user prompt content with optional summary and truncated diff markers.
// originalDiff: full diff (possibly large), truncatedDiff: trimmed part actually included, summary: optional summary.
func composeUserContent(originalDiff, truncatedDiff, summary string) string {
	if summary == "" {
		return truncatedDiff
	}
	omitted := len(originalDiff) - len(truncatedDiff)
	if omitted < 0 {
		omitted = 0
	}
	cutoffNote := "[TRUNCATED: showing first " + strconv.Itoa(len(truncatedDiff)) + " of " + strconv.Itoa(len(originalDiff)) + " chars; omitted " + strconv.Itoa(omitted) + "]"
	return "DIFF SUMMARY (model-generated)\n" + summary + "\n\n" + cutoffNote + "\n--- BEGIN TRUNCATED RAW DIFF ---\n" + truncatedDiff + "\n--- END TRUNCATED RAW DIFF ---\n" + cutoffNote
}
