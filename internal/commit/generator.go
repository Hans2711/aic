package commit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

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
	if v := os.Getenv("AIC_MODEL"); v != "" { cfg.Model = v }
	// Alias: plain gpt-5 -> specific dated release name
	if cfg.Model == "gpt-5" {
		cfg.Model = "gpt-5-2025-08-07"
	}
	if v := os.Getenv("AIC_SUGGESTIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 15 { // sanity limit
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
		if cfg.Suggestions > 0 && cfg.Suggestions < len(mock) { mock = mock[:cfg.Suggestions] }
		return mock, nil
	}
	if apiKey == "" { return nil, errors.New("missing OPENAI_API_KEY") }
	gitDiff, err := git.StagedDiff()
	if err != nil { return nil, err }
	if strings.TrimSpace(gitDiff) == "" { return nil, errors.New("no staged changes") }
	// Basic truncation safeguard
	if len(gitDiff) > 16000 { gitDiff = gitDiff[:16000] }

	systemMsg := "You are a helpful assistant that writes concise, conventional style Git commit messages. " +
		"Given a git diff, generate distinct high-quality commit message suggestions (max 30 tokens each). " +
		"Prioritize most impactful changes first; no line breaks within a message. " +
		"Return ONLY the commit messages, one per choice, with no numbering or bullets."
	if cfg.SystemAddition != "" { systemMsg += " Additional user instructions: " + cfg.SystemAddition }

	client := openai.NewClient(apiKey)
	temp := float32(0.4)
	resp, err := client.Chat(openai.ChatCompletionRequest{
		Model: cfg.Model,
		Messages: []openai.Message{{Role: "system", Content: systemMsg}, {Role: "user", Content: gitDiff}},
		MaxTokens: 256,
		N: cfg.Suggestions,
		Temperature: &temp,
	})
	if err != nil { return nil, err }
	if len(resp.Choices) == 0 { return nil, errors.New("no choices returned") }

	suggestions := make([]string, 0, len(resp.Choices))
	for _, c := range resp.Choices {
		msg := strings.TrimSpace(c.Message.Content)
		if msg == "" { continue }
		lines := []string{msg}
		if strings.Contains(msg, "\n") {
			lines = []string{}
			for _, line := range strings.Split(msg, "\n") {
				line = strings.TrimSpace(line)
				if line == "" { continue }
				lines = append(lines, line)
			}
		}
		for _, ln := range lines {
			ln = cli.StripLeadingListMarker(ln)
			if ln == "" { continue }
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

// PromptUserSelect lets the user choose a suggestion.
func PromptUserSelect(suggestions []string) (string, error) {
	// Non-interactive auto-select first suggestion if AIC_NON_INTERACTIVE=1
	if os.Getenv("AIC_NON_INTERACTIVE") == "1" {
		if len(suggestions) == 0 { return "", errors.New("no suggestions to select") }
		fmt.Printf("%s\n%sCommit message suggestions (non-interactive mode):%s\n", cli.ColorGray, cli.ColorBold, cli.ColorReset)
		for i, s := range suggestions { fmt.Printf("  %s[%d]%s %s%s%s\n", cli.ColorYellow, i+1, cli.ColorReset, cli.ColorCyan, s, cli.ColorReset) }
		return suggestions[0], nil
	}
	fmt.Printf("%s\n%s%s Commit message suggestions:%s\n", cli.ColorGray, cli.ColorBold, cli.IconInfo, cli.ColorReset)
	for i, s := range suggestions {
		fmt.Printf("  %s[%d]%s %s%s%s\n", cli.ColorYellow, i+1, cli.ColorReset, cli.ColorCyan, s, cli.ColorReset)
	}
	fmt.Printf("\n%s%s Choose a commit message %s[1-%d]%s %s[default: 1]%s: %s", cli.ColorBold, cli.IconPrompt, cli.ColorYellow, len(suggestions), cli.ColorReset, cli.ColorDim, cli.ColorReset, cli.ColorCyan)
	var choiceInput string
	fmt.Scanln(&choiceInput)
	selected := 1
	if choiceInput != "" {
		if v, err := strconv.Atoi(choiceInput); err == nil && v >= 1 && v <= len(suggestions) { selected = v }
	}
	fmt.Printf("%s", cli.ColorReset)
	return suggestions[selected-1], nil
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
	// best-effort clipboard (Linux xclip, fallback no-op)
	if _, err := exec.LookPath("xclip"); err == nil {
		c := exec.Command("xclip", "-selection", "clipboard")
		c.Stdin = strings.NewReader(msg)
		_ = c.Run()
		fmt.Printf("%sMessage copied to clipboard.%s\n", cli.ColorGreen, cli.ColorReset)
	}
	return nil
}
