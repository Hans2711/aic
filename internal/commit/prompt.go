package commit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/diesi/aic/internal/cli"
	"github.com/diesi/aic/internal/config"
	xterm "golang.org/x/term"
)

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
		fmt.Printf("%s", cli.ColorReset)
		if choiceInput != "" {
			if choiceInput == "0" && n == 10 {
				return suggestions[9], nil
			} else if v, err := strconv.Atoi(choiceInput); err == nil && v >= 1 && v <= n {
				return suggestions[v-1], nil
			}
		}
		return suggestions[0], nil
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
        fmt.Printf("%sUse ↑/↓ or j/k, Space to toggle select, numbers to pick (1-9%s), Enter to confirm%s.%s\n", cli.ColorDim, extra, multi, cli.ColorReset)
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
