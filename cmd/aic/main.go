package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/diesi/aic/internal/cli"
	"github.com/diesi/aic/internal/commit"
)

func main() {
	// Environment variables are used directly; no .env file loading.
	var systemAddition string
	args := os.Args[1:]

	// Simple flag parsing
	for i, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			printHelp()
			return
		}
		if arg == "-s" {
			if i+1 < len(args) {
				systemAddition = args[i+1]
				// basic: assumes value isn't another flag
			}
		}
	}

	cfg, err := commit.LoadConfig(systemAddition)
	if err != nil {
		fatal(err)
	}

	stop := cli.Spinner(fmt.Sprintf("%sRequesting %s%d%s suggestions from %s%s%s", cli.ColorDim, cli.ColorYellow, cfg.Suggestions, cli.ColorReset, cli.ColorCyan, cfg.Model, cli.ColorReset))
	suggestions, err := commit.GenerateSuggestions(cfg, os.Getenv("OPENAI_API_KEY"))
	stop()
	if err != nil {
		if isInvalidKeyErr(err) {
			fmt.Fprintln(os.Stderr, "Hint: Ensure your real OPENAI_API_KEY is exported (export OPENAI_API_KEY=sk-...)")
		}
		fatal(err)
	}
	msg, err := commit.PromptUserSelect(suggestions)
	if err != nil {
		fatal(err)
	}
	if err := commit.OfferCommit(msg); err != nil {
		fatal(err)
	}
}

func printHelp() {
	fmt.Printf(`%s%s aic%s – %sAI-assisted git commit message generator%s

%sUsage%s:
	aic [-s "extra instruction"]

%sDescription%s:
	Generates conventional-style git commit messages based on your staged changes.
	It requests suggestions from an AI model, lets you choose one, and then offers
	to perform the commit or copy the message to your clipboard.

%sArguments%s:
	%s-s%s <instruction>   Additional instruction, e.g. %s"focus on backend"%s

%sEnvironment%s:
	%sOPENAI_API_KEY%s   (required) OpenAI API key
	%sAIC_MODEL%s        (optional) Model (default: gpt-4o-mini)
	%sAIC_SUGGESTIONS%s  (optional) Suggestions count 1-15 (default: 5)
	%sAIC_PROVIDER%s     (optional) Provider (default: openai)
	%sAIC_DEBUG%s        (optional) Set to 1 for raw response debug
	%sAIC_MOCK%s         (optional) Set to 1 to use mock suggestions (no API call)
	%sAIC_NON_INTERACTIVE%s (optional) Set to 1 to auto-select first suggestion & skip commit
	%sAIC_AUTO_COMMIT%s  (optional) With NON_INTERACTIVE=1, also perform the commit

	%sExample%s:
		aic -s "Refactor auth logic"
	`,
			cli.ColorBold, cli.ColorCyan, cli.ColorReset, cli.ColorMagenta, cli.ColorReset,
			cli.ColorBold, cli.ColorReset,
			cli.ColorBold, cli.ColorReset,
			cli.ColorBold, cli.ColorReset,
			cli.ColorYellow, cli.ColorReset, cli.ColorGreen, cli.ColorReset,
			cli.ColorBold, cli.ColorReset,
			cli.ColorYellow, cli.ColorReset,
			cli.ColorYellow, cli.ColorReset,
			cli.ColorYellow, cli.ColorReset,
			cli.ColorYellow, cli.ColorReset,
			cli.ColorYellow, cli.ColorReset,
			cli.ColorYellow, cli.ColorReset,
			cli.ColorYellow, cli.ColorReset,
			cli.ColorBold, cli.ColorReset,
		)
}
func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

func isInvalidKeyErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(strings.ToLower(s), "invalid_api_key") || strings.Contains(strings.ToLower(s), "incorrect api key")
}
