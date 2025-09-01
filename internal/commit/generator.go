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
	"github.com/diesi/aic/internal/config"
	"github.com/diesi/aic/internal/git"
	"github.com/diesi/aic/internal/openai"
	"github.com/diesi/aic/internal/provider"
	xterm "golang.org/x/term"
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
		if cfg.Provider == "claude" {
			return nil, errors.New("missing CLAUDE_API_KEY")
		}
		return nil, errors.New("missing OPENAI_API_KEY")
	}
	gitDiff, err := git.StagedDiff()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(gitDiff) == "" {
		return nil, errors.New("no staged changes")
	}

	var p provider.Provider
	if cfg.Provider == "claude" {
		p = provider.NewClaude(apiKey)
	} else {
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
	systemMsg := "You are a helpful assistant that writes concise, conventional style Git commit messages. " +
		"Given a git diff, generate distinct high-quality commit message suggestions (max 30 tokens each). " +
		"Prioritize most impactful changes first; no line breaks within a message. " +
		"Return ONLY the commit messages, one per choice, with no numbering or bullets."
	if cfg.SystemAddition != "" {
		systemMsg += " Additional user instructions: " + cfg.SystemAddition
	}

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
			"fix: address edge cases from combined"}
		if cfg.Suggestions > 0 && cfg.Suggestions < len(out) {
			out = out[:cfg.Suggestions]
		}
		return out, nil
	}
	if apiKey == "" {
		if cfg.Provider == "claude" {
			return nil, errors.New("missing CLAUDE_API_KEY")
		}
		return nil, errors.New("missing OPENAI_API_KEY")
	}
	var p provider.Provider
	if cfg.Provider == "claude" {
		p = provider.NewClaude(apiKey)
	} else {
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

// PromptUserSelect lets the user choose a suggestion.
func PromptUserSelect(suggestions []string) (string, error) {
	// Non-interactive auto-select first suggestion if AIC_NON_INTERACTIVE=1
	if config.Bool(config.EnvAICNonInteractive) {
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
		if n == 10 {
			rangeLabel = "1-9,0"
		}
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
		if n == 10 {
			rangeLabel = "1-9,0"
		}
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
	checked := map[int]bool{}
	countChecked := func() int {
		c := 0
		for _, v := range checked {
			if v {
				c++
			}
		}
		return c
	}
	render := func() {
		cols := termCols()
		// Compute visible prefix width precisely: prefix (2) + "[d]" (3) + space (1) + "[ ]" (3) + space (1)
		// idxLabel is one visible char (1..9 or 0 for 10th)
		visiblePrefix := 2 + 3 + 1 + 3 + 1 // = 10
		maxMsg := cols - visiblePrefix
		if maxMsg < 10 {
			maxMsg = 10
		}
		// Header (single line, no leading blank line)
		fmt.Printf("%s%s %sCommit message suggestions:%s\n", cli.ColorGray, cli.ColorBold, cli.IconInfo, cli.ColorReset)
		for i := 0; i < n; i++ {
			idxLabel := fmt.Sprintf("%d", i+1)
			if n == 10 && i == 9 {
				idxLabel = "0"
			}
			// Checkbox indicator for multi-select
			box := "[ ]"
			if checked[i] {
				box = "[x]"
			}
			prefix := "  "
			lineColorStart := cli.ColorCyan
			lineColorEnd := cli.ColorReset
			if i == selected {
				// Highlight selected line
				prefix = fmt.Sprintf("%s> %s", cli.ColorYellow, cli.ColorReset)
				lineColorStart = cli.ColorGreen + cli.ColorBold
			}
			msg := suggestions[i]
			if runeLen(msg) > maxMsg {
				msg = truncateRunes(msg, maxMsg)
			}
			fmt.Printf("%s[%s] %s %s%s%s\n", prefix, idxLabel, box, lineColorStart, msg, lineColorEnd)
		}
		// Instructions
		extra := ",0"
		if n != 10 {
			extra = ""
		}
		multi := ""
		if countChecked() >= 2 {
			multi = fmt.Sprintf(" – %d selected; Enter combines", countChecked())
		}
		fmt.Printf("%sUse ↑/↓, Space to toggle select, numbers to pick (1-9%s), Enter to confirm%s.%s\n", cli.ColorDim, extra, multi, cli.ColorReset)
	}

	// Initial render
	render()

	// Read keys and update selection; act immediately on number press
	in := make([]byte, 3)
	// total lines for header + list + instruction
	backLines := n + 2
	moveUp := func(lines int) {
		if lines > 0 {
			fmt.Printf("\033[%dA", lines)
		}
	}
	clearLine := func() { fmt.Printf("\033[2K\r") }
	for {
		// Read one byte; handle escape sequences manually
		_, err := os.Stdin.Read(in[:1])
		if err != nil {
			// On read error, just return current selection
			break
		}
		b := in[0]
		if b == 0 {
			continue
		}
		switch b {
		case 3: // Ctrl+C
			return "", errors.New("selection canceled")
		case '\r', '\n':
			// Enter confirms or combines
			if countChecked() >= 2 {
				// Restore terminal to normal before network call/spinner
				restore()
				// Collect selected messages in order of appearance
				combined := make([]string, 0, countChecked())
				for i := 0; i < n; i++ {
					if checked[i] {
						combined = append(combined, suggestions[i])
					}
				}
				// Load cfg from env for combine step
				cfg, _ := LoadConfig("")
				var key string
				if cfg.Provider == "claude" {
					key = config.Get(config.EnvClaudeAPIKey)
				} else {
					key = config.Get(config.EnvOpenAIAPIKey)
				}
				stop := cli.Spinner(fmt.Sprintf("Combining %d selected messages via %s", len(combined), cfg.Model))
				newSugs, err := GenerateCombinedSuggestions(cfg, key, combined)
				stop(err == nil)
				if err != nil {
					return "", err
				}
				// Re-enter cbreak mode for interactive selection
				var reErr error
				restore, reErr = enableCBreak()
				if reErr != nil {
					// Fallback: simple selection prompt
					// Print combined suggestions and pick first by default
					fmt.Printf("%s%s %sCombined suggestions:%s\n", cli.ColorGray, cli.ColorBold, cli.IconInfo, cli.ColorReset)
					for i, s := range newSugs {
						fmt.Printf("  %s[%d]%s %s%s%s\n", cli.ColorYellow, i+1, cli.ColorReset, cli.ColorCyan, s, cli.ColorReset)
					}
					fmt.Printf("\n%s%s Choose a commit message [default: 1]: %s", cli.ColorBold, cli.IconPrompt, cli.ColorCyan)
					var choiceInput string
					fmt.Scanln(&choiceInput)
					fmt.Printf("%s", cli.ColorReset)
					if choiceInput == "" {
						return newSugs[0], nil
					}
					if v, err := strconv.Atoi(choiceInput); err == nil && v >= 1 && v <= len(newSugs) {
						return newSugs[v-1], nil
					}
					return newSugs[0], nil
				}
				// Ensure terminal will be restored on function exit for the new cbreak session too
				defer restore()
				// Replace list and reset state, then re-render
				suggestions = newSugs
				n = min(len(suggestions), 10)
				selected = 0
				checked = map[int]bool{}
				// Recompute lines and render
				backLines = n + 2
				render()
				continue
			}
			// If exactly one is checked, return it; otherwise return current selection
			if countChecked() == 1 {
				for i := 0; i < n; i++ {
					if checked[i] {
						return suggestions[i], nil
					}
				}
			}
			return suggestions[selected], nil
		case ' ': // Space toggles selection on current line
			checked[selected] = !checked[selected]
		case 'k': // vim-like up (optional)
			if selected > 0 {
				selected--
			}
		case 'j': // vim-like down (optional)
			if selected < n-1 {
				selected++
			}
		case 27: // ESC sequence
			// Read next two bytes if available for CSI
			os.Stdin.Read(in[1:2])
			if in[1] != '[' {
				continue
			}
			os.Stdin.Read(in[2:3])
			switch in[2] {
			case 'A': // Up arrow
				if selected > 0 {
					selected--
				}
			case 'B': // Down arrow
				if selected < n-1 {
					selected++
				}
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
		for i := 0; i < backLines; i++ {
			clearLine()
			if i < backLines-1 {
				fmt.Printf("\n")
			}
		}
		moveUp(backLines - 1)
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
		if restored {
			return
		}
		restored = true
		cmd := exec.Command("stty", strings.TrimSpace(string(state)))
		cmd.Stdin = os.Stdin
		_ = cmd.Run()
	}
	return restore, nil
}

// termCols returns the terminal width in columns, falling back to env COLUMNS or 80.
func termCols() int {
	// First choice: get size from the actual TTY we write to (stdout).
	if xterm.IsTerminal(int(os.Stdout.Fd())) {
		if width, _, err := xterm.GetSize(int(os.Stdout.Fd())); err == nil && width > 20 {
			return width
		}
	}
	// Second choice: query stdin TTY (often the same device during interactive selection).
	if xterm.IsTerminal(int(os.Stdin.Fd())) {
		if width, _, err := xterm.GetSize(int(os.Stdin.Fd())); err == nil && width > 20 {
			return width
		}
	}
	// Third: shell-provided COLUMNS env.
	if v := config.Get(config.EnvColumns); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 20 {
			return n
		}
	}
	// Last resort: stty size if stdin is a TTY.
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		cmd := exec.Command("stty", "size")
		cmd.Stdin = os.Stdin
		if out, err := cmd.Output(); err == nil {
			parts := strings.Fields(strings.TrimSpace(string(out)))
			if len(parts) == 2 {
				if cols, err := strconv.Atoi(parts[1]); err == nil && cols > 20 {
					return cols
				}
			}
		}
	}
	return 80
}

// runeLen returns number of runes in s.
func runeLen(s string) int { return len([]rune(s)) }

// truncateRunes truncates s to max runes and appends an ellipsis if truncated.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// OfferCommit asks to commit or copy to clipboard.
func OfferCommit(msg string) error {
	fmt.Printf("\n%sSelected commit message:%s\n  %s%s%s\n", cli.ColorBold, cli.ColorReset, cli.ColorGreen, msg, cli.ColorReset)
	if config.Bool(config.EnvAICNonInteractive) {
		// In CI/test mode, don't attempt to commit unless explicitly allowed
		if config.Bool(config.EnvAICAutoCommit) {
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
