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
		mock := []string{"Update code for mock change", "Fix mock issue in logic", "Update dependencies"}
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
    // Choose diff source: in daemon mode, include all changes (staged + unstaged)
    var gitDiff string
    var err error
    if config.DaemonEnabled() {
        gitDiff, err = git.WorktreeDiff()
    } else {
        gitDiff, err = git.StagedDiff()
    }
    if err != nil {
        return nil, err
    }
    if strings.TrimSpace(gitDiff) == "" {
        if config.DaemonEnabled() {
            return nil, errors.New("no changes compared to HEAD")
        }
        return nil, errors.New("no staged changes")
    }
    warnIfSecrets(gitDiff)

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
		if summary != "" && config.Bool(config.EnvAICDebug) {
			fmt.Fprintf(os.Stderr, "%s\n[debug] diff summarized (orig=%d chars, shown=%d)\n%s\n", cli.ColorDim, len(originalDiff), len(gitDiff), cli.ColorReset)
			fmt.Fprintf(os.Stderr, "===== DIFF SUMMARY DEBUG START =====\n%s\n===== DIFF SUMMARY DEBUG END =====\n", summary)
		}
	}

    // When we summarize, include both head and tail of the raw diff to give
    // the model broader coverage while staying within a similar total budget.
    if summary != "" && len([]rune(originalDiff)) > hardLimit {
        half := hardLimit / 2
        head := firstNRunes(originalDiff, half)
        tail := lastNRunes(originalDiff, half)
        gitDiff = head + "\n--- TAIL OF TRUNCATED RAW DIFF ---\n" + tail
    }
    ctx := git.RepoContext()
    userContent := composeUserContent(originalDiff, gitDiff, summary)
    if ctx != "" {
        userContent = ctx + "\n\n" + userContent
    }
    systemMsg := "You write concise, natural-language Git commit subjects. " +
        "Rules: one line per message (<=92 chars), imperative mood, no trailing period; " +
        "do NOT use type prefixes or scopes (no 'feat:' or 'feat(scope):'). " +
        "Do not mention files, authors, diffs, or explain rationale. No numbering, bullets, quotes, emojis, or reasoning. " +
        "Output: return ONLY the subjects, one per choice. " +
        "Produce exactly " + strconv.Itoa(cfg.Suggestions) + " distinct options prioritizing the most impactful changes."
	if cfg.SystemAddition != "" {
		systemMsg += " Additional user instructions: " + cfg.SystemAddition
	}

	if config.Bool(config.EnvAICDebug) {
		fmt.Fprintln(os.Stderr, "[aic][debug] system prompt for suggestions:")
		fmt.Fprintln(os.Stderr, systemMsg)
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
	// Choose a safe model for summarization. For custom providers, allow auto-pick.
	model := defaultModelFor(providerName)
	if providerName == "custom" {
		model = "" // let custom provider pick via /v1/models
	}
	req := openai.ChatCompletionRequest{
		Model: model,
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

// lastNRunes returns at most n runes from the end of the input string.
// It ensures any truncation occurs on rune boundaries so the result is valid UTF-8.
func lastNRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[len(r)-n:]
	}
	return string(r)
}

// composeUserContent builds the final user prompt content with optional summary and truncated diff markers.
// originalDiff: full diff (possibly large), truncatedDiff: trimmed part actually included, summary: optional summary.
func composeUserContent(originalDiff, truncatedDiff, summary string) string {
	if summary == "" {
		return truncatedDiff
	}
	// Count runes to report accurate character counts for non-ASCII data
	rlen := func(s string) int { return len([]rune(s)) }
	omitted := rlen(originalDiff) - rlen(truncatedDiff)
	if omitted < 0 {
		omitted = 0
	}
	cutoffNote := "[TRUNCATED: showing parts totaling " + strconv.Itoa(rlen(truncatedDiff)) + " of " + strconv.Itoa(rlen(originalDiff)) + " chars; omitted " + strconv.Itoa(omitted) + "]"
	return "DIFF SUMMARY (model-generated)\n" + summary + "\n\n" + cutoffNote + "\n--- BEGIN TRUNCATED RAW DIFF ---\n" + truncatedDiff + "\n--- END TRUNCATED RAW DIFF ---\n" + cutoffNote
}
