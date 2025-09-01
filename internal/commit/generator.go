package commit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/diesi/aic/internal/cli"
	"github.com/diesi/aic/internal/git"
	"github.com/diesi/aic/internal/openai"
)

const (
	defaultModel       = "gpt-4o-mini"
	defaultSuggestions = 5
)

// Config holds runtime parameters loaded from env.
type Config struct {
	Model          string
	Suggestions    int
	SystemAddition string
}

func LoadConfig(systemAddition string) (Config, error) {
	cfg := Config{Model: defaultModel, Suggestions: defaultSuggestions, SystemAddition: systemAddition}
	if v := os.Getenv("AIC_MODEL"); v != "" {
		cfg.Model = v
	}
	// Alias: plain gpt-5 -> specific dated release name
	if cfg.Model == "gpt-5" {
		cfg.Model = "gpt-5-2025-08-07"
	}
    if v := os.Getenv("AIC_SUGGESTIONS"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 10 { // sanity limit (max 10 for quick selection)
            cfg.Suggestions = n
        }
    }
	return cfg, nil
}

// GenerateSuggestions creates commit message suggestions based on staged diff.
func GenerateSuggestions(cfg Config, apiKey string) ([]string, error) {
	// Mock mode for testing without hitting real API (set AIC_MOCK=1)
	if os.Getenv("AIC_MOCK") == "1" {
		mock := []string{"feat: mock change", "fix: mock issue", "chore: update dependencies"}
		if cfg.Suggestions > 0 && cfg.Suggestions < len(mock) {
			mock = mock[:cfg.Suggestions]
		}
		return mock, nil
	}
	if apiKey == "" {
		return nil, errors.New("missing OPENAI_API_KEY")
	}
	gitDiff, err := git.StagedDiff()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(gitDiff) == "" {
		return nil, errors.New("no staged changes")
	}

	originalDiff := gitDiff
	const hardLimit = 16000 // legacy safeguard / final truncation size for raw diff included in prompt
	var summary string
	if len(originalDiff) > hardLimit {
		// Perform rich summarization using provider default model (ignores user override)
		if s, sumErr := summarizeDiff(apiKey, originalDiff); sumErr == nil && strings.TrimSpace(s) != "" {
			summary = s
		} else {
			// Fallback: no summary (will just truncate like before)
			summary = ""
		}
		// Always truncate raw diff part after (optionally) obtaining summary to keep prompt bounded.
		if len(gitDiff) > hardLimit {
			// ensure we truncate on rune boundary to avoid broken UTF-8
			if !utf8.ValidString(gitDiff[:hardLimit]) {
				// walk back to last rune boundary
				cut := hardLimit
				for cut > 0 && (gitDiff[cut]&0xC0) == 0x80 {
					cut--
				}
				gitDiff = gitDiff[:cut]
			} else {
				gitDiff = gitDiff[:hardLimit]
			}
		}
		if summary != "" && os.Getenv("AIC_DEBUG_SUMMARY") == "1" {
			fmt.Fprintf(os.Stderr, "%s\n[debug] diff summarized (orig=%d chars, shown=%d)\n%s\n", cli.ColorDim, len(originalDiff), len(gitDiff), cli.ColorReset)
			fmt.Fprintf(os.Stderr, "===== DIFF SUMMARY DEBUG START =====\n%s\n===== DIFF SUMMARY DEBUG END =====\n", summary)
		}
	}

	// Compose user content: include summary (if any) plus truncated diff.
	userContent := composeUserContent(originalDiff, gitDiff, summary)

	systemMsg := "You are a helpful assistant that writes concise, conventional style Git commit messages. " +
		"Given a git diff, generate distinct high-quality commit message suggestions (max 30 tokens each). " +
		"Prioritize most impactful changes first; no line breaks within a message. " +
		"Return ONLY the commit messages, one per choice, with no numbering or bullets."
	if cfg.SystemAddition != "" {
		systemMsg += " Additional user instructions: " + cfg.SystemAddition
	}

	client := openai.NewClient(apiKey)
	temp := float32(0.4)
	resp, err := client.Chat(openai.ChatCompletionRequest{
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
	for _, c := range resp.Choices {
		msg := strings.TrimSpace(c.Message.Content)
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
	// If we still have no suggestions, include snippet of raw response for context.
	if len(suggestions) == 0 {
		errMsg := "empty suggestions after processing"
		if os.Getenv("AIC_DEBUG") == "1" && resp != nil && resp.Raw != "" {
			errMsg = fmt.Sprintf("%s\n\nRaw Response:\n%s", errMsg, resp.Raw)
		}
		return nil, errors.New(errMsg)
	}
	// Trim to requested number if model returned more.
	if len(suggestions) > cfg.Suggestions {
		suggestions = suggestions[:cfg.Suggestions]
	}
	return suggestions, nil
}

// summarizeDiff creates a concise structured summary of a very large diff.
// It ALWAYS uses the providers default model (defaultModel constant) regardless of user override.
// The output is intentionally compact: bullet-style high level file change descriptions + notable additions/removals.
func summarizeDiff(apiKey, diff string) (string, error) {
	if apiKey == "" {
		return "", errors.New("missing api key for summarization")
	}
	client := openai.NewClient(apiKey)
	// Light temperature for determinism
	temp := float32(0.2)
	// We cap tokens aggressively; summary should stay small.
	req := openai.ChatCompletionRequest{
		Model: defaultModel, // enforce provider default model per requirement
		Messages: []openai.Message{
			{
				Role:    "system",
				Content: "You summarize git diffs. Produce a concise overview: list each file (max 1 line) with nature of change (add/remove/modify/rename) and highlight any: API signature changes, new public functions, deleted functions, dependency/version changes, security related changes, configuration changes. After the list, include a short 'Key Impacts:' section (<=3 bullet lines). No commit messages, no speculation.",
			},
			{
				Role:    "user",
				Content: firstNRunes(diff, 48000), // guard extremely huge diffs using rune count
			},
		},
		MaxTokens:   384,
		Temperature: &temp,
	}
	resp, err := client.Chat(req)
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("empty summary response")
	}
	out := strings.TrimSpace(resp.Choices[0].Message.Content)
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

// PromptUserSelect lets the user choose a suggestion.
func PromptUserSelect(suggestions []string) (string, error) {
    // Non-interactive auto-select first suggestion if AIC_NON_INTERACTIVE=1
    if os.Getenv("AIC_NON_INTERACTIVE") == "1" {
        if len(suggestions) == 0 {
            return "", errors.New("no suggestions to select")
        }
        fmt.Printf("%s\n%sCommit message suggestions (non-interactive mode):%s\n", cli.ColorGray, cli.ColorBold, cli.ColorReset)
        for i, s := range suggestions {
            fmt.Printf("  %s[%d]%s %s%s%s\n", cli.ColorYellow, i+1, cli.ColorReset, cli.ColorCyan, s, cli.ColorReset)
        }
        return suggestions[0], nil
    }

    // Limit display and selection to at most 10 (keys 1-9,0)
    n := min(len(suggestions), 10)
    if n == 0 {
        return "", errors.New("no suggestions to select")
    }

    // If STDIN is not a TTY (e.g., piped input), fall back to simple Scanln to remain scriptable
    if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
        fmt.Printf("%s%s %sCommit message suggestions:%s\n", cli.ColorGray, cli.ColorBold, cli.IconInfo, cli.ColorReset)
        for i := 0; i < n; i++ {
            fmt.Printf("  %s[%d]%s %s%s%s\n", cli.ColorYellow, i+1, cli.ColorReset, cli.ColorCyan, suggestions[i], cli.ColorReset)
        }
        // Indicate 0 for tenth if applicable
        rangeLabel := fmt.Sprintf("1-%d", n)
        if n == 10 { rangeLabel = "1-9,0" }
        fmt.Printf("\n%s%s Choose a commit message %s[%s]%s %s[default: 1]%s: %s", cli.ColorBold, cli.IconPrompt, cli.ColorYellow, rangeLabel, cli.ColorReset, cli.ColorDim, cli.ColorReset, cli.ColorCyan)
        var choiceInput string
        fmt.Scanln(&choiceInput)
        selected := 1
        if choiceInput != "" {
            if choiceInput == "0" && n == 10 {
                selected = 10
            } else if v, err := strconv.Atoi(choiceInput); err == nil && v >= 1 && v <= n {
                selected = v
            }
        }
        fmt.Printf("%s", cli.ColorReset)
        return suggestions[selected-1], nil
    }

    // Interactive TTY mode with single-key selection and arrow navigation
    // Save current terminal settings and switch to non-canonical, no-echo mode (cbreak)
    restore, err := enableCBreak()
    if err != nil {
        // Fallback to Scanln if terminal tweak fails
        fmt.Printf("%s%s %sCommit message suggestions:%s\n", cli.ColorGray, cli.ColorBold, cli.IconInfo, cli.ColorReset)
        for i := 0; i < n; i++ {
            fmt.Printf("  %s[%d]%s %s%s%s\n", cli.ColorYellow, i+1, cli.ColorReset, cli.ColorCyan, suggestions[i], cli.ColorReset)
        }
        rangeLabel := fmt.Sprintf("1-%d", n)
        if n == 10 { rangeLabel = "1-9,0" }
        fmt.Printf("\n%s%s Choose a commit message %s[%s]%s %s[default: 1]%s: %s", cli.ColorBold, cli.IconPrompt, cli.ColorYellow, rangeLabel, cli.ColorReset, cli.ColorDim, cli.ColorReset, cli.ColorCyan)
        var choiceInput string
        fmt.Scanln(&choiceInput)
        selected := 1
        if choiceInput != "" {
            if choiceInput == "0" && n == 10 {
                selected = 10
            } else if v, err := strconv.Atoi(choiceInput); err == nil && v >= 1 && v <= n {
                selected = v
            }
        }
        fmt.Printf("%s", cli.ColorReset)
        return suggestions[selected-1], nil
    }
    defer restore()

    selected := 0
    render := func() {
        cols := termCols()
        // content prefix length estimate: "> " or two spaces + "[x] " ~ 6 chars
        maxMsg := cols - 8
        if maxMsg < 10 { maxMsg = 10 }
        // Header (single line, no leading blank line)
        fmt.Printf("%s%s %sCommit message suggestions:%s\n", cli.ColorGray, cli.ColorBold, cli.IconInfo, cli.ColorReset)
        for i := 0; i < n; i++ {
            idxLabel := fmt.Sprintf("%d", i+1)
            if n == 10 && i == 9 { idxLabel = "0" }
            prefix := "  "
            lineColorStart := cli.ColorCyan
            lineColorEnd := cli.ColorReset
            if i == selected {
                // Highlight selected line
                prefix = fmt.Sprintf("%s> %s", cli.ColorYellow, cli.ColorReset)
                lineColorStart = cli.ColorGreen + cli.ColorBold
            }
            msg := suggestions[i]
            if runeLen(msg) > maxMsg { msg = truncateRunes(msg, maxMsg) }
            fmt.Printf("%s[%s] %s%s%s\n", prefix, idxLabel, lineColorStart, msg, lineColorEnd)
        }
        // Instructions
        fmt.Printf("%sUse ↑/↓ to navigate, numbers to select (1-9%s), Enter to confirm.%s\n", cli.ColorDim, func() string { if n == 10 { return ",0" }; return "" }(), cli.ColorReset)
    }

    // Initial render
    render()

    // Read keys and update selection; act immediately on number press
    in := make([]byte, 3)
    // total lines for header + list + instruction
    backLines := n + 2
    moveUp := func(lines int) { if lines > 0 { fmt.Printf("\033[%dA", lines) } }
    clearLine := func() { fmt.Printf("\033[2K\r") }
    for {
        // Read one byte; handle escape sequences manually
        _, err := os.Stdin.Read(in[:1])
        if err != nil {
            // On read error, just return current selection
            break
        }
        b := in[0]
        if b == 0 { continue }
        switch b {
        case 3: // Ctrl+C
            return "", errors.New("selection canceled")
        case '\r', '\n':
            // Enter confirms
            return suggestions[selected], nil
        case 'k': // vim-like up (optional)
            if selected > 0 { selected-- }
        case 'j': // vim-like down (optional)
            if selected < n-1 { selected++ }
        case 27: // ESC sequence
            // Read next two bytes if available for CSI
            os.Stdin.Read(in[1:2])
            if in[1] != '[' { continue }
            os.Stdin.Read(in[2:3])
            switch in[2] {
            case 'A': // Up arrow
                if selected > 0 { selected-- }
            case 'B': // Down arrow
                if selected < n-1 { selected++ }
            }
        default:
            // Number keys: 1..9 select directly; 0 selects 10th when available
            if b >= '1' && b <= '9' {
                v := int(b - '0')
                if v >= 1 && v <= n {
                    return suggestions[v-1], nil
                }
            } else if b == '0' && n == 10 {
                return suggestions[9], nil
            }
        }
        // Re-render list in place without drifting
        moveUp(backLines)
        for i := 0; i < backLines; i++ { clearLine(); if i < backLines-1 { fmt.Printf("\n") } }
        moveUp(backLines-1)
        render()
    }
    return suggestions[selected], nil
}

// enableCBreak switches terminal to non-canonical, no-echo mode using `stty` and returns a restore func.
func enableCBreak() (func(), error) {
    // Save current settings
    save := exec.Command("stty", "-g")
    save.Stdin = os.Stdin
    state, err := save.Output()
    if err != nil {
        return func() {}, err
    }
    // Set to cbreak (non-canonical) and no-echo, return after 1 byte
    set := exec.Command("stty", "-icanon", "-echo", "min", "1", "time", "0")
    set.Stdin = os.Stdin
    if err := set.Run(); err != nil {
        return func() {}, err
    }
    restored := false
    restore := func() {
        if restored { return }
        restored = true
        cmd := exec.Command("stty", strings.TrimSpace(string(state)))
        cmd.Stdin = os.Stdin
        _ = cmd.Run()
    }
    return restore, nil
}

// termCols returns the terminal width in columns, falling back to env COLUMNS or 80.
func termCols() int {
    if v := os.Getenv("COLUMNS"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 20 { return n }
    }
    return 80
}

// runeLen returns number of runes in s.
func runeLen(s string) int { return len([]rune(s)) }

// truncateRunes truncates s to max runes and appends an ellipsis if truncated.
func truncateRunes(s string, max int) string {
    r := []rune(s)
    if len(r) <= max { return s }
    if max <= 1 { return string(r[:max]) }
    return string(r[:max-1]) + "…"
}

// OfferCommit asks to commit or copy to clipboard.
func OfferCommit(msg string) error {
	fmt.Printf("\n%sSelected commit message:%s\n  %s%s%s\n", cli.ColorBold, cli.ColorReset, cli.ColorGreen, msg, cli.ColorReset)
	if os.Getenv("AIC_NON_INTERACTIVE") == "1" {
		// In CI/test mode, don't attempt to commit unless explicitly allowed
		if os.Getenv("AIC_AUTO_COMMIT") == "1" {
			cmd := exec.Command("git", "commit", "-m", msg)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		fmt.Printf("Non-interactive mode: skipping commit (set AIC_AUTO_COMMIT=1 to enable).\n")
		return nil
	}
	fmt.Printf("\n%s%s Commit with this message now?%s %s[Y|n]%s %s[default: Y]%s: %s", cli.ColorBold, cli.IconPrompt, cli.ColorReset, cli.ColorYellow, cli.ColorReset, cli.ColorDim, cli.ColorReset, cli.ColorCyan)
	var commitChoice string
	fmt.Scanln(&commitChoice)
	if strings.ToLower(commitChoice) == "y" || commitChoice == "" {
		cmd := exec.Command("git", "commit", "-m", msg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	if copyToClipboard(msg) {
		fmt.Printf("%sMessage copied to clipboard.%s\n", cli.ColorGreen, cli.ColorReset)
	}
	return nil
}

type clipboardTool struct {
	name string
	args []string
}

var clipboardTools = []clipboardTool{
	{name: "pbcopy"},
	{name: "wl-copy"},
	{name: "xclip", args: []string{"-selection", "clipboard"}},
	{name: "clip"},
}

// copyToClipboard tries a series of common clipboard tools until one succeeds.
func copyToClipboard(msg string) bool {
	run := func(name string, args ...string) error {
		c := exec.Command(name, args...)
		c.Stdin = strings.NewReader(msg)
		return c.Run()
	}
	return tryClipboard(msg, exec.LookPath, run)
}

// tryClipboard attempts available clipboard tools using provided lookPath and run helpers.
func tryClipboard(msg string, lookPath func(string) (string, error), run func(string, ...string) error) bool {
	for _, t := range clipboardTools {
		if _, err := lookPath(t.name); err == nil {
			if run(t.name, t.args...) == nil {
				return true
			}
		}
	}
	return false
}
